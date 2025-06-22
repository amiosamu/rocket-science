# Payment Service Makefile
# Convenient commands for development and deployment

# Variables
SERVICE_NAME := payment-service
VERSION := 1.0.0
DOCKER_REGISTRY := localhost:5000
DOCKER_IMAGE := $(DOCKER_REGISTRY)/$(SERVICE_NAME):$(VERSION)

# Go variables
GOPATH := $(shell go env GOPATH)
GOBIN := $(GOPATH)/bin

# Build variables
BUILD_TIME := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

.PHONY: help
help: ## Show this help message
	@echo "Payment Service - Available Commands:"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo ""

# ============================================================================
# Development Commands
# ============================================================================

.PHONY: run
run: ## Run the service locally
	@echo "🚀 Running Payment Service locally..."
	go run cmd/main.go

.PHONY: build
build: ## Build the service binary
	@echo "🔨 Building Payment Service..."
	go build -ldflags="-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT)" \
		-o bin/payment-service cmd/main.go

.PHONY: clean
clean: ## Clean build artifacts
	@echo "🧹 Cleaning build artifacts..."
	rm -rf bin/
	go clean -cache
	go clean -modcache

.PHONY: deps
deps: ## Download and tidy dependencies
	@echo "📦 Managing dependencies..."
	go mod download
	go mod tidy

.PHONY: proto
proto: ## Generate protobuf code
	@echo "🔧 Generating protobuf code..."
	@if [ ! -f "$(GOBIN)/protoc-gen-go" ]; then \
		echo "Installing protoc-gen-go..."; \
		go install google.golang.org/protobuf/cmd/protoc-gen-go@latest; \
	fi
	@if [ ! -f "$(GOBIN)/protoc-gen-go-grpc" ]; then \
		echo "Installing protoc-gen-go-grpc..."; \
		go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest; \
	fi
	find proto -name "*.proto" -exec protoc \
		--go_out=. \
		--go-grpc_out=. \
		--go_opt=paths=source_relative \
		--go-grpc_opt=paths=source_relative \
		{} \;

# ============================================================================
# Testing Commands
# ============================================================================

.PHONY: test
test: ## Run unit tests
	@echo "🧪 Running unit tests..."
	go test -v ./...

.PHONY: test-coverage
test-coverage: ## Run tests with coverage
	@echo "🧪 Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "🧪 Running integration tests..."
	go test -v -tags=integration ./...

.PHONY: lint
lint: ## Run linter
	@echo "🔍 Running linter..."
	@if [ ! -f "$(GOBIN)/golangci-lint" ]; then \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	golangci-lint run

# ============================================================================
# Docker Commands
# ============================================================================

.PHONY: docker-build
docker-build: ## Build Docker image
	@echo "🐳 Building Docker image..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh runtime

.PHONY: docker-build-dev
docker-build-dev: ## Build development Docker image
	@echo "🐳 Building development Docker image..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh development

.PHONY: docker-build-all
docker-build-all: ## Build all Docker images
	@echo "🐳 Building all Docker images..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh all

.PHONY: docker-run
docker-run: docker-build ## Build and run Docker container
	@echo "🚀 Running Docker container..."
	docker run --rm -p 50052:50052 \
		-e PAYMENT_SERVICE_PORT=50052 \
		-e LOG_LEVEL=info \
		-e ENVIRONMENT=docker \
		--name $(SERVICE_NAME) \
		$(DOCKER_IMAGE)

.PHONY: docker-test
docker-test: docker-build ## Test Docker image
	@echo "🧪 Testing Docker image..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh runtime

.PHONY: docker-push
docker-push: ## Build and push Docker image
	@echo "📤 Building and pushing Docker image..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh push

.PHONY: docker-clean
docker-clean: ## Clean Docker images and containers
	@echo "🧹 Cleaning Docker artifacts..."
	chmod +x scripts/docker-build.sh
	./scripts/docker-build.sh clean

# ============================================================================
# Docker Compose Commands
# ============================================================================

.PHONY: compose-up
compose-up: ## Start services with docker-compose
	@echo "🚀 Starting services with docker-compose..."
	docker-compose up -d payment-service

.PHONY: compose-up-dev
compose-up-dev: ## Start development services with docker-compose
	@echo "🚀 Starting development services..."
	docker-compose --profile development up -d payment-service-dev

.PHONY: compose-up-tools
compose-up-tools: ## Start with gRPC tools
	@echo "🚀 Starting services with gRPC tools..."
	docker-compose --profile tools up -d

.PHONY: compose-down
compose-down: ## Stop docker-compose services
	@echo "🛑 Stopping docker-compose services..."
	docker-compose down

.PHONY: compose-logs
compose-logs: ## Show docker-compose logs
	docker-compose logs -f payment-service

.PHONY: compose-ps
compose-ps: ## Show docker-compose services status
	docker-compose ps

# ============================================================================
# Utility Commands
# ============================================================================

.PHONY: fmt
fmt: ## Format Go code
	@echo "✨ Formatting Go code..."
	go fmt ./...
	goimports -w .

.PHONY: vet
vet: ## Run go vet
	@echo "🔍 Running go vet..."
	go vet ./...

.PHONY: mod-verify
mod-verify: ## Verify go modules
	@echo "🔍 Verifying go modules..."
	go mod verify

.PHONY: security-scan
security-scan: ## Run security scan
	@echo "🔒 Running security scan..."
	@if [ ! -f "$(GOBIN)/gosec" ]; then \
		echo "Installing gosec..."; \
		go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest; \
	fi
	gosec ./...

.PHONY: benchmark
benchmark: ## Run benchmarks
	@echo "📊 Running benchmarks..."
	go test -bench=. -benchmem ./...

# ============================================================================
# Development Workflow
# ============================================================================

.PHONY: dev-setup
dev-setup: deps proto ## Setup development environment
	@echo "🛠️ Setting up development environment..."
	@echo "✅ Development environment ready!"

.PHONY: dev-check
dev-check: fmt vet lint test ## Run all development checks
	@echo "✅ All development checks passed!"

.PHONY: ci
ci: dev-check test-coverage docker-test ## Run CI pipeline
	@echo "✅ CI pipeline completed successfully!"

.PHONY: release
release: ci docker-push ## Build and release
	@echo "🎉 Release completed successfully!"

# ============================================================================
# Quick Commands
# ============================================================================

.PHONY: quick-start
quick-start: dev-setup run ## Quick start for development

.PHONY: quick-docker
quick-docker: docker-build docker-run ## Quick Docker start

.PHONY: quick-test
quick-test: proto test ## Quick test run

# Default target
.DEFAULT_GOAL := help