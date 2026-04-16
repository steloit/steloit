package observability

import (
	"log/slog"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	"brokle/internal/core/domain/observability"
	storageDomain "brokle/internal/core/domain/storage"
	infraStorage "brokle/internal/infrastructure/storage"
	"brokle/internal/infrastructure/streams"
)

type ServiceRegistry struct {
	TraceService          *TraceService
	ScoreService          *ScoreService
	ScoreAnalyticsService *ScoreAnalyticsService
	MetricsService        *MetricsService
	LogsService           *LogsService
	GenAIEventsService    *GenAIEventsService
	BlobStorageService    storageDomain.BlobStorageService
	ArchiveService        *ArchiveService
	SpanQueryService      *SpanQueryService
	FilterPresetService   *FilterPresetService

	OTLPConverterService        *OTLPConverterService
	OTLPMetricsConverterService *OTLPMetricsConverterService
	OTLPLogsConverterService    *OTLPLogsConverterService
	OTLPEventsConverterService  *OTLPEventsConverterService

	StreamProducer       *streams.TelemetryStreamProducer
	DeduplicationService observability.TelemetryDeduplicationService
	TelemetryService     observability.TelemetryService
}

func NewServiceRegistry(
	traceRepo observability.TraceRepository,
	scoreRepo observability.ScoreRepository,
	scoreAnalyticsRepo observability.ScoreAnalyticsRepository,
	metricsRepo observability.MetricsRepository,
	logsRepo observability.LogsRepository,
	genaiEventsRepo observability.GenAIEventsRepository,
	filterPresetRepo observability.FilterPresetRepository,
	blobStorageService storageDomain.BlobStorageService,
	s3Client *infraStorage.S3Client,
	archiveConfig *config.ArchiveConfig,

	streamProducer *streams.TelemetryStreamProducer,
	deduplicationService observability.TelemetryDeduplicationService,

	telemetryService observability.TelemetryService,
	providerPricingService analytics.ProviderPricingService,
	observabilityConfig *config.ObservabilityConfig,

	logger *slog.Logger,
) *ServiceRegistry {
	otlpConverterService := NewOTLPConverterService(logger, providerPricingService, observabilityConfig)
	otlpMetricsConverterService := NewOTLPMetricsConverterService(logger)
	otlpLogsConverterService := NewOTLPLogsConverterService(logger)
	otlpEventsConverterService := NewOTLPEventsConverterService(logger)
	traceService := NewTraceService(traceRepo, logger)
	scoreService := NewScoreService(scoreRepo, traceRepo)
	scoreAnalyticsService := NewScoreAnalyticsService(scoreAnalyticsRepo, logger)
	metricsService := NewMetricsService(metricsRepo, logger)
	logsService := NewLogsService(logsRepo, logger)
	genaiEventsService := NewGenAIEventsService(genaiEventsRepo, logger)
	spanQueryService := NewSpanQueryService(traceRepo, logger)
	filterPresetService := NewFilterPresetService(filterPresetRepo, logger)

	var archiveService *ArchiveService
	if archiveConfig != nil && archiveConfig.Enabled && s3Client != nil {
		parquetWriter := NewParquetWriter(archiveConfig.CompressionLevel)
		archiveService = NewArchiveService(s3Client, parquetWriter, blobStorageService, archiveConfig, logger)
		logger.Info("Archive service initialized for S3 raw telemetry archival", "bucket", s3Client.GetBucketName(), "path_prefix", archiveConfig.PathPrefix, "compression_level", archiveConfig.CompressionLevel)
	}

	return &ServiceRegistry{
		TraceService:                traceService,
		ScoreService:                scoreService,
		ScoreAnalyticsService:       scoreAnalyticsService,
		MetricsService:              metricsService,
		LogsService:                 logsService,
		GenAIEventsService:          genaiEventsService,
		BlobStorageService:          blobStorageService,
		ArchiveService:              archiveService,
		SpanQueryService:            spanQueryService,
		FilterPresetService:         filterPresetService,
		OTLPConverterService:        otlpConverterService,
		OTLPMetricsConverterService: otlpMetricsConverterService,
		OTLPLogsConverterService:    otlpLogsConverterService,
		OTLPEventsConverterService:  otlpEventsConverterService,
		StreamProducer:              streamProducer,
		DeduplicationService:        deduplicationService,
		TelemetryService:            telemetryService,
	}
}

func (r *ServiceRegistry) GetTraceService() *TraceService {
	return r.TraceService
}

func (r *ServiceRegistry) GetScoreService() *ScoreService {
	return r.ScoreService
}

func (r *ServiceRegistry) GetBlobStorageService() storageDomain.BlobStorageService {
	return r.BlobStorageService
}

func (r *ServiceRegistry) GetOTLPConverterService() *OTLPConverterService {
	return r.OTLPConverterService
}

func (r *ServiceRegistry) GetTelemetryService() observability.TelemetryService {
	return r.TelemetryService
}

func (r *ServiceRegistry) GetSpanQueryService() *SpanQueryService {
	return r.SpanQueryService
}

func (r *ServiceRegistry) GetScoreAnalyticsService() *ScoreAnalyticsService {
	return r.ScoreAnalyticsService
}
