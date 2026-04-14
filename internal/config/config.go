// Package config provides configuration management for the Brokle platform.
//
// Configuration is loaded from multiple sources in this order:
// 1. Configuration files (YAML)
// 2. Environment variables
// 3. Command line flags (if applicable)
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config represents the complete application configuration.
type Config struct {
	External      ExternalConfig      `mapstructure:"external"`
	Auth          AuthConfig          `mapstructure:"auth"`
	ClickHouse    ClickHouseConfig    `mapstructure:"clickhouse"`
	Database      DatabaseConfig      `mapstructure:"database"`
	App           AppConfig           `mapstructure:"app"`
	Environment   string              `mapstructure:"environment"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
	Enterprise    EnterpriseConfig    `mapstructure:"enterprise"`
	Server        ServerConfig        `mapstructure:"server"`
	GRPC          GRPCConfig          `mapstructure:"grpc"`
	BlobStorage   BlobStorageConfig   `mapstructure:"blob_storage"`
	Monitoring    MonitoringConfig    `mapstructure:"monitoring"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Archive       ArchiveConfig       `mapstructure:"archive"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	Redis         RedisConfig         `mapstructure:"redis"`
	Workers       WorkersConfig       `mapstructure:"workers"`
	Features      FeatureConfig       `mapstructure:"features"`
	Encryption    EncryptionConfig    `mapstructure:"encryption"`
}

// EncryptionConfig contains encryption settings for sensitive data at rest.
type EncryptionConfig struct {
	// AIKeyEncryptionKey is the 256-bit key for encrypting AI provider API keys (base64 encoded).
	// Generate with: openssl rand -base64 32
	AIKeyEncryptionKey string `mapstructure:"ai_key_encryption_key"`
}

// Validate validates encryption configuration (required for server mode, skipped for workers).
func (ec *EncryptionConfig) Validate() error {
	if os.Getenv("APP_MODE") == "worker" {
		return nil
	}

	if ec.AIKeyEncryptionKey == "" {
		return errors.New("AI_KEY_ENCRYPTION_KEY is required for credential management. Generate with: openssl rand -base64 32")
	}

	key, err := base64.StdEncoding.DecodeString(ec.AIKeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("AI_KEY_ENCRYPTION_KEY must be valid base64: %w", err)
	}

	if len(key) != 32 {
		return fmt.Errorf("AI_KEY_ENCRYPTION_KEY must be exactly 32 bytes (got %d). Generate with: openssl rand -base64 32", len(key))
	}

	return nil
}

// AppConfig contains application-level configuration.
type AppConfig struct {
	Version string `mapstructure:"version"`
	Name    string `mapstructure:"name"`
}

// ServerConfig contains HTTP and WebSocket server configuration.
type ServerConfig struct {
	Environment        string        `mapstructure:"environment"`
	Host               string        `mapstructure:"host"`
	AppURL             string        `mapstructure:"app_url"` // Base URL for the frontend app (e.g., invitation links)
	CORSAllowedOrigins []string      `mapstructure:"cors_allowed_origins"`
	TrustedProxies     []string      `mapstructure:"trusted_proxies"`
	CORSAllowedHeaders []string      `mapstructure:"cors_allowed_headers"`
	CORSAllowedMethods []string      `mapstructure:"cors_allowed_methods"`
	CookieDomain       string        `mapstructure:"cookie_domain"`
	ReadTimeout        time.Duration `mapstructure:"read_timeout"`
	MaxRequestSize     int64         `mapstructure:"max_request_size"`
	ShutdownTimeout    time.Duration `mapstructure:"shutdown_timeout"`
	IdleTimeout        time.Duration `mapstructure:"idle_timeout"`
	WriteTimeout       time.Duration `mapstructure:"write_timeout"`
	Port               int           `mapstructure:"port"`
	EnableCORS         bool          `mapstructure:"enable_cors"`
}

// GRPCConfig contains gRPC server configuration for OTLP ingestion
type GRPCConfig struct {
	Port int `mapstructure:"port"`
}

// DatabaseConfig contains PostgreSQL database configuration.
type DatabaseConfig struct {
	SSLMode         string        `mapstructure:"ssl_mode"`
	Host            string        `mapstructure:"host"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	URL             string        `mapstructure:"url"`
	MigrationsPath  string        `mapstructure:"migrations_path"`
	Port            int           `mapstructure:"port"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
}

// ClickHouseConfig contains ClickHouse database configuration.
type ClickHouseConfig struct {
	MigrationsPath   string        `mapstructure:"migrations_path"`
	Host             string        `mapstructure:"host"`
	MigrationsEngine string        `mapstructure:"migrations_engine"`
	User             string        `mapstructure:"user"`
	Password         string        `mapstructure:"password"`
	Database         string        `mapstructure:"database"`
	URL              string        `mapstructure:"url"`
	MigrationsTable  string        `mapstructure:"migrations_table"`
	MaxOpenConns     int           `mapstructure:"max_open_conns"`
	ReadTimeout      time.Duration `mapstructure:"read_timeout"`
	WriteTimeout     time.Duration `mapstructure:"write_timeout"`
	ConnMaxLifetime  time.Duration `mapstructure:"conn_max_lifetime"`
	MaxIdleConns     int           `mapstructure:"max_idle_conns"`
	Port             int           `mapstructure:"port"`
}

// RedisConfig contains Redis configuration.
type RedisConfig struct {
	URL          string        `mapstructure:"url"`
	Host         string        `mapstructure:"host"`
	Password     string        `mapstructure:"password"`
	Port         int           `mapstructure:"port"`
	Database     int           `mapstructure:"database"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	MaxRetries   int           `mapstructure:"max_retries"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	Level      string `mapstructure:"level"`    // debug, info, warn, error
	Format     string `mapstructure:"format"`   // json, text
	Output     string `mapstructure:"output"`   // stdout, stderr, file
	File       string `mapstructure:"file"`     // file path if output=file
	MaxSize    int    `mapstructure:"max_size"` // megabytes
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAge     int    `mapstructure:"max_age"` // days
}

// ExternalConfig contains external service configurations.
// Note: AI API keys (OpenAI, Anthropic, etc.) are configured per-project via database,
// not via environment variables. See: Settings > AI Providers in the dashboard.
type ExternalConfig struct {
	Email      EmailConfig   `mapstructure:"email"`
	Stripe     StripeConfig  `mapstructure:"stripe"`
	LLMTimeout time.Duration `mapstructure:"llm_timeout"` // Default timeout for LLM API calls
}

// StripeConfig contains Stripe API configuration.
type StripeConfig struct {
	SecretKey      string `mapstructure:"secret_key"`
	PublishableKey string `mapstructure:"publishable_key"`
	WebhookSecret  string `mapstructure:"webhook_secret"`
	Environment    string `mapstructure:"environment"` // test, live
}

// EmailConfig contains email service configuration.
// Supported providers: resend (default), smtp, ses, sendgrid
type EmailConfig struct {
	Provider string `mapstructure:"provider"` // resend, smtp, ses, sendgrid

	// Common fields (required for all providers)
	FromEmail  string `mapstructure:"from_email"`
	FromName   string `mapstructure:"from_name"`
	ReplyEmail string `mapstructure:"reply_email"`

	// Resend provider
	ResendAPIKey string `mapstructure:"resend_api_key"`

	// SMTP provider
	SMTPHost     string `mapstructure:"smtp_host"`
	SMTPPort     int    `mapstructure:"smtp_port"`
	SMTPUsername string `mapstructure:"smtp_username"`
	SMTPPassword string `mapstructure:"smtp_password"`
	SMTPUseTLS   bool   `mapstructure:"smtp_use_tls"`

	// AWS SES provider (uses default credential chain if keys not provided)
	SESRegion    string `mapstructure:"ses_region"`
	SESAccessKey string `mapstructure:"ses_access_key"` // Optional - uses AWS default chain
	SESSecretKey string `mapstructure:"ses_secret_key"` // Optional - uses AWS default chain

	// SendGrid provider
	SendGridAPIKey string `mapstructure:"sendgrid_api_key"`
}

// FeatureConfig contains feature flag configuration.
type FeatureConfig struct {
	RealTimeMetrics bool `mapstructure:"real_time_metrics"`
	CustomModels    bool `mapstructure:"custom_models"`
	MultiModal      bool `mapstructure:"multi_modal"`
	BackgroundJobs  bool `mapstructure:"background_jobs"`
	RateLimiting    bool `mapstructure:"rate_limiting"`
	AuditLogging    bool `mapstructure:"audit_logging"`
}

// MonitoringConfig contains monitoring and observability configuration.
type MonitoringConfig struct {
	MetricsPath    string        `mapstructure:"metrics_path"`
	JaegerEndpoint string        `mapstructure:"jaeger_endpoint"`
	PrometheusPort int           `mapstructure:"prometheus_port"`
	SampleRate     float64       `mapstructure:"sample_rate"`
	FlushInterval  time.Duration `mapstructure:"flush_interval"`
	Enabled        bool          `mapstructure:"enabled"`
}

// ObservabilityConfig contains OTLP and telemetry configuration.
type ObservabilityConfig struct {
	PreserveRawOTLP bool `mapstructure:"preserve_raw_otlp" env:"OTLP_PRESERVE_RAW" envDefault:"true"`
}

// ArchiveConfig contains S3 raw telemetry archival configuration.
type ArchiveConfig struct {
	Enabled              bool   `mapstructure:"enabled"`
	PathPrefix           string `mapstructure:"path_prefix"`
	CompressionLevel     int    `mapstructure:"compression_level"`
	DefaultRetentionDays int    `mapstructure:"default_retention_days"`
}

// WorkersConfig contains background worker configuration.
type WorkersConfig struct {
	AnalyticsWorkers         int              `mapstructure:"analytics_workers"`
	NotificationWorkers      int              `mapstructure:"notification_workers"`
	UsageSyncIntervalMinutes int              `mapstructure:"usage_sync_interval_minutes"` // Billing usage sync interval (default: 5)
	AlertDeduplicationHours  int              `mapstructure:"alert_deduplication_hours"`   // Alert deduplication window (default: 24)
	EvaluatorWorker          EvaluatorWorkerConfig `mapstructure:"evaluator_worker"`
}

// EvaluatorWorkerConfig contains evaluator worker configuration.
type EvaluatorWorkerConfig struct {
	BatchSize          int    `mapstructure:"batch_size"`
	BlockDurationMs    int    `mapstructure:"block_duration_ms"`
	MaxRetries         int    `mapstructure:"max_retries"`
	RetryBackoffMs     int    `mapstructure:"retry_backoff_ms"`
	DiscoveryInterval  string `mapstructure:"discovery_interval"`
	MaxStreamsPerRead  int    `mapstructure:"max_streams_per_read"`
	EvaluatorCacheTTL  string `mapstructure:"evaluator_cache_ttl"`
}

// NotificationsConfig contains notification system configuration.
type NotificationsConfig struct {
	AlertWebhookURL            string `mapstructure:"alert_webhook_url"`
	WebsiteNotificationEmail   string `mapstructure:"website_notification_email"`
}

// BlobStorageConfig contains blob storage configuration for large payload offloading
type BlobStorageConfig struct {
	Provider        string `mapstructure:"provider"`          // "s3", "minio", "gcs", "azure"
	BucketName      string `mapstructure:"bucket_name"`       // "brokle"
	Region          string `mapstructure:"region"`            // "us-east-1"
	Endpoint        string `mapstructure:"endpoint"`          // For MinIO: "http://localhost:9000"
	AccessKeyID     string `mapstructure:"access_key_id"`     // AWS access key
	SecretAccessKey string `mapstructure:"secret_access_key"` // AWS secret
	UsePathStyle    bool   `mapstructure:"use_path_style"`    // true for MinIO
	Threshold       int    `mapstructure:"threshold"`         // 10000 (10KB)
}

// Validate validates the main configuration and all sub-configurations.
func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config validation failed: %w", err)
	}

	if err := c.Database.Validate(); err != nil {
		return fmt.Errorf("database config validation failed: %w", err)
	}

	if err := c.ClickHouse.Validate(); err != nil {
		return fmt.Errorf("clickhouse config validation failed: %w", err)
	}

	if err := c.Redis.Validate(); err != nil {
		return fmt.Errorf("redis config validation failed: %w", err)
	}

	if err := c.Auth.Validate(); err != nil {
		return fmt.Errorf("auth config validation failed: %w", err)
	}

	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("logging config validation failed: %w", err)
	}

	if err := c.External.Validate(); err != nil {
		return fmt.Errorf("external config validation failed: %w", err)
	}

	if err := c.Features.Validate(); err != nil {
		return fmt.Errorf("features config validation failed: %w", err)
	}

	if err := c.Monitoring.Validate(); err != nil {
		return fmt.Errorf("monitoring config validation failed: %w", err)
	}

	if err := c.Enterprise.Validate(); err != nil {
		return fmt.Errorf("enterprise config validation failed: %w", err)
	}

	if err := c.Encryption.Validate(); err != nil {
		return fmt.Errorf("encryption config validation failed: %w", err)
	}

	return nil
}

// Validate validates server configuration.
func (sc *ServerConfig) Validate() error {
	if sc.Port <= 0 || sc.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", sc.Port)
	}

	if sc.Host == "" {
		return errors.New("host cannot be empty")
	}

	if sc.ReadTimeout < 0 {
		return errors.New("read_timeout cannot be negative")
	}

	if sc.WriteTimeout < 0 {
		return errors.New("write_timeout cannot be negative")
	}

	if sc.MaxRequestSize <= 0 {
		return errors.New("max_request_size must be positive")
	}

	return nil
}

// Validate validates database configuration.
func (dc *DatabaseConfig) Validate() error {
	// If URL is provided, minimal validation
	if dc.URL != "" {
		// URL takes precedence, minimal validation
		if dc.MaxOpenConns < 0 {
			return errors.New("max_open_conns cannot be negative")
		}

		if dc.MaxIdleConns < 0 {
			return errors.New("max_idle_conns cannot be negative")
		}

		return nil
	}

	// If no URL, validate individual fields
	if dc.Host == "" {
		return errors.New("either url or host must be provided")
	}

	if dc.Port <= 0 || dc.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", dc.Port)
	}

	if dc.User == "" {
		return errors.New("user cannot be empty when using individual fields")
	}

	if dc.Database == "" {
		return errors.New("database name cannot be empty when using individual fields")
	}

	if dc.MaxOpenConns < 0 {
		return errors.New("max_open_conns cannot be negative")
	}

	if dc.MaxIdleConns < 0 {
		return errors.New("max_idle_conns cannot be negative")
	}

	return nil
}

// Validate validates ClickHouse configuration.
func (cc *ClickHouseConfig) Validate() error {
	// If URL is provided, minimal validation
	if cc.URL != "" {
		return nil // URL takes precedence
	}

	// If no URL, validate individual fields
	if cc.Host == "" {
		return errors.New("either url or host must be provided for clickhouse")
	}

	if cc.Port <= 0 || cc.Port > 65535 {
		return fmt.Errorf("invalid clickhouse port: %d (must be 1-65535)", cc.Port)
	}

	if cc.Database == "" {
		return errors.New("clickhouse database name cannot be empty when using individual fields")
	}

	return nil
}

// Validate validates Redis configuration.
func (rc *RedisConfig) Validate() error {
	// If URL is provided, minimal validation
	if rc.URL != "" {
		// URL takes precedence, minimal validation
		if rc.PoolSize < 0 {
			return errors.New("pool_size cannot be negative")
		}

		return nil
	}

	// If no URL, validate individual fields
	if rc.Host == "" {
		return errors.New("either url or host must be provided for redis")
	}

	if rc.Port <= 0 || rc.Port > 65535 {
		return fmt.Errorf("invalid redis port: %d (must be 1-65535)", rc.Port)
	}

	if rc.Database < 0 || rc.Database > 15 {
		return fmt.Errorf("invalid redis database number: %d (must be 0-15)", rc.Database)
	}

	if rc.PoolSize < 0 {
		return errors.New("pool_size cannot be negative")
	}

	return nil
}

// Validate validates logging configuration.
func (lc *LoggingConfig) Validate() error {
	validLevels := []string{"debug", "info", "warn", "error"}
	isValid := false
	for _, level := range validLevels {
		if lc.Level == level {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid log level: %s (must be one of %v)", lc.Level, validLevels)
	}

	validFormats := []string{"json", "text"}
	isValid = false
	for _, format := range validFormats {
		if lc.Format == format {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid log format: %s (must be one of %v)", lc.Format, validFormats)
	}

	validOutputs := []string{"stdout", "stderr", "file"}
	isValid = false
	for _, output := range validOutputs {
		if lc.Output == output {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid log output: %s (must be one of %v)", lc.Output, validOutputs)
	}

	if lc.Output == "file" && lc.File == "" {
		return errors.New("file path is required when output is 'file'")
	}

	return nil
}

// Validate validates external services configuration.
// Note: LLM API keys are configured per-project, not validated here.
func (ec *ExternalConfig) Validate() error {
	if err := ec.Stripe.Validate(); err != nil {
		return fmt.Errorf("stripe config: %w", err)
	}

	if err := ec.Email.Validate(); err != nil {
		return fmt.Errorf("email config: %w", err)
	}

	return nil
}

// Validate validates Stripe configuration.
func (sc *StripeConfig) Validate() error {
	if sc.SecretKey != "" {
		validEnvs := []string{"test", "live", ""}
		isValid := false
		for _, env := range validEnvs {
			if sc.Environment == env {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("invalid stripe environment: %s (must be 'test' or 'live')", sc.Environment)
		}
	}

	return nil
}

// Validate validates email configuration.
func (ec *EmailConfig) Validate() error {
	// Empty provider means email is disabled - valid configuration
	if ec.Provider == "" {
		return nil
	}

	// Validate provider is supported
	validProviders := []string{"resend", "smtp", "ses", "sendgrid"}
	isValid := false
	for _, provider := range validProviders {
		if ec.Provider == provider {
			isValid = true
			break
		}
	}
	if !isValid {
		return fmt.Errorf("invalid email provider: %s (must be one of %v)", ec.Provider, validProviders)
	}

	// FromEmail is required for all providers
	if ec.FromEmail == "" {
		return errors.New("EMAIL_FROM_ADDRESS is required when email provider is set")
	}

	// Provider-specific validation
	switch ec.Provider {
	case "resend":
		if ec.ResendAPIKey == "" {
			return errors.New("RESEND_API_KEY is required for Resend provider")
		}

	case "smtp":
		if ec.SMTPHost == "" {
			return errors.New("SMTP_HOST is required for SMTP provider")
		}
		if ec.SMTPPort <= 0 || ec.SMTPPort > 65535 {
			return fmt.Errorf("invalid SMTP_PORT: %d (must be 1-65535)", ec.SMTPPort)
		}

	case "ses":
		if ec.SESRegion == "" {
			return errors.New("SES_REGION is required for AWS SES provider")
		}
		// Note: SESAccessKey and SESSecretKey are optional - uses AWS default credential chain

	case "sendgrid":
		if ec.SendGridAPIKey == "" {
			return errors.New("SENDGRID_API_KEY is required for SendGrid provider")
		}
	}

	return nil
}

// Validate validates feature configuration.
func (fc *FeatureConfig) Validate() error {
	// No specific validations needed for feature flags currently
	return nil
}

// Validate validates monitoring configuration.
func (mc *MonitoringConfig) Validate() error {
	if mc.Enabled {
		if mc.PrometheusPort <= 0 || mc.PrometheusPort > 65535 {
			return fmt.Errorf("invalid prometheus_port: %d", mc.PrometheusPort)
		}

		if mc.MetricsPath == "" {
			return errors.New("metrics_path is required when monitoring is enabled")
		}

		if mc.SampleRate < 0 || mc.SampleRate > 1 {
			return fmt.Errorf("sample_rate must be between 0 and 1, got %f", mc.SampleRate)
		}
	}

	return nil
}

// Load loads configuration from files and environment variables.
func Load() (*Config, error) {
	// Load .env file if it exists (optional, for local development)
	// This sets environment variables that Viper can then read
	_ = godotenv.Load(".env")

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./configs")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/brokle")

	// Read config file (optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found - continue with defaults and env vars
	}

	// Set environment variable support (takes precedence over config files)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind standard infrastructure variables (no BROKLE_ prefix)
	//nolint:errcheck // BindEnv only errors with invalid args, safe with string literals
	//nolint:errcheck
	viper.BindEnv("database.url", "DATABASE_URL")
	//nolint:errcheck
	//nolint:errcheck
	viper.BindEnv("clickhouse.url", "CLICKHOUSE_URL")
	//nolint:errcheck
	//nolint:errcheck
	viper.BindEnv("redis.url", "REDIS_URL")
	//nolint:errcheck
	//nolint:errcheck
	viper.BindEnv("server.port", "PORT")
	//nolint:errcheck
	//nolint:errcheck
	viper.BindEnv("server.environment", "ENV")
	//nolint:errcheck
	viper.BindEnv("server.app_url", "APP_URL")
	//nolint:errcheck
	//nolint:errcheck
	viper.BindEnv("logging.level", "LOG_LEVEL")
	//nolint:errcheck
	viper.BindEnv("logging.format", "LOG_FORMAT")

	// CORS configuration (OSS-standard naming)
	//nolint:errcheck
	viper.BindEnv("server.cors_allowed_origins", "CORS_ALLOWED_ORIGINS")
	//nolint:errcheck
	viper.BindEnv("server.cors_allowed_methods", "CORS_ALLOWED_METHODS")
	//nolint:errcheck
	viper.BindEnv("server.cors_allowed_headers", "CORS_ALLOWED_HEADERS")
	//nolint:errcheck
	viper.BindEnv("server.cookie_domain", "COOKIE_DOMAIN")

	// External API keys (standard names)
	// Note: AI API keys (OpenAI, Anthropic, Cohere, etc.) are no longer loaded from env.
	// Playground uses project-scoped credentials stored in database.
	// Configure via dashboard: Settings > AI Providers
	//nolint:errcheck
	viper.BindEnv("external.stripe.secret_key", "STRIPE_SECRET_KEY")

	// Blob Storage configuration (for large payload offloading)
	//nolint:errcheck
	viper.BindEnv("blob_storage.provider", "BLOB_STORAGE_PROVIDER")
	//nolint:errcheck
	viper.BindEnv("blob_storage.bucket_name", "BLOB_STORAGE_BUCKET_NAME")
	//nolint:errcheck
	viper.BindEnv("blob_storage.region", "BLOB_STORAGE_REGION")
	//nolint:errcheck
	viper.BindEnv("blob_storage.endpoint", "BLOB_STORAGE_ENDPOINT")
	//nolint:errcheck
	viper.BindEnv("blob_storage.access_key_id", "BLOB_STORAGE_ACCESS_KEY_ID")
	//nolint:errcheck
	viper.BindEnv("blob_storage.secret_access_key", "BLOB_STORAGE_SECRET_ACCESS_KEY")
	//nolint:errcheck
	viper.BindEnv("blob_storage.use_path_style", "BLOB_STORAGE_USE_PATH_STYLE")
	//nolint:errcheck
	viper.BindEnv("blob_storage.threshold", "BLOB_STORAGE_THRESHOLD")

	// Archive configuration (S3 raw telemetry archival)
	//nolint:errcheck
	viper.BindEnv("archive.enabled", "ARCHIVE_ENABLED")
	//nolint:errcheck
	viper.BindEnv("archive.path_prefix", "ARCHIVE_PATH_PREFIX")
	//nolint:errcheck
	viper.BindEnv("archive.compression_level", "ARCHIVE_COMPRESSION_LEVEL")
	//nolint:errcheck
	viper.BindEnv("archive.default_retention_days", "ARCHIVE_DEFAULT_RETENTION_DAYS")

	//nolint:errcheck
	viper.BindEnv("external.stripe.publishable_key", "STRIPE_PUBLISHABLE_KEY")
	//nolint:errcheck
	viper.BindEnv("external.stripe.webhook_secret", "STRIPE_WEBHOOK_SECRET")

	// Email configuration (multi-provider: resend, smtp, ses, sendgrid)
	//nolint:errcheck
	viper.BindEnv("external.email.provider", "EMAIL_PROVIDER")
	//nolint:errcheck
	viper.BindEnv("external.email.from_email", "EMAIL_FROM_ADDRESS")
	//nolint:errcheck
	viper.BindEnv("external.email.from_name", "EMAIL_FROM_NAME")

	// Email - Resend provider
	//nolint:errcheck
	viper.BindEnv("external.email.resend_api_key", "RESEND_API_KEY")

	// Email - SMTP provider
	//nolint:errcheck
	viper.BindEnv("external.email.smtp_host", "SMTP_HOST")
	//nolint:errcheck
	viper.BindEnv("external.email.smtp_port", "SMTP_PORT")
	//nolint:errcheck
	viper.BindEnv("external.email.smtp_username", "SMTP_USERNAME")
	//nolint:errcheck
	viper.BindEnv("external.email.smtp_password", "SMTP_PASSWORD")
	//nolint:errcheck
	viper.BindEnv("external.email.smtp_use_tls", "SMTP_USE_TLS")

	// Email - AWS SES provider (uses default credential chain if keys not set)
	//nolint:errcheck
	viper.BindEnv("external.email.ses_region", "SES_REGION")
	//nolint:errcheck
	viper.BindEnv("external.email.ses_access_key", "SES_ACCESS_KEY")
	//nolint:errcheck
	viper.BindEnv("external.email.ses_secret_key", "SES_SECRET_KEY")

	// Email - SendGrid provider
	//nolint:errcheck
	viper.BindEnv("external.email.sendgrid_api_key", "SENDGRID_API_KEY")

	// Website notifications
	//nolint:errcheck
	viper.BindEnv("notifications.website_notification_email", "WEBSITE_NOTIFICATION_EMAIL")

	// Auth configuration (flexible JWT configuration - HS256 or RS256)
	//nolint:errcheck
	viper.BindEnv("auth.access_token_ttl", "ACCESS_TOKEN_TTL")
	//nolint:errcheck
	viper.BindEnv("auth.refresh_token_ttl", "REFRESH_TOKEN_TTL")
	//nolint:errcheck
	viper.BindEnv("auth.token_rotation_enabled", "TOKEN_ROTATION_ENABLED")
	//nolint:errcheck
	viper.BindEnv("auth.rate_limit_enabled", "RATE_LIMIT_ENABLED")
	//nolint:errcheck
	viper.BindEnv("auth.rate_limit_per_ip", "RATE_LIMIT_PER_IP")
	//nolint:errcheck
	viper.BindEnv("auth.rate_limit_per_user", "RATE_LIMIT_PER_USER")
	//nolint:errcheck
	viper.BindEnv("auth.rate_limit_window", "RATE_LIMIT_WINDOW")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_signing_method", "JWT_SIGNING_METHOD")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_issuer", "JWT_ISSUER")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_private_key_path", "JWT_PRIVATE_KEY_PATH")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_public_key_path", "JWT_PUBLIC_KEY_PATH")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_private_key_base64", "JWT_PRIVATE_KEY_BASE64")
	//nolint:errcheck
	viper.BindEnv("auth.jwt_public_key_base64", "JWT_PUBLIC_KEY_BASE64")

	// Encryption configuration (for sensitive data at rest)
	//nolint:errcheck
	viper.BindEnv("encryption.ai_key_encryption_key", "AI_KEY_ENCRYPTION_KEY")

	// OAuth configuration (Google/GitHub Signup)
	//nolint:errcheck
	viper.BindEnv("auth.google_client_id", "GOOGLE_CLIENT_ID")
	//nolint:errcheck
	viper.BindEnv("auth.google_client_secret", "GOOGLE_CLIENT_SECRET")
	//nolint:errcheck
	viper.BindEnv("auth.google_redirect_url", "GOOGLE_REDIRECT_URL")
	//nolint:errcheck
	viper.BindEnv("auth.github_client_id", "GITHUB_CLIENT_ID")
	//nolint:errcheck
	viper.BindEnv("auth.github_client_secret", "GITHUB_CLIENT_SECRET")
	//nolint:errcheck
	viper.BindEnv("auth.github_redirect_url", "GITHUB_REDIRECT_URL")

	// Database configuration (granular environment variables)
	//nolint:errcheck
	viper.BindEnv("database.host", "DB_HOST")
	//nolint:errcheck
	viper.BindEnv("database.port", "DB_PORT")
	//nolint:errcheck
	viper.BindEnv("database.user", "DB_USER")
	//nolint:errcheck
	viper.BindEnv("database.password", "DB_PASSWORD")
	//nolint:errcheck
	viper.BindEnv("database.database", "DB_NAME")
	//nolint:errcheck
	viper.BindEnv("database.ssl_mode", "DB_SSLMODE")

	// Database migration configuration
	//nolint:errcheck
	viper.BindEnv("database.auto_migrate", "DB_AUTO_MIGRATE")
	//nolint:errcheck
	viper.BindEnv("database.migrations_path", "DATABASE_MIGRATIONS_PATH")
	//nolint:errcheck
	viper.BindEnv("database.username", "DB_USERNAME")
	//nolint:errcheck
	viper.BindEnv("database.migrations_table", "DB_MIGRATIONS_TABLE")

	// ClickHouse migration configuration
	//nolint:errcheck
	viper.BindEnv("clickhouse.migrations_path", "CLICKHOUSE_MIGRATIONS_PATH")
	//nolint:errcheck
	viper.BindEnv("clickhouse.migrations_engine", "CLICKHOUSE_MIGRATIONS_ENGINE")

	// Keep BROKLE_ prefix for Brokle-specific variables
	//nolint:errcheck
	viper.BindEnv("enterprise.license.key", "BROKLE_ENTERPRISE_LICENSE_KEY")
	//nolint:errcheck
	viper.BindEnv("enterprise.license.type", "BROKLE_ENTERPRISE_LICENSE_TYPE")
	//nolint:errcheck
	viper.BindEnv("enterprise.license.offline_mode", "BROKLE_ENTERPRISE_LICENSE_OFFLINE_MODE")
	//nolint:errcheck
	viper.BindEnv("enterprise.sso.enabled", "BROKLE_ENTERPRISE_SSO_ENABLED")
	//nolint:errcheck
	viper.BindEnv("enterprise.sso.provider", "BROKLE_ENTERPRISE_SSO_PROVIDER")
	//nolint:errcheck
	viper.BindEnv("enterprise.rbac.enabled", "BROKLE_ENTERPRISE_RBAC_ENABLED")
	//nolint:errcheck
	viper.BindEnv("enterprise.compliance.enabled", "BROKLE_ENTERPRISE_COMPLIANCE_ENABLED")
	//nolint:errcheck
	viper.BindEnv("enterprise.analytics.enabled", "BROKLE_ENTERPRISE_ANALYTICS_ENABLED")

	// Set default values
	setDefaults()

	// Read config file (optional)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found - continue with defaults and env vars
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create and validate license wrapper
	licenseWrapper := NewLicenseWrapper(&cfg)
	if err := licenseWrapper.ValidateLicense(); err != nil {
		return nil, fmt.Errorf("license validation failed: %w", err)
	}

	return &cfg, nil
}

// GetLicenseWrapper returns a license wrapper for enhanced license management
func (c *Config) GetLicenseWrapper() *LicenseWrapper {
	return NewLicenseWrapper(c)
}

// setDefaults sets default configuration values.
func setDefaults() {
	// App defaults
	viper.SetDefault("app.name", "Brokle Platform")
	viper.SetDefault("app.version", "1.0.0")

	// Server defaults
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.environment", "development")
	viper.SetDefault("server.read_timeout", "30s")
	viper.SetDefault("server.write_timeout", "30s")
	viper.SetDefault("server.shutdown_timeout", "30s")
	viper.SetDefault("server.max_request_size", 32<<20) // 32MB
	viper.SetDefault("server.enable_cors", true)
	viper.SetDefault("server.app_url", "http://localhost:3000") // Frontend app URL for invitation links

	// CORS defaults (dev-friendly)
	viper.SetDefault("server.cors_allowed_origins", []string{"http://localhost:3000", "http://localhost:3001"})
	viper.SetDefault("server.cors_allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "PATCH"})
	viper.SetDefault("server.cors_allowed_headers", []string{
		"Content-Type",
		"Authorization",
		"X-API-Key",
		"X-Org-ID",
		"X-Project-ID",
		"X-Environment-ID",
	})
	viper.SetDefault("server.cookie_domain", "") // Empty = use request host (single domain). Set to ".example.com" for cross-subdomain

	// gRPC OTLP defaults (industry standard port 4317)
	viper.SetDefault("grpc.port", 4317) // OTLP gRPC standard port

	// Database defaults (URL-first, individual fields as fallback)
	viper.SetDefault("database.url", "") // Preferred: Set via DATABASE_URL env var
	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "brokle")
	viper.SetDefault("database.database", "") // Empty default - will be populated from URL if present
	viper.SetDefault("database.ssl_mode", "disable")
	viper.SetDefault("database.max_open_conns", 100)
	viper.SetDefault("database.max_idle_conns", 10)
	viper.SetDefault("database.conn_max_lifetime", "1h")
	viper.SetDefault("database.conn_max_idle_time", "15m")

	// Database migration defaults
	viper.SetDefault("database.auto_migrate", false)
	viper.SetDefault("database.migrations_path", "migrations")
	viper.SetDefault("database.username", "")
	viper.SetDefault("database.migrations_table", "schema_migrations")

	// ClickHouse defaults (URL-first, individual fields as fallback)
	viper.SetDefault("clickhouse.url", "") // Preferred: Set via CLICKHOUSE_URL env var
	viper.SetDefault("clickhouse.host", "localhost")
	viper.SetDefault("clickhouse.port", 9000)
	viper.SetDefault("clickhouse.user", "default")
	viper.SetDefault("clickhouse.database", "default")
	viper.SetDefault("clickhouse.max_open_conns", 50)
	viper.SetDefault("clickhouse.max_idle_conns", 5)
	viper.SetDefault("clickhouse.conn_max_lifetime", "1h")
	viper.SetDefault("clickhouse.read_timeout", "30s")
	viper.SetDefault("clickhouse.write_timeout", "30s")
	viper.SetDefault("clickhouse.migrations_engine", "MergeTree")

	// Redis defaults (URL-first, individual fields as fallback)
	viper.SetDefault("redis.url", "") // Preferred: Set via REDIS_URL env var
	viper.SetDefault("redis.host", "localhost")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.database", 0)
	viper.SetDefault("redis.pool_size", 20)
	viper.SetDefault("redis.min_idle_conns", 5)
	viper.SetDefault("redis.idle_timeout", "5m")
	viper.SetDefault("redis.max_retries", 3)

	// Auth defaults (flexible JWT and rate limiting configuration)
	viper.SetDefault("auth.access_token_ttl", "15m")
	viper.SetDefault("auth.refresh_token_ttl", "168h") // 7 days
	viper.SetDefault("auth.token_rotation_enabled", true)
	viper.SetDefault("auth.rate_limit_enabled", false) // Disabled by default for development
	viper.SetDefault("auth.rate_limit_per_ip", 100)
	viper.SetDefault("auth.rate_limit_per_user", 1000)
	viper.SetDefault("auth.rate_limit_window", "1h")
	viper.SetDefault("auth.jwt_signing_method", "HS256") // HS256 for development ease
	viper.SetDefault("auth.jwt_issuer", "brokle")
	viper.SetDefault("auth.jwt_secret", "") // Must be set in environment for HS256
	viper.SetDefault("auth.jwt_private_key_path", "")
	viper.SetDefault("auth.jwt_public_key_path", "")
	viper.SetDefault("auth.jwt_private_key_base64", "")
	viper.SetDefault("auth.jwt_public_key_base64", "")

	// OAuth defaults (optional - only needed if OAuth signup is enabled)
	viper.SetDefault("auth.google_client_id", "")
	viper.SetDefault("auth.google_client_secret", "")
	viper.SetDefault("auth.google_redirect_url", "")
	viper.SetDefault("auth.github_client_id", "")
	viper.SetDefault("auth.github_client_secret", "")
	viper.SetDefault("auth.github_redirect_url", "")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	viper.SetDefault("logging.output", "stdout")

	// External service defaults
	// LLM timeout for API calls (per-project credentials are stored in database)
	viper.SetDefault("external.llm_timeout", 30*time.Second)

	// Email defaults (multi-provider: resend, smtp, ses, sendgrid)
	viper.SetDefault("external.email.provider", "")        // Empty = disabled; Options: resend, smtp, ses, sendgrid
	viper.SetDefault("external.email.from_email", "")      // Required: e.g., "noreply@yourdomain.com"
	viper.SetDefault("external.email.from_name", "Brokle") // Display name in emails

	// Resend defaults
	viper.SetDefault("external.email.resend_api_key", "")

	// SMTP defaults
	viper.SetDefault("external.email.smtp_host", "")
	viper.SetDefault("external.email.smtp_port", 587) // Default TLS port
	viper.SetDefault("external.email.smtp_username", "")
	viper.SetDefault("external.email.smtp_password", "")
	viper.SetDefault("external.email.smtp_use_tls", true)

	// AWS SES defaults
	viper.SetDefault("external.email.ses_region", "")
	viper.SetDefault("external.email.ses_access_key", "") // Optional - uses AWS default credential chain
	viper.SetDefault("external.email.ses_secret_key", "") // Optional - uses AWS default credential chain

	// SendGrid defaults
	viper.SetDefault("external.email.sendgrid_api_key", "")

	// Feature flags defaults
	viper.SetDefault("features.real_time_metrics", true)
	viper.SetDefault("features.background_jobs", true)
	viper.SetDefault("features.rate_limiting", true)
	viper.SetDefault("features.audit_logging", true)

	// Monitoring defaults
	viper.SetDefault("monitoring.enabled", true)
	viper.SetDefault("monitoring.prometheus_port", 9090)
	viper.SetDefault("monitoring.metrics_path", "/metrics")
	viper.SetDefault("monitoring.sample_rate", 0.1)

	// Enterprise defaults
	viper.SetDefault("enterprise.license.type", "free")
	viper.SetDefault("enterprise.license.max_requests", 10000) // Free tier: 10K requests
	viper.SetDefault("enterprise.license.max_users", 5)        // Free tier: 5 users
	viper.SetDefault("enterprise.license.max_projects", 2)     // Free tier: 2 projects
	viper.SetDefault("enterprise.license.offline_mode", false)

	// SSO defaults (disabled by default)
	viper.SetDefault("enterprise.sso.enabled", false)
	viper.SetDefault("enterprise.sso.provider", "")

	// RBAC defaults (disabled by default)
	viper.SetDefault("enterprise.rbac.enabled", false)

	// Compliance defaults (disabled by default)
	viper.SetDefault("enterprise.compliance.enabled", false)
	viper.SetDefault("enterprise.compliance.audit_retention", "168h") // Basic: 7 days
	viper.SetDefault("enterprise.compliance.data_retention", "720h")  // Basic: 30 days
	viper.SetDefault("enterprise.compliance.pii_anonymization", false)
	viper.SetDefault("enterprise.compliance.soc2_compliance", false)
	viper.SetDefault("enterprise.compliance.hipaa_compliance", false)
	viper.SetDefault("enterprise.compliance.gdpr_compliance", false)

	// Analytics defaults (basic enabled)
	viper.SetDefault("enterprise.analytics.enabled", true)
	viper.SetDefault("enterprise.analytics.predictive_insights", false)
	viper.SetDefault("enterprise.analytics.custom_dashboards", false)
	viper.SetDefault("enterprise.analytics.ml_models", false)

	// Support defaults
	viper.SetDefault("enterprise.support.level", "standard")
	viper.SetDefault("enterprise.support.sla", "99.9%")
	viper.SetDefault("enterprise.support.dedicated_manager", false)
	viper.SetDefault("enterprise.support.on_call_support", false)

	// Blob Storage defaults (S3/MinIO for large payload offloading)
	viper.SetDefault("blob_storage.provider", "minio")
	viper.SetDefault("blob_storage.bucket_name", "brokle")
	viper.SetDefault("blob_storage.region", "us-east-1")
	viper.SetDefault("blob_storage.endpoint", "http://localhost:9100")
	viper.SetDefault("blob_storage.use_path_style", true)
	viper.SetDefault("blob_storage.threshold", 10_000)

	viper.SetDefault("archive.enabled", false)
	viper.SetDefault("archive.path_prefix", "telemetry/")
	viper.SetDefault("archive.compression_level", 3)
	viper.SetDefault("archive.default_retention_days", 2555)

	// Encryption defaults (must be set in production via AI_KEY_ENCRYPTION_KEY env var)
	viper.SetDefault("encryption.ai_key_encryption_key", "")

	// Evaluator worker defaults
	viper.SetDefault("workers.evaluator_worker.batch_size", 50)
	viper.SetDefault("workers.evaluator_worker.block_duration_ms", 1000)
	viper.SetDefault("workers.evaluator_worker.max_retries", 3)
	viper.SetDefault("workers.evaluator_worker.retry_backoff_ms", 500)
	viper.SetDefault("workers.evaluator_worker.discovery_interval", "30s")
	viper.SetDefault("workers.evaluator_worker.max_streams_per_read", 10)
	viper.SetDefault("workers.evaluator_worker.evaluator_cache_ttl", "30s")
}

// GetServerAddress returns the server address string.
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// GetDatabaseURL returns the PostgreSQL connection URL.
func (c *Config) GetDatabaseURL() string {
	// Priority 1: Use URL if provided
	if c.Database.URL != "" {
		return c.Database.URL
	}

	// Priority 2: Construct from individual fields
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.Database.User, c.Database.Password, c.Database.Host,
		c.Database.Port, c.Database.Database, c.Database.SSLMode)
}

// GetClickHouseURL returns the ClickHouse connection URL.
// The URL includes x-multi-statement=true to allow migrations with multiple SQL statements.
func (c *Config) GetClickHouseURL() string {
	if c.ClickHouse.URL != "" {
		// Append x-multi-statement if not already present (required for multi-statement migrations)
		if !strings.Contains(c.ClickHouse.URL, "x-multi-statement") {
			separator := "?"
			if strings.Contains(c.ClickHouse.URL, "?") {
				separator = "&"
			}
			return c.ClickHouse.URL + separator + "x-multi-statement=true"
		}
		return c.ClickHouse.URL
	}

	// Priority 2: Construct from individual fields with x-multi-statement=true
	return fmt.Sprintf("clickhouse://%s:%s@%s:%d/%s?x-multi-statement=true",
		c.ClickHouse.User, c.ClickHouse.Password, c.ClickHouse.Host,
		c.ClickHouse.Port, c.ClickHouse.Database)
}

// GetRedisURL returns the Redis connection URL.
func (c *Config) GetRedisURL() string {
	// Priority 1: Use URL if provided
	if c.Redis.URL != "" {
		return c.Redis.URL
	}

	// Priority 2: Construct from individual fields
	if c.Redis.Password != "" {
		return fmt.Sprintf("redis://:%s@%s:%d/%d",
			c.Redis.Password, c.Redis.Host, c.Redis.Port, c.Redis.Database)
	}
	return fmt.Sprintf("redis://%s:%d/%d",
		c.Redis.Host, c.Redis.Port, c.Redis.Database)
}

// IsDevelopment returns true if running in development environment.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development" || c.Environment == "dev"
}

// IsProduction returns true if running in production environment.
func (c *Config) IsProduction() bool {
	return c.Environment == "production" || c.Environment == "prod"
}

// IsEnterpriseFeatureEnabled checks if an enterprise feature is enabled
func (c *Config) IsEnterpriseFeatureEnabled(feature string) bool {
	if !c.IsEnterpriseLicense() {
		return false
	}

	for _, f := range c.Enterprise.License.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsEnterpriseLicense returns true if the license supports enterprise features
func (c *Config) IsEnterpriseLicense() bool {
	return c.Enterprise.License.Type == "pro" ||
		c.Enterprise.License.Type == "business" ||
		c.Enterprise.License.Type == "enterprise"
}

// GetLicenseTier returns the current license tier
func (c *Config) GetLicenseTier() string {
	if c.Enterprise.License.Type != "" {
		return c.Enterprise.License.Type
	}
	return "free" // Default to free tier
}

// CanUseFeature checks if a specific feature can be used based on license
func (c *Config) CanUseFeature(feature string) bool {
	// Allow all features in development mode
	if c.IsDevelopment() {
		return true
	}

	// Check if it's an enterprise feature
	enterpriseFeatures := []string{
		"advanced_rbac", "sso_integration", "custom_compliance",
		"predictive_insights", "custom_dashboards", "on_premise_deployment",
		"dedicated_support", "advanced_integrations", "cross_org_analytics",
	}

	for _, ef := range enterpriseFeatures {
		if ef == feature {
			return c.IsEnterpriseFeatureEnabled(feature)
		}
	}

	// Non-enterprise features are always available
	return true
}
