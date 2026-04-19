package server

import (
	"log/slog"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	httprateredis "github.com/go-chi/httprate-redis"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"brokle/internal/config"
	authDomain "brokle/internal/core/domain/auth"
	orgDomain "brokle/internal/core/domain/organization"
	userDomain "brokle/internal/core/domain/user"
	websiteDomain "brokle/internal/core/domain/website"
	"brokle/internal/core/services/registration"
	"brokle/internal/transport/http/middleware"
)

// Deps groups every dependency the HTTP server needs to wire its
// route table. Populated by internal/app/providers.go and passed
// once to server.New.
//
// The set grows incrementally as handler domains are converted to
// Huma operations (Step 4 of the chi+Huma migration). Each new
// domain adds its service field here; per-domain RegisterRoutes
// functions (in internal/transport/http/handlers/<domain>/routes.go)
// take the explicit services they need rather than reaching into the
// whole Deps struct — Mat Ryer's "ask for what you need" rule
// applied at the registration boundary.
type Deps struct {
	Config *config.Config
	Logger *slog.Logger

	// Infrastructure handles for health pings + rate-limit backend.
	DB         *pgxpool.Pool
	Redis      *redis.Client
	ClickHouse driver.Conn

	// Auth/identity services consumed by RequireAuth, RequireSDKAuth,
	// RequireProjectAccess, and RequirePermission middleware.
	JWT       authDomain.JWTService
	Blacklist authDomain.BlacklistedTokenService
	OrgMember authDomain.OrganizationMemberService
	APIKey    authDomain.APIKeyService

	// Project service used by RequireProjectAccess.
	Project orgDomain.ProjectService

	// Domain services. Add as handler domains migrate to Huma. The
	// list grows with each vertical slice; domains not yet converted
	// to Huma operations are NOT listed here (CLAUDE.md scaffolded-
	// but-unreachable rule).

	// Auth domain handlers — login, signup, logout, refresh, password
	// mgmt, /me. Distinct from the middleware-facing JWT/Blacklist/
	// OrgMember services above: those carry invariant checks for
	// every protected route; Auth/User/Registration are the
	// "business logic" services the auth handler operations invoke.
	Auth         authDomain.AuthService
	User         userDomain.UserService
	Registration registration.RegistrationService

	// Website contact-form handler.
	Website websiteDomain.WebsiteService
}

// authMiddlewareDeps assembles the middleware.AuthDeps struct from
// the matching Deps fields. Centralised so route registration in
// addRoutes doesn't repeat the field mapping.
func (d Deps) authMiddlewareDeps() middleware.AuthDeps {
	return middleware.AuthDeps{
		JWT:       d.JWT,
		Blacklist: d.Blacklist,
		OrgMember: d.OrgMember,
		Project:   d.Project,
		Logger:    d.Logger,
	}
}

// sdkAuthMiddlewareDeps assembles the middleware.SDKAuthDeps struct
// for SDK route protection.
func (d Deps) sdkAuthMiddlewareDeps() middleware.SDKAuthDeps {
	return middleware.SDKAuthDeps{
		APIKey: d.APIKey,
		Logger: d.Logger,
	}
}

// rateLimitMiddlewareDeps builds the RateLimitDeps with a Redis
// backend pointed at the same instance the rest of the application
// uses. httprateredis.Config accepts a Client field of type
// redis.UniversalClient (which *redis.Client satisfies) — passing
// our shared client avoids a duplicate connection pool and keeps
// rate-limit traffic correlated with the rest of our Redis usage in
// dashboards.
func (d Deps) rateLimitMiddlewareDeps() middleware.RateLimitDeps {
	return middleware.RateLimitDeps{
		Redis: &httprateredis.Config{
			Client:    d.Redis,
			PrefixKey: "ratelimit:",
		},
		Auth:   &d.Config.Auth,
		Logger: d.Logger,
	}
}

// healthDeps assembles the readiness-check dependencies. Nil
// pointers skip the corresponding check (used by tests).
func (d Deps) healthDeps() healthDeps {
	return healthDeps{
		DB:         d.DB,
		Redis:      d.Redis,
		ClickHouse: d.ClickHouse,
		Logger:     d.Logger,
	}
}
