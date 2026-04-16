package observability

import (
	"context"
	"errors"
	"log/slog"

	"brokle/internal/core/domain/observability"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"

	"gorm.io/gorm"
)

// FilterPresetService handles filter preset business logic.
type FilterPresetService struct {
	repo   observability.FilterPresetRepository
	logger *slog.Logger
}

// NewFilterPresetService creates a new filter preset service.
func NewFilterPresetService(
	repo observability.FilterPresetRepository,
	logger *slog.Logger,
) *FilterPresetService {
	return &FilterPresetService{
		repo:   repo,
		logger: logger,
	}
}

// Create creates a new filter preset.
func (s *FilterPresetService) Create(ctx context.Context, projectID string, userID string, req *observability.CreateFilterPresetRequest) (*observability.FilterPreset, error) {
	if validationErrs := observability.ValidateCreateFilterPresetRequest(req); len(validationErrs) > 0 {
		return nil, appErrors.NewValidationError(validationErrs[0].Field, validationErrs[0].Message)
	}

	exists, err := s.repo.ExistsByName(ctx, projectID, req.Name, nil)
	if err != nil {
		s.logger.Error("failed to check preset name",
			"error", err,
			"project_id", projectID,
			"name", req.Name,
		)
		return nil, appErrors.NewInternalError("failed to check preset name", err)
	}
	if exists {
		return nil, appErrors.NewConflictError("a filter preset with this name already exists")
	}

	preset := &observability.FilterPreset{
		ID:               uid.New().String(),
		ProjectID:        projectID,
		Name:             req.Name,
		Description:      req.Description,
		TargetTable:      req.TargetTable,
		Filters:          req.Filters,
		ColumnOrder:      req.ColumnOrder,
		ColumnVisibility: req.ColumnVisibility,
		SearchQuery:      req.SearchQuery,
		SearchTypes:      observability.StringArray(req.SearchTypes),
		IsPublic:         req.IsPublic,
		CreatedBy:        &userID,
	}

	if err := s.repo.Create(ctx, preset); err != nil {
		s.logger.Error("failed to create filter preset",
			"error", err,
			"project_id", projectID,
			"name", req.Name,
		)
		return nil, appErrors.NewInternalError("failed to create filter preset", err)
	}

	s.logger.Info("filter preset created",
		"preset_id", preset.ID,
		"project_id", projectID,
		"name", preset.Name,
		"user_id", userID,
	)

	return preset, nil
}

func (s *FilterPresetService) GetByID(ctx context.Context, projectID string, id string, userID string) (*observability.FilterPreset, error) {
	preset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, appErrors.NewNotFoundError("filter preset " + id)
		}
		return nil, appErrors.NewInternalError("failed to get filter preset", err)
	}

	// Verify project scoping - return 404 to avoid information leakage
	if preset.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError("filter preset " + id)
	}

	// Check access: user can access their own presets or public presets
	if !preset.IsPublic && (preset.CreatedBy == nil || *preset.CreatedBy != userID) {
		return nil, appErrors.NewForbiddenError("access to filter preset denied")
	}

	return preset, nil
}

// Update updates a filter preset.
func (s *FilterPresetService) Update(ctx context.Context, projectID string, id string, userID string, req *observability.UpdateFilterPresetRequest) (*observability.FilterPreset, error) {
	// Get existing preset
	preset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, appErrors.NewNotFoundError("filter preset " + id)
		}
		return nil, appErrors.NewInternalError("failed to get filter preset", err)
	}

	// Verify project scoping - return 404 to avoid information leakage
	if preset.ProjectID != projectID {
		return nil, appErrors.NewNotFoundError("filter preset " + id)
	}

	// Check ownership
	if preset.CreatedBy == nil || *preset.CreatedBy != userID {
		return nil, appErrors.NewForbiddenError("only the owner can update this preset")
	}

	// Check for duplicate name if name is being changed
	if req.Name != nil && *req.Name != preset.Name {
		exists, err := s.repo.ExistsByName(ctx, preset.ProjectID, *req.Name, &id)
		if err != nil {
			return nil, appErrors.NewInternalError("failed to check preset name", err)
		}
		if exists {
			return nil, appErrors.NewConflictError("a filter preset with this name already exists")
		}
		preset.Name = *req.Name
	}

	// Apply updates
	if req.Description != nil {
		preset.Description = req.Description
	}
	if req.Filters != nil {
		preset.Filters = req.Filters
	}
	if req.ColumnOrder != nil {
		preset.ColumnOrder = req.ColumnOrder
	}
	if req.ColumnVisibility != nil {
		preset.ColumnVisibility = req.ColumnVisibility
	}
	if req.SearchQuery != nil {
		preset.SearchQuery = req.SearchQuery
	}
	if req.SearchTypes != nil {
		preset.SearchTypes = observability.StringArray(req.SearchTypes)
	}
	if req.IsPublic != nil {
		preset.IsPublic = *req.IsPublic
	}

	if err := s.repo.Update(ctx, preset); err != nil {
		s.logger.Error("failed to update filter preset",
			"error", err,
			"preset_id", id,
		)
		return nil, appErrors.NewInternalError("failed to update filter preset", err)
	}

	s.logger.Info("filter preset updated",
		"preset_id", id,
		"user_id", userID,
	)

	return preset, nil
}

// Delete deletes a filter preset.
func (s *FilterPresetService) Delete(ctx context.Context, projectID string, id string, userID string) error {
	// Get existing preset
	preset, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return appErrors.NewNotFoundError("filter preset " + id)
		}
		return appErrors.NewInternalError("failed to get filter preset", err)
	}

	// Verify project scoping - return 404 to avoid information leakage
	if preset.ProjectID != projectID {
		return appErrors.NewNotFoundError("filter preset " + id)
	}

	// Check ownership
	if preset.CreatedBy == nil || *preset.CreatedBy != userID {
		return appErrors.NewForbiddenError("only the owner can delete this preset")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.Error("failed to delete filter preset",
			"error", err,
			"preset_id", id,
		)
		return appErrors.NewInternalError("failed to delete filter preset", err)
	}

	s.logger.Info("filter preset deleted",
		"preset_id", id,
		"user_id", userID,
	)

	return nil
}

// List retrieves filter presets for a project.
// Returns both user's own presets and public presets if includePublic is true.
func (s *FilterPresetService) List(ctx context.Context, projectID string, userID string, tableName *string, includePublic bool) ([]*observability.FilterPreset, error) {
	filter := &observability.FilterPresetFilter{
		ProjectID:   projectID,
		TargetTable: tableName,
		UserID:      userID,
		IncludeAll:  includePublic,
		Limit:       100,
	}

	presets, err := s.repo.List(ctx, filter)
	if err != nil {
		s.logger.Error("failed to list filter presets",
			"error", err,
			"project_id", projectID,
		)
		return nil, appErrors.NewInternalError("failed to list filter presets", err)
	}

	return presets, nil
}
