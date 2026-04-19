package auth

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/pagination"
)

// auditLogColumns is the canonical SELECT list. It mirrors the field order
// of authDomain.AuditLog (struct tags map by name, so order is cosmetic —
// but keeping a single source of truth makes the intent clear).
var auditLogColumns = []string{
	"id",
	"user_id",
	"organization_id",
	"action",
	"resource",
	"resource_id",
	"metadata",
	"ip_address",
	"user_agent",
	"created_at",
}

// GetByFilters is the only audit-log read that needs a dynamic WHERE —
// Stripe-style admin search tools let operators combine any subset of
// (user, org, action, resource, resource_id, IP, date range) in a single
// call. Squirrel handles the composition; the paginated rows and a total
// count sharing the same WHERE are returned together.
//
// The domain AuditLog mirrors the Postgres schema exactly (nullable
// columns are pointers, metadata is json.RawMessage), so pgx scans
// directly into the struct via RowToAddrOfStructByNameLax — zero adapter
// code. COUNT(*) is int64 end to end because audit tables grow past 2B
// rows in successful products.
func (r *auditLogRepository) GetByFilters(ctx context.Context, filters *authDomain.AuditLogFilters) ([]*authDomain.AuditLog, int64, error) {
	countSQL, countArgs, err := applyAuditLogFilterWhere(
		sq.Select("COUNT(*)").From("audit_logs"),
		filters,
	).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build audit_logs count query: %w", err)
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit_logs by filter: %w", err)
	}

	dataSQL, args, err := applyAuditLogFilterPaging(
		applyAuditLogFilterSort(
			applyAuditLogFilterWhere(
				sq.Select(auditLogColumns...).From("audit_logs"),
				filters,
			),
			filters,
		),
		filters,
	).PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build audit_logs filter query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit_logs by filter: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByNameLax[authDomain.AuditLog])
	if err != nil {
		return nil, 0, fmt.Errorf("collect audit_logs rows: %w", err)
	}
	return out, total, nil
}

func applyAuditLogFilterWhere(q sq.SelectBuilder, filters *authDomain.AuditLogFilters) sq.SelectBuilder {
	if filters == nil {
		return q
	}
	if filters.UserID != nil {
		q = q.Where(sq.Eq{"user_id": *filters.UserID})
	}
	if filters.OrganizationID != nil {
		q = q.Where(sq.Eq{"organization_id": *filters.OrganizationID})
	}
	if filters.Action != nil && *filters.Action != "" {
		q = q.Where(sq.Eq{"action": *filters.Action})
	}
	if filters.Resource != nil && *filters.Resource != "" {
		q = q.Where(sq.Eq{"resource": *filters.Resource})
	}
	if filters.ResourceID != nil && *filters.ResourceID != "" {
		q = q.Where(sq.Eq{"resource_id": *filters.ResourceID})
	}
	if filters.IPAddress != nil && *filters.IPAddress != "" {
		q = q.Where(sq.Eq{"ip_address": *filters.IPAddress})
	}
	if filters.StartDate != nil {
		q = q.Where(sq.GtOrEq{"created_at": *filters.StartDate})
	}
	if filters.EndDate != nil {
		q = q.Where(sq.LtOrEq{"created_at": *filters.EndDate})
	}
	return q
}

func applyAuditLogFilterSort(q sq.SelectBuilder, filters *authDomain.AuditLogFilters) sq.SelectBuilder {
	allowed := []string{"created_at", "action", "ip_address", "user_agent", "id"}
	sortField := "created_at"
	sortDir := "DESC"
	if filters != nil {
		if filters.Params.SortBy != "" {
			if v, err := pagination.ValidateSortField(filters.Params.SortBy, allowed); err == nil && v != "" {
				sortField = v
			}
		}
		if filters.Params.SortDir == "asc" {
			sortDir = "ASC"
		}
	}
	return q.OrderBy(
		fmt.Sprintf("%s %s", sortField, sortDir),
		fmt.Sprintf("id %s", sortDir),
	)
}

func applyAuditLogFilterPaging(q sq.SelectBuilder, filters *authDomain.AuditLogFilters) sq.SelectBuilder {
	limit := uint64(pagination.DefaultPageSize)
	var offset uint64
	if filters != nil {
		if filters.Params.Limit > 0 {
			limit = uint64(filters.Params.Limit)
		}
		offset = uint64(filters.Params.GetOffset())
	}
	return q.Limit(limit).Offset(offset)
}
