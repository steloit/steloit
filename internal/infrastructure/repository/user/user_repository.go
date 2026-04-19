package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	userDomain "brokle/internal/core/domain/user"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// userRepository is the pgx+sqlc implementation of userDomain.Repository.
// Dynamic search/filter queries live in user_filter.go (squirrel).
type userRepository struct {
	tm *db.TxManager
}

// NewUserRepository returns the pgx-backed repository.
func NewUserRepository(tm *db.TxManager) userDomain.Repository {
	return &userRepository{tm: tm}
}

// ----- CRUD ----------------------------------------------------------

func (r *userRepository) Create(ctx context.Context, u *userDomain.User) error {
	now := time.Now()
	if u.CreatedAt.IsZero() {
		u.CreatedAt = now
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = now
	}
	if u.Timezone == "" {
		u.Timezone = "UTC"
	}
	if u.Language == "" {
		u.Language = "en"
	}
	if u.AuthMethod == "" {
		u.AuthMethod = "password"
	}
	if err := r.tm.Queries(ctx).CreateUser(ctx, gen.CreateUserParams{
		ID:                    u.ID,
		Email:                 u.Email,
		FirstName:             u.FirstName,
		LastName:              u.LastName,
		Password:              u.Password,
		IsActive:              u.IsActive,
		IsEmailVerified:       u.IsEmailVerified,
		EmailVerifiedAt:       u.EmailVerifiedAt,
		Timezone:              u.Timezone,
		Language:              u.Language,
		LastLoginAt:           u.LastLoginAt,
		LoginCount:            int32(u.LoginCount),
		DefaultOrganizationID: u.DefaultOrganizationID,
		Role:                  u.Role,
		ReferralSource:        u.ReferralSource,
		AuthMethod:            u.AuthMethod,
		OauthProvider:         u.OAuthProvider,
		OauthProviderID:       u.OAuthProviderID,
		CreatedAt:             u.CreatedAt,
		UpdatedAt:             u.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create user %s: %w", u.Email, err)
	}
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id uuid.UUID) (*userDomain.User, error) {
	row, err := r.tm.Queries(ctx).GetUserByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get user %s: %w", id, userDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user %s: %w", id, err)
	}
	return userFromRow(&row), nil
}

func (r *userRepository) GetByEmail(ctx context.Context, email string) (*userDomain.User, error) {
	row, err := r.tm.Queries(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get user by email %s: %w", email, userDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get user by email %s: %w", email, err)
	}
	return userFromRow(&row), nil
}

// GetByEmailWithPassword is kept on the interface because the GORM-era
// repo used a `Select("*")` to ensure the password column loaded even
// when a caller had configured column masking. With sqlc the password
// is always on the row struct, so this delegates to GetByEmail.
func (r *userRepository) GetByEmailWithPassword(ctx context.Context, email string) (*userDomain.User, error) {
	return r.GetByEmail(ctx, email)
}

func (r *userRepository) Update(ctx context.Context, u *userDomain.User) error {
	if err := r.tm.Queries(ctx).UpdateUser(ctx, gen.UpdateUserParams{
		ID:                    u.ID,
		Email:                 u.Email,
		FirstName:             u.FirstName,
		LastName:              u.LastName,
		Password:              u.Password,
		IsActive:              u.IsActive,
		IsEmailVerified:       u.IsEmailVerified,
		EmailVerifiedAt:       u.EmailVerifiedAt,
		Timezone:              u.Timezone,
		Language:              u.Language,
		LastLoginAt:           u.LastLoginAt,
		LoginCount:            int32(u.LoginCount),
		DefaultOrganizationID: u.DefaultOrganizationID,
		Role:                  u.Role,
		ReferralSource:        u.ReferralSource,
		AuthMethod:            u.AuthMethod,
		OauthProvider:         u.OAuthProvider,
		OauthProviderID:       u.OAuthProviderID,
	}); err != nil {
		return fmt.Errorf("update user %s: %w", u.ID, err)
	}
	return nil
}

func (r *userRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteUser(ctx, id); err != nil {
		return fmt.Errorf("soft-delete user %s: %w", id, err)
	}
	return nil
}

// List delegates to the squirrel-based filter implementation.
// ListFilters and UserFilters are aliases of the same type.
func (r *userRepository) List(ctx context.Context, filters *userDomain.ListFilters) ([]*userDomain.User, int, error) {
	return r.listByFilters(ctx, (*userDomain.UserFilters)(filters))
}

// ----- Authentication / state -----------------------------------------

func (r *userRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, hashedPassword string) error {
	if err := r.tm.Queries(ctx).UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
		ID:       userID,
		Password: &hashedPassword,
	}); err != nil {
		return fmt.Errorf("update user password %s: %w", userID, err)
	}
	return nil
}

func (r *userRepository) UpdateLastLogin(ctx context.Context, userID uuid.UUID) error {
	if err := r.tm.Queries(ctx).UpdateUserLastLogin(ctx, userID); err != nil {
		return fmt.Errorf("update last login for user %s: %w", userID, err)
	}
	return nil
}

func (r *userRepository) MarkEmailAsVerified(ctx context.Context, userID uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkUserEmailVerified(ctx, userID); err != nil {
		return fmt.Errorf("mark email verified for user %s: %w", userID, err)
	}
	return nil
}

// VerifyEmail is a legacy token-validating entry point used by the
// service layer; the token is validated elsewhere and this delegates
// to MarkEmailAsVerified.
func (r *userRepository) VerifyEmail(ctx context.Context, userID uuid.UUID, _ string) error {
	return r.MarkEmailAsVerified(ctx, userID)
}

func (r *userRepository) SetDefaultOrganization(ctx context.Context, userID uuid.UUID, orgID uuid.UUID) error {
	var orgPtr *uuid.UUID
	if orgID != uuid.Nil {
		orgPtr = &orgID
	}
	if err := r.tm.Queries(ctx).SetUserDefaultOrganization(ctx, gen.SetUserDefaultOrganizationParams{
		ID:                    userID,
		DefaultOrganizationID: orgPtr,
	}); err != nil {
		return fmt.Errorf("set default org for user %s: %w", userID, err)
	}
	return nil
}

func (r *userRepository) GetDefaultOrganization(ctx context.Context, userID uuid.UUID) (*uuid.UUID, error) {
	ptr, err := r.tm.Queries(ctx).GetUserDefaultOrganization(ctx, userID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get default org for user %s: %w", userID, userDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get default org for user %s: %w", userID, err)
	}
	return ptr, nil
}

func (r *userRepository) DeactivateUser(ctx context.Context, userID uuid.UUID) error {
	return r.setActive(ctx, userID, false)
}

func (r *userRepository) ReactivateUser(ctx context.Context, userID uuid.UUID) error {
	return r.setActive(ctx, userID, true)
}

func (r *userRepository) setActive(ctx context.Context, userID uuid.UUID, active bool) error {
	if err := r.tm.Queries(ctx).SetUserActive(ctx, gen.SetUserActiveParams{
		ID:       userID,
		IsActive: active,
	}); err != nil {
		return fmt.Errorf("set user %s active=%t: %w", userID, active, err)
	}
	return nil
}

// ----- Batch + listings ----------------------------------------------

func (r *userRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*userDomain.User, error) {
	if len(ids) == 0 {
		return []*userDomain.User{}, nil
	}
	rows, err := r.tm.Queries(ctx).ListUsersByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("list users by ids (n=%d): %w", len(ids), err)
	}
	return usersFromRows(rows), nil
}

func (r *userRepository) GetUsersByOrganization(ctx context.Context, organizationID uuid.UUID) ([]*userDomain.User, error) {
	rows, err := r.tm.Queries(ctx).ListUsersByOrganization(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list users for org %s: %w", organizationID, err)
	}
	return usersFromRows(rows), nil
}

// ----- Statistics -----------------------------------------------------

func (r *userRepository) GetUserStats(ctx context.Context) (*userDomain.UserStats, error) {
	q := r.tm.Queries(ctx)

	total, err := q.CountActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	since := time.Now().AddDate(0, 0, -30)
	active, err := q.CountUsersLoggedInSince(ctx, &since)
	if err != nil {
		return nil, fmt.Errorf("count active users: %w", err)
	}
	verified, err := q.CountVerifiedUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count verified users: %w", err)
	}
	newToday, err := q.CountUsersCreatedSince(ctx, time.Now().Truncate(24*time.Hour))
	if err != nil {
		return nil, fmt.Errorf("count new users today: %w", err)
	}
	return &userDomain.UserStats{
		TotalUsers:    total,
		ActiveUsers:   active,
		VerifiedUsers: verified,
		NewUsersToday: newToday,
	}, nil
}

func (r *userRepository) GetNewUsersCount(ctx context.Context, since time.Time) (int64, error) {
	n, err := r.tm.Queries(ctx).CountUsersCreatedSince(ctx, since)
	if err != nil {
		return 0, fmt.Errorf("count new users since %s: %w", since, err)
	}
	return n, nil
}

// ----- Profile operations --------------------------------------------

func (r *userRepository) CreateProfile(ctx context.Context, p *userDomain.UserProfile) error {
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	if p.Theme == "" {
		p.Theme = "light"
	}
	if p.Language == "" {
		p.Language = "en"
	}
	if p.Timezone == "" {
		p.Timezone = "UTC"
	}
	if err := r.tm.Queries(ctx).CreateUserProfile(ctx, gen.CreateUserProfileParams{
		UserID:                p.UserID,
		Bio:                   p.Bio,
		Location:              p.Location,
		Website:               p.Website,
		TwitterUrl:            p.TwitterURL,
		LinkedinUrl:           p.LinkedInURL,
		GithubUrl:             p.GitHubURL,
		AvatarUrl:             p.AvatarURL,
		Phone:                 p.Phone,
		Timezone:              p.Timezone,
		Language:              p.Language,
		Theme:                 p.Theme,
		EmailNotifications:    p.EmailNotifications,
		PushNotifications:     p.PushNotifications,
		MarketingEmails:       p.MarketingEmails,
		WeeklyReports:         p.WeeklyReports,
		MonthlyReports:        p.MonthlyReports,
		SecurityAlerts:        p.SecurityAlerts,
		BillingAlerts:         p.BillingAlerts,
		UsageThresholdPercent: int32(p.UsageThresholdPercent),
		CreatedAt:             p.CreatedAt,
		UpdatedAt:             p.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create profile for user %s: %w", p.UserID, err)
	}
	return nil
}

func (r *userRepository) GetProfile(ctx context.Context, userID uuid.UUID) (*userDomain.UserProfile, error) {
	row, err := r.tm.Queries(ctx).GetUserProfile(ctx, userID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get profile for user %s: %w", userID, userDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get profile for user %s: %w", userID, err)
	}
	return profileFromRow(&row), nil
}

func (r *userRepository) UpdateProfile(ctx context.Context, p *userDomain.UserProfile) error {
	if err := r.tm.Queries(ctx).UpdateUserProfile(ctx, gen.UpdateUserProfileParams{
		UserID:                p.UserID,
		Bio:                   p.Bio,
		Location:              p.Location,
		Website:               p.Website,
		TwitterUrl:            p.TwitterURL,
		LinkedinUrl:           p.LinkedInURL,
		GithubUrl:             p.GitHubURL,
		AvatarUrl:             p.AvatarURL,
		Phone:                 p.Phone,
		Timezone:              p.Timezone,
		Language:              p.Language,
		Theme:                 p.Theme,
		EmailNotifications:    p.EmailNotifications,
		PushNotifications:     p.PushNotifications,
		MarketingEmails:       p.MarketingEmails,
		WeeklyReports:         p.WeeklyReports,
		MonthlyReports:        p.MonthlyReports,
		SecurityAlerts:        p.SecurityAlerts,
		BillingAlerts:         p.BillingAlerts,
		UsageThresholdPercent: int32(p.UsageThresholdPercent),
	}); err != nil {
		return fmt.Errorf("update profile for user %s: %w", p.UserID, err)
	}
	return nil
}

// Transaction is on the domain interface but has no callers. Kept to
// satisfy the interface; delegates to TxManager. The fn receives a new
// userRepository that shares the same TxManager — tx scoping travels
// through ctx.
func (r *userRepository) Transaction(fn func(userDomain.Repository) error) error {
	return r.tm.WithinTransaction(context.Background(), func(ctx context.Context) error {
		return fn(r)
	})
}

// ----- gen ↔ domain boundary ----------------------------------------

func userFromRow(row *gen.User) *userDomain.User {
	return &userDomain.User{
		ID:                    row.ID,
		Email:                 row.Email,
		FirstName:             row.FirstName,
		LastName:              row.LastName,
		Password:              row.Password,
		IsActive:              row.IsActive,
		IsEmailVerified:       row.IsEmailVerified,
		EmailVerifiedAt:       row.EmailVerifiedAt,
		Timezone:              row.Timezone,
		Language:              row.Language,
		LastLoginAt:           row.LastLoginAt,
		LoginCount:            int(row.LoginCount),
		DefaultOrganizationID: row.DefaultOrganizationID,
		Role:                  row.Role,
		ReferralSource:        row.ReferralSource,
		AuthMethod:            row.AuthMethod,
		OAuthProvider:         row.OauthProvider,
		OAuthProviderID:       row.OauthProviderID,
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
		DeletedAt:             row.DeletedAt,
	}
}

func usersFromRows(rows []gen.User) []*userDomain.User {
	out := make([]*userDomain.User, 0, len(rows))
	for i := range rows {
		out = append(out, userFromRow(&rows[i]))
	}
	return out
}

func profileFromRow(row *gen.UserProfile) *userDomain.UserProfile {
	return &userDomain.UserProfile{
		UserID:                row.UserID,
		Bio:                   row.Bio,
		Location:              row.Location,
		Website:               row.Website,
		TwitterURL:            row.TwitterUrl,
		LinkedInURL:           row.LinkedinUrl,
		GitHubURL:             row.GithubUrl,
		AvatarURL:             row.AvatarUrl,
		Phone:                 row.Phone,
		Timezone:              row.Timezone,
		Language:              row.Language,
		Theme:                 row.Theme,
		EmailNotifications:    row.EmailNotifications,
		PushNotifications:     row.PushNotifications,
		MarketingEmails:       row.MarketingEmails,
		WeeklyReports:         row.WeeklyReports,
		MonthlyReports:        row.MonthlyReports,
		SecurityAlerts:        row.SecurityAlerts,
		BillingAlerts:         row.BillingAlerts,
		UsageThresholdPercent: int(row.UsageThresholdPercent),
		CreatedAt:             row.CreatedAt,
		UpdatedAt:             row.UpdatedAt,
	}
}

