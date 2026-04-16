package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	userDomain "brokle/internal/core/domain/user"
	appErrors "brokle/pkg/errors"
)

// userService implements the user.UserService interface
type userService struct {
	userRepo      userDomain.Repository
	authService   authDomain.AuthService
	orgMemberRepo authDomain.OrganizationMemberRepository
}

// NewUserService creates a new user service instance
func NewUserService(
	userRepo userDomain.Repository,
	authService authDomain.AuthService,
	orgMemberRepo authDomain.OrganizationMemberRepository,
) userDomain.UserService {
	return &userService{
		userRepo:      userRepo,
		authService:   authService,
		orgMemberRepo: orgMemberRepo,
	}
}

// GetUser retrieves user by ID
func (s *userService) GetUser(ctx context.Context, userID uuid.UUID) (*userDomain.User, error) {
	return s.userRepo.GetByID(ctx, userID)
}

// GetUserByEmail retrieves user by email (without password)
func (s *userService) GetUserByEmail(ctx context.Context, email string) (*userDomain.User, error) {
	return s.userRepo.GetByEmail(ctx, email)
}

// GetUserByEmailWithPassword retrieves user by email with password for authentication
func (s *userService) GetUserByEmailWithPassword(ctx context.Context, email string) (*userDomain.User, error) {
	return s.userRepo.GetByEmailWithPassword(ctx, email)
}

// UpdateUser updates user information
func (s *userService) UpdateUser(ctx context.Context, userID uuid.UUID, req *userDomain.UpdateUserRequest) (*userDomain.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewNotFoundError("User not found")
		}
		return nil, appErrors.NewInternalError("User lookup failed", err)
	}

	// Update fields if provided
	if req.FirstName != nil {
		user.FirstName = *req.FirstName
	}
	if req.LastName != nil {
		user.LastName = *req.LastName
	}
	if req.Timezone != nil {
		user.Timezone = *req.Timezone
	}
	if req.Language != nil {
		user.Language = *req.Language
	}

	user.UpdatedAt = time.Now()

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to update user", err)
	}

	return user, nil
}

// DeactivateUser deactivates a user account
func (s *userService) DeactivateUser(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	user.IsActive = false
	user.UpdatedAt = time.Now()

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return appErrors.NewInternalError("Failed to deactivate user", err)
	}

	return nil
}

// ReactivateUser reactivates a deactivated user account
func (s *userService) ReactivateUser(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	user.IsActive = true
	user.UpdatedAt = time.Now()

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return appErrors.NewInternalError("Failed to reactivate user", err)
	}

	return nil
}

// DeleteUser soft deletes a user account
func (s *userService) DeleteUser(ctx context.Context, userID uuid.UUID) error {
	_, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	err = s.userRepo.Delete(ctx, userID)
	if err != nil {
		return appErrors.NewInternalError("Failed to delete user", err)
	}

	return nil
}

// ListUsers retrieves users with pagination and filters
func (s *userService) ListUsers(ctx context.Context, filters *userDomain.ListFilters) ([]*userDomain.User, int, error) {
	users, total, err := s.userRepo.List(ctx, filters)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("Failed to list users", err)
	}

	return users, total, nil
}

// SearchUsers searches for users by query
func (s *userService) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*userDomain.User, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*userDomain.User{}, 0, nil
	}

	// For now, use List with basic filters since Search may not be implemented
	filters := &userDomain.ListFilters{}
	filters.Params.Limit = limit
	filters.Params.Page = 1 // First page for search results
	filters.Params.SortBy = "created_at"
	filters.Params.SortDir = "desc"
	users, total, err := s.userRepo.List(ctx, filters)
	if err != nil {
		return nil, 0, appErrors.NewInternalError("Failed to search users", err)
	}

	return users, total, nil
}

// GetUsersByIDs retrieves multiple users by their IDs
func (s *userService) GetUsersByIDs(ctx context.Context, userIDs []uuid.UUID) ([]*userDomain.User, error) {
	if len(userIDs) == 0 {
		return []*userDomain.User{}, nil
	}

	return s.userRepo.GetByIDs(ctx, userIDs)
}

// GetPublicUsers retrieves public user information by IDs
func (s *userService) GetPublicUsers(ctx context.Context, userIDs []uuid.UUID) ([]*userDomain.PublicUser, error) {
	users, err := s.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	publicUsers := make([]*userDomain.PublicUser, len(users))
	for i, user := range users {
		publicUsers[i] = user.ToPublic()
	}

	return publicUsers, nil
}

// VerifyEmail verifies user's email with token
func (s *userService) VerifyEmail(ctx context.Context, userID uuid.UUID, token string) error {
	// This would typically validate the token and mark email as verified
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	now := time.Now()
	user.IsEmailVerified = true
	user.EmailVerifiedAt = &now
	user.UpdatedAt = now

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return appErrors.NewInternalError("Failed to verify email", err)
	}

	return nil
}

// MarkEmailAsVerified directly marks user's email as verified
func (s *userService) MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	now := time.Now()
	user.IsEmailVerified = true
	user.EmailVerifiedAt = &now
	user.UpdatedAt = now

	return s.userRepo.Update(ctx, user)
}

// SendVerificationEmail sends email verification email
func (s *userService) SendVerificationEmail(ctx context.Context, userID uuid.UUID) error {
	// This would integrate with email service to send verification email
	// Implementation would trigger email via notification service
	return nil
}

// RequestPasswordReset initiates password reset process
func (s *userService) RequestPasswordReset(ctx context.Context, email string) error {
	_, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		// Don't reveal if email exists or not for security
		return nil
	}

	// This would generate reset token and send email
	// Implementation would create token and trigger email via notification service
	return nil
}

// ResetPassword resets user password with token
func (s *userService) ResetPassword(ctx context.Context, token, newPassword string) error {
	// This would validate token and update password
	// Implementation would need token validation logic
	return nil
}

// ChangePassword changes user password
func (s *userService) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	// Verify current password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword))
	if err != nil {
		return appErrors.NewUnauthorizedError("Current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return appErrors.NewInternalError("Failed to hash password", err)
	}

	user.Password = string(hashedPassword)
	user.UpdatedAt = time.Now()

	err = s.userRepo.Update(ctx, user)
	if err != nil {
		return appErrors.NewInternalError("Failed to update password", err)
	}

	return nil
}

// UpdateLastLogin updates user's last login time
func (s *userService) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	now := time.Now()
	user.LastLoginAt = &now
	user.LoginCount++
	user.UpdatedAt = now

	return s.userRepo.Update(ctx, user)
}

// GetUserActivity retrieves user activity metrics
func (s *userService) GetUserActivity(ctx context.Context, userID uuid.UUID) (*userDomain.UserActivity, error) {
	// This would aggregate activity data from various sources
	// For now, return basic activity
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewNotFoundError("User not found")
		}
		return nil, appErrors.NewInternalError("User lookup failed", err)
	}

	activity := &userDomain.UserActivity{
		UserID:           userID,
		TotalLogins:      0, // Would be calculated from sessions
		DashboardViews:   0, // Would be calculated from analytics
		APIRequestsCount: 0, // Would be calculated from API logs
		CreatedProjects:  0, // Would be calculated from projects
		JoinedOrgs:       0, // Would be calculated from organization memberships
	}

	if user.LastLoginAt != nil {
		lastLogin := user.LastLoginAt.Format(time.RFC3339)
		activity.LastLoginAt = &lastLogin
	}

	return activity, nil
}

// SetDefaultOrganization sets user's default organization
func (s *userService) SetDefaultOrganization(ctx context.Context, userID, orgID uuid.UUID) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("User not found")
		}
		return appErrors.NewInternalError("User lookup failed", err)
	}

	user.DefaultOrganizationID = &orgID
	return s.userRepo.Update(ctx, user)
}

// GetDefaultOrganization gets user's default organization
func (s *userService) GetDefaultOrganization(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewNotFoundError("User not found")
		}
		return nil, appErrors.NewInternalError("User lookup failed", err)
	}

	return user.DefaultOrganizationID, nil
}

// ValidateUserOrgMembership checks if user is a member of the organization
func (s *userService) ValidateUserOrgMembership(ctx context.Context, userID, orgID uuid.UUID) (bool, error) {
	// Use Exists method - returns (false, nil) when not found, no ErrRecordNotFound handling needed
	return s.orgMemberRepo.Exists(ctx, userID, orgID)
}

// GetUserStats retrieves aggregate user statistics
func (s *userService) GetUserStats(ctx context.Context) (*userDomain.UserStats, error) {
	// This would aggregate statistics from the database
	// For now, return basic stats structure
	return &userDomain.UserStats{
		TotalUsers:        0, // Would be calculated
		ActiveUsers:       0, // Would be calculated
		VerifiedUsers:     0, // Would be calculated
		NewUsersToday:     0, // Would be calculated
		NewUsersThisWeek:  0, // Would be calculated
		NewUsersThisMonth: 0, // Would be calculated
	}, nil
}
