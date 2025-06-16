#!/bin/bash

# Protobuf Generation Script for Rocket Science Project
# This script generates Go code from protobuf definitions

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

echo "🚀 Generating protobuf code for Rocket Science services..."
echo "📁 Project root: $PROJECT_ROOT"

# Function to generate protobuf for a service
generate_proto() {
    local service_name=$1
    local proto_dir="$PROJECT_ROOT/services/$service_name/proto"
    
    if [ ! -d "$proto_dir" ]; then
        echo "❌ Proto directory not found: $proto_dir"
        return 1
    fi
    
    echo "🔧 Generating protobuf for $service_name..."
    
    cd "$PROJECT_ROOT/services/$service_name"
    
    # Find all .proto files and generate Go code
    find proto -name "*.proto" -exec protoc \
        --go_out=. \
        --go-grpc_out=. \
        --go_opt=paths=source_relative \
        --go-grpc_opt=paths=source_relative \
        {} \;
    
    echo "✅ Generated protobuf for $service_name"
}

# Check if protoc is installed
if ! command -v protoc &> /dev/null; then
    echo "❌ protoc is not installed. Please install Protocol Buffers compiler."
    echo "   On macOS: brew install protobuf"
    echo "   On Ubuntu: sudo apt-get install protobuf-compiler"
    exit 1
fi

# Check if protoc-gen-go is installed
if ! command -v protoc-gen-go &> /dev/null; then
    echo "❌ protoc-gen-go is not installed. Installing..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
fi

# Check if protoc-gen-go-grpc is installed
if ! command -v protoc-gen-go-grpc &> /dev/null; then
    echo "❌ protoc-gen-go-grpc is not installed. Installing..."
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
fi

echo ""
echo "🔧 Starting protobuf generation..."

# Generate for each service
generate_proto "payment-service"
generate_proto "iam-service" 
generate_proto "inventory-service"

echo ""
echo "🎉 Protobuf generation completed!"
echo ""
echo "📋 Next steps:"
echo "1. Run: chmod +x tools/proto-gen/generate.sh"
echo "2. Run: ./tools/proto-gen/generate.sh"
echo "3. Check generated .pb.go files in each service's proto directory"