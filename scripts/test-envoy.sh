#!/bin/bash

# Test script for Envoy gateway functionality

set -e

echo "🧪 Testing Envoy Gateway Configuration..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

GATEWAY_URL="http://localhost"
ADMIN_URL="http://localhost:9901"

# Function to test endpoint
test_endpoint() {
    local url=$1
    local expected_status=$2
    local description=$3
    local headers=$4
    
    echo -e "${BLUE}Testing: ${description}${NC}"
    
    if [ -n "$headers" ]; then
        response=$(curl -s -o /dev/null -w "%{http_code}" -H "$headers" "$url" || echo "000")
    else
        response=$(curl -s -o /dev/null -w "%{http_code}" "$url" || echo "000")
    fi
    
    if [ "$response" -eq "$expected_status" ]; then
        echo -e "${GREEN}✅ PASS: $description (Status: $response)${NC}"
        return 0
    else
        echo -e "${RED}❌ FAIL: $description (Expected: $expected_status, Got: $response)${NC}"
        return 1
    fi
}

# Function to test Envoy admin endpoints
test_admin() {
    echo -e "\n${YELLOW}=== Testing Envoy Admin Interface ===${NC}"
    
    test_endpoint "$ADMIN_URL/ready" 200 "Envoy readiness check"
    test_endpoint "$ADMIN_URL/stats/prometheus" 200 "Envoy Prometheus metrics"
    test_endpoint "$ADMIN_URL/clusters" 200 "Envoy cluster status"
    
    echo -e "\n${BLUE}Checking cluster health...${NC}"
    clusters=$(curl -s "$ADMIN_URL/clusters" | grep -E "(order-service|iam-service|payment-service|inventory-service)" | head -5 || true)
    if [ -n "$clusters" ]; then
        echo -e "${GREEN}✅ Backend clusters configured${NC}"
        echo "$clusters"
    else
        echo -e "${RED}❌ No backend clusters found${NC}"
    fi
}

# Function to test public endpoints (no auth required)
test_public_endpoints() {
    echo -e "\n${YELLOW}=== Testing Public Endpoints ===${NC}"
    
    test_endpoint "$GATEWAY_URL/health" 200 "Health check endpoint"
}

# Function to test protected endpoints (auth required)
test_protected_endpoints() {
    echo -e "\n${YELLOW}=== Testing Protected Endpoints (Should Return 401) ===${NC}"
    
    test_endpoint "$GATEWAY_URL/api/orders" 401 "Orders API without auth"
}

# Function to test monitoring routes
test_monitoring_routes() {
    echo -e "\n${YELLOW}=== Testing Monitoring Routes ===${NC}"
    
    # These might return various status codes depending on service availability
    # 200 = service up, 503 = service down but route configured
    echo -e "${BLUE}Testing monitoring service routes (200 or 503 expected)...${NC}"
    
    grafana_status=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/grafana" || echo "000")
    jaeger_status=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/jaeger" || echo "000")
    kibana_status=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/kibana" || echo "000")
    prometheus_status=$(curl -s -o /dev/null -w "%{http_code}" "$GATEWAY_URL/prometheus" || echo "000")
    
    echo -e "Grafana route: $grafana_status"
    echo -e "Jaeger route: $jaeger_status"
    echo -e "Kibana route: $kibana_status"
    echo -e "Prometheus route: $prometheus_status"
}

# Function to test with mock auth token
test_with_mock_token() {
    echo -e "\n${YELLOW}=== Testing with Mock Auth Token ===${NC}"
    
    mock_token="mock-session-token-for-testing"
    
    # This should still return 401 because IAM service will reject the mock token
    test_endpoint "$GATEWAY_URL/api/orders" 401 "Orders API with mock token" "Authorization: Bearer $mock_token"
    
    echo -e "${BLUE}Note: 401 is expected because mock token won't be validated by IAM service${NC}"
}

# Main test execution
main() {
    echo -e "${BLUE}🧪 Envoy Gateway Test Suite${NC}"
    echo -e "${BLUE}Testing gateway at: $GATEWAY_URL${NC}"
    echo -e "${BLUE}Testing admin interface at: $ADMIN_URL${NC}"
    echo ""
    
    # Check if Envoy is running
    if ! curl -s "$ADMIN_URL/ready" > /dev/null 2>&1; then
        echo -e "${RED}❌ Envoy is not running or not accessible at $ADMIN_URL${NC}"
        echo -e "${YELLOW}Make sure to start the system first: ./scripts/start-system.sh${NC}"
        exit 1
    fi
    
    total_tests=0
    passed_tests=0
    
    # Run test suites
    test_admin
    test_public_endpoints
    test_protected_endpoints
    test_monitoring_routes
    test_with_mock_token
    
    echo ""
    echo -e "${GREEN}🎉 Test suite completed!${NC}"
    echo ""
    echo -e "${BLUE}📋 Summary:${NC}"
    echo -e "  - Envoy admin interface: ✅ Accessible"
    echo -e "  - Public endpoints: ✅ Working"
    echo -e "  - Protected endpoints: ✅ Properly secured"
    echo -e "  - Monitoring routes: 🔄 Configured"
    echo ""
    echo -e "${YELLOW}💡 Next steps:${NC}"
    echo -e "  1. Start all services: ./scripts/start-system.sh"
    echo -e "  2. Test with real auth token from IAM service"
    echo -e "  3. Verify monitoring dashboards at /grafana, /jaeger, etc."
    echo ""
}

# Run tests
main "$@" 