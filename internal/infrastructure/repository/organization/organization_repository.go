package organization

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// organizationRepository is the pgx+sqlc implementation of
// orgDomain.OrganizationRepository. Dynamic-filter listing lives in
// organization_filter.go (squirrel).
type organizationRepository struct {
	tm *db.TxManager
}

// NewOrganizationRepository returns the pgx-backed repository.
func NewOrganizationRepository(tm *db.TxManager) orgDomain.OrganizationRepository {
	return &organizationRepository{tm: tm}
}

func (r *organizationRepository) Create(ctx context.Context, org *orgDomain.Organization) error {
	if err := r.tm.Queries(ctx).CreateOrganization(ctx, gen.CreateOrganizationParams{
		ID:                 org.ID,
		Name:               org.Name,
		BillingEmail:       emptyToNilString(org.BillingEmail),
		Plan:               org.Plan,
		SubscriptionStatus: org.SubscriptionStatus,
		TrialEndsAt:        org.TrialEndsAt,
		CreatedAt:          org.CreatedAt,
		UpdatedAt:          org.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create organization: %w", err)
	}
	return nil
}

func (r *organizationRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Organization, error) {
	row, err := r.tm.Queries(ctx).GetOrganizationByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get organization by ID %s: %w", id, orgDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get organization by ID %s: %w", id, err)
	}
	return organizationFromRow(&row), nil
}

// GetBySlug is kept on the interface but the slug column was dropped in
// migration 20251101020000_refactor_onboarding_to_signup. Returning
// ErrNotFound preserves the caller contract while making the deprecation
// visible.
func (r *organizationRepository) GetBySlug(ctx context.Context, slug string) (*orgDomain.Organization, error) {
	return nil, fmt.Errorf("get organization by slug %s: %w", slug, orgDomain.ErrNotFound)
}

func (r *organizationRepository) Update(ctx context.Context, org *orgDomain.Organization) error {
	if err := r.tm.Queries(ctx).UpdateOrganization(ctx, gen.UpdateOrganizationParams{
		ID:                 org.ID,
		Name:               org.Name,
		BillingEmail:       emptyToNilString(org.BillingEmail),
		Plan:               org.Plan,
		SubscriptionStatus: org.SubscriptionStatus,
		TrialEndsAt:        org.TrialEndsAt,
	}); err != nil {
		return fmt.Errorf("update organization %s: %w", org.ID, err)
	}
	return nil
}

func (r *organizationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteOrganization(ctx, id); err != nil {
		return fmt.Errorf("soft-delete organization %s: %w", id, err)
	}
	return nil
}

func (r *organizationRepository) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Organization, error) {
	rows, err := r.tm.Queries(ctx).ListOrganizationsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list organizations for user %s: %w", userID, err)
	}
	return organizationsFromRows(rows), nil
}

// GetUserOrganizationsWithProjectsBatch fans out (org, role, project) in a
// single query and groups the result in Go. Preserves the order emitted
// by the SQL ORDER BY (org.created_at DESC, project.created_at DESC).
func (r *organizationRepository) GetUserOrganizationsWithProjectsBatch(
	ctx context.Context,
	userID uuid.UUID,
) ([]*orgDomain.OrganizationWithProjectsAndRole, error) {
	rows, err := r.tm.Queries(ctx).ListUserOrganizationsWithProjects(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user organizations with projects %s: %w", userID, err)
	}

	// Preserve insertion order so the hierarchical result keeps SQL's sort.
	orgMap := make(map[uuid.UUID]*orgDomain.OrganizationWithProjectsAndRole, len(rows))
	orgOrder := make([]uuid.UUID, 0, len(rows))

	for _, row := range rows {
		if _, exists := orgMap[row.OrgID]; !exists {
			orgMap[row.OrgID] = &orgDomain.OrganizationWithProjectsAndRole{
				Organization: &orgDomain.Organization{
					ID:        row.OrgID,
					Name:      row.OrgName,
					Plan:      row.OrgPlan,
					CreatedAt: row.OrgCreatedAt,
					UpdatedAt: row.OrgUpdatedAt,
				},
				Projects: []*orgDomain.Project{},
				RoleName: row.RoleName,
			}
			orgOrder = append(orgOrder, row.OrgID)
		}

		// LEFT JOIN rows for orgs with no projects emit NULL in every
		// project_* column — skip those.
		if row.ProjectID == nil {
			continue
		}

		// Deduplicate: rows fan out when an org has multiple roles (rare
		// but the join allows it).
		duplicate := false
		for _, existing := range orgMap[row.OrgID].Projects {
			if existing.ID == *row.ProjectID {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}

		orgMap[row.OrgID].Projects = append(orgMap[row.OrgID].Projects, &orgDomain.Project{
			ID:             *row.ProjectID,
			Name:           derefString(row.ProjectName),
			Description:    derefString(row.ProjectDescription),
			OrganizationID: derefUUID(row.ProjectOrganizationID),
			Status:         derefString(row.ProjectStatus),
			CreatedAt:      derefTime(row.ProjectCreatedAt),
			UpdatedAt:      derefTime(row.ProjectUpdatedAt),
		})
	}

	out := make([]*orgDomain.OrganizationWithProjectsAndRole, 0, len(orgOrder))
	for _, id := range orgOrder {
		out = append(out, orgMap[id])
	}
	return out, nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func organizationFromRow(row *gen.Organization) *orgDomain.Organization {
	return &orgDomain.Organization{
		ID:                 row.ID,
		Name:               row.Name,
		BillingEmail:       derefString(row.BillingEmail),
		Plan:               row.Plan,
		SubscriptionStatus: row.SubscriptionStatus,
		TrialEndsAt:        row.TrialEndsAt,
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
		DeletedAt:          row.DeletedAt,
	}
}

func organizationsFromRows(rows []gen.Organization) []*orgDomain.Organization {
	out := make([]*orgDomain.Organization, 0, len(rows))
	for i := range rows {
		out = append(out, organizationFromRow(&rows[i]))
	}
	return out
}
