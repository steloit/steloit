package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/common"
	promptDomain "brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
)

// Name validation pattern: starts with letter, alphanumeric + underscore + hyphen
var namePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// Label validation pattern: lowercase alphanumeric + dots + dashes + underscores
var labelPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]*$`)

// Default cache TTL
const defaultCacheTTL = 60 * time.Second

type promptService struct {
	transactor         common.Transactor
	promptRepo         promptDomain.PromptRepository
	versionRepo        promptDomain.VersionRepository
	labelRepo          promptDomain.LabelRepository
	protectedLabelRepo promptDomain.ProtectedLabelRepository
	cacheRepo          promptDomain.CacheRepository
	compiler           promptDomain.CompilerService
	logger             *slog.Logger
}

func NewPromptService(
	transactor common.Transactor,
	promptRepo promptDomain.PromptRepository,
	versionRepo promptDomain.VersionRepository,
	labelRepo promptDomain.LabelRepository,
	protectedLabelRepo promptDomain.ProtectedLabelRepository,
	cacheRepo promptDomain.CacheRepository,
	compiler promptDomain.CompilerService,
	logger *slog.Logger,
) promptDomain.PromptService {
	return &promptService{
		transactor:         transactor,
		promptRepo:         promptRepo,
		versionRepo:        versionRepo,
		labelRepo:          labelRepo,
		protectedLabelRepo: protectedLabelRepo,
		cacheRepo:          cacheRepo,
		compiler:           compiler,
		logger:             logger,
	}
}

func (s *promptService) CreatePrompt(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *promptDomain.CreatePromptRequest) (*promptDomain.Prompt, *promptDomain.Version, []string, error) {
	if !namePattern.MatchString(req.Name) {
		return nil, nil, nil, appErrors.NewValidationError("name", "must start with letter and contain only alphanumeric, underscore, and hyphen")
	}

	promptType := req.Type
	if promptType == "" {
		promptType = s.inferPromptType(req.Template)
	}

	if err := s.compiler.ValidateTemplate(req.Template, promptType); err != nil {
		return nil, nil, nil, appErrors.NewValidationError("template", err.Error())
	}

	variables, err := s.compiler.ExtractVariables(req.Template, promptType)
	if err != nil {
		return nil, nil, nil, appErrors.NewValidationError("template", err.Error())
	}

	templateJSON, err := json.Marshal(req.Template)
	if err != nil {
		return nil, nil, nil, appErrors.NewInternalError("failed to marshal template", err)
	}

	// Validate all labels for format and protection (BEFORE transaction to fail fast)
	for _, labelName := range req.Labels {
		if labelName == promptDomain.LabelLatest {
			continue
		}
		if !labelPattern.MatchString(labelName) {
			return nil, nil, nil, appErrors.NewValidationError("labels", fmt.Sprintf("invalid label name: %s", labelName))
		}

		// CRITICAL: Check if label is protected (fail-closed for security)
		isProtected, err := s.protectedLabelRepo.IsProtected(ctx, projectID, labelName)
		if err != nil {
			return nil, nil, nil, appErrors.NewInternalError("failed to check label protection", err)
		}
		if isProtected {
			return nil, nil, nil, appErrors.NewForbiddenError(fmt.Sprintf("label '%s' is protected and requires admin permissions", labelName))
		}
	}

	prompt := promptDomain.NewPrompt(projectID, req.Name, promptType, req.Description, req.Tags)
	version := promptDomain.NewVersion(prompt.ID, 1, templateJSON, req.Config, variables, req.CommitMessage, userID)

	// TRANSACTION: Create prompt, version, and labels atomically
	err = s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := s.promptRepo.Create(ctx, prompt); err != nil {
			if appErrors.IsUniqueViolation(err) {
				return appErrors.NewConflictError(fmt.Sprintf("prompt '%s' already exists in this project", req.Name))
			}
			return appErrors.NewInternalError("failed to create prompt", err)
		}

		if err := s.versionRepo.Create(ctx, version); err != nil {
			return appErrors.NewInternalError("failed to create version", err)
		}

		if err := s.labelRepo.SetLabel(ctx, prompt.ID, version.ID, promptDomain.LabelLatest, userID); err != nil {
			return appErrors.NewInternalError("failed to create latest label", err)
		}

		for _, labelName := range req.Labels {
			if labelName == promptDomain.LabelLatest {
				continue
			}
			if err := s.labelRepo.SetLabel(ctx, prompt.ID, version.ID, labelName, userID); err != nil {
				return appErrors.NewInternalError(fmt.Sprintf("failed to create label '%s'", labelName), err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, nil, err
	}

	createdLabels := []string{promptDomain.LabelLatest}
	for _, labelName := range req.Labels {
		if labelName != promptDomain.LabelLatest {
			createdLabels = append(createdLabels, labelName)
		}
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
		s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
	}
	s.logger.Info("prompt created", "prompt_id", prompt.ID, "name", prompt.Name, "project_id", projectID)

	return prompt, version, createdLabels, nil
}

func (s *promptService) GetPrompt(ctx context.Context, projectID uuid.UUID, name string, opts *promptDomain.GetPromptOptions) (*promptDomain.PromptResponse, error) {
	label := promptDomain.LabelLatest
	if opts != nil && opts.Label != "" {
		label = opts.Label
	}

	var cacheKey string
	if opts != nil && opts.Version != nil {
		cacheKey = s.cacheRepo.BuildKey(projectID, name, fmt.Sprintf("v%d", *opts.Version))
	} else {
		cacheKey = s.cacheRepo.BuildKey(projectID, name, label)
	}

	if opts == nil || !opts.BypassCache {
		cached, err := s.cacheRepo.Get(ctx, cacheKey)
		if err == nil {
			return s.cachedPromptToResponse(cached), nil
		}
	}

	prompt, err := s.promptRepo.GetByName(ctx, projectID, name)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt '%s'", name))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	var version *promptDomain.Version
	if opts != nil && opts.Version != nil {
		version, err = s.versionRepo.GetByPromptAndVersion(ctx, prompt.ID, *opts.Version)
		if err != nil {
			if promptDomain.IsNotFoundError(err) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %d of prompt '%s'", *opts.Version, name))
			}
			return nil, appErrors.NewInternalError("failed to get version", err)
		}
	} else {
		labelEntity, err := s.labelRepo.GetByPromptAndName(ctx, prompt.ID, label)
		if err != nil {
			if promptDomain.IsNotFoundError(err) {
				return nil, appErrors.NewNotFoundError(fmt.Sprintf("label '%s' for prompt '%s'", label, name))
			}
			return nil, appErrors.NewInternalError("failed to get label", err)
		}
		version, err = s.versionRepo.GetByID(ctx, labelEntity.VersionID)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to get version by label", err)
		}
	}

	labels, err := s.labelRepo.ListByVersion(ctx, version.ID)
	if err != nil {
		s.logger.Warn("failed to get labels for version", "error", err)
		labels = nil
	}

	response := s.buildPromptResponse(prompt, version, labels)

	ttl := defaultCacheTTL
	if opts != nil && opts.CacheTTL != nil {
		ttl = time.Duration(*opts.CacheTTL) * time.Second
	}
	if ttl > 0 {
		cached := s.responseToCachedPrompt(response)
		if err := s.cacheRepo.Set(ctx, cacheKey, cached, ttl); err != nil {
			s.logger.Warn("failed to cache prompt", "error", err)
		}
	}

	return response, nil
}

func (s *promptService) GetPromptByID(ctx context.Context, projectID, promptID uuid.UUID) (*promptDomain.Prompt, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	return prompt, nil
}

func (s *promptService) UpdatePrompt(ctx context.Context, projectID, promptID uuid.UUID, req *promptDomain.UpdatePromptRequest) (*promptDomain.Prompt, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	oldName := prompt.Name
	nameChanged := false

	if req.Name != nil {
		if !namePattern.MatchString(*req.Name) {
			return nil, appErrors.NewValidationError("name", "must start with letter and contain only alphanumeric, underscore, and hyphen")
		}
		if *req.Name != oldName {
			nameChanged = true
		}
		prompt.Name = *req.Name
	}
	if req.Description != nil {
		prompt.Description = *req.Description
	}
	if req.Tags != nil {
		prompt.Tags = req.Tags
	}

	prompt.UpdatedAt = time.Now()

	if err := s.promptRepo.Update(ctx, prompt); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return nil, appErrors.NewConflictError(fmt.Sprintf("prompt '%s' already exists", *req.Name))
		}
		return nil, appErrors.NewInternalError("failed to update prompt", err)
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, oldName); err != nil {
		s.logger.Warn("failed to invalidate cache for old name", "project_id", prompt.ProjectID, "name", oldName, "error", err)
	}

	if nameChanged {
		if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
			s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
		}
	}

	return prompt, nil
}

func (s *promptService) DeletePrompt(ctx context.Context, projectID, promptID uuid.UUID) error {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	if err := s.promptRepo.SoftDelete(ctx, promptID); err != nil {
		return appErrors.NewInternalError("failed to delete prompt", err)
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
		s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
	}

	s.logger.Info("prompt deleted", "prompt_id", promptID, "name", prompt.Name)

	return nil
}

func (s *promptService) ListPrompts(ctx context.Context, projectID uuid.UUID, filters *promptDomain.PromptFilters) ([]*promptDomain.PromptListItem, int64, error) {
	prompts, total, err := s.promptRepo.ListByProject(ctx, projectID, filters)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("failed to list prompts", err)
	}

	if len(prompts) == 0 {
		return []*promptDomain.PromptListItem{}, total, nil
	}

	promptIDs := make([]uuid.UUID, len(prompts))
	for i, p := range prompts {
		promptIDs[i] = p.ID
	}

	latestVersions, err := s.versionRepo.GetLatestByPrompts(ctx, promptIDs)
	if err != nil {
		s.logger.Warn("failed to batch fetch latest versions", "error", err)
	}
	versionMap := make(map[uuid.UUID]*promptDomain.Version)
	for _, v := range latestVersions {
		versionMap[v.PromptID] = v
	}

	allLabels, err := s.labelRepo.ListByPrompts(ctx, promptIDs)
	if err != nil {
		s.logger.Warn("failed to batch fetch labels", "error", err)
	}
	labelMap := make(map[uuid.UUID][]*promptDomain.Label)
	for _, l := range allLabels {
		labelMap[l.PromptID] = append(labelMap[l.PromptID], l)
	}

	versionIDsSet := make(map[uuid.UUID]bool)
	for _, labelList := range labelMap {
		for _, label := range labelList {
			versionIDsSet[label.VersionID] = true
		}
	}

	versionIDs := make([]uuid.UUID, 0, len(versionIDsSet))
	for versionID := range versionIDsSet {
		versionIDs = append(versionIDs, versionID)
	}

	labelVersionMap := make(map[uuid.UUID]int)
	if len(versionIDs) > 0 {
		labelVersions, err := s.versionRepo.GetByIDs(ctx, versionIDs)
		if err != nil {
			s.logger.Warn("failed to batch fetch label versions", "error", err)
		}
		for _, v := range labelVersions {
			labelVersionMap[v.ID] = v.Version
		}
	}

	items := make([]*promptDomain.PromptListItem, 0, len(prompts))
	for _, prompt := range prompts {
		item := &promptDomain.PromptListItem{
			ID:          prompt.ID.String(),
			Name:        prompt.Name,
			Type:        prompt.Type,
			Description: prompt.Description,
			Tags:        []string(prompt.Tags),
			CreatedAt:   prompt.CreatedAt,
			UpdatedAt:   prompt.UpdatedAt,
		}

		if v, ok := versionMap[prompt.ID]; ok {
			item.LatestVersion = v.Version
		}

		if labels, ok := labelMap[prompt.ID]; ok {
			item.Labels = make([]promptDomain.PromptListLabelInfo, 0, len(labels))
			for _, label := range labels {
				if versionNum, ok := labelVersionMap[label.VersionID]; ok {
					item.Labels = append(item.Labels, promptDomain.PromptListLabelInfo{
						Name:    label.Name,
						Version: versionNum,
					})
				}
			}
		}

		items = append(items, item)
	}

	return items, total, nil
}

func (s *promptService) UpsertPrompt(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, req *promptDomain.UpsertPromptRequest) (*promptDomain.UpsertResponse, error) {
	prompt, err := s.promptRepo.GetByName(ctx, projectID, req.Name)
	if err != nil {
		if !promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewInternalError("failed to check prompt existence", err)
		}

		createReq := &promptDomain.CreatePromptRequest{
			Name:          req.Name,
			Type:          req.Type,
			Description:   req.Description,
			Tags:          req.Tags,
			Template:      req.Template,
			Config:        req.Config,
			Labels:        req.Labels,
			CommitMessage: req.CommitMessage,
		}

		newPrompt, version, labels, err := s.CreatePrompt(ctx, projectID, userID, createReq)
		if err != nil {
			return nil, err
		}

		return &promptDomain.UpsertResponse{
			ID:          newPrompt.ID.String(),
			Name:        newPrompt.Name,
			Type:        newPrompt.Type,
			Version:     version.Version,
			IsNewPrompt: true,
			Labels:      labels,
			CreatedAt:   version.CreatedAt,
		}, nil
	}

	versionReq := &promptDomain.CreateVersionRequest{
		Template:      req.Template,
		Config:        req.Config,
		Labels:        req.Labels,
		CommitMessage: req.CommitMessage,
	}

	version, labels, err := s.CreateVersion(ctx, projectID, prompt.ID, userID, versionReq)
	if err != nil {
		return nil, err
	}

	return &promptDomain.UpsertResponse{
		ID:          prompt.ID.String(),
		Name:        prompt.Name,
		Type:        prompt.Type,
		Version:     version.Version,
		IsNewPrompt: false,
		Labels:      labels,
		CreatedAt:   version.CreatedAt,
	}, nil
}

func (s *promptService) CreateVersion(ctx context.Context, projectID, promptID uuid.UUID, userID *uuid.UUID, req *promptDomain.CreateVersionRequest) (*promptDomain.Version, []string, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	if err := s.compiler.ValidateTemplate(req.Template, prompt.Type); err != nil {
		return nil, nil, appErrors.NewValidationError("template", err.Error())
	}

	variables, err := s.compiler.ExtractVariables(req.Template, prompt.Type)
	if err != nil {
		return nil, nil, appErrors.NewValidationError("template", err.Error())
	}

	templateJSON, err := json.Marshal(req.Template)
	if err != nil {
		return nil, nil, appErrors.NewInternalError("failed to marshal template", err)
	}

	// Validate labels before transaction to fail fast
	for _, labelName := range req.Labels {
		if labelName == promptDomain.LabelLatest {
			continue
		}
		if !labelPattern.MatchString(labelName) {
			return nil, nil, appErrors.NewValidationError("labels", fmt.Sprintf("invalid label name: %s", labelName))
		}

		// CRITICAL: Check if label is protected (fail-closed for security)
		isProtected, err := s.protectedLabelRepo.IsProtected(ctx, prompt.ProjectID, labelName)
		if err != nil {
			return nil, nil, appErrors.NewInternalError("failed to check label protection", err)
		}
		if isProtected {
			return nil, nil, appErrors.NewForbiddenError(fmt.Sprintf("label '%s' is protected and requires admin permissions", labelName))
		}
	}

	var version *promptDomain.Version

	// TRANSACTION: Get version number + create version + update labels atomically
	err = s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		versionNum, err := s.versionRepo.GetNextVersionNumber(ctx, promptID)
		if err != nil {
			return appErrors.NewInternalError("failed to get next version number", err)
		}

		version = promptDomain.NewVersion(promptID, versionNum, templateJSON, req.Config, variables, req.CommitMessage, userID)

		if err := s.versionRepo.Create(ctx, version); err != nil {
			return appErrors.NewInternalError("failed to create version", err)
		}

		if err := s.labelRepo.SetLabel(ctx, promptID, version.ID, promptDomain.LabelLatest, userID); err != nil {
			return appErrors.NewInternalError("failed to update latest label", err)
		}

		for _, labelName := range req.Labels {
			if labelName == promptDomain.LabelLatest {
				continue
			}
			if err := s.labelRepo.SetLabel(ctx, promptID, version.ID, labelName, userID); err != nil {
				return appErrors.NewInternalError(fmt.Sprintf("failed to create label '%s'", labelName), err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, nil, err
	}

	createdLabels := []string{promptDomain.LabelLatest}
	for _, labelName := range req.Labels {
		if labelName != promptDomain.LabelLatest {
			createdLabels = append(createdLabels, labelName)
		}
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
		s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
	}
	s.logger.Info("version created", "prompt_id", promptID, "version", version.Version)

	return version, createdLabels, nil
}

func (s *promptService) GetVersion(ctx context.Context, projectID, promptID uuid.UUID, version int) (*promptDomain.VersionResponse, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	v, err := s.versionRepo.GetByPromptAndVersion(ctx, promptID, version)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %d", version))
		}
		return nil, appErrors.NewInternalError("failed to get version", err)
	}

	// Fetch labels separately (single query, no N+1)
	labels, err := s.labelRepo.ListByVersion(ctx, v.ID)
	if err != nil {
		s.logger.Warn("failed to fetch version labels", "error", err, "version_id", v.ID)
		labels = []*promptDomain.Label{}
	}

	labelNames := make([]string, 0, len(labels))
	for _, l := range labels {
		labelNames = append(labelNames, l.Name)
	}

	return s.buildVersionResponseWithLabels(v, labelNames)
}

func (s *promptService) GetVersionEntity(ctx context.Context, projectID, promptID, versionID uuid.UUID) (*promptDomain.Version, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	version, err := s.versionRepo.GetByID(ctx, versionID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
		}
		return nil, appErrors.NewInternalError("failed to get version", err)
	}

	// CRITICAL: Validate version belongs to prompt
	if version.PromptID != promptID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
	}

	return version, nil
}

func (s *promptService) GetVersionByID(ctx context.Context, projectID, promptID, versionID uuid.UUID) (*promptDomain.VersionResponse, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	version, err := s.versionRepo.GetByID(ctx, versionID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
		}
		return nil, appErrors.NewInternalError("failed to get version", err)
	}

	// CRITICAL: Validate version belongs to prompt
	if version.PromptID != promptID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
	}

	labels, err := s.labelRepo.ListByVersion(ctx, version.ID)
	if err != nil {
		s.logger.Warn("failed to fetch version labels", "error", err, "version_id", version.ID)
		labels = []*promptDomain.Label{}
	}

	labelNames := make([]string, 0, len(labels))
	for _, l := range labels {
		labelNames = append(labelNames, l.Name)
	}

	return s.buildVersionResponseWithLabels(version, labelNames)
}

func (s *promptService) ListVersions(ctx context.Context, projectID, promptID uuid.UUID) ([]*promptDomain.VersionResponse, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	versions, err := s.versionRepo.ListByPrompt(ctx, promptID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to list versions", err)
	}

	if len(versions) == 0 {
		return []*promptDomain.VersionResponse{}, nil
	}

	versionIDs := make([]uuid.UUID, len(versions))
	for i, v := range versions {
		versionIDs[i] = v.ID
	}

	allLabels, err := s.labelRepo.ListByVersions(ctx, versionIDs)
	if err != nil {
		s.logger.Warn("failed to batch fetch version labels", "error", err)
	}

	labelMap := make(map[uuid.UUID][]string)
	for _, l := range allLabels {
		labelMap[l.VersionID] = append(labelMap[l.VersionID], l.Name)
	}

	responses := make([]*promptDomain.VersionResponse, 0, len(versions))
	for _, v := range versions {
		resp, err := s.buildVersionResponseWithLabels(v, labelMap[v.ID])
		if err != nil {
			continue
		}
		responses = append(responses, resp)
	}

	return responses, nil
}

func (s *promptService) GetVersionDiff(ctx context.Context, projectID, promptID uuid.UUID, fromVersion, toVersion int) (*promptDomain.VersionDiffResponse, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	from, err := s.versionRepo.GetByPromptAndVersion(ctx, promptID, fromVersion)
	if err != nil {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %d", fromVersion))
	}

	to, err := s.versionRepo.GetByPromptAndVersion(ctx, promptID, toVersion)
	if err != nil {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %d", toVersion))
	}

	var templateFrom, templateTo interface{}
	json.Unmarshal(from.Template, &templateFrom)
	json.Unmarshal(to.Template, &templateTo)

	fromVars := make(map[string]bool)
	for _, v := range from.Variables {
		fromVars[v] = true
	}
	toVars := make(map[string]bool)
	for _, v := range to.Variables {
		toVars[v] = true
	}

	added := make([]string, 0)
	removed := make([]string, 0)
	for _, v := range to.Variables {
		if !fromVars[v] {
			added = append(added, v)
		}
	}
	for _, v := range from.Variables {
		if !toVars[v] {
			removed = append(removed, v)
		}
	}

	return &promptDomain.VersionDiffResponse{
		FromVersion:      fromVersion,
		ToVersion:        toVersion,
		TemplateFrom:     templateFrom,
		TemplateTo:       templateTo,
		VariablesAdded:   added,
		VariablesRemoved: removed,
	}, nil
}

func (s *promptService) SetLabels(ctx context.Context, projectID, promptID, versionID uuid.UUID, userID *uuid.UUID, labels []string) ([]string, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	version, err := s.versionRepo.GetByID(ctx, versionID)
	if err != nil {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("version %s", versionID))
	}

	// CRITICAL: Validate version belongs to prompt
	if version.PromptID != promptID {
		return nil, appErrors.NewValidationError("version_id", "version does not belong to this prompt")
	}

	currentLabels, err := s.labelRepo.ListByVersion(ctx, versionID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get current labels", err)
	}

	newLabelSet := make(map[string]bool)
	for _, labelName := range labels {
		if labelName != promptDomain.LabelLatest {
			newLabelSet[labelName] = true
		}
	}

	// Remove labels not in new set ("latest" is auto-managed, never removed)
	for _, currentLabel := range currentLabels {
		if currentLabel.Name == promptDomain.LabelLatest {
			continue
		}
		if !newLabelSet[currentLabel.Name] {
			if err := s.labelRepo.RemoveLabel(ctx, promptID, currentLabel.Name); err != nil {
				return nil, appErrors.NewInternalError(fmt.Sprintf("failed to remove label %s", currentLabel.Name), err)
			}
		}
	}

	for _, labelName := range labels {
		if labelName == promptDomain.LabelLatest {
			continue
		}
		if !labelPattern.MatchString(labelName) {
			return nil, appErrors.NewValidationError("labels", fmt.Sprintf("invalid label name: %s", labelName))
		}

		// CRITICAL: Check if label is protected (fail-closed for security)
		isProtected, err := s.protectedLabelRepo.IsProtected(ctx, prompt.ProjectID, labelName)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check label protection", err)
		}
		if isProtected {
			return nil, appErrors.NewForbiddenError(fmt.Sprintf("label '%s' is protected and requires admin permissions to modify", labelName))
		}

		if err := s.labelRepo.SetLabel(ctx, promptID, versionID, labelName, userID); err != nil {
			return nil, appErrors.NewInternalError(fmt.Sprintf("failed to set label %s", labelName), err)
		}
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
		s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
	}

	// Return the final label state for this version
	finalLabels, err := s.labelRepo.ListByVersion(ctx, versionID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get final labels", err)
	}
	labelNames := make([]string, len(finalLabels))
	for i, l := range finalLabels {
		labelNames[i] = l.Name
	}
	return labelNames, nil
}

func (s *promptService) RemoveLabel(ctx context.Context, projectID, promptID uuid.UUID, userID *uuid.UUID, labelName string) error {
	if labelName == promptDomain.LabelLatest {
		return appErrors.NewValidationError("label", "'latest' label cannot be removed")
	}

	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	if err := s.labelRepo.RemoveLabel(ctx, promptID, labelName); err != nil {
		if promptDomain.IsNotFoundError(err) {
			return appErrors.NewNotFoundError(fmt.Sprintf("label '%s'", labelName))
		}
		return appErrors.NewInternalError("failed to remove label", err)
	}

	if err := s.InvalidateCache(ctx, prompt.ProjectID, prompt.Name); err != nil {
		s.logger.Warn("failed to invalidate cache", "project_id", prompt.ProjectID, "name", prompt.Name, "error", err)
	}

	return nil
}

func (s *promptService) GetVersionByLabel(ctx context.Context, projectID, promptID uuid.UUID, label string) (*promptDomain.Version, error) {
	prompt, err := s.promptRepo.GetByID(ctx, promptID)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
		}
		return nil, appErrors.NewInternalError("failed to get prompt", err)
	}

	// CRITICAL: Validate project ownership
	if prompt.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError(fmt.Sprintf("prompt %s", promptID))
	}

	labelEntity, err := s.labelRepo.GetByPromptAndName(ctx, promptID, label)
	if err != nil {
		if promptDomain.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("label '%s'", label))
		}
		return nil, appErrors.NewInternalError("failed to get label", err)
	}

	version, err := s.versionRepo.GetByID(ctx, labelEntity.VersionID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get version", err)
	}

	return version, nil
}

func (s *promptService) GetProtectedLabels(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	labels, err := s.protectedLabelRepo.ListByProject(ctx, projectID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get protected labels", err)
	}

	result := make([]string, 0, len(labels))
	for _, l := range labels {
		result = append(result, l.LabelName)
	}

	return result, nil
}

func (s *promptService) SetProtectedLabels(ctx context.Context, projectID uuid.UUID, userID *uuid.UUID, labels []string) ([]string, error) {
	for _, labelName := range labels {
		if !labelPattern.MatchString(labelName) {
			return nil, appErrors.NewValidationError("protected_labels", fmt.Sprintf("invalid label name: %s", labelName))
		}
	}

	if err := s.protectedLabelRepo.SetProtectedLabels(ctx, projectID, labels, userID); err != nil {
		return nil, appErrors.NewInternalError("failed to set protected labels", err)
	}

	return labels, nil
}

func (s *promptService) IsLabelProtected(ctx context.Context, projectID uuid.UUID, labelName string) (bool, error) {
	return s.protectedLabelRepo.IsProtected(ctx, projectID, labelName)
}

func (s *promptService) InvalidateCache(ctx context.Context, projectID uuid.UUID, promptName string) error {
	pattern := fmt.Sprintf("prompt:%s:%s:*", projectID.String(), promptName)
	return s.cacheRepo.DeleteByPattern(ctx, pattern)
}

func (s *promptService) inferPromptType(template interface{}) promptDomain.PromptType {
	if m, ok := template.(map[string]interface{}); ok {
		if _, hasMessages := m["messages"]; hasMessages {
			return promptDomain.PromptTypeChat
		}
	}
	return promptDomain.PromptTypeText
}

func (s *promptService) buildPromptResponse(prompt *promptDomain.Prompt, version *promptDomain.Version, labels []*promptDomain.Label) *promptDomain.PromptResponse {
	var template interface{}
	json.Unmarshal(version.Template, &template)

	labelNames := make([]string, 0, len(labels))
	for _, l := range labels {
		labelNames = append(labelNames, l.Name)
	}

	var createdBy string
	if version.CreatedBy != nil {
		createdBy = version.CreatedBy.String()
	}

	// Detect dialect from template content
	dialect, _ := s.compiler.DetectDialect(template, prompt.Type)

	return &promptDomain.PromptResponse{
		ID:            prompt.ID.String(),
		ProjectID:     prompt.ProjectID.String(),
		Name:          prompt.Name,
		Type:          prompt.Type,
		Description:   prompt.Description,
		Tags:          []string(prompt.Tags),
		Version:       version.Version,
		VersionID:     version.ID.String(),
		Labels:        labelNames,
		Template:      template,
		Config:        version.Config,
		Variables:     []string(version.Variables),
		Dialect:       dialect,
		CommitMessage: version.CommitMessage,
		CreatedAt:     version.CreatedAt,
		UpdatedAt:     prompt.UpdatedAt,
		CreatedBy:     createdBy,
	}
}

// buildVersionResponseWithLabels builds a version response with preloaded labels
// This avoids the N+1 query problem when listing multiple versions
func (s *promptService) buildVersionResponseWithLabels(v *promptDomain.Version, labels []string) (*promptDomain.VersionResponse, error) {
	var template interface{}
	if err := json.Unmarshal(v.Template, &template); err != nil {
		return nil, err
	}

	if labels == nil {
		labels = []string{}
	}

	var createdBy string
	if v.CreatedBy != nil {
		createdBy = v.CreatedBy.String()
	}

	// Detect dialect from template content (infer type from template structure)
	promptType := s.inferPromptType(template)
	dialect, _ := s.compiler.DetectDialect(template, promptType)

	return &promptDomain.VersionResponse{
		ID:            v.ID.String(),
		Version:       v.Version,
		Template:      template,
		Config:        v.Config,
		Variables:     []string(v.Variables),
		Dialect:       dialect,
		CommitMessage: v.CommitMessage,
		Labels:        labels,
		CreatedAt:     v.CreatedAt,
		CreatedBy:     createdBy,
	}, nil
}

func (s *promptService) cachedPromptToResponse(cached *promptDomain.CachedPrompt) *promptDomain.PromptResponse {
	// Detect dialect from template content
	dialect, _ := s.compiler.DetectDialect(cached.Template, cached.Type)

	return &promptDomain.PromptResponse{
		ID:            cached.PromptID,
		ProjectID:     cached.ProjectID,
		Name:          cached.Name,
		Type:          cached.Type,
		Description:   cached.Description,
		Tags:          cached.Tags,
		Version:       cached.Version,
		VersionID:     cached.VersionID,
		Labels:        cached.Labels,
		Template:      cached.Template,
		Config:        cached.Config,
		Variables:     cached.Variables,
		Dialect:       dialect,
		CommitMessage: cached.CommitMessage,
		CreatedAt:     cached.CreatedAt,
		UpdatedAt:     cached.UpdatedAt,
		CreatedBy:     cached.CreatedBy,
	}
}

func (s *promptService) responseToCachedPrompt(resp *promptDomain.PromptResponse) *promptDomain.CachedPrompt {
	return &promptDomain.CachedPrompt{
		PromptID:      resp.ID,
		ProjectID:     resp.ProjectID,
		Name:          resp.Name,
		Type:          resp.Type,
		Description:   resp.Description,
		Tags:          resp.Tags,
		Version:       resp.Version,
		VersionID:     resp.VersionID,
		Labels:        resp.Labels,
		Template:      resp.Template,
		Config:        resp.Config,
		Variables:     resp.Variables,
		CommitMessage: resp.CommitMessage,
		CreatedAt:     resp.CreatedAt,
		UpdatedAt:     resp.UpdatedAt,
		CreatedBy:     resp.CreatedBy,
	}
}

