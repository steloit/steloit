package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// organizationMemberRepository is the pgx+sqlc implementation of
// authDomain.OrganizationMemberRepository. The GORM-era repository
// eagerly Preloaded Role on every read; handlers fetch roles via
// roleService instead, so the preload is dropped.
type organizationMemberRepository struct {
	tm *db.TxManager
}

// NewOrganizationMemberRepository returns the pgx-backed repository.
func NewOrganizationMemberRepository(tm *db.TxManager) authDomain.OrganizationMemberRepository {
	return &organizationMemberRepository{tm: tm}
}

// ----- CRUD ----------------------------------------------------------

func (r *organizationMemberRepository) Create(ctx context.Context, m *authDomain.OrganizationMember) error {
	now := time.Now()
	if m.JoinedAt.IsZero() {
		m.JoinedAt = now
	}
	if m.Status == "" {
		m.Status = authDomain.MemberStatusActive
	}
	if err := r.tm.Queries(ctx).CreateMember(ctx, gen.CreateMemberParams{
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		RoleID:         m.RoleID,
		Status:         m.Status,
		JoinedAt:       m.JoinedAt,
		InvitedBy:      m.InvitedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return fmt.Errorf("create organization member (user=%s org=%s): %w", m.UserID, m.OrganizationID, err)
	}
	return nil
}

func (r *organizationMemberRepository) GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*authDomain.OrganizationMember, error) {
	row, err := r.tm.Queries(ctx).GetMemberByUserAndOrg(ctx, gen.GetMemberByUserAndOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get member (user=%s org=%s): %w", userID, orgID, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get member (user=%s org=%s): %w", userID, orgID, err)
	}
	return authMemberFromRow(&row), nil
}

func (r *organizationMemberRepository) Update(ctx context.Context, m *authDomain.OrganizationMember) error {
	if err := r.tm.Queries(ctx).UpdateMember(ctx, gen.UpdateMemberParams{
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		RoleID:         m.RoleID,
		Status:         m.Status,
		InvitedBy:      m.InvitedBy,
	}); err != nil {
		return fmt.Errorf("update organization member (user=%s org=%s): %w", m.UserID, m.OrganizationID, err)
	}
	return nil
}

func (r *organizationMemberRepository) Delete(ctx context.Context, userID, orgID uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteMemberByUserAndOrg(ctx, gen.SoftDeleteMemberByUserAndOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("delete organization member (user=%s org=%s): %w", userID, orgID, err)
	}
	return nil
}

// ----- Membership queries -------------------------------------------

func (r *organizationMemberRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	rows, err := r.tm.Queries(ctx).ListMembersByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships for user %s: %w", userID, err)
	}
	return authMembersFromRows(rows), nil
}

func (r *organizationMemberRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	rows, err := r.tm.Queries(ctx).ListMembersByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members for org %s: %w", orgID, err)
	}
	return authMembersFromRows(rows), nil
}

func (r *organizationMemberRepository) GetByRole(ctx context.Context, roleID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	rows, err := r.tm.Queries(ctx).ListMembersByRole(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("list members for role %s: %w", roleID, err)
	}
	return authMembersFromRows(rows), nil
}

func (r *organizationMemberRepository) Exists(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsMember(ctx, gen.IsMemberParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return false, fmt.Errorf("check member existence (user=%s org=%s): %w", userID, orgID, err)
	}
	return ok, nil
}

// ----- Permission queries -------------------------------------------

func (r *organizationMemberRepository) GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	perms, err := r.tm.Queries(ctx).ListUserEffectivePermissionsGlobal(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list effective permissions for user %s: %w", userID, err)
	}
	return perms, nil
}

func (r *organizationMemberRepository) HasUserPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	ok, err := r.tm.Queries(ctx).UserHasPermissionGlobal(ctx, gen.UserHasPermissionGlobalParams{
		UserID: userID,
		Column2: permission,
	})
	if err != nil {
		return false, fmt.Errorf("check permission %s for user %s: %w", permission, userID, err)
	}
	return ok, nil
}

func (r *organizationMemberRepository) CheckUserPermissions(ctx context.Context, userID uuid.UUID, permissions []string) (map[string]bool, error) {
	if len(permissions) == 0 {
		return map[string]bool{}, nil
	}
	granted, err := r.tm.Queries(ctx).ListUserEffectivePermissionsGlobal(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list effective permissions for user %s: %w", userID, err)
	}
	grantedSet := make(map[string]struct{}, len(granted))
	for _, p := range granted {
		grantedSet[p] = struct{}{}
	}
	result := make(map[string]bool, len(permissions))
	for _, p := range permissions {
		_, ok := grantedSet[p]
		result[p] = ok
	}
	return result, nil
}

func (r *organizationMemberRepository) GetUserPermissionsInOrganization(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	perms, err := r.tm.Queries(ctx).ListUserEffectivePermissionsInOrg(ctx, gen.ListUserEffectivePermissionsInOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("list permissions in org (user=%s org=%s): %w", userID, orgID, err)
	}
	return perms, nil
}

// ----- Status management --------------------------------------------

func (r *organizationMemberRepository) ActivateMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return r.setMemberStatus(ctx, userID, orgID, authDomain.MemberStatusActive)
}

func (r *organizationMemberRepository) SuspendMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return r.setMemberStatus(ctx, userID, orgID, authDomain.MemberStatusSuspended)
}

func (r *organizationMemberRepository) setMemberStatus(ctx context.Context, userID, orgID uuid.UUID, status string) error {
	if err := r.tm.Queries(ctx).UpdateMemberStatus(ctx, gen.UpdateMemberStatusParams{
		UserID:         userID,
		OrganizationID: orgID,
		Status:         status,
	}); err != nil {
		return fmt.Errorf("set member status %s (user=%s org=%s): %w", status, userID, orgID, err)
	}
	return nil
}

func (r *organizationMemberRepository) GetActiveMembers(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	rows, err := r.tm.Queries(ctx).ListActiveMembersByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list active members for org %s: %w", orgID, err)
	}
	return authMembersFromRows(rows), nil
}

// ----- Role management ----------------------------------------------

func (r *organizationMemberRepository) UpdateMemberRole(ctx context.Context, userID, orgID, roleID uuid.UUID) error {
	if err := r.tm.Queries(ctx).UpdateMemberRole(ctx, gen.UpdateMemberRoleParams{
		UserID:         userID,
		OrganizationID: orgID,
		RoleID:         roleID,
	}); err != nil {
		return fmt.Errorf("update member role (user=%s org=%s): %w", userID, orgID, err)
	}
	return nil
}

// ----- Bulk operations ----------------------------------------------

func (r *organizationMemberRepository) BulkCreate(ctx context.Context, members []*authDomain.OrganizationMember) error {
	if len(members) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		q := r.tm.Queries(ctx)
		now := time.Now()
		for _, m := range members {
			if m.JoinedAt.IsZero() {
				m.JoinedAt = now
			}
			if m.Status == "" {
				m.Status = authDomain.MemberStatusActive
			}
			if err := q.CreateMember(ctx, gen.CreateMemberParams{
				UserID:         m.UserID,
				OrganizationID: m.OrganizationID,
				RoleID:         m.RoleID,
				Status:         m.Status,
				JoinedAt:       m.JoinedAt,
				InvitedBy:      m.InvitedBy,
				CreatedAt:      now,
				UpdatedAt:      now,
			}); err != nil {
				return fmt.Errorf("bulk-create member (user=%s org=%s): %w", m.UserID, m.OrganizationID, err)
			}
		}
		return nil
	})
}

func (r *organizationMemberRepository) BulkUpdateRoles(ctx context.Context, updates []authDomain.MemberRoleUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	userIDs := make([]uuid.UUID, len(updates))
	orgIDs := make([]uuid.UUID, len(updates))
	roleIDs := make([]uuid.UUID, len(updates))
	for i, u := range updates {
		userIDs[i] = u.UserID
		orgIDs[i] = u.OrganizationID
		roleIDs[i] = u.RoleID
	}
	if err := r.tm.Queries(ctx).BulkUpdateMemberRoles(ctx, gen.BulkUpdateMemberRolesParams{
		Column1: userIDs,
		Column2: orgIDs,
		Column3: roleIDs,
	}); err != nil {
		return fmt.Errorf("bulk-update member roles (%d updates): %w", len(updates), err)
	}
	return nil
}

// ----- Statistics ---------------------------------------------------

func (r *organizationMemberRepository) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountActiveMembersByOrganization(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("count active members for org %s: %w", orgID, err)
	}
	return int(n), nil
}

func (r *organizationMemberRepository) GetMembersByRole(ctx context.Context, orgID uuid.UUID) (map[string]int, error) {
	rows, err := r.tm.Queries(ctx).CountActiveMembersByRoleName(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("count members by role for org %s: %w", orgID, err)
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.RoleName] = int(row.Count)
	}
	return out, nil
}

// ----- gen ↔ domain boundary ----------------------------------------

func authMemberFromRow(row *gen.OrganizationMember) *authDomain.OrganizationMember {
	return &authDomain.OrganizationMember{
		UserID:         row.UserID,
		OrganizationID: row.OrganizationID,
		RoleID:         row.RoleID,
		Status:         row.Status,
		JoinedAt:       row.JoinedAt,
		InvitedBy:      row.InvitedBy,
	}
}

func authMembersFromRows(rows []gen.OrganizationMember) []*authDomain.OrganizationMember {
	out := make([]*authDomain.OrganizationMember, 0, len(rows))
	for i := range rows {
		out = append(out, authMemberFromRow(&rows[i]))
	}
	return out
}
