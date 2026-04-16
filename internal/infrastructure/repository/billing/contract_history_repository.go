package billing

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/shared"
)

type contractHistoryRepository struct {
	db *gorm.DB
}

func NewContractHistoryRepository(db *gorm.DB) billing.ContractHistoryRepository {
	return &contractHistoryRepository{db: db}
}

// getDB returns transaction-aware DB instance
func (r *contractHistoryRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *contractHistoryRepository) Log(ctx context.Context, history *billing.ContractHistory) error {
	return r.getDB(ctx).WithContext(ctx).Create(history).Error
}

func (r *contractHistoryRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billing.ContractHistory, error) {
	var history []*billing.ContractHistory
	err := r.getDB(ctx).WithContext(ctx).
		Where("contract_id = ?", contractID).
		Order("changed_at DESC").
		Find(&history).Error

	if err != nil {
		return nil, fmt.Errorf("get contract history: %w", err)
	}

	return history, nil
}
