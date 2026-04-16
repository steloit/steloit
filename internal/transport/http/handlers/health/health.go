package health

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"brokle/internal/config"
	"brokle/internal/version"
)

// Handler handles health check endpoints
type Handler struct {
	config    *config.Config
	logger    *slog.Logger
	startTime time.Time
}

// NewHandler creates a new health handler
func NewHandler(config *config.Config, logger *slog.Logger) *Handler {
	return &Handler{
		config:    config,
		logger:    logger,
		startTime: time.Now(),
	}
}

// HealthResponse represents the health check response
// @Description Health check response containing service status
type HealthResponse struct {
	Checks    map[string]HealthCheck `json:"checks,omitempty" description:"Individual component health checks"`
	Status    string                 `json:"status" example:"healthy" description:"Overall service status (healthy, unhealthy, alive)"`
	Timestamp string                 `json:"timestamp" example:"2023-12-01T10:30:00Z" description:"Health check timestamp in ISO 8601 format"`
	Version   string                 `json:"version,omitempty" example:"1.0.0" description:"Application version"`
	Uptime    string                 `json:"uptime" example:"2h30m15s" description:"Service uptime duration"`
}

// HealthCheck represents an individual health check
// @Description Individual component health check result
type HealthCheck struct {
	Status      string `json:"status" example:"healthy" description:"Component status (healthy, unhealthy)"`
	Message     string `json:"message,omitempty" example:"Database connection is healthy" description:"Status message"`
	LastChecked string `json:"last_checked" example:"2023-12-01T10:30:00Z" description:"Last check timestamp"`
	Duration    string `json:"duration,omitempty" example:"5ms" description:"Check execution duration"`
}

// Check handles basic health check
// @Summary Basic health check
// @Description Get basic health status of the service
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponse "Service is healthy"
// @Router /health [get]
func (h *Handler) Check(c *gin.Context) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version.Get(),
		Uptime:    time.Since(h.startTime).String(),
	}

	// Direct c.JSON: infrastructure endpoint consumed by K8s probes and monitoring
	// tools that expect a flat HealthResponse — not wrapped in APIResponse envelope.
	c.JSON(http.StatusOK, response)
}

// Ready handles readiness check with dependencies
// @Summary Readiness check
// @Description Check if service and all dependencies are ready to handle requests
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponse "Service and all dependencies are ready"
// @Failure 503 {object} HealthResponse "Service or dependencies are not ready"
// @Router /health/ready [get]
func (h *Handler) Ready(c *gin.Context) {
	checks := make(map[string]HealthCheck)
	overallStatus := "healthy"
	statusCode := http.StatusOK

	// Check database connectivity
	dbCheck := h.checkDatabase()
	checks["database"] = dbCheck
	if dbCheck.Status != "healthy" {
		overallStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	// Check Redis connectivity
	redisCheck := h.checkRedis()
	checks["redis"] = redisCheck
	if redisCheck.Status != "healthy" {
		overallStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	// Check ClickHouse connectivity
	clickhouseCheck := h.checkClickHouse()
	checks["clickhouse"] = clickhouseCheck
	if clickhouseCheck.Status != "healthy" {
		overallStatus = "unhealthy"
		statusCode = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Status:    overallStatus,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version.Get(),
		Uptime:    time.Since(h.startTime).String(),
		Checks:    checks,
	}

	// Direct c.JSON: infrastructure endpoint with dynamic status code (200/503).
	// SuccessWithStatus would produce {success: true} on 503, which is contradictory.
	c.JSON(statusCode, response)
}

// Live handles liveness check
// @Summary Liveness check
// @Description Check if service is alive and responsive
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponse "Service is alive and responsive"
// @Router /health/live [get]
func (h *Handler) Live(c *gin.Context) {
	response := HealthResponse{
		Status:    "alive",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Uptime:    time.Since(h.startTime).String(),
	}

	// Direct c.JSON: infrastructure endpoint — see Check() comment.
	c.JSON(http.StatusOK, response)
}

// checkDatabase checks database connectivity
func (h *Handler) checkDatabase() HealthCheck {
	start := time.Now()

	// TODO: Implement actual database ping
	// For now, simulate a successful check

	return HealthCheck{
		Status:      "healthy",
		Message:     "Database connection is healthy",
		LastChecked: time.Now().UTC().Format(time.RFC3339),
		Duration:    time.Since(start).String(),
	}
}

// checkRedis checks Redis connectivity
func (h *Handler) checkRedis() HealthCheck {
	start := time.Now()

	// TODO: Implement actual Redis ping
	// For now, simulate a successful check

	return HealthCheck{
		Status:      "healthy",
		Message:     "Redis connection is healthy",
		LastChecked: time.Now().UTC().Format(time.RFC3339),
		Duration:    time.Since(start).String(),
	}
}

// checkClickHouse checks ClickHouse connectivity
func (h *Handler) checkClickHouse() HealthCheck {
	start := time.Now()

	// TODO: Implement actual ClickHouse ping
	// For now, simulate a successful check

	return HealthCheck{
		Status:      "healthy",
		Message:     "ClickHouse connection is healthy",
		LastChecked: time.Now().UTC().Format(time.RFC3339),
		Duration:    time.Since(start).String(),
	}
}
