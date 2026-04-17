// Package db owns PostgreSQL access via pgx/v5.
//
// Responsibilities:
//   - Construct the pgxpool.Pool from DatabaseConfig
//   - Register pgx codecs (shopspring.Decimal ↔ NUMERIC) on every new connection
//   - Expose the pool to the TxManager, which is the single handle all
//     repositories depend on
//
// pgxpool design note: unlike database/sql, pgxpool has no "maximum idle
// connections" knob. Idle connections are pruned by MaxConnIdleTime and
// MaxConnLifetime. MinConns is a *floor* (always-open), not a ceiling, so
// we deliberately do not map stdlib-style MaxIdleConns onto it — that would
// force connections permanently open and exhaust database budgets.
package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"brokle/internal/config"
)

// NewPool constructs a configured pgxpool.Pool against the PostgreSQL URL in
// cfg. The pool eagerly opens a connection to surface misconfiguration at
// startup and registers codecs on every subsequent connection via AfterConnect.
func NewPool(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.GetDatabaseURL())
	if err != nil {
		return nil, fmt.Errorf("parse postgres url: %w", err)
	}

	// Map Brokle DatabaseConfig → pgxpool.Config. Only knobs pgxpool
	// actually models are mapped; see the package doc for why no
	// MaxIdleConns analogue exists. Conversions to int32 are safe for any
	// pool size a single application process would realistically need.
	dbCfg := cfg.Database
	if dbCfg.MaxOpenConns > 0 {
		poolCfg.MaxConns = int32(dbCfg.MaxOpenConns)
	}
	if dbCfg.ConnMaxLifetime > 0 {
		poolCfg.MaxConnLifetime = dbCfg.ConnMaxLifetime
	}
	if dbCfg.ConnMaxIdleTime > 0 {
		poolCfg.MaxConnIdleTime = dbCfg.ConnMaxIdleTime
	}

	// AfterConnect runs for every new connection — this is where custom pgx
	// type codecs (e.g. shopspring.Decimal) must be registered so subsequent
	// queries on that connection can marshal them without per-query glue.
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return registerCodecs(conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}

	// Surface connection errors at startup, not on the first request.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	logger.Info("pgx pool ready",
		"max_conns", poolCfg.MaxConns,
		"min_conns", poolCfg.MinConns,
	)
	return pool, nil
}
