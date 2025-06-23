#!/bin/bash

# Rocket Science Microservices Build Script
# Builds all Docker images for the microservices

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
REGISTRY=""
TAG="latest"
PARALLEL_BUILD=true
PUSH_IMAGES=false

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
    echo "Usage: $0 [options]"
    echo ""
    echo "Options:"
    echo "  --registry REGISTRY   Docker registry to use (e.g., docker.io/myuser)"
    echo "  --tag TAG            Docker image tag (default: latest)"
    echo "  --no-parallel        Build images sequentially"
    echo "  --push               Push images to registry after building"
    echo "  --help               Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0                                    # Build all images locally"
    echo "  $0 --registry myregistry.com/myuser   # Build with registry prefix"
    echo "  $0 --tag v1.0.0 --push              # Build, tag as v1.0.0 and push"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --registry)
            REGISTRY="$2"
            shift 2
            ;;
        --tag)
            TAG="$2"
            shift 2
            ;;
        --no-parallel)
            PARALLEL_BUILD=false
            shift
            ;;
        --push)
            PUSH_IMAGES=true
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

# Set image prefix
IMAGE_PREFIX=""
if [ -n "$REGISTRY" ]; then
    IMAGE_PREFIX="${REGISTRY}/"
fi

# Service definitions
declare -A SERVICES=(
    ["iam-service"]="./services/iam-service"
    ["order-service"]="./services/order-service"
    ["payment-service"]="./services/payment-service"
    ["inventory-service"]="./services/inventory-service"
    ["assembly-service"]="./services/assembly-service"
    ["notification-service"]="./services/notification-service"
    ["envoy"]="./infrastructure/envoy"
)

print_status "Building Rocket Science Microservices"
print_status "Registry: ${REGISTRY:-'local'}"
print_status "Tag: $TAG"
print_status "Parallel build: $PARALLEL_BUILD"

# Check if Docker is running
if ! docker info >/dev/null 2>&1; then
    print_error "Docker is not running. Please start Docker and try again."
    exit 1
fi

# Function to build a single service
build_service() {
    local service_name=$1
    local service_path=$2
    local image_name="${IMAGE_PREFIX}${service_name}:${TAG}"
    
    print_status "Building $service_name..."
    
    if [ ! -f "$service_path/Dockerfile" ]; then
        print_error "Dockerfile not found in $service_path"
        return 1
    fi
    
    if docker build -t "$image_name" "$service_path"; then
        print_success "$service_name built successfully as $image_name"
        
        # Push if requested
        if [ "$PUSH_IMAGES" = true ] && [ -n "$REGISTRY" ]; then
            print_status "Pushing $image_name..."
            if docker push "$image_name"; then
                print_success "$image_name pushed successfully"
            else
                print_error "Failed to push $image_name"
                return 1
            fi
        fi
    else
        print_error "Failed to build $service_name"
        return 1
    fi
}

# Build services
if [ "$PARALLEL_BUILD" = true ]; then
    print_status "Building all services in parallel..."
    
    # Start all builds in background
    pids=()
    for service_name in "${!SERVICES[@]}"; do
        service_path="${SERVICES[$service_name]}"
        build_service "$service_name" "$service_path" &
        pids+=($!)
    done
    
    # Wait for all builds to complete
    failed_builds=()
    for i in "${!pids[@]}"; do
        if ! wait "${pids[$i]}"; then
            failed_builds+=("${!SERVICES[$i]}")
        fi
    done
    
    if [ ${#failed_builds[@]} -eq 0 ]; then
        print_success "All services built successfully!"
    else
        print_error "Failed to build: ${failed_builds[*]}"
        exit 1
    fi
else
    print_status "Building services sequentially..."
    
    failed_builds=()
    for service_name in "${!SERVICES[@]}"; do
        service_path="${SERVICES[$service_name]}"
        if ! build_service "$service_name" "$service_path"; then
            failed_builds+=("$service_name")
        fi
    done
    
    if [ ${#failed_builds[@]} -eq 0 ]; then
        print_success "All services built successfully!"
    else
        print_error "Failed to build: ${failed_builds[*]}"
        exit 1
    fi
fi

# Show final status
echo ""
print_success "Build Summary:"
for service_name in "${!SERVICES[@]}"; do
    image_name="${IMAGE_PREFIX}${service_name}:${TAG}"
    echo "  ✓ $image_name"
done

if [ "$PUSH_IMAGES" = true ] && [ -n "$REGISTRY" ]; then
    echo ""
    print_success "All images pushed to registry: $REGISTRY"
fi

echo ""
print_status "Next steps:"
echo "  1. Deploy locally: ./deployments/scripts/deploy.sh local"
echo "  2. Deploy to production: ./deployments/scripts/deploy.sh production"
echo "  3. View images: docker images | grep rocket"
