package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// passwordResetTokenRepository is the pgx+sqlc implementation of
// authDomain.PasswordResetTokenRepository.
type passwordResetTokenRepository struct {
	tm *db.TxManager
}

// NewPasswordResetTokenRepository returns the repository backed by the
// shared TxManager so all queries participate in ctx-scoped transactions.
func NewPasswordResetTokenRepository(tm *db.TxManager) authDomain.PasswordResetTokenRepository {
	return &passwordResetTokenRepository{tm: tm}
}

func (r *passwordResetTokenRepository) Create(ctx context.Context, token *authDomain.PasswordResetToken) error {
	if err := r.tm.Queries(ctx).CreatePasswordResetToken(ctx, gen.CreatePasswordResetTokenParams{
		ID:        token.ID,
		UserID:    token.UserID,
		Token:     token.Token,
		ExpiresAt: token.ExpiresAt,
		UsedAt:    token.UsedAt,
		CreatedAt: token.CreatedAt,
		UpdatedAt: token.UpdatedAt,
	}); err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

func (r *passwordResetTokenRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.PasswordResetToken, error) {
	row, err := r.tm.Queries(ctx).GetPasswordResetTokenByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get password reset token: %w", authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get password reset token %s: %w", id, err)
	}
	return passwordResetTokenFromRow(&row), nil
}

func (r *passwordResetTokenRepository) GetByToken(ctx context.Context, tokenStr string) (*authDomain.PasswordResetToken, error) {
	row, err := r.tm.Queries(ctx).GetPasswordResetTokenByToken(ctx, tokenStr)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get password reset token: %w", authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get password reset token by token: %w", err)
	}
	return passwordResetTokenFromRow(&row), nil
}

func (r *passwordResetTokenRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.PasswordResetToken, error) {
	rows, err := r.tm.Queries(ctx).ListPasswordResetTokensByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list password reset tokens by user %s: %w", userID, err)
	}
	out := make([]*authDomain.PasswordResetToken, 0, len(rows))
	for i := range rows {
		out = append(out, passwordResetTokenFromRow(&rows[i]))
	}
	return out, nil
}

func (r *passwordResetTokenRepository) Update(ctx context.Context, token *authDomain.PasswordResetToken) error {
	if err := r.tm.Queries(ctx).UpdatePasswordResetToken(ctx, gen.UpdatePasswordResetTokenParams{
		ID:        token.ID,
		Token:     token.Token,
		ExpiresAt: token.ExpiresAt,
		UsedAt:    token.UsedAt,
	}); err != nil {
		return fmt.Errorf("update password reset token %s: %w", token.ID, err)
	}
	return nil
}

func (r *passwordResetTokenRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeletePasswordResetToken(ctx, id); err != nil {
		return fmt.Errorf("delete password reset token %s: %w", id, err)
	}
	return nil
}

func (r *passwordResetTokenRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkPasswordResetTokenAsUsed(ctx, id); err != nil {
		return fmt.Errorf("mark password reset token %s used: %w", id, err)
	}
	return nil
}

func (r *passwordResetTokenRepository) IsUsed(ctx context.Context, id uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsPasswordResetTokenUsed(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check password reset token %s used: %w", id, err)
	}
	return ok, nil
}

func (r *passwordResetTokenRepository) IsValid(ctx context.Context, id uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsPasswordResetTokenValid(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check password reset token %s valid: %w", id, err)
	}
	return ok, nil
}

func (r *passwordResetTokenRepository) GetValidTokenByUserID(ctx context.Context, userID uuid.UUID) (*authDomain.PasswordResetToken, error) {
	row, err := r.tm.Queries(ctx).GetValidPasswordResetTokenByUser(ctx, userID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get valid password reset token for user %s: %w", userID, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get valid password reset token for user %s: %w", userID, err)
	}
	return passwordResetTokenFromRow(&row), nil
}

func (r *passwordResetTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	if _, err := r.tm.Queries(ctx).CleanupExpiredPasswordResetTokens(ctx); err != nil {
		return fmt.Errorf("cleanup expired password reset tokens: %w", err)
	}
	return nil
}

func (r *passwordResetTokenRepository) CleanupUsedTokens(ctx context.Context, olderThan time.Time) error {
	if _, err := r.tm.Queries(ctx).CleanupUsedPasswordResetTokens(ctx, olderThan); err != nil {
		return fmt.Errorf("cleanup used password reset tokens older than %s: %w", olderThan, err)
	}
	return nil
}

func (r *passwordResetTokenRepository) InvalidateAllUserTokens(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).InvalidateUserPasswordResetTokens(ctx, userID); err != nil {
		return fmt.Errorf("invalidate password reset tokens for user %s: %w", userID, err)
	}
	return nil
}

// passwordResetTokenFromRow adapts a sqlc-generated row to the domain type.
// The domain struct still carries a legacy `Used bool` field (dead — the
// schema dropped the column in migration 20250906113026); it's left unset
// here. The field will be removed alongside other domain cleanup in P1.9.
func passwordResetTokenFromRow(row *gen.PasswordResetToken) *authDomain.PasswordResetToken {
	return &authDomain.PasswordResetToken{
		ID:        row.ID,
		UserID:    row.UserID,
		Token:     row.Token,
		ExpiresAt: row.ExpiresAt,
		UsedAt:    row.UsedAt,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
