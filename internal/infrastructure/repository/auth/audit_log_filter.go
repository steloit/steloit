package auth

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/pagination"
)

// GetByFilters is the only audit-log read that needs a dynamic WHERE —
// Stripe-style admin search tools let operators combine any subset of
// (user, org, action, resource, resource_id, IP, date range) in a single
// call. Squirrel handles the composition; the GetByFilters result is
// returned alongside a total count computed with the same WHERE for
// cursor-less pagination.
func (r *auditLogRepository) GetByFilters(ctx context.Context, filters *authDomain.AuditLogFilters) ([]*authDomain.AuditLog, int, error) {
	countBuilder := sq.Select("COUNT(*)").From("audit_logs")
	countBuilder = applyAuditLogFilterWhere(countBuilder, filters)
	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build audit_logs count query: %w", err)
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit_logs by filter: %w", err)
	}

	dataBuilder := sq.Select(
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
	).From("audit_logs")
	dataBuilder = applyAuditLogFilterWhere(dataBuilder, filters)
	dataBuilder = applyAuditLogFilterSort(dataBuilder, filters)
	dataBuilder = applyAuditLogFilterPaging(dataBuilder, filters)

	sqlStr, args, err := dataBuilder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build audit_logs filter query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query audit_logs by filter: %w", err)
	}
	defer rows.Close()

	var out []*authDomain.AuditLog
	for rows.Next() {
		var (
			id             any
			userID         *any
			organizationID *any
			action         string
			resource       string
			resourceID     *string
			metadata       []byte
			ipAddress      *string
			userAgent      *string
			createdAt      = new(any)
		)
		// Scan into the domain struct directly via pointers to its fields.
		log := &authDomain.AuditLog{}
		if err := rows.Scan(
			&log.ID,
			&log.UserID,
			&log.OrganizationID,
			&action,
			&resource,
			&resourceID,
			&metadata,
			&ipAddress,
			&userAgent,
			&log.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan audit_logs row: %w", err)
		}
		log.Action = action
		log.Resource = resource
		log.ResourceID = derefString(resourceID)
		log.Metadata = string(metadata)
		log.IPAddress = derefString(ipAddress)
		log.UserAgent = derefString(userAgent)
		out = append(out, log)
		_ = id
		_ = userID
		_ = organizationID
		_ = createdAt
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit_logs rows: %w", err)
	}
	return out, int(total), nil
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
