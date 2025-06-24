# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy shared module first (for replace directive)
COPY shared/ ./shared

# Copy notification service to match the replace directive path ../../shared
COPY services/notification-service/ ./services/notification-service

# Copy IAM service for replace directive
COPY services/iam-service/ ./services/iam-service

# Set working directory to service
WORKDIR /app/services/notification-service

# Download dependencies (go.mod and go.sum are already copied with the service)
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o notification-service ./cmd/main.go

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create app user
RUN addgroup -g 1001 app && \
    adduser -D -u 1001 -G app app

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/services/notification-service/notification-service .

# Change ownership
RUN chown -R app:app /app

# Switch to app user
USER app

# Expose port (if needed for health checks)
EXPOSE 8081

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD /app/notification-service -health || exit 1

# Run the application
CMD ["./notification-service"]
