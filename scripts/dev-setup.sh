#!/bin/bash

# Development environment setup script

set -e

echo "🔧 Setting up development environment..."

# Install pre-commit hooks
echo "📌 Setting up git hooks..."
cat > .git/hooks/pre-commit << 'EOF'
#!/bin/bash
# Run tests before commit
make test
if [ $? -ne 0 ]; then
    echo "❌ Tests failed. Commit aborted."
    exit 1
fi

# Run linters
make lint
if [ $? -ne 0 ]; then
    echo "❌ Linting failed. Commit aborted."
    exit 1
fi
EOF

chmod +x .git/hooks/pre-commit

# Install development tools
echo "🛠️  Installing development tools..."
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Create necessary directories
echo "📁 Creating project directories..."
mkdir -p bin
mkdir -p logs
mkdir -p test/vectors
mkdir -p test/certification

# Generate test certificates (for development only)
echo "🔐 Generating test certificates..."
mkdir -p configs/certs
openssl genrsa -out configs/certs/test-private.pem 2048
openssl rsa -in configs/certs/test-private.pem -pubout -out configs/certs/test-public.pem

echo "✅ Development environment setup complete!"
echo ""
echo "Next steps:"
echo "1. Run ./quickstart.sh to start infrastructure"
echo "2. Run make build to build the gateway"
echo "3. Run make test to run tests"
echo "4. Start developing! 🚀"
