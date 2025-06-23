# Assembly Service Makefile

.PHONY: help build run dev stop clean logs test docker-build docker-run

# Variables
SERVICE_NAME = assembly-service
VERSION ?= dev
BUILD_TIME = $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Default target
help:
	@echo "Available commands:"
	@echo "  build      - Build the Go binary"
	@echo "  run        - Run the service locally"
	@echo "  dev        - Start development environment with Docker Compose"
	@echo "  prod       - Start production environment with Docker Compose"
	@echo "  stop       - Stop all Docker containers"
	@echo "  clean      - Clean up Docker containers and volumes"
	@echo "  logs       - Show service logs"
	@echo "  test       - Run tests"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-run - Run single Docker container"

# Go build
build:
	@echo "Building $(SERVICE_NAME)..."
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)" -o $(SERVICE_NAME) ./cmd/main.go

# Run locally
run: build
	@echo "Running $(SERVICE_NAME) locally..."
	./$(SERVICE_NAME)

# Development environment
dev:
	@echo "Starting development environment..."
	@cp .env.example .env 2>/dev/null || true
	docker-compose -f docker-compose.dev.yml up --build -d
	@echo "Development environment started!"
	@echo "Services available at:"
	@echo "  - Assembly Service: http://localhost:8083"
	@echo "  - Kafka UI: http://localhost:8080"

# Production environment
prod:
	@echo "Starting production environment..."
	@cp .env.example .env 2>/dev/null || true
	docker-compose up --build -d
	@echo "Production environment started!"

# Stop all services
stop:
	@echo "Stopping services..."
	docker-compose -f docker-compose.dev.yml down 2>/dev/null || true
	docker-compose down 2>/dev/null || true

# Clean up
clean: stop
	@echo "Cleaning up Docker resources..."
	docker-compose -f docker-compose.dev.yml down -v --remove-orphans 2>/dev/null || true
	docker-compose down -v --remove-orphans 2>/dev/null || true
	docker system prune -f
	@echo "Cleanup completed!"

# Show logs
logs:
	@echo "Showing $(SERVICE_NAME) logs..."
	docker-compose -f docker-compose.dev.yml logs -f assembly-service 2>/dev/null || docker-compose logs -f assembly-service

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(SERVICE_NAME):$(VERSION) \
		-t $(SERVICE_NAME):latest \
		-f Dockerfile \
		../../

# Run single container
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm -it \
		-p 8083:8083 \
		-e LOG_LEVEL=debug \
		-e KAFKA_BROKERS=host.docker.internal:9092 \
		$(SERVICE_NAME):latest

# Create directories
setup:
	@mkdir -p logs monitoring/grafana/dashboards monitoring/grafana/datasources
	@echo "Directory structure created!"

# Health check
health:
	@echo "Checking service health..."
	@curl -f http://localhost:8083/health 2>/dev/null && echo "✅ Service is healthy" || echo "❌ Service is not responding"

# Send test payment event (requires kafka running)
test-payment:
	@echo "Sending test payment event..."
	@docker exec assembly-kafka-dev kafka-console-producer --bootstrap-server localhost:9092 --topic payment.processed << EOF
	{
		"id": "test-payment-$(shell date +%s)",
		"type": "payment.processed",
		"source": "payment-service", 
		"subject": "order-123",
		"timestamp": "$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")",
		"data": {
			"order_id": "order-123",
			"user_id": "user-456",
			"payment_id": "payment-789",
			"amount": {"amount": 50000, "currency": "USD"},
			"status": "PAYMENT_STATUS_COMPLETED"
		}
	}
	EOF

# Monitor logs in real time
monitor:
	@echo "Monitoring all service logs..."
	docker-compose -f docker-compose.dev.yml logs -f 