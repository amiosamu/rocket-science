# Rocket Science Microservices - Makefile
# Provides convenient commands for building, deploying, and managing the system

# Variables
DOCKER_COMPOSE = docker-compose
DOCKER_COMPOSE_CMD = docker compose
PROJECT_NAME = rocket-science
REGISTRY ?= 
TAG ?= latest
ENVIRONMENT ?= local

# Colors
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[1;33m
RED := \033[0;31m
NC := \033[0m # No Color

# Check if docker compose or docker-compose is available
ifeq ($(shell docker compose version > /dev/null 2>&1 && echo ok),ok)
    DOCKER_COMPOSE_CMD = docker compose
else
    DOCKER_COMPOSE_CMD = docker-compose
endif

# Default target
.DEFAULT_GOAL := help

# =================================
# HELP
# =================================

.PHONY: help
help: ## Show this help message
	@echo "$(BLUE)Rocket Science Microservices$(NC)"
	@echo "$(BLUE)=============================$(NC)"
	@echo ""
	@echo "$(GREEN)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "$(GREEN)Examples:$(NC)"
	@echo "  make deploy-local          # Deploy locally for development"
	@echo "  make deploy-prod           # Deploy to production"
	@echo "  make build-all             # Build all Docker images"
	@echo "  make logs SERVICE=order-service  # View logs for specific service"

# =================================
# ENVIRONMENT SETUP
# =================================

.PHONY: setup
setup: ## Setup development environment
	@echo "$(BLUE)[INFO]$(NC) Setting up development environment..."
	@if [ ! -f .env ]; then \
		cp .env.example .env; \
		echo "$(GREEN)[SUCCESS]$(NC) Created .env file from .env.example"; \
		echo "$(YELLOW)[WARNING]$(NC) Please edit .env file with your configuration"; \
	else \
		echo "$(YELLOW)[WARNING]$(NC) .env file already exists"; \
	fi
	@chmod +x deployments/scripts/*.sh
	@chmod +x scripts/*.sh
	@echo "$(GREEN)[SUCCESS]$(NC) Development environment setup complete"

.PHONY: check-env
check-env: ## Check if required environment variables are set
	@echo "$(BLUE)[INFO]$(NC) Checking environment configuration..."
	@if [ ! -f .env ]; then \
		echo "$(RED)[ERROR]$(NC) .env file not found. Run 'make setup' first"; \
		exit 1; \
	fi
	@echo "$(GREEN)[SUCCESS]$(NC) Environment configuration check passed"

# =================================
# BUILD TARGETS
# =================================

.PHONY: build-all
build-all: ## Build all Docker images
	@echo "$(BLUE)[INFO]$(NC) Building all Docker images..."
	@./deployments/scripts/build.sh

.PHONY: build-registry
build-registry: ## Build and tag images for registry
	@echo "$(BLUE)[INFO]$(NC) Building images for registry: $(REGISTRY)"
	@./deployments/scripts/build.sh --registry $(REGISTRY) --tag $(TAG)

.PHONY: build-push
build-push: ## Build and push images to registry
	@echo "$(BLUE)[INFO]$(NC) Building and pushing images to registry: $(REGISTRY)"
	@./deployments/scripts/build.sh --registry $(REGISTRY) --tag $(TAG) --push

.PHONY: build-service
build-service: ## Build specific service (usage: make build-service SERVICE=order-service)
ifndef SERVICE
	@echo "$(RED)[ERROR]$(NC) SERVICE parameter is required. Usage: make build-service SERVICE=order-service"
	@exit 1
endif
	@echo "$(BLUE)[INFO]$(NC) Building $(SERVICE)..."
	@cd services/$(SERVICE) && docker build -t $(SERVICE):$(TAG) .

# =================================
# DEPLOYMENT TARGETS
# =================================

.PHONY: deploy-local
deploy-local: ## Deploy to local development environment
	@echo "$(BLUE)[INFO]$(NC) Deploying to local environment..."
	@./deployments/scripts/deploy.sh local

.PHONY: deploy-staging
deploy-staging: check-env ## Deploy to staging environment
	@echo "$(BLUE)[INFO]$(NC) Deploying to staging environment..."
	@./deployments/scripts/deploy.sh staging

.PHONY: deploy-prod
deploy-prod: check-env ## Deploy to production environment
	@echo "$(BLUE)[INFO]$(NC) Deploying to production environment..."
	@./deployments/scripts/deploy.sh production

.PHONY: deploy-no-build
deploy-no-build: ## Deploy without building images
	@echo "$(BLUE)[INFO]$(NC) Deploying $(ENVIRONMENT) without building..."
	@./deployments/scripts/deploy.sh $(ENVIRONMENT) --no-build

# =================================
# SERVICE MANAGEMENT
# =================================

.PHONY: up
up: ## Start all services
	@echo "$(BLUE)[INFO]$(NC) Starting all services..."
	@$(DOCKER_COMPOSE_CMD) up -d

.PHONY: down
down: ## Stop all services
	@echo "$(BLUE)[INFO]$(NC) Stopping all services..."
	@$(DOCKER_COMPOSE_CMD) down

.PHONY: restart
restart: ## Restart all services
	@echo "$(BLUE)[INFO]$(NC) Restarting all services..."
	@$(DOCKER_COMPOSE_CMD) restart

.PHONY: restart-service
restart-service: ## Restart specific service (usage: make restart-service SERVICE=order-service)
ifndef SERVICE
	@echo "$(RED)[ERROR]$(NC) SERVICE parameter is required. Usage: make restart-service SERVICE=order-service"
	@exit 1
endif
	@echo "$(BLUE)[INFO]$(NC) Restarting $(SERVICE)..."
	@$(DOCKER_COMPOSE_CMD) restart $(SERVICE)

.PHONY: scale
scale: ## Scale specific service (usage: make scale SERVICE=order-service REPLICAS=3)
ifndef SERVICE
	@echo "$(RED)[ERROR]$(NC) SERVICE parameter is required. Usage: make scale SERVICE=order-service REPLICAS=3"
	@exit 1
endif
ifndef REPLICAS
	@echo "$(RED)[ERROR]$(NC) REPLICAS parameter is required. Usage: make scale SERVICE=order-service REPLICAS=3"
	@exit 1
endif
	@echo "$(BLUE)[INFO]$(NC) Scaling $(SERVICE) to $(REPLICAS) replicas..."
	@$(DOCKER_COMPOSE_CMD) up -d --scale $(SERVICE)=$(REPLICAS) $(SERVICE)

# =================================
# MONITORING & DEBUGGING
# =================================

.PHONY: status
status: ## Show status of all services
	@echo "$(BLUE)[INFO]$(NC) Service status:"
	@$(DOCKER_COMPOSE_CMD) ps

.PHONY: logs
logs: ## Show logs for all services or specific service (usage: make logs SERVICE=order-service)
ifdef SERVICE
	@echo "$(BLUE)[INFO]$(NC) Showing logs for $(SERVICE)..."
	@$(DOCKER_COMPOSE_CMD) logs -f $(SERVICE)
else
	@echo "$(BLUE)[INFO]$(NC) Showing logs for all services..."
	@$(DOCKER_COMPOSE_CMD) logs -f
endif

.PHONY: health
health: ## Check health of all services
	@echo "$(BLUE)[INFO]$(NC) Checking service health..."
	@echo "API Gateway Health:"
	@curl -f http://localhost/health 2>/dev/null && echo "$(GREEN)✓ Healthy$(NC)" || echo "$(RED)✗ Unhealthy$(NC)"
	@echo ""
	@echo "Service Status:"
	@$(DOCKER_COMPOSE_CMD) ps --format "table {{.Name}}\t{{.State}}\t{{.Status}}"

.PHONY: shell
shell: ## Open shell in specific service (usage: make shell SERVICE=order-service)
ifndef SERVICE
	@echo "$(RED)[ERROR]$(NC) SERVICE parameter is required. Usage: make shell SERVICE=order-service"
	@exit 1
endif
	@echo "$(BLUE)[INFO]$(NC) Opening shell in $(SERVICE)..."
	@$(DOCKER_COMPOSE_CMD) exec $(SERVICE) /bin/sh

# =================================
# DATABASE OPERATIONS
# =================================

.PHONY: db-connect
db-connect: ## Connect to PostgreSQL database
	@echo "$(BLUE)[INFO]$(NC) Connecting to PostgreSQL database..."
	@$(DOCKER_COMPOSE_CMD) exec postgres psql -U rocket_user -d rocket_science

.PHONY: mongo-connect
mongo-connect: ## Connect to MongoDB
	@echo "$(BLUE)[INFO]$(NC) Connecting to MongoDB..."
	@$(DOCKER_COMPOSE_CMD) exec mongo mongosh -u rocket_user -p rocket_password

.PHONY: redis-connect
redis-connect: ## Connect to Redis
	@echo "$(BLUE)[INFO]$(NC) Connecting to Redis..."
	@$(DOCKER_COMPOSE_CMD) exec redis redis-cli

# =================================
# TESTING
# =================================

.PHONY: test
test: ## Run all tests
	@echo "$(BLUE)[INFO]$(NC) Running tests..."
	@./scripts/test-integration.sh

.PHONY: test-api
test-api: ## Test API endpoints
	@echo "$(BLUE)[INFO]$(NC) Testing API endpoints..."
	@curl -f http://localhost/health || echo "$(RED)[ERROR]$(NC) API Gateway is not responding"
	@echo ""

# =================================
# CLEANUP
# =================================

.PHONY: clean
clean: ## Remove all containers and volumes
	@echo "$(BLUE)[INFO]$(NC) Cleaning up containers and volumes..."
	@$(DOCKER_COMPOSE_CMD) down -v --remove-orphans
	@docker system prune -f

.PHONY: clean-images
clean-images: ## Remove all project Docker images
	@echo "$(BLUE)[INFO]$(NC) Removing project Docker images..."
	@docker images | grep -E "(iam-service|order-service|payment-service|inventory-service|assembly-service|notification-service|envoy)" | awk '{print $$3}' | xargs -r docker rmi -f

.PHONY: clean-all
clean-all: clean clean-images ## Complete cleanup (containers, volumes, and images)
	@echo "$(GREEN)[SUCCESS]$(NC) Complete cleanup finished"

# =================================
# DEVELOPMENT UTILITIES
# =================================

.PHONY: dev-setup
dev-setup: setup build-all deploy-local ## Complete development setup
	@echo "$(GREEN)[SUCCESS]$(NC) Development environment is ready!"
	@echo ""
	@echo "$(BLUE)Access your application:$(NC)"
	@echo "  🌐 API Gateway: http://localhost"
	@echo "  📊 Grafana: http://localhost/grafana"
	@echo "  🔍 Jaeger: http://localhost/jaeger"
	@echo "  📈 Kibana: http://localhost/kibana"

.PHONY: quick-start
quick-start: ## Quick start for development (without building)
	@echo "$(BLUE)[INFO]$(NC) Quick starting development environment..."
	@$(DOCKER_COMPOSE_CMD) up -d

.PHONY: format
format: ## Format Go code in all services
	@echo "$(BLUE)[INFO]$(NC) Formatting Go code..."
	@find services -name "*.go" -not -path "*/vendor/*" -not -path "*/.git/*" | xargs gofmt -w
	@echo "$(GREEN)[SUCCESS]$(NC) Code formatting complete"

.PHONY: proto-gen
proto-gen: ## Generate protobuf files
	@echo "$(BLUE)[INFO]$(NC) Generating protobuf files..."
	@./tools/proto-gen/generate.sh
	@echo "$(GREEN)[SUCCESS]$(NC) Protobuf generation complete"
