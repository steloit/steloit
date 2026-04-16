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

// invitationRepository implements orgDomain.InvitationRepository using GORM
type invitationRepository struct {
	db *gorm.DB
}

// NewInvitationRepository creates a new invitation repository instance
func NewInvitationRepository(db *gorm.DB) orgDomain.InvitationRepository {
	return &invitationRepository{
		db: db,
	}
}

// getDB returns transaction-aware DB instance
func (r *invitationRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

// Create creates a new invitation
func (r *invitationRepository) Create(ctx context.Context, invitation *orgDomain.Invitation) error {
	return r.getDB(ctx).WithContext(ctx).Create(invitation).Error
}

// GetByID retrieves an invitation by ID
func (r *invitationRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Invitation, error) {
	var invitation orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).Where("id = ?", id).First(&invitation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get invitation by ID %s: %w", id, orgDomain.ErrInvitationNotFound)
		}
		return nil, err
	}
	return &invitation, nil
}

// Update updates an invitation
func (r *invitationRepository) Update(ctx context.Context, invitation *orgDomain.Invitation) error {
	return r.getDB(ctx).WithContext(ctx).Save(invitation).Error
}

// GetByOrganizationID retrieves all invitations for an organization
func (r *invitationRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// GetByOrganizationAndStatus retrieves invitations by organization and status
func (r *invitationRepository) GetByOrganizationAndStatus(ctx context.Context, orgID uuid.UUID, status orgDomain.InvitationStatus) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND status = ?", orgID, status).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// GetByUserID retrieves all invitations for a user
func (r *invitationRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// GetByUserAndStatus retrieves invitations by user and status
func (r *invitationRepository) GetByUserAndStatus(ctx context.Context, userID uuid.UUID, status orgDomain.InvitationStatus) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("user_id = ? AND status = ?", userID, status).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// GetPendingByEmail retrieves pending invitations by email
func (r *invitationRepository) GetPendingByEmail(ctx context.Context, orgID uuid.UUID, email string) (*orgDomain.Invitation, error) {
	var invitation orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND email = ? AND status = ?", orgID, email, orgDomain.InvitationStatusPending).
		First(&invitation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get pending invitation by org %s and email %s: %w", orgID, email, orgDomain.ErrInvitationNotFound)
		}
		return nil, err
	}
	return &invitation, nil
}

// GetExpiredInvitations retrieves expired invitations
func (r *invitationRepository) GetExpiredInvitations(ctx context.Context) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("status = ? AND expires_at < ?", orgDomain.InvitationStatusPending, time.Now()).
		Find(&invitations).Error
	return invitations, err
}

// Delete soft deletes an invitation
func (r *invitationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).Model(&orgDomain.Invitation{}).Where("id = ?", id).Update("deleted_at", time.Now()).Error
}

// GetByToken retrieves an invitation by token
func (r *invitationRepository) GetByToken(ctx context.Context, token string) (*orgDomain.Invitation, error) {
	var invitation orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).Where("token = ? AND deleted_at IS NULL", token).First(&invitation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get invitation by token: %w", orgDomain.ErrInvitationNotFound)
		}
		return nil, err
	}
	return &invitation, nil
}

// GetByEmail retrieves all invitations for an email address
func (r *invitationRepository) GetByEmail(ctx context.Context, email string) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("email = ? AND deleted_at IS NULL", email).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// GetPendingInvitations retrieves pending invitations for an organization
func (r *invitationRepository) GetPendingInvitations(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Invitation, error) {
	var invitations []*orgDomain.Invitation
	err := r.getDB(ctx).WithContext(ctx).
		Where("organization_id = ? AND status = ? AND deleted_at IS NULL", orgID, orgDomain.InvitationStatusPending).
		Order("created_at DESC").
		Find(&invitations).Error
	return invitations, err
}

// MarkAccepted marks an invitation as accepted
func (r *invitationRepository) MarkAccepted(ctx context.Context, id uuid.UUID, acceptedByID uuid.UUID) error {
	now := time.Now()
	return r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":         orgDomain.InvitationStatusAccepted,
			"accepted_at":    now,
			"accepted_by_id": acceptedByID,
			"updated_at":     now,
		}).Error
}

// CleanupExpiredInvitations removes expired invitations
func (r *invitationRepository) CleanupExpiredInvitations(ctx context.Context) error {
	return r.getDB(ctx).WithContext(ctx).
		Where("status = ? AND expires_at < ?", orgDomain.InvitationStatusPending, time.Now()).
		Delete(&orgDomain.Invitation{}).Error
}

// IsEmailAlreadyInvited checks if an email already has a pending invitation for an organization
func (r *invitationRepository) IsEmailAlreadyInvited(ctx context.Context, email string, orgID uuid.UUID) (bool, error) {
	var count int64
	err := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("organization_id = ? AND email = ? AND status = ? AND deleted_at IS NULL", orgID, email, orgDomain.InvitationStatusPending).
		Count(&count).Error
	return count > 0, err
}

// MarkExpired marks an invitation as expired
func (r *invitationRepository) MarkExpired(ctx context.Context, id uuid.UUID) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     orgDomain.InvitationStatusExpired,
			"updated_at": time.Now(),
		}).Error
}

// RevokeInvitation revokes an invitation
func (r *invitationRepository) RevokeInvitation(ctx context.Context, id uuid.UUID, revokedByID uuid.UUID) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":        orgDomain.InvitationStatusRevoked,
			"revoked_at":    now,
			"revoked_by_id": revokedByID,
			"updated_at":    now,
		}).Error
}

// GetByTokenHash retrieves an invitation by token hash (secure lookup)
func (r *invitationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*orgDomain.Invitation, error) {
	var invitation orgDomain.Invitation
	err := r.db.WithContext(ctx).
		Where("token_hash = ? AND deleted_at IS NULL", tokenHash).
		First(&invitation).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get invitation by token hash: %w", orgDomain.ErrInvitationNotFound)
		}
		return nil, err
	}
	return &invitation, nil
}

// CreateAuditEvent creates an audit event for an invitation
func (r *invitationRepository) CreateAuditEvent(ctx context.Context, event *orgDomain.InvitationAuditEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// GetAuditEventsByInvitationID retrieves all audit events for an invitation
func (r *invitationRepository) GetAuditEventsByInvitationID(ctx context.Context, invitationID uuid.UUID) ([]*orgDomain.InvitationAuditEvent, error) {
	var events []*orgDomain.InvitationAuditEvent
	err := r.db.WithContext(ctx).
		Where("invitation_id = ?", invitationID).
		Order("created_at ASC").
		Find(&events).Error
	return events, err
}

// MarkResent atomically increments resent_count if within limits.
// This prevents race conditions where concurrent requests could bypass resend limits.
// Returns ErrResendLimitReached or ErrResendCooldown if constraints are not met.
func (r *invitationRepository) MarkResent(
	ctx context.Context,
	id uuid.UUID,
	newExpiresAt time.Time,
	maxAttempts int,
	cooldown time.Duration,
) error {
	now := time.Now()
	cooldownThreshold := now.Add(-cooldown)

	// Atomic update with conditions - only succeeds if all constraints are met
	result := r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("id = ?", id).
		Where("resent_count < ?", maxAttempts).
		Where("(resent_at IS NULL OR resent_at < ?)", cooldownThreshold).
		Updates(map[string]interface{}{
			"resent_at":    now,
			"resent_count": gorm.Expr("resent_count + 1"),
			"expires_at":   newExpiresAt,
			"updated_at":   now,
		})

	if result.Error != nil {
		return result.Error
	}

	// If no rows affected, determine which constraint failed
	if result.RowsAffected == 0 {
		var inv orgDomain.Invitation
		if err := r.getDB(ctx).WithContext(ctx).
			Where("id = ?", id).First(&inv).Error; err != nil {
			return err
		}
		if inv.ResentCount >= maxAttempts {
			return ErrResendLimitReached
		}
		return ErrResendCooldown
	}

	return nil
}

// UpdateTokenHash updates only the token hash and preview fields.
// Use this instead of Update when you need to preserve other field changes
// made by atomic operations (like AtomicMarkResent).
func (r *invitationRepository) UpdateTokenHash(ctx context.Context, id uuid.UUID, tokenHash, tokenPreview string) error {
	return r.getDB(ctx).WithContext(ctx).
		Model(&orgDomain.Invitation{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"token_hash":    tokenHash,
			"token_preview": tokenPreview,
			"updated_at":    time.Now(),
		}).Error
}
