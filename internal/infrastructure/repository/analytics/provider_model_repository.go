package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	analyticsDomain "brokle/internal/core/domain/analytics"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// ProviderModelRepositoryImpl is the pgx+sqlc implementation of
// analyticsDomain.ProviderModelRepository. Project-specific overrides
// take precedence over global rows — the precedence selection lives
// in squirrel because sqlc can't express the `CASE WHEN project_id = X
// THEN 0 ELSE 1 END` tiebreaker with a typed parameter.
type ProviderModelRepositoryImpl struct {
	tm *db.TxManager
}

func NewProviderModelRepository(tm *db.TxManager) analyticsDomain.ProviderModelRepository {
	return &ProviderModelRepositoryImpl{tm: tm}
}

// ----- provider_models ------------------------------------------------

func (r *ProviderModelRepositoryImpl) CreateProviderModel(ctx context.Context, m *analyticsDomain.ProviderModel) error {
	cfg, err := marshalTokenizerConfig(m.TokenizerConfig)
	if err != nil {
		return fmt.Errorf("create provider model: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateProviderModel(ctx, gen.CreateProviderModelParams{
		ID:              m.ID,
		ProjectID:       m.ProjectID,
		ModelName:       m.ModelName,
		MatchPattern:    m.MatchPattern,
		Provider:        m.Provider,
		DisplayName:     m.DisplayName,
		StartDate:       m.StartDate,
		Unit:            m.Unit,
		TokenizerID:     m.TokenizerID,
		TokenizerConfig: cfg,
	}); err != nil {
		return fmt.Errorf("create provider model: %w", err)
	}
	return nil
}

func (r *ProviderModelRepositoryImpl) GetProviderModel(ctx context.Context, modelID uuid.UUID) (*analyticsDomain.ProviderModel, error) {
	row, err := r.tm.Queries(ctx).GetProviderModelByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	return providerModelFromRow(&row)
}

func (r *ProviderModelRepositoryImpl) GetProviderModelByName(
	ctx context.Context,
	projectID *uuid.UUID,
	modelName string,
) (*analyticsDomain.ProviderModel, error) {
	return r.GetProviderModelAtTime(ctx, projectID, modelName, time.Now())
}

// GetProviderModelAtTime resolves the active pricing row at the given
// instant. Project-specific rows win over global; among eligible rows,
// the most recent start_date wins.
func (r *ProviderModelRepositoryImpl) GetProviderModelAtTime(
	ctx context.Context,
	projectID *uuid.UUID,
	modelName string,
	atTime time.Time,
) (*analyticsDomain.ProviderModel, error) {
	b := sq.Select(providerModelColumns...).From("provider_models").
		Where("(model_name = ? OR ? ~ match_pattern)", modelName, modelName).
		Where(sq.LtOrEq{"start_date": atTime})

	if projectID != nil {
		b = b.Where("(project_id = ? OR project_id IS NULL)", *projectID).
			OrderBy(fmt.Sprintf("CASE WHEN project_id = '%s' THEN 0 ELSE 1 END", projectID.String()))
	} else {
		b = b.Where("project_id IS NULL")
	}
	b = b.OrderBy("start_date DESC").Limit(1)

	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build provider model at-time query: %w", err)
	}
	row := r.tm.DB(ctx).QueryRow(ctx, sqlStr, args...)
	m, err := scanProviderModel(row)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("model not found: %s", modelName)
		}
		return nil, err
	}
	return m, nil
}

func (r *ProviderModelRepositoryImpl) ListProviderModels(ctx context.Context, projectID *uuid.UUID) ([]*analyticsDomain.ProviderModel, error) {
	if projectID == nil {
		rows, err := r.tm.Queries(ctx).ListProviderModelsGlobal(ctx)
		if err != nil {
			return nil, err
		}
		return providerModelsFromRows(rows)
	}
	rows, err := r.tm.Queries(ctx).ListProviderModelsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return providerModelsFromRows(rows)
}

func (r *ProviderModelRepositoryImpl) ListByProviders(ctx context.Context, providers []string) ([]*analyticsDomain.ProviderModel, error) {
	if len(providers) == 0 {
		return []*analyticsDomain.ProviderModel{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListProviderModelsByProviders(ctx, providers)
	if err != nil {
		return nil, err
	}
	return providerModelsFromRows(rows)
}

func (r *ProviderModelRepositoryImpl) UpdateProviderModel(ctx context.Context, modelID uuid.UUID, m *analyticsDomain.ProviderModel) error {
	cfg, err := marshalTokenizerConfig(m.TokenizerConfig)
	if err != nil {
		return fmt.Errorf("update provider model: %w", err)
	}
	if err := r.tm.Queries(ctx).UpdateProviderModel(ctx, gen.UpdateProviderModelParams{
		ID:              modelID,
		ProjectID:       m.ProjectID,
		ModelName:       m.ModelName,
		MatchPattern:    m.MatchPattern,
		Provider:        m.Provider,
		DisplayName:     m.DisplayName,
		StartDate:       m.StartDate,
		Unit:            m.Unit,
		TokenizerID:     m.TokenizerID,
		TokenizerConfig: cfg,
	}); err != nil {
		return fmt.Errorf("update provider model: %w", err)
	}
	return nil
}

func (r *ProviderModelRepositoryImpl) DeleteProviderModel(ctx context.Context, modelID uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeleteProviderModel(ctx, modelID); err != nil {
		return fmt.Errorf("delete provider model: %w", err)
	}
	return nil
}

// ----- provider_prices -----------------------------------------------

func (r *ProviderModelRepositoryImpl) CreateProviderPrice(ctx context.Context, p *analyticsDomain.ProviderPrice) error {
	if err := r.tm.Queries(ctx).CreateProviderPrice(ctx, gen.CreateProviderPriceParams{
		ID:              p.ID,
		ProviderModelID: p.ProviderModelID,
		ProjectID:       p.ProjectID,
		UsageType:       p.UsageType,
		Price:           p.Price,
	}); err != nil {
		return fmt.Errorf("create provider price: %w", err)
	}
	return nil
}

// GetProviderPrices returns project-specific + global prices for a
// model. Dedup logic mirrors the GORM original: project override wins
// per usage_type.
func (r *ProviderModelRepositoryImpl) GetProviderPrices(
	ctx context.Context,
	modelID uuid.UUID,
	projectID *uuid.UUID,
) ([]*analyticsDomain.ProviderPrice, error) {
	if projectID == nil {
		rows, err := r.tm.Queries(ctx).ListProviderPricesByModelGlobal(ctx, modelID)
		if err != nil {
			return nil, err
		}
		return providerPricesFromRows(rows), nil
	}
	rows, err := r.tm.Queries(ctx).ListProviderPricesByModelAndProject(ctx, gen.ListProviderPricesByModelAndProjectParams{
		ProviderModelID: modelID,
		ProjectID:       projectID,
	})
	if err != nil {
		return nil, err
	}
	// Dedup by usage_type, project-specific overrides global.
	byType := make(map[string]*analyticsDomain.ProviderPrice, len(rows))
	for i := range rows {
		p := providerPriceFromRow(&rows[i])
		existing, ok := byType[p.UsageType]
		if !ok {
			byType[p.UsageType] = p
			continue
		}
		if p.ProjectID != nil && existing.ProjectID == nil {
			byType[p.UsageType] = p
		}
	}
	out := make([]*analyticsDomain.ProviderPrice, 0, len(byType))
	for _, p := range byType {
		out = append(out, p)
	}
	return out, nil
}

func (r *ProviderModelRepositoryImpl) UpdateProviderPrice(ctx context.Context, priceID uuid.UUID, p *analyticsDomain.ProviderPrice) error {
	if err := r.tm.Queries(ctx).UpdateProviderPrice(ctx, gen.UpdateProviderPriceParams{
		ID:              priceID,
		ProviderModelID: p.ProviderModelID,
		ProjectID:       p.ProjectID,
		UsageType:       p.UsageType,
		Price:           p.Price,
	}); err != nil {
		return fmt.Errorf("update provider price: %w", err)
	}
	return nil
}

func (r *ProviderModelRepositoryImpl) DeleteProviderPrice(ctx context.Context, priceID uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeleteProviderPrice(ctx, priceID); err != nil {
		return fmt.Errorf("delete provider price: %w", err)
	}
	return nil
}

// GetPriceForUsageType resolves the single price row for (model, usage_type)
// with project-specific precedence. Squirrel is used for the CASE WHEN
// tiebreaker (same pattern as GetProviderModelAtTime).
func (r *ProviderModelRepositoryImpl) GetPriceForUsageType(
	ctx context.Context,
	modelID uuid.UUID,
	projectID *uuid.UUID,
	usageType string,
) (*analyticsDomain.ProviderPrice, error) {
	b := sq.Select("id", "created_at", "updated_at", "provider_model_id", "project_id", "usage_type", "price").
		From("provider_prices").
		Where(sq.Eq{"provider_model_id": modelID, "usage_type": usageType})

	if projectID != nil {
		b = b.Where("(project_id = ? OR project_id IS NULL)", *projectID).
			OrderBy(fmt.Sprintf("CASE WHEN project_id = '%s' THEN 0 ELSE 1 END", projectID.String()))
	} else {
		b = b.Where("project_id IS NULL")
	}
	b = b.Limit(1)

	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build price-for-usage-type query: %w", err)
	}
	p := &analyticsDomain.ProviderPrice{}
	if err := r.tm.DB(ctx).QueryRow(ctx, sqlStr, args...).Scan(
		&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.ProviderModelID,
		&p.ProjectID, &p.UsageType, &p.Price,
	); err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("price not found for usage type: %s", usageType)
		}
		return nil, err
	}
	return p, nil
}

// ----- helpers + gen↔domain -----------------------------------------

var providerModelColumns = []string{
	"id", "created_at", "updated_at", "project_id",
	"model_name", "match_pattern",
	"provider", "display_name",
	"start_date", "unit", "tokenizer_id", "tokenizer_config",
}

// scanProviderModel reads a row in providerModelColumns order.
func scanProviderModel(row interface {
	Scan(dest ...any) error
}) (*analyticsDomain.ProviderModel, error) {
	var (
		m       analyticsDomain.ProviderModel
		cfg     json.RawMessage
	)
	if err := row.Scan(
		&m.ID, &m.CreatedAt, &m.UpdatedAt, &m.ProjectID,
		&m.ModelName, &m.MatchPattern,
		&m.Provider, &m.DisplayName,
		&m.StartDate, &m.Unit, &m.TokenizerID, &cfg,
	); err != nil {
		return nil, err
	}
	parsed, err := unmarshalTokenizerConfig(cfg)
	if err != nil {
		return nil, err
	}
	m.TokenizerConfig = parsed
	return &m, nil
}

func providerModelFromRow(row *gen.ProviderModel) (*analyticsDomain.ProviderModel, error) {
	cfg, err := unmarshalTokenizerConfig(row.TokenizerConfig)
	if err != nil {
		return nil, err
	}
	return &analyticsDomain.ProviderModel{
		ID:              row.ID,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		ProjectID:       row.ProjectID,
		ModelName:       row.ModelName,
		MatchPattern:    row.MatchPattern,
		Provider:        row.Provider,
		DisplayName:     row.DisplayName,
		StartDate:       row.StartDate,
		Unit:            row.Unit,
		TokenizerID:     row.TokenizerID,
		TokenizerConfig: cfg,
	}, nil
}

func providerModelsFromRows(rows []gen.ProviderModel) ([]*analyticsDomain.ProviderModel, error) {
	out := make([]*analyticsDomain.ProviderModel, 0, len(rows))
	for i := range rows {
		m, err := providerModelFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func providerPriceFromRow(row *gen.ProviderPrice) *analyticsDomain.ProviderPrice {
	return &analyticsDomain.ProviderPrice{
		ID:              row.ID,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		ProviderModelID: row.ProviderModelID,
		ProjectID:       row.ProjectID,
		UsageType:       row.UsageType,
		Price:           row.Price,
	}
}

func providerPricesFromRows(rows []gen.ProviderPrice) []*analyticsDomain.ProviderPrice {
	out := make([]*analyticsDomain.ProviderPrice, 0, len(rows))
	for i := range rows {
		out = append(out, providerPriceFromRow(&rows[i]))
	}
	return out
}

func marshalTokenizerConfig(m map[string]any) (json.RawMessage, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return json.Marshal(m)
}

func unmarshalTokenizerConfig(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}
