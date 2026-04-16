package observability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/observability"
	storageDomain "brokle/internal/core/domain/storage"
	infraStorage "brokle/internal/infrastructure/storage"
	"brokle/pkg/uid"
)

// ArchiveService handles raw telemetry archival to S3 in Parquet format with ZSTD compression.
type ArchiveService struct {
	s3Client           *infraStorage.S3Client
	parquetWriter      *ParquetWriter
	blobStorageService storageDomain.BlobStorageService
	config             *config.ArchiveConfig
	logger             *slog.Logger
}

func NewArchiveService(
	s3Client *infraStorage.S3Client,
	parquetWriter *ParquetWriter,
	blobStorageService storageDomain.BlobStorageService,
	cfg *config.ArchiveConfig,
	logger *slog.Logger,
) *ArchiveService {
	return &ArchiveService{
		s3Client:           s3Client,
		parquetWriter:      parquetWriter,
		blobStorageService: blobStorageService,
		config:             cfg,
		logger:             logger,
	}
}

// ArchiveBatch writes raw telemetry records to S3 as Parquet and returns the S3 path.
func (s *ArchiveService) ArchiveBatch(
	ctx context.Context,
	projectID string,
	batchID uuid.UUID,
	records []observability.RawTelemetryRecord,
) (*observability.ArchiveBatchResult, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("no records to archive")
	}

	signalType := records[0].SignalType
	timestamp := records[0].Timestamp
	s3Path := s.GenerateS3Path(projectID, signalType, timestamp, batchID)

	parquetData, err := s.parquetWriter.WriteRecords(records)
	if err != nil {
		return nil, fmt.Errorf("failed to write parquet: %w", err)
	}

	if err := s.s3Client.Upload(ctx, s3Path, parquetData, "application/x-parquet"); err != nil {
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	now := time.Now()
	blobRef := &storageDomain.BlobStorageFileLog{
		ID:         uid.New().String(),
		ProjectID:  projectID,
		EntityType: observability.EntityTypeArchiveBatch,
		EntityID:   batchID.String(),
		EventID:    records[0].RecordID,
		BucketName: s.s3Client.GetBucketName(),
		BucketPath: s3Path,
		FileSizeBytes: func() *uint64 {
			size := uint64(len(parquetData))
			return &size
		}(),
		ContentType: func() *string {
			ct := "application/x-parquet"
			return &ct
		}(),
		Compression: func() *string {
			comp := "zstd"
			return &comp
		}(),
		CreatedAt: now,
	}

	if err := s.blobStorageService.CreateBlobReference(ctx, blobRef); err != nil {
		s.logger.Warn("Failed to create blob reference (S3 upload succeeded)", "error", err, "batch_id", batchID.String(), "s3_path", s3Path)
	}

	return &observability.ArchiveBatchResult{
		S3Path:        s3Path,
		BucketName:    s.s3Client.GetBucketName(),
		RecordCount:   len(records),
		FileSizeBytes: int64(len(parquetData)),
		ArchivedAt:    now,
	}, nil
}

// GenerateS3Path creates Hive-style partition path: {prefix}/project_id={id}/signal={type}/year={y}/month={m}/day={d}/{batch_id}.parquet
func (s *ArchiveService) GenerateS3Path(projectID, signalType string, timestamp time.Time, batchID uuid.UUID) string {
	return fmt.Sprintf(
		"%sproject_id=%s/signal=%s/year=%04d/month=%02d/day=%02d/%s.parquet",
		s.config.PathPrefix,
		projectID,
		signalType,
		timestamp.Year(),
		timestamp.Month(),
		timestamp.Day(),
		batchID.String(),
	)
}

func (s *ArchiveService) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}
