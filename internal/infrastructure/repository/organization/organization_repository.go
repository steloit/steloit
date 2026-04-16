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
	"brokle/pkg/pagination"
)

// organizationRepository implements orgDomain.OrganizationRepository using GORM
type organizationRepository struct {
	db *gorm.DB
}

// NewOrganizationRepository creates a new organization repository instance
func NewOrganizationRepository(db *gorm.DB) orgDomain.OrganizationRepository {
	return &organizationRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *organizationRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new organization
func (r *organizationRepository) Create(ctx context.Context, org *orgDomain.Organization) error {
	return r.getDB(ctx).WithContext(ctx).Create(org).Error
}

// GetByID retrieves an organization by ID
func (r *organizationRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Organization, error) {
	var org orgDomain.Organization
	err := r.getDB(ctx).WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get organization by ID %s: %w", id, orgDomain.ErrNotFound)
		}
		return nil, err
	}
	return &org, nil
}

// GetBySlug retrieves an organization by slug
func (r *organizationRepository) GetBySlug(ctx context.Context, slug string) (*orgDomain.Organization, error) {
	var org orgDomain.Organization
	err := r.getDB(ctx).WithContext(ctx).Where("slug = ? AND deleted_at IS NULL", slug).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get organization by slug %s: %w", slug, orgDomain.ErrNotFound)
		}
		return nil, err
	}
	return &org, nil
}

// Update updates an organization
func (r *organizationRepository) Update(ctx context.Context, org *orgDomain.Organization) error {
	return r.getDB(ctx).WithContext(ctx).Save(org).Error
}

// Delete soft deletes an organization
func (r *organizationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Model(&orgDomain.Organization{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// List retrieves organizations with cursor pagination
func (r *organizationRepository) List(ctx context.Context, filters *orgDomain.OrganizationFilters) ([]*orgDomain.Organization, error) {
	var orgs []*orgDomain.Organization

	query := r.getDB(ctx).WithContext(ctx).Where("deleted_at IS NULL")

	// Apply filters
	if filters != nil {
		if filters.Name != nil {
			query = query.Where("name LIKE ?", "%"+*filters.Name+"%")
		}
		if filters.Plan != nil {
			query = query.Where("plan = ?", *filters.Plan)
		}
		if filters.Status != nil {
			query = query.Where("status = ?", *filters.Status)
		}
	}

	// Determine sort field and direction with validation
	allowedSortFields := []string{"created_at", "updated_at", "name", "id"}
	sortField := "created_at" // default
	sortDir := "DESC"

	if filters != nil {
		// Validate sort field against whitelist
		if filters.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filters.Params.SortBy, allowedSortFields)
			if err != nil {
				return nil, err
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filters.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}

	// Apply sorting with secondary sort on id for stable ordering
	query = query.Order(fmt.Sprintf("%s %s, id %s", sortField, sortDir, sortDir))

	// Apply limit and offset for pagination
	limit := pagination.DefaultPageSize
	offset := 0
	if filters != nil {
		if filters.Params.Limit > 0 {
			limit = filters.Params.Limit
		}
		offset = filters.Params.GetOffset()
	}
	query = query.Limit(limit).Offset(offset)

	err := query.Find(&orgs).Error
	return orgs, err
}

// GetOrganizationsByUserID retrieves organizations for a user
func (r *organizationRepository) GetOrganizationsByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Organization, error) {
	var orgs []*orgDomain.Organization
	err := r.getDB(ctx).WithContext(ctx).
		Table("organizations").
		Select("organizations.*").
		Joins("JOIN organization_members ON organizations.id = organization_members.organization_id").
		Where("organization_members.user_id = ? AND organizations.deleted_at IS NULL", userID).
		Order("organizations.created_at DESC").
		Find(&orgs).Error
	return orgs, err
}

// orgProjectRow represents a row from the batch query joining orgs, projects, and roles
type orgProjectRow struct {
	OrgCreatedAt   time.Time
	OrgUpdatedAt   time.Time
	ProjectID      *uuid.UUID
	ProjectName    *string
	ProjectDesc    *string
	ProjectOrgID   *uuid.UUID
	ProjectCreated *time.Time
	ProjectUpdated *time.Time
	ProjectStatus  *string
	OrgName        string
	OrgPlan        string
	RoleName       string
	OrgID          uuid.UUID
}

// GetUserOrganizationsWithProjectsBatch fetches all user's organizations with nested projects in a single optimized query
func (r *organizationRepository) GetUserOrganizationsWithProjectsBatch(
	ctx context.Context,
	userID uuid.UUID,
) ([]*orgDomain.OrganizationWithProjectsAndRole, error) {
	var rows []orgProjectRow

	err := r.getDB(ctx).WithContext(ctx).
		Table("organizations").
		Select(`
			organizations.id as org_id,
			organizations.name as org_name,
			organizations.plan as org_plan,
			organizations.created_at as org_created_at,
			organizations.updated_at as org_updated_at,
			roles.name as role_name,
			projects.id as project_id,
			projects.name as project_name,
			projects.description as project_desc,
			projects.organization_id as project_org_id,
			projects.created_at as project_created,
			projects.updated_at as project_updated,
			projects.status as project_status
		`).
		Joins("INNER JOIN organization_members ON organization_members.organization_id = organizations.id").
		Joins("INNER JOIN roles ON roles.id = organization_members.role_id").
		Joins("LEFT JOIN projects ON projects.organization_id = organizations.id AND projects.deleted_at IS NULL").
		Where("organization_members.user_id = ? AND organizations.deleted_at IS NULL", userID).
		Order("organizations.created_at DESC, projects.created_at DESC").
		Scan(&rows).Error

	if err != nil {
		return nil, err
	}

	return groupByOrganization(rows), nil
}

// groupByOrganization converts flattened SQL results into hierarchical structure
// Preserves SQL ORDER BY by tracking insertion order
func groupByOrganization(rows []orgProjectRow) []*orgDomain.OrganizationWithProjectsAndRole {
	orgMap := make(map[uuid.UUID]*orgDomain.OrganizationWithProjectsAndRole)
	orgOrder := make([]uuid.UUID, 0) // Track insertion order to preserve SQL ORDER BY

	for _, row := range rows {
		// Create organization entry if doesn't exist
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
			orgOrder = append(orgOrder, row.OrgID) // Track order
		}

		// Add project if all required fields are non-NULL
		if row.ProjectID != nil && row.ProjectName != nil && row.ProjectOrgID != nil {
			// Check for duplicates (SQL JOIN can create duplicate rows)
			isDuplicate := false
			for _, existingProj := range orgMap[row.OrgID].Projects {
				if existingProj.ID == *row.ProjectID {
					isDuplicate = true
					break
				}
			}

			if !isDuplicate {
				// Guard optional fields with fallbacks
				description := ""
				if row.ProjectDesc != nil {
					description = *row.ProjectDesc
				}

				createdAt := time.Time{}
				if row.ProjectCreated != nil {
					createdAt = *row.ProjectCreated
				}

				updatedAt := time.Time{}
				if row.ProjectUpdated != nil {
					updatedAt = *row.ProjectUpdated
				}

				project := &orgDomain.Project{
					ID:             *row.ProjectID,
					Name:           *row.ProjectName,
					Description:    description,
					OrganizationID: *row.ProjectOrgID,
					Status:         *row.ProjectStatus,
					CreatedAt:      createdAt,
					UpdatedAt:      updatedAt,
				}
				orgMap[row.OrgID].Projects = append(orgMap[row.OrgID].Projects, project)
			}
		}
	}

	// Convert map to slice in preserved order
	result := make([]*orgDomain.OrganizationWithProjectsAndRole, 0, len(orgOrder))
	for _, orgID := range orgOrder {
		result = append(result, orgMap[orgID])
	}

	return result
}
