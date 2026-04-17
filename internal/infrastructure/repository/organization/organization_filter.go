package organization

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/pkg/pagination"
)

// List is the dynamic-filter listing. OrganizationFilters has three
// optional predicates (name ILIKE, plan, subscription_status) plus
// pagination — squirrel keeps the WHERE readable and partial-index
// friendly.
//
// Historical note: the filter struct names its status field Status but
// it maps onto the subscription_status column. The legacy GORM code
// referenced a non-existent `status` column and would have failed at
// runtime; fixed here.
func (r *organizationRepository) List(ctx context.Context, filters *orgDomain.OrganizationFilters) ([]*orgDomain.Organization, error) {
	q := sq.Select(
		"id",
		"name",
		"billing_email",
		"plan",
		"subscription_status",
		"trial_ends_at",
		"created_at",
		"updated_at",
		"deleted_at",
	).From("organizations").Where("deleted_at IS NULL")

	if filters != nil {
		if filters.Name != nil {
			q = q.Where(sq.ILike{"name": "%" + *filters.Name + "%"})
		}
		if filters.Plan != nil {
			q = q.Where(sq.Eq{"plan": *filters.Plan})
		}
		if filters.Status != nil {
			q = q.Where(sq.Eq{"subscription_status": *filters.Status})
		}
	}

	allowed := []string{"created_at", "updated_at", "name", "id"}
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
	q = q.OrderBy(
		fmt.Sprintf("%s %s", sortField, sortDir),
		fmt.Sprintf("id %s", sortDir),
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
		return nil, fmt.Errorf("build organizations filter query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query organizations by filter: %w", err)
	}
	defer rows.Close()

	var out []*orgDomain.Organization
	for rows.Next() {
		o := &orgDomain.Organization{}
		var (
			billingEmail *string
			deletedAt    *time.Time
		)
		if err := rows.Scan(
			&o.ID,
			&o.Name,
			&billingEmail,
			&o.Plan,
			&o.SubscriptionStatus,
			&o.TrialEndsAt,
			&o.CreatedAt,
			&o.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan organizations row: %w", err)
		}
		o.BillingEmail = derefString(billingEmail)
		o.DeletedAt = deletedAt
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate organizations rows: %w", err)
	}
	return out, nil
}
