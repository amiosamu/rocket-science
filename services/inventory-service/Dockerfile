# Inventory Service Dockerfile
# Multi-stage build for optimized production image

# ============================================================================
# Build Stage
# ============================================================================
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    protobuf \
    protobuf-dev

# Set working directory
WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Install protobuf Go plugins
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy source code
COPY . .

# Generate protobuf code
RUN find proto -name "*.proto" -exec protoc \
    --go_out=. \
    --go-grpc_out=. \
    --go_opt=paths=source_relative \
    --go-grpc_opt=paths=source_relative \
    {} \;

# Build the application
# CGO_ENABLED=0 for static binary
# -ldflags for smaller binary size and version info
ARG VERSION=1.0.0
ARG BUILD_TIME
ARG GIT_COMMIT

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -a -installsuffix cgo \
    -o inventory-service \
    ./cmd/main.go

# ============================================================================
# Runtime Stage
# ============================================================================
FROM alpine:3.18 AS runtime

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl

# Create non-root user for security
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set timezone to UTC
ENV TZ=UTC

# Create app directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/inventory-service .

# Change ownership to non-root user
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Expose gRPC port
EXPOSE 50053

# Environment variables with defaults
ENV INVENTORY_SERVICE_PORT=50053
ENV LOG_LEVEL=info
ENV ENVIRONMENT=production
ENV METRICS_ENABLED=true
ENV TRACING_ENABLED=true
ENV SERVICE_NAME=inventory-service
ENV SERVICE_VERSION=1.0.0

# MongoDB Configuration
ENV MONGODB_CONNECTION_URL=mongodb://mongodb:27017
ENV MONGODB_DATABASE_NAME=inventory_db
ENV MONGODB_CONNECT_TIMEOUT=10s
ENV MONGODB_QUERY_TIMEOUT=5s
ENV MONGODB_MAX_POOL_SIZE=100
ENV MONGODB_MIN_POOL_SIZE=10

# Inventory Configuration
ENV INVENTORY_DEFAULT_STOCK_LEVEL=100
ENV INVENTORY_LOW_STOCK_THRESHOLD=10
ENV INVENTORY_MAX_RESERVATION_TIME_MIN=30
ENV INVENTORY_AUTO_RESTOCK_ENABLED=false

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Note: For gRPC health check, we'd need to implement HTTP health endpoint
# or use grpc_health_probe. For now, using a placeholder.

# Run the application
ENTRYPOINT ["./inventory-service"]

# ============================================================================
# Development Stage (optional)
# ============================================================================
FROM golang:1.21-alpine AS development

# Install development tools
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    protobuf \
    protobuf-dev \
    make \
    curl

# Install Go tools
RUN go install github.com/cosmtrek/air@latest && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Set working directory
WORKDIR /app

# Development environment variables
ENV ENVIRONMENT=development
ENV LOG_LEVEL=debug
ENV GO_ENV=development
ENV SEED_TEST_DATA=true

# For development, we'll mount the source code as a volume
# So no COPY needed here

# Expose gRPC port and debugging port
EXPOSE 50053 40000

# Use air for hot reloading in development
CMD ["air"]