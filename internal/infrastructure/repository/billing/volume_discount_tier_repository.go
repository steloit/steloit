package billing

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type volumeDiscountTierRepository struct {
	db *gorm.DB
}

func NewVolumeDiscountTierRepository(db *gorm.DB) billing.VolumeDiscountTierRepository {
	return &volumeDiscountTierRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *volumeDiscountTierRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *volumeDiscountTierRepository) Create(ctx context.Context, tier *billing.VolumeDiscountTier) error {
	return r.getDB(ctx).WithContext(ctx).Create(tier).Error
}

func (r *volumeDiscountTierRepository) CreateBatch(ctx context.Context, tiers []*billing.VolumeDiscountTier) error {
	if len(tiers) == 0 {
		return nil
	}

	return r.getDB(ctx).WithContext(ctx).Create(&tiers).Error
}

func (r *volumeDiscountTierRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billing.VolumeDiscountTier, error) {
	var tiers []*billing.VolumeDiscountTier
	err := r.getDB(ctx).WithContext(ctx).
		Where("contract_id = ?", contractID).
		Order("dimension ASC, priority ASC, tier_min ASC").
		Find(&tiers).Error

	if err != nil {
		return nil, fmt.Errorf("get volume tiers for contract: %w", err)
	}

	return tiers, nil
}

func (r *volumeDiscountTierRepository) DeleteByContractID(ctx context.Context, contractID uuid.UUID) error {
	result := r.getDB(ctx).WithContext(ctx).
		Where("contract_id = ?", contractID).
		Delete(&billing.VolumeDiscountTier{})

	if result.Error != nil {
		return fmt.Errorf("delete volume tiers: %w", result.Error)
	}

	return nil
}
