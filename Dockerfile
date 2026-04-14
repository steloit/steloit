# Multi-stage build for Brokle HTTP Server
FROM golang:1.26-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Install swag for generating API docs (pinned version for reproducibility)
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

# Generate Swagger documentation (required for server build)
RUN swag init -g cmd/server/main.go --output docs

# Build the server binary (with cache for faster rebuilds)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s' \
    -o bin/brokle-server \
    ./cmd/server

# Build migration tool (with cache for faster rebuilds)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s' \
    -o bin/migrate \
    ./cmd/migrate

# Final stage
FROM alpine:latest

# Install ca-certificates and wget for HTTPS and health checks
RUN apk --no-cache add ca-certificates wget

# Create non-root user
RUN adduser -D -s /bin/sh brokle

# Set working directory
WORKDIR /app

# Copy binaries from builder stage
COPY --from=builder /app/bin/brokle-server ./brokle-server
COPY --from=builder /app/bin/migrate ./migrate

# Copy configuration files
COPY --from=builder /app/configs ./configs

# Copy migrations for database initialization
COPY --from=builder /app/migrations ./migrations

# Copy seed data
COPY --from=builder /app/seeds ./seeds

# Create necessary directories
RUN mkdir -p logs tmp && chown -R brokle:brokle /app

# Switch to non-root user
USER brokle

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
CMD ["./brokle-server"]
