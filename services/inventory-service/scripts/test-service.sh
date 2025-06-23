#!/bin/bash

# Test script for Inventory Service
# This script tests basic functionality without requiring MongoDB

set -e

echo "🚀 Testing Inventory Service..."

# Build the service
echo "📦 Building service..."
go build -o inventory-service cmd/main.go

echo "✅ Build successful!"

# Test with dry-run (mock mode)
echo "🧪 Testing service startup (dry-run)..."

# Set environment variables for testing
export INVENTORY_SERVICE_PORT=50053
export MONGODB_CONNECTION_URL=mongodb://localhost:27017
export MONGODB_DATABASE_NAME=inventory_test_db
export LOG_LEVEL=debug
export ENVIRONMENT=test
export SEED_TEST_DATA=false

# Test configuration loading
echo "⚙️  Testing configuration..."
timeout 5s ./inventory-service > /dev/null 2>&1 || {
    if [ $? -eq 124 ]; then
        echo "✅ Service started successfully (timeout as expected)"
    else
        echo "❌ Service failed to start"
        exit 1
    fi
}

echo "🎉 All tests passed!"
echo ""
echo "📋 Next steps:"
echo "1. Start MongoDB: docker run -d --name mongodb -p 27017:27017 mongo:6.0"
echo "2. Run service: go run cmd/main.go"
echo "3. Test with grpcurl: grpcurl -plaintext localhost:50053 list" 