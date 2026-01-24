.PHONY: proto build test clean run-gateway run-simulator docker-build docker-up integration cert-test lint

# Build variables
BINARY_DIR := bin
PROTO_DIR := proto
PKG_DIR := pkg
CMD_DIR := cmd

# Binaries
GATEWAY_BIN := $(BINARY_DIR)/gateway
SIMULATOR_BIN := $(BINARY_DIR)/simulator
CERTTEST_BIN := $(BINARY_DIR)/certtest

# Go parameters
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := $(GOCMD) fmt
GOVET := $(GOCMD) vet

# Protoc parameters
PROTOC := protoc
PROTOC_GEN_GO := $(GOPATH)/bin/protoc-gen-go
PROTOC_GEN_GO_GRPC := $(GOPATH)/bin/protoc-gen-go-grpc

all: proto build

# Install dependencies
deps:
	$(GOGET) -u google.golang.org/protobuf/cmd/protoc-gen-go
	$(GOGET) -u google.golang.org/grpc/cmd/protoc-gen-go-grpc
	$(GOMOD) download
	$(GOMOD) tidy

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@mkdir -p $(PKG_DIR)/proto
	$(PROTOC) --go_out=$(PKG_DIR)/proto --go_opt=paths=source_relative \
		--go-grpc_out=$(PKG_DIR)/proto --go-grpc_opt=paths=source_relative \
		-I=$(PROTO_DIR) \
		$(PROTO_DIR)/canonical/*.proto \
		$(PROTO_DIR)/iso8583/*.proto \
		$(PROTO_DIR)/events/*.proto

# Build all binaries
build: build-gateway build-simulator build-certtest

build-gateway:
	@echo "Building gateway..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(GATEWAY_BIN) $(CMD_DIR)/gateway/main.go

build-simulator:
	@echo "Building simulator..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(SIMULATOR_BIN) $(CMD_DIR)/simulator/main.go

build-certtest:
	@echo "Building certtest..."
	@mkdir -p $(BINARY_DIR)
	$(GOBUILD) -o $(CERTTEST_BIN) $(CMD_DIR)/certtest/main.go

# Run services
run-gateway: build-gateway
	$(GATEWAY_BIN) --config configs/gateway.yaml

run-simulator: build-simulator
	$(SIMULATOR_BIN) --config configs/simulator.yaml

# Tests
test:
	@echo "Running unit tests..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./test/integration/...

cert-test: build-certtest
	@echo "Running certification tests..."
	$(CERTTEST_BIN) --config configs/certtest.yaml

# Code quality
lint:
	@echo "Running linters..."
	$(GOFMT) ./...
	$(GOVET) ./...
	@which golangci-lint > /dev/null 2>&1 || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Docker
docker-build:
	docker build -t canonical-gateway:latest -f docker/Dockerfile.gateway .
	docker build -t canonical-simulator:latest -f docker/Dockerfile.simulator .

docker-up:
	docker-compose -f docker/docker-compose.yml up -d

docker-down:
	docker-compose -f docker/docker-compose.yml down

# Clean
clean:
	@echo "Cleaning..."
	rm -rf $(BINARY_DIR)
	rm -f coverage.out
	rm -rf $(PKG_DIR)/proto

# Help
help:
	@echo "Available targets:"
	@echo "  make deps          - Install dependencies"
	@echo "  make proto         - Generate protobuf code"
	@echo "  make build         - Build all binaries"
	@echo "  make test          - Run unit tests"
	@echo "  make integration   - Run integration tests"
	@echo "  make cert-test     - Run certification tests"
	@echo "  make lint          - Run linters"
	@echo "  make run-gateway   - Run gateway service"
	@echo "  make run-simulator - Run ISO message simulator"
	@echo "  make docker-build  - Build Docker images"
	@echo "  make docker-up     - Start services with Docker Compose"
	@echo "  make clean         - Clean build artifacts"
