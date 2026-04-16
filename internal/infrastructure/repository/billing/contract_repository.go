package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type contractRepository struct {
	db *gorm.DB
}

func NewContractRepository(db *gorm.DB) billing.ContractRepository {
	return &contractRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *contractRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *contractRepository) Create(ctx context.Context, contract *billing.Contract) error {
	return r.getDB(ctx).WithContext(ctx).Create(contract).Error
}

func (r *contractRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.Contract, error) {
	var contract billing.Contract
	err := r.getDB(ctx).WithContext(ctx).
		Preload("VolumeTiers").
		Where("id = ?", id).
		First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, billing.NewContractNotFoundError(id.String())
		}
		return nil, fmt.Errorf("get contract: %w", err)
	}
	return &contract, nil
}

func (r *contractRepository) GetActiveByOrgID(ctx context.Context, orgID uuid.UUID) (*billing.Contract, error) {
	var contract billing.Contract
	err := r.getDB(ctx).WithContext(ctx).
		Preload("VolumeTiers").
		Where("organization_id = ? AND status = ?", orgID, billing.ContractStatusActive).
		First(&contract).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No active contract is valid - not an error
		}
		return nil, fmt.Errorf("get active contract: %w", err)
	}
	return &contract, nil
}

func (r *contractRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*billing.Contract, error) {
	var contracts []*billing.Contract
	err := r.getDB(ctx).WithContext(ctx).
		Preload("VolumeTiers").
		Where("organization_id = ?", orgID).
		Order("created_at DESC").
		Find(&contracts).Error
	if err != nil {
		return nil, fmt.Errorf("get contracts for organization: %w", err)
	}
	return contracts, nil
}

func (r *contractRepository) Update(ctx context.Context, contract *billing.Contract) error {
	contract.UpdatedAt = time.Now()
	return r.getDB(ctx).WithContext(ctx).Save(contract).Error
}

func (r *contractRepository) Expire(ctx context.Context, contractID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Model(&billing.Contract{}).
		Where("id = ?", contractID).
		Updates(map[string]interface{}{
			"status":     billing.ContractStatusExpired,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("expire contract: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return billing.NewContractNotFoundError(contractID.String())
	}

	return nil
}

func (r *contractRepository) Cancel(ctx context.Context, contractID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Model(&billing.Contract{}).
		Where("id = ?", contractID).
		Updates(map[string]interface{}{
			"status":     billing.ContractStatusCancelled,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("cancel contract: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return billing.NewContractNotFoundError(contractID.String())
	}

	return nil
}

func (r *contractRepository) GetExpiring(ctx context.Context, days int) ([]*billing.Contract, error) {
	// Calculate target time in UTC
	now := time.Now().UTC()
	targetTime := now.AddDate(0, 0, days)

	// Timestamp-based expiration (no normalization)
	// Query: expires_at <= targetTime
	// Example: Worker runs Jan 9 00:00, contract expires Jan 8 10:15
	// Result: Found (10:15 yesterday <= now) ✓
	// Max delay: 24 hours (daily worker acceptable for enterprise contracts)
	var contracts []*billing.Contract
	err := r.getDB(ctx).WithContext(ctx).
		Preload("VolumeTiers").
		Where("status = ? AND end_date IS NOT NULL AND end_date <= ?",
			billing.ContractStatusActive, targetTime).
		Order("end_date ASC").
		Find(&contracts).Error

	if err != nil {
		return nil, fmt.Errorf("get expiring contracts: %w", err)
	}

	return contracts, nil
}
