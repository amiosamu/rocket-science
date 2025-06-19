#!/bin/bash

# Rocket Science Microservices System Startup Script

set -e

echo "🚀 Starting Rocket Science Microservices System..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Create .env if it doesn't exist
if [ ! -f .env ]; then
    echo -e "${YELLOW}Creating .env file from template...${NC}"
    cp .env.example .env
    echo -e "${YELLOW}Please update the .env file with your configuration before continuing.${NC}"
    echo -e "${RED}Press Enter to continue after updating .env file...${NC}"
    read
fi

# Create necessary directories
echo -e "${BLUE}Creating necessary directories...${NC}"
mkdir -p infrastructure/monitoring/elasticsearch
mkdir -p logs/envoy
mkdir -p logs/services

# Set permissions
chmod +x infrastructure/envoy/lua/auth_check.lua

# Pull images first to avoid timeout issues
echo -e "${BLUE}Pulling Docker images...${NC}"
docker-compose pull

# Build custom images
echo -e "${BLUE}Building custom images...${NC}"
docker-compose build

# Start infrastructure first (databases, kafka, etc.)
echo -e "${BLUE}Starting infrastructure services...${NC}"
docker-compose up -d postgres mongo redis kafka

# Wait for infrastructure to be ready
echo -e "${YELLOW}Waiting for infrastructure to be ready...${NC}"
sleep 30

# Check if Kafka is ready
echo -e "${BLUE}Checking if Kafka is ready...${NC}"
timeout 60 bash -c "until docker exec kafka kafka-topics --bootstrap-server localhost:9092 --list >/dev/null 2>&1; do sleep 2; done"

# Start monitoring services
echo -e "${BLUE}Starting monitoring services...${NC}"
docker-compose up -d elasticsearch prometheus jaeger otel-collector

# Wait for monitoring to be ready
sleep 20

# Start monitoring dashboards
docker-compose up -d grafana kibana

# Start core microservices
echo -e "${BLUE}Starting core microservices...${NC}"
docker-compose up -d iam-service payment-service inventory-service

# Wait for services to be ready
sleep 15

# Start order service and consumers
docker-compose up -d order-service assembly-service notification-service

# Finally start Envoy gateway
echo -e "${BLUE}Starting Envoy gateway...${NC}"
docker-compose up -d envoy

# Show system status
echo ""
echo -e "${GREEN}🎉 System startup complete!${NC}"
echo ""
echo -e "${BLUE}System Access Points:${NC}"
echo -e "  🌐 Main Gateway: http://localhost"
echo -e "  📊 Grafana: http://localhost/grafana (admin/admin123)"
echo -e "  🔍 Jaeger: http://localhost/jaeger"
echo -e "  📋 Kibana: http://localhost/kibana"
echo -e "  📈 Prometheus: http://localhost/prometheus"
echo -e "  🔧 Envoy Admin: http://localhost:9901"
echo ""
echo -e "${BLUE}API Endpoints:${NC}"
echo -e "  📦 Orders API: http://localhost/api/orders"
echo -e "  🔑 IAM gRPC: http://localhost/iam.IAMService/"
echo -e "  💳 Payment gRPC: http://localhost/payment.PaymentService/"
echo -e "  📋 Inventory gRPC: http://localhost/inventory.InventoryService/"
echo ""
echo -e "${YELLOW}Check service status with:${NC} docker-compose ps"
echo -e "${YELLOW}View logs with:${NC} docker-compose logs -f [service-name]"
echo -e "${YELLOW}Stop system with:${NC} docker-compose down"
echo ""
echo -e "${GREEN}Happy coding! 🚀${NC}" 