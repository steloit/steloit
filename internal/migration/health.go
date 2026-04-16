package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// HealthService provides migration health check capabilities
type HealthService struct {
	manager *Manager
	logger  *slog.Logger
}

// NewHealthService creates a new migration health service
func NewHealthService(manager *Manager, logger *slog.Logger) *HealthService {
	return &HealthService{
		manager: manager,
		logger:  logger,
	}
}

// HealthCheckResponse represents the structure of health check response
type HealthCheckResponse struct {
	Status    string                         `json:"status"`
	Timestamp time.Time                      `json:"timestamp"`
	Databases map[string]DatabaseHealthCheck `json:"databases"`
	Overall   OverallHealth                  `json:"overall"`
}

// DatabaseHealthCheck represents health status of a single database
type DatabaseHealthCheck struct {
	LastChecked    time.Time `json:"last_checked"`
	Status         string    `json:"status"`
	Error          string    `json:"error,omitempty"`
	ResponseTime   string    `json:"response_time"`
	CurrentVersion uint      `json:"current_version"`
	IsDirty        bool      `json:"is_dirty"`
}

// OverallHealth represents overall migration system health
type OverallHealth struct {
	LastFullHealthCheck time.Time `json:"last_full_health_check"`
	Status              string    `json:"status"`
	AverageResponseTime string    `json:"average_response_time"`
	DirtyDatabases      []string  `json:"dirty_databases,omitempty"`
	ErrorDatabases      []string  `json:"error_databases,omitempty"`
	Recommendations     []string  `json:"recommendations,omitempty"`
	HealthyDatabases    int       `json:"healthy_databases"`
	TotalDatabases      int       `json:"total_databases"`
}

// GetHealthStatus returns comprehensive health status of all migration systems
func (hs *HealthService) GetHealthStatus(ctx context.Context) (*HealthCheckResponse, error) {
	startTime := time.Now()
	hs.logger.Info("Starting migration health check")

	response := &HealthCheckResponse{
		Timestamp: startTime,
		Databases: make(map[string]DatabaseHealthCheck),
		Overall: OverallHealth{
			TotalDatabases:      2, // PostgreSQL + ClickHouse
			LastFullHealthCheck: startTime,
		},
	}

	var totalResponseTime time.Duration
	var healthyCount int
	var dirtyDatabases []string
	var errorDatabases []string
	var recommendations []string

	// Check PostgreSQL health
	pgHealth := hs.checkPostgreSQLHealth(ctx)
	response.Databases["postgres"] = pgHealth
	totalResponseTime += hs.parseResponseTime(pgHealth.ResponseTime)

	switch pgHealth.Status {
	case "healthy":
		healthyCount++
	case "dirty":
		dirtyDatabases = append(dirtyDatabases, "postgres")
		recommendations = append(recommendations, "PostgreSQL database has dirty migration state - consider running 'migrate force -version N' to resolve")
	case "error":
		errorDatabases = append(errorDatabases, "postgres")
		recommendations = append(recommendations, "PostgreSQL database has migration errors - check database connectivity and logs")
	}

	// Check ClickHouse health
	chHealth := hs.checkClickHouseHealth(ctx)
	response.Databases["clickhouse"] = chHealth
	totalResponseTime += hs.parseResponseTime(chHealth.ResponseTime)

	switch chHealth.Status {
	case "healthy":
		healthyCount++
	case "dirty":
		dirtyDatabases = append(dirtyDatabases, "clickhouse")
		recommendations = append(recommendations, "ClickHouse database has dirty migration state - consider running 'migrate force -version N' to resolve")
	case "error":
		errorDatabases = append(errorDatabases, "clickhouse")
		recommendations = append(recommendations, "ClickHouse database has migration errors - check database connectivity and logs")
	}

	// Determine overall health status
	response.Overall.HealthyDatabases = healthyCount
	response.Overall.DirtyDatabases = dirtyDatabases
	response.Overall.ErrorDatabases = errorDatabases
	response.Overall.Recommendations = recommendations
	response.Overall.AverageResponseTime = fmt.Sprintf("%.2fms", float64(totalResponseTime.Nanoseconds()/2)/1e6)

	if len(errorDatabases) > 0 {
		response.Overall.Status = "critical"
		response.Status = "unhealthy"
	} else if len(dirtyDatabases) > 0 {
		response.Overall.Status = "warning"
		response.Status = "degraded"
	} else if healthyCount == response.Overall.TotalDatabases {
		response.Overall.Status = "healthy"
		response.Status = "healthy"
	} else {
		response.Overall.Status = "unknown"
		response.Status = "unknown"
	}

	duration := time.Since(startTime)
	hs.logger.Info("Migration health check completed", "status", response.Status, "duration", duration, "healthy_dbs", healthyCount, "total_dbs", response.Overall.TotalDatabases)

	return response, nil
}

// checkPostgreSQLHealth performs health check on PostgreSQL migrations
func (hs *HealthService) checkPostgreSQLHealth(ctx context.Context) DatabaseHealthCheck {
	startTime := time.Now()

	version, dirty, err := hs.manager.postgresRunner.Version()
	duration := time.Since(startTime)

	health := DatabaseHealthCheck{
		LastChecked:  startTime,
		ResponseTime: fmt.Sprintf("%.2fms", float64(duration.Nanoseconds())/1e6),
	}

	if err != nil {
		health.Status = "error"
		health.Error = err.Error()
		hs.logger.Error("PostgreSQL migration health check failed", "error", err)
		return health
	}

	health.CurrentVersion = version
	health.IsDirty = dirty

	if dirty {
		health.Status = "dirty"
		hs.logger.Warn("PostgreSQL migrations are in dirty state", "version", version)
	} else {
		health.Status = "healthy"
		hs.logger.Debug("PostgreSQL migrations are healthy", "version", version)
	}

	return health
}

// checkClickHouseHealth performs health check on ClickHouse migrations
func (hs *HealthService) checkClickHouseHealth(ctx context.Context) DatabaseHealthCheck {
	startTime := time.Now()

	version, dirty, err := hs.manager.clickhouseRunner.Version()
	duration := time.Since(startTime)

	health := DatabaseHealthCheck{
		LastChecked:  startTime,
		ResponseTime: fmt.Sprintf("%.2fms", float64(duration.Nanoseconds())/1e6),
	}

	if err != nil {
		health.Status = "error"
		health.Error = err.Error()
		hs.logger.Error("ClickHouse migration health check failed", "error", err)
		return health
	}

	health.CurrentVersion = version
	health.IsDirty = dirty

	if dirty {
		health.Status = "dirty"
		hs.logger.Warn("ClickHouse migrations are in dirty state", "version", version)
	} else {
		health.Status = "healthy"
		hs.logger.Debug("ClickHouse migrations are healthy", "version", version)
	}

	return health
}

// parseResponseTime parses response time string back to duration for calculations
func (hs *HealthService) parseResponseTime(responseTime string) time.Duration {
	// Simple parsing for "XX.XXms" format
	var ms float64
	fmt.Sscanf(responseTime, "%fms", &ms)
	return time.Duration(ms * float64(time.Millisecond))
}

// HTTPHealthHandler provides HTTP endpoint for migration health checks
func (hs *HealthService) HTTPHealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		health, err := hs.GetHealthStatus(ctx)
		if err != nil {
			hs.logger.Error("Failed to get migration health status", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// Set HTTP status code based on health status
		switch health.Status {
		case "healthy":
			w.WriteHeader(http.StatusOK)
		case "degraded":
			w.WriteHeader(http.StatusOK) // Still operational but degraded
		case "unhealthy":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		if err := json.NewEncoder(w).Encode(health); err != nil {
			hs.logger.Error("Failed to encode health response", "error", err)
		}
	}
}

// GetSimpleHealthStatus returns a simple health status for basic monitoring
func (hs *HealthService) GetSimpleHealthStatus(ctx context.Context) map[string]interface{} {
	health := make(map[string]interface{})

	// Check PostgreSQL
	pgVersion, pgDirty, pgErr := hs.manager.postgresRunner.Version()
	health["postgres"] = map[string]interface{}{
		"status":          hs.getSimpleHealthStatus(pgErr, pgDirty),
		"current_version": pgVersion,
		"dirty":           pgDirty,
	}
	if pgErr != nil {
		health["postgres"].(map[string]interface{})["error"] = pgErr.Error()
	}

	// Check ClickHouse
	chVersion, chDirty, chErr := hs.manager.clickhouseRunner.Version()
	health["clickhouse"] = map[string]interface{}{
		"status":          hs.getSimpleHealthStatus(chErr, chDirty),
		"current_version": chVersion,
		"dirty":           chDirty,
	}
	if chErr != nil {
		health["clickhouse"].(map[string]interface{})["error"] = chErr.Error()
	}

	// Overall status
	overallHealthy := pgErr == nil && chErr == nil && !pgDirty && !chDirty
	if overallHealthy {
		health["overall_status"] = "healthy"
	} else if pgErr != nil || chErr != nil {
		health["overall_status"] = "critical"
	} else {
		health["overall_status"] = "degraded"
	}

	health["timestamp"] = time.Now()

	return health
}

// getSimpleHealthStatus converts error and dirty state to simple health status
func (hs *HealthService) getSimpleHealthStatus(err error, dirty bool) string {
	if err != nil {
		return "error"
	}
	if dirty {
		return "dirty"
	}
	return "healthy"
}

// CheckDrift detects schema drift between expected and actual database state
func (hs *HealthService) CheckDrift(ctx context.Context) (*DriftReport, error) {
	hs.logger.Info("Starting schema drift detection")

	report := &DriftReport{
		Timestamp: time.Now(),
		Databases: make(map[string]DatabaseDrift),
	}

	// Check PostgreSQL drift
	pgDrift, err := hs.checkPostgresDrift(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check PostgreSQL drift: %w", err)
	}
	report.Databases["postgres"] = pgDrift

	// Check ClickHouse drift
	chDrift, err := hs.checkClickHouseDrift(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check ClickHouse drift: %w", err)
	}
	report.Databases["clickhouse"] = chDrift

	// Determine overall drift status
	report.HasDrift = pgDrift.HasDrift || chDrift.HasDrift

	if report.HasDrift {
		hs.logger.Warn("Schema drift detected in migration system")
	} else {
		hs.logger.Info("No schema drift detected")
	}

	return report, nil
}

// DriftReport represents schema drift detection results
type DriftReport struct {
	Timestamp time.Time                `json:"timestamp"`
	Databases map[string]DatabaseDrift `json:"databases"`
	HasDrift  bool                     `json:"has_drift"`
}

// DatabaseDrift represents drift status for a single database
type DatabaseDrift struct {
	DriftDetails      string   `json:"drift_details,omitempty"`
	MissingMigrations []string `json:"missing_migrations,omitempty"`
	ExtraMigrations   []string `json:"extra_migrations,omitempty"`
	ExpectedVersion   uint     `json:"expected_version"`
	ActualVersion     uint     `json:"actual_version"`
	HasDrift          bool     `json:"has_drift"`
}

// checkPostgresDrift checks for PostgreSQL schema drift
func (hs *HealthService) checkPostgresDrift(ctx context.Context) (DatabaseDrift, error) {
	drift := DatabaseDrift{}

	// Get current migration version
	version, dirty, err := hs.manager.postgresRunner.Version()
	if err != nil {
		return drift, fmt.Errorf("failed to get PostgreSQL version: %w", err)
	}

	drift.ActualVersion = version

	// For now, we assume no drift if the database is not dirty
	// In a more sophisticated implementation, we would:
	// 1. Read expected migrations from migration files
	// 2. Compare with actual applied migrations in the database
	// 3. Detect missing or extra migrations

	if dirty {
		drift.HasDrift = true
		drift.DriftDetails = "Database is in dirty state - incomplete migration detected"
	}

	return drift, nil
}

// checkClickHouseDrift checks for ClickHouse schema drift
func (hs *HealthService) checkClickHouseDrift(ctx context.Context) (DatabaseDrift, error) {
	drift := DatabaseDrift{}

	// Get current migration version
	version, dirty, err := hs.manager.clickhouseRunner.Version()
	if err != nil {
		return drift, fmt.Errorf("failed to get ClickHouse version: %w", err)
	}

	drift.ActualVersion = version

	// Similar to PostgreSQL, check for dirty state
	if dirty {
		drift.HasDrift = true
		drift.DriftDetails = "Database is in dirty state - incomplete migration detected"
	}

	return drift, nil
}

// StartPeriodicHealthCheck starts a background goroutine for periodic health checks
func (hs *HealthService) StartPeriodicHealthCheck(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	hs.logger.Info("Starting periodic migration health checks", "interval", interval)

	for {
		select {
		case <-ctx.Done():
			hs.logger.Info("Stopping periodic migration health checks")
			return
		case <-ticker.C:
			health, err := hs.GetHealthStatus(ctx)
			if err != nil {
				hs.logger.Error("Periodic health check failed", "error", err)
				continue
			}

			// Log health status periodically
			hs.logger.Info("Periodic migration health check completed", "status", health.Status, "healthy_dbs", health.Overall.HealthyDatabases, "total_dbs", health.Overall.TotalDatabases, "avg_response_time", health.Overall.AverageResponseTime)

			// Alert on degraded health
			if health.Status != "healthy" {
				hs.logger.Warn("Migration system health is degraded", "status", health.Status, "dirty_databases", health.Overall.DirtyDatabases, "error_databases", health.Overall.ErrorDatabases, "recommendations", health.Overall.Recommendations)
			}
		}
	}
}
