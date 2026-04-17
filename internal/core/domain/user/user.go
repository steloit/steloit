// Package user provides the user domain model and core business logic.
//
// The user domain handles user account management, profiles, and preferences.
// It maintains user identity and authentication state across the platform.
package user

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// User represents a platform user with full authentication support.
type User struct {
	UpdatedAt             time.Time  `json:"updated_at"`
	CreatedAt             time.Time  `json:"created_at"`
	EmailVerifiedAt       *time.Time `json:"email_verified_at,omitempty"`
	OAuthProviderID       *string    `json:"-"`
	OAuthProvider         *string    `json:"oauth_provider,omitempty"`
	ReferralSource        *string    `json:"referral_source,omitempty"`
	DefaultOrganizationID *uuid.UUID `json:"default_organization_id,omitempty"`
	LastLoginAt           *time.Time `json:"last_login_at,omitempty"`
	DeletedAt             *time.Time `json:"deleted_at,omitempty"`
	AuthMethod            string     `json:"auth_method"`
	Language              string     `json:"language"`
	Role                  string     `json:"role"`
	Timezone              string     `json:"timezone"`
	Password              string     `json:"-"`
	LastName              string     `json:"last_name"`
	FirstName             string     `json:"first_name"`
	Email                 string     `json:"email"`
	LoginCount            int        `json:"login_count"`
	ID                    uuid.UUID  `json:"id"`
	IsEmailVerified       bool       `json:"is_email_verified"`
	IsActive              bool       `json:"is_active"`
}

// UserProfile represents extended user profile information and preferences.
type UserProfile struct {
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	Bio                   *string   `json:"bio,omitempty"`
	Location              *string   `json:"location,omitempty"`
	Website               *string   `json:"website,omitempty"`
	TwitterURL            *string   `json:"twitter_url,omitempty"`
	LinkedInURL           *string   `json:"linkedin_url,omitempty"`
	GitHubURL             *string   `json:"github_url,omitempty"`
	AvatarURL             *string   `json:"avatar_url,omitempty"`
	Phone                 *string   `json:"phone,omitempty"`
	Language              string    `json:"language"`
	Theme                 string    `json:"theme"`
	Timezone              string    `json:"timezone"`
	UsageThresholdPercent int       `json:"usage_threshold_percent"`
	UserID                uuid.UUID `json:"user_id"`
	EmailNotifications    bool      `json:"email_notifications"`
	PushNotifications     bool      `json:"push_notifications"`
	MarketingEmails       bool      `json:"marketing_emails"`
	WeeklyReports         bool      `json:"weekly_reports"`
	MonthlyReports        bool      `json:"monthly_reports"`
	SecurityAlerts        bool      `json:"security_alerts"`
	BillingAlerts         bool      `json:"billing_alerts"`
}

// CreateUserRequest represents the data needed to create a new user.
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email"`
	FirstName string `json:"first_name" validate:"required,min=1,max=100"`
	LastName  string `json:"last_name" validate:"required,min=1,max=100"`
	Password  string `json:"password" validate:"required,min=8"`
	Timezone  string `json:"timezone,omitempty" validate:"omitempty"`
	Language  string `json:"language,omitempty" validate:"omitempty,len=2"`
}

// UpdateUserRequest represents the data that can be updated for a user.
type UpdateUserRequest struct {
	FirstName *string `json:"first_name,omitempty" validate:"omitempty,min=1,max=100"`
	LastName  *string `json:"last_name,omitempty" validate:"omitempty,min=1,max=100"`
	Timezone  *string `json:"timezone,omitempty" validate:"omitempty"`
	Language  *string `json:"language,omitempty" validate:"omitempty,len=2"`
}

// UpdateProfileRequest represents the data that can be updated for a user profile.
type UpdateProfileRequest struct {
	// Profile information
	Bio         *string `json:"bio,omitempty" validate:"omitempty,max=500"`
	Location    *string `json:"location,omitempty" validate:"omitempty,max=100"`
	Website     *string `json:"website,omitempty" validate:"omitempty,url"`
	TwitterURL  *string `json:"twitter_url,omitempty" validate:"omitempty,url"`
	LinkedInURL *string `json:"linkedin_url,omitempty" validate:"omitempty,url"`
	GitHubURL   *string `json:"github_url,omitempty" validate:"omitempty,url"`

	// Contact information
	AvatarURL *string `json:"avatar_url,omitempty" validate:"omitempty,url"`
	Phone     *string `json:"phone,omitempty" validate:"omitempty,max=50"`

	// Display preferences
	Timezone *string `json:"timezone,omitempty" validate:"omitempty"`
	Language *string `json:"language,omitempty" validate:"omitempty,len=2"`
	Theme    *string `json:"theme,omitempty" validate:"omitempty,oneof=light dark auto"`

	// Notification preferences
	EmailNotifications    *bool `json:"email_notifications,omitempty"`
	PushNotifications     *bool `json:"push_notifications,omitempty"`
	MarketingEmails       *bool `json:"marketing_emails,omitempty"`
	WeeklyReports         *bool `json:"weekly_reports,omitempty"`
	MonthlyReports        *bool `json:"monthly_reports,omitempty"`
	SecurityAlerts        *bool `json:"security_alerts,omitempty"`
	BillingAlerts         *bool `json:"billing_alerts,omitempty"`
	UsageThresholdPercent *int  `json:"usage_threshold_percent,omitempty" validate:"omitempty,min=0,max=100"`
}

// PublicUser represents a user without sensitive information.
type PublicUser struct {
	CreatedAt       time.Time `json:"created_at"`
	Name            string    `json:"name"`
	ID              uuid.UUID `json:"id"`
	IsEmailVerified bool      `json:"is_email_verified"`
}

// ToPublic converts a User to PublicUser, removing sensitive information.
func (u *User) ToPublic() *PublicUser {
	return &PublicUser{
		ID:              u.ID,
		Name:            u.GetFullName(),
		IsEmailVerified: u.IsEmailVerified,
		CreatedAt:       u.CreatedAt,
	}
}

// GetFullName returns the user's full name.
func (u *User) GetFullName() string {
	return u.FirstName + " " + u.LastName
}

// IsEmailVerified checks if user's email is verified.
func (u *User) IsVerified() bool {
	return u.IsEmailVerified
}

// MarkEmailAsVerified marks the user's email as verified.
func (u *User) MarkEmailAsVerified() {
	now := time.Now()
	u.IsEmailVerified = true
	u.EmailVerifiedAt = &now
	u.UpdatedAt = now
}

// UpdateLastLogin updates the user's last login timestamp and count.
func (u *User) UpdateLastLogin() {
	now := time.Now()
	u.LastLoginAt = &now
	u.LoginCount++
	u.UpdatedAt = now
}

// SetPassword sets the user's password hash.
func (u *User) SetPassword(hashedPassword string) {
	u.Password = hashedPassword
	u.UpdatedAt = time.Now()
}

// SetDefaultOrganization sets the user's default organization.
func (u *User) SetDefaultOrganization(orgID uuid.UUID) {
	u.DefaultOrganizationID = &orgID
	u.UpdatedAt = time.Now()
}

// Deactivate deactivates the user account.
func (u *User) Deactivate() {
	u.IsActive = false
	u.UpdatedAt = time.Now()
}

// Reactivate reactivates the user account.
func (u *User) Reactivate() {
	u.IsActive = true
	u.UpdatedAt = time.Now()
}

// NewUser creates a new user with default values.
func NewUser(email, firstName, lastName, role string) *User {
	return &User{
		ID:              uid.New(),
		Email:           email,
		FirstName:       firstName,
		LastName:        lastName,
		Role:            role,
		IsActive:        true,
		IsEmailVerified: false,
		Timezone:        "UTC",
		Language:        "en",
		LoginCount:      0,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// NewUserProfile creates a new user profile with default values.
func NewUserProfile(userID uuid.UUID) *UserProfile {
	return &UserProfile{
		UserID:   userID,
		Timezone: "UTC",
		Language: "en",
		Theme:    "light",

		// Default notification preferences
		EmailNotifications:    true,
		PushNotifications:     true,
		MarketingEmails:       false,
		WeeklyReports:         true,
		MonthlyReports:        true,
		SecurityAlerts:        true,
		BillingAlerts:         true,
		UsageThresholdPercent: 80,

		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

