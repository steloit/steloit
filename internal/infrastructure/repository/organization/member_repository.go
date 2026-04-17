package organization

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// memberRepository is the pgx+sqlc implementation of orgDomain.MemberRepository.
// organization_members has a composite PK (user_id, organization_id); the
// single-ID Delete / GetByID methods on the interface are intentionally
// rejected because they cannot address a row unambiguously.
type memberRepository struct {
	tm *db.TxManager
}

// NewMemberRepository returns the pgx-backed repository.
func NewMemberRepository(tm *db.TxManager) orgDomain.MemberRepository {
	return &memberRepository{tm: tm}
}

func (r *memberRepository) Create(ctx context.Context, m *orgDomain.Member) error {
	now := time.Now()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = now
	}
	if m.JoinedAt.IsZero() {
		m.JoinedAt = now
	}
	if m.Status == "" {
		m.Status = "active"
	}
	if err := r.tm.Queries(ctx).CreateMember(ctx, gen.CreateMemberParams{
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		RoleID:         m.RoleID,
		Status:         m.Status,
		JoinedAt:       m.JoinedAt,
		InvitedBy:      m.InvitedBy,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create member (user=%s org=%s): %w", m.UserID, m.OrganizationID, err)
	}
	return nil
}

// GetByID is not supported — members are addressed by composite key.
func (r *memberRepository) GetByID(ctx context.Context, _ uuid.UUID) (*orgDomain.Member, error) {
	return nil, fmt.Errorf("get member by single ID not supported (composite key): %w", orgDomain.ErrMemberNotFound)
}

func (r *memberRepository) GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*orgDomain.Member, error) {
	row, err := r.tm.Queries(ctx).GetMemberByUserAndOrg(ctx, gen.GetMemberByUserAndOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get member (user=%s org=%s): %w", userID, orgID, orgDomain.ErrMemberNotFound)
		}
		return nil, fmt.Errorf("get member (user=%s org=%s): %w", userID, orgID, err)
	}
	return memberFromRow(&row), nil
}

func (r *memberRepository) GetByUserAndOrg(ctx context.Context, userID, orgID uuid.UUID) (*orgDomain.Member, error) {
	return r.GetByUserAndOrganization(ctx, userID, orgID)
}

func (r *memberRepository) Update(ctx context.Context, m *orgDomain.Member) error {
	if err := r.tm.Queries(ctx).UpdateMember(ctx, gen.UpdateMemberParams{
		UserID:         m.UserID,
		OrganizationID: m.OrganizationID,
		RoleID:         m.RoleID,
		Status:         m.Status,
		InvitedBy:      m.InvitedBy,
	}); err != nil {
		return fmt.Errorf("update member (user=%s org=%s): %w", m.UserID, m.OrganizationID, err)
	}
	return nil
}

// Delete is not supported — members are addressed by composite key.
// Use DeleteByUserAndOrg instead.
func (r *memberRepository) Delete(ctx context.Context, _ uuid.UUID) error {
	return fmt.Errorf("delete member by single ID not supported (composite key): %w", orgDomain.ErrInsufficientRole)
}

func (r *memberRepository) DeleteByUserAndOrg(ctx context.Context, orgID, userID uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteMemberByUserAndOrg(ctx, gen.SoftDeleteMemberByUserAndOrgParams{
		UserID:         userID,
		OrganizationID: orgID,
	}); err != nil {
		return fmt.Errorf("soft-delete member (user=%s org=%s): %w", userID, orgID, err)
	}
	return nil
}

func (r *memberRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Member, error) {
	rows, err := r.tm.Queries(ctx).ListMembersByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list members for org %s: %w", orgID, err)
	}
	return membersFromRows(rows), nil
}

func (r *memberRepository) GetMembersByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Member, error) {
	return r.GetByOrganizationID(ctx, orgID)
}

func (r *memberRepository) GetMembersByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Member, error) {
	rows, err := r.tm.Queries(ctx).ListMembersByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list memberships for user %s: %w", userID, err)
	}
	return membersFromRows(rows), nil
}

func (r *memberRepository) UpdateMemberRole(ctx context.Context, orgID, userID, roleID uuid.UUID) error {
	if err := r.tm.Queries(ctx).UpdateMemberRole(ctx, gen.UpdateMemberRoleParams{
		UserID:         userID,
		OrganizationID: orgID,
		RoleID:         roleID,
	}); err != nil {
		return fmt.Errorf("update member role (user=%s org=%s): %w", userID, orgID, err)
	}
	return nil
}

func (r *memberRepository) GetMemberRole(ctx context.Context, userID, orgID uuid.UUID) (uuid.UUID, error) {
	id, err := r.tm.Queries(ctx).GetMemberRoleID(ctx, gen.GetMemberRoleIDParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return uuid.Nil, fmt.Errorf("get member role (user=%s org=%s): %w", userID, orgID, orgDomain.ErrMemberNotFound)
		}
		return uuid.Nil, fmt.Errorf("get member role (user=%s org=%s): %w", userID, orgID, err)
	}
	return id, nil
}

func (r *memberRepository) CountByOrganizationAndRole(ctx context.Context, orgID, roleID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountMembersByOrganizationAndRole(ctx, gen.CountMembersByOrganizationAndRoleParams{
		OrganizationID: orgID,
		RoleID:         roleID,
	})
	if err != nil {
		return 0, fmt.Errorf("count members (org=%s role=%s): %w", orgID, roleID, err)
	}
	return int(n), nil
}

func (r *memberRepository) IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsMember(ctx, gen.IsMemberParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return false, fmt.Errorf("is-member check (user=%s org=%s): %w", userID, orgID, err)
	}
	return ok, nil
}

func (r *memberRepository) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountMembersByOrganization(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("count members for org %s: %w", orgID, err)
	}
	return int(n), nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func memberFromRow(row *gen.OrganizationMember) *orgDomain.Member {
	return &orgDomain.Member{
		UserID:         row.UserID,
		OrganizationID: row.OrganizationID,
		RoleID:         row.RoleID,
		Status:         row.Status,
		JoinedAt:       row.JoinedAt,
		InvitedBy:      row.InvitedBy,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		DeletedAt:      row.DeletedAt,
	}
}

func membersFromRows(rows []gen.OrganizationMember) []*orgDomain.Member {
	out := make([]*orgDomain.Member, 0, len(rows))
	for i := range rows {
		out = append(out, memberFromRow(&rows[i]))
	}
	return out
}
