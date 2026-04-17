package evaluation

import (
	"context"

	"github.com/google/uuid"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type ExperimentItemRepository struct {
	tm *db.TxManager
}

func NewExperimentItemRepository(tm *db.TxManager) *ExperimentItemRepository {
	return &ExperimentItemRepository{tm: tm}
}

func (r *ExperimentItemRepository) Create(ctx context.Context, item *evalDomain.ExperimentItem) error {
	return r.insertOne(ctx, item)
}

func (r *ExperimentItemRepository) CreateBatch(ctx context.Context, items []*evalDomain.ExperimentItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.tm.WithinTransaction(ctx, func(ctx context.Context) error {
		for _, it := range items {
			if err := r.insertOne(ctx, it); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ExperimentItemRepository) insertOne(ctx context.Context, item *evalDomain.ExperimentItem) error {
	input, err := marshalEvalJSON(item.Input)
	if err != nil {
		return err
	}
	output, err := marshalEvalJSON(item.Output)
	if err != nil {
		return err
	}
	expected, err := marshalEvalJSON(item.Expected)
	if err != nil {
		return err
	}
	meta, err := marshalEvalJSON(item.Metadata)
	if err != nil {
		return err
	}
	return r.tm.Queries(ctx).CreateExperimentItem(ctx, gen.CreateExperimentItemParams{
		ID:            item.ID,
		ExperimentID:  item.ExperimentID,
		DatasetItemID: item.DatasetItemID,
		TraceID:       item.TraceID,
		Input:         input,
		Output:        output,
		Expected:      expected,
		TrialNumber:   int32(item.TrialNumber),
		Metadata:      meta,
		Error:         item.Error,
	})
}

func (r *ExperimentItemRepository) List(ctx context.Context, experimentID uuid.UUID, limit, offset int) ([]*evalDomain.ExperimentItem, int64, error) {
	total, err := r.tm.Queries(ctx).CountExperimentItems(ctx, experimentID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.Queries(ctx).ListExperimentItems(ctx, gen.ListExperimentItemsParams{
		ExperimentID: experimentID,
		Limit:        int32(limit),
		Offset:       int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]*evalDomain.ExperimentItem, 0, len(rows))
	for i := range rows {
		it, err := experimentItemFromRow(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, nil
}

func (r *ExperimentItemRepository) CountByExperiment(ctx context.Context, experimentID uuid.UUID) (int64, error) {
	return r.tm.Queries(ctx).CountExperimentItems(ctx, experimentID)
}

func experimentItemFromRow(row *gen.ExperimentItem) (*evalDomain.ExperimentItem, error) {
	it := &evalDomain.ExperimentItem{
		ID:            row.ID,
		ExperimentID:  row.ExperimentID,
		DatasetItemID: row.DatasetItemID,
		TraceID:       row.TraceID,
		TrialNumber:   int(row.TrialNumber),
		CreatedAt:     row.CreatedAt,
		Error:         row.Error,
	}
	if err := unmarshalEvalJSON(row.Input, &it.Input); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.Output, &it.Output); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.Expected, &it.Expected); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.Metadata, &it.Metadata); err != nil {
		return nil, err
	}
	return it, nil
}
