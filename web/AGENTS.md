# Repository Guidelines

## Project Overview
Next.js 15 frontend for the Brokle AI observability dashboard. Feature-based architecture documented in `ARCHITECTURE.md`.

## Build, Test, and Development Commands
```bash
pnpm install              # Install dependencies
pnpm dev                  # Dev server on :3000
pnpm build                # Production build (standalone output)
pnpm test                 # Vitest unit/integration tests
pnpm test:coverage        # Tests with coverage report
pnpm test:e2e             # Playwright E2E tests
pnpm lint                 # ESLint (next/core-web-vitals + @typescript-eslint)
pnpm type-check           # tsc --noEmit (strict mode)
pnpm format               # Prettier formatting
```

## Coding Style & Naming Conventions
- TypeScript strict mode enabled. Path aliases: `@/*`, `@/features/*`, `@/components/*`, `@/ui/*`, `@/lib/*`, `@/hooks/*`.
- Feature imports: `@/features/[feature]` only — never into internal subdirectories (`@/features/auth/hooks/use-auth` is wrong).
- React components in kebab-case files. shadcn/ui primitives in `src/components/ui/`.
- State management: Zustand with `devtools` and `persist` middleware.
- Data fetching: React Query (TanStack Query) with 60s stale time.

## Testing Guidelines
- **Unit/Integration**: Vitest + jsdom + MSW for API mocking. Tests in `*.test.ts[x]` co-located with source.
- **E2E**: Playwright (Chromium, Firefox, WebKit). Tests in `e2e/*.spec.ts`. Spins up local dev server unless CI.
- Coverage thresholds: 10% (low — add tests for new behavior).

## Commit & Pull Request Guidelines
- Follow Conventional Commits: `feat(web): ...`, `fix(web): ...`.
- Include screenshots for UI changes.
- Run `pnpm lint && pnpm type-check && pnpm test` before opening PRs.

## Known Gotchas

1. **`ignoreBuildErrors: true` in next.config.ts** — TypeScript errors don't fail the build. Always run `pnpm type-check` locally to catch type errors before they reach production.
2. **Standalone output mode** — `next.config.ts` sets `output: 'standalone'`. Docker images use `node .next/standalone`, not static file serving. Don't assume SSG.
3. **No middleware.ts** — Auth is entirely client-side via `src/components/providers.tsx`. There is no Next.js middleware for auth redirects. Session expiry fires a custom `'auth:session-expired'` event that hard-redirects to signin.
4. **API client uses httpOnly cookies, not bearer tokens** — `withCredentials: true` is critical. CSRF tokens are extracted from cookies and added only to mutation requests (POST/PUT/PATCH/DELETE), not GET. Auth token refresh is owned by the Zustand auth store (`useAuthStore`), not the API client.
5. **Context headers are opt-in** — API client methods require explicit flags (`includeOrgContext`, `includeProjectContext`) to send `X-Org-ID` / `X-Project-ID` headers. They are not sent by default — missing them causes 403s.
6. **Smaller-than-default font sizes** — `tailwind.config.ts` overrides: `text-sm` = 0.825rem, `text-base` = 0.9rem (standard Tailwind is 0.875rem / 1rem). Don't assume standard Tailwind sizing.
7. **CSS variables for colors** — Theme colors come from CSS variables in `globals.css`, not Tailwind config values. Use semantic tokens (`bg-background`, `text-foreground`) not raw colors.
8. **Route groups with deep nesting** — Routes use groups: `(auth)`, `(dashboard)`, `(errors)`. Dashboard routes nest deeply: `projects/[projectSlug]/(observability)/traces/[traceId]/`. Pages are thin wrappers delegating to feature components.
9. **Sidebar state in cookies** — Dashboard layout reads sidebar state from cookies server-side to prevent hydration flash. Uses `await cookies()` in server components.
10. **42 shadcn/ui components** — `src/components/ui/` has 42 components including custom ones (`password-input`, `brokle-logo`, `bipolar-slider`). Check existing components before creating new UI primitives.

## Compatibility Notes
- Backward compatibility is not required yet; there is no production data because the product has not been released.
