package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"brokle/internal/config"
)

// ClickHouseDB represents ClickHouse database connection
type ClickHouseDB struct {
	Conn   driver.Conn
	config *config.Config
	logger *slog.Logger
}

// NewClickHouseDB creates a new ClickHouse database connection
func NewClickHouseDB(cfg *config.Config, logger *slog.Logger) (*ClickHouseDB, error) {
	// Parse connection options from URL
	options, err := clickhouse.ParseDSN(cfg.GetClickHouseURL())
	if err != nil {
		return nil, fmt.Errorf("failed to parse ClickHouse DSN: %w", err)
	}

	// Configure connection settings
	options.Settings = clickhouse.Settings{
		"max_execution_time": 60,
		"max_memory_usage":   "10000000000",
	}

	options.DialTimeout = 5 * time.Second
	options.Compression = &clickhouse.Compression{
		Method: clickhouse.CompressionLZ4,
	}

	// Configure connection pooling for high-throughput analytics worker
	// Sized for 4.5k buffer capacity with concurrent processing
	options.MaxOpenConns = 50           // Maximum open connections
	options.MaxIdleConns = 10           // Keep connections alive for reuse
	options.ConnMaxLifetime = time.Hour // Connection lifetime

	// Configure batch processing settings
	options.BlockBufferSize = 10 // Optimize for batch inserts

	// Open connection
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	logger.Info("Connected to ClickHouse database")

	return &ClickHouseDB{
		Conn:   conn,
		config: cfg,
		logger: logger,
	}, nil
}

// Close closes the ClickHouse connection
func (c *ClickHouseDB) Close() error {
	c.logger.Info("Closing ClickHouse connection")
	return c.Conn.Close()
}

// Health checks ClickHouse health
func (c *ClickHouseDB) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.Conn.Ping(ctx)
}

// Execute executes a query without returning results
func (c *ClickHouseDB) Execute(ctx context.Context, query string, args ...interface{}) error {
	return c.Conn.Exec(ctx, query, args...)
}

// Query executes a query and returns rows
func (c *ClickHouseDB) Query(ctx context.Context, query string, args ...interface{}) (driver.Rows, error) {
	return c.Conn.Query(ctx, query, args...)
}

// QueryRow executes a query and returns a single row
func (c *ClickHouseDB) QueryRow(ctx context.Context, query string, args ...interface{}) driver.Row {
	return c.Conn.QueryRow(ctx, query, args...)
}
