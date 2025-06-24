# Build stage
FROM golang:1.23.2-alpine AS builder

# Install required packages
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy shared module first (for replace directive)
COPY shared/ ./shared

# Copy order service and dependencies to match replace directives
COPY services/order-service/ ./services/order-service
COPY services/inventory-service/ ./services/inventory-service  
COPY services/payment-service/ ./services/payment-service

# Set working directory to service
WORKDIR /app/services/order-service

# Download dependencies (go.mod and go.sum are already copied with the service)
RUN go mod download

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /root/

# Copy the binary from builder stage  
COPY --from=builder /app/services/order-service/main .

# Copy any additional files if needed
# COPY --from=builder /app/configs ./configs

# Change ownership to non-root user
RUN chown -R appuser:appgroup /root/

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./main"]