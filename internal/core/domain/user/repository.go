package user

import (
	"context"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/pagination"
)

// Repository defines the interface for user data persistence.
// This interface abstracts the data access layer, allowing for different
// implementations (PostgreSQL, in-memory, etc.) while keeping the domain
// logic independent of storage details.
type Repository interface {
	// User operations
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByEmailWithPassword(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, filters *ListFilters) ([]*User, int, error)

	// Profile operations
	CreateProfile(ctx context.Context, profile *UserProfile) error
	GetProfile(ctx context.Context, userID uuid.UUID) (*UserProfile, error)
	UpdateProfile(ctx context.Context, profile *UserProfile) error

	// Authentication operations
	UpdatePassword(ctx context.Context, userID uuid.UUID, hashedPassword string) error
	MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error
	VerifyEmail(ctx context.Context, userID uuid.UUID, token string) error
	SetDefaultOrganization(ctx context.Context, userID, orgID uuid.UUID) error
	GetDefaultOrganization(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error)
	DeactivateUser(ctx context.Context, userID uuid.UUID) error
	ReactivateUser(ctx context.Context, userID uuid.UUID) error

	// Batch operations
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*User, error)
	UpdateLastLogin(ctx context.Context, userID uuid.UUID) error

	// Search and filtering
	Search(ctx context.Context, query string, limit, offset int) ([]*User, int, error)
	GetActiveUsers(ctx context.Context, limit, offset int) ([]*User, int, error)
	GetVerifiedUsers(ctx context.Context, limit, offset int) ([]*User, int, error)
	GetUsersByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*User, error)

	// User statistics
	GetUserStats(ctx context.Context) (*UserStats, error)
	GetNewUsersCount(ctx context.Context, since time.Time) (int64, error)

	// Transaction support
	Transaction(fn func(Repository) error) error
}

// Filter defines a generic filter type for compatibility
type Filter = ListFilters

// ListFilters defines filters for listing users.
type ListFilters struct {
	// Domain filters
	IsActive        *bool      `json:"is_active,omitempty"`
	IsVerified      *bool      `json:"is_verified,omitempty"`
	IsEmailVerified *bool      `json:"is_email_verified,omitempty"`
	CreatedAfter    *time.Time `json:"created_after,omitempty"`    // Date filter
	CreatedBefore   *time.Time `json:"created_before,omitempty"`   // Date filter
	LastLoginAfter  *time.Time `json:"last_login_after,omitempty"` // Last login filter
	Search          string     `json:"search,omitempty"`           // Search in name and email
	HasDefaultOrg   *bool      `json:"has_default_org,omitempty"`  // Filter by having default organization

	// Pagination (embedded for DRY)
	pagination.Params `json:",inline"`
}

// UserFilters is an alias for ListFilters for backward compatibility
type UserFilters = ListFilters
