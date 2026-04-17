package seeder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"gopkg.in/yaml.v3"

	"github.com/google/uuid"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	"brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/billing"
	"brokle/internal/core/domain/dashboard"
	"brokle/internal/infrastructure/db"
	analyticsRepo "brokle/internal/infrastructure/repository/analytics"
	authRepo "brokle/internal/infrastructure/repository/auth"
	billingRepo "brokle/internal/infrastructure/repository/billing"
	dashboardRepo "brokle/internal/infrastructure/repository/dashboard"
	"brokle/pkg/logging"
	"brokle/pkg/uid"
)

type Seeder struct {
	pool   *pgxpool.Pool
	tm     *db.TxManager
	cfg    *config.Config
	logger *slog.Logger

	// Repositories
	roleRepo          auth.RoleRepository
	permissionRepo    auth.PermissionRepository
	rolePermRepo      auth.RolePermissionRepository
	providerModelRepo analytics.ProviderModelRepository
	templateRepo      dashboard.TemplateRepository
	planRepo          billing.PlanRepository
	orgBillingRepo    billing.OrganizationBillingRepository
}

func New(cfg *config.Config) (*Seeder, error) {
	// Create logger for seeding - use Info level so progress and verbose output are visible
	logger := logging.NewLoggerWithFormat(slog.LevelInfo, cfg.Logging.Format)

	pool, err := db.NewPool(context.Background(), cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open pgx pool: %w", err)
	}

	s := &Seeder{
		pool:   pool,
		tm:     db.NewTxManager(pool),
		cfg:    cfg,
		logger: logger,
	}

	// Initialize repositories
	s.roleRepo = authRepo.NewRoleRepository(s.tm)
	s.permissionRepo = authRepo.NewPermissionRepository(s.tm)
	s.rolePermRepo = authRepo.NewRolePermissionRepository(s.tm)
	s.providerModelRepo = analyticsRepo.NewProviderModelRepository(s.tm)
	s.templateRepo = dashboardRepo.NewTemplateRepository(s.tm)
	s.planRepo = billingRepo.NewPlanRepository(s.tm)
	s.orgBillingRepo = billingRepo.NewOrganizationBillingRepository(s.tm)

	return s, nil
}

func (s *Seeder) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
}

func (s *Seeder) SeedAll(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed PostgreSQL with permissions, roles, and pricing")
		return nil
	}

	s.logger.Info("Starting PostgreSQL seeding...")

	// Reset data if requested
	if opts.Reset {
		s.logger.Info("Resetting existing data...")
		if err := s.Reset(ctx, opts.Verbose); err != nil {
			return fmt.Errorf("failed to reset data: %w", err)
		}
	}

	// Load and seed RBAC data
	permissions, err := s.loadPermissions()
	if err != nil {
		return fmt.Errorf("failed to load permissions: %w", err)
	}

	roles, err := s.loadRoles()
	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

	entityMaps := NewEntityMaps()

	s.logger.Debug("Starting seeding process", "permissions", len(permissions), "roles", len(roles))

	// 1. Seed permissions (no dependencies)
	if err := s.seedPermissions(ctx, permissions, entityMaps, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed permissions: %w", err)
	}

	// 2. Seed roles (depends on permissions)
	if err := s.seedRoles(ctx, roles, entityMaps, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed roles: %w", err)
	}

	// 3. Seed pricing (independent)
	if err := s.seedPricingFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed pricing: %w", err)
	}

	// 4. Seed dashboard templates (independent)
	if err := s.seedTemplatesFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed templates: %w", err)
	}

	// 5. Seed billing configs (independent)
	if err := s.seedBillingConfigsFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed billing configs: %w", err)
	}

	// 6. Provision billing for existing organizations without billing records
	if err := s.SeedOrganizationBillings(ctx, opts); err != nil {
		return fmt.Errorf("failed to seed organization billings: %w", err)
	}

	s.logger.Info("PostgreSQL seeding completed successfully")
	return nil
}

func (s *Seeder) SeedRBAC(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed RBAC (permissions and roles)")
		return nil
	}

	s.logger.Info("Starting RBAC seeding...")

	// Reset RBAC data if requested
	if opts.Reset {
		s.logger.Info("Resetting existing RBAC data...")
		if err := s.resetRBAC(ctx, opts.Verbose); err != nil {
			return fmt.Errorf("failed to reset RBAC data: %w", err)
		}
	}

	// Load seed data
	permissions, err := s.loadPermissions()
	if err != nil {
		return fmt.Errorf("failed to load permissions: %w", err)
	}

	roles, err := s.loadRoles()
	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

	entityMaps := NewEntityMaps()

	// Seed permissions
	if err := s.seedPermissions(ctx, permissions, entityMaps, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed permissions: %w", err)
	}

	// Seed roles
	if err := s.seedRoles(ctx, roles, entityMaps, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed roles: %w", err)
	}

	// Print statistics
	stats, err := s.GetRBACStatistics(ctx)
	if err == nil {
		s.logger.Debug("RBAC Statistics", "permissions", stats.TotalPermissions, "roles", stats.TotalRoles)
	}

	s.logger.Info("RBAC seeding completed successfully")
	return nil
}

func (s *Seeder) SeedPricing(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed provider pricing")
		return nil
	}

	s.logger.Info("Starting provider pricing seeding...")

	// Reset pricing data if requested
	if opts.Reset {
		s.logger.Info("Resetting existing pricing data...")
		if err := s.resetPricing(ctx, opts.Verbose); err != nil {
			return fmt.Errorf("failed to reset pricing data: %w", err)
		}
	}

	// Load and seed pricing data
	if err := s.seedPricingFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed pricing: %w", err)
	}

	// Print statistics
	stats, err := s.GetPricingStatistics(ctx)
	if err == nil {
		s.logger.Debug("Pricing Statistics", "models", stats.TotalModels, "prices", stats.TotalPrices)
	}

	s.logger.Info("Provider pricing seeding completed successfully")
	return nil
}

func (s *Seeder) Reset(ctx context.Context, verbose bool) error {
	s.logger.Info("Starting data reset...")

	// Reset RBAC data
	if err := s.resetRBAC(ctx, verbose); err != nil {
		s.logger.Warn(" Could not reset RBAC data", "error", err)
	}

	// Reset pricing data
	if err := s.resetPricing(ctx, verbose); err != nil {
		s.logger.Warn(" Could not reset pricing data", "error", err)
	}

	// Reset template data
	if err := s.resetTemplates(ctx, verbose); err != nil {
		s.logger.Warn(" Could not reset template data", "error", err)
	}

	// Reset billing configs data
	if err := s.resetBillingConfigs(ctx, verbose); err != nil {
		s.logger.Warn(" Could not reset billing configs data", "error", err)
	}

	s.logger.Info("Data reset completed")
	return nil
}

func (s *Seeder) PrintSeedPlan(data *SeedData) {
	fmt.Println("\nSEED PLAN:")
	fmt.Println("=====================================")

	fmt.Printf("\nPermissions: %d\n", len(data.Permissions))
	for _, perm := range data.Permissions {
		fmt.Printf("  - %s\n", perm.Name)
	}

	fmt.Printf("\nRoles: %d\n", len(data.Roles))
	for _, role := range data.Roles {
		fmt.Printf("  - %s (%s scope) - %d permissions\n", role.Name, role.ScopeType, len(role.Permissions))
	}

	fmt.Println("=====================================")
}

// For dry-run preview.
func (s *Seeder) LoadSeedData() (*SeedData, error) {
	permissions, err := s.loadPermissions()
	if err != nil {
		return nil, fmt.Errorf("failed to load permissions: %w", err)
	}

	roles, err := s.loadRoles()
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}

	return &SeedData{
		Permissions: permissions,
		Roles:       roles,
	}, nil
}

func (s *Seeder) loadPermissions() ([]PermissionSeed, error) {
	seedFile := findSeedFile("seeds/permissions.yaml")
	if seedFile == "" {
		return nil, errors.New("permissions file not found: seeds/permissions.yaml")
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", seedFile, err)
	}

	var permissionsFile PermissionsFile
	if err := yaml.Unmarshal(data, &permissionsFile); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", seedFile, err)
	}

	// Validate
	permissionNames := make(map[string]bool)
	for _, permission := range permissionsFile.Permissions {
		if permission.Name == "" {
			return nil, errors.New("permission missing required field: name")
		}
		if permissionNames[permission.Name] {
			return nil, fmt.Errorf("duplicate permission name: %s", permission.Name)
		}
		permissionNames[permission.Name] = true
	}

	return permissionsFile.Permissions, nil
}

func (s *Seeder) loadRoles() ([]RoleSeed, error) {
	seedFile := findSeedFile("seeds/roles.yaml")
	if seedFile == "" {
		return nil, errors.New("roles file not found: seeds/roles.yaml")
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", seedFile, err)
	}

	var rolesFile RolesFile
	if err := yaml.Unmarshal(data, &rolesFile); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", seedFile, err)
	}

	// Validate
	roleNames := make(map[string]bool)
	for _, role := range rolesFile.Roles {
		if role.Name == "" || role.ScopeType == "" {
			return nil, errors.New("role missing required fields (name, scope_type)")
		}
		if roleNames[role.Name] {
			return nil, fmt.Errorf("duplicate role name: %s", role.Name)
		}
		roleNames[role.Name] = true
	}

	return rolesFile.Roles, nil
}

func (s *Seeder) loadPricing() (*ProviderPricingSeedData, error) {
	seedFile := findSeedFile("seeds/pricing.yaml")
	if seedFile == "" {
		return nil, errors.New("pricing file not found: seeds/pricing.yaml")
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", seedFile, err)
	}

	var pricingData ProviderPricingSeedData
	if err := yaml.Unmarshal(data, &pricingData); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", seedFile, err)
	}

	// Validate
	if len(pricingData.ProviderModels) == 0 {
		return nil, errors.New("no provider models defined")
	}

	modelNames := make(map[string]bool)
	for i, model := range pricingData.ProviderModels {
		if model.ModelName == "" {
			return nil, fmt.Errorf("model %d missing required field: model_name", i)
		}
		if model.MatchPattern == "" {
			return nil, fmt.Errorf("model %s missing required field: match_pattern", model.ModelName)
		}
		if model.StartDate == "" {
			return nil, fmt.Errorf("model %s missing required field: start_date", model.ModelName)
		}
		if modelNames[model.ModelName] {
			return nil, fmt.Errorf("duplicate model_name: %s", model.ModelName)
		}
		modelNames[model.ModelName] = true

		if len(model.Prices) == 0 {
			return nil, fmt.Errorf("model %s has no prices defined", model.ModelName)
		}

		usageTypes := make(map[string]bool)
		for _, price := range model.Prices {
			if price.UsageType == "" {
				return nil, fmt.Errorf("model %s has price with empty usage_type", model.ModelName)
			}
			if usageTypes[price.UsageType] {
				return nil, fmt.Errorf("model %s has duplicate usage_type: %s", model.ModelName, price.UsageType)
			}
			usageTypes[price.UsageType] = true
			if price.Price < 0 {
				return nil, fmt.Errorf("model %s has negative price for %s", model.ModelName, price.UsageType)
			}
		}
	}

	return &pricingData, nil
}

func findSeedFile(seedFile string) string {
	if _, err := os.Stat(seedFile); err == nil {
		return seedFile
	}
	broklePath := filepath.Join("brokle", seedFile)
	if _, err := os.Stat(broklePath); err == nil {
		return broklePath
	}
	return ""
}

func (s *Seeder) seedPermissions(ctx context.Context, permissionSeeds []PermissionSeed, entityMaps *EntityMaps, verbose bool) error {
	if verbose {
		s.logger.Info("Seeding permissions", "count", len(permissionSeeds))
	}

	for _, permSeed := range permissionSeeds {
		// Parse resource and action from name (format: "resource:action")
		parts := strings.SplitN(permSeed.Name, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid permission name format: %s (expected resource:action)", permSeed.Name)
		}

		resource := parts[0]
		action := parts[1]

		// Check if permission already exists (idempotent)
		existing, err := s.permissionRepo.GetByResourceAction(ctx, resource, action)
		if err == nil && existing != nil {
			if verbose {
				s.logger.Info("Permission already exists, skipping", "name", permSeed.Name)
			}
			entityMaps.Permissions[permSeed.Name] = existing.ID
			continue
		}

		// Determine scope level from resource name
		scopeLevel := auth.ScopeLevelOrganization
		category := resource

		projectResources := map[string]bool{
			"traces":          true,
			"analytics":       true,
			"provider_models": true,
			"providers":       true,
			"costs":           true,
			"prompts":         true,
		}

		if projectResources[resource] {
			scopeLevel = auth.ScopeLevelProject

			if resource == "traces" || resource == "analytics" || resource == "costs" {
				category = "observability"
			} else {
				category = "gateway"
			}
		}

		// Create new permission with scope level
		permission := auth.NewPermissionWithScope(resource, action, permSeed.Description, scopeLevel, category)

		if err := s.permissionRepo.Create(ctx, permission); err != nil {
			return fmt.Errorf("failed to create permission %s: %w", permSeed.Name, err)
		}

		entityMaps.Permissions[permSeed.Name] = permission.ID

		if verbose {
			s.logger.Info("Created permission", "name", permSeed.Name)
		}
	}

	return nil
}

func (s *Seeder) seedRoles(ctx context.Context, roleSeeds []RoleSeed, entityMaps *EntityMaps, verbose bool) error {
	if verbose {
		s.logger.Info("Seeding template roles", "count", len(roleSeeds))
	}

	for _, roleSeed := range roleSeeds {
		// Check if template role already exists
		existing, err := s.roleRepo.GetByNameAndScope(ctx, roleSeed.Name, roleSeed.ScopeType)
		if err == nil && existing != nil {
			// Role exists - sync permissions to match YAML definition
			entityMaps.Roles[fmt.Sprintf("%s:%s", roleSeed.ScopeType, roleSeed.Name)] = existing.ID

			// Build permission ID list from YAML
			var permissionIDs []uuid.UUID
			for _, permName := range roleSeed.Permissions {
				permID, exists := entityMaps.Permissions[permName]
				if !exists {
					return fmt.Errorf("permission not found: %s for role %s", permName, roleSeed.Name)
				}
				permissionIDs = append(permissionIDs, permID)
			}

			// Sync permissions (replaces all existing with YAML definition)
			if err := s.roleRepo.UpdateRolePermissions(ctx, existing.ID, permissionIDs, nil); err != nil {
				return fmt.Errorf("failed to sync permissions for role %s: %w", roleSeed.Name, err)
			}

			if verbose {
				s.logger.Info("Synced role", "name", roleSeed.Name, "scope", roleSeed.ScopeType, "permissions", len(roleSeed.Permissions))
			}
			continue
		}

		// Create new template role
		role := auth.NewRole(roleSeed.Name, roleSeed.ScopeType, roleSeed.Description)

		if err := s.roleRepo.Create(ctx, role); err != nil {
			return fmt.Errorf("failed to create template role %s: %w", roleSeed.Name, err)
		}

		entityMaps.Roles[fmt.Sprintf("%s:%s", roleSeed.ScopeType, roleSeed.Name)] = role.ID

		// Assign permissions to role template
		if len(roleSeed.Permissions) > 0 {
			var permissionIDs []uuid.UUID
			for _, permName := range roleSeed.Permissions {
				permID, exists := entityMaps.Permissions[permName]
				if !exists {
					return fmt.Errorf("permission not found: %s for role %s", permName, roleSeed.Name)
				}
				permissionIDs = append(permissionIDs, permID)
			}

			if err := s.roleRepo.AssignRolePermissions(ctx, role.ID, permissionIDs, nil); err != nil {
				return fmt.Errorf("failed to assign permissions to role %s: %w", roleSeed.Name, err)
			}
		}

		if verbose {
			s.logger.Info("Created template role", "name", roleSeed.Name, "scope", roleSeed.ScopeType, "permissions", len(roleSeed.Permissions))
		}
	}

	return nil
}

func (s *Seeder) seedPricingFromFile(ctx context.Context, verbose bool) error {
	pricingData, err := s.loadPricing()
	if err != nil {
		// Pricing data is optional - log warning but don't fail
		s.logger.Warn(" Could not load pricing data", "error", err)
		return nil
	}

	return s.seedPricingData(ctx, pricingData, verbose)
}

func (s *Seeder) seedPricingData(ctx context.Context, data *ProviderPricingSeedData, verbose bool) error {
	if verbose {
		s.logger.Info("Seeding provider models with pricing", "count", len(data.ProviderModels))
	}

	for _, modelSeed := range data.ProviderModels {
		if err := s.seedModel(ctx, modelSeed, verbose); err != nil {
			return fmt.Errorf("failed to seed model %s: %w", modelSeed.ModelName, err)
		}
	}

	if verbose {
		s.logger.Info("Provider pricing seeded successfully", "version", data.Version)
	}
	return nil
}

func (s *Seeder) seedModel(ctx context.Context, modelSeed ProviderModelSeed, verbose bool) error {
	// Parse start date
	startDate, err := time.Parse("2006-01-02", modelSeed.StartDate)
	if err != nil {
		return fmt.Errorf("invalid start_date format: %w", err)
	}

	// Check if model already exists (idempotent seeding)
	existing, _ := s.providerModelRepo.GetProviderModelByName(ctx, nil, modelSeed.ModelName)
	if existing != nil {
		if verbose {
			s.logger.Info("Model already exists, skipping", "name", modelSeed.ModelName, "id", existing.ID.String())
		}
		return nil
	}

	// Determine unit (default to TOKENS)
	unit := modelSeed.Unit
	if unit == "" {
		unit = "TOKENS"
	}

	// Create provider model with new UUID
	model := &analytics.ProviderModel{
		ID:           uid.New(),
		ModelName:    modelSeed.ModelName,
		Provider:     modelSeed.Provider,
		MatchPattern: modelSeed.MatchPattern,
		StartDate:    startDate,
		Unit:         unit,
	}

	// Set display name if provided
	if modelSeed.DisplayName != "" {
		model.DisplayName = &modelSeed.DisplayName
	}

	// Set tokenizer fields if provided
	if modelSeed.TokenizerID != "" {
		model.TokenizerID = &modelSeed.TokenizerID
	}
	if len(modelSeed.TokenizerConfig) > 0 {
		model.TokenizerConfig = modelSeed.TokenizerConfig
	}

	// Create model in database
	if err := s.providerModelRepo.CreateProviderModel(ctx, model); err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}

	if verbose {
		s.logger.Info("Created model", "name", model.ModelName, "provider", model.Provider, "id", model.ID.String())
	}

	// Create prices for this model
	for _, priceSeed := range modelSeed.Prices {
		price := &analytics.ProviderPrice{
			ID:              uid.New(),
			ProviderModelID: model.ID,
			UsageType:       priceSeed.UsageType,
			Price:           decimal.NewFromFloat(priceSeed.Price),
		}

		if err := s.providerModelRepo.CreateProviderPrice(ctx, price); err != nil {
			return fmt.Errorf("failed to create price %s: %w", priceSeed.UsageType, err)
		}

		if verbose {
			s.logger.Info("Created price", "usage_type", priceSeed.UsageType, "price", priceSeed.Price)
		}
	}

	return nil
}

func (s *Seeder) resetRBAC(ctx context.Context, verbose bool) error {
	if verbose {
		s.logger.Info("Resetting RBAC data...")
	}

	// Get all roles and delete them
	roles, err := s.roleRepo.GetAllRoles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list roles for reset: %w", err)
	}

	deletedRoles := 0
	for _, role := range roles {
		if err := s.roleRepo.Delete(ctx, role.ID); err != nil {
			s.logger.Warn(" Could not delete role", "name", role.Name, "error", err)
		} else {
			deletedRoles++
			if verbose {
				s.logger.Info("Deleted role", "name", role.Name)
			}
		}
	}

	// Get all permissions and delete them
	permissions, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list permissions for reset: %w", err)
	}

	deletedPerms := 0
	for _, perm := range permissions {
		if err := s.permissionRepo.Delete(ctx, perm.ID); err != nil {
			s.logger.Warn(" Could not delete permission", "resource", perm.Resource, "action", perm.Action, "error", err)
		} else {
			deletedPerms++
			if verbose {
				s.logger.Info("Deleted permission", "resource", perm.Resource, "action", perm.Action)
			}
		}
	}

	if verbose {
		s.logger.Info("RBAC reset completed", "roles_deleted", deletedRoles, "permissions_deleted", deletedPerms)
	}
	return nil
}

func (s *Seeder) resetPricing(ctx context.Context, verbose bool) error {
	if verbose {
		s.logger.Info("Resetting provider pricing data...")
	}

	// Get all models and delete them (cascade deletes prices)
	models, err := s.providerModelRepo.ListProviderModels(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list models for reset: %w", err)
	}

	deletedCount := 0
	for _, model := range models {
		if err := s.providerModelRepo.DeleteProviderModel(ctx, model.ID); err != nil {
			s.logger.Warn(" Could not delete model", "name", model.ModelName, "error", err)
		} else {
			deletedCount++
			if verbose {
				s.logger.Info("Deleted model", "name", model.ModelName)
			}
		}
	}

	if verbose {
		s.logger.Info("Provider pricing reset completed", "models_deleted", deletedCount)
	}
	return nil
}

func (s *Seeder) GetRBACStatistics(ctx context.Context) (*RBACStatistics, error) {
	allRoles, err := s.roleRepo.GetAllRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all roles: %w", err)
	}

	allPerms, err := s.permissionRepo.GetAllPermissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all permissions: %w", err)
	}

	stats := &RBACStatistics{
		TotalRoles:        len(allRoles),
		TotalPermissions:  len(allPerms),
		ScopeDistribution: make(map[string]int),
		RoleDistribution:  make(map[string]int),
	}

	for _, role := range allRoles {
		stats.ScopeDistribution[role.ScopeType]++
		stats.RoleDistribution[role.Name]++
	}

	return stats, nil
}

func (s *Seeder) GetPricingStatistics(ctx context.Context) (*PricingStatistics, error) {
	models, err := s.providerModelRepo.ListProviderModels(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}

	stats := &PricingStatistics{
		TotalModels:          len(models),
		ProviderDistribution: make(map[string]int),
	}

	for _, model := range models {
		// Use Provider field if set, otherwise infer from model name for backwards compatibility
		provider := model.Provider
		if provider == "" {
			provider = InferProvider(model.ModelName)
		}
		stats.ProviderDistribution[provider]++

		prices, err := s.providerModelRepo.GetProviderPrices(ctx, model.ID, nil)
		if err == nil {
			stats.TotalPrices += len(prices)
		}
	}

	return stats, nil
}

// SeedTemplates seeds dashboard templates from the YAML file.
func (s *Seeder) SeedTemplates(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed dashboard templates")
		return nil
	}

	s.logger.Info("Starting dashboard template seeding...")

	// Reset template data if requested
	if opts.Reset {
		s.logger.Info("Resetting existing template data...")
		if err := s.resetTemplates(ctx, opts.Verbose); err != nil {
			return fmt.Errorf("failed to reset template data: %w", err)
		}
	}

	// Load and seed template data
	if err := s.seedTemplatesFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed templates: %w", err)
	}

	// Print statistics
	stats, err := s.GetTemplateStatistics(ctx)
	if err == nil {
		s.logger.Debug("Template Statistics", "total", stats.TotalTemplates)
	}

	s.logger.Info("Dashboard template seeding completed successfully")
	return nil
}

func (s *Seeder) loadTemplates() (*TemplatesFile, error) {
	seedFile := findSeedFile("seeds/dashboard_templates.yaml")
	if seedFile == "" {
		return nil, errors.New("templates file not found: seeds/dashboard_templates.yaml")
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", seedFile, err)
	}

	var templatesFile TemplatesFile
	if err := yaml.Unmarshal(data, &templatesFile); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", seedFile, err)
	}

	// Validate
	if len(templatesFile.Templates) == 0 {
		return nil, errors.New("no templates defined")
	}

	templateNames := make(map[string]bool)
	for i, tmpl := range templatesFile.Templates {
		if tmpl.Name == "" {
			return nil, fmt.Errorf("template %d missing required field: name", i)
		}
		if tmpl.Category == "" {
			return nil, fmt.Errorf("template %s missing required field: category", tmpl.Name)
		}
		if templateNames[tmpl.Name] {
			return nil, fmt.Errorf("duplicate template name: %s", tmpl.Name)
		}
		templateNames[tmpl.Name] = true

		if len(tmpl.Config.Widgets) == 0 {
			return nil, fmt.Errorf("template %s has no widgets defined", tmpl.Name)
		}
	}

	return &templatesFile, nil
}

func (s *Seeder) seedTemplatesFromFile(ctx context.Context, verbose bool) error {
	templatesData, err := s.loadTemplates()
	if err != nil {
		// Template data is optional - log warning but don't fail
		s.logger.Warn("Could not load template data", "error", err)
		return nil
	}

	return s.seedTemplates(ctx, templatesData, verbose)
}

func (s *Seeder) seedTemplates(ctx context.Context, data *TemplatesFile, verbose bool) error {
	if verbose {
		s.logger.Info("Seeding dashboard templates", "count", len(data.Templates))
	}

	for _, tmplSeed := range data.Templates {
		if err := s.seedTemplate(ctx, tmplSeed, verbose); err != nil {
			return fmt.Errorf("failed to seed template %s: %w", tmplSeed.Name, err)
		}
	}

	if verbose {
		s.logger.Info("Dashboard templates seeded successfully", "version", data.Version)
	}
	return nil
}

func (s *Seeder) seedTemplate(ctx context.Context, tmplSeed TemplateSeed, verbose bool) error {
	// Check if template already exists by name (idempotent seeding)
	existing, _ := s.templateRepo.GetByName(ctx, tmplSeed.Name)
	if existing != nil {
		if verbose {
			s.logger.Info("Template already exists, updating", "name", tmplSeed.Name, "id", existing.ID.String())
		}
		// Update existing template
		existing.Description = tmplSeed.Description
		existing.Category = dashboard.TemplateCategory(tmplSeed.Category)
		existing.Config = s.convertConfig(tmplSeed.Config)
		existing.Layout = s.convertLayout(tmplSeed.Layout)
		existing.IsActive = true

		if err := s.templateRepo.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update template: %w", err)
		}
		return nil
	}

	// Create new template
	template := &dashboard.Template{
		ID:          uid.New(),
		Name:        tmplSeed.Name,
		Description: tmplSeed.Description,
		Category:    dashboard.TemplateCategory(tmplSeed.Category),
		Config:      s.convertConfig(tmplSeed.Config),
		Layout:      s.convertLayout(tmplSeed.Layout),
		IsActive:    true,
	}

	if err := s.templateRepo.Create(ctx, template); err != nil {
		return fmt.Errorf("failed to create template: %w", err)
	}

	if verbose {
		s.logger.Info("Created template", "name", template.Name, "category", template.Category, "id", template.ID.String())
	}

	return nil
}

func (s *Seeder) convertConfig(seedConfig TemplateConfigSeed) dashboard.DashboardConfig {
	config := dashboard.DashboardConfig{
		RefreshRate: seedConfig.RefreshRate,
		Widgets:     make([]dashboard.Widget, len(seedConfig.Widgets)),
	}

	for i, w := range seedConfig.Widgets {
		config.Widgets[i] = dashboard.Widget{
			ID:          w.ID,
			Type:        dashboard.WidgetType(w.Type),
			Title:       w.Title,
			Description: w.Description,
			Query:       s.convertQuery(w.Query),
			Config:      w.Config,
		}
	}

	return config
}

func (s *Seeder) convertQuery(seedQuery TemplateQuerySeed) dashboard.WidgetQuery {
	query := dashboard.WidgetQuery{
		View:       dashboard.ViewType(seedQuery.View),
		Measures:   seedQuery.Measures,
		Dimensions: seedQuery.Dimensions,
		Limit:      seedQuery.Limit,
		OrderBy:    seedQuery.OrderBy,
		OrderDir:   seedQuery.OrderDir,
	}

	if len(seedQuery.Filters) > 0 {
		query.Filters = make([]dashboard.QueryFilter, len(seedQuery.Filters))
		for i, f := range seedQuery.Filters {
			query.Filters[i] = dashboard.QueryFilter{
				Field:    f.Field,
				Operator: dashboard.FilterOperator(f.Operator),
				Value:    f.Value,
			}
		}
	}

	return query
}

func (s *Seeder) convertLayout(seedLayout []TemplateLayoutSeed) []dashboard.LayoutItem {
	layout := make([]dashboard.LayoutItem, len(seedLayout))
	for i, l := range seedLayout {
		layout[i] = dashboard.LayoutItem{
			WidgetID: l.WidgetID,
			X:        l.X,
			Y:        l.Y,
			W:        l.W,
			H:        l.H,
		}
	}
	return layout
}

func (s *Seeder) resetTemplates(ctx context.Context, verbose bool) error {
	if verbose {
		s.logger.Info("Resetting dashboard template data...")
	}

	// Get all templates and delete them
	templates, err := s.templateRepo.List(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list templates for reset: %w", err)
	}

	deletedCount := 0
	for _, tmpl := range templates {
		if err := s.templateRepo.Delete(ctx, tmpl.ID); err != nil {
			s.logger.Warn("Could not delete template", "name", tmpl.Name, "error", err)
		} else {
			deletedCount++
			if verbose {
				s.logger.Info("Deleted template", "name", tmpl.Name)
			}
		}
	}

	if verbose {
		s.logger.Info("Dashboard template reset completed", "templates_deleted", deletedCount)
	}
	return nil
}

func (s *Seeder) GetTemplateStatistics(ctx context.Context) (*TemplateStatistics, error) {
	templates, err := s.templateRepo.List(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get templates: %w", err)
	}

	stats := &TemplateStatistics{
		TotalTemplates:       len(templates),
		CategoryDistribution: make(map[string]int),
	}

	for _, tmpl := range templates {
		stats.CategoryDistribution[string(tmpl.Category)]++
	}

	return stats, nil
}

// SeedBillingConfigs seeds billing pricing configurations from the YAML file.
func (s *Seeder) SeedBillingConfigs(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed billing pricing configurations")
		return nil
	}

	s.logger.Info("Starting billing configs seeding...")

	// Reset billing configs if requested
	if opts.Reset {
		s.logger.Info("Resetting existing billing configs data...")
		if err := s.resetBillingConfigs(ctx, opts.Verbose); err != nil {
			return fmt.Errorf("failed to reset billing configs data: %w", err)
		}
	}

	// Load and seed billing configs
	if err := s.seedBillingConfigsFromFile(ctx, opts.Verbose); err != nil {
		return fmt.Errorf("failed to seed billing configs: %w", err)
	}

	// Print statistics
	stats, err := s.GetBillingStatistics(ctx)
	if err == nil {
		s.logger.Debug("Billing Statistics", "plans", stats.TotalPlans, "default", stats.DefaultConfigName)
	}

	s.logger.Info("Billing configs seeding completed successfully")
	return nil
}

func (s *Seeder) loadBillingConfigs() (*BillingConfigsFile, error) {
	seedFile := findSeedFile("seeds/billing_configs.yaml")
	if seedFile == "" {
		return nil, errors.New("billing configs file not found: seeds/billing_configs.yaml")
	}

	data, err := os.ReadFile(seedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", seedFile, err)
	}

	var configsFile BillingConfigsFile
	if err := yaml.Unmarshal(data, &configsFile); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", seedFile, err)
	}

	// Validate
	if len(configsFile.Plans) == 0 {
		return nil, errors.New("no plans defined")
	}

	configNames := make(map[string]bool)
	defaultCount := 0
	for i, cfg := range configsFile.Plans {
		if cfg.Name == "" {
			return nil, fmt.Errorf("pricing config %d missing required field: name", i)
		}
		if configNames[cfg.Name] {
			return nil, fmt.Errorf("duplicate pricing config name: %s", cfg.Name)
		}
		configNames[cfg.Name] = true
		if cfg.IsDefault {
			defaultCount++
		}
	}

	if defaultCount == 0 {
		return nil, errors.New("no default plan defined (exactly one must have is_default: true)")
	}
	if defaultCount > 1 {
		return nil, fmt.Errorf("multiple default plans defined (%d), exactly one must have is_default: true", defaultCount)
	}

	return &configsFile, nil
}

func (s *Seeder) seedBillingConfigsFromFile(ctx context.Context, verbose bool) error {
	configsData, err := s.loadBillingConfigs()
	if err != nil {
		// Billing configs data is optional - log warning but don't fail
		s.logger.Warn("Could not load billing configs data", "error", err)
		return nil
	}

	return s.seedBillingConfigsData(ctx, configsData, verbose)
}

func (s *Seeder) seedBillingConfigsData(ctx context.Context, data *BillingConfigsFile, verbose bool) error {
	if verbose {
		s.logger.Info("Seeding billing plans", "count", len(data.Plans))
	}

	now := time.Now()

	for _, cfgSeed := range data.Plans {
		// Check if plan already exists by name (idempotent seeding)
		existing, _ := s.planRepo.GetByName(ctx, cfgSeed.Name)
		if existing != nil {
			if verbose {
				s.logger.Info("Plan already exists, updating", "name", cfgSeed.Name, "id", existing.ID.String())
			}
			// Update existing plan
			existing.IsDefault = cfgSeed.IsDefault
			existing.FreeSpans = cfgSeed.FreeSpans
			existing.FreeGB = decimal.NewFromFloat(cfgSeed.FreeGB)
			existing.FreeScores = cfgSeed.FreeScores
			existing.PricePer100KSpans = float64PtrToDecimalPtr(cfgSeed.PricePer100KSpans)
			existing.PricePerGB = float64PtrToDecimalPtr(cfgSeed.PricePerGB)
			existing.PricePer1KScores = float64PtrToDecimalPtr(cfgSeed.PricePer1KScores)
			existing.IsActive = true
			existing.UpdatedAt = now

			if err := s.planRepo.Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update plan %s: %w", cfgSeed.Name, err)
			}
			continue
		}

		// Create new plan with runtime UUID
		config := &billing.Plan{
			ID:                uid.New(),
			Name:              cfgSeed.Name,
			IsDefault:         cfgSeed.IsDefault,
			FreeSpans:         cfgSeed.FreeSpans,
			FreeGB:            decimal.NewFromFloat(cfgSeed.FreeGB),
			FreeScores:        cfgSeed.FreeScores,
			PricePer100KSpans: float64PtrToDecimalPtr(cfgSeed.PricePer100KSpans),
			PricePerGB:        float64PtrToDecimalPtr(cfgSeed.PricePerGB),
			PricePer1KScores:  float64PtrToDecimalPtr(cfgSeed.PricePer1KScores),
			IsActive:          true,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if err := s.planRepo.Create(ctx, config); err != nil {
			return fmt.Errorf("failed to create plan %s: %w", cfgSeed.Name, err)
		}

		if verbose {
			s.logger.Info("Created plan", "name", config.Name, "is_default", config.IsDefault, "id", config.ID.String())
		}
	}

	if verbose {
		s.logger.Info("Billing plans seeded successfully", "version", data.Version)
	}
	return nil
}

func (s *Seeder) resetBillingConfigs(ctx context.Context, verbose bool) error {
	if verbose {
		s.logger.Info("Resetting billing plans data...")
	}

	// Get all active plans and deactivate them (soft delete)
	plans, err := s.planRepo.GetActive(ctx)
	if err != nil {
		return fmt.Errorf("failed to list plans for reset: %w", err)
	}

	deactivatedCount := 0
	for _, plan := range plans {
		plan.IsActive = false
		plan.UpdatedAt = time.Now()
		if err := s.planRepo.Update(ctx, plan); err != nil {
			s.logger.Warn("Could not deactivate plan", "name", plan.Name, "error", err)
		} else {
			deactivatedCount++
			if verbose {
				s.logger.Info("Deactivated plan", "name", plan.Name)
			}
		}
	}

	if verbose {
		s.logger.Info("Billing plans reset completed", "plans_deactivated", deactivatedCount)
	}
	return nil
}

func (s *Seeder) GetBillingStatistics(ctx context.Context) (*BillingStatistics, error) {
	plans, err := s.planRepo.GetActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get plans: %w", err)
	}

	stats := &BillingStatistics{
		TotalPlans: len(plans),
	}

	for _, plan := range plans {
		if plan.IsDefault {
			stats.DefaultConfigName = plan.Name
			break
		}
	}

	return stats, nil
}

// SeedOrganizationBillings provisions billing records for existing organizations that don't have one.
func (s *Seeder) SeedOrganizationBillings(ctx context.Context, opts *Options) error {
	if opts.DryRun {
		fmt.Println("DRY RUN: Would seed billing for existing organizations")
		return nil
	}

	s.logger.Info("Starting organization billing provisioning...")

	// Get default plan
	defaultPlan, err := s.planRepo.GetDefault(ctx)
	if err != nil {
		return fmt.Errorf("failed to get default plan: %w", err)
	}

	// Find organizations without billing records using raw SQL
	type orgWithoutBilling struct {
		ID   string
		Name string
	}

	var orgs []orgWithoutBilling
	rows, err := s.tm.DB(ctx).Query(ctx, `
		SELECT o.id, o.name
		FROM organizations o
		LEFT JOIN organization_billings ob ON o.id = ob.organization_id
		WHERE ob.organization_id IS NULL AND o.deleted_at IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to find organizations without billing: %w", err)
	}
	for rows.Next() {
		var o orgWithoutBilling
		if err := rows.Scan(&o.ID, &o.Name); err != nil {
			rows.Close()
			return fmt.Errorf("scan organization: %w", err)
		}
		orgs = append(orgs, o)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate organizations: %w", err)
	}

	if len(orgs) == 0 {
		s.logger.Info("All organizations already have billing records")
		return nil
	}

	s.logger.Info("Found organizations without billing", "count", len(orgs))

	now := time.Now()
	seededCount := 0

	for _, org := range orgs {
		// Parse the org ID
		orgID, err := uuid.Parse(org.ID)
		if err != nil {
			s.logger.Warn("Invalid organization ID, skipping", "id", org.ID, "error", err)
			continue
		}

		// Create billing record with default plan
		billingRecord := &billing.OrganizationBilling{
			OrganizationID:        orgID,
			PlanID:                defaultPlan.ID,
			BillingCycleStart:     now,
			BillingCycleAnchorDay: 1,
			FreeSpansRemaining:    defaultPlan.FreeSpans,
			FreeBytesRemaining:    defaultPlan.FreeGB.Mul(decimal.NewFromInt(1024 * 1024 * 1024)).IntPart(),
			FreeScoresRemaining:   defaultPlan.FreeScores,
			CurrentPeriodSpans:    0,
			CurrentPeriodBytes:    0,
			CurrentPeriodScores:   0,
			CurrentPeriodCost:     decimal.Zero,
			LastSyncedAt:          now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}

		if err := s.orgBillingRepo.Create(ctx, billingRecord); err != nil {
			s.logger.Warn("Failed to create billing for organization", "org_id", org.ID, "org_name", org.Name, "error", err)
			continue
		}

		seededCount++
		if opts.Verbose {
			s.logger.Info("Provisioned billing for organization", "org_id", org.ID, "org_name", org.Name, "plan", defaultPlan.Name)
		}
	}

	s.logger.Info("Organization billing provisioning completed", "seeded", seededCount, "total", len(orgs))
	return nil
}

// float64PtrToDecimalPtr converts a *float64 to *decimal.Decimal
func float64PtrToDecimalPtr(f *float64) *decimal.Decimal {
	if f == nil {
		return nil
	}
	d := decimal.NewFromFloat(*f)
	return &d
}
