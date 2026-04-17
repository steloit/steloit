package auth

import (
	"context"
	"fmt"

	sq "github.com/Masterminds/squirrel"

	authDomain "brokle/internal/core/domain/auth"
)

// ListPermissions is the paginated plain list. Because there is no
// search filter it could have been a sqlc query, but the domain method
// returns (items, total, error) and we want to keep the count and data
// queries adjacent here for clarity. Both go through TxManager.DB(ctx)
// so they still honour any outer transaction.
func (r *permissionRepository) ListPermissions(ctx context.Context, limit, offset int) ([]*authDomain.Permission, int, error) {
	return r.runPermissionSearch(ctx, "", limit, offset)
}

// SearchPermissions runs a case-insensitive substring search over the
// name, description, resource, action, and category columns, returning
// one page of results plus the total matching count. Composed with
// squirrel because the LIKE expression varies per call and sqlc's named
// parameters can't express ILIKE over multiple columns cleanly.
func (r *permissionRepository) SearchPermissions(ctx context.Context, query string, limit, offset int) ([]*authDomain.Permission, int, error) {
	return r.runPermissionSearch(ctx, query, limit, offset)
}

func (r *permissionRepository) runPermissionSearch(ctx context.Context, query string, limit, offset int) ([]*authDomain.Permission, int, error) {
	countBuilder := sq.Select("COUNT(*)").From("permissions")
	dataBuilder := sq.Select("*").From("permissions")

	if query != "" {
		pattern := "%" + query + "%"
		like := sq.Or{
			sq.ILike{"name": pattern},
			sq.ILike{"description": pattern},
			sq.ILike{"resource": pattern},
			sq.ILike{"action": pattern},
			sq.ILike{"category": pattern},
		}
		countBuilder = countBuilder.Where(like)
		dataBuilder = dataBuilder.Where(like)
	}

	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build permissions count query: %w", err)
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count permissions: %w", err)
	}

	// Caller controls the limit; GORM code fetched +1 to detect has_next,
	// but sqlc repositories have been returning the exact page size
	// consistently — keep that contract here.
	if limit <= 0 {
		limit = 50
	}
	dataBuilder = dataBuilder.
		OrderBy("category ASC NULLS LAST", "resource ASC", "action ASC", "id ASC").
		Limit(uint64(limit)).
		Offset(uint64(offset))

	sqlStr, args, err := dataBuilder.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("build permissions search query: %w", err)
	}

	rows, err := r.tm.DB(ctx).Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query permissions search: %w", err)
	}
	defer rows.Close()

	var out []*authDomain.Permission
	for rows.Next() {
		p := &authDomain.Permission{}
		var description, category *string
		var scopeLevel string
		if err := rows.Scan(
			&p.ID,
			&p.Name,
			&description,
			&p.Resource,
			&p.Action,
			&p.CreatedAt,
			&scopeLevel,
			&category,
		); err != nil {
			return nil, 0, fmt.Errorf("scan permissions row: %w", err)
		}
		p.Description = derefString(description)
		p.ScopeLevel = authDomain.ScopeLevel(scopeLevel)
		p.Category = derefString(category)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate permissions rows: %w", err)
	}
	return out, int(total), nil
}
