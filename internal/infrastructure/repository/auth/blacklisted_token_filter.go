package auth

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/pagination"
)

// GetBlacklistedTokensByUser composes the SELECT dynamically because the
// filter struct has >3 optional predicates — sqlc's @param + COALESCE
// pattern doesn't scale there (see sqlc discussion #364). Queries flow
// through TxManager.DB(ctx), so they participate in outer transactions
// exactly like sqlc-generated queries do.
func (r *blacklistedTokenRepository) GetBlacklistedTokensByUser(ctx context.Context, filters *authDomain.BlacklistedTokenFilter) ([]*authDomain.BlacklistedToken, error) {
	q := sq.Select(
		"jti",
		"user_id",
		"expires_at",
		"revoked_at",
		"reason",
		"token_type",
		"blacklist_timestamp",
		"created_at",
	).From("blacklisted_tokens")

	if filters != nil {
		if filters.UserID != nil {
			q = q.Where(sq.Eq{"user_id": *filters.UserID})
		}
		if filters.Reason != nil {
			q = q.Where(sq.Eq{"reason": *filters.Reason})
		}
	}

	// Sort with a stable secondary key so paginated reads are deterministic
	// even when two rows share the primary sort value.
	sortField := "created_at"
	sortDir := "DESC"
	if filters != nil {
		allowed := []string{"created_at", "expires_at", "reason", "jti"}
		if filters.Params.SortBy != "" {
			validated, err := pagination.ValidateSortField(filters.Params.SortBy, allowed)
			if err != nil {
				return nil, err
			}
			if validated != "" {
				sortField = validated
			}
		}
		if filters.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}
	q = q.OrderBy(
		fmt.Sprintf("%s %s", sortField, sortDir),
		fmt.Sprintf("jti %s", sortDir),
	)

	limit := uint64(pagination.DefaultPageSize)
	var offset uint64
	if filters != nil {
		if filters.Params.Limit > 0 {
			limit = uint64(filters.Params.Limit)
		}
		offset = uint64(filters.Params.GetOffset())
	}
	q = q.Limit(limit).Offset(offset)

	sqlStr, args, err := q.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build blacklisted_tokens filter query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query blacklisted_tokens by filter: %w", err)
	}
	defer rows.Close()

	var out []*authDomain.BlacklistedToken
	for rows.Next() {
		var (
			jti       uuid.UUID
			userID    uuid.UUID
			expiresAt time.Time
			revokedAt time.Time
			reason    string
			tokenType string
			blackTS   *int64
			createdAt time.Time
		)
		if err := rows.Scan(
			&jti, &userID, &expiresAt, &revokedAt, &reason, &tokenType, &blackTS, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan blacklisted_tokens row: %w", err)
		}
		out = append(out, &authDomain.BlacklistedToken{
			JTI:                jti.String(),
			UserID:             userID,
			ExpiresAt:          expiresAt,
			RevokedAt:          revokedAt,
			Reason:             reason,
			TokenType:          tokenType,
			BlacklistTimestamp: blackTS,
			CreatedAt:          createdAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blacklisted_tokens rows: %w", err)
	}
	return out, nil
}
