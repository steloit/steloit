package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"brokle/internal/core/domain/common"
	"brokle/internal/infrastructure/db/gen"
)

// pgxTxKey is the unexported context key used to propagate an active
// pgx.Tx from a service's WithinTransaction callback down into every
// repository call that runs under it.
type pgxTxKey struct{}

// DBTX is the common execution surface shared by *pgxpool.Pool and pgx.Tx.
// Repositories that build dynamic SQL (squirrel, filter_*.go) take a DBTX
// from TxManager.DB(ctx) so the same code path runs transactional or not.
// It mirrors sqlc's generated DBTX interface so the two can coexist once
// Phase 1 lands sqlc output.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
	CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error)
}

// TxManager is the single persistence entrypoint for services and
// repositories.
//
// Responsibilities:
//
//   - Services call WithinTransaction(ctx, fn) to group work atomically.
//     The *TxManager satisfies the existing common.Transactor interface,
//     so service constructors do not change during the migration.
//   - Repositories call DB(ctx) to obtain a DBTX that automatically tracks
//     whether the caller is inside a transaction.
//
// Reentrancy semantics: FLATTEN — nested WithinTransaction calls reuse
// the outer transaction. This matches SigNoz, Grafana, and Three Dots
// Labs, and is simpler than GORM's implicit SAVEPOINT behaviour. A codebase
// audit confirmed zero nested call sites exist, so the change is
// behaviour-preserving. If partial rollback is ever needed, open a
// SAVEPOINT at the call site explicitly rather than globally changing
// reentrancy semantics here.
type TxManager struct {
	pool *pgxpool.Pool
	q    *gen.Queries // bound to the pool, cloned via WithTx inside a transaction
}

// NewTxManager wraps a pgxpool.Pool in a TxManager and prepares the
// sqlc-generated *Queries bound to the pool's default execution context.
func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{
		pool: pool,
		q:    gen.New(pool),
	}
}

// DB returns the pgx execution surface for the current context. If the
// context was created by a surrounding WithinTransaction call, this is the
// tx-scoped pgx.Tx; otherwise it's the pool itself.
//
// Use this from repositories that build SQL dynamically (filter_*.go via
// squirrel) or for one-off queries outside the sqlc-generated API.
func (m *TxManager) DB(ctx context.Context) DBTX {
	if tx, ok := ctx.Value(pgxTxKey{}).(pgx.Tx); ok {
		return tx
	}
	return m.pool
}

// Queries returns the sqlc-generated *Queries for the current context. If
// the context carries an active pgx.Tx, the returned *Queries is bound to
// that transaction (via gen.Queries.WithTx); otherwise the pool-bound
// *Queries is returned.
//
// This is the tx-aware entrypoint every repository uses for static queries
// that sqlc generated from queries/*.sql. Pair with DB(ctx) when a
// repository also has dynamic queries built by squirrel.
func (m *TxManager) Queries(ctx context.Context) *gen.Queries {
	if tx, ok := ctx.Value(pgxTxKey{}).(pgx.Tx); ok {
		return m.q.WithTx(tx)
	}
	return m.q
}

// WithinTransaction runs fn inside a transaction. If ctx already carries an
// active transaction (nested call), fn is invoked with the outer transaction
// — outer commit/rollback decides the outcome. Otherwise a new transaction is
// opened; it commits iff fn returns nil, rolls back on error or panic.
//
// Satisfies common.Transactor.
func (m *TxManager) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if _, already := ctx.Value(pgxTxKey{}).(pgx.Tx); already {
		return fn(ctx)
	}
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		return fn(context.WithValue(ctx, pgxTxKey{}, tx))
	})
}

// Compile-time proof that *TxManager satisfies common.Transactor. Keeps the
// service layer oblivious to the GORM → pgx switch during the migration.
var _ common.Transactor = (*TxManager)(nil)

// ErrNoRows re-exports pgx's sentinel so repository packages don't need to
// import pgx directly just to map "no row" → domain NotFound errors.
var ErrNoRows = pgx.ErrNoRows

// IsNoRows reports whether err was caused by pgx returning zero rows.
// Repositories map this to their domain-specific NotFound sentinel.
func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
