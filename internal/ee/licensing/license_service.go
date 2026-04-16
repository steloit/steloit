//go:build !enterprise
// +build !enterprise

package license

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"brokle/internal/config"
)

// LicenseService OSS stub - all enterprise features disabled
type LicenseService struct {
	config *config.Config
	logger *slog.Logger
	redis  *redis.Client
}

// LicenseInfo represents license information
type LicenseInfo struct {
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	ValidUntil    time.Time `json:"valid_until"`
	MaxRequests   int64     `json:"max_requests"`
	MaxUsers      int       `json:"max_users"`
	MaxProjects   int       `json:"max_projects"`
	Features      []string  `json:"features"`
	Organization  string    `json:"organization,omitempty"`
	ContactEmail  string    `json:"contact_email,omitempty"`
	IsValid       bool      `json:"is_valid"`
	LastValidated time.Time `json:"last_validated"`
}

// UsageInfo represents current usage statistics
type UsageInfo struct {
	Requests    int64     `json:"requests"`
	Users       int       `json:"users"`
	Projects    int       `json:"projects"`
	LastUpdated time.Time `json:"last_updated"`
}

// LicenseStatus represents the combined license and usage status
type LicenseStatus struct {
	License *LicenseInfo `json:"license"`
	Usage   *UsageInfo   `json:"usage"`
	IsValid bool         `json:"is_valid"`
	Errors  []string     `json:"errors,omitempty"`
}

// NewLicenseService creates OSS license service stub (matches enterprise signature)
func NewLicenseService(cfg *config.Config, logger *slog.Logger, redisClient *redis.Client) (*LicenseService, error) {
	logger.Info("Brokle OSS - Enterprise license features disabled")
	return &LicenseService{
		config: cfg,
		logger: logger,
		redis:  redisClient,
	}, nil
}

// ValidateLicense - OSS always returns valid (no license required)
func (ls *LicenseService) ValidateLicense(ctx context.Context) (*LicenseStatus, error) {
	return &LicenseStatus{
		License: &LicenseInfo{
			Key:           "oss",
			Type:          "open-source",
			ValidUntil:    time.Now().AddDate(100, 0, 0), // Always valid
			MaxRequests:   -1,                            // Unlimited
			MaxUsers:      -1,                            // Unlimited
			MaxProjects:   -1,                            // Unlimited
			Features:      []string{},                    // No enterprise features
			IsValid:       true,
			LastValidated: time.Now(),
		},
		Usage: &UsageInfo{
			Requests:    0,
			Users:       0,
			Projects:    0,
			LastUpdated: time.Now(),
		},
		IsValid: true,
		Errors:  []string{},
	}, nil
}

// CheckFeatureEntitlement - OSS denies all enterprise features
func (ls *LicenseService) CheckFeatureEntitlement(ctx context.Context, feature string) (bool, error) {
	ls.logger.Debug("Enterprise feature not available in OSS build", "feature", feature)
	return false, fmt.Errorf("enterprise feature '%s' requires enterprise license", feature)
}

// CheckUsageLimit - OSS has no usage limits (unlimited)
func (ls *LicenseService) CheckUsageLimit(ctx context.Context, limitType string) (bool, int64, error) {
	return true, -1, nil // true = within limits, -1 = unlimited
}

// UpdateUsage - OSS no-op (no usage tracking in free tier)
func (ls *LicenseService) UpdateUsage(ctx context.Context, usageType string, increment int64) error {
	// No usage tracking in OSS build
	return nil
}
