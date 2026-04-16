package handlers

import (
	"log/slog"

	"brokle/internal/config"
	analyticsDomain "brokle/internal/core/domain/analytics"
	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/core/domain/auth"
	billingDomain "brokle/internal/core/domain/billing"
	commentDomain "brokle/internal/core/domain/comment"
	credentialsDomain "brokle/internal/core/domain/credentials"
	dashboardDomain "brokle/internal/core/domain/dashboard"
	evaluationDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/core/domain/organization"
	playgroundDomain "brokle/internal/core/domain/playground"
	promptDomain "brokle/internal/core/domain/prompt"
	"brokle/internal/core/domain/user"
	websiteDomain "brokle/internal/core/domain/website"
	authService "brokle/internal/core/services/auth"
	credentialsService "brokle/internal/core/services/credentials"
	obsServices "brokle/internal/core/services/observability"
	"brokle/internal/core/services/registration"
	"brokle/internal/transport/http/handlers/admin"
	"brokle/internal/transport/http/handlers/analytics"
	annotationHandler "brokle/internal/transport/http/handlers/annotation"
	"brokle/internal/transport/http/handlers/apikey"
	authHandler "brokle/internal/transport/http/handlers/auth"
	"brokle/internal/transport/http/handlers/billing"
	commentHandler "brokle/internal/transport/http/handlers/comment"
	"brokle/internal/transport/http/handlers/credentials"
	"brokle/internal/transport/http/handlers/dashboard"
	evaluationHandler "brokle/internal/transport/http/handlers/evaluation"
	"brokle/internal/transport/http/handlers/health"
	"brokle/internal/transport/http/handlers/logs"
	"brokle/internal/transport/http/handlers/metrics"
	"brokle/internal/transport/http/handlers/observability"
	organizationHandler "brokle/internal/transport/http/handlers/organization"
	"brokle/internal/transport/http/handlers/overview"
	"brokle/internal/transport/http/handlers/playground"
	"brokle/internal/transport/http/handlers/project"
	"brokle/internal/transport/http/handlers/prompt"
	"brokle/internal/transport/http/handlers/rbac"
	userHandler "brokle/internal/transport/http/handlers/user"
	websiteHandler "brokle/internal/transport/http/handlers/website"
	"brokle/internal/transport/http/handlers/websocket"
)

type Handlers struct {
	Health             *health.Handler
	Metrics            *metrics.Handler
	Auth               *authHandler.Handler
	User               *userHandler.Handler
	Organization       *organizationHandler.Handler
	Project            *project.Handler
	APIKey             *apikey.Handler
	Analytics          *analytics.Handler
	Logs               *logs.Handler
	Billing            *billing.Handler
	WebSocket          *websocket.Handler
	Admin              *admin.TokenAdminHandler
	RBAC               *rbac.Handler
	Observability      *observability.Handler
	OTLP               *observability.OTLPHandler
	OTLPMetrics        *observability.OTLPMetricsHandler
	OTLPLogs           *observability.OTLPLogsHandler
	Prompt             *prompt.Handler
	Playground         *playground.Handler
	SDKPlayground      *playground.SDKPlaygroundHandler
	Credentials        *credentials.Handler
	Evaluation         *evaluationHandler.ScoreConfigHandler
	SDKScore           *evaluationHandler.SDKScoreHandler
	Dataset            *evaluationHandler.DatasetHandler
	DatasetItem        *evaluationHandler.DatasetItemHandler
	DatasetVersion     *evaluationHandler.DatasetVersionHandler
	Experiment         *evaluationHandler.ExperimentHandler
	ExperimentItem     *evaluationHandler.ExperimentItemHandler
	ExperimentWizard   *evaluationHandler.ExperimentWizardHandler
	Evaluator          *evaluationHandler.EvaluatorHandler
	EvaluatorExecution *evaluationHandler.EvaluatorExecutionHandler
	SpanQuery          *observability.SpanQueryHandler
	Dashboard          *dashboard.Handler
	DashboardTemplate  *dashboard.TemplateHandler
	Overview           *overview.Handler
	// Usage-based billing handlers (Spans + GB + Scores)
	Usage    *billing.UsageHandler
	Budget   *billing.BudgetHandler
	Contract *billing.ContractHandler
	// Annotation queue handlers (HITL evaluation)
	AnnotationQueue      *annotationHandler.QueueHandler
	AnnotationItem       *annotationHandler.ItemHandler
	AnnotationAssignment *annotationHandler.AssignmentHandler
	// Comment handlers
	Comment *commentHandler.Handler
	// Website handlers (public contact form)
	Website *websiteHandler.Handler
}

func NewHandlers(
	cfg *config.Config,
	logger *slog.Logger,
	authSvc auth.AuthService,
	apiKeyService auth.APIKeyService,
	blacklistedTokens auth.BlacklistedTokenService,
	registrationService registration.RegistrationService,
	oauthProvider *authService.OAuthProviderService,
	userService user.UserService,
	profileService user.ProfileService,
	organizationService organization.OrganizationService,
	memberService organization.MemberService,
	projectService organization.ProjectService,
	invitationService organization.InvitationService,
	settingsService organization.OrganizationSettingsService,
	roleService auth.RoleService,
	permissionService auth.PermissionService,
	organizationMemberService auth.OrganizationMemberService,
	scopeService auth.ScopeService,
	observabilityServices *obsServices.ServiceRegistry,
	promptService promptDomain.PromptService,
	compilerService promptDomain.CompilerService,
	credentialsSvc credentialsDomain.ProviderCredentialService,
	modelCatalogSvc credentialsService.ModelCatalogService,
	playgroundService playgroundDomain.PlaygroundService,
	scoreConfigService evaluationDomain.ScoreConfigService,
	datasetService evaluationDomain.DatasetService,
	datasetItemService evaluationDomain.DatasetItemService,
	datasetVersionService evaluationDomain.DatasetVersionService,
	experimentService evaluationDomain.ExperimentService,
	experimentItemService evaluationDomain.ExperimentItemService,
	experimentWizardService evaluationDomain.ExperimentWizardService,
	evaluatorService evaluationDomain.EvaluatorService,
	evaluatorExecutionService evaluationDomain.EvaluatorExecutionService,
	dashboardService dashboardDomain.DashboardService,
	widgetQueryService dashboardDomain.WidgetQueryService,
	templateService dashboardDomain.TemplateService,
	overviewService analyticsDomain.OverviewService,
	// Usage-based billing services
	usageService billingDomain.BillableUsageService,
	budgetService billingDomain.BudgetService,
	// Enterprise custom pricing services
	contractService billingDomain.ContractService,
	pricingService billingDomain.PricingService,
	// Annotation queue services (HITL evaluation)
	annotationQueueService annotationDomain.QueueService,
	annotationItemService annotationDomain.ItemService,
	annotationAssignmentService annotationDomain.AssignmentService,
	// Comment service
	commentService commentDomain.Service,
	// Website service (public contact form)
	websiteService websiteDomain.WebsiteService,
) *Handlers {
	return &Handlers{
		Health:             health.NewHandler(cfg, logger),
		Metrics:            metrics.NewHandler(cfg, logger),
		Auth:               authHandler.NewHandler(cfg, logger, authSvc, apiKeyService, userService, registrationService, oauthProvider),
		User:               userHandler.NewHandler(cfg, logger, userService, profileService, organizationService),
		Organization:       organizationHandler.NewHandler(cfg, logger, organizationService, memberService, projectService, invitationService, settingsService, userService, roleService),
		Project:            project.NewHandler(cfg, logger, projectService, organizationService, memberService),
		APIKey:             apikey.NewHandler(cfg, logger, apiKeyService),
		Analytics:          analytics.NewHandler(cfg, logger),
		Logs:               logs.NewHandler(cfg, logger),
		Billing:            billing.NewHandler(cfg, logger),
		WebSocket:          websocket.NewHandler(cfg, logger),
		Admin:              admin.NewTokenAdminHandler(authSvc, blacklistedTokens, logger),
		RBAC:               rbac.NewHandler(cfg, logger, roleService, permissionService, organizationMemberService, scopeService),
		Observability:      observability.NewHandler(cfg, logger, observabilityServices),
		OTLP:               observability.NewOTLPHandler(observabilityServices.StreamProducer, observabilityServices.DeduplicationService, observabilityServices.OTLPConverterService, logger),
		OTLPMetrics:        observability.NewOTLPMetricsHandler(observabilityServices.StreamProducer, observabilityServices.OTLPMetricsConverterService, logger),
		OTLPLogs:           observability.NewOTLPLogsHandler(observabilityServices.StreamProducer, observabilityServices.OTLPLogsConverterService, observabilityServices.OTLPEventsConverterService, logger),
		Prompt:             prompt.NewHandler(cfg, logger, promptService, compilerService),
		Playground:         playground.NewHandler(cfg, logger, playgroundService, projectService),
		SDKPlayground:      playground.NewSDKPlaygroundHandler(logger, playgroundService),
		Credentials:        credentials.NewHandler(cfg, logger, credentialsSvc, modelCatalogSvc),
		Evaluation:         evaluationHandler.NewScoreConfigHandler(logger, scoreConfigService),
		SDKScore:           evaluationHandler.NewSDKScoreHandler(logger, observabilityServices.ScoreService, scoreConfigService),
		Dataset:            evaluationHandler.NewDatasetHandler(logger, datasetService, datasetItemService),
		DatasetItem:        evaluationHandler.NewDatasetItemHandler(logger, datasetItemService),
		DatasetVersion:     evaluationHandler.NewDatasetVersionHandler(logger, datasetVersionService),
		Experiment:         evaluationHandler.NewExperimentHandler(logger, experimentService, experimentItemService),
		ExperimentItem:     evaluationHandler.NewExperimentItemHandler(logger, experimentItemService),
		ExperimentWizard:   evaluationHandler.NewExperimentWizardHandler(logger, experimentWizardService),
		Evaluator:          evaluationHandler.NewEvaluatorHandler(evaluatorService),
		EvaluatorExecution: evaluationHandler.NewEvaluatorExecutionHandler(evaluatorExecutionService),
		SpanQuery:          observability.NewSpanQueryHandler(observabilityServices.SpanQueryService, logger),
		Dashboard:          dashboard.NewHandler(cfg, logger, dashboardService, widgetQueryService),
		DashboardTemplate:  dashboard.NewTemplateHandler(cfg, logger, templateService),
		Overview:           overview.NewHandler(cfg, logger, overviewService),
		// Usage-based billing handlers
		Usage:    billing.NewUsageHandler(cfg, logger, usageService),
		Budget:   billing.NewBudgetHandler(cfg, logger, budgetService),
		Contract: billing.NewContractHandler(cfg, logger, contractService, pricingService),
		// Annotation queue handlers
		AnnotationQueue:      annotationHandler.NewQueueHandler(logger, annotationQueueService),
		AnnotationItem:       annotationHandler.NewItemHandler(logger, annotationItemService, annotationAssignmentService),
		AnnotationAssignment: annotationHandler.NewAssignmentHandler(logger, annotationAssignmentService),
		// Comment handler
		Comment: commentHandler.NewHandler(commentService),
		// Website handler
		Website: websiteHandler.NewHandler(logger, websiteService),
	}
}
