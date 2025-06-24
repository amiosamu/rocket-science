# Build stage
FROM golang:1.23.2-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy shared module first (for replace directive)  
COPY shared/ ./shared

# Copy IAM service to match the replace directive path ../../shared
COPY services/iam-service/ ./services/iam-service

# Set working directory to service
WORKDIR /build/services/iam-service

# Download dependencies (go.mod and go.sum are already copied with the service)
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o iam-service \
    ./cmd/main.go

# Production stage
FROM alpine:3.18

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /build/services/iam-service/iam-service .

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set ownership
RUN chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ./iam-service --health-check || exit 1

# Expose port
EXPOSE 8080

# Set environment variables
ENV GIN_MODE=release
ENV LOG_LEVEL=info
ENV TZ=UTC

# Run the application
ENTRYPOINT ["./iam-service"]
