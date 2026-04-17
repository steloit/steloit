package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// auditLogRepository is the pgx+sqlc implementation of
// authDomain.AuditLogRepository. Dynamic-filter reads live in
// audit_log_filter.go (squirrel); aggregate stats live in
// audit_log_stats.go.
type auditLogRepository struct {
	tm *db.TxManager
}

// NewAuditLogRepository returns the pgx-backed repository.
func NewAuditLogRepository(tm *db.TxManager) authDomain.AuditLogRepository {
	return &auditLogRepository{tm: tm}
}

func (r *auditLogRepository) Create(ctx context.Context, log *authDomain.AuditLog) error {
	meta := json.RawMessage(log.Metadata)
	if len(meta) == 0 {
		meta = nil
	}
	if err := r.tm.Queries(ctx).CreateAuditLog(ctx, gen.CreateAuditLogParams{
		ID:             log.ID,
		UserID:         log.UserID,
		OrganizationID: log.OrganizationID,
		Action:         log.Action,
		Resource:       log.Resource,
		ResourceID:     emptyToNilString(log.ResourceID),
		Metadata:       meta,
		IpAddress:      emptyToNilString(log.IPAddress),
		UserAgent:      emptyToNilString(log.UserAgent),
		CreatedAt:      log.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create audit_log: %w", err)
	}
	return nil
}

func (r *auditLogRepository) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.AuditLog, error) {
	row, err := r.tm.Queries(ctx).GetAuditLogByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get audit_log by ID %s: %w", id, authDomain.ErrNotFound)
		}
		return nil, fmt.Errorf("get audit_log by ID %s: %w", id, err)
	}
	return auditLogFromRow(&row), nil
}

func (r *auditLogRepository) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*authDomain.AuditLog, error) {
	rows, err := r.tm.Queries(ctx).ListAuditLogsByUser(ctx, gen.ListAuditLogsByUserParams{
		UserID: &userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit_logs for user %s: %w", userID, err)
	}
	return auditLogsFromRows(rows), nil
}

func (r *auditLogRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]*authDomain.AuditLog, error) {
	rows, err := r.tm.Queries(ctx).ListAuditLogsByOrganization(ctx, gen.ListAuditLogsByOrganizationParams{
		OrganizationID: &orgID,
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit_logs for org %s: %w", orgID, err)
	}
	return auditLogsFromRows(rows), nil
}

func (r *auditLogRepository) GetByResource(ctx context.Context, resource, resourceID string, limit, offset int) ([]*authDomain.AuditLog, error) {
	rows, err := r.tm.Queries(ctx).ListAuditLogsByResource(ctx, gen.ListAuditLogsByResourceParams{
		Resource:   resource,
		ResourceID: emptyToNilString(resourceID),
		Limit:      int32(limit),
		Offset:     int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit_logs for resource %s/%s: %w", resource, resourceID, err)
	}
	return auditLogsFromRows(rows), nil
}

func (r *auditLogRepository) GetByAction(ctx context.Context, action string, limit, offset int) ([]*authDomain.AuditLog, error) {
	rows, err := r.tm.Queries(ctx).ListAuditLogsByAction(ctx, gen.ListAuditLogsByActionParams{
		Action: action,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit_logs by action %s: %w", action, err)
	}
	return auditLogsFromRows(rows), nil
}

func (r *auditLogRepository) GetByDateRange(ctx context.Context, startDate, endDate time.Time, limit, offset int) ([]*authDomain.AuditLog, error) {
	rows, err := r.tm.Queries(ctx).ListAuditLogsByDateRange(ctx, gen.ListAuditLogsByDateRangeParams{
		CreatedAt:   startDate,
		CreatedAt_2: endDate,
		Limit:       int32(limit),
		Offset:      int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list audit_logs by date range: %w", err)
	}
	return auditLogsFromRows(rows), nil
}

// Search is a domain alias that delegates to GetByFilters.
func (r *auditLogRepository) Search(ctx context.Context, filters *authDomain.AuditLogFilters) ([]*authDomain.AuditLog, int, error) {
	return r.GetByFilters(ctx, filters)
}

func (r *auditLogRepository) CleanupOldLogs(ctx context.Context, olderThan time.Time) error {
	if _, err := r.tm.Queries(ctx).CleanupAuditLogsOlderThan(ctx, olderThan); err != nil {
		return fmt.Errorf("cleanup audit_logs older than %s: %w", olderThan, err)
	}
	return nil
}

// ----- gen ↔ domain boundary -----------------------------------------

// auditLogFromRow adapts a sqlc-generated row to the domain type. The
// domain surface has always used `string` for nullable text columns, so
// NULLs are surfaced as "".
func auditLogFromRow(row *gen.AuditLog) *authDomain.AuditLog {
	return &authDomain.AuditLog{
		ID:             row.ID,
		UserID:         row.UserID,
		OrganizationID: row.OrganizationID,
		Action:         row.Action,
		Resource:       row.Resource,
		ResourceID:     derefString(row.ResourceID),
		Metadata:       string(row.Metadata),
		IPAddress:      derefString(row.IpAddress),
		UserAgent:      derefString(row.UserAgent),
		CreatedAt:      row.CreatedAt,
	}
}

func auditLogsFromRows(rows []gen.AuditLog) []*authDomain.AuditLog {
	out := make([]*authDomain.AuditLog, 0, len(rows))
	for i := range rows {
		out = append(out, auditLogFromRow(&rows[i]))
	}
	return out
}

func emptyToNilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
