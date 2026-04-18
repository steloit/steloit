package storage

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"brokle/internal/core/domain/storage"
	"brokle/pkg/pagination"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

var _ storage.BlobStorageRepository = (*blobStorageRepository)(nil)

type blobStorageRepository struct {
	db clickhouse.Conn
}

func NewBlobStorageRepository(db clickhouse.Conn) storage.BlobStorageRepository {
	return &blobStorageRepository{db: db}
}

func (r *blobStorageRepository) Create(ctx context.Context, blob *storage.BlobStorageFileLog) error {
	query := `
		INSERT INTO blob_storage_file_log (
			id, project_id, entity_type, entity_id, event_id,
			bucket_name, bucket_path,
			file_size_bytes, content_type, compression,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	return r.db.Exec(ctx, query,
		blob.ID,
		blob.ProjectID,
		blob.EntityType,
		blob.EntityID,
		blob.EventID,
		blob.BucketName,
		blob.BucketPath,
		blob.FileSizeBytes,
		blob.ContentType,
		blob.Compression,
		blob.CreatedAt,
	)
}

func (r *blobStorageRepository) Update(ctx context.Context, blob *storage.BlobStorageFileLog) error {
	return r.Create(ctx, blob)
}

func (r *blobStorageRepository) Delete(ctx context.Context, id string) error {
	query := `ALTER TABLE blob_storage_file_log DELETE WHERE id = ?`
	return r.db.Exec(ctx, query, id)
}

func (r *blobStorageRepository) GetByID(ctx context.Context, id string) (*storage.BlobStorageFileLog, error) {
	query := `
		SELECT
			id, project_id, entity_type, entity_id, event_id,
			bucket_name, bucket_path,
			file_size_bytes, content_type, compression,
			created_at
		FROM blob_storage_file_log
		WHERE id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`

	row := r.db.QueryRow(ctx, query, id)
	return r.scanBlobRow(row)
}

func (r *blobStorageRepository) GetByEntityID(ctx context.Context, entityType, entityID string) ([]*storage.BlobStorageFileLog, error) {
	query := `
		SELECT
			id, project_id, entity_type, entity_id, event_id,
			bucket_name, bucket_path,
			file_size_bytes, content_type, compression,
			created_at
		FROM blob_storage_file_log
		WHERE entity_type = ? AND entity_id = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("query blobs by entity: %w", err)
	}
	defer rows.Close()

	return r.scanBlobs(rows)
}

func (r *blobStorageRepository) GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *storage.BlobStorageFilter) ([]*storage.BlobStorageFileLog, error) {
	query := `
		SELECT
			id, project_id, entity_type, entity_id, event_id,
			bucket_name, bucket_path,
			file_size_bytes, content_type, compression,
			created_at
		FROM blob_storage_file_log
		WHERE project_id = ?
	`

	args := []any{projectID}

	if filter != nil {
		if filter.EntityType != nil {
			query += " AND entity_type = ?"
			args = append(args, *filter.EntityType)
		}
		if filter.StartTime != nil {
			query += " AND created_at >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND created_at <= ?"
			args = append(args, *filter.EndTime)
		}
		if filter.MinSizeBytes != nil {
			query += " AND file_size_bytes >= ?"
			args = append(args, *filter.MinSizeBytes)
		}
		if filter.MaxSizeBytes != nil {
			query += " AND file_size_bytes <= ?"
			args = append(args, *filter.MaxSizeBytes)
		}
	}

	// SQL injection protection via whitelist validation
	allowedSortFields := []string{"created_at", "file_size_bytes", "id"}
	sortField := "created_at"
	sortDir := "DESC"

	if filter != nil {
		if filter.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filter.Params.SortBy, allowedSortFields)
			if err != nil {
				return nil, fmt.Errorf("invalid sort field: %w", err)
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filter.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}

	query += fmt.Sprintf(" ORDER BY %s %s", sortField, sortDir)

	limit := pagination.DefaultPageSize
	offset := 0
	if filter != nil {
		if filter.Params.Limit > 0 {
			limit = filter.Params.Limit
		}
		offset = filter.Params.GetOffset()
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query blobs by project: %w", err)
	}
	defer rows.Close()

	return r.scanBlobs(rows)
}

func (r *blobStorageRepository) Count(ctx context.Context, filter *storage.BlobStorageFilter) (int64, error) {
	query := "SELECT count() FROM blob_storage_file_log WHERE 1=1"
	args := []any{}

	if filter != nil {
		if filter.EntityType != nil {
			query += " AND entity_type = ?"
			args = append(args, *filter.EntityType)
		}
		if filter.StartTime != nil {
			query += " AND created_at >= ?"
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			query += " AND created_at <= ?"
			args = append(args, *filter.EndTime)
		}
	}

	var count int64
	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	return count, err
}

func (r *blobStorageRepository) scanBlobRow(row driver.Row) (*storage.BlobStorageFileLog, error) {
	var blob storage.BlobStorageFileLog

	err := row.Scan(
		&blob.ID,
		&blob.ProjectID,
		&blob.EntityType,
		&blob.EntityID,
		&blob.EventID,
		&blob.BucketName,
		&blob.BucketPath,
		&blob.FileSizeBytes,
		&blob.ContentType,
		&blob.Compression,
		&blob.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("scan blob: %w", err)
	}

	return &blob, nil
}

func (r *blobStorageRepository) scanBlobs(rows driver.Rows) ([]*storage.BlobStorageFileLog, error) {
	var blobs []*storage.BlobStorageFileLog

	for rows.Next() {
		var blob storage.BlobStorageFileLog

		err := rows.Scan(
			&blob.ID,
			&blob.ProjectID,
			&blob.EntityType,
			&blob.EntityID,
			&blob.EventID,
			&blob.BucketName,
			&blob.BucketPath,
			&blob.FileSizeBytes,
			&blob.ContentType,
			&blob.Compression,
			&blob.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("scan blob row: %w", err)
		}

		blobs = append(blobs, &blob)
	}

	return blobs, rows.Err()
}
