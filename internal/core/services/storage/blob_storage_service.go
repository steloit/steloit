package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/storage"
	infraStorage "brokle/internal/infrastructure/storage"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/preview"
	"brokle/pkg/uid"
)

// Ensure BlobStorageService implements the interface
var _ storage.BlobStorageService = (*BlobStorageService)(nil)

// BlobStorageService implements business logic for blob storage management
type BlobStorageService struct {
	blobRepo storage.BlobStorageRepository
	s3Client *infraStorage.S3Client
	config   *config.BlobStorageConfig
	logger   *slog.Logger
}

// NewBlobStorageService creates a new blob storage service instance
func NewBlobStorageService(
	blobRepo storage.BlobStorageRepository,
	s3Client *infraStorage.S3Client,
	cfg *config.BlobStorageConfig,
	logger *slog.Logger,
) *BlobStorageService {
	return &BlobStorageService{
		blobRepo: blobRepo,
		s3Client: s3Client,
		config:   cfg,
		logger:   logger,
	}
}

// CreateBlobReference creates a new blob storage reference
func (s *BlobStorageService) CreateBlobReference(ctx context.Context, blob *storage.BlobStorageFileLog) error {
	if blob.ProjectID == uuid.Nil {
		return appErrors.NewValidationError("project_id is required", "blob must have a valid project_id")
	}
	if blob.EntityType == "" {
		return appErrors.NewValidationError("entity_type is required", "blob must have an entity_type")
	}
	if blob.EntityID == "" {
		return appErrors.NewValidationError("entity_id is required", "blob must have an entity_id")
	}
	if blob.BucketName == "" {
		return appErrors.NewValidationError("bucket_name is required", "blob must have a bucket_name")
	}
	if blob.BucketPath == "" {
		return appErrors.NewValidationError("bucket_path is required", "blob must have a bucket_path")
	}

	if blob.ID == "" {
		blob.ID = uid.New().String()
	}
	if blob.EventID == "" {
		blob.EventID = uid.New().String()
	}
	if blob.CreatedAt.IsZero() {
		blob.CreatedAt = time.Now()
	}

	if err := s.blobRepo.Create(ctx, blob); err != nil {
		return appErrors.NewInternalError("failed to create blob reference", err)
	}

	return nil
}

// UpdateBlobReference updates an existing blob storage reference
func (s *BlobStorageService) UpdateBlobReference(ctx context.Context, blob *storage.BlobStorageFileLog) error {
	existing, err := s.blobRepo.GetByID(ctx, blob.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("blob " + blob.ID)
		}
		return appErrors.NewInternalError("failed to get blob", err)
	}

	mergeBlobFields(existing, blob)

	if err := s.blobRepo.Update(ctx, existing); err != nil {
		return appErrors.NewInternalError("failed to update blob reference", err)
	}

	return nil
}

// DeleteBlobReference deletes a blob storage reference and its S3 object
func (s *BlobStorageService) DeleteBlobReference(ctx context.Context, id string) error {
	blob, err := s.blobRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return appErrors.NewNotFoundError("blob " + id)
		}
		return appErrors.NewInternalError("failed to get blob", err)
	}

	// S3 deletion is best-effort
	if s.s3Client != nil {
		if err := s.s3Client.Delete(ctx, blob.BucketPath); err != nil {
			s.logger.Warn("Failed to delete from S3, continuing with reference deletion", "error", err)
		}
	}

	if err := s.blobRepo.Delete(ctx, id); err != nil {
		return appErrors.NewInternalError("failed to delete blob reference", err)
	}

	return nil
}

// GetBlobByID retrieves a blob storage reference by ID
func (s *BlobStorageService) GetBlobByID(ctx context.Context, id string) (*storage.BlobStorageFileLog, error) {
	blob, err := s.blobRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, appErrors.NewNotFoundError("blob " + id)
		}
		return nil, appErrors.NewInternalError("failed to get blob", err)
	}

	return blob, nil
}

// GetBlobsByEntityID retrieves all blob references for an entity
func (s *BlobStorageService) GetBlobsByEntityID(ctx context.Context, entityType, entityID string) ([]*storage.BlobStorageFileLog, error) {
	blobs, err := s.blobRepo.GetByEntityID(ctx, entityType, entityID)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get blobs by entity", err)
	}

	return blobs, nil
}

// GetBlobsByProjectID retrieves blobs by project ID with optional filters
func (s *BlobStorageService) GetBlobsByProjectID(ctx context.Context, projectID uuid.UUID, filter *storage.BlobStorageFilter) ([]*storage.BlobStorageFileLog, error) {
	blobs, err := s.blobRepo.GetByProjectID(ctx, projectID, filter)
	if err != nil {
		return nil, appErrors.NewInternalError("failed to get blobs by project", err)
	}

	return blobs, nil
}

// ShouldOffload returns true if content exceeds the configured threshold (default: 10KB)
func (s *BlobStorageService) ShouldOffload(content string) bool {
	return len(content) > s.config.Threshold
}

// UploadToS3 uploads content to S3 and creates a blob reference
func (s *BlobStorageService) UploadToS3(ctx context.Context, content string, projectID uuid.UUID, entityType, entityID, eventID string) (*storage.BlobStorageFileLog, error) {
	if s.s3Client == nil {
		return nil, appErrors.NewInternalError("S3 client not initialized - check BLOB_STORAGE configuration in environment", nil)
	}

	blobID := uid.New().String()
	s3Key := fmt.Sprintf("%s/%s/%s.json", entityType, entityID, blobID)

	contentBytes := []byte(content)
	if err := s.s3Client.Upload(ctx, s3Key, contentBytes, "application/json"); err != nil {
		return nil, appErrors.NewInternalError("failed to upload to S3", err)
	}

	blob := &storage.BlobStorageFileLog{
		ID:         blobID,
		ProjectID:  projectID,
		EntityType: entityType,
		EntityID:   entityID,
		EventID:    eventID,
		BucketName: s.config.BucketName,
		BucketPath: s3Key,
		FileSizeBytes: func() *uint64 {
			size := uint64(len(contentBytes))
			return &size
		}(),
		ContentType: func() *string {
			ct := "application/json"
			return &ct
		}(),
		CreatedAt: time.Now(),
	}

	if err := s.CreateBlobReference(ctx, blob); err != nil {
		_ = s.s3Client.Delete(ctx, s3Key) // Cleanup on failure
		return nil, err
	}

	return blob, nil
}

// UploadToS3WithPreview uploads content to S3 and returns blob info + preview
func (s *BlobStorageService) UploadToS3WithPreview(ctx context.Context, content string, projectID uuid.UUID, entityType, entityID, eventID string) (*storage.BlobStorageFileLog, string, error) {
	blob, err := s.UploadToS3(ctx, content, projectID, entityType, entityID, eventID)
	if err != nil {
		return nil, "", err
	}

	previewText := preview.GeneratePreview(content)
	return blob, previewText, nil
}

// DownloadFromS3 downloads content from S3 using blob reference
func (s *BlobStorageService) DownloadFromS3(ctx context.Context, blobID string) (string, error) {
	if s.s3Client == nil {
		return "", appErrors.NewInternalError("S3 client not initialized - check BLOB_STORAGE configuration in environment", nil)
	}

	blob, err := s.blobRepo.GetByID(ctx, blobID)
	if err != nil {
		return "", appErrors.NewNotFoundError("blob " + blobID)
	}

	contentBytes, err := s.s3Client.Download(ctx, blob.BucketPath)
	if err != nil {
		return "", appErrors.NewInternalError("failed to download from S3", err)
	}

	return string(contentBytes), nil
}

// CountBlobs returns the count of blob references matching the filter
func (s *BlobStorageService) CountBlobs(ctx context.Context, filter *storage.BlobStorageFilter) (int64, error) {
	count, err := s.blobRepo.Count(ctx, filter)
	if err != nil {
		return 0, appErrors.NewInternalError("failed to count blobs", err)
	}

	return count, nil
}

func mergeBlobFields(dst *storage.BlobStorageFileLog, src *storage.BlobStorageFileLog) {
	if src.FileSizeBytes != nil {
		dst.FileSizeBytes = src.FileSizeBytes
	}
	if src.ContentType != nil {
		dst.ContentType = src.ContentType
	}
	if src.Compression != nil {
		dst.Compression = src.Compression
	}
}
