package organization

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/infrastructure/shared"
)

// projectRepository implements orgDomain.ProjectRepository using GORM
type projectRepository struct {
	db *gorm.DB
}

// NewProjectRepository creates a new project repository instance
func NewProjectRepository(db *gorm.DB) orgDomain.ProjectRepository {
	return &projectRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *projectRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new project
func (r *projectRepository) Create(ctx context.Context, project *orgDomain.Project) error {
	return r.getDB(ctx).WithContext(ctx).Create(project).Error
}

// GetByID retrieves a project by ID
func (r *projectRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Project, error) {
	var project orgDomain.Project
	err := r.getDB(ctx).WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get project by ID %s: %w", id, orgDomain.ErrProjectNotFound)
		}
		return nil, err
	}
	return &project, nil
}

// GetBySlug retrieves a project by organization and slug
func (r *projectRepository) GetBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*orgDomain.Project, error) {
	var project orgDomain.Project
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND slug = ? AND deleted_at IS NULL", orgID, slug).
		First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get project by org %s and slug %s: %w", orgID, slug, orgDomain.ErrProjectNotFound)
		}
		return nil, err
	}
	return &project, nil
}

// Update updates a project
func (r *projectRepository) Update(ctx context.Context, project *orgDomain.Project) error {
	return r.getDB(ctx).WithContext(ctx).Save(project).Error
}

// Delete soft deletes a project
func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Model(&orgDomain.Project{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// GetByOrganizationID retrieves all projects in an organization
func (r *projectRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Project, error) {
	var projects []*orgDomain.Project
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND deleted_at IS NULL", orgID).
		Order("created_at ASC").
		Find(&projects).Error
	return projects, err
}

// CountByOrganization counts projects in an organization
func (r *projectRepository) CountByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Project{}).
		Where("organization_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

// GetProjectCount returns the count of projects in an organization
func (r *projectRepository) GetProjectCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Project{}).
		Where("organization_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return int(count), err
}

// CanUserAccessProject checks if a user has access to a project
func (r *projectRepository) CanUserAccessProject(ctx context.Context, userID, projectID uuid.UUID) (bool, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Table("projects").
		Joins("JOIN organization_members ON projects.organization_id = organization_members.organization_id").
		Where("projects.id = ? AND organization_members.user_id = ? AND projects.deleted_at IS NULL", projectID, userID).
		Count(&count).Error
	return count > 0, err
}
