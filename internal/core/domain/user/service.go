package user

import (
	"context"

	"github.com/google/uuid"
)

// UserService defines the interface for core user management operations.
type UserService interface {
	// User lifecycle management
	GetUser(ctx context.Context, userID uuid.UUID) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByEmailWithPassword(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, userID uuid.UUID, req *UpdateUserRequest) (*User, error)
	DeactivateUser(ctx context.Context, userID uuid.UUID) error
	ReactivateUser(ctx context.Context, userID uuid.UUID) error
	DeleteUser(ctx context.Context, userID uuid.UUID) error

	// User listing and search
	ListUsers(ctx context.Context, filters *ListFilters) ([]*User, int, error)
	SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, int, error)
	GetUsersByIDs(ctx context.Context, userIDs []uuid.UUID) ([]*User, error)
	GetPublicUsers(ctx context.Context, userIDs []uuid.UUID) ([]*PublicUser, error)

	// Email verification
	VerifyEmail(ctx context.Context, userID uuid.UUID, token string) error
	MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error
	SendVerificationEmail(ctx context.Context, userID uuid.UUID) error

	// Password management
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, token, newPassword string) error
	ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error

	// Activity tracking
	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error
	GetUserActivity(ctx context.Context, userID uuid.UUID) (*UserActivity, error)

	// Organization context
	SetDefaultOrganization(ctx context.Context, userID, orgID uuid.UUID) error
	GetDefaultOrganization(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error)
	ValidateUserOrgMembership(ctx context.Context, userID, orgID uuid.UUID) (bool, error)

	// User statistics
	GetUserStats(ctx context.Context) (*UserStats, error)
}

// ProfileService defines the interface for user profile management and preferences.
type ProfileService interface {
	// Profile management
	GetProfile(ctx context.Context, userID uuid.UUID) (*UserProfile, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req *UpdateProfileRequest) (*UserProfile, error)
	UploadAvatar(ctx context.Context, userID uuid.UUID, imageData []byte, contentType string) (*UserProfile, error)
	RemoveAvatar(ctx context.Context, userID uuid.UUID) error

	// Notification preferences (consolidated from preferences service)
	GetNotificationPreferences(ctx context.Context, userID uuid.UUID) (*NotificationPreferences, error)
	UpdateNotificationPreferences(ctx context.Context, userID uuid.UUID, req *UpdateNotificationPreferencesRequest) (*NotificationPreferences, error)

	// Theme and UI preferences (consolidated from preferences service)
	GetThemePreferences(ctx context.Context, userID uuid.UUID) (*ThemePreferences, error)
	UpdateThemePreferences(ctx context.Context, userID uuid.UUID, req *UpdateThemePreferencesRequest) (*ThemePreferences, error)

	// Profile visibility and privacy
	UpdateProfileVisibility(ctx context.Context, userID uuid.UUID, visibility ProfileVisibility) error
	GetPublicProfile(ctx context.Context, userID uuid.UUID) (*PublicProfile, error)
	GetPrivacyPreferences(ctx context.Context, userID uuid.UUID) (*PrivacyPreferences, error)
	UpdatePrivacyPreferences(ctx context.Context, userID uuid.UUID, req *UpdatePrivacyPreferencesRequest) (*PrivacyPreferences, error)

	// Profile completeness and validation
	GetProfileCompleteness(ctx context.Context, userID uuid.UUID) (*ProfileCompleteness, error)
	ValidateProfile(ctx context.Context, userID uuid.UUID) (*ProfileValidation, error)
}

// Supporting types for the new service interfaces

// ProfileVisibility represents profile visibility options
type ProfileVisibility string

const (
	ProfileVisibilityPublic  ProfileVisibility = "public"
	ProfileVisibilityPrivate ProfileVisibility = "private"
	ProfileVisibilityFriends ProfileVisibility = "friends"
	ProfileVisibilityTeam    ProfileVisibility = "team"
)

// PublicProfile represents a public view of a user profile
type PublicProfile struct {
	AvatarURL *string   `json:"avatar_url,omitempty"`
	Bio       *string   `json:"bio,omitempty"`
	Location  *string   `json:"location,omitempty"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	UserID    uuid.UUID `json:"user_id"`
}

// ProfileCompleteness represents profile completion status
type ProfileCompleteness struct {
	Sections        map[string]int `json:"sections"`
	CompletedFields []string       `json:"completed_fields"`
	MissingFields   []string       `json:"missing_fields"`
	Recommendations []string       `json:"recommendations"`
	OverallScore    int            `json:"overall_score"`
}

// ProfileValidation represents profile validation results
type ProfileValidation struct {
	Errors  []ProfileValidationError `json:"errors"`
	IsValid bool                     `json:"is_valid"`
}

type ProfileValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// NotificationPreferences represents user notification settings
type NotificationPreferences struct {
	EmailNotifications      bool `json:"email_notifications"`
	PushNotifications       bool `json:"push_notifications"`
	SMSNotifications        bool `json:"sms_notifications"`
	MarketingEmails         bool `json:"marketing_emails"`
	SecurityAlerts          bool `json:"security_alerts"`
	ProductUpdates          bool `json:"product_updates"`
	WeeklyDigest            bool `json:"weekly_digest"`
	InvitationNotifications bool `json:"invitation_notifications"`
}

type UpdateNotificationPreferencesRequest struct {
	EmailNotifications      *bool `json:"email_notifications,omitempty"`
	PushNotifications       *bool `json:"push_notifications,omitempty"`
	SMSNotifications        *bool `json:"sms_notifications,omitempty"`
	MarketingEmails         *bool `json:"marketing_emails,omitempty"`
	SecurityAlerts          *bool `json:"security_alerts,omitempty"`
	ProductUpdates          *bool `json:"product_updates,omitempty"`
	WeeklyDigest            *bool `json:"weekly_digest,omitempty"`
	InvitationNotifications *bool `json:"invitation_notifications,omitempty"`
}

// ThemePreferences represents user UI/theme preferences
type ThemePreferences struct {
	Theme          string `json:"theme"` // light, dark, auto
	PrimaryColor   string `json:"primary_color"`
	Language       string `json:"language"`
	TimeFormat     string `json:"time_format"` // 12h, 24h
	DateFormat     string `json:"date_format"`
	Timezone       string `json:"timezone"`
	CompactMode    bool   `json:"compact_mode"`
	ShowAnimations bool   `json:"show_animations"`
	HighContrast   bool   `json:"high_contrast"`
}

type UpdateThemePreferencesRequest struct {
	Theme          *string `json:"theme,omitempty"`
	PrimaryColor   *string `json:"primary_color,omitempty"`
	Language       *string `json:"language,omitempty"`
	TimeFormat     *string `json:"time_format,omitempty"`
	DateFormat     *string `json:"date_format,omitempty"`
	Timezone       *string `json:"timezone,omitempty"`
	CompactMode    *bool   `json:"compact_mode,omitempty"`
	ShowAnimations *bool   `json:"show_animations,omitempty"`
	HighContrast   *bool   `json:"high_contrast,omitempty"`
}

// PrivacyPreferences represents user privacy settings
type PrivacyPreferences struct {
	ProfileVisibility      ProfileVisibility `json:"profile_visibility"`
	ShowEmail              bool              `json:"show_email"`
	ShowLastSeen           bool              `json:"show_last_seen"`
	AllowDirectMessages    bool              `json:"allow_direct_messages"`
	DataProcessingConsent  bool              `json:"data_processing_consent"`
	AnalyticsConsent       bool              `json:"analytics_consent"`
	ThirdPartyIntegrations bool              `json:"third_party_integrations"`
}

type UpdatePrivacyPreferencesRequest struct {
	ProfileVisibility      *ProfileVisibility `json:"profile_visibility,omitempty"`
	ShowEmail              *bool              `json:"show_email,omitempty"`
	ShowLastSeen           *bool              `json:"show_last_seen,omitempty"`
	AllowDirectMessages    *bool              `json:"allow_direct_messages,omitempty"`
	DataProcessingConsent  *bool              `json:"data_processing_consent,omitempty"`
	AnalyticsConsent       *bool              `json:"analytics_consent,omitempty"`
	ThirdPartyIntegrations *bool              `json:"third_party_integrations,omitempty"`
}

// UserType represents different types of users
type UserType string

const (
	UserTypeDeveloper UserType = "developer"
	UserTypeManager   UserType = "manager"
	UserTypeAnalyst   UserType = "analyst"
	UserTypeAdmin     UserType = "admin"
)

// UserActivity represents user activity and engagement metrics.
type UserActivity struct {
	LastLoginAt      *string   `json:"last_login_at,omitempty"`
	LastAPIRequestAt *string   `json:"last_api_request_at,omitempty"`
	TotalLogins      int64     `json:"total_logins"`
	DashboardViews   int64     `json:"dashboard_views"`
	APIRequestsCount int64     `json:"api_requests_count"`
	CreatedProjects  int64     `json:"created_projects"`
	JoinedOrgs       int64     `json:"joined_orgs"`
	UserID           uuid.UUID `json:"user_id"`
}

// UserStats represents aggregate user statistics.
type UserStats struct {
	TotalUsers        int64 `json:"total_users"`
	ActiveUsers       int64 `json:"active_users"`
	VerifiedUsers     int64 `json:"verified_users"`
	NewUsersToday     int64 `json:"new_users_today"`
	NewUsersThisWeek  int64 `json:"new_users_this_week"`
	NewUsersThisMonth int64 `json:"new_users_this_month"`
}
