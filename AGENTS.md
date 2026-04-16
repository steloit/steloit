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
- Implement backend changes using the Repository → Service → Handler flow.
- In services, construct errors with `AppError` constructors; in handlers, return errors through `response.Error()`.
- **Handler validation errors** must use `response.Error(c, appErrors.NewValidationError(message, details))` — never `response.BadRequest()`, never `response.ValidationError()` directly. Messages use Title Case (`"Invalid project ID"`), details use lowercase (`"projectId must be a valid UUID"`).
- **DELETE handlers** must return `response.NoContent(c)` (HTTP 204) — never `response.Success(c, gin.H{"message": ...})`.
- **UPDATE handlers** must return the updated entity via `response.Success(c, entity)` — never a message-only response (auth security flows like logout/password reset are the exception).
- **Parameter parsing** uses inline `uuid.Parse(c.Param(...))` and `c.ShouldBindJSON(&req)` directly in handlers — no shared wrapper helpers. This follows the Go/Gin ecosystem convention (PhotoPrism, Apache Answer).

## Known Gotchas

### Backend

1. **Swagger docs are generated, not tracked** — `docs/` is gitignored. Run `make generate` after changing API annotations. If tests fail with "cannot find package brokle/docs", run `make generate`.
2. **Migration files must use the CLI** — `go run cmd/migrate/main.go -db <postgres|clickhouse> -name <name> create`. Manual files in `migrations/` are silently ignored by the framework.
3. **`json.RawMessage` fields in domain entities** — Several domain types use `json.RawMessage` for provider-agnostic JSON (`observability.Score.Metadata`, `prompt.ModelConfig.Tools/ToolChoice/ResponseFormat`). Always use DTO conversion in handlers; never serialize domain entities with `json.RawMessage` fields directly via `response.Success()`.
4. **Error import alias** — Services and handlers import `appErrors "brokle/pkg/errors"` (not `errors`). Handlers use `response.Error(c, appErrors.NewValidationError(msg, details))` for validation failures and `response.Error(c, err)` to forward service errors. The canonical validation pattern is: `response.Error(c, appErrors.NewValidationError("Invalid project ID", "projectId must be a valid UUID"))`.
5. **Transaction injection only works with PostgreSQL repos** — Repositories using `*gorm.DB` extract transactions from context via `shared.GetDB(ctx, r.db)`. ClickHouse repositories use `clickhouse.Conn` directly (raw driver, no GORM) — transaction injection does not apply to them.
6. **Dual auth context keys** — SDK routes (`/v1/*`) set `SDKAuthContextKey`, `APIKeyIDKey`, `ProjectIDKey`. Dashboard routes (`/api/v1/*`) set `AuthContextKey`, `UserIDKey`. Using `GetUserID()` in an SDK handler or `GetSDKAuthContext()` in a dashboard handler silently returns zero values. Always match the getter to the route group.
7. **Dual ports** — HTTP API runs on port **8080**, gRPC (OTLP telemetry ingestion) runs on port **4317**. They are independent servers.
8. **Enterprise build tag** — `-tags="enterprise"` gates SSO, RBAC, and compliance features in `internal/ee/`. OSS builds have stubs. Don't assume EE features exist without the tag.
9. **Background email pattern** — All `emailSender.Send()` calls triggered by HTTP requests must use a detached context: `go func() { ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second); defer cancel(); ... }()`. Never use the request context for async work that must complete after the handler returns.
10. **Worker DLQ semantics** — `telemetry_stream_consumer.go` uses `ErrMovedToDLQ` to signal "safe to ACK even though processing failed" (data preserved in DLQ). Non-DLQ errors leave messages pending for retry. New workers consuming from Redis streams must follow this pattern: ACK on success OR `ErrMovedToDLQ`, leave pending on other errors.
11. **Route middleware order matters** — In `server.go`, middleware is applied in order: auth → CSRF → rate limit → handler. CSRF must come after auth (needs user context). Rate limiters differ by route group: SDK routes use `RateLimitByAPIKey()`, dashboard routes use default limits.
12. **Config validation is mode-specific** — `config.go` skips certain validations when `APP_MODE=worker` (e.g., `JWT_SECRET` not required). If promoting a worker to server, all server-mode validations must pass.

### Frontend

13. **Frontend `ignoreBuildErrors: true`** — `web/next.config.ts` disables TypeScript build errors. Type errors won't fail the build; run `pnpm type-check` locally.
14. **Standalone output mode** — Frontend builds as a standalone Node.js app (`output: 'standalone'`), not static files. Docker uses `node .next/standalone`.
15. **API client auth is cookie-based** — `withCredentials: true` is required for httpOnly cookies. CSRF tokens are extracted from cookies and added to mutation requests (POST/PUT/PATCH/DELETE) only, not GET. Auth token refresh is owned by the Zustand auth store, not the API client.
16. **Context headers are opt-in** — API client methods require explicit flags (`includeOrgContext`, `includeProjectContext`) to attach `X-Org-ID` / `X-Project-ID` headers. They are not sent by default.
17. **Feature module structure** — Each feature in `web/src/features/` follows: `api/`, `components/`, `hooks/`, `stores/`, `types/`, `utils/`. Import across features only via `@/features/[feature]`, never into internal subdirectories.

### SDKs (JavaScript & Python)

18. **SDKs are git submodules** — `sdk/javascript/` and `sdk/python/` are independent repos mounted as submodules. Commits inside them don't appear in the main repo's `git diff` — only the submodule pointer changes. Use `cd sdk/javascript && git status` to inspect SDK changes. Run `git submodule update --init` after cloning.
19. **First-write-wins singleton** — Both SDKs use a global singleton pattern. JS uses `Symbol.for('brokle')` on `globalThis`; Python uses a module-level `_client` variable. First `BrokleClient()` / `Brokle()` call wins — subsequent calls with different configs are ignored. Use `setClient()` / `set_client()` to explicitly override.
20. **Provider wrappers are optional peer deps** — JS: `wrapOpenAI`, `wrapAnthropic`, etc. require the provider SDK installed but won't fail at import — only at wrapper call. Python: same pattern via `brokle.wrappers`. Never bundle provider SDKs as direct dependencies.
21. **JS: Proxy pattern for wrappers** — `wrapOpenAI()` returns a recursive `Proxy` that intercepts method calls without modifying the original client. Don't extend or subclass provider clients; wrap them.
22. **JS: Multi-entry tsup build** — 11 entry points (core + 10 integrations) with separate `.d.ts` files. Import from sub-paths: `import { wrapOpenAI } from 'brokle/openai'`, not from root `'brokle'`.
23. **JS: Node >= 20 required** — Relies on `AsyncLocalStorage` for context scoping. No browser or Node 18 support without polyfills.
24. **Python: Lazy module loading** — `brokle/__init__.py` uses `__getattr__` + `importlib` for 150+ exports. `from brokle import *` won't work. Import specific names.
25. **Python: `BROKLE_` env var prefix** — All env vars are uppercase with `BROKLE_` prefix: `BROKLE_API_KEY`, `BROKLE_BASE_URL`, `BROKLE_ENABLED`. Lowercase variants don't work.
26. **Python: No atexit flush** — SDK doesn't register process exit handlers. Serverless/CLI apps must call `brokle.flush()` before process exit or traces are lost.
27. **Python: mypy selectively disabled** — `pyproject.toml` has `ignore_errors = true` overrides for wrappers, config, and client modules. Don't assume full type safety in those areas.

## Lessons Learned

- 2026-04-14: Changing a domain entity field type (e.g., `string` → `json.RawMessage`) requires auditing ALL handlers that serialize that entity. DTO conversion helpers (`toScoreResponse()`) must be used at every endpoint — missing one creates a serialization regression. Swagger annotations must also be updated to reference the DTO type, not the domain entity.
- 2026-04-14: Synchronous email sends using the HTTP request context are silently dropped when the client disconnects. Always detach email sends into a goroutine with `context.WithTimeout(context.Background(), 30*time.Second)` — matching the invitation service pattern at `internal/core/services/organization/invitation_service.go:147`.
- 2026-04-14: Swagger `@Success` annotations must reference the DTO type actually returned by the handler, not the domain entity. Stale annotations cause `make generate` to produce incorrect OpenAPI schemas that mislead SDK clients.
- 2026-04-15: Handler-layer consistency was enforced by standardizing on ONE error pattern (`response.Error(c, appErrors.NewValidationError(...))`) and inline `uuid.Parse(c.Param(...))` / `c.ShouldBindJSON(&req)` across all 41+ handler files. Shared param extraction helpers were tried and removed — the Go/Gin ecosystem (PhotoPrism, Apache Answer) uses inline parsing, not abstractions. The `evaluation` domain retains a package-local `extractProjectID()` for dual SDK/Dashboard routes.

## Current Product Focus

- Prioritize core observability, evaluation, and analytics flows.
- Website/landing page features (contact forms, etc.) are secondary — keep them working but don't over-engineer.
- SDK improvements (JavaScript, Python) are high priority when explicitly requested.
- Do not spend time hardening deferred features for theoretical completeness.

## Compatibility Notes
- Backward compatibility is not required yet; there is no production data because the product has not been released.
