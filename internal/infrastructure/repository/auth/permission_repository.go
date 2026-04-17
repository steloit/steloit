package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// permissionRepository is the pgx+sqlc implementation of
// authDomain.PermissionRepository. Dynamic ILIKE-based search lives in
// permission_filter.go (squirrel).
type permissionRepository struct {
	tm *db.TxManager
}

// NewPermissionRepository returns the pgx-backed repository.
func NewPermissionRepository(tm *db.TxManager) authDomain.PermissionRepository {
	return &permissionRepository{tm: tm}
}

// ----- CRUD ----------------------------------------------------------

func (r *permissionRepository) Create(ctx context.Context, p *authDomain.Permission) error {
	if err := r.tm.Queries(ctx).CreatePermission(ctx, gen.CreatePermissionParams{
		ID:          p.ID,
		Name:        p.Name,
		Resource:    p.Resource,
		Action:      p.Action,
		Description: emptyToNilString(p.Description),
		ScopeLevel:  string(p.ScopeLevel),
		Category:    emptyToNilString(p.Category),
		CreatedAt:   p.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create permission %s: %w", p.Name, err)
	}
	return nil
}

func (r *permissionRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.Permission, error) {
	row, err := r.tm.Queries(ctx).GetPermissionByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get permission %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get permission %s: %w", id, err)
	}
	return permissionFromRow(&row), nil
}

func (r *permissionRepository) GetByName(ctx context.Context, name string) (*authDomain.Permission, error) {
	row, err := r.tm.Queries(ctx).GetPermissionByName(ctx, name)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get permission by name %s: %w", name, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get permission by name %s: %w", name, err)
	}
	return permissionFromRow(&row), nil
}

func (r *permissionRepository) GetByResourceAction(ctx context.Context, resource, action string) (*authDomain.Permission, error) {
	row, err := r.tm.Queries(ctx).GetPermissionByResourceAction(ctx, gen.GetPermissionByResourceActionParams{
		Resource: resource,
		Action:   action,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get permission %s:%s: %w", resource, action, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get permission %s:%s: %w", resource, action, err)
	}
	return permissionFromRow(&row), nil
}

func (r *permissionRepository) Update(ctx context.Context, p *authDomain.Permission) error {
	if err := r.tm.Queries(ctx).UpdatePermission(ctx, gen.UpdatePermissionParams{
		ID:          p.ID,
		Name:        p.Name,
		Resource:    p.Resource,
		Action:      p.Action,
		Description: emptyToNilString(p.Description),
		ScopeLevel:  string(p.ScopeLevel),
		Category:    emptyToNilString(p.Category),
	}); err != nil {
		return fmt.Errorf("update permission %s: %w", p.ID, err)
	}
	return nil
}

func (r *permissionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeletePermission(ctx, id); err != nil {
		return fmt.Errorf("delete permission %s: %w", id, err)
	}
	return nil
}

// ----- Simple listings ----------------------------------------------

func (r *permissionRepository) GetAll(ctx context.Context) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListAllPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	return permissionsFromRows(rows), nil
}

// GetAllPermissions is a legacy alias kept for the domain interface.
func (r *permissionRepository) GetAllPermissions(ctx context.Context) ([]*authDomain.Permission, error) {
	return r.GetAll(ctx)
}

func (r *permissionRepository) GetByNames(ctx context.Context, names []string) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByNames(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("list permissions by names: %w", err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetByResource(ctx context.Context, resource string) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByResource(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("list permissions by resource %s: %w", resource, err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetByResourceActions(ctx context.Context, resourceActions []string) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByResourceActions(ctx, resourceActions)
	if err != nil {
		return nil, fmt.Errorf("list permissions by resource_actions: %w", err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetByScopeLevel(ctx context.Context, scopeLevel authDomain.ScopeLevel) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByScopeLevel(ctx, string(scopeLevel))
	if err != nil {
		return nil, fmt.Errorf("list permissions by scope %s: %w", scopeLevel, err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetByCategory(ctx context.Context, category string) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByCategory(ctx, &category)
	if err != nil {
		return nil, fmt.Errorf("list permissions by category %s: %w", category, err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetByScopeLevelAndCategory(ctx context.Context, scopeLevel authDomain.ScopeLevel, category string) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsByScopeAndCategory(ctx, gen.ListPermissionsByScopeAndCategoryParams{
		ScopeLevel: string(scopeLevel),
		Category:   &category,
	})
	if err != nil {
		return nil, fmt.Errorf("list permissions by scope %s and category %s: %w", scopeLevel, category, err)
	}
	return permissionsFromRows(rows), nil
}

// ----- Aggregate helpers --------------------------------------------

func (r *permissionRepository) GetAvailableResources(ctx context.Context) ([]string, error) {
	resources, err := r.tm.Queries(ctx).ListDistinctResources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list distinct resources: %w", err)
	}
	return resources, nil
}

func (r *permissionRepository) GetActionsForResource(ctx context.Context, resource string) ([]string, error) {
	actions, err := r.tm.Queries(ctx).ListActionsForResource(ctx, resource)
	if err != nil {
		return nil, fmt.Errorf("list actions for resource %s: %w", resource, err)
	}
	return actions, nil
}

func (r *permissionRepository) GetPermissionCategories(ctx context.Context) ([]string, error) {
	cats, err := r.tm.Queries(ctx).ListDistinctCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list distinct categories: %w", err)
	}
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		if c != nil {
			out = append(out, *c)
		}
	}
	return out, nil
}

// GetAvailableCategories aliases GetPermissionCategories for the domain interface.
func (r *permissionRepository) GetAvailableCategories(ctx context.Context) ([]string, error) {
	return r.GetPermissionCategories(ctx)
}

// ----- Role / user permission joins ---------------------------------

func (r *permissionRepository) GetPermissionsByRoleID(ctx context.Context, roleID uuid.UUID) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsForRole(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions for role %s: %w", roleID, err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetUserPermissions(ctx context.Context, userID, orgID uuid.UUID) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListUserPermissionsInOrg(ctx, gen.ListUserPermissionsInOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list user permissions (user=%s org=%s): %w", userID, orgID, err)
	}
	return permissionsFromRows(rows), nil
}

func (r *permissionRepository) GetUserPermissionStrings(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	strs, err := r.tm.Queries(ctx).ListUserResourceActionsInOrg(ctx, gen.ListUserResourceActionsInOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list user resource_actions (user=%s org=%s): %w", userID, orgID, err)
	}
	return strs, nil
}

func (r *permissionRepository) GetUserEffectivePermissions(ctx context.Context, userID, orgID uuid.UUID) (map[string]bool, error) {
	strs, err := r.GetUserPermissionStrings(ctx, userID, orgID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(strs))
	for _, ra := range strs {
		result[ra] = true
	}
	return result, nil
}

func (r *permissionRepository) GetRolePermissionMap(ctx context.Context, roleID uuid.UUID) (map[string]bool, error) {
	strs, err := r.tm.Queries(ctx).ListResourceActionsForRolePermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list resource_actions for role %s: %w", roleID, err)
	}
	result := make(map[string]bool, len(strs))
	for _, ra := range strs {
		result[ra] = true
	}
	return result, nil
}

func (r *permissionRepository) GetUserPermissionsByAPIKey(ctx context.Context, apiKeyID uuid.UUID) ([]string, error) {
	names, err := r.tm.Queries(ctx).ListPermissionNamesForAPIKey(ctx, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("list permission names for api_key %s: %w", apiKeyID, err)
	}
	return names, nil
}

// ----- Existence checks ---------------------------------------------

func (r *permissionRepository) ValidateResourceAction(ctx context.Context, resource, action string) error {
	if resource == "" {
		return fmt.Errorf("validate permission resource: %w", authDomain.ErrInvalidCredentials)
	}
	if action == "" {
		return fmt.Errorf("validate permission action: %w", authDomain.ErrInvalidCredentials)
	}
	return nil
}

func (r *permissionRepository) PermissionExists(ctx context.Context, resource, action string) (bool, error) {
	ok, err := r.tm.Queries(ctx).ExistsPermissionByResourceAction(ctx, gen.ExistsPermissionByResourceActionParams{
		Resource: resource,
		Action:   action,
	})
	if err != nil {
		return false, fmt.Errorf("check permission %s:%s: %w", resource, action, err)
	}
	return ok, nil
}

func (r *permissionRepository) BulkPermissionExists(ctx context.Context, resourceActions []string) (map[string]bool, error) {
	existing, err := r.tm.Queries(ctx).ListExistingResourceActions(ctx, resourceActions)
	if err != nil {
		return nil, fmt.Errorf("list existing resource_actions: %w", err)
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, ra := range existing {
		existingSet[ra] = struct{}{}
	}
	result := make(map[string]bool, len(resourceActions))
	for _, ra := range resourceActions {
		_, ok := existingSet[ra]
		result[ra] = ok
	}
	return result, nil
}

// ----- Bulk writes --------------------------------------------------

func (r *permissionRepository) BulkCreate(ctx context.Context, permissions []*authDomain.Permission) error {
	if len(permissions) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, p := range permissions {
			if err := q.CreatePermission(ctx, gen.CreatePermissionParams{
				ID:          p.ID,
				Name:        p.Name,
				Resource:    p.Resource,
				Action:      p.Action,
				Description: emptyToNilString(p.Description),
				ScopeLevel:  string(p.ScopeLevel),
				Category:    emptyToNilString(p.Category),
				CreatedAt:   p.CreatedAt,
			}); err != nil {
				return fmt.Errorf("bulk-create permission %s: %w", p.Name, err)
			}
		}
		return nil
	})
}

func (r *permissionRepository) BulkUpdate(ctx context.Context, permissions []*authDomain.Permission) error {
	if len(permissions) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, p := range permissions {
			if err := q.UpdatePermission(ctx, gen.UpdatePermissionParams{
				ID:          p.ID,
				Name:        p.Name,
				Resource:    p.Resource,
				Action:      p.Action,
				Description: emptyToNilString(p.Description),
				ScopeLevel:  string(p.ScopeLevel),
				Category:    emptyToNilString(p.Category),
			}); err != nil {
				return fmt.Errorf("bulk-update permission %s: %w", p.ID, err)
			}
		}
		return nil
	})
}

func (r *permissionRepository) BulkDelete(ctx context.Context, permissionIDs []uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}
	if _, err := r.tm.Queries(ctx).DeletePermissionsIn(ctx, permissionIDs); err != nil {
		return fmt.Errorf("bulk-delete permissions: %w", err)
	}
	return nil
}

// ----- gen ↔ domain boundary ----------------------------------------

func permissionFromRow(row *gen.Permission) *authDomain.Permission {
	return &authDomain.Permission{
		ID:          row.ID,
		Name:        row.Name,
		Resource:    row.Resource,
		Action:      row.Action,
		Description: derefString(row.Description),
		ScopeLevel:  authDomain.ScopeLevel(row.ScopeLevel),
		Category:    derefString(row.Category),
		CreatedAt:   row.CreatedAt,
	}
}

func permissionsFromRows(rows []gen.Permission) []*authDomain.Permission {
	out := make([]*authDomain.Permission, 0, len(rows))
	for i := range rows {
		out = append(out, permissionFromRow(&rows[i]))
	}
	return out
}
