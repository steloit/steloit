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

// memberRepository implements orgDomain.MemberRepository using GORM
type memberRepository struct {
	db *gorm.DB
}

// NewMemberRepository creates a new member repository instance
func NewMemberRepository(db *gorm.DB) orgDomain.MemberRepository {
	return &memberRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *memberRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new member
func (r *memberRepository) Create(ctx context.Context, member *orgDomain.Member) error {
	return r.getDB(ctx).WithContext(ctx).Create(member).Error
}

// GetByID retrieves a member by ID
func (r *memberRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Member, error) {
	var member orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get member by ID %s: %w", id, orgDomain.ErrMemberNotFound)
		}
		return nil, err
	}
	return &member, nil
}

// GetByUserAndOrganization retrieves a member by user and organization
func (r *memberRepository) GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*orgDomain.Member, error) {
	var member orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND organization_id = ? AND deleted_at IS NULL", userID, orgID).
		First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get member by user %s and org %s: %w", userID, orgID, orgDomain.ErrMemberNotFound)
		}
		return nil, err
	}
	return &member, nil
}

// Update updates a member
func (r *memberRepository) Update(ctx context.Context, member *orgDomain.Member) error {
	return r.getDB(ctx).WithContext(ctx).Save(member).Error
}

// Delete soft deletes a member by composite key (userID as the parameter for compatibility)
func (r *memberRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	// Note: This method signature is problematic since Member has composite key
	// This is a temporary fix - ideally the interface should be updated
	return fmt.Errorf("delete member method requires organizationID: %w", orgDomain.ErrInsufficientRole)
}

// DeleteByUserAndOrg soft deletes a member by composite key
func (r *memberRepository) DeleteByUserAndOrg(ctx context.Context, orgID, userID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Model(&orgDomain.Member{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Update("deleted_at", time.Now()).Error
}

// GetByOrganizationID retrieves all members of an organization
func (r *memberRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Member, error) {
	var members []*orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND deleted_at IS NULL", orgID).
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

// GetByUserID retrieves all memberships for a user
func (r *memberRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Member, error) {
	var members []*orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

// IsMember checks if a user is a member of an organization
func (r *memberRepository) IsMember(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Member{}).
		Where("user_id = ? AND organization_id = ? AND deleted_at IS NULL", userID, orgID).
		Count(&count).Error
	return count > 0, err
}

// CountByOrganization counts members in an organization
func (r *memberRepository) CountByOrganization(ctx context.Context, orgID uuid.UUID) (int64, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Member{}).
		Where("organization_id = ? AND deleted_at IS NULL", orgID).
		Count(&count).Error
	return count, err
}

// GetByUserAndOrg is an alias for GetByUserAndOrganization for interface compliance
func (r *memberRepository) GetByUserAndOrg(ctx context.Context, userID, orgID uuid.UUID) (*orgDomain.Member, error) {
	return r.GetByUserAndOrganization(ctx, userID, orgID)
}

// CountByOrganizationAndRole counts members with a specific role in an organization
func (r *memberRepository) CountByOrganizationAndRole(ctx context.Context, orgID, roleID uuid.UUID) (int, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Member{}).
		Where("organization_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).
		Count(&count).Error
	return int(count), err
}

// GetByOrganizationAndRole retrieves members with a specific role in an organization
func (r *memberRepository) GetByOrganizationAndRole(ctx context.Context, orgID, roleID uuid.UUID) ([]*orgDomain.Member, error) {
	var members []*orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

// GetMembersByOrganizationID is an alias for GetByOrganizationID for interface compliance
func (r *memberRepository) GetMembersByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Member, error) {
	return r.GetByOrganizationID(ctx, orgID)
}

// GetMembersByUserID is an alias for GetByUserID for interface compliance
func (r *memberRepository) GetMembersByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Member, error) {
	return r.GetByUserID(ctx, userID)
}

// GetMemberCount is an alias for CountByOrganization for interface compliance
func (r *memberRepository) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	count, err := r.CountByOrganization(ctx, orgID)
	return int(count), err
}

// UpdateMemberRole updates the role of a member
func (r *memberRepository) UpdateMemberRole(ctx context.Context, orgID, userID, roleID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Member{}).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Update("role_id", roleID).Error
}

// GetMemberRole retrieves the role of a member
func (r *memberRepository) GetMemberRole(ctx context.Context, userID, orgID uuid.UUID) (uuid.UUID, error) {
	var member orgDomain.Member
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		First(&member).Error
	if err != nil {
		return uuid.UUID{}, err
	}
	return member.RoleID, nil
}
