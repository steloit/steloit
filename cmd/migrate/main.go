// Package main provides database migration tool for both PostgreSQL and ClickHouse.
//
// This is a production-ready migration CLI with comprehensive safety features,
// interactive confirmations for destructive operations, and full support for
// both PostgreSQL and ClickHouse databases.
//
// Usage Examples:
//
//	go run cmd/migrate/main.go up                    # Run all pending migrations
//	go run cmd/migrate/main.go down                  # Rollback 1 migration (with confirmation)
//	go run cmd/migrate/main.go down -steps 5         # Rollback 5 migrations (with confirmation)
//	go run cmd/migrate/main.go -db postgres up       # Run PostgreSQL migrations only
//	go run cmd/migrate/main.go -db clickhouse up     # Run ClickHouse migrations only
//	go run cmd/migrate/main.go status                # Show migration status
//	go run cmd/migrate/main.go goto -version 5       # Migrate to specific version (with confirmation)
//	go run cmd/migrate/main.go force -version 3      # Force version (with confirmation)
//	go run cmd/migrate/main.go drop                  # Drop all tables (with confirmation)
//	go run cmd/migrate/main.go steps -steps 2        # Run 2 steps forward
//	go run cmd/migrate/main.go steps -steps -1       # Run 1 step backward
//	go run cmd/migrate/main.go info                  # Show detailed migration information
//	go run cmd/migrate/main.go create -name "add_users" -db postgres  # Create new migration
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"brokle/internal/config"
	"brokle/internal/migration"
	"brokle/internal/seeder"
)

// MigrateFlags holds all parsed command-line flags
type MigrateFlags struct {
	Database string
	Name     string
	Steps    int
	Version  int
	DryRun   bool
	Reset    bool
	Verbose  bool
}

// parseFlags parses flags from arguments, supporting flags before or after the command
func parseFlags(args []string) (*MigrateFlags, string, error) {
	// Check for help first
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "help" {
			return nil, "help", nil
		}
	}

	if len(args) == 0 {
		return nil, "", errors.New("no command specified")
	}

	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	flags := &MigrateFlags{}
	fs.StringVar(&flags.Database, "db", "all", "Database to migrate: all, postgres, clickhouse")
	fs.IntVar(&flags.Steps, "steps", 0, "Number of migration steps (0 = all)")
	fs.IntVar(&flags.Version, "version", 0, "Target version for goto/force commands")
	fs.StringVar(&flags.Name, "name", "", "Migration name for create command")
	fs.BoolVar(&flags.DryRun, "dry-run", false, "Show what would be migrated without executing")
	fs.BoolVar(&flags.Reset, "reset", false, "Reset existing data before seeding")
	fs.BoolVar(&flags.Verbose, "verbose", false, "Verbose output for seeding")

	// Parse all arguments - fs.Parse will stop at the first non-flag arg
	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}

	remainingArgs := fs.Args()
	if len(remainingArgs) == 0 {
		return nil, "", errors.New("no command specified")
	}

	command := remainingArgs[0]

	// If there are more args after the command, they might be additional flags
	// Parse them as well
	if len(remainingArgs) > 1 {
		if err := fs.Parse(remainingArgs[1:]); err != nil {
			return nil, "", err
		}
	}

	return flags, command, nil
}

func main() {
	// Parse flags and extract command (supports flags before or after command)
	flags, command, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("Error parsing flags: %v", err)
	}

	// Handle help command
	if command == "help" || command == "" {
		printUsage()
		return
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Parse database selection
	databases, err := parseDatabaseSelection(flags.Database)
	if err != nil {
		log.Fatalf("Invalid database selection: %v", err)
	}

	// Initialize migration manager with selected databases
	manager, err := migration.NewManagerWithDatabases(cfg, databases)
	if err != nil {
		log.Fatalf("Failed to initialize migration manager: %v", err)
	}
	defer func() {
		if err := manager.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "migration manager shutdown: %v\n", err)
		}
	}()

	ctx := context.Background()

	// Handle different commands
	switch command {
	case "up":
		if err := runMigrations(ctx, manager, flags.Database, "up", flags.Steps, flags.DryRun); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("✅ Migrations completed successfully")

	case "down":
		// Default to 1 step if no -steps flag provided
		downSteps := flags.Steps
		if downSteps == 0 {
			downSteps = 1
		}

		if !confirmDestructiveOperation(fmt.Sprintf("rollback %d migration(s)", downSteps)) {
			fmt.Println("Operation cancelled")
			return
		}
		if err := runMigrations(ctx, manager, flags.Database, "down", downSteps, flags.DryRun); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("✅ Rollback completed successfully")

	case "status":
		if err := showStatus(ctx, manager, flags.Database); err != nil {
			log.Fatalf("Failed to show status: %v", err)
		}

	case "goto":
		if flags.Version == 0 {
			log.Fatal("Version must be specified for goto command (use -version flag)")
		}
		if !confirmDestructiveOperation(fmt.Sprintf("migrate to version %d", flags.Version)) {
			fmt.Println("Operation cancelled")
			return
		}
		if err := gotoVersion(manager, flags.Database, uint(flags.Version)); err != nil {
			log.Fatalf("Failed to migrate to version %d: %v", flags.Version, err)
		}
		fmt.Printf("✅ Migrated to version %d successfully\n", flags.Version)

	case "force":
		if flags.Version == 0 {
			log.Fatal("Version must be specified for force command (use -version flag)")
		}
		if !confirmDestructiveOperation(fmt.Sprintf("FORCE migration to version %d (DANGEROUS)", flags.Version)) {
			fmt.Println("Operation cancelled")
			return
		}
		if err := forceVersion(manager, flags.Database, flags.Version); err != nil {
			log.Fatalf("Failed to force migration to version %d: %v", flags.Version, err)
		}
		fmt.Printf("⚠️  Forced migration to version %d successfully\n", flags.Version)

	case "drop":
		if !confirmDestructiveOperation("DROP ALL TABLES (PERMANENT DATA LOSS)") {
			fmt.Println("Operation cancelled")
			return
		}
		if err := dropTables(manager, flags.Database); err != nil {
			log.Fatalf("Failed to drop tables: %v", err)
		}
		fmt.Println("⚠️  Tables dropped successfully")

	case "reset":
		if !confirmDestructiveOperation("COMPLETE SCHEMA RESET (drops ALL objects including types, sequences, functions)") {
			fmt.Println("Operation cancelled")
			return
		}
		if err := resetDatabase(ctx, manager, flags.Database); err != nil {
			log.Fatalf("Failed to reset database: %v", err)
		}
		fmt.Println("⚠️  Database schema reset successfully")

	case "steps":
		if flags.Steps == 0 {
			log.Fatal("Steps must be specified for steps command (use -steps flag)")
		}
		if flags.Steps < 0 && !confirmDestructiveOperation(fmt.Sprintf("rollback %d migration steps", -flags.Steps)) {
			fmt.Println("Operation cancelled")
			return
		}
		if err := runSteps(manager, flags.Database, flags.Steps); err != nil {
			log.Fatalf("Failed to run %d migration steps: %v", flags.Steps, err)
		}
		fmt.Printf("✅ Ran %d migration steps successfully\n", flags.Steps)

	case "info":
		if err := showDetailedInfo(manager); err != nil {
			log.Fatalf("Failed to get migration info: %v", err)
		}

	case "create":
		if flags.Name == "" {
			log.Fatal("Migration name is required for create command (use -name flag)")
		}
		if err := createMigration(manager, flags.Database, flags.Name); err != nil {
			log.Fatalf("Failed to create migration: %v", err)
		}

	case "seed":
		if err := runSeeding(ctx, cfg, flags.Reset, flags.DryRun, flags.Verbose); err != nil {
			log.Fatalf("Seeding failed: %v", err)
		}
		fmt.Println("✅ Seeding completed successfully")

	case "seed-pricing":
		if err := runPricingSeeding(ctx, cfg, flags.Reset, flags.DryRun, flags.Verbose); err != nil {
			log.Fatalf("Pricing seeding failed: %v", err)
		}
		fmt.Println("✅ Provider pricing seeding completed successfully")

	case "seed-rbac":
		if err := runRBACSeeding(ctx, cfg, flags.Reset, flags.DryRun, flags.Verbose); err != nil {
			log.Fatalf("RBAC seeding failed: %v", err)
		}
		fmt.Println("✅ RBAC seeding completed successfully")

	case "seed-templates":
		if err := runTemplateSeeding(ctx, cfg, flags.Reset, flags.DryRun, flags.Verbose); err != nil {
			log.Fatalf("Template seeding failed: %v", err)
		}
		fmt.Println("✅ Dashboard template seeding completed successfully")

	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

// confirmDestructiveOperation prompts user for confirmation on dangerous operations
func confirmDestructiveOperation(operation string) bool {
	fmt.Printf("⚠️  DANGER: About to %s.\n", operation)
	fmt.Printf("This action cannot be undone and may result in data loss.\n")
	fmt.Print("Type 'yes' to confirm (anything else will cancel): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}

func runMigrations(ctx context.Context, manager *migration.Manager, database, direction string, steps int, dryRun bool) error {
	if dryRun {
		fmt.Printf("🔍 DRY RUN: Would run %s migrations for %s", direction, database)
		if steps > 0 {
			fmt.Printf(" (%d steps)", steps)
		}
		fmt.Println()
	}

	switch database {
	case "postgres":
		if direction == "up" {
			return manager.MigratePostgresUp(ctx, steps, dryRun)
		}
		return manager.MigratePostgresDown(ctx, steps, dryRun)
	case "clickhouse":
		if direction == "up" {
			return manager.MigrateClickHouseUp(ctx, steps, dryRun)
		}
		return manager.MigrateClickHouseDown(ctx, steps, dryRun)
	case "all":
		if direction == "up" {
			if err := manager.MigratePostgresUp(ctx, steps, dryRun); err != nil {
				return fmt.Errorf("postgres migration failed: %w", err)
			}
			return manager.MigrateClickHouseUp(ctx, steps, dryRun)
		}
		// For down migrations, reverse order (ClickHouse first, then PostgreSQL)
		if err := manager.MigrateClickHouseDown(ctx, steps, dryRun); err != nil {
			return fmt.Errorf("clickhouse migration failed: %w", err)
		}
		return manager.MigratePostgresDown(ctx, steps, dryRun)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func showStatus(ctx context.Context, manager *migration.Manager, database string) error {
	switch database {
	case "postgres":
		return manager.ShowPostgresStatus(ctx)
	case "clickhouse":
		return manager.ShowClickHouseStatus(ctx)
	case "all":
		fmt.Println("🐘 PostgreSQL Migration Status:")
		if err := manager.ShowPostgresStatus(ctx); err != nil {
			fmt.Printf("❌ Error getting PostgreSQL status: %v\n", err)
		}
		fmt.Println()

		fmt.Println("🏠 ClickHouse Migration Status:")
		if err := manager.ShowClickHouseStatus(ctx); err != nil {
			fmt.Printf("❌ Error getting ClickHouse status: %v\n", err)
		}

		// Show overall health
		fmt.Println()
		info, err := manager.GetMigrationInfo()
		if err != nil {
			fmt.Printf("❌ Error getting overall status: %v\n", err)
		} else {
			switch info.Overall {
			case "healthy":
				fmt.Println("Overall Status: 🟢 HEALTHY")
			case "dirty":
				fmt.Println("Overall Status: 🟡 DIRTY (requires attention)")
			case "error":
				fmt.Println("Overall Status: 🔴 ERROR")
			default:
				fmt.Printf("Overall Status: ❓ %s\n", info.Overall)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func gotoVersion(manager *migration.Manager, database string, version uint) error {
	switch database {
	case "postgres":
		return manager.GotoPostgres(version)
	case "clickhouse":
		return manager.GotoClickHouse(version)
	case "all":
		if err := manager.GotoPostgres(version); err != nil {
			return fmt.Errorf("postgres goto failed: %w", err)
		}
		return manager.GotoClickHouse(version)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func forceVersion(manager *migration.Manager, database string, version int) error {
	switch database {
	case "postgres":
		return manager.ForcePostgres(version)
	case "clickhouse":
		return manager.ForceClickHouse(version)
	case "all":
		if err := manager.ForcePostgres(version); err != nil {
			return fmt.Errorf("postgres force failed: %w", err)
		}
		return manager.ForceClickHouse(version)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func dropTables(manager *migration.Manager, database string) error {
	switch database {
	case "postgres":
		return manager.DropPostgres()
	case "clickhouse":
		return manager.DropClickHouse()
	case "all":
		if err := manager.DropClickHouse(); err != nil {
			return fmt.Errorf("clickhouse drop failed: %w", err)
		}
		return manager.DropPostgres()
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func resetDatabase(ctx context.Context, manager *migration.Manager, database string) error {
	switch database {
	case "postgres":
		return manager.ResetPostgresComplete(ctx)
	case "clickhouse":
		// ClickHouse doesn't have custom types, regular drop is sufficient
		return manager.DropClickHouse()
	case "all":
		if err := manager.DropClickHouse(); err != nil {
			return fmt.Errorf("clickhouse reset failed: %w", err)
		}
		return manager.ResetPostgresComplete(ctx)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func runSteps(manager *migration.Manager, database string, steps int) error {
	switch database {
	case "postgres":
		return manager.StepsPostgres(steps)
	case "clickhouse":
		return manager.StepsClickHouse(steps)
	case "all":
		if err := manager.StepsPostgres(steps); err != nil {
			return fmt.Errorf("postgres steps failed: %w", err)
		}
		return manager.StepsClickHouse(steps)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

func showDetailedInfo(manager *migration.Manager) error {
	info, err := manager.GetMigrationInfo()
	if err != nil {
		return err
	}

	fmt.Println("📊 Detailed Migration Information")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("\n🐘 PostgreSQL:")
	fmt.Printf("  Status: %s\n", getStatusIcon(info.Postgres.Status))
	fmt.Printf("  Current Version: %d\n", info.Postgres.CurrentVersion)
	fmt.Printf("  Dirty State: %v\n", info.Postgres.IsDirty)
	fmt.Printf("  Migrations Path: %s\n", info.Postgres.MigrationsPath)
	if info.Postgres.Error != "" {
		fmt.Printf("  Error: %s\n", info.Postgres.Error)
	}

	fmt.Println("\n🏠 ClickHouse:")
	fmt.Printf("  Status: %s\n", getStatusIcon(info.ClickHouse.Status))
	fmt.Printf("  Current Version: %d\n", info.ClickHouse.CurrentVersion)
	fmt.Printf("  Dirty State: %v\n", info.ClickHouse.IsDirty)
	fmt.Printf("  Migrations Path: %s\n", info.ClickHouse.MigrationsPath)
	if info.ClickHouse.Error != "" {
		fmt.Printf("  Error: %s\n", info.ClickHouse.Error)
	}

	fmt.Printf("\n🌐 Overall Status: %s\n", getStatusIcon(info.Overall))

	return nil
}

func getStatusIcon(status string) string {
	switch status {
	case "healthy":
		return "🟢 HEALTHY"
	case "dirty":
		return "🟡 DIRTY"
	case "error":
		return "🔴 ERROR"
	default:
		return "❓ " + strings.ToUpper(status)
	}
}

func createMigration(manager *migration.Manager, database, name string) error {
	switch database {
	case "postgres":
		return manager.CreatePostgresMigration(name)
	case "clickhouse":
		return manager.CreateClickHouseMigration(name)
	case "all":
		fmt.Println("Creating migrations for both databases...")
		if err := manager.CreatePostgresMigration(name); err != nil {
			return fmt.Errorf("failed to create postgres migration: %w", err)
		}
		return manager.CreateClickHouseMigration(name)
	default:
		return fmt.Errorf("unknown database: %s", database)
	}
}

// parseDatabaseSelection converts database flag string to DatabaseType slice
func parseDatabaseSelection(database string) ([]migration.DatabaseType, error) {
	switch database {
	case "postgres":
		return []migration.DatabaseType{migration.PostgresDB}, nil
	case "clickhouse":
		return []migration.DatabaseType{migration.ClickHouseDB}, nil
	case "all":
		return []migration.DatabaseType{migration.PostgresDB, migration.ClickHouseDB}, nil
	default:
		return nil, fmt.Errorf("unknown database: %s (valid options: postgres, clickhouse, all)", database)
	}
}

func printUsage() {
	fmt.Println("🚀 Brokle Migration Tool - Production Database Migration & Seeding CLI")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  migrate <command> [flags]")
	fmt.Println()
	fmt.Println("COMMANDS:")
	fmt.Println("  up                    Run all pending migrations")
	fmt.Println("  down                  Rollback 1 migration (use -steps for more)")
	fmt.Println("  status                Show migration status for all databases")
	fmt.Println("  goto -version N       Migrate to specific version (with confirmation)")
	fmt.Println("  force -version N      Force version without migration (DANGEROUS)")
	fmt.Println("  drop                  Drop all tables (DANGEROUS)")
	fmt.Println("  reset                 Complete schema reset including types (DANGEROUS)")
	fmt.Println("  steps -steps N        Run N migration steps (negative for rollback)")
	fmt.Println("  info                  Show detailed migration information")
	fmt.Println("  create -name NAME     Create new migration files")
	fmt.Println("  seed                  Seed system data (permissions, roles, pricing, templates)")
	fmt.Println("  seed-rbac             Seed RBAC data only (permissions and roles)")
	fmt.Println("  seed-pricing          Seed provider pricing data only")
	fmt.Println("  seed-templates        Seed dashboard templates only")
	fmt.Println()
	fmt.Println("FLAGS:")
	fmt.Println("  -db string           Database to target: all, postgres, clickhouse (default: all)")
	fmt.Println("  -steps int           Number of migration steps")
	fmt.Println("  -version int         Target version for goto/force commands")
	fmt.Println("  -name string         Migration name for create command")
	fmt.Println("  -dry-run             Show what would happen without executing")
	fmt.Println("  -reset               Reset existing data before seeding (DANGEROUS)")
	fmt.Println("  -verbose             Verbose output for seeding operations")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  migrate up                              # Run all pending migrations")
	fmt.Println("  migrate status -db postgres             # Show PostgreSQL status only")
	fmt.Println("  migrate status -db clickhouse           # Show ClickHouse status only")
	fmt.Println("  migrate up -db postgres                 # Run PostgreSQL migrations only")
	fmt.Println("  migrate down                            # Rollback 1 migration")
	fmt.Println("  migrate down -steps 5                   # Rollback 5 migrations")
	fmt.Println("  migrate down -db postgres -steps 3      # Rollback 3 PostgreSQL migrations")
	fmt.Println("  migrate goto -version 5                 # Go to version 5 with confirmation")
	fmt.Println("  migrate steps -steps 2                  # Run 2 migration steps")
	fmt.Println("  migrate create -name 'add_users'        # Create new migration")
	fmt.Println("  migrate info                            # Show detailed information")
	fmt.Println("  migrate up -dry-run                     # Preview migrations")
	fmt.Println("  migrate seed                            # Seed permissions, roles, pricing")
	fmt.Println("  migrate seed -reset -verbose            # Reset and seed with verbose output")
	fmt.Println("  migrate seed-rbac                       # Seed permissions and roles only")
	fmt.Println("  migrate seed-pricing                    # Seed provider pricing only")
	fmt.Println("  migrate seed-pricing -reset             # Reset and reseed pricing")
	fmt.Println("  migrate seed-templates                  # Seed dashboard templates only")
	fmt.Println("  migrate seed-templates -reset -verbose  # Reset and reseed templates")
	fmt.Println()
	fmt.Println("NOTE:")
	fmt.Println("  Flags can be placed before or after the command:")
	fmt.Println("  migrate status -db postgres    (recommended)")
	fmt.Println("  migrate -db postgres status    (also works)")
	fmt.Println()
	fmt.Println("SAFETY:")
	fmt.Println("  🛡️  Destructive operations require explicit 'yes' confirmation")
	fmt.Println("  🔍 Use -dry-run to preview changes safely")
	fmt.Println("  📊 Check 'status' and 'info' before running migrations")
	fmt.Println("  🌱 Use 'seed' to populate database with system data")
}

// runSeeding handles database seeding operations
func runSeeding(ctx context.Context, cfg *config.Config, reset, dryRun, verbose bool) error {
	// Initialize seeder
	s, err := seeder.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize seeder: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "seeder close: %v\n", err)
		}
	}()

	// Configure seeder options
	options := &seeder.Options{
		Reset:   reset,
		DryRun:  dryRun,
		Verbose: verbose,
	}

	// Confirm reset operation if requested
	if reset && !dryRun {
		if !confirmDestructiveOperation("RESET ALL DATA and reseed") {
			return errors.New("seeding cancelled by user")
		}
	}

	// Show seed plan in dry-run mode
	if dryRun {
		fmt.Println("🔍 DRY RUN: Seeding plan:")

		seedData, err := s.LoadSeedData()
		if err != nil {
			return fmt.Errorf("failed to load seed data for preview: %w", err)
		}

		s.PrintSeedPlan(seedData)
		return nil
	}

	// Run actual seeding
	return s.SeedAll(ctx, options)
}

// runPricingSeeding handles provider pricing seeding operations (standalone)
func runPricingSeeding(ctx context.Context, cfg *config.Config, reset, dryRun, verbose bool) error {
	// Initialize seeder
	s, err := seeder.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize seeder: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "seeder close: %v\n", err)
		}
	}()

	// Configure seeder options
	options := &seeder.Options{
		Reset:   reset,
		DryRun:  dryRun,
		Verbose: verbose,
	}

	// Confirm reset operation if requested
	if reset && !dryRun {
		if !confirmDestructiveOperation("RESET ALL PRICING DATA and reseed") {
			return errors.New("pricing seeding cancelled by user")
		}
	}

	// Run pricing seeding
	return s.SeedPricing(ctx, options)
}

// runRBACSeeding handles RBAC seeding operations (standalone)
func runRBACSeeding(ctx context.Context, cfg *config.Config, reset, dryRun, verbose bool) error {
	// Initialize seeder
	s, err := seeder.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize seeder: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "seeder close: %v\n", err)
		}
	}()

	// Configure seeder options
	options := &seeder.Options{
		Reset:   reset,
		DryRun:  dryRun,
		Verbose: verbose,
	}

	// Confirm reset operation if requested
	if reset && !dryRun {
		if !confirmDestructiveOperation("RESET ALL RBAC DATA and reseed") {
			return errors.New("RBAC seeding cancelled by user")
		}
	}

	// Run RBAC seeding
	return s.SeedRBAC(ctx, options)
}

// runTemplateSeeding handles dashboard template seeding operations (standalone)
func runTemplateSeeding(ctx context.Context, cfg *config.Config, reset, dryRun, verbose bool) error {
	// Initialize seeder
	s, err := seeder.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize seeder: %w", err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "seeder close: %v\n", err)
		}
	}()

	// Configure seeder options
	options := &seeder.Options{
		Reset:   reset,
		DryRun:  dryRun,
		Verbose: verbose,
	}

	// Confirm reset operation if requested
	if reset && !dryRun {
		if !confirmDestructiveOperation("RESET ALL TEMPLATE DATA and reseed") {
			return errors.New("template seeding cancelled by user")
		}
	}

	// Run template seeding
	return s.SeedTemplates(ctx, options)
}
