//go:build enterprise
// +build enterprise

package license

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"brokle/internal/config"
)

// LicenseService handles license validation and management
type LicenseService struct {
	config     *config.Config
	logger     *slog.Logger
	redis      *redis.Client
	httpClient *http.Client
	publicKey  *rsa.PublicKey
}

// LicenseInfo represents license information
type LicenseInfo struct {
	ValidUntil    time.Time `json:"valid_until"`
	LastValidated time.Time `json:"last_validated"`
	Key           string    `json:"key"`
	Type          string    `json:"type"`
	Organization  string    `json:"organization,omitempty"`
	ContactEmail  string    `json:"contact_email,omitempty"`
	Features      []string  `json:"features"`
	MaxRequests   int64     `json:"max_requests"`
	MaxUsers      int       `json:"max_users"`
	MaxProjects   int       `json:"max_projects"`
	IsValid       bool      `json:"is_valid"`
}

// UsageInfo represents current usage statistics
type UsageInfo struct {
	LastUpdated time.Time `json:"last_updated"`
	Requests    int64     `json:"requests"`
	Users       int       `json:"users"`
	Projects    int       `json:"projects"`
}

// LicenseStatus represents the overall license status
type LicenseStatus struct {
	License *LicenseInfo `json:"license"`
	Usage   *UsageInfo   `json:"usage"`
	Errors  []string     `json:"errors,omitempty"`
	IsValid bool         `json:"is_valid"`
}

const (
	licenseValidationURL = "https://api.brokle.com/v1/licenses/validate"
	licenseCacheKey      = "brokle:license:info"
	licenseCacheTTL      = 1 * time.Hour
	offlineGracePeriod   = 24 * time.Hour
)

// NewLicenseService creates a new license service instance
func NewLicenseService(cfg *config.Config, logger *slog.Logger, redisClient *redis.Client) (*LicenseService, error) {
	service := &LicenseService{
		config: cfg,
		logger: logger,
		redis:  redisClient,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Parse public key for license signature validation
	if cfg.Enterprise.License.Key != "" {
		// In production, you would load the Brokle public key
		// For now, we'll skip signature validation in development
		if cfg.IsProduction() {
			// TODO: Load and parse RSA public key for signature validation
			// publicKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
			// if err != nil {
			//     return nil, fmt.Errorf("failed to parse license public key: %w", err)
			// }
			// service.publicKey = publicKey
		}
	}

	return service, nil
}

// ValidateLicense validates the current license and returns status
func (ls *LicenseService) ValidateLicense(ctx context.Context) (*LicenseStatus, error) {
	// Check if license is cached and still valid
	if cachedLicense, err := ls.getCachedLicense(ctx); err == nil && cachedLicense != nil {
		if time.Since(cachedLicense.LastValidated) < licenseCacheTTL {
			ls.logger.Debug("Using cached license information")
			usage, _ := ls.getCurrentUsage(ctx)
			return &LicenseStatus{
				License: cachedLicense,
				Usage:   usage,
				IsValid: cachedLicense.IsValid,
			}, nil
		}
	}

	// Perform fresh license validation
	licenseInfo, err := ls.performLicenseValidation(ctx)
	if err != nil {
		ls.logger.Warn("License validation failed", "error", err)

		// Check if we can use offline validation
		if cachedLicense, cacheErr := ls.getCachedLicense(ctx); cacheErr == nil && cachedLicense != nil {
			if time.Since(cachedLicense.LastValidated) < offlineGracePeriod {
				ls.logger.Info("Using cached license due to validation failure (within grace period)")
				usage, _ := ls.getCurrentUsage(ctx)
				return &LicenseStatus{
					License: cachedLicense,
					Usage:   usage,
					IsValid: cachedLicense.IsValid,
					Errors:  []string{fmt.Sprintf("Online validation failed: %v", err)},
				}, nil
			}
		}

		// Return free tier if validation fails
		return ls.getFreetierStatus(ctx), nil
	}

	// Cache the validated license
	if err := ls.cacheLicense(ctx, licenseInfo); err != nil {
		ls.logger.Warn("Failed to cache license information", "error", err)
	}

	usage, _ := ls.getCurrentUsage(ctx)
	return &LicenseStatus{
		License: licenseInfo,
		Usage:   usage,
		IsValid: licenseInfo.IsValid,
	}, nil
}

// CheckFeatureEntitlement checks if a specific feature is available
func (ls *LicenseService) CheckFeatureEntitlement(ctx context.Context, feature string) (bool, error) {
	status, err := ls.ValidateLicense(ctx)
	if err != nil {
		return false, err
	}

	if !status.IsValid {
		return false, nil
	}

	// Check if feature is included in license
	for _, f := range status.License.Features {
		if f == feature {
			return true, nil
		}
	}

	return false, nil
}

// CheckUsageLimit checks if current usage is within license limits
func (ls *LicenseService) CheckUsageLimit(ctx context.Context, limitType string) (bool, int64, error) {
	status, err := ls.ValidateLicense(ctx)
	if err != nil {
		return false, 0, err
	}

	if status.Usage == nil {
		return true, 0, nil // No usage data, allow
	}

	switch limitType {
	case "requests":
		limit := status.License.MaxRequests
		current := status.Usage.Requests
		return current < limit, limit - current, nil
	case "users":
		limit := int64(status.License.MaxUsers)
		current := int64(status.Usage.Users)
		return current < limit, limit - current, nil
	case "projects":
		limit := int64(status.License.MaxProjects)
		current := int64(status.Usage.Projects)
		return current < limit, limit - current, nil
	default:
		return true, 0, nil
	}
}

// UpdateUsage updates the current usage statistics
func (ls *LicenseService) UpdateUsage(ctx context.Context, usageType string, increment int64) error {
	key := "brokle:usage:" + usageType

	// Use Redis to track usage with expiration
	_, err := ls.redis.IncrBy(ctx, key, increment).Result()
	if err != nil {
		return fmt.Errorf("failed to update usage: %w", err)
	}

	// Set expiration to reset monthly
	ls.redis.Expire(ctx, key, 30*24*time.Hour)

	return nil
}

// performLicenseValidation performs actual license validation
func (ls *LicenseService) performLicenseValidation(ctx context.Context) (*LicenseInfo, error) {
	license := &ls.config.Enterprise.License

	// If no license key, return free tier
	if license.Key == "" {
		return ls.createFreeTierLicense(), nil
	}

	// For offline mode or development, validate locally
	if license.OfflineMode || ls.config.IsDevelopment() {
		return ls.validateLicenseLocally(license)
	}

	// Online validation with Brokle license server
	return ls.validateLicenseOnline(ctx, license)
}

// validateLicenseLocally validates license locally (offline mode)
func (ls *LicenseService) validateLicenseLocally(license *config.LicenseConfig) (*LicenseInfo, error) {
	if ls.publicKey != nil {
		// Parse and validate JWT license
		token, err := jwt.Parse(license.Key, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return ls.publicKey, nil
		})

		if err != nil {
			return nil, fmt.Errorf("invalid license signature: %w", err)
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			return ls.parseLicenseClaims(claims)
		}
	}

	// Fallback: use config values (for development)
	return &LicenseInfo{
		Key:           license.Key,
		Type:          license.Type,
		ValidUntil:    license.ValidUntil,
		MaxRequests:   int64(license.MaxRequests),
		MaxUsers:      license.MaxUsers,
		MaxProjects:   license.MaxProjects,
		Features:      license.Features,
		IsValid:       true,
		LastValidated: time.Now(),
	}, nil
}

// validateLicenseOnline validates license with Brokle license server
func (ls *LicenseService) validateLicenseOnline(ctx context.Context, license *config.LicenseConfig) (*LicenseInfo, error) {
	// Prepare validation request
	req := map[string]interface{}{
		"license_key": license.Key,
		"platform":    "brokle-platform",
		"version":     ls.config.App.Version,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", licenseValidationURL,
		strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create validation request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := ls.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("license validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("license validation failed with status: %d", resp.StatusCode)
	}

	var result struct {
		License *LicenseInfo `json:"license"`
		Error   string       `json:"error,omitempty"`
		Valid   bool         `json:"valid"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode validation response: %w", err)
	}

	if !result.Valid {
		return nil, fmt.Errorf("license validation failed: %s", result.Error)
	}

	result.License.LastValidated = time.Now()
	return result.License, nil
}

// Helper methods

func (ls *LicenseService) createFreeTierLicense() *LicenseInfo {
	return &LicenseInfo{
		Key:           "",
		Type:          "free",
		ValidUntil:    time.Now().AddDate(1, 0, 0), // Valid for 1 year
		MaxRequests:   10000,                       // 10K requests
		MaxUsers:      5,                           // 5 users
		MaxProjects:   2,                           // 2 projects
		Features:      []string{},                  // No enterprise features
		IsValid:       true,
		LastValidated: time.Now(),
	}
}

func (ls *LicenseService) parseLicenseClaims(claims jwt.MapClaims) (*LicenseInfo, error) {
	info := &LicenseInfo{
		LastValidated: time.Now(),
		IsValid:       true,
	}

	if typ, ok := claims["type"].(string); ok {
		info.Type = typ
	}

	if validUntil, ok := claims["valid_until"].(float64); ok {
		info.ValidUntil = time.Unix(int64(validUntil), 0)
		if time.Now().After(info.ValidUntil) {
			info.IsValid = false
		}
	}

	if maxReq, ok := claims["max_requests"].(float64); ok {
		info.MaxRequests = int64(maxReq)
	}

	if features, ok := claims["features"].([]interface{}); ok {
		for _, f := range features {
			if feature, ok := f.(string); ok {
				info.Features = append(info.Features, feature)
			}
		}
	}

	return info, nil
}

func (ls *LicenseService) getCachedLicense(ctx context.Context) (*LicenseInfo, error) {
	data, err := ls.redis.Get(ctx, licenseCacheKey).Result()
	if err != nil {
		return nil, err
	}

	var info LicenseInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func (ls *LicenseService) cacheLicense(ctx context.Context, info *LicenseInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}

	return ls.redis.Set(ctx, licenseCacheKey, data, licenseCacheTTL).Err()
}

func (ls *LicenseService) getCurrentUsage(ctx context.Context) (*UsageInfo, error) {
	usage := &UsageInfo{
		LastUpdated: time.Now(),
	}

	// Get current usage from Redis
	if requests, err := ls.redis.Get(ctx, "brokle:usage:requests").Int64(); err == nil {
		usage.Requests = requests
	}

	if users, err := ls.redis.Get(ctx, "brokle:usage:users").Int(); err == nil {
		usage.Users = users
	}

	if projects, err := ls.redis.Get(ctx, "brokle:usage:projects").Int(); err == nil {
		usage.Projects = projects
	}

	return usage, nil
}

func (ls *LicenseService) getFreetierStatus(ctx context.Context) *LicenseStatus {
	license := ls.createFreeTierLicense()
	usage, _ := ls.getCurrentUsage(ctx)

	return &LicenseStatus{
		License: license,
		Usage:   usage,
		IsValid: true,
		Errors:  []string{"Using free tier due to license validation failure"},
	}
}
