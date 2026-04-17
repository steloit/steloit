package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// userSessionRepository is the pgx+sqlc implementation of
// authDomain.UserSessionRepository.
type userSessionRepository struct {
	tm *db.TxManager
}

// NewUserSessionRepository returns the pgx-backed repository.
func NewUserSessionRepository(tm *db.TxManager) authDomain.UserSessionRepository {
	return &userSessionRepository{tm: tm}
}

func (r *userSessionRepository) Create(ctx context.Context, session *authDomain.UserSession) error {
	params, err := buildCreateUserSessionParams(session)
	if err != nil {
		return fmt.Errorf("build create user_session params: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateUserSession(ctx, params); err != nil {
		return fmt.Errorf("create user_session: %w", err)
	}
	return nil
}

func (r *userSessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.UserSession, error) {
	row, err := r.tm.Queries(ctx).GetUserSessionByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get user session by ID %s: %w", id, authDomain.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("get user session by ID %s: %w", id, err)
	}
	return userSessionFromRow(&row), nil
}

func (r *userSessionRepository) GetByJTI(ctx context.Context, jti string) (*authDomain.UserSession, error) {
	jtiUUID, err := uuid.Parse(jti)
	if err != nil {
		return nil, fmt.Errorf("parse JTI %q: %w", jti, err)
	}
	row, err := r.tm.Queries(ctx).GetUserSessionByJTI(ctx, jtiUUID)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get user session by JTI %s: %w", jti, authDomain.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("get user session by JTI %s: %w", jti, err)
	}
	return userSessionFromRow(&row), nil
}

func (r *userSessionRepository) GetByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (*authDomain.UserSession, error) {
	row, err := r.tm.Queries(ctx).GetUserSessionByRefreshTokenHash(ctx, refreshTokenHash)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get user session by refresh token hash: %w", authDomain.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("get user session by refresh token hash: %w", err)
	}
	return userSessionFromRow(&row), nil
}

func (r *userSessionRepository) Update(ctx context.Context, session *authDomain.UserSession) error {
	params, err := buildUpdateUserSessionParams(session)
	if err != nil {
		return fmt.Errorf("build update user_session params: %w", err)
	}
	if err := r.tm.Queries(ctx).UpdateUserSession(ctx, params); err != nil {
		return fmt.Errorf("update user_session %s: %w", session.ID, err)
	}
	return nil
}

func (r *userSessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeleteUserSession(ctx, id); err != nil {
		return fmt.Errorf("delete user_session %s: %w", id, err)
	}
	return nil
}

func (r *userSessionRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	rows, err := r.tm.Queries(ctx).ListUserSessionsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user sessions for user %s: %w", userID, err)
	}
	return userSessionsFromRows(rows), nil
}

func (r *userSessionRepository) GetActiveSessionsByUserID(ctx context.Context, userID uuid.UUID) ([]*authDomain.UserSession, error) {
	rows, err := r.tm.Queries(ctx).ListActiveUserSessionsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list active user sessions for user %s: %w", userID, err)
	}
	return userSessionsFromRows(rows), nil
}

func (r *userSessionRepository) DeactivateSession(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).DeactivateUserSession(ctx, id); err != nil {
		return fmt.Errorf("deactivate user_session %s: %w", id, err)
	}
	return nil
}

func (r *userSessionRepository) DeactivateUserSessions(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).DeactivateUserSessionsForUser(ctx, userID); err != nil {
		return fmt.Errorf("deactivate user_sessions for user %s: %w", userID, err)
	}
	return nil
}

func (r *userSessionRepository) RevokeSession(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).RevokeUserSession(ctx, id); err != nil {
		return fmt.Errorf("revoke user_session %s: %w", id, err)
	}
	return nil
}

func (r *userSessionRepository) RevokeUserSessions(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.tm.Queries(ctx).RevokeUserSessionsForUser(ctx, userID); err != nil {
		return fmt.Errorf("revoke user_sessions for user %s: %w", userID, err)
	}
	return nil
}

func (r *userSessionRepository) CleanupExpiredSessions(ctx context.Context) error {
	if _, err := r.tm.Queries(ctx).CleanupExpiredUserSessions(ctx); err != nil {
		return fmt.Errorf("cleanup expired user_sessions: %w", err)
	}
	return nil
}

func (r *userSessionRepository) CleanupRevokedSessions(ctx context.Context) error {
	if _, err := r.tm.Queries(ctx).CleanupRevokedUserSessions(ctx); err != nil {
		return fmt.Errorf("cleanup revoked user_sessions: %w", err)
	}
	return nil
}

func (r *userSessionRepository) MarkAsUsed(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkUserSessionUsed(ctx, id); err != nil {
		return fmt.Errorf("mark user_session %s used: %w", id, err)
	}
	return nil
}

func (r *userSessionRepository) GetByDeviceInfo(ctx context.Context, userID uuid.UUID, deviceInfo interface{}) ([]*authDomain.UserSession, error) {
	raw, err := json.Marshal(deviceInfo)
	if err != nil {
		return nil, fmt.Errorf("marshal device info: %w", err)
	}
	rows, err := r.tm.Queries(ctx).ListUserSessionsByDeviceInfo(ctx, gen.ListUserSessionsByDeviceInfoParams{
		UserID:     userID,
		DeviceInfo: raw,
	})
	if err != nil {
		return nil, fmt.Errorf("list user sessions by device info for user %s: %w", userID, err)
	}
	return userSessionsFromRows(rows), nil
}

func (r *userSessionRepository) GetActiveSessionsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := r.tm.Queries(ctx).CountActiveUserSessions(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("count active user_sessions for user %s: %w", userID, err)
	}
	return int(n), nil
}

// ----- gen ↔ domain conversion ---------------------------------------

func buildCreateUserSessionParams(s *authDomain.UserSession) (gen.CreateUserSessionParams, error) {
	currentJTI, err := uuid.Parse(s.CurrentJTI)
	if err != nil {
		return gen.CreateUserSessionParams{}, fmt.Errorf("parse current JTI %q: %w", s.CurrentJTI, err)
	}
	ip, err := parseIPAddr(s.IPAddress)
	if err != nil {
		return gen.CreateUserSessionParams{}, err
	}
	device, err := marshalDeviceInfo(s.DeviceInfo)
	if err != nil {
		return gen.CreateUserSessionParams{}, err
	}
	return gen.CreateUserSessionParams{
		ID:                  s.ID,
		UserID:              s.UserID,
		RefreshTokenHash:    s.RefreshTokenHash,
		RefreshTokenVersion: int32(s.RefreshTokenVersion),
		CurrentJti:          currentJTI,
		ExpiresAt:           s.ExpiresAt,
		RefreshExpiresAt:    s.RefreshExpiresAt,
		IpAddress:           ip,
		UserAgent:           s.UserAgent,
		DeviceInfo:          device,
		IsActive:            s.IsActive,
		LastUsedAt:          s.LastUsedAt,
		RevokedAt:           s.RevokedAt,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}, nil
}

func buildUpdateUserSessionParams(s *authDomain.UserSession) (gen.UpdateUserSessionParams, error) {
	currentJTI, err := uuid.Parse(s.CurrentJTI)
	if err != nil {
		return gen.UpdateUserSessionParams{}, fmt.Errorf("parse current JTI %q: %w", s.CurrentJTI, err)
	}
	ip, err := parseIPAddr(s.IPAddress)
	if err != nil {
		return gen.UpdateUserSessionParams{}, err
	}
	device, err := marshalDeviceInfo(s.DeviceInfo)
	if err != nil {
		return gen.UpdateUserSessionParams{}, err
	}
	return gen.UpdateUserSessionParams{
		ID:                  s.ID,
		RefreshTokenHash:    s.RefreshTokenHash,
		RefreshTokenVersion: int32(s.RefreshTokenVersion),
		CurrentJti:          currentJTI,
		ExpiresAt:           s.ExpiresAt,
		RefreshExpiresAt:    s.RefreshExpiresAt,
		IpAddress:           ip,
		UserAgent:           s.UserAgent,
		DeviceInfo:          device,
		IsActive:            s.IsActive,
		LastUsedAt:          s.LastUsedAt,
		RevokedAt:           s.RevokedAt,
	}, nil
}

func userSessionFromRow(row *gen.UserSession) *authDomain.UserSession {
	return &authDomain.UserSession{
		ID:                  row.ID,
		UserID:              row.UserID,
		RefreshTokenHash:    row.RefreshTokenHash,
		RefreshTokenVersion: int(row.RefreshTokenVersion),
		CurrentJTI:          row.CurrentJti.String(),
		ExpiresAt:           row.ExpiresAt,
		RefreshExpiresAt:    row.RefreshExpiresAt,
		IPAddress:           ipAddrToString(row.IpAddress),
		UserAgent:           row.UserAgent,
		DeviceInfo:          decodeDeviceInfo(row.DeviceInfo),
		IsActive:            row.IsActive,
		LastUsedAt:          row.LastUsedAt,
		RevokedAt:           row.RevokedAt,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}

func userSessionsFromRows(rows []gen.UserSession) []*authDomain.UserSession {
	out := make([]*authDomain.UserSession, 0, len(rows))
	for i := range rows {
		out = append(out, userSessionFromRow(&rows[i]))
	}
	return out
}

// parseIPAddr converts the domain's *string IP into pgx's *netip.Addr.
// Returns (nil, nil) when the domain value is nil/empty.
func parseIPAddr(ip *string) (*netip.Addr, error) {
	if ip == nil || *ip == "" {
		return nil, nil
	}
	addr, err := netip.ParseAddr(*ip)
	if err != nil {
		return nil, fmt.Errorf("parse ip %q: %w", *ip, err)
	}
	return &addr, nil
}

func ipAddrToString(ip *netip.Addr) *string {
	if ip == nil || !ip.IsValid() {
		return nil
	}
	s := ip.String()
	return &s
}

// marshalDeviceInfo converts the opaque interface{} domain field into
// json.RawMessage for the sqlc-generated params. Empty/nil in = null on
// the wire; the JSONB column is nullable.
func marshalDeviceInfo(v interface{}) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	// If the caller already passes json.RawMessage / []byte, use it directly.
	switch raw := v.(type) {
	case json.RawMessage:
		if len(raw) == 0 {
			return nil, nil
		}
		return raw, nil
	case []byte:
		if len(raw) == 0 {
			return nil, nil
		}
		return raw, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal device info: %w", err)
	}
	// Collapse "null" to SQL NULL so the column stays clean.
	if string(data) == "null" {
		return nil, nil
	}
	return data, nil
}

// decodeDeviceInfo restores the domain's interface{} field from the raw
// JSONB bytes. The domain has never relied on a concrete type here, so
// we return the decoded Go value (map/slice/literal) rather than keeping
// the bytes.
func decodeDeviceInfo(raw json.RawMessage) interface{} {
	if len(raw) == 0 {
		return nil
	}
	var out interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		// Surface the raw payload on decode failure so callers can still
		// audit it; swallowing would lose forensic data.
		return string(raw)
	}
	return out
}

// _ = time.Time used only through generated code; keep the import stable.
var _ = time.Time{}
