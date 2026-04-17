# Repository Guidelines

## Project Structure & Module Organization
Brokle is a monorepo with a Go backend, a Next.js dashboard, and SDKs.
- `cmd/`: service entrypoints (`server`, `worker`, `migrate`).
- `internal/`: core domain logic, services, infrastructure, and workers.
- `pkg/`: shared Go utilities used across services.
- `web/`: Next.js app (`src/` features, `e2e/` Playwright tests, `public/` assets).
- `migrations/` and `seeds/`: Postgres/ClickHouse schema migrations and seed data.
- `tests/`: Go integration/infrastructure tests.
- `sdk/python` and `sdk/javascript`: language SDKs with independent tooling.

## Build, Test, and Development Commands
Use Make from the repo root for platform workflows:
- `make setup`: install dependencies/tools, start DBs, run migrations/seeds, generate docs.
- `make dev`: run backend server and worker with hot reload (`air`).
- `make dev-frontend`: run dashboard only (`web` on `:3000`).
- `make test` / `make test-unit` / `make test-integration`: Go test suites.
- `make lint` / `make fmt`: Go + frontend lint/format wrappers.
- `make build` and `make build-frontend`: production builds.

Frontend direct commands (inside `web/`): `pnpm test`, `pnpm test:coverage`, `pnpm test:e2e`.

## Coding Style & Naming Conventions
- Go: follow `gofmt`/`goimports`; lint rules are enforced via `.golangci.yml`.
- TypeScript/React: ESLint + Prettier (`web/eslint.config.mjs`, `pnpm format`).
- Python SDK: Black/isort/flake8/mypy (`sdk/python/tox.ini`).
- Naming: keep Go packages lowercase, migration files timestamp-prefixed, React component files in kebab-case, and tests adjacent to relevant modules where possible.
- Frontend imports: use feature boundaries (`@/features/[feature]`) and avoid internal deep imports across features.

## Testing Guidelines
- Go tests use `*_test.go`; run `make test` before opening PRs.
- Frontend unit/integration tests use Vitest (`*.test.ts[x]`), E2E uses Playwright (`web/e2e/*.spec.ts`).
- Python SDK uses pytest (`test_*.py`) with markers (`unit`, `integration`, `slow`).
- No strict global coverage threshold is defined; add or update tests for every behavior change.

## Commit & Pull Request Guidelines
- Follow Conventional Commit style seen in history: `feat(scope): ...`, `fix(scope): ...`, `refactor(scope): ...`, `chore: ...`.
- Keep commits focused and logically grouped.
- PRs should follow `.github/pull_request_template.md`: include summary, change type, testing performed, related issue (`Closes #...`), and screenshots for UI changes.
- Ensure local tests pass before requesting review.

## Mandatory Development Rules
- Create migrations only via CLI: `go run cmd/migrate/main.go -db <postgres|clickhouse> -name <name> create`.
- Do not create files manually under `migrations/`.
- **DDL canonical forms**: use `TIMESTAMP WITH TIME ZONE` (not `TIMESTAMPTZ`) in all Postgres DDL. sqlc's parser treats the short form as a non-`pg_catalog.*` type in `ALTER TABLE ADD COLUMN`, so the `pg_catalog.timestamptz` → `time.Time` override silently misses and the generated type falls back to `pgtype.Timestamptz`. Same principle: prefer full canonical forms in migrations (`TIMESTAMP WITHOUT TIME ZONE`, `INTEGER`, `BOOLEAN`) over shortcuts.
- **sqlc override keys use `pg_catalog.*`**: for built-in Postgres types, the override key is `pg_catalog.<type>` (e.g. `pg_catalog.numeric`, `pg_catalog.timestamptz`, `pg_catalog.timestamp`). Unprefixed forms like `numeric` or `timestamptz` are dead config — sqlc canonicalises column types to the `pg_catalog.*` form before matching, so shortened keys never match.
- **Editing sqlc.yaml — mandatory procedure**:
  1. Read the existing file; grep for the type in both prefixed and unprefixed forms.
  2. Run `make generate-sqlc` with the current config — do not assume what the output will be.
  3. Grep the generated output: `grep -c "pgtype.<Type>" internal/infrastructure/db/gen/*.go`. If count is 0, the existing config works — do not edit.
  4. If count is >0, inspect which columns leak and diagnose root cause (DDL shorthand? schema-lie nullable? codec missing?) before editing YAML.
  5. When adding an override, use the `pg_catalog.<type>` form.
  6. Every Go type mapping in sqlc.yaml must have a corresponding pgx codec registered in `internal/infrastructure/db/codecs.go` (via `pgxpool.Config.AfterConnect`). YAML tells codegen what type to emit; pgx codec tells the driver how to decode. Both must be present for runtime correctness — missing codec = compile success + scan-time panic.
- Implement backend changes using the Repository → Service → Handler flow.
- In services, construct errors with `AppError` constructors; in handlers, return errors through `response.Error()`.
- **Handler validation errors** must use `response.Error(c, appErrors.NewValidationError(message, details))` — never `response.BadRequest()`, never `response.ValidationError()` directly. Messages use Title Case (`"Invalid project ID"`), details use lowercase (`"projectId must be a valid UUID"`).
- **DELETE handlers** must return `response.NoContent(c)` (HTTP 204) — never `response.Success(c, gin.H{"message": ...})`.
- **UPDATE handlers** must return the updated entity via `response.Success(c, entity)` — never a message-only response (auth security flows like logout/password reset are the exception).
- **Parameter parsing** uses inline `uuid.Parse(c.Param(...))` and `c.ShouldBindJSON(&req)` directly in handlers — no shared wrapper helpers. This follows the Go/Gin ecosystem convention (PhotoPrism, Apache Answer).
- **Auth context access** uses the `Must*` helpers on routes protected by `RequireAuth`/`RequireSDKAuth`: `middleware.MustGetUserID(c)`, `MustGetAuthContext(c)`, `MustGetTokenClaims(c)`, `MustGetProjectID(c)`, `MustGetOrganizationID(c)`, `MustGetSDKAuthContext(c)`. Panics are caught by `middleware.Recovery` → HTTP 500 on misconfiguration (missing auth middleware). The tuple-return `GetUserIDFromContext(c) (uuid.UUID, bool)` / `GetProjectID(c) (uuid.UUID, bool)` / etc. are reserved for `OptionalAuth` routes and audit-field population where unauthenticated is a legitimate runtime case. Never defensively combine `Must*` with an existence check — it defeats the invariant signal.
- **ID generation** uses `uid.New()` (UUIDv7, monotonic by spec). Do not introduce new ULID usage; `pkg/ulid` has been removed.

## Known Gotchas

### Backend

1. **Swagger docs are generated, not tracked** — `docs/` is gitignored. Run `make generate` after changing API annotations. If tests fail with "cannot find package brokle/docs", run `make generate`.
2. **Migration files must use the CLI** — `go run cmd/migrate/main.go -db <postgres|clickhouse> -name <name> create`. Manual files in `migrations/` are silently ignored by the framework.
3. **`json.RawMessage` fields in domain entities** — Several domain types use `json.RawMessage` for provider-agnostic JSON (`observability.Score.Metadata`, `prompt.ModelConfig.Tools/ToolChoice/ResponseFormat`). Always use DTO conversion in handlers; never serialize domain entities with `json.RawMessage` fields directly via `response.Success()`.
4. **Error import alias** — Services and handlers import `appErrors "brokle/pkg/errors"` (not `errors`). Handlers use `response.Error(c, appErrors.NewValidationError(msg, details))` for validation failures and `response.Error(c, err)` to forward service errors. The canonical validation pattern is: `response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))`.
5. **Transaction scoping** — PostgreSQL repositories take `*db.TxManager`; methods call `r.tm.Queries(ctx)` for sqlc-generated queries and `r.tm.DB(ctx)` for squirrel-built dynamic queries. Transactions propagate through `ctx` (set by `TxManager.WithinTransaction(ctx, fn)`), so passing the same context through nested service calls keeps them in the same tx. ClickHouse repositories use `clickhouse.Conn` directly (raw driver) — transaction scoping does not apply there.
6. **Dual auth context keys** — SDK routes (`/v1/*`) set `SDKAuthContextKey`, `APIKeyIDKey`, `ProjectIDKey`, `OrganizationIDKey`. Dashboard routes (`/api/v1/*`) set `AuthContextKey`, `UserIDKey`, `TokenClaimsKey`. Using `MustGetUserID()` in an SDK handler or `MustGetSDKAuthContext()` in a dashboard handler panics (caught by Recovery → 500). Always match the getter to the route group.
7. **Dual ports** — HTTP API runs on port **8080**, gRPC (OTLP telemetry ingestion) runs on port **4317**. They are independent servers.
8. **Enterprise build tag** — `-tags="enterprise"` gates SSO, RBAC, and compliance features in `internal/ee/`. OSS builds have stubs. Don't assume EE features exist without the tag.
9. **Background email pattern** — All `emailSender.Send()` calls triggered by HTTP requests must use a detached context: `go func() { ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second); defer cancel(); ... }()`. Never use the request context for async work that must complete after the handler returns.
10. **Worker DLQ semantics** — `telemetry_stream_consumer.go` uses `ErrMovedToDLQ` to signal "safe to ACK even though processing failed" (data preserved in DLQ). Non-DLQ errors leave messages pending for retry. New workers consuming from Redis streams must follow this pattern: ACK on success OR `ErrMovedToDLQ`, leave pending on other errors.
11. **Route middleware order matters** — In `server.go`, middleware is applied in order: auth → CSRF → rate limit → handler. CSRF must come after auth (needs user context). Rate limiters differ by route group: SDK routes use `RateLimitByAPIKey()`, dashboard routes use default limits.
12. **Config validation is mode-specific** — `config.go` skips certain validations when `APP_MODE=worker` (e.g., `JWT_SECRET` not required). If promoting a worker to server, all server-mode validations must pass.
13. **Auth context storage types** — `UserIDKey` / `ProjectIDKey` / `OrganizationIDKey` all store `uuid.UUID` **by value**. Only `APIKeyIDKey` stores `*uuid.UUID` because `AuthContext.APIKeyID` is legitimately nullable (session-based auth contexts have no API key). Don't mix value/pointer forms — `MustGetProjectID` etc. strictly type-assert `uuid.UUID` and will panic on a pointer.
14. **No platform-admin concept** — All roles in `seeds/roles.yaml` are `scope_type: "organization"` (owner, admin, developer, viewer). There is no platform-wide admin role; the `annotation.RoleAdmin` constant is an unrelated per-queue reviewer role. If you think you need platform-admin endpoints (token revocation, incident response, etc.), stop and design the role/permission model first — don't scaffold unreachable guarded routes.

### Frontend

15. **Frontend `ignoreBuildErrors: true`** — `web/next.config.ts` disables TypeScript build errors. Type errors won't fail the build; run `pnpm type-check` locally.
16. **Standalone output mode** — Frontend builds as a standalone Node.js app (`output: 'standalone'`), not static files. Docker uses `node .next/standalone`.
17. **API client auth is cookie-based** — `withCredentials: true` is required for httpOnly cookies. CSRF tokens are extracted from cookies and added to mutation requests (POST/PUT/PATCH/DELETE) only, not GET. Auth token refresh is owned by the Zustand auth store, not the API client.
18. **Context headers are opt-in** — API client methods require explicit flags (`includeOrgContext`, `includeProjectContext`) to attach `X-Org-ID` / `X-Project-ID` headers. They are not sent by default.
19. **Feature module structure** — Each feature in `web/src/features/` follows: `api/`, `components/`, `hooks/`, `stores/`, `types/`, `utils/`. Import across features only via `@/features/[feature]`, never into internal subdirectories.

### SDKs (JavaScript & Python)

20. **SDKs are git submodules** — `sdk/javascript/` and `sdk/python/` are independent repos mounted as submodules. Commits inside them don't appear in the main repo's `git diff` — only the submodule pointer changes. Use `cd sdk/javascript && git status` to inspect SDK changes. Run `git submodule update --init` after cloning.
21. **First-write-wins singleton** — Both SDKs use a global singleton pattern. JS uses `Symbol.for('brokle')` on `globalThis`; Python uses a module-level `_client` variable. First `BrokleClient()` / `Brokle()` call wins — subsequent calls with different configs are ignored. Use `setClient()` / `set_client()` to explicitly override.
22. **Provider wrappers are optional peer deps** — JS: `wrapOpenAI`, `wrapAnthropic`, etc. require the provider SDK installed but won't fail at import — only at wrapper call. Python: same pattern via `brokle.wrappers`. Never bundle provider SDKs as direct dependencies.
23. **JS: Proxy pattern for wrappers** — `wrapOpenAI()` returns a recursive `Proxy` that intercepts method calls without modifying the original client. Don't extend or subclass provider clients; wrap them.
24. **JS: Multi-entry tsup build** — 11 entry points (core + 10 integrations) with separate `.d.ts` files. Import from sub-paths: `import { wrapOpenAI } from 'brokle/openai'`, not from root `'brokle'`.
25. **JS: Node >= 20 required** — Relies on `AsyncLocalStorage` for context scoping. No browser or Node 18 support without polyfills.
26. **Python: Lazy module loading** — `brokle/__init__.py` uses `__getattr__` + `importlib` for 150+ exports. `from brokle import *` won't work. Import specific names.
27. **Python: `BROKLE_` env var prefix** — All env vars are uppercase with `BROKLE_` prefix: `BROKLE_API_KEY`, `BROKLE_BASE_URL`, `BROKLE_ENABLED`. Lowercase variants don't work.
28. **Python: No atexit flush** — SDK doesn't register process exit handlers. Serverless/CLI apps must call `brokle.flush()` before process exit or traces are lost.
29. **Python: mypy selectively disabled** — `pyproject.toml` has `ignore_errors = true` overrides for wrappers, config, and client modules. Don't assume full type safety in those areas.

## Lessons Learned

- 2026-04-14: Changing a domain entity field type (e.g., `string` → `json.RawMessage`) requires auditing ALL handlers that serialize that entity. DTO conversion helpers (`toScoreResponse()`) must be used at every endpoint — missing one creates a serialization regression. Swagger annotations must also be updated to reference the DTO type, not the domain entity.
- 2026-04-14: Synchronous email sends using the HTTP request context are silently dropped when the client disconnects. Always detach email sends into a goroutine with `context.WithTimeout(context.Background(), 30*time.Second)` — matching the invitation service pattern at `internal/core/services/organization/invitation_service.go:147`.
- 2026-04-14: Swagger `@Success` annotations must reference the DTO type actually returned by the handler, not the domain entity. Stale annotations cause `make generate` to produce incorrect OpenAPI schemas that mislead SDK clients.
- 2026-04-15: Handler-layer consistency was enforced by standardizing on ONE error pattern (`response.Error(c, appErrors.NewValidationError(...))`) and inline `uuid.Parse(c.Param(...))` / `c.ShouldBindJSON(&req)` across all 41+ handler files. Shared param extraction helpers were tried and removed — the Go/Gin ecosystem (PhotoPrism, Apache Answer) uses inline parsing, not abstractions. The `evaluation` domain retains a package-local `extractProjectID()` for dual SDK/Dashboard routes.
- 2026-04-17: Defensive handler-level auth checks (`if !exists { 401 }` after `middleware.GetXxx(c)`) mask programming errors and add boilerplate. The idiomatic Go fix is the `Must*` convention from `regexp.MustCompile` / `template.Must`: handlers trust the middleware invariant and call `middleware.MustGetUserID(c)` etc. directly; a misconfigured route (missing `RequireAuth`) now panics → `middleware.Recovery` → HTTP 500 with stack trace, surfacing the bug instead of silently returning a misleading 401. Tuple-return `Get*` forms are reserved for `OptionalAuth` routes. Before adding a new "defensive" 401 in a handler, ask whether the middleware invariant is guaranteed — if yes, use `Must*`.
- 2026-04-17: Bulk `sed` replacements on SQL migrations require replacing longer patterns first (`VARCHAR(26)` before `CHAR(26)`), otherwise `CHAR(26)` inside `VARCHAR(26)` matches first and produces invalid types like `VARUUID`. Same applies to frontend: `sed 's/ulid/uuid/g'` on TS files corrupted `import('ulid')` → `import('uuid')` (package didn't exist) and `ulid()` → `uuid()` (wrong API). Always verify dynamic imports and runtime calls after bulk renames.
- 2026-04-17: ID type wrappers should only exist when they earn their place. `uuid.UUID` from `google/uuid` already implements `Scan`, `Value`, `MarshalText`, `UnmarshalText` — wrapping it in a custom type (like SigNoz does) forces re-implementing 12+ interface methods for zero benefit. `pkg/uid` has exactly two functions: `uid.New()` (encodes the UUIDv7 decision) and `uid.TimeFromID()` (non-trivial extraction logic). Everything else is `uuid.Parse()`, `uuid.Nil`, `id.String()` directly. Removed `uid.Parse()`, `uid.MustParse()`, `uid.IsZero()`, `uid.Nil` after recognizing they were trivial pass-throughs.
- 2026-04-17: Scaffolded-but-unreachable code must be deleted, not kept "for later." The `admin/token_admin.go` package with four `/admin/tokens/*` routes was guarded by `admin:manage` — a permission never seeded. No user could ever call it; always 403. Kept for months until an audit found it. CLAUDE.md rule: "don't design for hypothetical future requirements." Git history preserves the implementation if a real need arrives.

## Current Product Focus

- Prioritize core observability, evaluation, and analytics flows.
- Website/landing page features (contact forms, etc.) are secondary — keep them working but don't over-engineer.
- SDK improvements (JavaScript, Python) are high priority when explicitly requested.
- Do not spend time hardening deferred features for theoretical completeness.

## Compatibility Notes
- Backward compatibility is not required yet; there is no production data because the product has not been released.
