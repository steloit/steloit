package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/analytics"

	"gorm.io/gorm"
)

type ProviderModelRepositoryImpl struct {
	db *gorm.DB
}

func NewProviderModelRepository(db *gorm.DB) analytics.ProviderModelRepository {
	return &ProviderModelRepositoryImpl{db: db}
}

func (r *ProviderModelRepositoryImpl) CreateProviderModel(ctx context.Context, model *analytics.ProviderModel) error {
	return r.db.WithContext(ctx).Create(model).Error
}

func (r *ProviderModelRepositoryImpl) GetProviderModel(ctx context.Context, modelID uuid.UUID) (*analytics.ProviderModel, error) {
	var model analytics.ProviderModel
	err := r.db.WithContext(ctx).Where("id = ?", modelID).First(&model).Error
	if err != nil {
		return nil, err
	}
	return &model, nil
}

func (r *ProviderModelRepositoryImpl) GetProviderModelByName(
	ctx context.Context,
	projectID *uuid.UUID,
	modelName string,
) (*analytics.ProviderModel, error) {
	return r.GetProviderModelAtTime(ctx, projectID, modelName, time.Now())
}

// Project-specific pricing takes precedence over global pricing.
func (r *ProviderModelRepositoryImpl) GetProviderModelAtTime(
	ctx context.Context,
	projectID *uuid.UUID,
	modelName string,
	atTime time.Time,
) (*analytics.ProviderModel, error) {
	var model analytics.ProviderModel

	query := r.db.WithContext(ctx).
		Where("(model_name = ? OR ? ~ match_pattern)", modelName, modelName).
		Where("start_date <= ?", atTime)

	// Project-specific pricing takes precedence
	if projectID != nil {
		query = query.Where("(project_id = ? OR project_id IS NULL)", projectID)
		query = query.Order(fmt.Sprintf("CASE WHEN project_id = '%s' THEN 0 ELSE 1 END", projectID.String()))
	} else {
		query = query.Where("project_id IS NULL")
	}

	err := query.
		Order("start_date DESC").
		First(&model).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("model not found: %s", modelName)
		}
		return nil, err
	}

	return &model, nil
}

func (r *ProviderModelRepositoryImpl) ListProviderModels(ctx context.Context, projectID *uuid.UUID) ([]*analytics.ProviderModel, error) {
	var models []*analytics.ProviderModel

	query := r.db.WithContext(ctx)
	if projectID != nil {
		query = query.Where("project_id = ?", projectID)
	} else {
		query = query.Where("project_id IS NULL")
	}

	err := query.Order("model_name ASC, start_date DESC").Find(&models).Error
	return models, err
}

// Returns default models for configured providers (model catalog).
func (r *ProviderModelRepositoryImpl) ListByProviders(ctx context.Context, providers []string) ([]*analytics.ProviderModel, error) {
	if len(providers) == 0 {
		return []*analytics.ProviderModel{}, nil
	}

	var models []*analytics.ProviderModel
	err := r.db.WithContext(ctx).
		Where("provider IN ?", providers).
		Where("project_id IS NULL"). // Global models only
		Order("provider ASC, model_name ASC").
		Find(&models).Error
	return models, err
}

func (r *ProviderModelRepositoryImpl) UpdateProviderModel(ctx context.Context, modelID uuid.UUID, model *analytics.ProviderModel) error {
	return r.db.WithContext(ctx).Where("id = ?", modelID).Updates(model).Error
}

// Cascade deletes prices.
func (r *ProviderModelRepositoryImpl) DeleteProviderModel(ctx context.Context, modelID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", modelID).Delete(&analytics.ProviderModel{}).Error
}

func (r *ProviderModelRepositoryImpl) CreateProviderPrice(ctx context.Context, price *analytics.ProviderPrice) error {
	return r.db.WithContext(ctx).Create(price).Error
}

// Project-specific prices override global prices.
func (r *ProviderModelRepositoryImpl) GetProviderPrices(
	ctx context.Context,
	modelID uuid.UUID,
	projectID *uuid.UUID,
) ([]*analytics.ProviderPrice, error) {
	var prices []*analytics.ProviderPrice

	// Build query for prices (project-specific override takes precedence)
	query := r.db.WithContext(ctx).
		Where("provider_model_id = ?", modelID)

	if projectID != nil {
		// Get both project-specific and global prices
		query = query.Where("(project_id = ? OR project_id IS NULL)", projectID)

		// Execute query
		var allPrices []*analytics.ProviderPrice
		if err := query.Find(&allPrices).Error; err != nil {
			return nil, err
		}

		// Deduplicate: project-specific overrides global
		priceMap := make(map[string]*analytics.ProviderPrice)
		for _, price := range allPrices {
			key := price.UsageType

			// If no price for this usage type yet, add it
			if existing, exists := priceMap[key]; !exists {
				priceMap[key] = price
			} else {
				// Project-specific price overrides global
				if price.ProjectID != nil && existing.ProjectID == nil {
					priceMap[key] = price
				}
			}
		}

		// Convert map to slice
		for _, price := range priceMap {
			prices = append(prices, price)
		}

		return prices, nil
	}

	// Global pricing only
	err := query.Where("project_id IS NULL").Find(&prices).Error
	return prices, err
}

func (r *ProviderModelRepositoryImpl) UpdateProviderPrice(ctx context.Context, priceID uuid.UUID, price *analytics.ProviderPrice) error {
	return r.db.WithContext(ctx).Where("id = ?", priceID).Updates(price).Error
}

func (r *ProviderModelRepositoryImpl) DeleteProviderPrice(ctx context.Context, priceID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", priceID).Delete(&analytics.ProviderPrice{}).Error
}

func (r *ProviderModelRepositoryImpl) GetPriceForUsageType(
	ctx context.Context,
	modelID uuid.UUID,
	projectID *uuid.UUID,
	usageType string,
) (*analytics.ProviderPrice, error) {
	var price analytics.ProviderPrice

	query := r.db.WithContext(ctx).
		Where("provider_model_id = ?", modelID).
		Where("usage_type = ?", usageType)

	if projectID != nil {
		// Project-specific price takes precedence
		query = query.Where("(project_id = ? OR project_id IS NULL)", projectID)
		query = query.Order(fmt.Sprintf("CASE WHEN project_id = '%s' THEN 0 ELSE 1 END", projectID.String()))
	} else {
		query = query.Where("project_id IS NULL")
	}

	err := query.First(&price).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound || err == sql.ErrNoRows {
			return nil, fmt.Errorf("price not found for usage type: %s", usageType)
		}
		return nil, err
	}

	return &price, nil
}
