package http

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"brokle/internal/config"
	"brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/transport/http/handlers"
	"brokle/internal/transport/http/middleware"

	"github.com/redis/go-redis/v9"
)

type Server struct {
	config              *config.Config
	logger              *slog.Logger
	server              *http.Server
	listener            net.Listener
	handlers            *handlers.Handlers
	engine              *gin.Engine
	authMiddleware      *middleware.AuthMiddleware
	sdkAuthMiddleware   *middleware.SDKAuthMiddleware
	rateLimitMiddleware *middleware.RateLimitMiddleware
	csrfMiddleware      *middleware.CSRFMiddleware
	serveErr            chan error
}

func NewServer(
	cfg *config.Config,
	logger *slog.Logger,
	handlers *handlers.Handlers,
	jwtService auth.JWTService,
	blacklistedTokens auth.BlacklistedTokenService,
	orgMemberService auth.OrganizationMemberService,
	projectService orgDomain.ProjectService,
	apiKeyService auth.APIKeyService,
	redisClient *redis.Client,
) *Server {
	authMiddleware := middleware.NewAuthMiddleware(
		jwtService,
		blacklistedTokens,
		orgMemberService,
		projectService,
		logger,
	)

	sdkAuthMiddleware := middleware.NewSDKAuthMiddleware(
		apiKeyService,
		logger,
	)

	rateLimitMiddleware := middleware.NewRateLimitMiddleware(
		redisClient,
		&cfg.Auth,
		logger,
	)

	csrfMiddleware := middleware.NewCSRFMiddleware(logger)

	return &Server{
		config:              cfg,
		logger:              logger,
		handlers:            handlers,
		authMiddleware:      authMiddleware,
		sdkAuthMiddleware:   sdkAuthMiddleware,
		rateLimitMiddleware: rateLimitMiddleware,
		csrfMiddleware:      csrfMiddleware,
	}
}

func (s *Server) Start() error {
	if s.config.Server.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	s.engine = gin.New()

	corsConfig := cors.DefaultConfig()

	// Validate wildcard incompatibility with credentials
	if len(s.config.Server.CORSAllowedOrigins) == 1 && s.config.Server.CORSAllowedOrigins[0] == "*" {
		// CRITICAL: Wildcard incompatible with AllowCredentials (cookies won't work)
		s.logger.Error("CORS misconfiguration: cannot use wildcard (*) origins with AllowCredentials (httpOnly cookies require specific origins). " +
			"Set specific origins in CORS_ALLOWED_ORIGINS environment variable.")
		os.Exit(1)
	}

	// Configure specific origins (only reached if not wildcard)
	corsConfig.AllowOrigins = s.config.Server.CORSAllowedOrigins

	// Validate at least one origin is configured
	if len(s.config.Server.CORSAllowedOrigins) == 0 {
		s.logger.Error("CORS misconfiguration: AllowCredentials requires specific AllowedOrigins. " +
			"Set CORS_ALLOWED_ORIGINS environment variable.")
		os.Exit(1)
	}

	corsConfig.AllowMethods = s.config.Server.CORSAllowedMethods

	// Ensure X-CSRF-Token is always allowed (required for CSRF protection)
	allowedHeaders := s.config.Server.CORSAllowedHeaders
	corsConfig.AllowHeaders = append(allowedHeaders, "X-CSRF-Token")

	corsConfig.AllowCredentials = true
	corsConfig.MaxAge = 5 * time.Minute
	s.engine.Use(cors.New(corsConfig))

	s.setupRoutes()

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Server.Port),
		Handler:      s.engine,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.Server.IdleTimeout) * time.Second,
	}

	s.serveErr = make(chan error, 1)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.config.Server.Port))
	if err != nil {
		return fmt.Errorf("failed to bind HTTP server: %w", err)
	}
	s.listener = lis

	s.logger.Info("HTTP server listening", "port", s.config.Server.Port)

	go func() {
		if err := s.server.Serve(lis); err != nil && err != http.ErrServerClosed {
			s.serveErr <- err
		}
	}()

	return nil
}

func (s *Server) ServeErr() <-chan error {
	return s.serveErr
}

func (s *Server) setupRoutes() {
	s.engine.Use(middleware.RequestID())
	s.engine.Use(middleware.Logger(s.logger))
	s.engine.Use(middleware.Recovery(s.logger))
	s.engine.Use(middleware.Metrics())

	s.engine.GET("/health", s.handlers.Health.Check)
	s.engine.HEAD("/health", s.handlers.Health.Check)
	s.engine.GET("/health/ready", s.handlers.Health.Ready)
	s.engine.HEAD("/health/ready", s.handlers.Health.Ready)
	s.engine.GET("/health/live", s.handlers.Health.Live)
	s.engine.HEAD("/health/live", s.handlers.Health.Live)

	s.engine.GET("/metrics", s.handlers.Metrics.Handler)

	if s.config.Server.Environment == "development" {
		s.engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))
	}

	sdk := s.engine.Group("/v1")
	sdkAuth := sdk.Group("/auth")
	{
		// validate-key runs before RequireSDKAuth / RateLimitByAPIKey are
		// applied to the /v1 group below. Apply IP + hashed-key-prefix
		// rate limiting inline so this endpoint is not a brute-force oracle.
		sdkAuth.POST("/validate-key",
			s.rateLimitMiddleware.RateLimitByIP(),
			s.rateLimitMiddleware.RateLimitByKeyPrefix(),
			s.handlers.Auth.ValidateAPIKeyHandler,
		)
	}
	sdk.Use(s.sdkAuthMiddleware.RequireSDKAuth())
	sdk.Use(s.rateLimitMiddleware.RateLimitByAPIKey())
	s.setupSDKRoutes(sdk)

	dashboard := s.engine.Group("/api/v1")
	s.setupDashboardRoutes(dashboard)

	s.engine.GET("/ws", s.handlers.WebSocket.Handle)
}

func (s *Server) setupDashboardRoutes(router *gin.RouterGroup) {
	router.Use(s.rateLimitMiddleware.RateLimitByIP())

	auth := router.Group("/auth")
	{
		auth.POST("/login", s.handlers.Auth.Login)
		auth.POST("/signup", s.handlers.Auth.Signup)
		auth.POST("/complete-oauth-signup", s.handlers.Auth.CompleteOAuthSignup)
		auth.POST("/exchange-session/:session_id", s.handlers.Auth.ExchangeLoginSession)
		auth.POST("/refresh", s.handlers.Auth.RefreshToken)
		auth.POST("/forgot-password", s.handlers.Auth.ForgotPassword)
		auth.POST("/reset-password", s.handlers.Auth.ResetPassword)
		auth.GET("/google", s.handlers.Auth.InitiateGoogleOAuth)
		auth.GET("/google/callback", s.handlers.Auth.GoogleCallback)
		auth.GET("/github", s.handlers.Auth.InitiateGitHubOAuth)
		auth.GET("/github/callback", s.handlers.Auth.GitHubCallback)
	}

	router.GET("/invitations/validate/:token", s.handlers.Organization.ValidateInvitationToken)
	router.POST("/invitations/decline", s.handlers.Organization.DeclineInvitation)

	// Public website form routes (rate-limited by IP via parent group)
	websiteRoutes := router.Group("/website")
	{
		websiteRoutes.POST("/contact", s.handlers.Website.SubmitContactForm)
	}

	// Protected routes: JWT → CSRF → rate limit
	protected := router.Group("")
	protected.Use(s.authMiddleware.RequireAuth())
	protected.Use(s.csrfMiddleware.ValidateCSRF())
	protected.Use(s.rateLimitMiddleware.RateLimitByUser())

	// User-scoped invitation routes
	invitations := protected.Group("/invitations")
	{
		invitations.GET("", s.handlers.Organization.GetUserInvitations)       // List invitations for current user
		invitations.POST("/accept", s.handlers.Organization.AcceptInvitation) // Accept an invitation
	}

	users := protected.Group("/users")
	{
		users.GET("/me", s.handlers.User.GetProfile)
		users.PATCH("/me", s.handlers.User.UpdateProfile)
		users.PUT("/me/default-organization", s.handlers.User.SetDefaultOrganization)
	}

	authSessions := protected.Group("/auth")
	{
		authSessions.GET("/me", s.handlers.Auth.GetCurrentUser)
		authSessions.POST("/logout", s.handlers.Auth.Logout)
		authSessions.GET("/sessions", s.handlers.Auth.ListSessions)
		authSessions.GET("/sessions/:session_id", s.handlers.Auth.GetSession)
		authSessions.POST("/sessions/:session_id/revoke", s.handlers.Auth.RevokeSession)
		authSessions.POST("/sessions/revoke-all", s.handlers.Auth.RevokeAllSessions)
	}

	orgs := protected.Group("/organizations")
	{
		orgs.GET("", s.handlers.Organization.List)
		orgs.POST("", s.handlers.Organization.Create)
		orgs.GET("/:orgId", s.handlers.Organization.Get)
		orgs.PATCH("/:orgId", s.authMiddleware.RequirePermission("organizations:write"), s.handlers.Organization.Update)
		orgs.DELETE("/:orgId", s.authMiddleware.RequirePermission("organizations:delete"), s.handlers.Organization.Delete)
		orgs.GET("/:orgId/members", s.authMiddleware.RequirePermission("members:read"), s.handlers.Organization.ListMembers)
		orgs.POST("/:orgId/members", s.authMiddleware.RequirePermission("members:invite"), s.handlers.Organization.InviteMember)
		orgs.DELETE("/:orgId/members/:userId", s.authMiddleware.RequirePermission("members:remove"), s.handlers.Organization.RemoveMember)

		// Invitation management routes
		orgs.GET("/:orgId/invitations", s.authMiddleware.RequirePermission("members:read"), s.handlers.Organization.GetPendingInvitations)
		orgs.POST("/:orgId/invitations", s.authMiddleware.RequirePermission("members:invite"), s.handlers.Organization.CreateInvitation)
		orgs.POST("/:orgId/invitations/:invitationId/resend", s.authMiddleware.RequirePermission("members:invite"), s.handlers.Organization.ResendInvitation)
		orgs.DELETE("/:orgId/invitations/:invitationId", s.authMiddleware.RequirePermission("members:invite"), s.handlers.Organization.RevokeInvitation)

		orgs.GET("/:orgId/settings", s.authMiddleware.RequirePermission("settings:read"), s.handlers.Organization.GetSettings)
		orgs.POST("/:orgId/settings", s.authMiddleware.RequirePermission("settings:write"), s.handlers.Organization.CreateSetting)
		orgs.GET("/:orgId/settings/:key", s.authMiddleware.RequirePermission("settings:read"), s.handlers.Organization.GetSetting)
		orgs.PUT("/:orgId/settings/:key", s.authMiddleware.RequirePermission("settings:write"), s.handlers.Organization.UpdateSetting)
		orgs.DELETE("/:orgId/settings/:key", s.authMiddleware.RequirePermission("settings:write"), s.handlers.Organization.DeleteSetting)
		orgs.POST("/:orgId/settings/bulk", s.authMiddleware.RequirePermission("settings:write"), s.handlers.Organization.BulkCreateSettings)
		orgs.GET("/:orgId/settings/export", s.authMiddleware.RequirePermission("settings:export"), s.handlers.Organization.ExportSettings)
		orgs.POST("/:orgId/settings/import", s.authMiddleware.RequireAllPermissions([]string{"settings:write", "settings:import"}), s.handlers.Organization.ImportSettings)
		orgs.POST("/:orgId/settings/reset", s.authMiddleware.RequireAnyPermission([]string{"settings:admin", "organizations:admin"}), s.handlers.Organization.ResetToDefaults)

		orgs.GET("/:orgId/roles", s.authMiddleware.RequirePermission("roles:read"), s.handlers.RBAC.GetCustomRoles)
		orgs.POST("/:orgId/roles", s.authMiddleware.RequirePermission("roles:write"), s.handlers.RBAC.CreateCustomRole)
		orgs.GET("/:orgId/roles/:roleId", s.authMiddleware.RequirePermission("roles:read"), s.handlers.RBAC.GetCustomRole)
		orgs.PUT("/:orgId/roles/:roleId", s.authMiddleware.RequirePermission("roles:write"), s.handlers.RBAC.UpdateCustomRole)
		orgs.DELETE("/:orgId/roles/:roleId", s.authMiddleware.RequirePermission("roles:delete"), s.handlers.RBAC.DeleteCustomRole)

		// AI Provider Credentials routes (organization-level)
		orgCredentials := orgs.Group("/:orgId/credentials")
		{
			aiCreds := orgCredentials.Group("/ai")
			{
				aiCreds.POST("", s.authMiddleware.RequirePermission("credentials:write"), s.handlers.Credentials.Create)
				aiCreds.POST("/test", s.authMiddleware.RequirePermission("credentials:write"), s.handlers.Credentials.TestConnection)
				aiCreds.GET("", s.authMiddleware.RequirePermission("credentials:read"), s.handlers.Credentials.List)
				aiCreds.GET("/models", s.authMiddleware.RequirePermission("credentials:read"), s.handlers.Credentials.GetAvailableModels)
				aiCreds.GET("/:credentialId", s.authMiddleware.RequirePermission("credentials:read"), s.handlers.Credentials.Get)
				aiCreds.PATCH("/:credentialId", s.authMiddleware.RequirePermission("credentials:write"), s.handlers.Credentials.Update)
				aiCreds.DELETE("/:credentialId", s.authMiddleware.RequirePermission("credentials:write"), s.handlers.Credentials.Delete)
			}
		}

		// Usage-based billing: Usage routes (Spans + GB + Scores)
		orgUsage := orgs.Group("/:orgId/usage")
		{
			orgUsage.GET("/overview", s.handlers.Usage.GetUsageOverview)
			orgUsage.GET("/timeseries", s.handlers.Usage.GetUsageTimeSeries)
			orgUsage.GET("/by-project", s.handlers.Usage.GetUsageByProject)
			orgUsage.GET("/export", s.handlers.Usage.ExportUsage)
		}

		// Usage-based billing: Budget management routes
		orgBudgets := orgs.Group("/:orgId/budgets")
		{
			orgBudgets.GET("", s.handlers.Budget.ListBudgets)
			orgBudgets.POST("", s.handlers.Budget.CreateBudget)
			orgBudgets.GET("/:budgetId", s.handlers.Budget.GetBudget)
			orgBudgets.PUT("/:budgetId", s.handlers.Budget.UpdateBudget)
			orgBudgets.DELETE("/:budgetId", s.handlers.Budget.DeleteBudget)
			orgBudgets.GET("/alerts", s.handlers.Budget.GetAlerts)
			orgBudgets.POST("/alerts/:alertId/acknowledge", s.handlers.Budget.AcknowledgeAlert)
		}

		// Enterprise custom pricing: Contract routes
		orgContracts := orgs.Group("/:orgId/contracts")
		{
			orgContracts.GET("", s.handlers.Contract.GetContractsByOrg)
		}

		// Enterprise custom pricing: Effective pricing
		orgs.GET("/:orgId/effective-pricing", s.handlers.Contract.GetEffectivePricing)
	}

	projects := protected.Group("/projects")
	{
		projects.GET("", s.handlers.Project.List)
		projects.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Project.Create)
		projects.GET("/:projectId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Project.Get)
		projects.PUT("/:projectId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Project.Update)
		projects.POST("/:projectId/archive", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Project.Archive)
		projects.POST("/:projectId/unarchive", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Project.Unarchive)
		projects.DELETE("/:projectId", s.authMiddleware.RequirePermission("projects:delete"), s.handlers.Project.Delete)

		// Project overview
		projects.GET("/:projectId/overview", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Overview.GetOverview)

		projects.GET("/:projectId/api-keys", s.authMiddleware.RequirePermission("api-keys:read"), s.handlers.APIKey.List)
		projects.POST("/:projectId/api-keys", s.authMiddleware.RequirePermission("api-keys:create"), s.handlers.APIKey.Create)
		projects.DELETE("/:projectId/api-keys/:keyId", s.authMiddleware.RequirePermission("api-keys:delete"), s.handlers.APIKey.Delete)

		prompts := projects.Group("/:projectId/prompts")
		{
			prompts.GET("/settings/protected-labels", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.GetProtectedLabels)
			prompts.PUT("/settings/protected-labels", s.authMiddleware.RequirePermission("prompts:update"), s.handlers.Prompt.SetProtectedLabels)

			prompts.POST("/validate-template", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.ValidateTemplate)
			prompts.POST("/preview-template", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.PreviewTemplate)
			prompts.POST("/detect-dialect", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.DetectDialect)

			prompts.GET("", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.ListPrompts)
			prompts.POST("", s.authMiddleware.RequirePermission("prompts:create"), s.handlers.Prompt.CreatePrompt)
			prompts.GET("/:promptId", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.GetPrompt)
			prompts.PUT("/:promptId", s.authMiddleware.RequirePermission("prompts:update"), s.handlers.Prompt.UpdatePrompt)
			prompts.DELETE("/:promptId", s.authMiddleware.RequirePermission("prompts:delete"), s.handlers.Prompt.DeletePrompt)
			prompts.GET("/:promptId/versions", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.ListVersions)
			prompts.POST("/:promptId/versions", s.authMiddleware.RequirePermission("prompts:create"), s.handlers.Prompt.CreateVersion)
			prompts.GET("/:promptId/versions/:versionId", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.GetVersion)
			prompts.PATCH("/:promptId/versions/:versionId/labels", s.authMiddleware.RequirePermission("prompts:update"), s.handlers.Prompt.SetLabels)
			prompts.GET("/:promptId/diff", s.authMiddleware.RequirePermission("prompts:read"), s.handlers.Prompt.GetVersionDiff)
		}

		playground := protected.Group("/playground")
		{
			playground.POST("/execute", s.handlers.Playground.Execute)
			playground.POST("/stream", s.handlers.Playground.Stream)
		}

		projectPlayground := projects.Group("/:projectId/playground")
		{
			sessions := projectPlayground.Group("/sessions")
			{
				sessions.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Playground.ListSessions)
				sessions.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Playground.CreateSession)
				sessions.GET("/:sessionId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Playground.GetSession)
				sessions.PUT("/:sessionId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Playground.UpdateSession)
				sessions.POST("/:sessionId/update", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Playground.UpdateSession) // sendBeacon fallback
				sessions.DELETE("/:sessionId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Playground.DeleteSession)
			}
		}

		scoreConfigs := projects.Group("/:projectId/score-configs")
		{
			scoreConfigs.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluation.List)
			scoreConfigs.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluation.Create)
			scoreConfigs.GET("/:configId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluation.Get)
			scoreConfigs.PUT("/:configId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluation.Update)
			scoreConfigs.DELETE("/:configId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluation.Delete)
		}

		datasets := projects.Group("/:projectId/datasets")
		{
			datasets.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dataset.List)
			datasets.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dataset.Create)
			datasets.GET("/:datasetId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dataset.Get)
			datasets.PUT("/:datasetId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dataset.Update)
			datasets.DELETE("/:datasetId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dataset.Delete)
			datasets.GET("/:datasetId/items", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetItem.List)
			datasets.POST("/:datasetId/items", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.Create)
			datasets.DELETE("/:datasetId/items/:itemId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.Delete)
			datasets.POST("/:datasetId/items/import-json", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.ImportFromJSON)
			datasets.POST("/:datasetId/items/import-csv", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.ImportFromCSV)
			datasets.POST("/:datasetId/items/from-traces", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.CreateFromTraces)
			datasets.POST("/:datasetId/items/from-spans", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetItem.CreateFromSpans)
			datasets.GET("/:datasetId/items/export", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetItem.Export)
			// Dataset versioning routes
			datasets.POST("/:datasetId/versions", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetVersion.CreateVersion)
			datasets.GET("/:datasetId/versions", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetVersion.ListVersions)
			datasets.GET("/:datasetId/versions/:versionId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetVersion.GetVersion)
			datasets.GET("/:datasetId/versions/:versionId/items", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetVersion.GetVersionItems)
			datasets.POST("/:datasetId/pin", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DatasetVersion.PinVersion)
			datasets.GET("/:datasetId/info", s.authMiddleware.RequirePermission("projects:read"), s.handlers.DatasetVersion.GetDatasetWithVersionInfo)
			// Dataset fields (for experiment wizard variable mapping)
			datasets.GET("/:datasetId/fields", s.authMiddleware.RequirePermission("projects:read"), s.handlers.ExperimentWizard.GetDatasetFields)
		}

		experiments := projects.Group("/:projectId/experiments")
		{
			experiments.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Experiment.List)
			experiments.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Experiment.Create)
			experiments.POST("/compare", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Experiment.CompareExperiments)
			experiments.GET("/:experimentId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Experiment.Get)
			experiments.PUT("/:experimentId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Experiment.Update)
			experiments.DELETE("/:experimentId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Experiment.Delete)
			experiments.POST("/:experimentId/rerun", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Experiment.Rerun)
			experiments.GET("/:experimentId/items", s.authMiddleware.RequirePermission("projects:read"), s.handlers.ExperimentItem.List)
			experiments.GET("/:experimentId/config", s.authMiddleware.RequirePermission("projects:read"), s.handlers.ExperimentWizard.GetExperimentConfig)
			experiments.GET("/:experimentId/progress", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Experiment.GetProgress)
			experiments.GET("/:experimentId/metrics", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Experiment.GetMetrics)

			// Experiment wizard routes
			experiments.POST("/wizard", s.authMiddleware.RequirePermission("projects:write"), s.handlers.ExperimentWizard.CreateFromWizard)
			experiments.POST("/wizard/validate", s.authMiddleware.RequirePermission("projects:write"), s.handlers.ExperimentWizard.ValidateStep)
			experiments.POST("/wizard/estimate", s.authMiddleware.RequirePermission("projects:read"), s.handlers.ExperimentWizard.EstimateCost)
		}

		evaluators := projects.Group("/:projectId/evaluators")
		{
			evaluators.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluator.List)
			evaluators.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Create)
			evaluators.GET("/:evaluatorId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluator.Get)
			evaluators.PUT("/:evaluatorId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Update)
			evaluators.DELETE("/:evaluatorId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Delete)
			evaluators.POST("/:evaluatorId/activate", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Activate)
			evaluators.POST("/:evaluatorId/deactivate", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Deactivate)
			evaluators.POST("/:evaluatorId/trigger", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Evaluator.Trigger)
			evaluators.POST("/:evaluatorId/test", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluator.Test)
			evaluators.GET("/:evaluatorId/analytics", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Evaluator.GetAnalytics)

			// Evaluator execution history
			evaluators.GET("/:evaluatorId/executions", s.authMiddleware.RequirePermission("projects:read"), s.handlers.EvaluatorExecution.List)
			evaluators.GET("/:evaluatorId/executions/latest", s.authMiddleware.RequirePermission("projects:read"), s.handlers.EvaluatorExecution.GetLatest)
			evaluators.GET("/:evaluatorId/executions/:executionId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.EvaluatorExecution.Get)
			evaluators.GET("/:evaluatorId/executions/:executionId/detail", s.authMiddleware.RequirePermission("projects:read"), s.handlers.EvaluatorExecution.GetDetail)
		}

		// Annotation Queues (HITL evaluation)
		annotationQueues := projects.Group("/:projectId/annotation-queues")
		{
			annotationQueues.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.AnnotationQueue.List)
			annotationQueues.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationQueue.Create)
			annotationQueues.GET("/:queueId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.AnnotationQueue.Get)
			annotationQueues.GET("/:queueId/stats", s.authMiddleware.RequirePermission("projects:read"), s.handlers.AnnotationQueue.GetWithStats)
			annotationQueues.PUT("/:queueId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationQueue.Update)
			annotationQueues.DELETE("/:queueId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationQueue.Delete)

			// Queue items
			annotationQueues.GET("/:queueId/items", s.authMiddleware.RequirePermission("projects:read"), s.handlers.AnnotationItem.ListItems)
			annotationQueues.POST("/:queueId/items", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationItem.AddItems)
			annotationQueues.POST("/:queueId/items/claim", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationItem.ClaimNext)
			annotationQueues.POST("/:queueId/items/:itemId/complete", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationItem.Complete)
			annotationQueues.POST("/:queueId/items/:itemId/skip", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationItem.Skip)
			annotationQueues.POST("/:queueId/items/:itemId/release", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationItem.ReleaseLock)

			// Queue assignments
			annotationQueues.GET("/:queueId/assignments", s.authMiddleware.RequirePermission("projects:read"), s.handlers.AnnotationAssignment.List)
			annotationQueues.POST("/:queueId/assignments", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationAssignment.Assign)
			annotationQueues.DELETE("/:queueId/assignments/:userId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.AnnotationAssignment.Unassign)
		}

		projectScores := projects.Group("/:projectId/scores")
		{
			projectScores.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.ListProjectScores)
			projectScores.GET("/names", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.GetScoreNames)
			projectScores.GET("/analytics", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.GetScoreAnalytics)
		}

		filterPresets := projects.Group("/:projectId/filter-presets")
		{
			filterPresets.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.ListFilterPresets)
			filterPresets.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Observability.CreateFilterPreset)
			filterPresets.GET("/:id", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.GetFilterPreset)
			filterPresets.PATCH("/:id", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Observability.UpdateFilterPreset)
			filterPresets.DELETE("/:id", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Observability.DeleteFilterPreset)
		}

		// Observability sessions (aggregated from traces by session_id)
		projects.GET("/:projectId/sessions", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Observability.ListSessions)

		dashboards := projects.Group("/:projectId/dashboards")
		{
			// List and create
			dashboards.GET("", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.ListDashboards)
			dashboards.POST("", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.CreateDashboard)

			dashboards.POST("/from-template", s.authMiddleware.RequirePermission("projects:write"), s.handlers.DashboardTemplate.CreateFromTemplate)
			dashboards.POST("/import", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.ImportDashboard)
			dashboards.GET("/variable-options", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.GetVariableOptions)

			// Dynamic routes with :dashboardId parameter
			dashboards.GET("/:dashboardId", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.GetDashboard)
			dashboards.PUT("/:dashboardId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.UpdateDashboard)
			dashboards.DELETE("/:dashboardId", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.DeleteDashboard)
			dashboards.POST("/:dashboardId/duplicate", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.DuplicateDashboard)
			dashboards.POST("/:dashboardId/lock", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.LockDashboard)
			dashboards.POST("/:dashboardId/unlock", s.authMiddleware.RequirePermission("projects:write"), s.handlers.Dashboard.UnlockDashboard)
			dashboards.GET("/:dashboardId/export", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.ExportDashboard)
			dashboards.POST("/:dashboardId/execute", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.ExecuteDashboardQueries)
			dashboards.POST("/:dashboardId/widgets/:widgetId/execute", s.authMiddleware.RequirePermission("projects:read"), s.handlers.Dashboard.ExecuteWidgetQuery)
		}
	}

	analytics := protected.Group("/analytics")
	{
		analytics.GET("/overview", s.handlers.Analytics.Overview)
		analytics.GET("/requests", s.handlers.Analytics.Requests)
		analytics.GET("/costs", s.handlers.Analytics.Costs)
		analytics.GET("/providers", s.handlers.Analytics.Providers)
		analytics.GET("/models", s.handlers.Analytics.Models)
	}

	// Dashboard query builder view definitions
	protected.GET("/dashboards/view-definitions", s.handlers.Dashboard.GetViewDefinitions)

	// Dashboard templates (accessible from all projects)
	dashboardTemplates := protected.Group("/dashboard-templates")
	{
		dashboardTemplates.GET("", s.handlers.DashboardTemplate.ListTemplates)
		dashboardTemplates.GET("/:templateId", s.handlers.DashboardTemplate.GetTemplate)
	}

	traces := protected.Group("/traces")
	{
		// Endpoints that accept a user-controlled project_id (query or path) are
		// wrapped with RequireProjectAccess so every request is validated to
		// originate from an org member. Detail endpoints that take only :id
		// (trace/span/score IDs) rely on service-layer tenancy checks after the
		// fetch — tracked as a follow-up.
		requireProject := s.authMiddleware.RequireProjectAccess()

		traces.GET("", requireProject, s.handlers.Observability.ListTraces)
		traces.GET("/filter-options", requireProject, s.handlers.Observability.GetTraceFilterOptions) // Must be before /:id
		traces.GET("/attributes", requireProject, s.handlers.Observability.DiscoverAttributes)        // Must be before /:id
		traces.GET("/:id", s.handlers.Observability.GetTrace)
		traces.GET("/:id/spans", s.handlers.Observability.GetTraceWithSpans)
		traces.GET("/:id/scores", s.handlers.Observability.GetTraceWithScores)
		traces.POST("/:id/scores", requireProject, s.handlers.Observability.CreateTraceScore)
		traces.DELETE("/:id/scores/:score_id", requireProject, s.handlers.Observability.DeleteTraceScore)
		traces.DELETE("/:id", s.handlers.Observability.DeleteTrace)
		traces.PUT("/:id/tags", requireProject, s.handlers.Observability.UpdateTraceTags)
		traces.PUT("/:id/bookmark", requireProject, s.handlers.Observability.UpdateTraceBookmark)

		// Trace comments — every comment endpoint accepts project_id from the
		// query string, so the whole group is wrapped.
		traceComments := traces.Group("/:id/comments", requireProject)
		{
			traceComments.POST("", s.handlers.Comment.CreateComment)
			traceComments.GET("", s.handlers.Comment.ListComments)
			traceComments.GET("/count", s.handlers.Comment.GetCommentCount)
			traceComments.PUT("/:comment_id", s.handlers.Comment.UpdateComment)
			traceComments.DELETE("/:comment_id", s.handlers.Comment.DeleteComment)
			traceComments.POST("/:comment_id/reactions", s.handlers.Comment.ToggleReaction)
			traceComments.POST("/:comment_id/replies", s.handlers.Comment.CreateReply)
		}
	}

	spans := protected.Group("/spans")
	{
		spans.GET("", s.authMiddleware.RequireProjectAccess(), s.handlers.Observability.ListSpans)
		spans.GET("/:id", s.handlers.Observability.GetSpan)
		spans.DELETE("/:id", s.handlers.Observability.DeleteSpan)
	}

	scores := protected.Group("/scores")
	{
		scores.GET("", s.authMiddleware.RequireProjectAccess(), s.handlers.Observability.ListScores)
		scores.GET("/:id", s.handlers.Observability.GetScore)
		scores.PUT("/:id", s.handlers.Observability.UpdateScore)
	}

	logs := protected.Group("/logs")
	{
		logs.GET("/requests", s.handlers.Logs.ListRequests)
		logs.GET("/requests/:requestId", s.handlers.Logs.GetRequest)
		logs.GET("/export", s.handlers.Logs.Export)
	}

	billing := protected.Group("/billing")
	{
		billing.GET("/:orgId/usage", s.handlers.Billing.GetUsage)
		billing.GET("/:orgId/invoices", s.handlers.Billing.ListInvoices)
		billing.GET("/:orgId/subscription", s.handlers.Billing.GetSubscription)
		billing.POST("/:orgId/subscription", s.handlers.Billing.UpdateSubscription)

		// Enterprise custom pricing: Contract management
		contracts := billing.Group("/contracts")
		{
			contracts.POST("", s.handlers.Contract.CreateContract)
			contracts.GET("/:contractId", s.handlers.Contract.GetContract)
			contracts.PUT("/:contractId", s.handlers.Contract.UpdateContract)
			contracts.DELETE("/:contractId", s.handlers.Contract.CancelContract)
			contracts.PUT("/:contractId/activate", s.handlers.Contract.ActivateContract)
			contracts.PUT("/:contractId/tiers", s.handlers.Contract.UpdateVolumeTiers)
			contracts.GET("/:contractId/history", s.handlers.Contract.GetContractHistory)
		}
	}

	rbac := protected.Group("/rbac")
	{
		rbac.GET("/roles", s.handlers.RBAC.ListRoles)
		rbac.POST("/roles", s.handlers.RBAC.CreateRole)
		rbac.GET("/roles/:roleId", s.handlers.RBAC.GetRole)
		rbac.PUT("/roles/:roleId", s.handlers.RBAC.UpdateRole)
		rbac.DELETE("/roles/:roleId", s.handlers.RBAC.DeleteRole)
		rbac.GET("/roles/statistics", s.handlers.RBAC.GetRoleStatistics)
		rbac.GET("/permissions", s.handlers.RBAC.ListPermissions)
		rbac.POST("/permissions", s.handlers.RBAC.CreatePermission)
		rbac.GET("/permissions/:permissionId", s.handlers.RBAC.GetPermission)
		rbac.GET("/permissions/resources", s.handlers.RBAC.GetAvailableResources)
		rbac.GET("/permissions/resources/:resource/actions", s.handlers.RBAC.GetActionsForResource)
		rbac.GET("/users/:userId/organizations/:orgId/role", s.handlers.RBAC.GetUserRole)
		rbac.POST("/users/:userId/organizations/:orgId/role", s.handlers.RBAC.AssignOrganizationRole)
		rbac.GET("/users/:userId/organizations/:orgId/permissions", s.handlers.RBAC.GetUserPermissions)          // legacy
		rbac.POST("/users/:userId/organizations/:orgId/permissions/check", s.handlers.RBAC.CheckUserPermissions) // legacy
		rbac.POST("/users/:userId/scopes/check", s.handlers.RBAC.CheckUserScopes)
		rbac.GET("/users/:userId/scopes", s.handlers.RBAC.GetUserScopes)
		rbac.GET("/scopes", s.handlers.RBAC.GetAvailableScopes)
		rbac.GET("/scopes/categories", s.handlers.RBAC.GetScopeCategories)
	}

	// User's annotation queue assignments (cross-project view)
	protected.GET("/annotation-queues/my-assignments", s.handlers.AnnotationAssignment.GetMyAssignments)
}

func (s *Server) setupSDKRoutes(router *gin.RouterGroup) {
	// OTLP ingestion - supports Protobuf + JSON, gzip compression
	router.POST("/traces", s.handlers.OTLP.HandleTraces)
	router.POST("/metrics", s.handlers.OTLPMetrics.HandleMetrics)
	router.POST("/logs", s.handlers.OTLPLogs.HandleLogs)

	prompts := router.Group("/prompts")
	{
		prompts.GET("", s.handlers.Prompt.ListPromptsSDK)
		prompts.POST("", s.handlers.Prompt.UpsertPrompt)
		prompts.GET("/:name", s.handlers.Prompt.GetPromptByName)
	}

	scores := router.Group("/scores")
	{
		scores.POST("", s.handlers.SDKScore.Create)
		scores.POST("/batch", s.handlers.SDKScore.CreateBatch)
	}

	sdkDatasets := router.Group("/datasets")
	{
		sdkDatasets.GET("", s.handlers.Dataset.List)
		sdkDatasets.POST("", s.handlers.Dataset.Create)
		sdkDatasets.GET("/:datasetId", s.handlers.Dataset.Get)
		sdkDatasets.PATCH("/:datasetId", s.handlers.Dataset.Update)
		sdkDatasets.DELETE("/:datasetId", s.handlers.Dataset.Delete)
		sdkDatasets.POST("/:datasetId/items", s.handlers.Dataset.CreateItems)
		sdkDatasets.GET("/:datasetId/items", s.handlers.Dataset.ListItems)
		sdkDatasets.GET("/:datasetId/items/export", s.handlers.Dataset.ExportItems)
		// Dataset import SDK routes
		sdkDatasets.POST("/:datasetId/items/import-json", s.handlers.Dataset.ImportFromJSON)
		sdkDatasets.POST("/:datasetId/items/import-csv", s.handlers.Dataset.ImportFromCSV)
		sdkDatasets.POST("/:datasetId/items/from-traces", s.handlers.Dataset.CreateFromTraces)
		sdkDatasets.POST("/:datasetId/items/from-spans", s.handlers.Dataset.CreateFromSpans)
		// Dataset versioning SDK routes
		sdkDatasets.POST("/:datasetId/versions", s.handlers.DatasetVersion.CreateVersion)
		sdkDatasets.GET("/:datasetId/versions", s.handlers.DatasetVersion.ListVersions)
		sdkDatasets.GET("/:datasetId/versions/:versionId", s.handlers.DatasetVersion.GetVersion)
		sdkDatasets.GET("/:datasetId/versions/:versionId/items", s.handlers.DatasetVersion.GetVersionItems)
		sdkDatasets.POST("/:datasetId/pin", s.handlers.DatasetVersion.PinVersion)
		sdkDatasets.GET("/:datasetId/info", s.handlers.DatasetVersion.GetDatasetWithVersionInfo)
	}

	sdkExperiments := router.Group("/experiments")
	{
		sdkExperiments.GET("", s.handlers.Experiment.List)
		sdkExperiments.POST("", s.handlers.Experiment.Create)
		sdkExperiments.POST("/compare", s.handlers.Experiment.CompareExperiments)
		sdkExperiments.GET("/:experimentId", s.handlers.Experiment.Get)
		sdkExperiments.PATCH("/:experimentId", s.handlers.Experiment.Update)
		sdkExperiments.POST("/:experimentId/items", s.handlers.Experiment.CreateItems)
		sdkExperiments.POST("/:experimentId/rerun", s.handlers.Experiment.Rerun)
	}

	spans := router.Group("/spans")
	{
		spans.POST("/query", s.handlers.SpanQuery.HandleQuery)
		spans.POST("/query/validate", s.handlers.SpanQuery.HandleValidate)
	}

	// Playground execution for SDK (LLMScorer)
	playground := router.Group("/playground")
	{
		playground.POST("/execute", s.handlers.SDKPlayground.Execute)
	}

	// Annotation queues SDK routes (programmatic item management)
	sdkAnnotationQueues := router.Group("/annotation-queues")
	{
		sdkAnnotationQueues.GET("/:queueId/items", s.handlers.AnnotationItem.ListItemsSDK)
		sdkAnnotationQueues.POST("/:queueId/items", s.handlers.AnnotationItem.AddItemsSDK)
		sdkAnnotationQueues.POST("/:queueId/items/batch", s.handlers.AnnotationItem.AddItemsSDK) // Alias for batch
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
