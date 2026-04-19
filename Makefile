# Makefile for Brokle AI Control Plane
#
# This Makefile provides automation for development, testing, building,
# and deployment of the Brokle platform.

# Available commands:
.PHONY: help setup install-deps install-tools ensure-swag setup-databases
.PHONY: dev dev-server dev-worker dev-frontend stop-dev
.PHONY: build build-oss build-enterprise build-server-oss build-worker-oss
.PHONY: build-server-enterprise build-worker-enterprise build-frontend build-all
.PHONY: build-dev-server build-dev-worker
.PHONY: migrate-up migrate-down migrate-status seed create-migration
.PHONY: test test-coverage test-unit test-integration
.PHONY: lint lint-go lint-frontend fmt fmt-frontend
.PHONY: generate docs-generate
.PHONY: clean-builds shell-db shell-redis shell-clickhouse
.PHONY: release-patch release-minor release-major release-patch-skip-tests release-dry

# Default target
help: ## Show this help message
	@echo "Brokle AI Control Plane - Available Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""

##@ Development

# Helper to kill processes on specific ports
define kill-port
	@-lsof -ti:$(1) | xargs kill 2>/dev/null || true
endef

setup: ## Setup development environment
	@echo "🚀 Setting up development environment..."
	@$(MAKE) install-deps
	@$(MAKE) install-tools
	@$(MAKE) setup-databases
	@$(MAKE) migrate-up
	@$(MAKE) seed
	@$(MAKE) generate
	@echo "✅ Development environment ready!"

install-deps: ## Install Go and Node.js dependencies
	@echo "📦 Installing dependencies..."
	go mod download
	cd web && pnpm install

install-tools: ## Install development tools (swag, air, golangci-lint, sqlc)
	@echo "🔧 Installing development tools..."
	@echo "Installing Go development tools (swag, air, sqlc)..."
	@go install github.com/swaggo/swag/cmd/swag@v1.16.6
	@go install github.com/air-verse/air@latest
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
	@echo "Installing golangci-lint v2.6.2 (Go 1.25 compatible)..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin v2.6.2
	@echo "✅ Tools installed successfully"

ensure-sqlc: ## Ensure sqlc is installed (auto-installs if missing)
	@command -v sqlc >/dev/null 2>&1 || { \
		echo "⚠️  sqlc not found, installing sqlc v1.30.0..."; \
		go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0; \
		echo "✅ sqlc installed successfully"; \
	}

ensure-swag: ## Ensure swag is installed (auto-installs if missing)
	@command -v swag >/dev/null 2>&1 || { \
		echo "⚠️  swag not found, installing swag v1.16.6..."; \
		go install github.com/swaggo/swag/cmd/swag@v1.16.6; \
		echo "✅ swag installed successfully"; \
	}

setup-databases: ## Start databases with Docker Compose
	@echo "🗄️ Starting databases..."
	docker compose up -d postgres clickhouse redis
	@echo "⏳ Waiting for databases to be ready..."
	@sleep 10

dev: ## Start full stack (server + worker)
	@echo "🔥 Starting full stack development..."
	@$(MAKE) -j2 dev-server dev-worker

dev-server: ## Start HTTP server with hot reload
	@echo "🔥 Starting HTTP server with hot reload..."
	@echo "   Cleaning up previous instances..."
	@-pkill -f "air -c .air.toml" 2>/dev/null || true
	@-pkill -f "./tmp/server" 2>/dev/null || true
	$(call kill-port,8080)
	$(call kill-port,4317)
	@sleep 0.5
	air -c .air.toml

dev-worker: ## Start workers with hot reload
	@echo "🔥 Starting workers with hot reload..."
	@echo "   Cleaning up previous instances..."
	@-pkill -f "air -c .air.worker.toml" 2>/dev/null || true
	@-pkill -f "./tmp/worker" 2>/dev/null || true
	@sleep 0.5
	air -c .air.worker.toml

stop-dev: ## Stop all development processes
	@echo "🛑 Stopping all development processes..."
	@-pkill -f "air -c .air" 2>/dev/null || true
	@-pkill -f "./tmp/server" 2>/dev/null || true
	@-pkill -f "./tmp/worker" 2>/dev/null || true
	$(call kill-port,8080)
	$(call kill-port,4317)
	@echo "✅ All development processes stopped"

dev-frontend: ## Start Next.js development server only
	@echo "⚛️ Starting Next.js development server..."
	cd web && pnpm run dev

##@ Building

build: build-server-oss build-worker-oss ## Build both server and worker (OSS)

build-oss: ## Build OSS binaries (server + worker)
	@$(MAKE) build-server-oss
	@$(MAKE) build-worker-oss
	@echo "✅ OSS builds complete!"

build-enterprise: ## Build Enterprise binaries (server + worker)
	@$(MAKE) build-server-enterprise
	@$(MAKE) build-worker-enterprise
	@echo "✅ Enterprise builds complete!"

build-server-oss: ## Build HTTP server (OSS version)
	@echo "🔨 Building HTTP server (OSS)..."
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/brokle-server cmd/server/main.go

build-server-enterprise: ## Build HTTP server (Enterprise version)
	@echo "🔨 Building HTTP server (Enterprise)..."
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags="enterprise" -ldflags="-w -s" -o bin/brokle-server-enterprise cmd/server/main.go

build-worker-oss: ## Build worker process (OSS version)
	@echo "🔨 Building worker process (OSS)..."
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o bin/brokle-worker cmd/worker/main.go

build-worker-enterprise: ## Build worker process (Enterprise version)
	@echo "🔨 Building worker process (Enterprise)..."
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags="enterprise" -ldflags="-w -s" -o bin/brokle-worker-enterprise cmd/worker/main.go

build-frontend: ## Build Next.js for production
	@echo "🔨 Building Next.js frontend..."
	cd web && pnpm run build

build-all: build-server-oss build-worker-oss build-server-enterprise build-worker-enterprise build-frontend ## Build all variants
	@echo "✅ All builds complete!"

build-dev-server: ## Build server for development (faster, with debug info)
	@echo "🔨 Building server for development..."
	mkdir -p bin
	go build -o bin/brokle-dev-server cmd/server/main.go

build-dev-worker: ## Build worker for development (faster, with debug info)
	@echo "🔨 Building worker for development..."
	mkdir -p bin
	go build -o bin/brokle-dev-worker cmd/worker/main.go

##@ Database Operations

migrate-up: ## Run all pending migrations
	@echo "📊 Running database migrations..."
	go run cmd/migrate/main.go up

migrate-down: ## Rollback one migration
	@echo "📊 Rolling back one migration..."
	go run cmd/migrate/main.go down

migrate-status: ## Show migration status
	@echo "📊 Migration status:"
	go run cmd/migrate/main.go status

seed: ## Seed system data (permissions, roles, pricing)
	@echo "🌱 Seeding system data..."
	go run cmd/migrate/main.go seed

create-migration: ## Create new migration (usage: make create-migration DB=postgres NAME=add_users_table)
	@if [ -z "$(DB)" ] || [ -z "$(NAME)" ]; then \
		echo "Usage: make create-migration DB=postgres|clickhouse NAME=migration_name"; \
		exit 1; \
	fi
	go run cmd/migrate/main.go create -db $(DB) -name $(NAME)

##@ Testing

test: ## Run all tests
	@echo "🧪 Running all tests..."
	go test -v ./...

test-coverage: ## Run tests with coverage report
	@echo "🧪 Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

test-unit: ## Run unit tests only
	@echo "🧪 Running unit tests..."
	go test -v -short ./...

test-integration: ## Run integration tests only
	@echo "🧪 Running integration tests..."
	go test -v -tags=integration ./tests/integration/...

##@ Code Quality

lint: lint-conventions lint-go lint-frontend ## Run all linters

lint-go: ## Run Go linter
	@echo "🔍 Running Go linter..."
	golangci-lint run --config .golangci.yml

lint-conventions: ## Run project-convention lint (DDL, filename, etc.)
	@bash scripts/lint-conventions.sh

lint-frontend: ## Run frontend linter
	@echo "🔍 Running frontend linter..."
	cd web && pnpm run lint

fmt: ## Format Go code
	@echo "💅 Formatting Go code..."
	go fmt ./...
	goimports -w .

fmt-frontend: ## Format frontend code
	@echo "💅 Formatting frontend code..."
	cd web && pnpm run format

##@ Documentation

generate: ensure-swag generate-sqlc ## Generate swagger docs + sqlc types and run go generate
	@echo "📚 Generating swagger documentation..."
	swag init -g cmd/server/main.go --output docs
	@echo "✅ Code generation complete"

generate-sqlc: ensure-sqlc ## Generate type-safe Go bindings from SQL queries
	@echo "🧬 Generating sqlc bindings..."
	@sqlc generate
	@echo "✅ sqlc generation complete"

##@ Utilities

clean-builds: ## Clean only build artifacts (keep dependencies)
	@echo "🧹 Cleaning build artifacts only..."
	rm -rf bin/
	rm -rf web/.next/
	rm -f coverage.out coverage.html

shell-db: ## Get shell access to PostgreSQL
	docker compose exec postgres psql -U brokle -d brokle

shell-redis: ## Get shell access to Redis
	docker compose exec redis redis-cli

shell-clickhouse: ## Get shell access to ClickHouse
	docker compose exec clickhouse clickhouse-client

##@ Release

release-patch: ## Release patch version (v0.1.0 → v0.1.1)
	@bash scripts/release.sh patch

release-minor: ## Release minor version (v0.1.0 → v0.2.0)
	@bash scripts/release.sh minor

release-major: ## Release major version (v0.1.0 → v1.0.0)
	@bash scripts/release.sh major

release-patch-skip-tests: ## Release patch version (skip tests)
	@bash scripts/release.sh patch --skip-tests

release-dry: ## Preview release without making changes
	@bash scripts/release.sh patch --dry-run

##@ Default

.DEFAULT_GOAL := help
