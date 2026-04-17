package evaluation

import (
	"context"

	"github.com/google/uuid"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type ExperimentConfigRepository struct {
	tm *db.TxManager
}

func NewExperimentConfigRepository(tm *db.TxManager) *ExperimentConfigRepository {
	return &ExperimentConfigRepository{tm: tm}
}

func (r *ExperimentConfigRepository) Create(ctx context.Context, c *evalDomain.ExperimentConfig) error {
	modelCfg, err := marshalEvalJSON(c.ModelConfig)
	if err != nil {
		return err
	}
	variableMapping, err := marshalEvalJSON(c.VariableMapping)
	if err != nil {
		return err
	}
	evaluators, err := marshalEvalJSON(c.Evaluators)
	if err != nil {
		return err
	}
	return r.tm.Queries(ctx).CreateExperimentConfig(ctx, gen.CreateExperimentConfigParams{
		ID:               c.ID,
		ExperimentID:     c.ExperimentID,
		PromptID:         c.PromptID,
		PromptVersionID:  c.PromptVersionID,
		ModelConfig:      modelCfg,
		DatasetID:        c.DatasetID,
		DatasetVersionID: c.DatasetVersionID,
		VariableMapping:  variableMapping,
		Evaluators:       evaluators,
	})
}

func (r *ExperimentConfigRepository) GetByID(ctx context.Context, id uuid.UUID) (*evalDomain.ExperimentConfig, error) {
	row, err := r.tm.Queries(ctx).GetExperimentConfigByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrExperimentConfigNotFound
		}
		return nil, err
	}
	return experimentConfigFromRow(&row)
}

func (r *ExperimentConfigRepository) GetByExperimentID(ctx context.Context, experimentID uuid.UUID) (*evalDomain.ExperimentConfig, error) {
	row, err := r.tm.Queries(ctx).GetExperimentConfigByExperimentID(ctx, experimentID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrExperimentConfigNotFound
		}
		return nil, err
	}
	return experimentConfigFromRow(&row)
}

func (r *ExperimentConfigRepository) Update(ctx context.Context, c *evalDomain.ExperimentConfig) error {
	modelCfg, err := marshalEvalJSON(c.ModelConfig)
	if err != nil {
		return err
	}
	variableMapping, err := marshalEvalJSON(c.VariableMapping)
	if err != nil {
		return err
	}
	evaluators, err := marshalEvalJSON(c.Evaluators)
	if err != nil {
		return err
	}
	n, err := r.tm.Queries(ctx).UpdateExperimentConfig(ctx, gen.UpdateExperimentConfigParams{
		ID:               c.ID,
		PromptID:         c.PromptID,
		PromptVersionID:  c.PromptVersionID,
		ModelConfig:      modelCfg,
		DatasetID:        c.DatasetID,
		DatasetVersionID: c.DatasetVersionID,
		VariableMapping:  variableMapping,
		Evaluators:       evaluators,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentConfigNotFound
	}
	return nil
}

func (r *ExperimentConfigRepository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteExperimentConfig(ctx, id)
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrExperimentConfigNotFound
	}
	return nil
}

func experimentConfigFromRow(row *gen.ExperimentConfig) (*evalDomain.ExperimentConfig, error) {
	c := &evalDomain.ExperimentConfig{
		ID:               row.ID,
		ExperimentID:     row.ExperimentID,
		PromptID:         row.PromptID,
		PromptVersionID:  row.PromptVersionID,
		DatasetID:        row.DatasetID,
		DatasetVersionID: row.DatasetVersionID,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
	if err := unmarshalEvalJSON(row.ModelConfig, &c.ModelConfig); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.VariableMapping, &c.VariableMapping); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.Evaluators, &c.Evaluators); err != nil {
		return nil, err
	}
	return c, nil
}
