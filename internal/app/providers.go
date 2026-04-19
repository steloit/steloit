package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"brokle/internal/config"
	"brokle/internal/core/domain/analytics"
	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/core/domain/auth"
	"brokle/internal/core/domain/billing"
	commentDomain "brokle/internal/core/domain/comment"
	"brokle/internal/core/domain/common"
	credentialsDomain "brokle/internal/core/domain/credentials"
	dashboardDomain "brokle/internal/core/domain/dashboard"
	evaluationDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/observability"
	"brokle/internal/core/domain/organization"
	playgroundDomain "brokle/internal/core/domain/playground"
	promptDomain "brokle/internal/core/domain/prompt"
	storageDomain "brokle/internal/core/domain/storage"
	"brokle/internal/core/domain/user"
	websiteDomain "brokle/internal/core/domain/website"
	analyticsService "brokle/internal/core/services/analytics"
	annotationService "brokle/internal/core/services/annotation"
	authService "brokle/internal/core/services/auth"
	billingService "brokle/internal/core/services/billing"
	commentService "brokle/internal/core/services/comment"
	credentialsService "brokle/internal/core/services/credentials"
	dashboardService "brokle/internal/core/services/dashboard"
	evaluationService "brokle/internal/core/services/evaluation"
	observabilityService "brokle/internal/core/services/observability"
	orgService "brokle/internal/core/services/organization"
	playgroundService "brokle/internal/core/services/playground"
	promptService "brokle/internal/core/services/prompt"
	registrationService "brokle/internal/core/services/registration"
	storageService "brokle/internal/core/services/storage"
	userService "brokle/internal/core/services/user"
	websiteServicePkg "brokle/internal/core/services/website"
	eeAnalytics "brokle/internal/ee/analytics"
	"brokle/internal/ee/compliance"
	"brokle/internal/ee/rbac"
	"brokle/internal/ee/sso"
	"brokle/internal/infrastructure/database"
	"brokle/internal/infrastructure/db"
	analyticsRepo "brokle/internal/infrastructure/repository/analytics"
	annotationRepo "brokle/internal/infrastructure/repository/annotation"
	authRepo "brokle/internal/infrastructure/repository/auth"
	billingRepo "brokle/internal/infrastructure/repository/billing"
	commentRepo "brokle/internal/infrastructure/repository/comment"
	credentialsRepo "brokle/internal/infrastructure/repository/credentials"
	dashboardRepo "brokle/internal/infrastructure/repository/dashboard"
	evaluationRepo "brokle/internal/infrastructure/repository/evaluation"
	observabilityRepo "brokle/internal/infrastructure/repository/observability"
	orgRepo "brokle/internal/infrastructure/repository/organization"
	playgroundRepo "brokle/internal/infrastructure/repository/playground"
	promptRepo "brokle/internal/infrastructure/repository/prompt"
	storageRepo "brokle/internal/infrastructure/repository/storage"
	userRepo "brokle/internal/infrastructure/repository/user"
	websiteRepo "brokle/internal/infrastructure/repository/website"
	"brokle/internal/infrastructure/storage"
	"brokle/internal/infrastructure/streams"
	"brokle/internal/server"
	grpcTransport "brokle/internal/transport/grpc"
	"brokle/internal/workers"
	annotationWorker "brokle/internal/workers/annotation"
	evaluationWorker "brokle/internal/workers/evaluation"
	"brokle/pkg/email"
	"brokle/pkg/encryption"
	"brokle/pkg/uid"
)

type DeploymentMode string

const (
	ModeServer DeploymentMode = "server"
	ModeWorker DeploymentMode = "worker"
)

type CoreContainer struct {
	Config     *config.Config
	Logger     *slog.Logger
	Databases  *DatabaseContainer
	Repos      *RepositoryContainer
	Transactor common.Transactor
	Services   *ServiceContainer
	Enterprise *EnterpriseContainer
}

type ServerContainer struct {
	HTTPServer *server.Server
	GRPCServer *grpcTransport.Server
}

type ProviderContainer struct {
	Core    *CoreContainer
	Server  *ServerContainer // nil in worker mode
	Workers *WorkerContainer // nil in server mode
	Mode    DeploymentMode
}

type DatabaseContainer struct {
	// Pool is the process-wide pgx v5 pool owned by this container.
	// All repositories access PostgreSQL through TxManager, which wraps
	// this pool; there is no second handle.
	Pool       *pgxpool.Pool
	TxManager  *db.TxManager
	Redis      *database.RedisDB
	ClickHouse *database.ClickHouseDB
}

type WorkerContainer struct {
	TelemetryConsumer        *workers.TelemetryStreamConsumer
	EvaluatorWorker          *evaluationWorker.EvaluatorWorker
	EvaluationWorker         *evaluationWorker.EvaluationWorker
	ManualTriggerWorker      *evaluationWorker.ManualTriggerWorker
	UsageAggregationWorker   *workers.UsageAggregationWorker
	ContractExpirationWorker *workers.ContractExpirationWorker
	LockExpiryWorker         *annotationWorker.LockExpiryWorker
}

type RepositoryContainer struct {
	User          *UserRepositories
	Auth          *AuthRepositories
	Organization  *OrganizationRepositories
	Observability *ObservabilityRepositories
	Storage       *StorageRepositories
	Billing       *BillingRepositories
	Analytics     *AnalyticsRepositories
	Prompt        *PromptRepositories
	Credentials   *CredentialsRepositories
	Playground    *PlaygroundRepositories
	Evaluation    *EvaluationRepositories
	Dashboard     *DashboardRepositories
	Annotation    *AnnotationRepositories
	Website       *WebsiteRepositories
}

type ServiceContainer struct {
	User                *UserServices
	Auth                *AuthServices
	Registration        registrationService.RegistrationService
	OrganizationService organization.OrganizationService
	MemberService       organization.MemberService
	ProjectService      organization.ProjectService
	InvitationService   organization.InvitationService
	SettingsService     organization.OrganizationSettingsService
	Observability       *observabilityService.ServiceRegistry
	Billing             *BillingServices
	Analytics           *AnalyticsServices
	Prompt              *PromptServices
	Credentials         *CredentialsServices
	Playground          *PlaygroundServices
	Evaluation          *EvaluationServices
	Dashboard           *DashboardServices
	Annotation          *AnnotationServices
	Comment             commentDomain.Service
	Website             websiteDomain.WebsiteService
}

type EnterpriseContainer struct {
	SSO        sso.SSOProvider
	RBAC       rbac.RBACManager
	Compliance compliance.Compliance
	Analytics  eeAnalytics.EnterpriseAnalytics
}

type UserRepositories struct {
	User user.Repository
}

type AuthRepositories struct {
	UserSession        auth.UserSessionRepository
	BlacklistedToken   auth.BlacklistedTokenRepository
	PasswordResetToken auth.PasswordResetTokenRepository
	APIKey             auth.APIKeyRepository
	Role               auth.RoleRepository
	OrganizationMember auth.OrganizationMemberRepository
	Permission         auth.PermissionRepository
	RolePermission     auth.RolePermissionRepository
	AuditLog           auth.AuditLogRepository
}

type OrganizationRepositories struct {
	Organization organization.OrganizationRepository
	Member       organization.MemberRepository
	Project      organization.ProjectRepository
	Invitation   organization.InvitationRepository
	Settings     organization.OrganizationSettingsRepository
}

type ObservabilityRepositories struct {
	Trace                  observability.TraceRepository
	Score                  observability.ScoreRepository
	ScoreAnalytics         observability.ScoreAnalyticsRepository
	Metrics                observability.MetricsRepository
	Logs                   observability.LogsRepository
	GenAIEvents            observability.GenAIEventsRepository
	TelemetryDeduplication observability.TelemetryDeduplicationRepository
	FilterPreset           observability.FilterPresetRepository
}

type StorageRepositories struct {
	BlobStorage storageDomain.BlobStorageRepository
}

type BillingRepositories struct {
	BillingRecord billing.BillingRecordRepository
	// Usage-based billing repositories (Spans + GB + Scores)
	BillableUsage       billing.BillableUsageRepository
	Plan                billing.PlanRepository
	OrganizationBilling billing.OrganizationBillingRepository
	UsageBudget         billing.UsageBudgetRepository
	UsageAlert          billing.UsageAlertRepository
	// Enterprise custom pricing repositories
	Contract        billing.ContractRepository
	VolumeTier      billing.VolumeDiscountTierRepository
	ContractHistory billing.ContractHistoryRepository
}

type AnalyticsRepositories struct {
	ProviderModel analytics.ProviderModelRepository
	Overview      analytics.OverviewRepository
}

type PromptRepositories struct {
	Prompt         promptDomain.PromptRepository
	Version        promptDomain.VersionRepository
	Label          promptDomain.LabelRepository
	ProtectedLabel promptDomain.ProtectedLabelRepository
	Cache          promptDomain.CacheRepository
}

type CredentialsRepositories struct {
	ProviderCredential credentialsDomain.ProviderCredentialRepository
}

type PlaygroundRepositories struct {
	Session playgroundDomain.SessionRepository
}

type EvaluationRepositories struct {
	ScoreConfig        evaluationDomain.ScoreConfigRepository
	Dataset            evaluationDomain.DatasetRepository
	DatasetItem        evaluationDomain.DatasetItemRepository
	DatasetVersion     evaluationDomain.DatasetVersionRepository
	Experiment         evaluationDomain.ExperimentRepository
	ExperimentItem     evaluationDomain.ExperimentItemRepository
	ExperimentConfig   evaluationDomain.ExperimentConfigRepository
	Evaluator          evaluationDomain.EvaluatorRepository
	EvaluatorExecution evaluationDomain.EvaluatorExecutionRepository
}

type DashboardRepositories struct {
	Dashboard   dashboardDomain.DashboardRepository
	WidgetQuery dashboardDomain.WidgetQueryRepository
	Template    dashboardDomain.TemplateRepository
}

type AnnotationRepositories struct {
	Queue      annotationDomain.QueueRepository
	Item       annotationDomain.ItemRepository
	Assignment annotationDomain.AssignmentRepository
}

type WebsiteRepositories struct {
	ContactSubmission websiteDomain.ContactSubmissionRepository
}

type UserServices struct {
	User    user.UserService
	Profile user.ProfileService
}

type AuthServices struct {
	Auth                auth.AuthService
	JWT                 auth.JWTService
	Sessions            auth.SessionService
	APIKey              auth.APIKeyService
	Role                auth.RoleService
	Permission          auth.PermissionService
	OrganizationMembers auth.OrganizationMemberService
	BlacklistedTokens   auth.BlacklistedTokenService
	Scope               auth.ScopeService
	OAuthProvider       *authService.OAuthProviderService
}

type BillingServices struct {
	// Usage-based billing services (Spans + GB + Scores)
	BillableUsage billing.BillableUsageService
	Budget        billing.BudgetService
	// Enterprise custom pricing services
	Pricing  billing.PricingService
	Contract billing.ContractService
}

type AnalyticsServices struct {
	ProviderPricing analytics.ProviderPricingService
	Overview        analytics.OverviewService
}

type PromptServices struct {
	Prompt    promptDomain.PromptService
	Compiler  promptDomain.CompilerService
	Execution promptDomain.ExecutionService
}

type CredentialsServices struct {
	ProviderCredential credentialsDomain.ProviderCredentialService
	ModelCatalog       credentialsService.ModelCatalogService
}

type PlaygroundServices struct {
	Playground playgroundDomain.PlaygroundService
}

type EvaluationServices struct {
	ScoreConfig        evaluationDomain.ScoreConfigService
	Dataset            evaluationDomain.DatasetService
	DatasetItem        evaluationDomain.DatasetItemService
	DatasetVersion     evaluationDomain.DatasetVersionService
	Experiment         evaluationDomain.ExperimentService
	ExperimentItem     evaluationDomain.ExperimentItemService
	ExperimentWizard   evaluationDomain.ExperimentWizardService
	Evaluator          evaluationDomain.EvaluatorService
	EvaluatorExecution evaluationDomain.EvaluatorExecutionService
}

type DashboardServices struct {
	Dashboard   dashboardDomain.DashboardService
	WidgetQuery dashboardDomain.WidgetQueryService
	Template    dashboardDomain.TemplateService
}

type AnnotationServices struct {
	Queue      annotationDomain.QueueService
	Item       annotationDomain.ItemService
	Assignment annotationDomain.AssignmentService
}

func ProvideDatabases(cfg *config.Config, logger *slog.Logger) (*DatabaseContainer, error) {
	pool, err := db.NewPool(context.Background(), cfg, logger)
	if err != nil {
		return nil, err
	}

	redis, err := database.NewRedisDB(cfg, logger)
	if err != nil {
		pool.Close()
		return nil, err
	}

	clickhouse, err := database.NewClickHouseDB(cfg, logger)
	if err != nil {
		_ = redis.Close()
		pool.Close()
		return nil, err
	}

	return &DatabaseContainer{
		Pool:       pool,
		TxManager:  db.NewTxManager(pool),
		Redis:      redis,
		ClickHouse: clickhouse,
	}, nil
}

func ProvideWorkers(core *CoreContainer) (*WorkerContainer, error) {
	deduplicationService := observabilityService.NewTelemetryDeduplicationService(
		core.Repos.Observability.TelemetryDeduplication,
	)

	consumerConfig := &workers.TelemetryStreamConsumerConfig{
		ConsumerGroup:     "telemetry-workers",
		ConsumerID:        "worker-" + uid.New().String(),
		BatchSize:         50,
		BlockDuration:     time.Second,
		MaxRetries:        3,
		RetryBackoff:      500 * time.Millisecond,
		DiscoveryInterval: 30 * time.Second,
		MaxStreamsPerRead: 10,
	}

	telemetryConsumer := workers.NewTelemetryStreamConsumer(
		core.Databases.Redis,
		deduplicationService,
		core.Logger,
		consumerConfig,
		core.Services.Observability.TraceService,
		core.Services.Observability.ScoreService,
		core.Services.Observability.MetricsService,
		core.Services.Observability.LogsService,
		core.Services.Observability.GenAIEventsService,
		core.Services.Observability.ArchiveService, // S3 raw telemetry archival (nil if disabled)
		&core.Config.Archive,                       // Archive config
	)

	// Create evaluator worker using config
	discoveryInterval, _ := time.ParseDuration(core.Config.Workers.EvaluatorWorker.DiscoveryInterval)
	if discoveryInterval == 0 {
		discoveryInterval = 30 * time.Second
	}
	evaluatorCacheTTL, _ := time.ParseDuration(core.Config.Workers.EvaluatorWorker.EvaluatorCacheTTL)
	if evaluatorCacheTTL == 0 {
		evaluatorCacheTTL = 30 * time.Second
	}

	evaluatorWorkerConfig := &evaluationWorker.EvaluatorWorkerConfig{
		ConsumerGroup:     "evaluator-workers",
		ConsumerID:        "evaluator-worker-" + uid.New().String(),
		BatchSize:         core.Config.Workers.EvaluatorWorker.BatchSize,
		BlockDuration:     time.Duration(core.Config.Workers.EvaluatorWorker.BlockDurationMs) * time.Millisecond,
		MaxRetries:        core.Config.Workers.EvaluatorWorker.MaxRetries,
		RetryBackoff:      time.Duration(core.Config.Workers.EvaluatorWorker.RetryBackoffMs) * time.Millisecond,
		DiscoveryInterval: discoveryInterval,
		MaxStreamsPerRead: core.Config.Workers.EvaluatorWorker.MaxStreamsPerRead,
		EvaluatorCacheTTL: evaluatorCacheTTL,
	}

	evaluatorWorker := evaluationWorker.NewEvaluatorWorker(
		core.Databases.Redis,
		core.Services.Evaluation.Evaluator,
		core.Services.Evaluation.EvaluatorExecution,
		core.Logger,
		evaluatorWorkerConfig,
	)

	// Create scorers for evaluation worker
	builtinScorer := evaluationWorker.NewBuiltinScorer(core.Logger)
	regexScorer := evaluationWorker.NewRegexScorer(core.Logger)

	// LLM scorer requires credentials and execution services
	var llmScorer evaluationWorker.Scorer
	if core.Services.Credentials != nil && core.Services.Prompt != nil {
		llmScorer = evaluationWorker.NewLLMScorer(
			core.Services.Credentials.ProviderCredential,
			core.Services.Prompt.Execution,
			core.Logger,
		)
		core.Logger.Info("LLM scorer initialized for evaluation worker")
	} else {
		core.Logger.Warn("LLM scorer disabled: credentials or prompt services not available")
	}

	// Create evaluation worker
	evalWorkerConfig := &evaluationWorker.EvaluationWorkerConfig{
		ConsumerGroup:  "evaluation-execution-workers",
		ConsumerID:     "eval-worker-" + uid.New().String(),
		BatchSize:      10,
		BlockDuration:  time.Second,
		MaxRetries:     3,
		RetryBackoff:   500 * time.Millisecond,
		MaxConcurrency: 5,
	}

	evalWorker := evaluationWorker.NewEvaluationWorker(
		core.Databases.Redis,
		core.Services.Observability.ScoreService,
		core.Services.Evaluation.EvaluatorExecution,
		llmScorer,
		builtinScorer,
		regexScorer,
		core.Logger,
		evalWorkerConfig,
	)

	manualTriggerWorkerConfig := &evaluationWorker.ManualTriggerWorkerConfig{
		ConsumerGroup:  "manual-trigger-workers",
		ConsumerID:     "manual-trigger-" + uid.New().String(),
		BlockDuration:  time.Second,
		MaxRetries:     3,
		RetryBackoff:   500 * time.Millisecond,
		MaxConcurrency: 3,
	}

	manualTriggerWorker := evaluationWorker.NewManualTriggerWorker(
		core.Databases.Redis,
		core.Services.Observability.TraceService,
		core.Services.Evaluation.EvaluatorExecution,
		core.Logger,
		manualTriggerWorkerConfig,
	)

	// Create usage aggregation worker for billing (syncs ClickHouse → PostgreSQL)
	usageAggWorker := workers.NewUsageAggregationWorker(
		core.Config,
		core.Logger,
		core.Transactor, // Transaction support for atomic billing + budget updates
		core.Repos.Billing.BillableUsage,
		core.Repos.Billing.OrganizationBilling,
		core.Repos.Billing.UsageBudget,
		core.Repos.Billing.UsageAlert,
		core.Repos.Organization.Organization,
		core.Services.Billing.Pricing, // PricingService for effective pricing and tier calculations
		nil,                           // NotificationWorker - can be wired for email notifications
	)

	// Create contract expiration worker (daily job to expire contracts past end_date)
	contractExpWorker := workers.NewContractExpirationWorker(
		core.Config,
		core.Logger,
		core.Services.Billing.Contract,
		core.Repos.Billing.OrganizationBilling,
	)

	// Create annotation lock expiry worker (every minute, releases stale locks)
	lockExpiryWorker := annotationWorker.NewLockExpiryWorker(
		core.Logger,
		core.Repos.Annotation.Queue,
		core.Repos.Annotation.Item,
	)

	return &WorkerContainer{
		TelemetryConsumer:        telemetryConsumer,
		EvaluatorWorker:          evaluatorWorker,
		EvaluationWorker:         evalWorker,
		ManualTriggerWorker:      manualTriggerWorker,
		UsageAggregationWorker:   usageAggWorker,
		ContractExpirationWorker: contractExpWorker,
		LockExpiryWorker:         lockExpiryWorker,
	}, nil
}

func ProvideCore(cfg *config.Config, logger *slog.Logger) (*CoreContainer, error) {
	databases, err := ProvideDatabases(cfg, logger)
	if err != nil {
		return nil, err
	}

	repos := ProvideRepositories(databases, logger)

	// The TxManager satisfies common.Transactor; services that need
	// cross-aggregate writes (e.g. registration, annotation) take the
	// transactor interface, and the sqlc-backed TxManager binds it.
	transactor := databases.TxManager

	return &CoreContainer{
		Config:     cfg,
		Logger:     logger,
		Databases:  databases,
		Repos:      repos,
		Transactor: transactor, // Stored as common.Transactor interface
		Services:   nil,        // Populated by mode-specific provider
		Enterprise: nil,        // Populated by mode-specific provider
	}, nil
}

func ProvideServerServices(core *CoreContainer) *ServiceContainer {
	cfg := core.Config
	logger := core.Logger
	repos := core.Repos
	databases := core.Databases

	billingServices := ProvideBillingServices(core.Transactor, repos.Billing, repos.Organization, logger)
	analyticsServices := ProvideAnalyticsServices(repos.Analytics)
	observabilityServices := ProvideObservabilityServices(repos.Observability, repos.Storage, analyticsServices, databases.Redis, cfg, logger)
	authServices := ProvideAuthServices(cfg, repos.User, repos.Auth, repos.Organization, databases, logger)
	userServices := ProvideUserServices(repos.User, repos.Auth, logger)
	orgService, memberService, projectService, invitationService, settingsService :=
		ProvideOrganizationServices(repos.User, repos.Auth, repos.Organization, repos.Billing, authServices, cfg, logger)

	// Orchestrates user, org, project creation atomically
	registrationSvc := registrationService.NewRegistrationService(
		core.Transactor,
		repos.User.User,
		repos.Organization.Organization,
		repos.Organization.Member,
		repos.Organization.Project,
		repos.Organization.Invitation,
		authServices.Role,
		authServices.Auth,
		billingServices.BillableUsage,
	)

	promptServices := ProvidePromptServices(core.Transactor, repos.Prompt, analyticsServices.ProviderPricing, cfg, logger)

	// Config validation ensures AI_KEY_ENCRYPTION_KEY is valid, so credentials service is guaranteed to initialize
	credentialsServices := ProvideCredentialsServices(repos.Credentials, repos.Analytics, cfg, logger)

	playgroundServices := ProvidePlaygroundServices(
		repos.Playground,
		credentialsServices.ProviderCredential,
		promptServices.Compiler,
		promptServices.Execution,
		logger,
	)

	evaluationServices := ProvideEvaluationServices(core.Transactor, repos.Evaluation, repos.Observability, observabilityServices, repos.Prompt, databases.Redis, logger)

	dashboardServices := ProvideDashboardServices(repos.Dashboard, logger)

	annotationServices := ProvideAnnotationServices(core.Transactor, repos.Annotation, evaluationServices, observabilityServices, repos.Organization, logger)

	// Comment service for trace/span comments (with reactions support)
	commentSvc := commentService.NewCommentService(
		commentRepo.NewCommentRepository(databases.TxManager),
		commentRepo.NewReactionRepository(databases.TxManager),
		repos.Observability.Trace,
		logger,
	)

	// Website service (contact form) - reuses the same email sender as invitations
	websiteEmailSender, err := createEmailSender(&cfg.External.Email, logger)
	if err != nil {
		logger.Warn("failed to create email sender for website service, notifications disabled", "error", err)
		websiteEmailSender = &email.NoOpEmailSender{}
	}
	websiteSvc := websiteServicePkg.NewWebsiteService(
		repos.Website.ContactSubmission,
		websiteEmailSender,
		cfg.Notifications.WebsiteNotificationEmail,
		logger,
	)

	// Overview service needs projectService and credentials repo (created after other services)
	overviewSvc := analyticsService.NewOverviewService(
		repos.Analytics.Overview,
		projectService,
		repos.Credentials.ProviderCredential,
		logger,
	)
	analyticsServices.Overview = overviewSvc

	return &ServiceContainer{
		User:                userServices,
		Auth:                authServices,
		Registration:        registrationSvc,
		OrganizationService: orgService,
		MemberService:       memberService,
		ProjectService:      projectService,
		InvitationService:   invitationService,
		SettingsService:     settingsService,
		Observability:       observabilityServices,
		Billing:             billingServices,
		Analytics:           analyticsServices,
		Prompt:              promptServices,
		Credentials:         credentialsServices,
		Playground:          playgroundServices,
		Evaluation:          evaluationServices,
		Dashboard:           dashboardServices,
		Annotation:          annotationServices,
		Comment:             commentSvc,
		Website:             websiteSvc,
	}
}

func ProvideWorkerServices(core *CoreContainer) *ServiceContainer {
	cfg := core.Config
	logger := core.Logger
	repos := core.Repos
	databases := core.Databases

	billingServices := ProvideBillingServices(core.Transactor, repos.Billing, repos.Organization, logger)
	analyticsServices := ProvideAnalyticsServices(repos.Analytics)
	observabilityServices := ProvideObservabilityServices(repos.Observability, repos.Storage, analyticsServices, databases.Redis, cfg, logger)

	// Prompt services needed for LLM scorer
	promptServices := ProvidePromptServices(core.Transactor, repos.Prompt, analyticsServices.ProviderPricing, cfg, logger)

	// Credentials services needed for LLM scorer (optional - only if encryption key configured)
	var credentialsServices *CredentialsServices
	if cfg.Encryption.AIKeyEncryptionKey != "" {
		credentialsServices = ProvideCredentialsServices(repos.Credentials, repos.Analytics, cfg, logger)
	} else {
		logger.Warn("AI_KEY_ENCRYPTION_KEY not configured, LLM scorer will be disabled")
	}

	// Evaluation services needed for evaluator worker
	evaluationServices := ProvideEvaluationServices(core.Transactor, repos.Evaluation, repos.Observability, observabilityServices, repos.Prompt, databases.Redis, logger)

	return &ServiceContainer{
		User:                nil, // Worker doesn't need auth/user/org services
		Auth:                nil,
		Registration:        nil,
		OrganizationService: nil,
		MemberService:       nil,
		ProjectService:      nil,
		InvitationService:   nil,
		SettingsService:     nil,
		Prompt:              promptServices,      // Needed for LLM scorer
		Credentials:         credentialsServices, // Needed for LLM scorer
		Playground:          nil,
		Observability:       observabilityServices,
		Billing:             billingServices,
		Analytics:           analyticsServices,
		Evaluation:          evaluationServices, // Needed for evaluator worker
	}
}

func ProvideServer(core *CoreContainer) (*ServerContainer, error) {
	// Get credentials services (may be nil if encryption key not configured)
	var credentialsSvc credentialsDomain.ProviderCredentialService
	var modelCatalogSvc credentialsService.ModelCatalogService
	if core.Services.Credentials != nil {
		credentialsSvc = core.Services.Credentials.ProviderCredential
		modelCatalogSvc = core.Services.Credentials.ModelCatalog
	}

	// Get playground service
	var playgroundSvc playgroundDomain.PlaygroundService
	if core.Services.Playground != nil {
		playgroundSvc = core.Services.Playground.Playground
	}

	// Get evaluation services
	var scoreConfigSvc evaluationDomain.ScoreConfigService
	var datasetSvc evaluationDomain.DatasetService
	var datasetItemSvc evaluationDomain.DatasetItemService
	var datasetVersionSvc evaluationDomain.DatasetVersionService
	var experimentSvc evaluationDomain.ExperimentService
	var experimentItemSvc evaluationDomain.ExperimentItemService
	var experimentWizardSvc evaluationDomain.ExperimentWizardService
	var evaluatorSvc evaluationDomain.EvaluatorService
	var evaluatorExecutionSvc evaluationDomain.EvaluatorExecutionService
	if core.Services.Evaluation != nil {
		scoreConfigSvc = core.Services.Evaluation.ScoreConfig
		datasetSvc = core.Services.Evaluation.Dataset
		datasetItemSvc = core.Services.Evaluation.DatasetItem
		datasetVersionSvc = core.Services.Evaluation.DatasetVersion
		experimentSvc = core.Services.Evaluation.Experiment
		experimentItemSvc = core.Services.Evaluation.ExperimentItem
		experimentWizardSvc = core.Services.Evaluation.ExperimentWizard
		evaluatorSvc = core.Services.Evaluation.Evaluator
		evaluatorExecutionSvc = core.Services.Evaluation.EvaluatorExecution
	}

	// Get dashboard services
	var dashboardSvc dashboardDomain.DashboardService
	var widgetQuerySvc dashboardDomain.WidgetQueryService
	var templateSvc dashboardDomain.TemplateService
	if core.Services.Dashboard != nil {
		dashboardSvc = core.Services.Dashboard.Dashboard
		widgetQuerySvc = core.Services.Dashboard.WidgetQuery
		templateSvc = core.Services.Dashboard.Template
	}

	// Build the server dep set. Each handler domain that has been
	// converted to a Huma operation (Step 4 of the chi+Huma migration)
	// adds its service to the Deps struct here and self-registers via
	// RegisterRoutes in internal/server/routes.go. Un-converted
	// handler domains are NOT listed here — they will appear as they
	// migrate, not before (CLAUDE.md scaffolded-but-unreachable rule).
	//
	// Suppress unused variables for services whose handler domain
	// hasn't been ported yet. They are referenced by the core service
	// wiring above and will land in Deps as their domains migrate.
	_ = credentialsSvc
	_ = modelCatalogSvc
	_ = playgroundSvc
	_ = scoreConfigSvc
	_ = datasetSvc
	_ = datasetItemSvc
	_ = datasetVersionSvc
	_ = experimentSvc
	_ = experimentItemSvc
	_ = experimentWizardSvc
	_ = evaluatorSvc
	_ = evaluatorExecutionSvc
	_ = dashboardSvc
	_ = widgetQuerySvc
	_ = templateSvc

	httpServer, err := server.New(server.Deps{
		Config:     core.Config,
		Logger:     core.Logger,
		DB:         core.Databases.Pool,
		Redis:      core.Databases.Redis.Client,
		ClickHouse: core.Databases.ClickHouse.Conn,
		JWT:        core.Services.Auth.JWT,
		Blacklist:  core.Services.Auth.BlacklistedTokens,
		OrgMember:  core.Services.Auth.OrganizationMembers,
		APIKey:     core.Services.Auth.APIKey,
		Project:    core.Services.ProjectService,
		Auth:          core.Services.Auth.Auth,
		User:          core.Services.User.User,
		Profile:       core.Services.User.Profile,
		Registration:  core.Services.Registration,
		Session:       core.Services.Auth.Sessions,
		OAuthProvider: core.Services.Auth.OAuthProvider,
		Website:       core.Services.Website,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP server: %w", err)
	}

	slogLogger := core.Logger

	grpcOTLPHandler := grpcTransport.NewOTLPHandler(
		core.Services.Observability.StreamProducer,
		core.Services.Observability.DeduplicationService,
		core.Services.Observability.OTLPConverterService,
		slogLogger,
	)

	grpcOTLPMetricsHandler := grpcTransport.NewOTLPMetricsHandler(
		core.Services.Observability.StreamProducer,
		core.Services.Observability.OTLPMetricsConverterService,
		slogLogger,
	)

	grpcOTLPLogsHandler := grpcTransport.NewOTLPLogsHandler(
		core.Services.Observability.StreamProducer,
		core.Services.Observability.OTLPLogsConverterService,
		core.Services.Observability.OTLPEventsConverterService,
		slogLogger,
	)

	grpcAuthInterceptor := grpcTransport.NewAuthInterceptor(
		core.Services.Auth.APIKey,
		slogLogger,
	)

	grpcServer, err := grpcTransport.NewServer(
		core.Config.GRPC.Port,
		grpcOTLPHandler,
		grpcOTLPMetricsHandler,
		grpcOTLPLogsHandler,
		grpcAuthInterceptor,
		slogLogger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC server: %w", err)
	}

	core.Logger.Info("gRPC OTLP server initialized", "port", core.Config.GRPC.Port)

	return &ServerContainer{
		HTTPServer: httpServer,
		GRPCServer: grpcServer,
	}, nil
}

func ProvideUserRepositories(tm *db.TxManager) *UserRepositories {
	return &UserRepositories{
		User: userRepo.NewUserRepository(tm),
	}
}

func ProvideAuthRepositories(tm *db.TxManager) *AuthRepositories {
	return &AuthRepositories{
		UserSession:        authRepo.NewUserSessionRepository(tm),
		BlacklistedToken:   authRepo.NewBlacklistedTokenRepository(tm),
		PasswordResetToken: authRepo.NewPasswordResetTokenRepository(tm),
		APIKey:             authRepo.NewAPIKeyRepository(tm),
		Role:               authRepo.NewRoleRepository(tm),
		OrganizationMember: authRepo.NewOrganizationMemberRepository(tm),
		Permission:         authRepo.NewPermissionRepository(tm),
		RolePermission:     authRepo.NewRolePermissionRepository(tm),
		AuditLog:           authRepo.NewAuditLogRepository(tm),
	}
}

func ProvideOrganizationRepositories(tm *db.TxManager) *OrganizationRepositories {
	return &OrganizationRepositories{
		Organization: orgRepo.NewOrganizationRepository(tm),
		Member:       orgRepo.NewMemberRepository(tm),
		Project:      orgRepo.NewProjectRepository(tm),
		Invitation:   orgRepo.NewInvitationRepository(tm),
		Settings:     orgRepo.NewOrganizationSettingsRepository(tm),
	}
}

func ProvideObservabilityRepositories(clickhouseDB *database.ClickHouseDB, tm *db.TxManager, redisDB *database.RedisDB) *ObservabilityRepositories {
	return &ObservabilityRepositories{
		Trace:                  observabilityRepo.NewTraceRepository(clickhouseDB.Conn),
		Score:                  observabilityRepo.NewScoreRepository(clickhouseDB.Conn),
		ScoreAnalytics:         observabilityRepo.NewScoreAnalyticsRepository(clickhouseDB.Conn),
		Metrics:                observabilityRepo.NewMetricsRepository(clickhouseDB.Conn),
		Logs:                   observabilityRepo.NewLogsRepository(clickhouseDB.Conn),
		GenAIEvents:            observabilityRepo.NewGenAIEventsRepository(clickhouseDB.Conn),
		TelemetryDeduplication: observabilityRepo.NewTelemetryDeduplicationRepository(redisDB),
		FilterPreset:           observabilityRepo.NewFilterPresetRepository(tm),
	}
}

func ProvideStorageRepositories(clickhouseDB *database.ClickHouseDB) *StorageRepositories {
	return &StorageRepositories{
		BlobStorage: storageRepo.NewBlobStorageRepository(clickhouseDB.Conn),
	}
}

func ProvideBillingRepositories(tm *db.TxManager, clickhouseDB *database.ClickHouseDB, logger *slog.Logger) *BillingRepositories {
	return &BillingRepositories{
		BillingRecord: billingRepo.NewBillingRecordRepository(tm, logger),
		// Usage-based billing repositories
		BillableUsage:       billingRepo.NewBillableUsageRepository(clickhouseDB.Conn),
		Plan:                billingRepo.NewPlanRepository(tm),
		OrganizationBilling: billingRepo.NewOrganizationBillingRepository(tm),
		UsageBudget:         billingRepo.NewUsageBudgetRepository(tm),
		UsageAlert:          billingRepo.NewUsageAlertRepository(tm),
		// Enterprise custom pricing repositories
		Contract:        billingRepo.NewContractRepository(tm),
		VolumeTier:      billingRepo.NewVolumeDiscountTierRepository(tm),
		ContractHistory: billingRepo.NewContractHistoryRepository(tm),
	}
}

func ProvideAnalyticsRepositories(tm *db.TxManager, clickhouseDB *database.ClickHouseDB) *AnalyticsRepositories {
	return &AnalyticsRepositories{
		ProviderModel: analyticsRepo.NewProviderModelRepository(tm),
		Overview:      analyticsRepo.NewOverviewRepository(clickhouseDB.Conn),
	}
}

func ProvidePromptRepositories(tm *db.TxManager, redisDB *database.RedisDB) *PromptRepositories {
	return &PromptRepositories{
		Prompt:         promptRepo.NewPromptRepository(tm),
		Version:        promptRepo.NewVersionRepository(tm),
		Label:          promptRepo.NewLabelRepository(tm),
		ProtectedLabel: promptRepo.NewProtectedLabelRepository(tm),
		Cache:          promptRepo.NewCacheRepository(redisDB),
	}
}

func ProvideCredentialsRepositories(tm *db.TxManager) *CredentialsRepositories {
	return &CredentialsRepositories{
		ProviderCredential: credentialsRepo.NewProviderCredentialRepository(tm),
	}
}

func ProvidePlaygroundRepositories(tm *db.TxManager) *PlaygroundRepositories {
	return &PlaygroundRepositories{
		Session: playgroundRepo.NewSessionRepository(tm),
	}
}

func ProvideEvaluationRepositories(tm *db.TxManager) *EvaluationRepositories {
	return &EvaluationRepositories{
		ScoreConfig:        evaluationRepo.NewScoreConfigRepository(tm),
		Dataset:            evaluationRepo.NewDatasetRepository(tm),
		DatasetItem:        evaluationRepo.NewDatasetItemRepository(tm),
		DatasetVersion:     evaluationRepo.NewDatasetVersionRepository(tm),
		Experiment:         evaluationRepo.NewExperimentRepository(tm),
		ExperimentItem:     evaluationRepo.NewExperimentItemRepository(tm),
		ExperimentConfig:   evaluationRepo.NewExperimentConfigRepository(tm),
		Evaluator:          evaluationRepo.NewEvaluatorRepository(tm),
		EvaluatorExecution: evaluationRepo.NewEvaluatorExecutionRepository(tm),
	}
}

func ProvideDashboardRepositories(tm *db.TxManager, clickhouseDB *database.ClickHouseDB) *DashboardRepositories {
	return &DashboardRepositories{
		Dashboard:   dashboardRepo.NewDashboardRepository(tm),
		WidgetQuery: dashboardRepo.NewWidgetQueryRepository(clickhouseDB.Conn),
		Template:    dashboardRepo.NewTemplateRepository(tm),
	}
}

func ProvideAnnotationRepositories(tm *db.TxManager) *AnnotationRepositories {
	return &AnnotationRepositories{
		Queue:      annotationRepo.NewQueueRepository(tm),
		Item:       annotationRepo.NewItemRepository(tm),
		Assignment: annotationRepo.NewAssignmentRepository(tm),
	}
}

func ProvideWebsiteRepositories(tm *db.TxManager) *WebsiteRepositories {
	return &WebsiteRepositories{
		ContactSubmission: websiteRepo.NewContactSubmissionRepository(tm),
	}
}

func ProvideRepositories(dbs *DatabaseContainer, logger *slog.Logger) *RepositoryContainer {
	return &RepositoryContainer{
		User:          ProvideUserRepositories(dbs.TxManager),
		Auth:          ProvideAuthRepositories(dbs.TxManager),
		Organization:  ProvideOrganizationRepositories(dbs.TxManager),
		Observability: ProvideObservabilityRepositories(dbs.ClickHouse, dbs.TxManager, dbs.Redis),
		Storage:       ProvideStorageRepositories(dbs.ClickHouse),
		Billing:       ProvideBillingRepositories(dbs.TxManager, dbs.ClickHouse, logger),
		Analytics:     ProvideAnalyticsRepositories(dbs.TxManager, dbs.ClickHouse),
		Prompt:        ProvidePromptRepositories(dbs.TxManager, dbs.Redis),
		Credentials:   ProvideCredentialsRepositories(dbs.TxManager),
		Playground:    ProvidePlaygroundRepositories(dbs.TxManager),
		Evaluation:    ProvideEvaluationRepositories(dbs.TxManager),
		Dashboard:     ProvideDashboardRepositories(dbs.TxManager, dbs.ClickHouse),
		Annotation:    ProvideAnnotationRepositories(dbs.TxManager),
		Website:       ProvideWebsiteRepositories(dbs.TxManager),
	}
}

func ProvideUserServices(
	userRepos *UserRepositories,
	authRepos *AuthRepositories,
	logger *slog.Logger,
) *UserServices {
	userSvc := userService.NewUserService(
		userRepos.User,
		nil,
		authRepos.OrganizationMember,
	)

	profileSvc := userService.NewProfileService(
		userRepos.User,
	)

	return &UserServices{
		User:    userSvc,
		Profile: profileSvc,
	}
}

func ProvideAuthServices(
	cfg *config.Config,
	userRepos *UserRepositories,
	authRepos *AuthRepositories,
	orgRepos *OrganizationRepositories,
	databases *DatabaseContainer,
	logger *slog.Logger,
) *AuthServices {
	jwtService, err := authService.NewJWTService(&cfg.Auth)
	if err != nil {
		logger.Error("Failed to create JWT service", "error", err)
		os.Exit(1)
	}

	permissionService := authService.NewPermissionService(
		authRepos.Permission,
		authRepos.RolePermission,
	)

	roleService := authService.NewRoleService(
		authRepos.Role,
		authRepos.RolePermission,
	)

	orgMemberService := authService.NewOrganizationMemberService(
		authRepos.OrganizationMember,
		authRepos.Role,
	)

	blacklistedTokenService := authService.NewBlacklistedTokenService(
		authRepos.BlacklistedToken,
	)

	sessionService := authService.NewSessionService(
		&cfg.Auth,
		authRepos.UserSession,
		userRepos.User,
		jwtService,
	)

	apiKeyService := authService.NewAPIKeyService(
		authRepos.APIKey,
		authRepos.OrganizationMember,
		orgRepos.Project,
	)

	coreAuthSvc := authService.NewAuthService(
		&cfg.Auth,
		userRepos.User,
		authRepos.UserSession,
		jwtService,
		roleService,
		authRepos.PasswordResetToken,
		blacklistedTokenService,
		databases.Redis.Client,
	)

	// Audit decorator for clean separation of concerns
	authSvc := authService.NewAuditDecorator(coreAuthSvc, authRepos.AuditLog, logger)

	scopeService := authService.NewScopeService(
		authRepos.OrganizationMember,
		authRepos.Role,
		authRepos.Permission,
	)

	frontendURL := "http://localhost:3000"
	if url := os.Getenv("NEXT_PUBLIC_APP_URL"); url != "" {
		frontendURL = url
	}
	oauthProvider := authService.NewOAuthProviderService(
		&cfg.Auth,
		databases.Redis.Client,
		frontendURL,
	)

	return &AuthServices{
		Auth:                authSvc,
		JWT:                 jwtService,
		Sessions:            sessionService,
		APIKey:              apiKeyService,
		Role:                roleService,
		Permission:          permissionService,
		OrganizationMembers: orgMemberService,
		BlacklistedTokens:   blacklistedTokenService,
		Scope:               scopeService,
		OAuthProvider:       oauthProvider,
	}
}

func ProvideOrganizationServices(
	userRepos *UserRepositories,
	authRepos *AuthRepositories,
	orgRepos *OrganizationRepositories,
	billingRepos *BillingRepositories,
	authServices *AuthServices,
	cfg *config.Config,
	logger *slog.Logger,
) (
	organization.OrganizationService,
	organization.MemberService,
	organization.ProjectService,
	organization.InvitationService,
	organization.OrganizationSettingsService,
) {
	memberSvc := orgService.NewMemberService(
		orgRepos.Member,
		orgRepos.Organization,
		userRepos.User,
		authServices.Role,
	)

	projectSvc := orgService.NewProjectService(
		orgRepos.Project,
		orgRepos.Organization,
		orgRepos.Member,
	)

	// Create email sender based on configuration
	emailSender, err := createEmailSender(&cfg.External.Email, logger)
	if err != nil {
		logger.Error("failed to create email sender", "error", err)
		os.Exit(1)
	}

	invitationSvc := orgService.NewInvitationService(
		orgRepos.Invitation,
		orgRepos.Organization,
		orgRepos.Member,
		userRepos.User,
		authServices.Role,
		emailSender,
		orgService.InvitationServiceConfig{
			AppURL: cfg.Server.AppURL,
		},
		logger.With("service", "invitation"),
	)

	orgSvc := orgService.NewOrganizationService(
		orgRepos.Organization,
		userRepos.User,
		memberSvc,
		projectSvc,
		authServices.Role,
		billingRepos.OrganizationBilling,
		billingRepos.Plan,
		logger,
	)

	settingsSvc := orgService.NewOrganizationSettingsService(
		orgRepos.Settings,
		orgRepos.Member,
	)

	return orgSvc, memberSvc, projectSvc, invitationSvc, settingsSvc
}

// TelemetryService created without analytics worker - inject via SetAnalyticsWorker() after startup.
func ProvideObservabilityServices(
	observabilityRepos *ObservabilityRepositories,
	storageRepos *StorageRepositories,
	analyticsServices *AnalyticsServices,
	redisDB *database.RedisDB,
	cfg *config.Config,
	logger *slog.Logger,
) *observabilityService.ServiceRegistry {
	deduplicationService := observabilityService.NewTelemetryDeduplicationService(observabilityRepos.TelemetryDeduplication)
	streamProducer := streams.NewTelemetryStreamProducer(redisDB, logger)
	telemetryService := observabilityService.NewTelemetryService(
		deduplicationService,
		streamProducer,
		logger,
	)

	var s3Client *storage.S3Client
	if cfg.BlobStorage.Provider != "" && cfg.BlobStorage.BucketName != "" {
		var err error
		s3Client, err = storage.NewS3Client(&cfg.BlobStorage, logger)
		if err != nil {
			logger.Warn("Failed to initialize S3 client, blob storage will be disabled", "error", err)
		}
	}

	blobStorageSvc := storageService.NewBlobStorageService(
		storageRepos.BlobStorage,
		s3Client,
		&cfg.BlobStorage,
		logger,
	)

	return observabilityService.NewServiceRegistry(
		observabilityRepos.Trace,
		observabilityRepos.Score,
		observabilityRepos.ScoreAnalytics,
		observabilityRepos.Metrics,
		observabilityRepos.Logs,
		observabilityRepos.GenAIEvents,
		observabilityRepos.FilterPreset,
		blobStorageSvc,
		s3Client,
		&cfg.Archive, // Archive config for S3 raw telemetry archival
		streamProducer,
		deduplicationService,
		telemetryService,
		analyticsServices.ProviderPricing,
		&cfg.Observability,
		logger,
	)
}

func ProvideBillingServices(
	transactor common.Transactor,
	billingRepos *BillingRepositories,
	orgRepos *OrganizationRepositories,
	logger *slog.Logger,
) *BillingServices {
	// Enterprise custom pricing service (contract overrides + volume tiers)
	pricingService := billingService.NewPricingService(
		billingRepos.OrganizationBilling,
		billingRepos.Plan,
		billingRepos.Contract,
		billingRepos.VolumeTier,
		logger,
	)

	// Usage-based billing services (Spans + GB + Scores)
	billableUsageService := billingService.NewBillableUsageService(
		billingRepos.BillableUsage,
		billingRepos.OrganizationBilling,
		pricingService,
		billingRepos.Plan,
		logger,
	)

	budgetService := billingService.NewBudgetService(
		billingRepos.UsageBudget,
		billingRepos.UsageAlert,
		orgRepos.Project,
		logger,
	)

	// Contract lifecycle service (enterprise custom pricing)
	contractService := billingService.NewContractService(
		transactor,
		billingRepos.Contract,
		billingRepos.VolumeTier,
		billingRepos.ContractHistory,
		billingRepos.OrganizationBilling,
		logger,
	)

	return &BillingServices{
		BillableUsage: billableUsageService,
		Budget:        budgetService,
		Pricing:       pricingService,
		Contract:      contractService,
	}
}

func ProvideAnalyticsServices(
	analyticsRepos *AnalyticsRepositories,
) *AnalyticsServices {
	providerPricingServiceImpl := analyticsService.NewProviderPricingService(analyticsRepos.ProviderModel)

	return &AnalyticsServices{
		ProviderPricing: providerPricingServiceImpl,
	}
}

func ProvidePromptServices(
	transactor common.Transactor,
	promptRepos *PromptRepositories,
	pricingService analytics.ProviderPricingService,
	cfg *config.Config,
	logger *slog.Logger,
) *PromptServices {
	compilerSvc := promptService.NewCompilerService()
	aiClientConfig := &promptService.AIClientConfig{
		DefaultTimeout: cfg.External.LLMTimeout,
	}

	executionSvc := promptService.NewExecutionService(compilerSvc, pricingService, aiClientConfig)
	promptSvc := promptService.NewPromptService(
		transactor,
		promptRepos.Prompt,
		promptRepos.Version,
		promptRepos.Label,
		promptRepos.ProtectedLabel,
		promptRepos.Cache,
		compilerSvc,
		logger,
	)

	return &PromptServices{
		Prompt:    promptSvc,
		Compiler:  compilerSvc,
		Execution: executionSvc,
	}
}

func ProvideCredentialsServices(
	credentialsRepos *CredentialsRepositories,
	analyticsRepos *AnalyticsRepositories,
	cfg *config.Config,
	logger *slog.Logger,
) *CredentialsServices {
	encryptor, err := encryption.NewServiceFromBase64(cfg.Encryption.AIKeyEncryptionKey)
	if err != nil {
		panic(fmt.Sprintf("encryption initialization failed after config validation: %v (this is a bug)", err))
	}

	providerSvc := credentialsService.NewProviderCredentialService(
		credentialsRepos.ProviderCredential,
		encryptor,
		logger,
	)

	// Model catalog combines default models from DB with custom models from credentials
	modelCatalogSvc := credentialsService.NewModelCatalogService(
		credentialsRepos.ProviderCredential,
		analyticsRepos.ProviderModel,
		logger,
	)

	return &CredentialsServices{
		ProviderCredential: providerSvc,
		ModelCatalog:       modelCatalogSvc,
	}
}

func ProvidePlaygroundServices(
	playgroundRepos *PlaygroundRepositories,
	credentialsService credentialsDomain.ProviderCredentialService,
	compilerService promptDomain.CompilerService,
	executionService promptDomain.ExecutionService,
	logger *slog.Logger,
) *PlaygroundServices {
	playgroundSvc := playgroundService.NewPlaygroundService(
		playgroundRepos.Session,
		credentialsService,
		compilerService,
		executionService,
		logger,
	)

	return &PlaygroundServices{
		Playground: playgroundSvc,
	}
}

func ProvideEvaluationServices(
	transactor common.Transactor,
	evaluationRepos *EvaluationRepositories,
	observabilityRepos *ObservabilityRepositories,
	observabilityServices *observabilityService.ServiceRegistry,
	promptRepos *PromptRepositories,
	redisDB *database.RedisDB,
	logger *slog.Logger,
) *EvaluationServices {
	scoreConfigSvc := evaluationService.NewScoreConfigService(
		evaluationRepos.ScoreConfig,
		observabilityRepos.Score,
		logger,
	)

	datasetSvc := evaluationService.NewDatasetService(
		evaluationRepos.Dataset,
		logger,
	)

	datasetItemSvc := evaluationService.NewDatasetItemService(
		evaluationRepos.DatasetItem,
		evaluationRepos.Dataset,
		observabilityRepos.Trace,
		logger,
	)

	datasetVersionSvc := evaluationService.NewDatasetVersionService(
		transactor,
		evaluationRepos.DatasetVersion,
		evaluationRepos.Dataset,
		evaluationRepos.DatasetItem,
		logger,
	)

	experimentSvc := evaluationService.NewExperimentService(
		evaluationRepos.Experiment,
		evaluationRepos.Dataset,
		observabilityRepos.Score,
		logger,
	)

	experimentItemSvc := evaluationService.NewExperimentItemService(
		evaluationRepos.ExperimentItem,
		evaluationRepos.Experiment,
		evaluationRepos.DatasetItem,
		observabilityServices.ScoreService,
		logger,
	)

	experimentWizardSvc := evaluationService.NewExperimentWizardService(
		transactor,
		evaluationRepos.Experiment,
		evaluationRepos.ExperimentConfig,
		evaluationRepos.Dataset,
		evaluationRepos.DatasetItem,
		evaluationRepos.DatasetVersion,
		promptRepos.Prompt,
		promptRepos.Version,
		logger,
	)

	// EvaluatorExecutionService must be created before EvaluatorService since EvaluatorService depends on it
	evaluatorExecutionSvc := evaluationService.NewEvaluatorExecutionService(
		evaluationRepos.EvaluatorExecution,
		logger,
	)

	evaluatorSvc := evaluationService.NewEvaluatorService(
		evaluationRepos.Evaluator,
		evaluatorExecutionSvc,
		observabilityRepos.Trace,
		redisDB,
		logger,
	)

	return &EvaluationServices{
		ScoreConfig:        scoreConfigSvc,
		Dataset:            datasetSvc,
		DatasetItem:        datasetItemSvc,
		DatasetVersion:     datasetVersionSvc,
		Experiment:         experimentSvc,
		ExperimentItem:     experimentItemSvc,
		ExperimentWizard:   experimentWizardSvc,
		Evaluator:          evaluatorSvc,
		EvaluatorExecution: evaluatorExecutionSvc,
	}
}

func ProvideDashboardServices(
	dashboardRepos *DashboardRepositories,
	logger *slog.Logger,
) *DashboardServices {
	dashboardSvc := dashboardService.NewDashboardService(
		dashboardRepos.Dashboard,
		logger,
	)

	widgetQuerySvc := dashboardService.NewWidgetQueryService(
		dashboardRepos.Dashboard,
		dashboardRepos.WidgetQuery,
		logger,
	)

	templateSvc := dashboardService.NewTemplateService(
		dashboardRepos.Template,
		dashboardRepos.Dashboard,
		logger,
	)

	return &DashboardServices{
		Dashboard:   dashboardSvc,
		WidgetQuery: widgetQuerySvc,
		Template:    templateSvc,
	}
}

func ProvideAnnotationServices(
	transactor common.Transactor,
	annotationRepos *AnnotationRepositories,
	evaluationServices *EvaluationServices,
	observabilityServices *observabilityService.ServiceRegistry,
	orgRepos *OrganizationRepositories,
	logger *slog.Logger,
) *AnnotationServices {
	queueSvc := annotationService.NewQueueService(
		annotationRepos.Queue,
		annotationRepos.Item,
		annotationRepos.Assignment,
		logger,
	)

	itemSvc := annotationService.NewItemService(
		annotationRepos.Queue,
		annotationRepos.Item,
		annotationRepos.Assignment,
		evaluationServices.ScoreConfig,
		observabilityServices.ScoreService,
		orgRepos.Project,
		transactor,
		logger,
	)

	assignmentSvc := annotationService.NewAssignmentService(
		annotationRepos.Queue,
		annotationRepos.Assignment,
		logger,
	)

	return &AnnotationServices{
		Queue:      queueSvc,
		Item:       itemSvc,
		Assignment: assignmentSvc,
	}
}

func ProvideEnterpriseServices(cfg *config.Config, logger *slog.Logger) *EnterpriseContainer {
	return &EnterpriseContainer{
		SSO:        sso.New(),         // Uses stub or real based on build tags
		RBAC:       rbac.New(),        // Uses stub or real based on build tags
		Compliance: compliance.New(),  // Uses stub or real based on build tags
		Analytics:  eeAnalytics.New(), // Uses stub or real based on build tags
	}
}

func (pc *ProviderContainer) HealthCheck() map[string]string {
	health := make(map[string]string)

	if pc.Core != nil && pc.Core.Databases != nil {
		if pc.Core.Databases.Pool != nil {
			if err := pc.Core.Databases.Pool.Ping(context.Background()); err != nil {
				health["postgres"] = "unhealthy: " + err.Error()
			} else {
				health["postgres"] = "healthy"
			}
		}

		if pc.Core.Databases.Redis != nil {
			if err := pc.Core.Databases.Redis.Health(); err != nil {
				health["redis"] = "unhealthy: " + err.Error()
			} else {
				health["redis"] = "healthy"
			}
		}

		if pc.Core.Databases.ClickHouse != nil {
			if err := pc.Core.Databases.ClickHouse.Health(); err != nil {
				health["clickhouse"] = "unhealthy: " + err.Error()
			} else {
				health["clickhouse"] = "healthy"
			}
		}
	}

	if pc.Workers != nil && pc.Workers.TelemetryConsumer != nil {
		stats := pc.Workers.TelemetryConsumer.GetStats()
		// Consider healthy if: running (has stats) and error rate < 10% of processed batches
		batchesProcessed := stats["batches_processed"]
		errorsCount := stats["errors_count"]

		if batchesProcessed == 0 && errorsCount == 0 {
			// Newly started - healthy
			health["telemetry_stream_consumer"] = "healthy (no activity yet)"
		} else if batchesProcessed > 0 {
			errorRate := float64(errorsCount) / float64(batchesProcessed)
			if errorRate < 0.10 { // Less than 10% error rate
				health["telemetry_stream_consumer"] = fmt.Sprintf("healthy (processed: %d, errors: %d, streams: %d)",
					batchesProcessed, errorsCount, stats["active_streams"])
			} else {
				health["telemetry_stream_consumer"] = fmt.Sprintf("degraded (high error rate: %.1f%%)", errorRate*100)
			}
		} else {
			// Errors but no successful processing
			health["telemetry_stream_consumer"] = fmt.Sprintf("unhealthy (errors: %d, no successful processing)", errorsCount)
		}
	}

	// Evaluator worker health
	if pc.Workers != nil && pc.Workers.EvaluatorWorker != nil {
		stats := pc.Workers.EvaluatorWorker.GetStats()
		spansProcessed := stats["spans_processed"]
		errorsCount := stats["errors_count"]

		if spansProcessed == 0 && errorsCount == 0 {
			health["evaluator_worker"] = "healthy (no activity yet)"
		} else if spansProcessed > 0 {
			health["evaluator_worker"] = fmt.Sprintf("healthy (spans_processed: %d, jobs_emitted: %d, errors: %d)",
				spansProcessed, stats["jobs_emitted"], errorsCount)
		} else {
			health["evaluator_worker"] = fmt.Sprintf("unhealthy (errors: %d)", errorsCount)
		}
	}

	// Evaluation worker health
	if pc.Workers != nil && pc.Workers.EvaluationWorker != nil {
		stats := pc.Workers.EvaluationWorker.GetStats()
		jobsProcessed := stats["jobs_processed"]
		errorsCount := stats["errors_count"]

		if jobsProcessed == 0 && errorsCount == 0 {
			health["evaluation_worker"] = "healthy (no activity yet)"
		} else if jobsProcessed > 0 {
			health["evaluation_worker"] = fmt.Sprintf("healthy (processed: %d, scores: %d, llm: %d, builtin: %d, regex: %d)",
				jobsProcessed, stats["scores_created"], stats["llm_calls"], stats["builtin_calls"], stats["regex_calls"])
		} else {
			health["evaluation_worker"] = fmt.Sprintf("unhealthy (errors: %d)", errorsCount)
		}
	}

	health["mode"] = string(pc.Mode)

	return health
}

func (pc *ProviderContainer) Shutdown() error {
	var lastErr error
	logger := pc.Core.Logger

	if pc.Workers != nil {
		if pc.Workers.TelemetryConsumer != nil {
			logger.Info("Stopping telemetry stream consumer...")
			pc.Workers.TelemetryConsumer.Stop()
			logger.Info("Telemetry stream consumer stopped")
		}

		if pc.Workers.EvaluatorWorker != nil {
			logger.Info("Stopping evaluator worker...")
			pc.Workers.EvaluatorWorker.Stop()
			logger.Info("Evaluator worker stopped")
		}

		if pc.Workers.EvaluationWorker != nil {
			logger.Info("Stopping evaluation worker...")
			pc.Workers.EvaluationWorker.Stop()
			logger.Info("Evaluation worker stopped")
		}
	}

	if pc.Core != nil && pc.Core.Databases != nil {
		if pc.Core.Databases.Pool != nil {
			logger.Info("Closing pgx pool...")
			pc.Core.Databases.Pool.Close()
		}

		if pc.Core.Databases.Redis != nil {
			if err := pc.Core.Databases.Redis.Close(); err != nil {
				logger.Error("Failed to close Redis connection", "error", err)
				lastErr = err
			}
		}

		if pc.Core.Databases.ClickHouse != nil {
			if err := pc.Core.Databases.ClickHouse.Close(); err != nil {
				logger.Error("Failed to close ClickHouse connection", "error", err)
				lastErr = err
			}
		}
	}

	return lastErr
}

// createEmailSender creates an email sender based on the configured provider.
// Returns NoOpEmailSender if no provider is configured (email disabled).
func createEmailSender(cfg *config.EmailConfig, logger *slog.Logger) (email.EmailSender, error) {
	if cfg.Provider == "" {
		logger.Warn("email sender not configured, invitations will not be sent via email")
		return &email.NoOpEmailSender{}, nil
	}

	logger.Info("initializing email sender", "provider", cfg.Provider)

	switch cfg.Provider {
	case "resend":
		return email.NewResendClient(email.ResendConfig{
			APIKey:    cfg.ResendAPIKey,
			FromEmail: cfg.FromEmail,
			FromName:  cfg.FromName,
		}), nil

	case "smtp":
		return email.NewSMTPClient(email.SMTPConfig{
			Host:      cfg.SMTPHost,
			Port:      cfg.SMTPPort,
			Username:  cfg.SMTPUsername,
			Password:  cfg.SMTPPassword,
			FromEmail: cfg.FromEmail,
			FromName:  cfg.FromName,
			UseTLS:    cfg.SMTPUseTLS,
		}), nil

	case "ses":
		client, err := email.NewSESClient(email.SESConfig{
			Region:    cfg.SESRegion,
			AccessKey: cfg.SESAccessKey,
			SecretKey: cfg.SESSecretKey,
			FromEmail: cfg.FromEmail,
			FromName:  cfg.FromName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create SES client: %w", err)
		}
		return client, nil

	case "sendgrid":
		return email.NewSendGridClient(email.SendGridConfig{
			APIKey:    cfg.SendGridAPIKey,
			FromEmail: cfg.FromEmail,
			FromName:  cfg.FromName,
		}), nil

	default:
		return nil, fmt.Errorf("unknown email provider: %s", cfg.Provider)
	}
}
