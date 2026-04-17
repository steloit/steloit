package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// roleRepository is the pgx+sqlc implementation of authDomain.RoleRepository.
// The GORM-era repository eagerly Preloaded Permissions on every read;
// no service read that field, so permissions are now fetched explicitly
// through the permission repository when a caller actually needs them.
type roleRepository struct {
	tm *db.TxManager
}

// NewRoleRepository returns the pgx-backed repository.
func NewRoleRepository(tm *db.TxManager) authDomain.RoleRepository {
	return &roleRepository{tm: tm}
}

// ----- CRUD ----------------------------------------------------------

func (r *roleRepository) Create(ctx context.Context, role *authDomain.Role) error {
	if err := r.tm.Queries(ctx).CreateRole(ctx, gen.CreateRoleParams{
		ID:          role.ID,
		Name:        role.Name,
		ScopeType:   role.ScopeType,
		ScopeID:     role.ScopeID,
		Description: emptyToNilStringAuth(role.Description),
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create role %s: %w", role.Name, err)
	}
	return nil
}

func (r *roleRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.Role, error) {
	row, err := r.tm.Queries(ctx).GetRoleByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get role by ID %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by ID %s: %w", id, err)
	}
	return roleFromRow(&row), nil
}

func (r *roleRepository) GetByNameAndScope(ctx context.Context, name, scopeType string) (*authDomain.Role, error) {
	row, err := r.tm.Queries(ctx).GetRoleByNameAndScopeType(ctx, gen.GetRoleByNameAndScopeTypeParams{
		Name:      name,
		ScopeType: scopeType,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get role by name %s / scope %s: %w", name, scopeType, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get role by name %s / scope %s: %w", name, scopeType, err)
	}
	return roleFromRow(&row), nil
}

func (r *roleRepository) GetByNameScopeAndID(ctx context.Context, name, scopeType string, scopeID *uuid.UUID) (*authDomain.Role, error) {
	if scopeID == nil {
		return r.GetByNameAndScope(ctx, name, scopeType)
	}
	row, err := r.tm.Queries(ctx).GetRoleByNameScopeAndID(ctx, gen.GetRoleByNameScopeAndIDParams{
		Name:      name,
		ScopeType: scopeType,
		ScopeID:   scopeID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get role %s/%s/%s: %w", name, scopeType, *scopeID, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get role %s/%s/%s: %w", name, scopeType, *scopeID, err)
	}
	return roleFromRow(&row), nil
}

func (r *roleRepository) Update(ctx context.Context, role *authDomain.Role) error {
	if err := r.tm.Queries(ctx).UpdateRole(ctx, gen.UpdateRoleParams{
		ID:          role.ID,
		Name:        role.Name,
		ScopeType:   role.ScopeType,
		ScopeID:     role.ScopeID,
		Description: emptyToNilStringAuth(role.Description),
	}); err != nil {
		return fmt.Errorf("update role %s: %w", role.ID, err)
	}
	return nil
}

func (r *roleRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeleteRole(ctx, id); err != nil {
		return fmt.Errorf("delete role %s: %w", id, err)
	}
	return nil
}

// ----- Listings -------------------------------------------------------

func (r *roleRepository) GetByScopeType(ctx context.Context, scopeType string) ([]*authDomain.Role, error) {
	rows, err := r.tm.Queries(ctx).ListRolesByScopeType(ctx, scopeType)
	if err != nil {
		return nil, fmt.Errorf("list roles by scope %s: %w", scopeType, err)
	}
	return rolesFromRows(rows), nil
}

func (r *roleRepository) GetAllRoles(ctx context.Context) ([]*authDomain.Role, error) {
	rows, err := r.tm.Queries(ctx).ListAllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all roles: %w", err)
	}
	return rolesFromRows(rows), nil
}

func (r *roleRepository) GetSystemRoles(ctx context.Context) ([]*authDomain.Role, error) {
	rows, err := r.tm.Queries(ctx).ListSystemRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list system roles: %w", err)
	}
	return rolesFromRows(rows), nil
}

func (r *roleRepository) GetCustomRolesByScopeID(ctx context.Context, scopeType string, scopeID uuid.UUID) ([]*authDomain.Role, error) {
	rows, err := r.tm.Queries(ctx).ListCustomRolesByScopeID(ctx, gen.ListCustomRolesByScopeIDParams{
		ScopeType: scopeType,
		ScopeID:   &scopeID,
	})
	if err != nil {
		return nil, fmt.Errorf("list custom roles for %s/%s: %w", scopeType, scopeID, err)
	}
	return rolesFromRows(rows), nil
}

func (r *roleRepository) GetCustomRolesByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*authDomain.Role, error) {
	rows, err := r.tm.Queries(ctx).ListCustomRolesByOrganization(ctx, &organizationID)
	if err != nil {
		return nil, fmt.Errorf("list custom roles for org %s: %w", organizationID, err)
	}
	return rolesFromRows(rows), nil
}

// ----- Role ↔ permission management (thin facade over rolePermission repo)

// GetRolePermissions delegates to the permission repository. Kept on
// this interface for API compatibility.
func (r *roleRepository) GetRolePermissions(ctx context.Context, roleID uuid.UUID) ([]*authDomain.Permission, error) {
	rows, err := r.tm.Queries(ctx).ListPermissionsForRole(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions for role %s: %w", roleID, err)
	}
	out := make([]*authDomain.Permission, 0, len(rows))
	for i := range rows {
		out = append(out, permissionFromRow(&rows[i]))
	}
	return out, nil
}

func (r *roleRepository) AssignRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, pid := range permissionIDs {
			if err := q.CreateRolePermission(ctx, gen.CreateRolePermissionParams{
				RoleID:       roleID,
				PermissionID: pid,
				GrantedBy:    grantedBy,
			}); err != nil {
				return fmt.Errorf("assign permission %s to role %s: %w", pid, roleID, err)
			}
		}
		return nil
	})
}

func (r *roleRepository) RevokeRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}
	if _, err := r.tm.Queries(ctx).DeleteRolePermissionsForRoleIn(ctx, gen.DeleteRolePermissionsForRoleInParams{
		RoleID:        roleID,
		PermissionIds: permissionIDs,
	}); err != nil {
		return fmt.Errorf("revoke permissions from role %s: %w", roleID, err)
	}
	return nil
}

// UpdateRolePermissions replaces the entire permission set atomically.
// Same flatten-outer-wins reentrancy that ReplaceAllPermissions uses
// in role_permission_repository.
func (r *roleRepository) UpdateRolePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		if _, err := q.DeleteRolePermissionsByRoleID(ctx, roleID); err != nil {
			return fmt.Errorf("clear permissions for role %s: %w", roleID, err)
		}
		for _, pid := range permissionIDs {
			if err := q.CreateRolePermission(ctx, gen.CreateRolePermissionParams{
				RoleID:       roleID,
				PermissionID: pid,
				GrantedBy:    grantedBy,
			}); err != nil {
				return fmt.Errorf("assign permission %s to role %s: %w", pid, roleID, err)
			}
		}
		return nil
	})
}

// ----- Statistics ----------------------------------------------------

func (r *roleRepository) GetRoleStatistics(ctx context.Context) (*authDomain.RoleStatistics, error) {
	q := r.tm.Queries(ctx)
	stats := &authDomain.RoleStatistics{
		ScopeDistribution: make(map[string]int),
		RoleDistribution:  make(map[string]int),
	}

	total, err := q.CountRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("count roles: %w", err)
	}
	stats.TotalRoles = int(total)

	scopeRows, err := q.CountRolesByScopeType(ctx)
	if err != nil {
		return nil, fmt.Errorf("count roles by scope: %w", err)
	}
	for _, row := range scopeRows {
		stats.ScopeDistribution[row.ScopeType] = int(row.Count)
		switch row.ScopeType {
		case authDomain.ScopeOrganization:
			stats.OrganizationRoles = int(row.Count)
		case authDomain.ScopeProject:
			stats.ProjectRoles = int(row.Count)
		}
	}

	roleRows, err := q.CountMembersByRoleName(ctx)
	if err != nil {
		return nil, fmt.Errorf("count members by role: %w", err)
	}
	for _, row := range roleRows {
		stats.RoleDistribution[row.RoleName] = int(row.Count)
	}

	return stats, nil
}

// ----- Bulk ----------------------------------------------------------

func (r *roleRepository) BulkCreate(ctx context.Context, roles []*authDomain.Role) error {
	if len(roles) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, role := range roles {
			if err := q.CreateRole(ctx, gen.CreateRoleParams{
				ID:          role.ID,
				Name:        role.Name,
				ScopeType:   role.ScopeType,
				ScopeID:     role.ScopeID,
				Description: emptyToNilStringAuth(role.Description),
				CreatedAt:   role.CreatedAt,
				UpdatedAt:   role.UpdatedAt,
			}); err != nil {
				return fmt.Errorf("bulk-create role %s: %w", role.Name, err)
			}
		}
		return nil
	})
}

// ----- gen ↔ domain boundary ----------------------------------------

func roleFromRow(row *gen.Role) *authDomain.Role {
	return &authDomain.Role{
		ID:          row.ID,
		Name:        row.Name,
		ScopeType:   row.ScopeType,
		ScopeID:     row.ScopeID,
		Description: derefString(row.Description),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

func rolesFromRows(rows []gen.Role) []*authDomain.Role {
	out := make([]*authDomain.Role, 0, len(rows))
	for i := range rows {
		out = append(out, roleFromRow(&rows[i]))
	}
	return out
}

// emptyToNilStringAuth mirrors organization/helpers.go's emptyToNilString
// but lives in this package because we still have GORM-era repos in
// repository/auth/ and they don't share a helpers file yet.
func emptyToNilStringAuth(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
