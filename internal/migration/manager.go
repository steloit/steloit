package migration

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/clickhouse"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"

	"brokle/internal/config"
	"brokle/internal/infrastructure/database"
	"brokle/pkg/logging"
)

// Manager coordinates migrations across multiple databases
type Manager struct {
	config           *config.Config
	logger           *slog.Logger
	postgresRunner   *migrate.Migrate
	clickhouseRunner *migrate.Migrate
	clickhouseDB     *database.ClickHouseDB
}

// NewManager creates a new migration manager with all databases
func NewManager(cfg *config.Config) (*Manager, error) {
	return NewManagerWithDatabases(cfg, []DatabaseType{PostgresDB, ClickHouseDB})
}

// NewManagerWithDatabases creates a new migration manager with only specified databases
func NewManagerWithDatabases(cfg *config.Config, databases []DatabaseType) (*Manager, error) {
	// Initialize logger with clean text output for CLI
	// Force WarnLevel for migration CLI to keep output clean
	// Migration CLI should only show errors and warnings, not info/debug messages
	// This ensures clean output regardless of LOG_LEVEL environment variable
	logger := logging.NewTextLogger(slog.LevelWarn)

	manager := &Manager{
		config: cfg,
		logger: logger,
	}

	// Helper function to check if database is requested
	needsDatabase := func(dbType DatabaseType) bool {
		for _, db := range databases {
			if db == dbType {
				return true
			}
		}
		return false
	}

	// Conditionally initialize PostgreSQL
	if needsDatabase(PostgresDB) {
		if err := manager.initPostgresRunner(); err != nil {
			return nil, fmt.Errorf("failed to initialize postgres runner: %w", err)
		}
		logger.Info("PostgreSQL migration manager initialized")
	}

	// Conditionally initialize ClickHouse
	if needsDatabase(ClickHouseDB) {
		clickhouseDB, err := database.NewClickHouseDB(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize clickhouse database: %w", err)
		}
		manager.clickhouseDB = clickhouseDB

		// Initialize ClickHouse migration runner
		if err := manager.initClickHouseRunner(); err != nil {
			return nil, fmt.Errorf("failed to initialize clickhouse runner: %w", err)
		}
		logger.Info("ClickHouse migration manager initialized")
	}

	logger.Info("Migration manager initialized successfully", "databases", databases)
	return manager, nil
}

// initPostgresRunner initializes the PostgreSQL migration runner.
// Uses the pgx/v5 golang-migrate driver via URL-based init — symmetric
// with the ClickHouse runner below, no *sql.DB adapter required.
func (m *Manager) initPostgresRunner() error {
	migrationsPath := m.getMigrationsPath(PostgresDB)

	migrateURL, err := postgresMigrateURL(m.config)
	if err != nil {
		return fmt.Errorf("failed to build postgres migration URL: %w", err)
	}

	runner, err := migrate.New(
		"file://"+migrationsPath,
		migrateURL,
	)
	if err != nil {
		return fmt.Errorf("failed to create postgres migration runner: %w", err)
	}

	m.postgresRunner = runner
	m.logger.Info("PostgreSQL migration runner initialized", "migrations_path", migrationsPath)
	return nil
}

// postgresMigrateURL returns the connection URL for the pgx/v5 golang-migrate
// driver, which registers itself under the "pgx5" scheme. pgx (and libpq)
// accept both "postgres://" and "postgresql://" as synonymous; we rewrite
// the scheme using net/url so userinfo, query parameters, IPv6 hosts, and
// either long/short form all round-trip correctly. An unsupported scheme
// fails loudly at startup instead of producing a malformed URL.
func postgresMigrateURL(cfg *config.Config) (string, error) {
	u, err := url.Parse(cfg.GetDatabaseURL())
	if err != nil {
		return "", fmt.Errorf("parse database url: %w", err)
	}
	switch u.Scheme {
	case "postgres", "postgresql":
		u.Scheme = "pgx5"
	default:
		return "", fmt.Errorf("unsupported database url scheme %q (want postgres:// or postgresql://)", u.Scheme)
	}
	return u.String(), nil
}

// initClickHouseRunner initializes the ClickHouse migration runner
func (m *Manager) initClickHouseRunner() error {
	if m.clickhouseDB == nil {
		return errors.New("clickhouse database not initialized")
	}

	// Get migrations path
	migrationsPath := m.getMigrationsPath(ClickHouseDB)

	// Create migration runner using golang-migrate with URL-based approach
	// since ClickHouse uses driver.Conn not sql.DB
	runner, err := migrate.New(
		"file://"+migrationsPath,
		m.config.GetClickHouseURL(),
	)
	if err != nil {
		return fmt.Errorf("failed to create clickhouse migration runner: %w", err)
	}

	m.clickhouseRunner = runner
	m.logger.Info("ClickHouse migration runner initialized", "migrations_path", migrationsPath)
	return nil
}

// getMigrationsPath returns the migrations path for a specific database type
func (m *Manager) getMigrationsPath(dbType DatabaseType) string {
	basePath := "migrations"

	switch dbType {
	case PostgresDB:
		if m.config.Database.MigrationsPath != "" {
			return m.config.Database.MigrationsPath
		}
		return filepath.Join(basePath, "postgres")
	case ClickHouseDB:
		if m.config.ClickHouse.MigrationsPath != "" {
			return m.config.ClickHouse.MigrationsPath
		}
		return filepath.Join(basePath, "clickhouse")
	default:
		return basePath
	}
}

// MigratePostgresUp runs PostgreSQL migrations up
func (m *Manager) MigratePostgresUp(ctx context.Context, steps int, dryRun bool) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}

	if dryRun {
		m.logger.Info("DRY RUN: Would run PostgreSQL migrations up")
		return nil
	}

	m.logger.Info("Running PostgreSQL migrations up", "steps", steps)

	if steps == 0 {
		return m.postgresRunner.Up()
	}
	return m.postgresRunner.Steps(steps)
}

// MigratePostgresDown runs PostgreSQL migrations down
func (m *Manager) MigratePostgresDown(ctx context.Context, steps int, dryRun bool) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}

	if dryRun {
		m.logger.Info("DRY RUN: Would run PostgreSQL migrations down")
		return nil
	}

	m.logger.Info("Running PostgreSQL migrations down", "steps", steps)

	if steps == 0 {
		return m.postgresRunner.Down()
	}
	return m.postgresRunner.Steps(-steps)
}

// MigrateClickHouseUp runs ClickHouse migrations up
func (m *Manager) MigrateClickHouseUp(ctx context.Context, steps int, dryRun bool) error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}

	if dryRun {
		m.logger.Info("DRY RUN: Would run ClickHouse migrations up")
		return nil
	}

	m.logger.Info("Running ClickHouse migrations up", "steps", steps)

	if steps == 0 {
		return m.clickhouseRunner.Up()
	}
	return m.clickhouseRunner.Steps(steps)
}

// MigrateClickHouseDown runs ClickHouse migrations down
func (m *Manager) MigrateClickHouseDown(ctx context.Context, steps int, dryRun bool) error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}

	if dryRun {
		m.logger.Info("DRY RUN: Would run ClickHouse migrations down")
		return nil
	}

	m.logger.Info("Running ClickHouse migrations down", "steps", steps)

	if steps == 0 {
		return m.clickhouseRunner.Down()
	}
	return m.clickhouseRunner.Steps(-steps)
}

// ShowPostgresStatus displays PostgreSQL migration status
func (m *Manager) ShowPostgresStatus(ctx context.Context) error {
	if m.postgresRunner == nil {
		fmt.Println("PostgreSQL: ❌ NOT INITIALIZED")
		fmt.Println("  Run with -db postgres or -db all to initialize PostgreSQL")
		return nil
	}

	version, dirty, err := m.postgresRunner.Version()
	if err != nil {
		return fmt.Errorf("failed to get postgres version: %w", err)
	}

	status := "clean"
	statusIcon := "✅"
	if dirty {
		status = "dirty"
		statusIcon = "⚠️"
	}

	migrationsPath := m.getMigrationsPath(PostgresDB)

	fmt.Printf("PostgreSQL Migration Status:\n")
	fmt.Printf("  %s Current Version: %d (%s)\n", statusIcon, version, status)
	fmt.Printf("  📁 Migrations Path: %s\n", migrationsPath)

	// Get migration count from filesystem
	if count := m.countMigrations(m.getMigrationsPath(PostgresDB)); count > 0 {
		fmt.Printf("  📊 Total Migrations: %d\n", count)
	}

	return nil
}

// ShowClickHouseStatus displays ClickHouse migration status
func (m *Manager) ShowClickHouseStatus(ctx context.Context) error {
	if m.clickhouseRunner == nil {
		fmt.Println("ClickHouse: ❌ NOT INITIALIZED")
		fmt.Println("  Run with -db clickhouse or -db all to initialize ClickHouse")
		return nil
	}

	version, dirty, err := m.clickhouseRunner.Version()
	if err != nil {
		return fmt.Errorf("failed to get clickhouse version: %w", err)
	}

	status := "clean"
	statusIcon := "✅"
	if dirty {
		status = "dirty"
		statusIcon = "⚠️"
	}

	migrationsPath := m.getMigrationsPath(ClickHouseDB)

	fmt.Printf("ClickHouse Migration Status:\n")
	fmt.Printf("  %s Current Version: %d (%s)\n", statusIcon, version, status)
	fmt.Printf("  📁 Migrations Path: %s\n", migrationsPath)

	// Get migration count from filesystem
	if count := m.countMigrations(m.getMigrationsPath(ClickHouseDB)); count > 0 {
		fmt.Printf("  📊 Total Migrations: %d\n", count)
	}

	return nil
}

// GetMigrationInfo returns detailed migration information for both databases
func (m *Manager) GetMigrationInfo() (*MigrationInfo, error) {
	info := &MigrationInfo{}

	// Get PostgreSQL info
	if m.postgresRunner == nil {
		info.Postgres.Status = "not_initialized"
		info.Postgres.Error = "PostgreSQL not initialized - run with -db postgres or -db all"
		info.Postgres.Database = PostgresDB
		info.Postgres.MigrationsPath = m.getMigrationsPath(PostgresDB)
	} else {
		pgVersion, pgDirty, err := m.postgresRunner.Version()
		if err != nil {
			info.Postgres.Status = "error"
			info.Postgres.Error = err.Error()
		} else {
			info.Postgres.Database = PostgresDB
			info.Postgres.CurrentVersion = pgVersion
			info.Postgres.IsDirty = pgDirty
			info.Postgres.MigrationsPath = m.getMigrationsPath(PostgresDB)
			info.Postgres.TotalMigrations = m.countMigrations(m.getMigrationsPath(PostgresDB))
			if pgDirty {
				info.Postgres.Status = "dirty"
			} else {
				info.Postgres.Status = "healthy"
			}
		}
	}

	// Get ClickHouse info
	if m.clickhouseRunner == nil {
		info.ClickHouse.Status = "not_initialized"
		info.ClickHouse.Error = "ClickHouse not initialized - run with -db clickhouse or -db all"
		info.ClickHouse.Database = ClickHouseDB
		info.ClickHouse.MigrationsPath = m.getMigrationsPath(ClickHouseDB)
	} else {
		chVersion, chDirty, err := m.clickhouseRunner.Version()
		if err != nil {
			info.ClickHouse.Status = "error"
			info.ClickHouse.Error = err.Error()
		} else {
			info.ClickHouse.Database = ClickHouseDB
			info.ClickHouse.CurrentVersion = chVersion
			info.ClickHouse.IsDirty = chDirty
			info.ClickHouse.MigrationsPath = m.getMigrationsPath(ClickHouseDB)
			info.ClickHouse.TotalMigrations = m.countMigrations(m.getMigrationsPath(ClickHouseDB))
			if chDirty {
				info.ClickHouse.Status = "dirty"
			} else {
				info.ClickHouse.Status = "healthy"
			}
		}
	}

	// Determine overall status
	if info.Postgres.Status == "error" || info.ClickHouse.Status == "error" {
		info.Overall = "error"
	} else if info.Postgres.Status == "dirty" || info.ClickHouse.Status == "dirty" {
		info.Overall = "dirty"
	} else if info.Postgres.Status == "not_initialized" && info.ClickHouse.Status == "not_initialized" {
		info.Overall = "not_initialized"
	} else if info.Postgres.Status == "not_initialized" || info.ClickHouse.Status == "not_initialized" {
		info.Overall = "partial"
	} else {
		info.Overall = "healthy"
	}

	return info, nil
}

// HealthCheck returns health status for monitoring endpoints
func (m *Manager) HealthCheck() map[string]any {
	health := make(map[string]any)

	var pgErr, chErr error
	var pgDirty, chDirty bool
	var pgVersion, chVersion uint

	// Check PostgreSQL
	if m.postgresRunner == nil {
		health["postgres"] = map[string]any{
			"status": "not_initialized",
			"error":  "PostgreSQL not initialized - run with -db postgres or -db all",
		}
		pgErr = errors.New("not initialized")
	} else {
		pgVersion, pgDirty, pgErr = m.postgresRunner.Version()
		health["postgres"] = map[string]any{
			"status":          m.getHealthStatus(pgErr, pgDirty),
			"current_version": pgVersion,
			"dirty":           pgDirty,
		}
		if pgErr != nil {
			health["postgres"].(map[string]any)["error"] = pgErr.Error()
		}
	}

	// Check ClickHouse
	if m.clickhouseRunner == nil {
		health["clickhouse"] = map[string]any{
			"status": "not_initialized",
			"error":  "ClickHouse not initialized - run with -db clickhouse or -db all",
		}
		chErr = errors.New("not initialized")
	} else {
		chVersion, chDirty, chErr = m.clickhouseRunner.Version()
		health["clickhouse"] = map[string]any{
			"status":          m.getHealthStatus(chErr, chDirty),
			"current_version": chVersion,
			"dirty":           chDirty,
		}
		if chErr != nil {
			health["clickhouse"].(map[string]any)["error"] = chErr.Error()
		}
	}

	// Overall status
	overallHealthy := pgErr == nil && chErr == nil && !pgDirty && !chDirty
	if overallHealthy {
		health["overall_status"] = "healthy"
	} else {
		health["overall_status"] = "unhealthy"
	}

	return health
}

// getHealthStatus converts error and dirty state to health status string
func (m *Manager) getHealthStatus(err error, dirty bool) string {
	if err != nil {
		return "error"
	}
	if dirty {
		return "dirty"
	}
	return "healthy"
}

// GetStatus returns migration status for the manager (required by interface)
func (m *Manager) GetStatus() MigrationStatus {
	// Return overall status - in practice, this might return the most critical status
	pgVersion, pgDirty, pgErr := m.postgresRunner.Version()

	status := MigrationStatus{
		Database:       PostgresDB, // Primary database
		CurrentVersion: pgVersion,
		IsDirty:        pgDirty,
		MigrationsPath: m.getMigrationsPath(PostgresDB),
	}

	if pgErr != nil {
		status.Status = "error"
		status.Error = pgErr.Error()
	} else if pgDirty {
		status.Status = "dirty"
	} else {
		status.Status = "healthy"
	}

	return status
}

// AutoMigrate runs migrations automatically on startup if configured
func (m *Manager) AutoMigrate(ctx context.Context) error {
	if !m.CanAutoMigrate() {
		return errors.New("auto-migration is disabled")
	}

	m.logger.Info("Starting auto-migration")

	// Run PostgreSQL migrations
	if err := m.MigratePostgresUp(ctx, 0, false); err != nil {
		return fmt.Errorf("postgres auto-migration failed: %w", err)
	}

	// Run ClickHouse migrations
	if err := m.MigrateClickHouseUp(ctx, 0, false); err != nil {
		return fmt.Errorf("clickhouse auto-migration failed: %w", err)
	}

	m.logger.Info("Auto-migration completed successfully")
	return nil
}

// CanAutoMigrate returns true if auto-migration is enabled
func (m *Manager) CanAutoMigrate() bool {
	return m.config.Database.AutoMigrate
}

// Advanced operations

// GotoPostgres migrates PostgreSQL to a specific version
func (m *Manager) GotoPostgres(version uint) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}
	// golang-migrate uses different method - use Steps to get to specific version
	current, _, err := m.postgresRunner.Version()
	if err != nil {
		return err
	}
	steps := int(version) - int(current)
	if steps == 0 {
		return nil // already at target version
	}
	return m.postgresRunner.Steps(steps)
}

// GotoClickHouse migrates ClickHouse to a specific version
func (m *Manager) GotoClickHouse(version uint) error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}
	// golang-migrate uses different method - use Steps to get to specific version
	current, _, err := m.clickhouseRunner.Version()
	if err != nil {
		return err
	}
	steps := int(version) - int(current)
	if steps == 0 {
		return nil // already at target version
	}
	return m.clickhouseRunner.Steps(steps)
}

// ForcePostgres forces PostgreSQL to a specific version
func (m *Manager) ForcePostgres(version int) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}
	return m.postgresRunner.Force(version)
}

// ForceClickHouse forces ClickHouse to a specific version
func (m *Manager) ForceClickHouse(version int) error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}
	return m.clickhouseRunner.Force(version)
}

// DropPostgres drops all PostgreSQL tables
func (m *Manager) DropPostgres() error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}
	return m.postgresRunner.Drop()
}

// ResetPostgresComplete performs a complete database reset using DROP SCHEMA CASCADE.
// This removes ALL objects including tables, custom types/enums, sequences, functions, etc.
// Use this when the database is in a dirty/inconsistent state that Drop() can't handle.
func (m *Manager) ResetPostgresComplete(ctx context.Context) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}

	// Open a short-lived pgx connection for the DDL. The migrate runner holds
	// its own pool internally, so using a fresh conn here avoids interfering
	// with its state during what is already a destructive operation.
	conn, err := pgx.Connect(ctx, m.config.GetDatabaseURL())
	if err != nil {
		return fmt.Errorf("connect to postgres for reset: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reset transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, stmt := range []string{
		"DROP SCHEMA IF EXISTS public CASCADE",
		"CREATE SCHEMA public",
		"GRANT ALL ON SCHEMA public TO public",
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("reset schema (%q): %w", stmt, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reset transaction: %w", err)
	}

	m.logger.Info("PostgreSQL schema reset complete - all objects dropped")
	return nil
}

// DropClickHouse drops all ClickHouse tables
func (m *Manager) DropClickHouse() error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}
	return m.clickhouseRunner.Drop()
}

// StepsPostgres runs n PostgreSQL migration steps
func (m *Manager) StepsPostgres(n int) error {
	if m.postgresRunner == nil {
		return errors.New("PostgreSQL not initialized - run with -db postgres or -db all")
	}
	return m.postgresRunner.Steps(n)
}

// StepsClickHouse runs n ClickHouse migration steps
func (m *Manager) StepsClickHouse(n int) error {
	if m.clickhouseRunner == nil {
		return errors.New("ClickHouse not initialized - run with -db clickhouse or -db all")
	}
	return m.clickhouseRunner.Steps(n)
}

// CreatePostgresMigration creates a new PostgreSQL migration file
func (m *Manager) CreatePostgresMigration(name string) error {
	migrationsPath := m.getMigrationsPath(PostgresDB)

	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll(migrationsPath, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	timestamp := time.Now().Format("20060102150405")

	// Create up migration file
	upFile := filepath.Join(migrationsPath, fmt.Sprintf("%s_%s.up.sql", timestamp, name))
	if err := os.WriteFile(upFile, []byte("-- Migration: "+name+"\n-- Created: "+time.Now().Format(time.RFC3339)+"\n\n"), 0644); err != nil {
		return fmt.Errorf("failed to create up migration file: %w", err)
	}

	// Create down migration file
	downFile := filepath.Join(migrationsPath, fmt.Sprintf("%s_%s.down.sql", timestamp, name))
	if err := os.WriteFile(downFile, []byte("-- Rollback: "+name+"\n-- Created: "+time.Now().Format(time.RFC3339)+"\n\n"), 0644); err != nil {
		return fmt.Errorf("failed to create down migration file: %w", err)
	}

	m.logger.Info("PostgreSQL migration files created", "name", name, "up_file", upFile, "down_file", downFile)

	fmt.Printf("PostgreSQL migration files created:\n")
	fmt.Printf("  Up:   %s\n", upFile)
	fmt.Printf("  Down: %s\n", downFile)

	return nil
}

// CreateClickHouseMigration creates a new ClickHouse migration file
func (m *Manager) CreateClickHouseMigration(name string) error {
	migrationsPath := m.getMigrationsPath(ClickHouseDB)

	// Create migrations directory if it doesn't exist
	if err := os.MkdirAll(migrationsPath, 0755); err != nil {
		return fmt.Errorf("failed to create migrations directory: %w", err)
	}

	timestamp := time.Now().Format("20060102150405")

	// Create up migration file
	upFile := filepath.Join(migrationsPath, fmt.Sprintf("%s_%s.up.sql", timestamp, name))
	if err := os.WriteFile(upFile, []byte("-- ClickHouse Migration: "+name+"\n-- Created: "+time.Now().Format(time.RFC3339)+"\n\n"), 0644); err != nil {
		return fmt.Errorf("failed to create up migration file: %w", err)
	}

	// Create down migration file
	downFile := filepath.Join(migrationsPath, fmt.Sprintf("%s_%s.down.sql", timestamp, name))
	if err := os.WriteFile(downFile, []byte("-- ClickHouse Rollback: "+name+"\n-- Created: "+time.Now().Format(time.RFC3339)+"\n\n"), 0644); err != nil {
		return fmt.Errorf("failed to create down migration file: %w", err)
	}

	m.logger.Info("ClickHouse migration files created", "name", name, "up_file", upFile, "down_file", downFile)

	fmt.Printf("ClickHouse migration files created:\n")
	fmt.Printf("  Up:   %s\n", upFile)
	fmt.Printf("  Down: %s\n", downFile)

	return nil
}

// Shutdown gracefully shuts down the migration manager
func (m *Manager) Shutdown() error {
	m.logger.Info("Shutting down migration manager")

	var lastErr error

	// Close PostgreSQL runner
	if m.postgresRunner != nil {
		if _, err := m.postgresRunner.Close(); err != nil {
			m.logger.Error("Failed to close PostgreSQL migration runner", "error", err)
			lastErr = err
		}
	}

	// Close ClickHouse runner
	if m.clickhouseRunner != nil {
		if _, err := m.clickhouseRunner.Close(); err != nil {
			m.logger.Error("Failed to close ClickHouse migration runner", "error", err)
			lastErr = err
		}
	}

	// Close ClickHouse
	if m.clickhouseDB != nil {
		if err := m.clickhouseDB.Close(); err != nil {
			m.logger.Error("Failed to close ClickHouse connection", "error", err)
			lastErr = err
		}
	}

	m.logger.Info("Migration manager shutdown completed")
	return lastErr
}

// countMigrations counts the number of migration files in the given directory
func (m *Manager) countMigrations(migrationsPath string) int {
	if _, err := os.Stat(migrationsPath); os.IsNotExist(err) {
		return 0
	}

	count := 0
	filepath.WalkDir(migrationsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".up.sql") {
			count++
		}
		return nil
	})

	return count
}
