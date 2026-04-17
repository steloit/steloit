package evaluation

import (
	"context"

	"github.com/google/uuid"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type DatasetItemRepository struct {
	tm *db.TxManager
}

func NewDatasetItemRepository(tm *db.TxManager) *DatasetItemRepository {
	return &DatasetItemRepository{tm: tm}
}

func (r *DatasetItemRepository) Create(ctx context.Context, item *evalDomain.DatasetItem) error {
	return r.insertOne(ctx, item)
}

func (r *DatasetItemRepository) CreateBatch(ctx context.Context, items []*evalDomain.DatasetItem) error {
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

func (r *DatasetItemRepository) insertOne(ctx context.Context, item *evalDomain.DatasetItem) error {
	input, err := marshalEvalJSON(item.Input)
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
	return r.tm.Queries(ctx).CreateDatasetItem(ctx, gen.CreateDatasetItemParams{
		ID:            item.ID,
		DatasetID:     item.DatasetID,
		Input:         input,
		Expected:      expected,
		Metadata:      meta,
		Source:        string(item.Source),
		SourceTraceID: item.SourceTraceID,
		SourceSpanID:  item.SourceSpanID,
		ContentHash:   item.ContentHash,
	})
}

func (r *DatasetItemRepository) GetByID(ctx context.Context, id, datasetID uuid.UUID) (*evalDomain.DatasetItem, error) {
	row, err := r.tm.Queries(ctx).GetDatasetItemByID(ctx, gen.GetDatasetItemByIDParams{
		ID:        id,
		DatasetID: datasetID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrDatasetItemNotFound
		}
		return nil, err
	}
	return datasetItemFromRow(&row)
}

func (r *DatasetItemRepository) GetByIDForProject(ctx context.Context, id, projectID uuid.UUID) (*evalDomain.DatasetItem, error) {
	row, err := r.tm.Queries(ctx).GetDatasetItemByIDForProject(ctx, gen.GetDatasetItemByIDForProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrDatasetItemNotFound
		}
		return nil, err
	}
	return datasetItemFromRow(&row)
}

func (r *DatasetItemRepository) List(ctx context.Context, datasetID uuid.UUID, limit, offset int) ([]*evalDomain.DatasetItem, int64, error) {
	total, err := r.tm.Queries(ctx).CountDatasetItems(ctx, datasetID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.Queries(ctx).ListDatasetItems(ctx, gen.ListDatasetItemsParams{
		DatasetID: datasetID,
		Limit:     int32(limit),
		Offset:    int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	return datasetItemsFromRows(rows, total)
}

func (r *DatasetItemRepository) Delete(ctx context.Context, id, datasetID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteDatasetItem(ctx, gen.DeleteDatasetItemParams{
		ID:        id,
		DatasetID: datasetID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrDatasetItemNotFound
	}
	return nil
}

func (r *DatasetItemRepository) CountByDataset(ctx context.Context, datasetID uuid.UUID) (int64, error) {
	return r.tm.Queries(ctx).CountDatasetItems(ctx, datasetID)
}

func (r *DatasetItemRepository) FindByContentHash(ctx context.Context, datasetID uuid.UUID, contentHash string) (*evalDomain.DatasetItem, error) {
	row, err := r.tm.Queries(ctx).FindDatasetItemByContentHash(ctx, gen.FindDatasetItemByContentHashParams{
		DatasetID:   datasetID,
		ContentHash: &contentHash,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	return datasetItemFromRow(&row)
}

func (r *DatasetItemRepository) FindByContentHashes(ctx context.Context, datasetID uuid.UUID, contentHashes []string) (map[string]bool, error) {
	if len(contentHashes) == 0 {
		return map[string]bool{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListDatasetItemContentHashes(ctx, gen.ListDatasetItemContentHashesParams{
		DatasetID: datasetID,
		Column2:   contentHashes,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, h := range rows {
		if h != nil {
			out[*h] = true
		}
	}
	return out, nil
}

func (r *DatasetItemRepository) ListAll(ctx context.Context, datasetID uuid.UUID) ([]*evalDomain.DatasetItem, error) {
	rows, err := r.tm.Queries(ctx).ListAllDatasetItems(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	items, _, err := datasetItemsFromRows(rows, 0)
	return items, err
}

func datasetItemFromRow(row *gen.DatasetItem) (*evalDomain.DatasetItem, error) {
	it := &evalDomain.DatasetItem{
		ID:            row.ID,
		DatasetID:     row.DatasetID,
		CreatedAt:     row.CreatedAt,
		Source:        evalDomain.DatasetItemSource(row.Source),
		SourceTraceID: row.SourceTraceID,
		SourceSpanID:  row.SourceSpanID,
		ContentHash:   row.ContentHash,
	}
	if err := unmarshalEvalJSON(row.Input, &it.Input); err != nil {
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

func datasetItemsFromRows(rows []gen.DatasetItem, total int64) ([]*evalDomain.DatasetItem, int64, error) {
	out := make([]*evalDomain.DatasetItem, 0, len(rows))
	for i := range rows {
		it, err := datasetItemFromRow(&rows[i])
		if err != nil {
			return nil, 0, err
		}
		out = append(out, it)
	}
	return out, total, nil
}
