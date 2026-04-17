package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// rolePermissionRepository is the pgx+sqlc implementation of
// authDomain.RolePermissionRepository. All writes route through the
// TxManager so bulk operations can be wrapped in WithinTransaction by
// either the repository (for operations that are atomic by definition,
// e.g. ReplaceAllPermissions) or the service layer (for cross-aggregate
// flows).
type rolePermissionRepository struct {
	tm *db.TxManager
}

// NewRolePermissionRepository returns the pgx-backed repository.
func NewRolePermissionRepository(tm *db.TxManager) authDomain.RolePermissionRepository {
	return &rolePermissionRepository{tm: tm}
}

func (r *rolePermissionRepository) Create(ctx context.Context, rp *authDomain.RolePermission) error {
	return r.createRolePermission(ctx, rp)
}

// createRolePermission is the shared insert path used by Create, bulk
// assignments, and replace-all flows. Pulling it out lets bulk callers
// reuse the same ctx-scoped Queries without replicating argument shaping.
func (r *rolePermissionRepository) createRolePermission(ctx context.Context, rp *authDomain.RolePermission) error {
	params := gen.CreateRolePermissionParams{
		RoleID:       rp.RoleID,
		PermissionID: rp.PermissionID,
		GrantedBy:    rp.GrantedBy,
	}
	if !rp.GrantedAt.IsZero() {
		granted := rp.GrantedAt
		params.GrantedAt = &granted
	}
	if err := r.tm.Queries(ctx).CreateRolePermission(ctx, params); err != nil {
		return fmt.Errorf("create role_permission (role=%s permission=%s): %w", rp.RoleID, rp.PermissionID, err)
	}
	return nil
}

func (r *rolePermissionRepository) GetByRoleID(ctx context.Context, roleID uuid.UUID) ([]*authDomain.RolePermission, error) {
	rows, err := r.tm.Queries(ctx).ListRolePermissionsByRoleID(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role_permissions for role %s: %w", roleID, err)
	}
	return rolePermissionsFromRows(rows), nil
}

func (r *rolePermissionRepository) GetByPermissionID(ctx context.Context, permissionID uuid.UUID) ([]*authDomain.RolePermission, error) {
	rows, err := r.tm.Queries(ctx).ListRolePermissionsByPermissionID(ctx, permissionID)
	if err != nil {
		return nil, fmt.Errorf("list role_permissions for permission %s: %w", permissionID, err)
	}
	return rolePermissionsFromRows(rows), nil
}

func (r *rolePermissionRepository) Delete(ctx context.Context, roleID, permissionID uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeleteRolePermission(ctx, gen.DeleteRolePermissionParams{
		RoleID:       roleID,
		PermissionID: permissionID,
	}); err != nil {
		return fmt.Errorf("delete role_permission (role=%s permission=%s): %w", roleID, permissionID, err)
	}
	return nil
}

func (r *rolePermissionRepository) DeleteByRoleID(ctx context.Context, roleID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).DeleteRolePermissionsByRoleID(ctx, roleID); err != nil {
		return fmt.Errorf("delete role_permissions for role %s: %w", roleID, err)
	}
	return nil
}

func (r *rolePermissionRepository) DeleteByPermissionID(ctx context.Context, permissionID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).DeleteRolePermissionsByPermissionID(ctx, permissionID); err != nil {
		return fmt.Errorf("delete role_permissions for permission %s: %w", permissionID, err)
	}
	return nil
}

func (r *rolePermissionRepository) Exists(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).RolePermissionExists(ctx, gen.RolePermissionExistsParams{
		RoleID:       roleID,
		PermissionID: permissionID,
	})
	if err != nil {
		return false, fmt.Errorf("check role_permission (role=%s permission=%s): %w", roleID, permissionID, err)
	}
	return ok, nil
}

// HasPermission is an alias for Exists retained by the domain interface.
func (r *rolePermissionRepository) HasPermission(ctx context.Context, roleID, permissionID uuid.UUID) (bool, error) {
	return r.Exists(ctx, roleID, permissionID)
}

// AssignPermissions replaces the role's entire permission set with the given
// permissions. Wraps in a single transaction so a partial failure reverts
// the delete. WithinTransaction is reentrant-safe — if a service already has
// a tx open, we reuse it (outer wins).
func (r *rolePermissionRepository) AssignPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		if err := r.DeleteByRoleID(ctx, roleID); err != nil {
			return err
		}
		return r.bulkInsert(ctx, roleID, permissionIDs, grantedBy)
	})
}

// RevokePermissions removes the specified permissions from the role in a
// single statement — no round-trip per ID.
func (r *rolePermissionRepository) RevokePermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID) error {
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

// RevokeAllPermissions drops every permission from the role.
func (r *rolePermissionRepository) RevokeAllPermissions(ctx context.Context, roleID uuid.UUID) error {
	return r.DeleteByRoleID(ctx, roleID)
}

// ReplaceAllPermissions is the canonical full-reset primitive. Matches the
// AssignPermissions behaviour; both exist in the domain interface for
// historical reasons.
func (r *rolePermissionRepository) ReplaceAllPermissions(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	return r.AssignPermissions(ctx, roleID, permissionIDs, grantedBy)
}

func (r *rolePermissionRepository) HasResourceAction(ctx context.Context, roleID uuid.UUID, resource, action string) (bool, error) {
	ok, err := r.tm.Queries(ctx).RoleHasResourceAction(ctx, gen.RoleHasResourceActionParams{
		RoleID:   roleID,
		Resource: resource,
		Action:   action,
	})
	if err != nil {
		return false, fmt.Errorf("check role %s for %s:%s: %w", roleID, resource, action, err)
	}
	return ok, nil
}

// CheckResourceActions returns a map { "resource:action" -> bool } answering
// whether the role holds each requested permission. One DB round trip rather
// than N.
func (r *rolePermissionRepository) CheckResourceActions(ctx context.Context, roleID uuid.UUID, resourceActions []string) (map[string]bool, error) {
	result := make(map[string]bool, len(resourceActions))
	for _, ra := range resourceActions {
		result[ra] = false
	}
	if len(resourceActions) == 0 {
		return result, nil
	}

	granted, err := r.tm.Queries(ctx).ListResourceActionsForRole(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list resource actions for role %s: %w", roleID, err)
	}
	want := make(map[string]struct{}, len(resourceActions))
	for _, ra := range resourceActions {
		want[ra] = struct{}{}
	}
	for _, ra := range granted {
		// Only mark true for actions the caller asked about; a role with
		// extra unrelated permissions does not change the returned map.
		if _, ok := want[ra]; ok {
			result[ra] = true
		}
	}
	return result, nil
}

// BulkAssign inserts the given assignments atomically. Unique-constraint
// violations surface through the underlying insert — callers are expected
// to deduplicate when atomicity across conflicts matters.
func (r *rolePermissionRepository) BulkAssign(ctx context.Context, assignments []authDomain.RolePermissionAssignment) error {
	if len(assignments) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, a := range assignments {
			if err := q.CreateRolePermission(ctx, gen.CreateRolePermissionParams{
				RoleID:       a.RoleID,
				PermissionID: a.PermissionID,
			}); err != nil {
				return fmt.Errorf("bulk assign role=%s permission=%s: %w", a.RoleID, a.PermissionID, err)
			}
		}
		return nil
	})
}

// BulkRevoke deletes the given (role, permission) pairs atomically.
func (r *rolePermissionRepository) BulkRevoke(ctx context.Context, revocations []authDomain.RolePermissionRevocation) error {
	if len(revocations) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		for _, rev := range revocations {
			if err := q.DeleteRolePermission(ctx, gen.DeleteRolePermissionParams{
				RoleID:       rev.RoleID,
				PermissionID: rev.PermissionID,
			}); err != nil {
				return fmt.Errorf("bulk revoke role=%s permission=%s: %w", rev.RoleID, rev.PermissionID, err)
			}
		}
		return nil
	})
}

func (r *rolePermissionRepository) GetRolePermissionCount(ctx context.Context, roleID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountRolePermissionsByRole(ctx, roleID)
	if err != nil {
		return 0, fmt.Errorf("count role_permissions for role %s: %w", roleID, err)
	}
	return int(n), nil
}

func (r *rolePermissionRepository) GetPermissionRoleCount(ctx context.Context, permissionID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountRolePermissionsByPermission(ctx, permissionID)
	if err != nil {
		return 0, fmt.Errorf("count role_permissions for permission %s: %w", permissionID, err)
	}
	return int(n), nil
}

// bulkInsert creates multiple role_permission rows using a single tx-scoped
// *Queries. Called from AssignPermissions and ReplaceAllPermissions — both
// hold an outer transaction, so `WithinTransaction` nesting is flat.
func (r *rolePermissionRepository) bulkInsert(ctx context.Context, roleID uuid.UUID, permissionIDs []uuid.UUID, grantedBy *uuid.UUID) error {
	if len(permissionIDs) == 0 {
		return nil
	}
	q := r.tm.Queries(ctx)
	for _, pid := range permissionIDs {
		if err := q.CreateRolePermission(ctx, gen.CreateRolePermissionParams{
			RoleID:       roleID,
			PermissionID: pid,
			GrantedBy:    grantedBy,
		}); err != nil {
			return fmt.Errorf("insert role_permission role=%s permission=%s: %w", roleID, pid, err)
		}
	}
	return nil
}

func rolePermissionsFromRows(rows []gen.RolePermission) []*authDomain.RolePermission {
	out := make([]*authDomain.RolePermission, 0, len(rows))
	for i := range rows {
		out = append(out, &authDomain.RolePermission{
			RoleID:       rows[i].RoleID,
			PermissionID: rows[i].PermissionID,
			GrantedAt:    rows[i].GrantedAt,
			GrantedBy:    rows[i].GrantedBy,
		})
	}
	return out
}

// Compile-time silencer for strings import (used only via qualified symbol
// if the service layer later rejigs resource:action parsing here).
var _ = strings.TrimSpace
