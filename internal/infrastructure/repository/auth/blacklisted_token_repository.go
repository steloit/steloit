package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// blacklistedTokenRepository is the pgx+sqlc implementation of
// authDomain.BlacklistedTokenRepository. Static queries are sqlc-generated
// (internal/infrastructure/db/queries/blacklisted_token.sql); the single
// dynamic-filter method lives in blacklisted_token_filter.go and builds
// with squirrel.
type blacklistedTokenRepository struct {
	tm *db.TxManager
}

// NewBlacklistedTokenRepository returns a BlacklistedTokenRepository backed
// by the shared TxManager. The TxManager handles transaction propagation
// via ctx, so repository callers never need to pass a pgx.Tx explicitly.
func NewBlacklistedTokenRepository(tm *db.TxManager) authDomain.BlacklistedTokenRepository {
	return &blacklistedTokenRepository{tm: tm}
}

func (r *blacklistedTokenRepository) Create(ctx context.Context, token *authDomain.BlacklistedToken) error {
	jti, err := uuid.Parse(token.JTI)
	if err != nil {
		return fmt.Errorf("parse JTI %q: %w", token.JTI, err)
	}
	if err := r.tm.Queries(ctx).CreateBlacklistedToken(ctx, gen.CreateBlacklistedTokenParams{
		Jti:                jti,
		UserID:             token.UserID,
		ExpiresAt:          token.ExpiresAt,
		RevokedAt:          token.RevokedAt,
		Reason:             token.Reason,
		TokenType:          token.TokenType,
		BlacklistTimestamp: token.BlacklistTimestamp,
		CreatedAt:          token.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create blacklisted token: %w", err)
	}
	return nil
}

func (r *blacklistedTokenRepository) GetByJTI(ctx context.Context, jti string) (*authDomain.BlacklistedToken, error) {
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return nil, fmt.Errorf("parse JTI %q: %w", jti, err)
	}
	row, err := r.tm.Queries(ctx).GetBlacklistedTokenByJTI(ctx, jtiUUID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get blacklisted token by JTI %s: %w", jti, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get blacklisted token by JTI %s: %w", jti, err)
	}
	return blacklistedTokenFromRow(&row), nil
}

func (r *blacklistedTokenRepository) IsTokenBlacklisted(ctx context.Context, jti string) (bool, error) {
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return false, fmt.Errorf("parse JTI %q: %w", jti, err)
	}
	ok, err := r.tm.Queries(ctx).IsTokenBlacklisted(ctx, jtiUUID)
	if err != nil {
		return false, fmt.Errorf("check blacklisted token %s: %w", jti, err)
	}
	return ok, nil
}

func (r *blacklistedTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	if _, err := r.tm.Queries(ctx).CleanupExpiredBlacklistedTokens(ctx); err != nil {
		return fmt.Errorf("cleanup expired blacklisted tokens: %w", err)
	}
	return nil
}

func (r *blacklistedTokenRepository) CleanupTokensOlderThan(ctx context.Context, olderThan time.Time) error {
	if _, err := r.tm.Queries(ctx).CleanupBlacklistedTokensOlderThan(ctx, olderThan); err != nil {
		return fmt.Errorf("cleanup blacklisted tokens older than %s: %w", olderThan, err)
	}
	return nil
}

func (r *blacklistedTokenRepository) CreateUserTimestampBlacklist(ctx context.Context, userID uuid.UUID, blacklistTimestamp int64, reason string) error {
	entry := authDomain.NewUserTimestampBlacklistedToken(userID, blacklistTimestamp, reason)
	return r.Create(ctx, entry)
}

func (r *blacklistedTokenRepository) IsUserBlacklistedAfterTimestamp(ctx context.Context, userID uuid.UUID, tokenIssuedAt int64) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsUserBlacklistedAfterTimestamp(ctx, gen.IsUserBlacklistedAfterTimestampParams{
		UserID:        userID,
		TokenIssuedAt: tokenIssuedAt,
	})
	if err != nil {
		return false, fmt.Errorf("check user timestamp blacklist %s: %w", userID, err)
	}
	return ok, nil
}

func (r *blacklistedTokenRepository) GetUserBlacklistTimestamp(ctx context.Context, userID uuid.UUID) (*int64, error) {
	ts, err := r.tm.Queries(ctx).GetUserBlacklistTimestamp(ctx, userID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get user blacklist timestamp %s: %w", userID, err)
	}
	return ts, nil
}

// BlacklistUserTokens is retained for backward compatibility with the
// legacy interface; it delegates to CreateUserTimestampBlacklist, the only
// GDPR/SOC2-compliant primitive used by new call sites.
func (r *blacklistedTokenRepository) BlacklistUserTokens(ctx context.Context, userID uuid.UUID, reason string) error {
	return r.CreateUserTimestampBlacklist(ctx, userID, time.Now().Unix(), reason)
}

func (r *blacklistedTokenRepository) GetBlacklistedTokensCount(ctx context.Context) (int64, error) {
	count, err := r.tm.Queries(ctx).CountBlacklistedTokens(ctx)
	if err != nil {
		return 0, fmt.Errorf("count blacklisted tokens: %w", err)
	}
	return count, nil
}

func (r *blacklistedTokenRepository) GetBlacklistedTokensByReason(ctx context.Context, reason string) ([]*authDomain.BlacklistedToken, error) {
	rows, err := r.tm.Queries(ctx).ListBlacklistedTokensByReason(ctx, reason)
	if err != nil {
		return nil, fmt.Errorf("list blacklisted tokens by reason %q: %w", reason, err)
	}
	out := make([]*authDomain.BlacklistedToken, 0, len(rows))
	for i := range rows {
		out = append(out, blacklistedTokenFromRow(&rows[i]))
	}
	return out, nil
}

// blacklistedTokenFromRow converts a sqlc-generated gen.BlacklistedToken
// into the domain BlacklistedToken. Kept unexported — generated types must
// not escape the repository package.
func blacklistedTokenFromRow(row *gen.BlacklistedToken) *authDomain.BlacklistedToken {
	return &authDomain.BlacklistedToken{
		JTI:                row.Jti.String(),
		UserID:             row.UserID,
		ExpiresAt:          row.ExpiresAt,
		RevokedAt:          row.RevokedAt,
		Reason:             row.Reason,
		TokenType:          row.TokenType,
		BlacklistTimestamp: row.BlacklistTimestamp,
		CreatedAt:          row.CreatedAt,
	}
}

var _ = errors.Is // traversal helper used indirectly via db.IsNoRows
