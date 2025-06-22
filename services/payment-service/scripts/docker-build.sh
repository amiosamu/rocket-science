#!/bin/bash

# Payment Service Docker Build Script
# Builds Docker images for different environments

set -e  # Exit on any error

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Service configuration
SERVICE_NAME="payment-service"
DOCKERFILE="Dockerfile"
REGISTRY="${DOCKER_REGISTRY:-localhost:5000}"

# Build information
VERSION="${VERSION:-1.0.0}"
BUILD_TIME=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

# Default target
TARGET="${1:-runtime}"

echo -e "${BLUE}🚀 Building Payment Service Docker Image${NC}"
echo -e "${BLUE}====================================${NC}"
echo "Service: $SERVICE_NAME"
echo "Target: $TARGET"
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME"
echo "Git Commit: $GIT_COMMIT"
echo "Git Branch: $GIT_BRANCH"
echo ""

# Function to build image
build_image() {
    local target=$1
    local tag_suffix=$2
    local image_tag="${REGISTRY}/${SERVICE_NAME}:${VERSION}${tag_suffix}"
    local latest_tag="${REGISTRY}/${SERVICE_NAME}:latest${tag_suffix}"

    echo -e "${YELLOW}Building ${target} image...${NC}"
    
    docker build \
        --target $target \
        --build-arg VERSION=$VERSION \
        --build-arg BUILD_TIME=$BUILD_TIME \
        --build-arg GIT_COMMIT=$GIT_COMMIT \
        --tag $image_tag \
        --tag $latest_tag \
        --file $DOCKERFILE \
        .

    echo -e "${GREEN}✅ Built: $image_tag${NC}"
    echo -e "${GREEN}✅ Tagged: $latest_tag${NC}"
}

# Function to show image info
show_image_info() {
    local image_tag="${REGISTRY}/${SERVICE_NAME}:${VERSION}"
    
    echo -e "${BLUE}📊 Image Information:${NC}"
    docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedAt}}" | grep $SERVICE_NAME || true
    echo ""
}

# Function to test image
test_image() {
    local image_tag="${REGISTRY}/${SERVICE_NAME}:${VERSION}"
    
    echo -e "${YELLOW}🧪 Testing image...${NC}"
    
    # Test that the image can start
    docker run --rm --name ${SERVICE_NAME}-test -d \
        -p 50052:50052 \
        -e PAYMENT_SERVICE_PORT=50052 \
        -e LOG_LEVEL=info \
        $image_tag

    # Wait a moment for startup
    sleep 5

    # Check if container is running
    if docker ps | grep -q ${SERVICE_NAME}-test; then
        echo -e "${GREEN}✅ Container started successfully${NC}"
        
        # Stop test container
        docker stop ${SERVICE_NAME}-test
        echo -e "${GREEN}✅ Container stopped cleanly${NC}"
    else
        echo -e "${RED}❌ Container failed to start${NC}"
        docker logs ${SERVICE_NAME}-test
        exit 1
    fi
}

# Function to push image
push_image() {
    local tag_suffix=$1
    local image_tag="${REGISTRY}/${SERVICE_NAME}:${VERSION}${tag_suffix}"
    local latest_tag="${REGISTRY}/${SERVICE_NAME}:latest${tag_suffix}"

    echo -e "${YELLOW}📤 Pushing images to registry...${NC}"
    
    docker push $image_tag
    docker push $latest_tag
    
    echo -e "${GREEN}✅ Pushed: $image_tag${NC}"
    echo -e "${GREEN}✅ Pushed: $latest_tag${NC}"
}

# Function to clean up old images
cleanup() {
    echo -e "${YELLOW}🧹 Cleaning up old images...${NC}"
    
    # Remove dangling images
    docker image prune -f
    
    # Remove old versions (keep last 3)
    docker images --format "{{.Repository}}:{{.Tag}}" | grep $SERVICE_NAME | tail -n +4 | xargs docker rmi 2>/dev/null || true
    
    echo -e "${GREEN}✅ Cleanup completed${NC}"
}

# Main build logic
case $TARGET in
    "runtime"|"production")
        build_image "runtime" ""
        show_image_info
        test_image
        ;;
    
    "development"|"dev")
        build_image "development" "-dev"
        show_image_info
        ;;
    
    "all")
        build_image "runtime" ""
        build_image "development" "-dev"
        show_image_info
        test_image
        ;;
    
    "push")
        if [[ -z "$DOCKER_REGISTRY" ]]; then
            echo -e "${RED}❌ DOCKER_REGISTRY environment variable is required for push${NC}"
            exit 1
        fi
        
        build_image "runtime" ""
        test_image
        push_image ""
        ;;
    
    "push-dev")
        if [[ -z "$DOCKER_REGISTRY" ]]; then
            echo -e "${RED}❌ DOCKER_REGISTRY environment variable is required for push${NC}"
            exit 1
        fi
        
        build_image "development" "-dev"
        push_image "-dev"
        ;;
    
    "clean")
        cleanup
        exit 0
        ;;
    
    "help"|"-h"|"--help")
        echo "Usage: $0 [TARGET]"
        echo ""
        echo "Targets:"
        echo "  runtime      Build production runtime image (default)"
        echo "  development  Build development image with hot reload"
        echo "  all          Build both runtime and development images"
        echo "  push         Build, test, and push runtime image"
        echo "  push-dev     Build and push development image"
        echo "  clean        Clean up old images"
        echo "  help         Show this help message"
        echo ""
        echo "Environment Variables:"
        echo "  VERSION           Image version (default: 1.0.0)"
        echo "  DOCKER_REGISTRY   Docker registry URL (default: localhost:5000)"
        echo ""
        echo "Examples:"
        echo "  $0 runtime"
        echo "  VERSION=2.0.0 $0 all"
        echo "  DOCKER_REGISTRY=myregistry.com $0 push"
        exit 0
        ;;
    
    *)
        echo -e "${RED}❌ Unknown target: $TARGET${NC}"
        echo "Run '$0 help' for usage information"
        exit 1
        ;;
esac

echo ""
echo -e "${GREEN}🎉 Build completed successfully!${NC}"
echo ""
echo -e "${BLUE}Quick Start:${NC}"
echo "  Run:     docker run -p 50052:50052 ${REGISTRY}/${SERVICE_NAME}:${VERSION}"
echo "  Compose: docker-compose up payment-service"
echo "  Test:    grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check"