package auth

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	authDomain "brokle/internal/core/domain/auth"
	"brokle/pkg/pagination"
)

// GetByFilters handles the dynamic-WHERE read path: callers may restrict by
// user, project, organization (via a JOIN to projects), and expired status,
// and may paginate/sort via APIKeyFilters.Params. The filter surface is
// larger than sqlc's narg/COALESCE pattern scales to cleanly, so this
// single method uses squirrel. Writes still go through sqlc in the
// companion api_key_repository.go.
func (r *apiKeyRepository) GetByFilters(ctx context.Context, filters *authDomain.APIKeyFilters) ([]*authDomain.APIKey, error) {
	q := sq.Select(
		"api_keys.id",
		"api_keys.user_id",
		"api_keys.project_id",
		"api_keys.name",
		"api_keys.key_hash",
		"api_keys.key_preview",
		"api_keys.expires_at",
		"api_keys.last_used_at",
		"api_keys.created_at",
		"api_keys.updated_at",
		"api_keys.deleted_at",
	).From("api_keys")
	q = applyAPIKeyFilterWhere(q, filters)
	q = applyAPIKeyFilterSort(q, filters)
	q = applyAPIKeyFilterPaging(q, filters)

	sqlStr, args, err := q.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, fmt.Errorf("build api_keys filter query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query api_keys by filter: %w", err)
	}
	defer rows.Close()

	var out []*authDomain.APIKey
	for rows.Next() {
		a := &authDomain.APIKey{}
		if err := rows.Scan(
			&a.ID,
			&a.UserID,
			&a.ProjectID,
			&a.Name,
			&a.KeyHash,
			&a.KeyPreview,
			&a.ExpiresAt,
			&a.LastUsedAt,
			&a.CreatedAt,
			&a.UpdatedAt,
			&a.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api_keys row: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api_keys rows: %w", err)
	}
	return out, nil
}

// CountByFilters returns the count matching the same filter set. Pagination
// parameters (limit/offset/sort) are ignored for counts.
func (r *apiKeyRepository) CountByFilters(ctx context.Context, filters *authDomain.APIKeyFilters) (int64, error) {
	q := sq.Select("COUNT(*)").From("api_keys")
	q = applyAPIKeyFilterWhere(q, filters)

	sqlStr, args, err := q.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return 0, fmt.Errorf("build api_keys count query: %w", err)
	}

	var count int64
	if err := r.tm.DB(ctx).QueryRow(ctx, sqlStr, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count api_keys by filter: %w", err)
	}
	return count, nil
}

func applyAPIKeyFilterWhere(q sq.SelectBuilder, filters *authDomain.APIKeyFilters) sq.SelectBuilder {
	q = q.Where(sq.Eq{"api_keys.deleted_at": nil})
	if filters == nil {
		return q
	}
	if filters.UserID != nil {
		q = q.Where(sq.Eq{"api_keys.user_id": *filters.UserID})
	}
	if filters.ProjectID != nil {
		q = q.Where(sq.Eq{"api_keys.project_id": *filters.ProjectID})
	}
	if filters.OrganizationID != nil {
		// api_keys.organization_id was dropped in 20251005150009; the only
		// remaining path to the org is through projects.
		q = q.Join("projects ON api_keys.project_id = projects.id").
			Where(sq.Eq{"projects.organization_id": *filters.OrganizationID})
	}
	if filters.IsExpired != nil {
		if *filters.IsExpired {
			q = q.Where(sq.And{
				sq.NotEq{"api_keys.expires_at": nil},
				sq.Expr("api_keys.expires_at < NOW()"),
			})
		} else {
			// Structured sq.Or is mandatory here: squirrel joins Where()
			// clauses with AND, and SQL AND binds tighter than OR, so a
			// raw "X IS NULL OR X > NOW()" string would let the OR branch
			// short-circuit the upstream scope filters (user/project/org)
			// and leak keys across tenants. The NOW() side uses sq.Expr
			// because sq.Gt/Lt bind the RHS as a parameter regardless of
			// type, which would send squirrel.expr{sql:"NOW()"} as a bind
			// value to pgx and fail at runtime.
			q = q.Where(sq.Or{
				sq.Eq{"api_keys.expires_at": nil},
				sq.Expr("api_keys.expires_at > NOW()"),
			})
		}
	}
	return q
}

func applyAPIKeyFilterSort(q sq.SelectBuilder, filters *authDomain.APIKeyFilters) sq.SelectBuilder {
	allowed := []string{"created_at", "updated_at", "name", "expires_at", "last_used_at", "id"}
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
		fmt.Sprintf("api_keys.%s %s", sortField, sortDir),
		fmt.Sprintf("api_keys.id %s", sortDir),
	)
}

func applyAPIKeyFilterPaging(q sq.SelectBuilder, filters *authDomain.APIKeyFilters) sq.SelectBuilder {
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

