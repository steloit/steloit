// Package playground provides service implementations for playground session management.
package playground

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	credentialsDomain "brokle/internal/core/domain/credentials"
	playgroundDomain "brokle/internal/core/domain/playground"
	promptDomain "brokle/internal/core/domain/prompt"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type playgroundService struct {
	repo               playgroundDomain.SessionRepository
	credentialsService credentialsDomain.ProviderCredentialService
	compilerService    promptDomain.CompilerService
	executionService   promptDomain.ExecutionService
	logger             *slog.Logger
}

func NewPlaygroundService(
	repo playgroundDomain.SessionRepository,
	credentialsService credentialsDomain.ProviderCredentialService,
	compilerService promptDomain.CompilerService,
	executionService promptDomain.ExecutionService,
	logger *slog.Logger,
) playgroundDomain.PlaygroundService {
	return &playgroundService{
		repo:               repo,
		credentialsService: credentialsService,
		compilerService:    compilerService,
		executionService:   executionService,
		logger:             logger,
	}
}

// All sessions are saved (no ephemeral sessions).
func (s *playgroundService) CreateSession(ctx context.Context, req *playgroundDomain.CreateSessionRequest) (*playgroundDomain.SessionResponse, error) {
	if req.Name == "" {
		return nil, appErrors.NewValidationError("Name required", "name is required")
	}
	if len(req.Name) > playgroundDomain.MaxNameLength {
		return nil, appErrors.NewValidationError("Name too long", "name must be 200 characters or less")
	}

	if len(req.Windows) == 0 {
		return nil, appErrors.NewValidationError("Windows required", "windows must be provided")
	}

	if len(req.Tags) > playgroundDomain.MaxTagsCount {
		return nil, appErrors.NewValidationError("Too many tags", "maximum 10 tags allowed")
	}

	now := time.Now()

	var variables playgroundDomain.JSON
	if len(req.Variables) > 0 {
		variables = playgroundDomain.JSON(req.Variables)
	} else {
		variables = playgroundDomain.JSON([]byte("{}"))
	}

	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	name := req.Name
	session := &playgroundDomain.Session{
		ID:          uid.New(),
		ProjectID:   req.ProjectID,
		Name:        &name,
		Description: req.Description,
		Tags:        tags,
		Variables:   variables,
		Config:      playgroundDomain.JSON(req.Config),
		Windows:     playgroundDomain.JSON(req.Windows),
		CreatedBy:   req.CreatedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
		LastUsedAt:  now,
	}

	if err := s.repo.Create(ctx, session); err != nil {
		s.logger.Error("failed to create playground session",
			"error", err,
			"project_id", req.ProjectID,
		)
		return nil, appErrors.NewInternalError("Failed to create session", err)
	}

	s.logger.Info("playground session created",
		"session_id", session.ID,
		"project_id", req.ProjectID,
		"name", req.Name,
	)

	return session.ToResponse(), nil
}

func (s *playgroundService) GetSession(ctx context.Context, sessionID uuid.UUID) (*playgroundDomain.SessionResponse, error) {
	session, err := s.repo.GetByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, playgroundDomain.ErrSessionNotFound) {
			return nil, appErrors.NewNotFoundError("Session not found")
		}
		return nil, appErrors.NewInternalError("Failed to retrieve session", err)
	}

	return session.ToResponse(), nil
}

func (s *playgroundService) ListSessions(ctx context.Context, req *playgroundDomain.ListSessionsRequest) ([]*playgroundDomain.SessionSummary, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var sessions []*playgroundDomain.Session
	var err error

	if len(req.Tags) > 0 {
		sessions, err = s.repo.ListByTags(ctx, req.ProjectID, req.Tags, limit)
	} else {
		sessions, err = s.repo.List(ctx, req.ProjectID, limit)
	}

	if err != nil {
		return nil, appErrors.NewInternalError("Failed to list sessions", err)
	}

	summaries := make([]*playgroundDomain.SessionSummary, len(sessions))
	for i, session := range sessions {
		summaries[i] = session.ToSummary()
	}

	return summaries, nil
}

func (s *playgroundService) UpdateSession(ctx context.Context, req *playgroundDomain.UpdateSessionRequest) (*playgroundDomain.SessionResponse, error) {
	session, err := s.repo.GetByID(ctx, req.SessionID)
	if err != nil {
		if errors.Is(err, playgroundDomain.ErrSessionNotFound) {
			return nil, appErrors.NewNotFoundError("Session not found")
		}
		return nil, appErrors.NewInternalError("Failed to retrieve session", err)
	}

	if req.Name != nil {
		if len(*req.Name) > playgroundDomain.MaxNameLength {
			return nil, appErrors.NewValidationError("Name too long", "name must be 200 characters or less")
		}
		session.Name = req.Name
	}
	if req.Description != nil {
		session.Description = req.Description
	}
	if req.Tags != nil {
		if len(req.Tags) > playgroundDomain.MaxTagsCount {
			return nil, appErrors.NewValidationError("Too many tags", "maximum 10 tags allowed")
		}
		session.Tags = req.Tags
	}

	if len(req.Variables) > 0 {
		session.Variables = playgroundDomain.JSON(req.Variables)
	}
	if len(req.Config) > 0 {
		session.Config = playgroundDomain.JSON(req.Config)
	}
	if len(req.Windows) > 0 {
		session.Windows = playgroundDomain.JSON(req.Windows)
	}

	session.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, session); err != nil {
		s.logger.Error("failed to update playground session",
			"error", err,
			"session_id", req.SessionID,
		)
		return nil, appErrors.NewInternalError("Failed to update session", err)
	}

	return session.ToResponse(), nil
}

func (s *playgroundService) DeleteSession(ctx context.Context, sessionID uuid.UUID) error {
	if err := s.repo.Delete(ctx, sessionID); err != nil {
		if errors.Is(err, playgroundDomain.ErrSessionNotFound) {
			return appErrors.NewNotFoundError("Session not found")
		}
		return appErrors.NewInternalError("Failed to delete session", err)
	}

	s.logger.Info("playground session deleted",
		"session_id", sessionID,
	)

	return nil
}

func (s *playgroundService) UpdateLastRun(ctx context.Context, req *playgroundDomain.UpdateLastRunRequest) error {
	if req.LastRun == nil {
		return appErrors.NewValidationError("Last run required", "last_run cannot be empty")
	}

	lastRunJSON, err := json.Marshal(req.LastRun)
	if err != nil {
		return appErrors.NewInternalError("Failed to serialize last run", err)
	}

	if err := s.repo.UpdateLastRun(ctx, req.SessionID, playgroundDomain.JSON(lastRunJSON)); err != nil {
		if errors.Is(err, playgroundDomain.ErrSessionNotFound) {
			return appErrors.NewNotFoundError("Session not found")
		}
		s.logger.Error("failed to update last run",
			"error", err,
			"session_id", req.SessionID,
		)
		return appErrors.NewInternalError("Failed to update last run", err)
	}

	return nil
}

func (s *playgroundService) UpdateWindows(ctx context.Context, sessionID uuid.UUID, windows json.RawMessage) error {
	if err := s.repo.UpdateWindows(ctx, sessionID, playgroundDomain.JSON(windows)); err != nil {
		if errors.Is(err, playgroundDomain.ErrSessionNotFound) {
			return appErrors.NewNotFoundError("Session not found")
		}
		s.logger.Error("failed to update windows",
			"error", err,
			"session_id", sessionID,
		)
		return appErrors.NewInternalError("Failed to update windows", err)
	}

	return nil
}

func (s *playgroundService) ValidateProjectAccess(ctx context.Context, sessionID uuid.UUID, projectID uuid.UUID) error {
	exists, err := s.repo.ExistsByProjectID(ctx, sessionID, projectID)
	if err != nil {
		return appErrors.NewInternalError("Failed to validate access", err)
	}
	if !exists {
		return appErrors.NewNotFoundError("Session not found")
	}
	return nil
}

// ExecutePrompt executes a prompt with full orchestration:
// credential resolution → variable extraction → execution → session update
func (s *playgroundService) ExecutePrompt(ctx context.Context, req *playgroundDomain.ExecuteRequest) (*playgroundDomain.ExecuteResponse, error) {
	startTime := time.Now()

	variables, err := s.compilerService.ExtractVariables(req.Template, req.PromptType)
	if err != nil {
		return nil, appErrors.NewValidationError("Invalid template", err.Error())
	}

	resolvedConfig, err := s.resolveCredentials(ctx, req.OrganizationID, req.ConfigOverrides)
	if err != nil {
		return nil, err
	}

	promptResp := &promptDomain.PromptResponse{
		Type:      req.PromptType,
		Template:  req.Template,
		Variables: variables,
	}

	execResp, err := s.executionService.Execute(ctx, promptResp, req.Variables, resolvedConfig)
	if err != nil {
		s.logger.Error("playground execution failed",
			"error", err,
			"project_id", req.ProjectID.String(),
			"organization_id", req.OrganizationID.String(),
		)
		return nil, appErrors.NewInternalError("Execution failed", err)
	}

	// Update session last_run (async, non-blocking)
	if req.SessionID != nil {
		go s.updateSessionLastRun(context.WithoutCancel(ctx), *req.SessionID, execResp, startTime)
	}

	s.logger.Debug("playground execution completed",
		"project_id", req.ProjectID.String(),
		"latency_ms", execResp.LatencyMs,
	)

	return &playgroundDomain.ExecuteResponse{
		CompiledPrompt: execResp.CompiledPrompt,
		Response:       execResp.Response,
		LatencyMs:      execResp.LatencyMs,
		Error:          execResp.Error,
	}, nil
}

func (s *playgroundService) StreamPrompt(ctx context.Context, req *playgroundDomain.StreamRequest) (*playgroundDomain.StreamResponse, error) {
	startTime := time.Now()

	variables, err := s.compilerService.ExtractVariables(req.Template, req.PromptType)
	if err != nil {
		return nil, appErrors.NewValidationError("Invalid template", err.Error())
	}

	resolvedConfig, err := s.resolveCredentials(ctx, req.OrganizationID, req.ConfigOverrides)
	if err != nil {
		return nil, err
	}

	promptResp := &promptDomain.PromptResponse{
		Type:      req.PromptType,
		Template:  req.Template,
		Variables: variables,
	}

	eventChan, resultChan, err := s.executionService.ExecuteStream(ctx, promptResp, req.Variables, resolvedConfig)
	if err != nil {
		s.logger.Error("playground stream execution failed",
			"error", err,
			"project_id", req.ProjectID.String(),
			"organization_id", req.OrganizationID.String(),
		)
		return nil, appErrors.NewInternalError("Stream execution failed", err)
	}

	// Wrap result channel to intercept for session update
	wrappedResultChan := s.wrapResultForSessionUpdate(ctx, req.SessionID, resultChan, startTime)

	return &playgroundDomain.StreamResponse{
		EventChan:  eventChan,
		ResultChan: wrappedResultChan,
	}, nil
}

// resolveCredentials resolves organization-scoped credentials for execution.
// Requires both provider and credential_id to be specified.
func (s *playgroundService) resolveCredentials(ctx context.Context, orgID uuid.UUID, overrides *promptDomain.ModelConfig) (*promptDomain.ModelConfig, error) {
	if overrides == nil {
		overrides = &promptDomain.ModelConfig{}
	}

	// Provider must be explicitly specified
	if overrides.Provider == "" {
		return nil, appErrors.NewValidationError("Provider required", "provider must be specified")
	}

	// Credential ID is required (no fallback to adapter-based lookup)
	if overrides.CredentialID == nil || *overrides.CredentialID == "" {
		return nil, appErrors.NewValidationError("Credential required", "credential_id must be specified")
	}

	if s.credentialsService == nil {
		return nil, appErrors.NewInternalError("Credentials service not configured", nil)
	}

	credID, err := uuid.Parse(*overrides.CredentialID)
	if err != nil {
		return nil, appErrors.NewValidationError("Invalid credential ID", "credential_id must be a valid UUID")
	}

	keyConfig, err := s.credentialsService.GetExecutionConfig(ctx, orgID, credID, credentialsDomain.Provider(overrides.Provider))
	if err != nil {
		// Handle specific errors for better UX
		if errors.Is(err, credentialsDomain.ErrAdapterMismatch) {
			return nil, appErrors.NewValidationError("Credential mismatch", err.Error())
		}
		if errors.Is(err, credentialsDomain.ErrCredentialNotFound) {
			return nil, appErrors.NewNotFoundError("Credential not found")
		}
		if errors.Is(err, credentialsDomain.ErrNoKeyConfigured) {
			return nil, appErrors.NewNotFoundError("No credentials configured")
		}
		return nil, appErrors.NewInternalError("Failed to resolve credentials", err)
	}

	overrides.APIKey = keyConfig.APIKey
	if keyConfig.BaseURL != "" {
		overrides.ResolvedBaseURL = &keyConfig.BaseURL
	}

	// Pass provider-specific config (Azure deployment_id, api_version) and custom headers
	overrides.ProviderConfig = keyConfig.Config
	overrides.CustomHeaders = keyConfig.Headers

	s.logger.Debug("credentials resolved",
		"organization_id", orgID.String(),
		"provider", overrides.Provider,
		"credential_id", overrides.CredentialID,
	)

	return overrides, nil
}

// wrapResultForSessionUpdate intercepts the result channel to update session.
func (s *playgroundService) wrapResultForSessionUpdate(
	ctx context.Context,
	sessionID *uuid.UUID,
	resultChan <-chan *promptDomain.StreamResult,
	startTime time.Time,
) <-chan *promptDomain.StreamResult {
	wrappedChan := make(chan *promptDomain.StreamResult, 1)

	go func() {
		defer close(wrappedChan)

		for result := range resultChan {
			wrappedChan <- result

			if sessionID != nil && result != nil {
				go s.updateStreamSessionLastRun(context.WithoutCancel(ctx), *sessionID, result, startTime)
			}
		}
	}()

	return wrappedChan
}

func (s *playgroundService) updateSessionLastRun(
	ctx context.Context,
	sessionID uuid.UUID,
	execResp *promptDomain.ExecutePromptResponse,
	startTime time.Time,
) {
	lastRun := &playgroundDomain.LastRun{
		Timestamp: startTime,
		Metrics: &playgroundDomain.RunMetrics{
			LatencyMs: execResp.LatencyMs,
		},
	}

	if execResp.Response != nil {
		lastRun.Content = execResp.Response.Content
		if execResp.Response.Usage != nil {
			lastRun.Metrics.PromptTokens = execResp.Response.Usage.PromptTokens
			lastRun.Metrics.CompletionTokens = execResp.Response.Usage.CompletionTokens
			lastRun.Metrics.TotalTokens = execResp.Response.Usage.TotalTokens
		}
		if execResp.Response.Cost != nil {
			lastRun.Metrics.Cost = *execResp.Response.Cost
		}
		lastRun.Metrics.Model = execResp.Response.Model
	}

	if execResp.Error != "" {
		errStr := execResp.Error
		lastRun.Error = &errStr
	}

	req := &playgroundDomain.UpdateLastRunRequest{
		SessionID: sessionID,
		LastRun:   lastRun,
	}

	if err := s.UpdateLastRun(ctx, req); err != nil {
		s.logger.Warn("failed to update session last_run",
			"session_id", sessionID.String(),
			"error", err,
		)
	}
}

func (s *playgroundService) updateStreamSessionLastRun(
	ctx context.Context,
	sessionID uuid.UUID,
	result *promptDomain.StreamResult,
	startTime time.Time,
) {
	lastRun := &playgroundDomain.LastRun{
		Timestamp: startTime,
		Content:   result.Content,
		Metrics: &playgroundDomain.RunMetrics{
			LatencyMs: result.TotalDuration,
			Model:     result.Model,
		},
	}

	if result.Usage != nil {
		lastRun.Metrics.PromptTokens = result.Usage.PromptTokens
		lastRun.Metrics.CompletionTokens = result.Usage.CompletionTokens
		lastRun.Metrics.TotalTokens = result.Usage.TotalTokens
	}

	if result.Cost != nil {
		lastRun.Metrics.Cost = *result.Cost
	}

	if result.TTFTMs != nil {
		lastRun.Metrics.TTFTMs = int64(*result.TTFTMs)
	}

	req := &playgroundDomain.UpdateLastRunRequest{
		SessionID: sessionID,
		LastRun:   lastRun,
	}

	if err := s.UpdateLastRun(ctx, req); err != nil {
		s.logger.Warn("failed to update session last_run",
			"session_id", sessionID.String(),
			"error", err,
		)
	}
}
