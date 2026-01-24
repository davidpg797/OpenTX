#!/bin/bash

# Quick Start Script for Canonical Transaction Gateway

set -e

echo "🚀 Starting Canonical Transaction Gateway..."

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    echo "❌ Docker is not installed. Please install Docker first."
    exit 1
fi

if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose is not installed. Please install Docker Compose first."
    exit 1
fi

if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.21 or higher."
    exit 1
fi

echo "✅ Prerequisites met"

# Install Go dependencies
echo "📦 Installing Go dependencies..."
go mod download

# Generate protobuf code
echo "🔨 Generating protobuf code..."
if ! command -v protoc &> /dev/null; then
    echo " protoc not found. Installing protoc-gen-go..."
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
else
    make proto
fi

# Build the gateway
echo "Building gateway..."
make build

# Start infrastructure services
echo "Starting infrastructure services (Redis, Kafka, Jaeger, etc.)..."
cd docker
docker-compose up -d redis postgres kafka jaeger prometheus grafana
cd ..

# Wait for services to be ready
echo "⏳ Waiting for services to be ready..."
sleep 10

# Check service health
echo "🏥 Checking service health..."
docker-compose -f docker/docker-compose.yml ps

echo ""
echo "Infrastructure services are running!"
echo ""
echo "Access the following services:"
echo "   - Jaeger UI (Tracing): http://localhost:16686"
echo "   - Prometheus: http://localhost:9091"
echo "   - Grafana: http://localhost:3000 (admin/admin123)"
echo "   - Redis: localhost:6379"
echo "   - Kafka: localhost:9092"
echo ""
echo "To start the gateway:"
echo "   ./bin/gateway --config configs/gateway.yaml"
echo ""
echo "To run tests:"
echo "   make test"
echo ""
echo "To stop all services:"
echo "   docker-compose -f docker/docker-compose.yml down"
echo ""
