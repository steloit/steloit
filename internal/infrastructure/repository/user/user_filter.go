package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	sq "github.com/Masterminds/squirrel"

	userDomain "brokle/internal/core/domain/user"
	"brokle/internal/infrastructure/db"
	"brokle/pkg/pagination"
)

// Dynamic filter/search queries for the user aggregate. Static CRUD
// lives in user_repository.go and delegates to sqlc bindings; this
// file is squirrel-only.

var userSelectColumns = []string{
	"id", "email", "first_name", "last_name", "password",
	"is_active", "is_email_verified", "email_verified_at",
	"timezone", "language", "last_login_at", "login_count",
	"default_organization_id",
	"created_at", "updated_at", "deleted_at",
	"role", "referral_source",
	"auth_method", "oauth_provider", "oauth_provider_id",
}

var userAllowedSortFields = []string{"created_at", "updated_at", "email", "first_name", "last_name", "id"}

type rowScanner interface {
	Scan(dest ...any) error
}

// scanUser reads a row in userSelectColumns order into a domain User.
func scanUser(row rowScanner) (*userDomain.User, error) {
	var (
		u               userDomain.User
		password        *string
		emailVerifiedAt *time.Time
		lastLoginAt     *time.Time
		defaultOrgID    *uuid.UUID
		deletedAt       *time.Time
		referralSource  *string
		oauthProvider   *string
		oauthProviderID *string
		loginCount      int32
	)
	if err := row.Scan(
		&u.ID,
		&u.Email,
		&u.FirstName,
		&u.LastName,
		&password,
		&u.IsActive,
		&u.IsEmailVerified,
		&emailVerifiedAt,
		&u.Timezone,
		&u.Language,
		&lastLoginAt,
		&loginCount,
		&defaultOrgID,
		&u.CreatedAt,
		&u.UpdatedAt,
		&deletedAt,
		&u.Role,
		&referralSource,
		&u.AuthMethod,
		&oauthProvider,
		&oauthProviderID,
	); err != nil {
		return nil, err
	}
	u.Password = password
	u.EmailVerifiedAt = emailVerifiedAt
	u.LastLoginAt = lastLoginAt
	u.DefaultOrganizationID = defaultOrgID
	u.DeletedAt = deletedAt
	u.ReferralSource = referralSource
	u.OAuthProvider = oauthProvider
	u.OAuthProviderID = oauthProviderID
	u.LoginCount = int(loginCount)
	return &u, nil
}

// applyPagination resolves the final sort/limit/offset for a users
// query given optional pagination.Params. Returns (sortField, sortDir,
// limit, offset). Caller is responsible for applying these to the
// squirrel builder.
func applyPagination(p *pagination.Params, defaultField string) (string, string, uint64, uint64, error) {
	sortField := defaultField
	sortDir := "DESC"
	limit := uint64(pagination.DefaultPageSize)
	var offset uint64
	if p != nil {
		if p.SortBy != "" {
			v, err := pagination.ValidateSortField(p.SortBy, userAllowedSortFields)
			if err != nil {
				return "", "", 0, 0, err
			}
			if v != "" {
				sortField = v
			}
		}
		if p.SortDir == "asc" {
			sortDir = "ASC"
		}
		if p.Limit > 0 {
			limit = uint64(p.Limit)
		}
		offset = uint64(p.GetOffset())
	}
	return sortField, sortDir, limit, offset, nil
}

// listByFilters implements Repository.List with a dynamic WHERE clause.
// Returns (users, total, error). total uses an independent COUNT query
// with the same predicate.
func (r *userRepository) listByFilters(ctx context.Context, f *userDomain.UserFilters) ([]*userDomain.User, int, error) {
	selBuilder := sq.Select(userSelectColumns...).From("users").Where("deleted_at IS NULL")
	cntBuilder := sq.Select("COUNT(*)").From("users").Where("deleted_at IS NULL")

	if f != nil {
		if f.IsActive != nil {
			selBuilder = selBuilder.Where(sq.Eq{"is_active": *f.IsActive})
			cntBuilder = cntBuilder.Where(sq.Eq{"is_active": *f.IsActive})
		}
		if f.IsEmailVerified != nil {
			selBuilder = selBuilder.Where(sq.Eq{"is_email_verified": *f.IsEmailVerified})
			cntBuilder = cntBuilder.Where(sq.Eq{"is_email_verified": *f.IsEmailVerified})
		}
		if f.CreatedAfter != nil {
			selBuilder = selBuilder.Where(sq.Gt{"created_at": *f.CreatedAfter})
			cntBuilder = cntBuilder.Where(sq.Gt{"created_at": *f.CreatedAfter})
		}
		if f.CreatedBefore != nil {
			selBuilder = selBuilder.Where(sq.Lt{"created_at": *f.CreatedBefore})
			cntBuilder = cntBuilder.Where(sq.Lt{"created_at": *f.CreatedBefore})
		}
		if f.LastLoginAfter != nil {
			selBuilder = selBuilder.Where(sq.Gt{"last_login_at": *f.LastLoginAfter})
			cntBuilder = cntBuilder.Where(sq.Gt{"last_login_at": *f.LastLoginAfter})
		}
		if f.Search != "" {
			pattern := "%" + f.Search + "%"
			or := sq.Or{
				sq.ILike{"email": pattern},
				sq.ILike{"first_name": pattern},
				sq.ILike{"last_name": pattern},
			}
			selBuilder = selBuilder.Where(or)
			cntBuilder = cntBuilder.Where(or)
		}
		if f.HasDefaultOrg != nil {
			if *f.HasDefaultOrg {
				selBuilder = selBuilder.Where("default_organization_id IS NOT NULL")
				cntBuilder = cntBuilder.Where("default_organization_id IS NOT NULL")
			} else {
				selBuilder = selBuilder.Where("default_organization_id IS NULL")
				cntBuilder = cntBuilder.Where("default_organization_id IS NULL")
			}
		}
	}

	var params *pagination.Params
	if f != nil {
		params = &f.Params
	}
	sortField, sortDir, limit, offset, err := applyPagination(params, "created_at")
	if err != nil {
		return nil, 0, err
	}
	selBuilder = selBuilder.
		OrderBy(fmt.Sprintf("%s %s", sortField, sortDir), fmt.Sprintf("id %s", sortDir)).
		Limit(limit).
		Offset(offset)

	users, err := runUserSelect(ctx, r.tm, selBuilder)
	if err != nil {
		return nil, 0, err
	}

	total, err := runCount(ctx, r.tm, cntBuilder)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// GetActiveUsers returns users with a non-null last_login_at.
func (r *userRepository) GetActiveUsers(ctx context.Context, limit, offset int) ([]*userDomain.User, int, error) {
	pred := func(q sq.SelectBuilder) sq.SelectBuilder {
		return q.Where("last_login_at IS NOT NULL")
	}
	return r.paginated(ctx, pred, "last_login_at", limit, offset)
}

// GetVerifiedUsers returns users with is_email_verified = true.
func (r *userRepository) GetVerifiedUsers(ctx context.Context, limit, offset int) ([]*userDomain.User, int, error) {
	pred := func(q sq.SelectBuilder) sq.SelectBuilder {
		return q.Where(sq.Eq{"is_email_verified": true})
	}
	return r.paginated(ctx, pred, "created_at", limit, offset)
}

// Search matches email or first/last name (ILIKE).
func (r *userRepository) Search(ctx context.Context, query string, limit, offset int) ([]*userDomain.User, int, error) {
	if query == "" {
		return []*userDomain.User{}, 0, nil
	}
	pattern := "%" + query + "%"
	pred := func(q sq.SelectBuilder) sq.SelectBuilder {
		return q.Where(sq.Or{
			sq.ILike{"email": pattern},
			sq.ILike{"first_name": pattern},
			sq.ILike{"last_name": pattern},
		})
	}
	return r.paginated(ctx, pred, "created_at", limit, offset)
}

// paginated is the shared list/count flow used by GetActiveUsers,
// GetVerifiedUsers, and Search. pred applies the predicate to both
// the SELECT and COUNT builders so the total matches what was returned.
func (r *userRepository) paginated(
	ctx context.Context,
	pred func(sq.SelectBuilder) sq.SelectBuilder,
	orderField string,
	limit, offset int,
) ([]*userDomain.User, int, error) {
	selB := pred(sq.Select(userSelectColumns...).From("users").Where("deleted_at IS NULL"))
	cntB := pred(sq.Select("COUNT(*)").From("users").Where("deleted_at IS NULL"))

	if limit <= 0 {
		limit = pagination.DefaultPageSize
	}
	if offset < 0 {
		offset = 0
	}
	selB = selB.
		OrderBy(fmt.Sprintf("%s DESC", orderField), "id DESC").
		Limit(uint64(limit)).
		Offset(uint64(offset))

	users, err := runUserSelect(ctx, r.tm, selB)
	if err != nil {
		return nil, 0, err
	}
	total, err := runCount(ctx, r.tm, cntB)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// runUserSelect executes a user SELECT builder and scans every row
// into a domain User.
func runUserSelect(ctx context.Context, tm *db.TxManager, b sq.SelectBuilder) ([]*userDomain.User, error) {
	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build users filter query: %w", err)
	}
	rows, err := tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()
	out := make([]*userDomain.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan users row: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users rows: %w", err)
	}
	return out, nil
}

// runCount executes a COUNT(*) builder and returns the int total.
func runCount(ctx context.Context, tm *db.TxManager, b sq.SelectBuilder) (int, error) {
	sqlStr, args, err := b.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build users count query: %w", err)
	}
	row := tm.DB(ctx).QueryRow(ctx, sqlStr, args...)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("scan users count: %w", err)
	}
	return int(n), nil
}
