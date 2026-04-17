package errors

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL SQLSTATE codes we care about.
// https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	pgCodeUniqueViolation     = "23505"
	pgCodeForeignKeyViolation = "23503"
)

// IsUniqueViolation reports whether err wraps a PostgreSQL unique
// constraint violation (SQLSTATE 23505). Matches by typed
// *pgconn.PgError — no string heuristics.
func IsUniqueViolation(err error) bool {
	return hasPgCode(err, pgCodeUniqueViolation)
}

// IsForeignKeyViolation reports whether err wraps a PostgreSQL
// foreign key constraint violation (SQLSTATE 23503).
func IsForeignKeyViolation(err error) bool {
	return hasPgCode(err, pgCodeForeignKeyViolation)
}

func hasPgCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}
