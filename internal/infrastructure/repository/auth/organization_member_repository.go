package auth

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/shared"
)

// organizationMemberRepository implements authDomain.OrganizationMemberRepository using GORM
type organizationMemberRepository struct {
	db *gorm.DB
}

// NewOrganizationMemberRepository creates a new organization member repository instance
func NewOrganizationMemberRepository(db *gorm.DB) authDomain.OrganizationMemberRepository {
	return &organizationMemberRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *organizationMemberRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Core CRUD operations

func (r *organizationMemberRepository) Create(ctx context.Context, member *authDomain.OrganizationMember) error {
	return r.getDB(ctx).WithContext(ctx).Create(member).Error
}

func (r *organizationMemberRepository) GetByUserAndOrganization(ctx context.Context, userID, orgID uuid.UUID) (*authDomain.OrganizationMember, error) {
	var member authDomain.OrganizationMember
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Preload("Role").
		First(&member).Error

	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *organizationMemberRepository) Update(ctx context.Context, member *authDomain.OrganizationMember) error {
	return r.getDB(ctx).WithContext(ctx).Save(member).Error
}

func (r *organizationMemberRepository) Delete(ctx context.Context, userID, orgID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Delete(&authDomain.OrganizationMember{}).Error
}

// Membership queries

func (r *organizationMemberRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	var members []*authDomain.OrganizationMember
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ?", userID).
		Preload("Role").
		Find(&members).Error

	return members, err
}

func (r *organizationMemberRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	var members []*authDomain.OrganizationMember
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ?", orgID).
		Preload("Role").
		Find(&members).Error

	return members, err
}

func (r *organizationMemberRepository) GetByRole(ctx context.Context, roleID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	var members []*authDomain.OrganizationMember
	err := r.getDB(ctx).WithContext(ctx).
		Where("role_id = ?", roleID).
		Preload("Role").
		Find(&members).Error

	return members, err
}

func (r *organizationMemberRepository) Exists(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Count(&count).Error

	return count > 0, err
}

// Permission queries

func (r *organizationMemberRepository) GetUserEffectivePermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var permissions []string

	err := r.getDB(ctx).WithContext(ctx).Raw(`
		SELECT DISTINCT p.name 
		FROM organization_members om
		JOIN roles r ON om.role_id = r.id
		JOIN role_permissions rp ON r.id = rp.role_id  
		JOIN permissions p ON rp.permission_id = p.id
		WHERE om.user_id = ? AND om.status = 'active'
	`, userID).Scan(&permissions).Error

	return permissions, err
}

func (r *organizationMemberRepository) HasUserPermission(ctx context.Context, userID uuid.UUID, permission string) (bool, error) {
	var count int64

	err := r.getDB(ctx).WithContext(ctx).Raw(`
		SELECT COUNT(1) 
		FROM organization_members om
		JOIN roles r ON om.role_id = r.id
		JOIN role_permissions rp ON r.id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE om.user_id = ? AND om.status = 'active' AND p.name = ?
	`, userID, permission).Count(&count).Error

	return count > 0, err
}

func (r *organizationMemberRepository) CheckUserPermissions(ctx context.Context, userID uuid.UUID, permissions []string) (map[string]bool, error) {
	result := make(map[string]bool)

	for _, permission := range permissions {
		hasPermission, err := r.HasUserPermission(ctx, userID, permission)
		if err != nil {
			return nil, fmt.Errorf("failed to check permission %s: %w", permission, err)
		}
		result[permission] = hasPermission
	}

	return result, nil
}

func (r *organizationMemberRepository) GetUserPermissionsInOrganization(ctx context.Context, userID, orgID uuid.UUID) ([]string, error) {
	var permissions []string

	err := r.getDB(ctx).WithContext(ctx).Raw(`
		SELECT DISTINCT p.name 
		FROM organization_members om
		JOIN roles r ON om.role_id = r.id
		JOIN role_permissions rp ON r.id = rp.role_id  
		JOIN permissions p ON rp.permission_id = p.id
		WHERE om.user_id = ? AND om.organization_id = ? AND om.status = 'active'
	`, userID, orgID).Scan(&permissions).Error

	return permissions, err
}

// Status management

func (r *organizationMemberRepository) ActivateMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Update("status", authDomain.MemberStatusActive).Error
}

func (r *organizationMemberRepository) SuspendMember(ctx context.Context, userID, orgID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Update("status", authDomain.MemberStatusSuspended).Error
}

func (r *organizationMemberRepository) GetActiveMembers(ctx context.Context, orgID uuid.UUID) ([]*authDomain.OrganizationMember, error) {
	var members []*authDomain.OrganizationMember
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND status = ?", orgID, authDomain.MemberStatusActive).
		Preload("Role").
		Find(&members).Error

	return members, err
}

// Role management

func (r *organizationMemberRepository) UpdateMemberRole(ctx context.Context, userID, orgID, roleID uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Where("user_id = ? AND organization_id = ?", userID, orgID).
		Update("role_id", roleID).Error
}

// Bulk operations

func (r *organizationMemberRepository) BulkCreate(ctx context.Context, members []*authDomain.OrganizationMember) error {
	return r.getDB(ctx).WithContext(ctx).Create(&members).Error
}

func (r *organizationMemberRepository) BulkUpdateRoles(ctx context.Context, updates []authDomain.MemberRoleUpdate) error {
	return r.getDB(ctx).WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			if err := tx.Model(&authDomain.OrganizationMember{}).
				Where("user_id = ? AND organization_id = ?", update.UserID, update.OrganizationID).
				Update("role_id", update.RoleID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Statistics

func (r *organizationMemberRepository) GetMemberCount(ctx context.Context, orgID uuid.UUID) (int, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Where("organization_id = ? AND status = ?", orgID, authDomain.MemberStatusActive).
		Count(&count).Error

	return int(count), err
}

func (r *organizationMemberRepository) GetMembersByRole(ctx context.Context, orgID uuid.UUID) (map[string]int, error) {
	var results []struct {
		RoleName string
		Count    int64
	}

	err := r.getDB(ctx).WithContext(ctx).
		Model(&authDomain.OrganizationMember{}).
		Select("r.name as role_name, COUNT(*) as count").
		Joins("JOIN roles r ON organization_members.role_id = r.id").
		Where("organization_members.organization_id = ? AND organization_members.status = ?", orgID, authDomain.MemberStatusActive).
		Group("r.name").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	membersByRole := make(map[string]int)
	for _, result := range results {
		membersByRole[result.RoleName] = int(result.Count)
	}

	return membersByRole, nil
}
