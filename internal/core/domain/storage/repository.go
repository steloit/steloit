package storage

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

// BlobStorageRepository defines the interface for blob storage data access
type BlobStorageRepository interface {
	Create(ctx context.Context, blob *BlobStorageFileLog) error
	Update(ctx context.Context, blob *BlobStorageFileLog) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*BlobStorageFileLog, error)
	GetByEntityID(ctx context.Context, entityType, entityID string) ([]*BlobStorageFileLog, error)
	GetByProjectID(ctx context.Context, projectID uuid.UUID, filter *BlobStorageFilter) ([]*BlobStorageFileLog, error)
	Count(ctx context.Context, filter *BlobStorageFilter) (int64, error)
}

// BlobStorageFilter defines filter criteria for querying blob storage records
type BlobStorageFilter struct {
	EntityType   *string
	StartTime    *time.Time
	EndTime      *time.Time
	MinSizeBytes *uint64
	MaxSizeBytes *uint64
	Params       pagination.Params
}
