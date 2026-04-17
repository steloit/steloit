package evaluation

import (
	"context"

	"github.com/google/uuid"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
)

type DatasetVersionRepository struct {
	tm *db.TxManager
}

func NewDatasetVersionRepository(tm *db.TxManager) *DatasetVersionRepository {
	return &DatasetVersionRepository{tm: tm}
}

func (r *DatasetVersionRepository) Create(ctx context.Context, v *evalDomain.DatasetVersion) error {
	meta, err := marshalEvalJSON(v.Metadata)
	if err != nil {
		return err
	}
	if err := r.tm.Queries(ctx).CreateDatasetVersion(ctx, gen.CreateDatasetVersionParams{
		ID:          v.ID,
		DatasetID:   v.DatasetID,
		Version:     int32(v.Version),
		ItemCount:   int32(v.ItemCount),
		Description: v.Description,
		Metadata:    meta,
		CreatedBy:   v.CreatedBy,
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return evalDomain.ErrDatasetVersionExists
		}
		return err
	}
	return nil
}

func (r *DatasetVersionRepository) GetByID(ctx context.Context, id, datasetID uuid.UUID) (*evalDomain.DatasetVersion, error) {
	row, err := r.tm.Queries(ctx).GetDatasetVersionByID(ctx, gen.GetDatasetVersionByIDParams{
		ID:        id,
		DatasetID: datasetID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrDatasetVersionNotFound
		}
		return nil, err
	}
	return datasetVersionFromRow(&row)
}

func (r *DatasetVersionRepository) GetByVersionNumber(ctx context.Context, datasetID uuid.UUID, versionNum int) (*evalDomain.DatasetVersion, error) {
	row, err := r.tm.Queries(ctx).GetDatasetVersionByNumber(ctx, gen.GetDatasetVersionByNumberParams{
		DatasetID: datasetID,
		Version:   int32(versionNum),
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrDatasetVersionNotFound
		}
		return nil, err
	}
	return datasetVersionFromRow(&row)
}

func (r *DatasetVersionRepository) GetLatest(ctx context.Context, datasetID uuid.UUID) (*evalDomain.DatasetVersion, error) {
	row, err := r.tm.Queries(ctx).GetLatestDatasetVersion(ctx, datasetID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrDatasetVersionNotFound
		}
		return nil, err
	}
	return datasetVersionFromRow(&row)
}

func (r *DatasetVersionRepository) List(ctx context.Context, datasetID uuid.UUID) ([]*evalDomain.DatasetVersion, error) {
	rows, err := r.tm.Queries(ctx).ListDatasetVersions(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	out := make([]*evalDomain.DatasetVersion, 0, len(rows))
	for i := range rows {
		v, err := datasetVersionFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *DatasetVersionRepository) GetNextVersionNumber(ctx context.Context, datasetID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).GetNextDatasetVersionNumber(ctx, datasetID)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func (r *DatasetVersionRepository) AddItems(ctx context.Context, versionID uuid.UUID, itemIDs []uuid.UUID) error {
	if len(itemIDs) == 0 {
		return nil
	}
	versionIDs := make([]uuid.UUID, len(itemIDs))
	for i := range itemIDs {
		versionIDs[i] = versionID
	}
	return r.tm.Queries(ctx).InsertDatasetItemVersions(ctx, gen.InsertDatasetItemVersionsParams{
		Column1: versionIDs,
		Column2: itemIDs,
	})
}

func (r *DatasetVersionRepository) GetItemIDs(ctx context.Context, versionID uuid.UUID) ([]uuid.UUID, error) {
	return r.tm.Queries(ctx).ListDatasetItemIDsForVersion(ctx, versionID)
}

func (r *DatasetVersionRepository) GetItems(ctx context.Context, versionID uuid.UUID, limit, offset int) ([]*evalDomain.DatasetItem, int64, error) {
	total, err := r.tm.Queries(ctx).CountDatasetItemsForVersion(ctx, versionID)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.Queries(ctx).ListDatasetItemsForVersion(ctx, gen.ListDatasetItemsForVersionParams{
		DatasetVersionID: versionID,
		Limit:            int32(limit),
		Offset:           int32(offset),
	})
	if err != nil {
		return nil, 0, err
	}
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

func datasetVersionFromRow(row *gen.DatasetVersion) (*evalDomain.DatasetVersion, error) {
	v := &evalDomain.DatasetVersion{
		ID:          row.ID,
		DatasetID:   row.DatasetID,
		Version:     int(row.Version),
		ItemCount:   int(row.ItemCount),
		Description: row.Description,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
	}
	if err := unmarshalEvalJSON(row.Metadata, &v.Metadata); err != nil {
		return nil, err
	}
	return v, nil
}
