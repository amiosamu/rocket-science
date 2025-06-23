# Order Service Makefile

# Variables
SERVICE_NAME := order-service
VERSION := $(shell git describe --tags --always --dirty)
DOCKER_IMAGE := $(SERVICE_NAME):$(VERSION)
DOCKER_REGISTRY := your-registry.com
GO_VERSION := 1.21

# Build flags
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')"

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
.PHONY: help
help:
	@echo "Available commands:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application binary
.PHONY: build
build:
	@echo "Building $(SERVICE_NAME)..."
	go build $(LDFLAGS) -o bin/$(SERVICE_NAME) ./cmd/main.go

## run: Run the application locally
.PHONY: run
run:
	@echo "Running $(SERVICE_NAME)..."
	go run ./cmd/main.go

## dev: Run with live reload (requires air)
.PHONY: dev
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "air not found. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Falling back to regular run..."; \
		make run; \
	fi

## test: Run all tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race ./...

## test-cover: Run tests with coverage
.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## test-integration: Run integration tests
.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	go test -v -tags=integration ./tests/...

## lint: Run linter
.PHONY: lint
lint:
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not found. Install with:"; \
		echo "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## fmt: Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

## vet: Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

## mod: Download and tidy dependencies
.PHONY: mod
mod:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

## clean: Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean -cache

## docker-build: Build Docker image
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE) .
	docker tag $(DOCKER_IMAGE) $(SERVICE_NAME):latest

## docker-run: Run Docker container
.PHONY: docker-run
docker-run:
	@echo "Running Docker container..."
	docker run --rm -p 8080:8080 \
		-e DB_HOST=host.docker.internal \
		-e KAFKA_BROKERS=host.docker.internal:9092 \
		$(DOCKER_IMAGE)

## docker-push: Push Docker image to registry
.PHONY: docker-push
docker-push:
	@echo "Pushing Docker image to registry..."
	docker tag $(DOCKER_IMAGE) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE)

## proto: Generate protobuf code
.PHONY: proto
proto:
	@echo "Generating protobuf code..."
	cd ../../shared/proto && ./generate.sh

## migrate-up: Run database migrations up
.PHONY: migrate-up
migrate-up:
	@echo "Running database migrations..."
	# This would typically use a migration tool
	# For now, migrations run automatically on service startup

## migrate-down: Run database migrations down
.PHONY: migrate-down
migrate-down:
	@echo "Rolling back database migrations..."
	# This would typically use a migration tool

## deps: Install development dependencies
.PHONY: deps
deps:
	@echo "Installing development dependencies..."
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

## check: Run all checks (fmt, vet, lint, test)
.PHONY: check
check: fmt vet lint test

## ci: Run CI checks
.PHONY: ci
ci: mod check test-cover

## release: Build release version
.PHONY: release
release: clean check
	@echo "Building release version..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -a -installsuffix cgo -o bin/$(SERVICE_NAME)-linux-amd64 ./cmd/main.go
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -a -installsuffix cgo -o bin/$(SERVICE_NAME)-darwin-amd64 ./cmd/main.go
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -a -installsuffix cgo -o bin/$(SERVICE_NAME)-windows-amd64.exe ./cmd/main.go

## benchmark: Run benchmarks
.PHONY: benchmark
benchmark:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## security: Run security checks
.PHONY: security
security:
	@if command -v gosec > /dev/null; then \
		gosec ./...; \
	else \
		echo "gosec not found. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

## docs: Generate documentation
.PHONY: docs
docs:
	@echo "Generating documentation..."
	@if command -v godoc > /dev/null; then \
		echo "Starting godoc server at http://localhost:6060"; \
		godoc -http=:6060; \
	else \
		echo "godoc not found. Install with: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi

## env: Show environment variables template
.PHONY: env
env:
	@echo "Environment variables for $(SERVICE_NAME):"
	@echo ""
	@echo "# Server Configuration"
	@echo "export SERVER_HOST=0.0.0.0"
	@echo "export SERVER_PORT=8080"
	@echo ""
	@echo "# Database Configuration"
	@echo "export DB_HOST=localhost"
	@echo "export DB_PORT=5432"
	@echo "export DB_USER=postgres"
	@echo "export DB_PASSWORD=password"
	@echo "export DB_NAME=orders"
	@echo ""
	@echo "# Kafka Configuration"
	@echo "export KAFKA_BROKERS=localhost:9092"
	@echo "export KAFKA_PAYMENT_EVENTS_TOPIC=payment-events"
	@echo "export KAFKA_ASSEMBLY_EVENTS_TOPIC=assembly-events"
	@echo "export KAFKA_CONSUMER_GROUP=order-service"
	@echo ""
	@echo "# External Services"
	@echo "export INVENTORY_SERVICE_ADDRESS=localhost:9001"
	@echo "export PAYMENT_SERVICE_ADDRESS=localhost:9002"
	@echo ""
	@echo "# Observability"
	@echo "export LOG_LEVEL=info"
	@echo "export METRICS_ENABLED=true"
	@echo "export TRACING_ENABLED=true"
	@echo "export OTEL_ENDPOINT=http://localhost:4317"

## info: Show project information
.PHONY: info
info:
	@echo "Project: $(SERVICE_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "Docker Image: $(DOCKER_IMAGE)"
	@echo "Registry: $(DOCKER_REGISTRY)"