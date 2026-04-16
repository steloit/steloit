# Brokle Architecture Overview

## Scalable Monolith Design

Brokle uses a **scalable monolith** architecture with separate binaries for independent scaling:

### Binary Separation

**Server Binary** (`cmd/server/main.go`):
- Set `APP_MODE=server` in environment
- Handles HTTP API endpoints and WebSocket connections
- Runs database migrations (server owns migrations)
- Requires `JWT_SECRET` for authentication
- Port: 8080
- Typical scaling: 3-5 instances

**Worker Binary** (`cmd/worker/main.go`):
- Set `APP_MODE=worker` in environment
- Processes telemetry streams from Redis
- Handles gateway analytics aggregation
- Executes batch jobs
- No `JWT_SECRET` needed (doesn't handle auth)
- Typical scaling: 10-50+ instances at high load

**Shared Infrastructure**:
- Same database connections (PostgreSQL, ClickHouse, Redis)
- Same service layer (reused via DI container)
- Different resource profiles (APIs: low latency, Workers: high throughput)

## Domain-Driven Design

### Domains

Primary domains in `internal/core/domain/`:

| Domain | Purpose |
|--------|---------|
| auth | Authentication, sessions, API keys |
| billing | Usage tracking, subscriptions |
| common | Shared transaction patterns, utilities |
| gateway | AI provider routing |
| observability | Traces, spans, quality scores |
| organization | Multi-tenant org management |
| user | User management and profiles |

**Reference**: Check `internal/core/domain/` directory for complete list and implementation status

### Layer Organization

**Domain Layer** (`internal/core/domain/`):
- Entities and value objects
- Repository interfaces
- Service interfaces
- Domain errors
- Domain types and enums

**Service Layer** (`internal/core/services/`):
- Business logic implementations
- Service orchestration
- Domain event handling
- Business rule enforcement

**Infrastructure Layer** (`internal/infrastructure/`):
- Database repositories (3-tier: main → DB-specific → implementations)
- External API clients
- Redis streams for telemetry
- Message queues

**Transport Layer** (`internal/transport/http/`):
- HTTP handlers by domain
- Middleware (auth, CORS, rate limiting)
- WebSocket handlers
- Request/response DTOs

**Application Layer** (`internal/app/`):
- DI container and service registry
- Application bootstrapping
- Graceful shutdown handling
- Health check aggregation

## Multi-Database Strategy

### PostgreSQL (Transactional Data)
**Tables**:
- `users`, `auth_sessions` - Authentication & user management
- `organizations`, `organization_members` - Multi-tenant structure
- `projects` - Project management
- `api_keys` - Project-scoped API keys
- `gateway_*` - AI provider configurations
- `billing_usage` - Usage tracking and billing

**Access**: GORM ORM with struct-based queries

### ClickHouse (Analytics)

**Primary Tables**:
- **traces** - Distributed tracing data
- **spans** - LLM call spans with ZSTD compression
- **quality_scores** - Model performance metrics
- **blob_storage_file_log** - File storage metadata

**Features**:
- OTEL-native schema with namespace prefixes
- ZSTD compression for input/output fields (78% cost reduction)
- TTL-based automatic data retention (365 days)
- Optimized for analytical queries

**Reference**: See `migrations/clickhouse/*.up.sql` for exact schema and TTL configuration

**Access**: Raw SQL with `sqlx` for performance

### Redis (Cache & Queues)
**Usage**:
- JWT token and session storage
- Rate limiting counters
- Background job queues (analytics, notifications)
- Semantic cache for AI responses
- Real-time event pub/sub for WebSocket
- Telemetry streams for worker processing

## Authentication Architecture

### Dual Authentication System

**SDK Routes** (`/v1/*`):
- Authentication: API keys (`bk_{40_char_random}`)
- Format: Professional random key (not JWT-based)
- Storage: SHA-256 hashing for O(1) lookup
- Rate limiting: API key-based
- Middleware: `SDKAuthMiddleware`
- Context access: `middleware.GetSDKAuthContext(c)`

**Dashboard Routes** (`/api/v1/*`):
- Authentication: Bearer JWT tokens
- Session management with Redis storage
- Rate limiting: IP-based + user-based
- Middleware: `Authentication()` + `RateLimitByUser()`
- Context access: `middleware.GetUserIDFromContext(c)`

### API Key Management

**Format**: `bk_{40_char_random}`
- Prefix: `bk_` (Brokle identifier)
- Secret: 40 characters cryptographically secure random
- Example: `bk_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789AbCd`

**Security**:
- SHA-256 hashing for storage (deterministic, O(1) lookup)
- Unique database index on hash
- Project association stored in database
- No sensitive data embedded in key

**Utilities** (`internal/core/domain/auth/apikey_utils.go`):
- `GenerateAPIKey()` - Creates new key
- `ValidateAPIKeyFormat()` - Validates format
- `CreateKeyPreview()` - Secure preview (`bk_AbCd...XyZa`)

## Enterprise Edition Pattern

**Build Tags**: Use `-tags="enterprise"` for enterprise builds

**Structure**:
```
internal/ee/
├── license/       # License validation
├── sso/           # Single Sign-On (SAML 2.0, OIDC)
├── rbac/          # Role-Based Access Control
├── compliance/    # SOC 2, HIPAA, GDPR
└── analytics/     # Enterprise analytics
```

**Pattern**: Interface-based design with stub implementations for OSS

```go
// OSS build (stub)
func NewSSOProvider() SSOProvider {
    return &stubSSOProvider{}
}

// Enterprise build (full implementation)
func NewSSOProvider() SSOProvider {
    return &enterpriseSSOProvider{}
}
```

**Middleware**: `enterprise.go` gates enterprise features

## Background Workers

**Worker Types**:
- `analytics_worker.go` - Metrics aggregation and reporting
- `notification_worker.go` - Email/SMS notifications via queues
- `telemetry_stream_consumer.go` - Redis streams telemetry processing

**Execution**: Workers run in separate binary (`cmd/worker`) with independent scaling

## Dependency Injection

**Container** (`internal/app/app.go`):
- Initializes databases → repositories → services → handlers
- Manages graceful shutdown
- Health check aggregation
- Service lifecycle management

**Pattern**:
```go
// Initialize in order
databases := initDatabases()
repositories := initRepositories(databases)
services := initServices(repositories)
handlers := initHandlers(services)
```

## OTLP Telemetry Ingestion

**Endpoint**:
- `POST /v1/traces` - OTLP standard endpoint (OpenTelemetry specification)

**Processing**:
- Supports: Protobuf (binary) and JSON payloads with explicit Content-Type validation
- Compression: Automatic gzip decompression via Content-Encoding header
- Security: 10MB request size limit (DoS protection), HTTP 415 for invalid Content-Types
- Handler: `OTLPHandler` converts OTLP → Brokle schema
- Converter: `OTLPConverterService` with intelligent root span detection
- Storage: Events → Redis streams → `TelemetryStreamConsumer` worker → ClickHouse

**Schema**: OTEL-native with `brokle.*` namespace for custom attributes

## Multi-Tenant Architecture

**Four Scoping Patterns** (NOT all entities have `organization_id`):

### 1. Organization-Scoped (Direct `organization_id`)
```go
// organization/organization.go:54-57
type Project struct {
    ID             ulid.ULID
    OrganizationID ulid.ULID `json:"organization_id" gorm:"type:char(26);not null"`
    Name           string
}
```

### 2. Project-Scoped (Organization via Project)
```go
// auth/auth.go:94 - APIKey is project-scoped
type APIKey struct {
    ID        ulid.ULID
    ProjectID ulid.ULID `json:"project_id" gorm:"type:char(26);not null;index"`
    // Organization derived via Project join
}
```

### 3. Scoped (Flexible Pattern)
```go
// auth/auth.go:113-114 - Role uses flexible scope_type
type Role struct {
    ScopeType string     `json:"scope_type" gorm:"size:20;not null"`
    ScopeID   *ulid.ULID `json:"scope_id,omitempty" gorm:"type:char(26);index"`
}
```

### 4. Global (No organization_id)
```go
// user/user.go:39 - User is global
type User struct {
    ID    ulid.ULID
    Email string
    DefaultOrganizationID *ulid.ULID `json:"default_organization_id,omitempty"`
    // Users belong to multiple orgs via Member table
}
```

**Row-Level Isolation**: Middleware enforces organization context where applicable

## Health & Monitoring

**Health Endpoints** (`internal/transport/http/server.go:154-159`):
- `/health` - Basic health check (GET, HEAD)
- `/health/ready` - Readiness probe (Kubernetes)
- `/health/live` - Liveness probe (Kubernetes)

**Metrics**:
- `/metrics` - Prometheus-compatible metrics
- Request/response metrics
- Business metrics
- Infrastructure metrics

**Distributed Tracing**:
- Correlation IDs for request tracking
- Structured logging with context
- OpenTelemetry integration
