# Build stage
FROM golang:1.23.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata make

# Set working directory
WORKDIR /workspace

# Copy go.work files for workspace setup
COPY ../../go.work ../../go.work.sum ./

# Copy shared module first (for better caching)
COPY ../../shared ./shared

# Copy assembly service
COPY . ./services/assembly-service

# Set working directory to assembly service
WORKDIR /workspace/services/assembly-service

# Download dependencies
RUN go mod download

# Build the binary with version info
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -o assembly-service \
    ./cmd/main.go

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata curl && \
    addgroup -g 1001 assembly && \
    adduser -u 1001 -G assembly -D -s /bin/sh assembly

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /workspace/services/assembly-service/assembly-service .

# Change ownership
RUN chown -R assembly:assembly /app

# Switch to non-root user
USER assembly

# Expose port for health checks
EXPOSE 8083

# Health check using curl (more reliable than ps)
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:8083/health 2>/dev/null || ps aux | grep -v grep | grep assembly-service || exit 1

# Set default environment variables
ENV LOG_LEVEL=info \
    LOG_FORMAT=json \
    ENVIRONMENT=production \
    KAFKA_BROKERS=kafka:9092 \
    ASSEMBLY_SIMULATION_DURATION=10s \
    ASSEMBLY_MAX_CONCURRENT=10 \
    ASSEMBLY_FAILURE_RATE=0.05

# Metadata labels
LABEL org.opencontainers.image.title="Assembly Service" \
      org.opencontainers.image.description="Rocket assembly microservice for spare parts assembly simulation" \
      org.opencontainers.image.vendor="Rocket Science Co." \
      org.opencontainers.image.source="https://github.com/amiosamu/rocket-science"

# Run the binary
ENTRYPOINT ["./assembly-service"]
