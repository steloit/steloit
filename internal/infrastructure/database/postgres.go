package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	"brokle/internal/config"
)

// PostgresDB represents PostgreSQL database connection
type PostgresDB struct {
	DB     *gorm.DB
	SqlDB  *sql.DB
	config *config.Config
	logger *slog.Logger
}

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(cfg *config.Config, logger *slog.Logger) (*PostgresDB, error) {
	// Configure GORM logger
	glogger := gormLogger.Default

	// Open database connection
	db, err := gorm.Open(postgres.Open(cfg.GetDatabaseURL()), &gorm.Config{
		Logger:                 glogger,
		SkipDefaultTransaction: true,
		PrepareStmt:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// Get underlying SQL DB for connection pooling
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get SQL DB: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.Database.ConnMaxLifetime) * time.Minute)

	// Test connection
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	logger.Info("Connected to PostgreSQL database")

	return &PostgresDB{
		DB:     db,
		SqlDB:  sqlDB,
		config: cfg,
		logger: logger,
	}, nil
}

// Close closes the database connection
func (p *PostgresDB) Close() error {
	p.logger.Info("Closing PostgreSQL connection")
	return p.SqlDB.Close()
}

// Health checks database health
func (p *PostgresDB) Health() error {
	return p.SqlDB.Ping()
}

// GetStats returns database connection statistics
func (p *PostgresDB) GetStats() sql.DBStats {
	return p.SqlDB.Stats()
}
