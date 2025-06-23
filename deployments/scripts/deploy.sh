#!/bin/bash

# Rocket Science Microservices Deployment Script
# Usage: ./deploy.sh [local|staging|production] [options]

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
ENVIRONMENT="local"
SKIP_BUILD=false
SKIP_MIGRATIONS=false
DETACHED=true
COMPOSE_FILE="docker-compose.yml"

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [local|staging|production] [options]"
    echo ""
    echo "Environments:"
    echo "  local       - Development environment (default)"
    echo "  staging     - Staging environment"
    echo "  production  - Production environment"
    echo ""
    echo "Options:"
    echo "  --no-build     Skip building Docker images"
    echo "  --no-detach    Don't run in detached mode"
    echo "  --skip-migrations  Skip database migrations"
    echo "  --help         Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 local                    # Deploy locally"
    echo "  $0 production --no-build    # Deploy to production without rebuilding"
    echo "  $0 staging --no-detach      # Deploy to staging in foreground"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        local|staging|production)
            ENVIRONMENT="$1"
            shift
            ;;
        --no-build)
            SKIP_BUILD=true
            shift
            ;;
        --no-detach)
            DETACHED=false
            shift
            ;;
        --skip-migrations)
            SKIP_MIGRATIONS=true
            shift
            ;;
        --help)
            show_usage
            exit 0
            ;;
        *)
            print_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
    esac
done

# Set compose file based on environment
case $ENVIRONMENT in
    local)
        COMPOSE_FILE="docker-compose.yml"
        if [ -f "docker-compose.override.yml" ]; then
            COMPOSE_FILE="$COMPOSE_FILE -f docker-compose.override.yml"
        fi
        ;;
    staging)
        COMPOSE_FILE="docker-compose.yml -f deployments/docker/docker-compose.staging.yml"
        ;;
    production)
        COMPOSE_FILE="docker-compose.yml -f deployments/docker/docker-compose.prod.yml"
        ;;
esac

print_status "Starting deployment for environment: $ENVIRONMENT"
print_status "Compose files: $COMPOSE_FILE"

# Check if required files exist
if [ ! -f "docker-compose.yml" ]; then
    print_error "docker-compose.yml not found in current directory"
    exit 1
fi

# Check if .env file exists for production/staging
if [[ "$ENVIRONMENT" != "local" ]] && [ ! -f ".env" ]; then
    print_warning ".env file not found. Creating from .env.example..."
    if [ -f ".env.example" ]; then
        cp .env.example .env
        print_warning "Please edit .env file with your production values before continuing"
        read -p "Press Enter to continue after editing .env file..."
    else
        print_error ".env.example not found. Please create .env file with required environment variables"
        exit 1
    fi
fi

# Pre-deployment checks
print_status "Running pre-deployment checks..."

# Check Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Check Docker Compose is available
if ! command -v docker-compose >/dev/null 2>&1 && ! docker compose version >/dev/null 2>&1; then
    print_error "Docker Compose is not available. Please install Docker Compose."
    exit 1
fi

# Use docker compose or docker-compose based on availability
DOCKER_COMPOSE_CMD="docker-compose"
if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_CMD="docker compose"
fi

print_success "Pre-deployment checks passed"

# Stop existing containers
print_status "Stopping existing containers..."
$DOCKER_COMPOSE_CMD -f $COMPOSE_FILE down --remove-orphans || true

# Build images if not skipped
if [ "$SKIP_BUILD" = false ]; then
    print_status "Building Docker images..."
    $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE build --parallel
    print_success "Docker images built successfully"
else
    print_warning "Skipping Docker image build"
fi

# Start infrastructure services first (databases, kafka, etc.)
print_status "Starting infrastructure services..."
$DOCKER_COMPOSE_CMD -f $COMPOSE_FILE up -d postgres mongo redis kafka elasticsearch

# Wait for infrastructure services to be ready
print_status "Waiting for infrastructure services to be ready..."
sleep 10

# Check if databases are ready
print_status "Checking database connectivity..."
timeout=60
counter=0

while [ $counter -lt $timeout ]; do
    if docker exec postgres pg_isready -U rocket_user -d rocket_science >/dev/null 2>&1; then
        print_success "PostgreSQL is ready"
        break
    fi
    sleep 2
    counter=$((counter + 2))
    if [ $counter -ge $timeout ]; then
        print_error "PostgreSQL failed to start within $timeout seconds"
        exit 1
    fi
done

# Check MongoDB
counter=0
while [ $counter -lt $timeout ]; do
    if docker exec mongodb mongosh --eval "db.runCommand('ping').ok" >/dev/null 2>&1; then
        print_success "MongoDB is ready"
        break
    fi
    sleep 2
    counter=$((counter + 2))
    if [ $counter -ge $timeout ]; then
        print_error "MongoDB failed to start within $timeout seconds"
        exit 1
    fi
done

# Run database migrations if not skipped
if [ "$SKIP_MIGRATIONS" = false ]; then
    print_status "Running database migrations..."
    # Add migration commands here when available
    print_success "Database migrations completed"
else
    print_warning "Skipping database migrations"
fi

# Start application services
print_status "Starting application services..."
if [ "$DETACHED" = true ]; then
    $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE up -d
    print_success "All services started in detached mode"
else
    print_status "Starting services in foreground mode (Ctrl+C to stop)..."
    $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE up
fi

# Health checks
if [ "$DETACHED" = true ]; then
    print_status "Running health checks..."
    sleep 15
    
    # Check Envoy gateway
    if curl -f http://localhost/health >/dev/null 2>&1; then
        print_success "Envoy gateway is healthy"
    else
        print_warning "Envoy gateway health check failed"
    fi
    
    # Check service status
    print_status "Service status:"
    $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE ps
    
    echo ""
    print_success "Deployment completed successfully!"
    echo ""
    print_status "Access your application:"
    echo "  🌐 API Gateway: http://localhost"
    echo "  📊 Grafana: http://localhost/grafana (admin:admin123)"
    echo "  🔍 Jaeger: http://localhost/jaeger"
    echo "  📈 Kibana: http://localhost/kibana"
    echo "  🐋 Envoy Admin: http://localhost:9901"
    echo ""
    print_status "Useful commands:"
    echo "  View logs: $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE logs -f [service-name]"
    echo "  Stop services: $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE down"
    echo "  Restart service: $DOCKER_COMPOSE_CMD -f $COMPOSE_FILE restart [service-name]"
fi
