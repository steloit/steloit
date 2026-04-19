package organization

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// projectRepository is the pgx+sqlc implementation of
// orgDomain.ProjectRepository.
type projectRepository struct {
	tm *db.TxManager
}

// NewProjectRepository returns the pgx-backed repository.
func NewProjectRepository(tm *db.TxManager) orgDomain.ProjectRepository {
	return &projectRepository{tm: tm}
}

func (r *projectRepository) Create(ctx context.Context, project *orgDomain.Project) error {
	if err := r.tm.Queries(ctx).CreateProject(ctx, gen.CreateProjectParams{
		ID:             project.ID,
		OrganizationID: project.OrganizationID,
		Name:           project.Name,
		Description:    project.Description,
		Status:         project.Status,
		CreatedAt:      project.CreatedAt,
		UpdatedAt:      project.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

func (r *projectRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Project, error) {
	row, err := r.tm.Queries(ctx).GetProjectByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get project by ID %s: %w", id, orgDomain.ErrProjectNotFound)
		}
		return nil, fmt.Errorf("get project by ID %s: %w", id, err)
	}
	return projectFromRow(&row), nil
}

// GetBySlug is retained by the domain interface but the slug column was
// dropped in migration 20251101020000_refactor_onboarding_to_signup.
// Returns ErrProjectNotFound to surface the deprecation without panicking.
func (r *projectRepository) GetBySlug(ctx context.Context, orgID uuid.UUID, slug string) (*orgDomain.Project, error) {
	return nil, fmt.Errorf("get project by org %s and slug %s: %w", orgID, slug, orgDomain.ErrProjectNotFound)
}

func (r *projectRepository) Update(ctx context.Context, project *orgDomain.Project) error {
	if err := r.tm.Queries(ctx).UpdateProject(ctx, gen.UpdateProjectParams{
		ID:          project.ID,
		Name:        project.Name,
		Description: project.Description,
		Status:      project.Status,
	}); err != nil {
		return fmt.Errorf("update project %s: %w", project.ID, err)
	}
	return nil
}

func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteProject(ctx, id); err != nil {
		return fmt.Errorf("soft-delete project %s: %w", id, err)
	}
	return nil
}

func (r *projectRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Project, error) {
	rows, err := r.tm.Queries(ctx).ListProjectsByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list projects for org %s: %w", orgID, err)
	}
	out := make([]*orgDomain.Project, 0, len(rows))
	for i := range rows {
		out = append(out, projectFromRow(&rows[i]))
	}
	return out, nil
}

func (r *projectRepository) CountByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error) {
	n, err := r.tm.Queries(ctx).CountProjectsByOrganization(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("count projects for org %s: %w", orgID, err)
	}
	return n, nil
}

// GetProjectCount is a legacy alias that returns int instead of int64.
func (r *projectRepository) GetProjectCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	n, err := r.CountByOrganization(ctx, orgID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *projectRepository) CanUserAccessProject(ctx context.Context, userID, projectID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).UserCanAccessProject(ctx, gen.UserCanAccessProjectParams{
		ID:     projectID,
		UserID: userID,
	})
	if err != nil {
		return false, fmt.Errorf("check project access (user=%s project=%s): %w", userID, projectID, err)
	}
	return ok, nil
}

// projectFromRow adapts a sqlc-generated row to the domain type.
func projectFromRow(row *gen.Project) *orgDomain.Project {
	return &orgDomain.Project{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		Name:           row.Name,
		Description:    row.Description,
		Status:         row.Status,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		DeletedAt:      row.DeletedAt,
	}
}
