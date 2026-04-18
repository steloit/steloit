package storage

import (
	"context"

	"github.com/google/uuid"
)

// BlobStorageService defines the interface for blob storage business operations.
// ID / entityID / eventID remain string to match the ClickHouse String
// column types (EntityID/EventID are polymorphic; blob ID is opaque).
// ProjectID is a strongly-typed uuid.UUID — it maps to a UUID column.
type BlobStorageService interface {
	CreateBlobReference(ctx context.Context, blob *BlobStorageFileLog) error
	GetBlobByID(ctx context.Context, id string) (*BlobStorageFileLog, error)
	GetBlobsByEntityID(ctx context.Context, entityType, entityID string) ([]*BlobStorageFileLog, error)
	GetBlobsByProjectID(ctx context.Context, projectID uuid.UUID, filter *BlobStorageFilter) ([]*BlobStorageFileLog, error)
	UpdateBlobReference(ctx context.Context, blob *BlobStorageFileLog) error
	DeleteBlobReference(ctx context.Context, id string) error
	ShouldOffload(content string) bool
	UploadToS3(ctx context.Context, content string, projectID uuid.UUID, entityType, entityID, eventID string) (*BlobStorageFileLog, error)
	UploadToS3WithPreview(ctx context.Context, content string, projectID uuid.UUID, entityType, entityID, eventID string) (*BlobStorageFileLog, string, error)
	DownloadFromS3(ctx context.Context, blobID string) (string, error)
	CountBlobs(ctx context.Context, filter *BlobStorageFilter) (int64, error)
}
