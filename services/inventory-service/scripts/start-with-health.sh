#!/bin/bash

# Start script for Inventory Service with Health Check endpoints
# This script starts both the main gRPC service and the HTTP health server

set -e

# Configuration
SERVICE_NAME="inventory-service"
GRPC_PORT="${INVENTORY_SERVICE_PORT:-50053}"
HEALTH_PORT="${INVENTORY_HEALTH_PORT:-8080}"
LOG_LEVEL="${LOG_LEVEL:-info}"

echo "🚀 Starting Rocket Science Inventory Service"
echo "   - gRPC Port: $GRPC_PORT"
echo "   - Health Port: $HEALTH_PORT"
echo "   - Log Level: $LOG_LEVEL"

# Function to handle cleanup on script exit
cleanup() {
    echo "🛑 Shutting down services..."
    if [ ! -z "$GRPC_PID" ]; then
        kill $GRPC_PID 2>/dev/null || true
    fi
    if [ ! -z "$HEALTH_PID" ]; then
        kill $HEALTH_PID 2>/dev/null || true
    fi
    exit 0
}

# Set up signal handlers
trap cleanup SIGINT SIGTERM

# Start the main gRPC service
echo "📡 Starting gRPC service on port $GRPC_PORT..."
./main &
GRPC_PID=$!

# Give the main service a moment to start
sleep 2

# Start the health check server using a Go program
echo "🏥 Starting health server on port $HEALTH_PORT..."
go run ./scripts/health-server.go &
HEALTH_PID=$!

# Wait for both processes
echo "✅ Services started successfully"
echo "   - gRPC Service PID: $GRPC_PID"
echo "   - Health Server PID: $HEALTH_PID"
echo ""
echo "Health endpoints available at:"
echo "   - http://localhost:$HEALTH_PORT/health"
echo "   - http://localhost:$HEALTH_PORT/ready"
echo "   - http://localhost:$HEALTH_PORT/live"
echo "   - http://localhost:$HEALTH_PORT/metrics"
echo "   - http://localhost:$HEALTH_PORT/stats"
echo ""
echo "Press Ctrl+C to stop both services"

# Wait for either process to exit
wait 