# IAM Service Makefile

# Variables
IMAGE_NAME = iam-service
TAG = latest
REGISTRY = localhost:5000
FULL_IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(TAG)

# Colors for output
RED = \033[0;31m
GREEN = \033[0;32m
YELLOW = \033[0;33m
BLUE = \033[0;34m
NC = \033[0m # No Color

.PHONY: help build run stop clean logs test docker-build docker-push docker-pull dev prod

# Default target
help: ## Show this help message
	@echo "$(BLUE)IAM Service Makefile$(NC)"
	@echo "$(YELLOW)Available commands:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(GREEN)%-15s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Local Development
build: ## Build the Go application locally
	@echo "$(BLUE)Building IAM service...$(NC)"
	go build -o iam-service ./cmd/main.go
	@echo "$(GREEN)Build completed!$(NC)"

run: ## Run the application locally
	@echo "$(BLUE)Starting IAM service...$(NC)"
	go run ./cmd/main.go

test: ## Run tests
	@echo "$(BLUE)Running tests...$(NC)"
	go test -v ./...
	@echo "$(GREEN)Tests completed!$(NC)"

test-coverage: ## Run tests with coverage
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)Coverage report generated: coverage.html$(NC)"

lint: ## Run linter
	@echo "$(BLUE)Running linter...$(NC)"
	golangci-lint run
	@echo "$(GREEN)Linting completed!$(NC)"

clean: ## Clean build artifacts
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	rm -f iam-service coverage.out coverage.html
	go clean
	@echo "$(GREEN)Cleanup completed!$(NC)"

# Docker Operations
docker-build: ## Build Docker image
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t $(IMAGE_NAME):$(TAG) .
	@echo "$(GREEN)Docker image built: $(IMAGE_NAME):$(TAG)$(NC)"

docker-run: ## Run Docker container
	@echo "$(BLUE)Running Docker container...$(NC)"
	docker run -d --name $(IMAGE_NAME) -p 8080:8080 $(IMAGE_NAME):$(TAG)
	@echo "$(GREEN)Container started: $(IMAGE_NAME)$(NC)"

docker-stop: ## Stop Docker container
	@echo "$(BLUE)Stopping Docker container...$(NC)"
	docker stop $(IMAGE_NAME) || true
	docker rm $(IMAGE_NAME) || true
	@echo "$(GREEN)Container stopped and removed$(NC)"

docker-logs: ## Show Docker container logs
	docker logs -f $(IMAGE_NAME)

docker-push: docker-build ## Push Docker image to registry
	@echo "$(BLUE)Pushing Docker image to registry...$(NC)"
	docker tag $(IMAGE_NAME):$(TAG) $(FULL_IMAGE)
	docker push $(FULL_IMAGE)
	@echo "$(GREEN)Image pushed: $(FULL_IMAGE)$(NC)"

docker-pull: ## Pull Docker image from registry
	@echo "$(BLUE)Pulling Docker image from registry...$(NC)"
	docker pull $(FULL_IMAGE)
	docker tag $(FULL_IMAGE) $(IMAGE_NAME):$(TAG)
	@echo "$(GREEN)Image pulled: $(FULL_IMAGE)$(NC)"

# Docker Compose Operations
up: ## Start all services with docker-compose
	@echo "$(BLUE)Starting all services...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)All services started!$(NC)"

down: ## Stop all services with docker-compose
	@echo "$(BLUE)Stopping all services...$(NC)"
	docker-compose down
	@echo "$(GREEN)All services stopped!$(NC)"

dev: ## Start development environment
	@echo "$(BLUE)Starting development environment...$(NC)"
	docker-compose -f docker-compose.yml up -d postgres redis
	@echo "$(GREEN)Development environment ready!$(NC)"
	@echo "$(YELLOW)Run 'make run' to start the IAM service locally$(NC)"

logs: ## Show docker-compose logs
	docker-compose logs -f

ps: ## Show running containers
	docker-compose ps

restart: ## Restart all services
	@echo "$(BLUE)Restarting all services...$(NC)"
	docker-compose restart
	@echo "$(GREEN)All services restarted!$(NC)"

# Database Operations
db-migrate: ## Run database migrations
	@echo "$(BLUE)Running database migrations...$(NC)"
	go run tools/migration/migrate.go up
	@echo "$(GREEN)Migrations completed!$(NC)"

db-rollback: ## Rollback database migrations
	@echo "$(BLUE)Rolling back database migrations...$(NC)"
	go run tools/migration/migrate.go down
	@echo "$(GREEN)Rollback completed!$(NC)"

db-reset: ## Reset database (drop and recreate)
	@echo "$(YELLOW)Resetting database...$(NC)"
	docker-compose exec postgres psql -U iam_user -d postgres -c "DROP DATABASE IF EXISTS iam_db;"
	docker-compose exec postgres psql -U iam_user -d postgres -c "CREATE DATABASE iam_db;"
	$(MAKE) db-migrate
	@echo "$(GREEN)Database reset completed!$(NC)"

# Production Operations
prod: ## Deploy to production environment
	@echo "$(BLUE)Deploying to production...$(NC)"
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
	@echo "$(GREEN)Production deployment completed!$(NC)"

health: ## Check service health
	@echo "$(BLUE)Checking service health...$(NC)"
	curl -f http://localhost:8080/health || echo "$(RED)Service is not healthy$(NC)"

# Utility Operations
install-deps: ## Install Go dependencies
	@echo "$(BLUE)Installing Go dependencies...$(NC)"
	go mod download
	go mod tidy
	@echo "$(GREEN)Dependencies installed!$(NC)"

update-deps: ## Update Go dependencies
	@echo "$(BLUE)Updating Go dependencies...$(NC)"
	go get -u ./...
	go mod tidy
	@echo "$(GREEN)Dependencies updated!$(NC)"

proto-gen: ## Generate protobuf files
	@echo "$(BLUE)Generating protobuf files...$(NC)"
	protoc --go_out=. --go-grpc_out=. proto/iam/iam.proto
	@echo "$(GREEN)Protobuf files generated!$(NC)"

format: ## Format Go code
	@echo "$(BLUE)Formatting Go code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)Code formatted!$(NC)"

# Monitoring
monitoring-up: ## Start monitoring stack
	@echo "$(BLUE)Starting monitoring stack...$(NC)"
	docker-compose up -d prometheus grafana
	@echo "$(GREEN)Monitoring stack started!$(NC)"
	@echo "$(YELLOW)Prometheus: http://localhost:9090$(NC)"
	@echo "$(YELLOW)Grafana: http://localhost:3000 (admin/admin123)$(NC)"

monitoring-down: ## Stop monitoring stack
	@echo "$(BLUE)Stopping monitoring stack...$(NC)"
	docker-compose stop prometheus grafana
	@echo "$(GREEN)Monitoring stack stopped!$(NC)"

# Admin Tools
admin-up: ## Start admin tools (pgAdmin, RedisInsight)
	@echo "$(BLUE)Starting admin tools...$(NC)"
	docker-compose up -d pgadmin redis-insight
	@echo "$(GREEN)Admin tools started!$(NC)"
	@echo "$(YELLOW)pgAdmin: http://localhost:8081 (admin@iam.local/admin123)$(NC)"
	@echo "$(YELLOW)RedisInsight: http://localhost:8082$(NC)"

admin-down: ## Stop admin tools
	@echo "$(BLUE)Stopping admin tools...$(NC)"
	docker-compose stop pgadmin redis-insight
	@echo "$(GREEN)Admin tools stopped!$(NC)"

# Complete Operations
full-up: ## Start complete stack (all services)
	@echo "$(BLUE)Starting complete stack...$(NC)"
	docker-compose up -d
	@echo "$(GREEN)Complete stack started!$(NC)"
	@echo ""
	@echo "$(YELLOW)Services:$(NC)"
	@echo "  IAM Service: http://localhost:8080"
	@echo "  pgAdmin: http://localhost:8081 (admin@iam.local/admin123)"
	@echo "  RedisInsight: http://localhost:8082"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  Grafana: http://localhost:3000 (admin/admin123)"

full-down: ## Stop complete stack
	@echo "$(BLUE)Stopping complete stack...$(NC)"
	docker-compose down
	@echo "$(GREEN)Complete stack stopped!$(NC)"

# Cleanup Operations
clean-docker: ## Clean Docker artifacts
	@echo "$(BLUE)Cleaning Docker artifacts...$(NC)"
	docker system prune -f
	docker volume prune -f
	@echo "$(GREEN)Docker cleanup completed!$(NC)"

clean-all: clean clean-docker ## Clean everything
	@echo "$(GREEN)Complete cleanup finished!$(NC)" 