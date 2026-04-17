package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// GetAuditLogStats returns platform-wide audit statistics: total count,
// per-action and per-resource histograms, and the most recent event time.
// Three round trips; callers invoke this infrequently (admin dashboards).
func (r *auditLogRepository) GetAuditLogStats(ctx context.Context) (*authDomain.AuditLogStats, error) {
	return r.auditLogStats(ctx, nil, nil)
}

func (r *auditLogRepository) GetUserAuditLogStats(ctx context.Context, userID uuid.UUID) (*authDomain.AuditLogStats, error) {
	id := userID
	return r.auditLogStats(ctx, &id, nil)
}

func (r *auditLogRepository) GetOrganizationAuditLogStats(ctx context.Context, orgID uuid.UUID) (*authDomain.AuditLogStats, error) {
	id := orgID
	return r.auditLogStats(ctx, nil, &id)
}

// auditLogStats computes the three histograms a stats call produces.
// Exactly one of userID / orgID may be non-nil; passing both constrains
// the aggregates to the intersection (useful for platform admins).
func (r *auditLogRepository) auditLogStats(ctx context.Context, userID, orgID *uuid.UUID) (*authDomain.AuditLogStats, error) {
	q := r.tm.Queries(ctx)
	stats := &authDomain.AuditLogStats{
		LogsByAction:   make(map[string]int64),
		LogsByResource: make(map[string]int64),
	}

	switch {
	case userID != nil:
		n, err := q.CountAuditLogsByUser(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("count audit_logs for user %s: %w", *userID, err)
		}
		stats.TotalLogs = n
	case orgID != nil:
		n, err := q.CountAuditLogsByOrganization(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("count audit_logs for org %s: %w", *orgID, err)
		}
		stats.TotalLogs = n
	default:
		n, err := q.CountAuditLogs(ctx)
		if err != nil {
			return nil, fmt.Errorf("count audit_logs: %w", err)
		}
		stats.TotalLogs = n
	}

	actions, err := q.CountAuditLogsByActionGroup(ctx, gen.CountAuditLogsByActionGroupParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("group audit_logs by action: %w", err)
	}
	for _, a := range actions {
		stats.LogsByAction[a.Action] = a.Count
	}

	resources, err := q.CountAuditLogsByResourceGroup(ctx, gen.CountAuditLogsByResourceGroupParams{
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, fmt.Errorf("group audit_logs by resource: %w", err)
	}
	for _, r := range resources {
		stats.LogsByResource[r.Resource] = r.Count
	}

	// Last-log-time is cheap; always include it when we can reach it. Only
	// populate when the stats are platform-wide so user/org views don't
	// leak cross-scope activity.
	if userID == nil && orgID == nil {
		latest, err := q.GetLatestAuditLogTime(ctx)
		if err != nil && !db.IsNoRows(err) {
			return nil, fmt.Errorf("latest audit_log time: %w", err)
		}
		if err == nil {
			t := latest
			stats.LastLogTime = &t
		}
	}

	return stats, nil
}
