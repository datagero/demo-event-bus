# Makefile for Demo Event Bus API Server

.PHONY: test test-verbose test-coverage test-race test-unit test-integration build clean lint help

# Default target
help:
	@echo "Available targets:"
	@echo "  test          - Run all tests"
	@echo "  test-verbose  - Run tests with verbose output"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  test-race     - Run tests with race detection"
	@echo "  test-unit     - Run only unit tests"
	@echo "  test-handlers - Run only handler tests"
	@echo "  build         - Build the API server"
	@echo "  clean         - Clean build artifacts"
	@echo "  lint          - Run linter"
	@echo "  run           - Run the API server"
	@echo "  deps          - Download dependencies"

# Test targets
test:
	@echo "🧪 Running all tests..."
	go test ./...

test-verbose:
	@echo "🧪 Running tests with verbose output..."
	go test -v ./...

test-coverage:
	@echo "🧪 Running tests with coverage..."
	go test -cover ./...
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "📊 Coverage report generated: coverage.html"

test-race:
	@echo "🧪 Running tests with race detection..."
	go test -race ./...

test-unit:
	@echo "🧪 Running unit tests..."
	go test -run "^Test" ./internal/...

test-handlers:
	@echo "🧪 Running handler tests..."
	go test -v ./internal/api/handlers/

test-rabbitmq:
	@echo "🧪 Running RabbitMQ handler tests..."
	go test -v -run "TestRabbitMQ" ./internal/api/handlers/

test-messages:
	@echo "🧪 Running message handler tests..."
	go test -v -run "TestMessage" ./internal/api/handlers/

# Build targets
build:
	@echo "🏗️ Building API server..."
	go build -o api-server-test .

build-production:
	@echo "🏗️ Building production API server..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api-server .

clean:
	@echo "🧹 Cleaning build artifacts..."
	rm -f api-server api-server-test coverage.out coverage.html

# Development targets
run:
	@echo "🚀 Running API server..."
	go run main.go

run-test:
	@echo "🚀 Running test API server..."
	./api-server-test

deps:
	@echo "📦 Downloading dependencies..."
	go mod download
	go mod tidy

# Linting
lint:
	@echo "🔍 Running linter..."
	golangci-lint run

# Docker targets (if needed)
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t demo-event-bus-api .

# Integration test targets (for when services are running)
test-integration:
	@echo "🧪 Running integration tests..."
	@echo "⚠️  Make sure RabbitMQ and Workers service are running"
	go test -tags=integration ./tests/integration/

# Test with isolated environment (separate ports)
test-isolated:
	@echo "🧪 Running tests with isolated environment..."
	@echo "📡 Tests will use ports 9001 (API) and 8002 (Workers) to avoid conflicts"
	go test -v ./internal/api/handlers/

test-recruitment:
	@echo "🧪 Testing recruitment functionality specifically..."
	go test -v -run "TestPlayerEndpoints" ./internal/api/handlers/

# Quick issue diagnosis
diagnose:
	@echo "🔍 Running quick diagnosis..."
	@../debug-helpers.sh full-check

# Test just the route contracts
test-contracts:
	@echo "🔗 Testing frontend-backend route contracts..."
	@../debug-helpers.sh contracts
	@../debug-helpers.sh routes

# Fast feedback loop for development
dev-check:
	@echo "⚡ Fast development check..."
	@../debug-helpers.sh health
	@echo ""
	@echo "🧪 Quick tests:"
	go test -run "TestHealthEndpoint|TestPlayerEndpoints/.*CompatibilityRoute" ./internal/api/handlers/ -v

# Benchmark tests
benchmark:
	@echo "📈 Running benchmarks..."
	go test -bench=. ./...

# Test specific endpoints
test-health:
	@echo "🧪 Testing health endpoint..."
	go test -v -run "TestHealthEndpoint" ./internal/api/handlers/

test-game:
	@echo "🧪 Testing game endpoints..."
	go test -v -run "TestGameState\|TestPlayer" ./internal/api/handlers/

test-chaos:
	@echo "🧪 Testing chaos endpoints..."
	go test -v -run "TestChaos" ./internal/api/handlers/

test-dlq:
	@echo "🧪 Testing DLQ endpoints..."
	go test -v -run "TestDLQ" ./internal/api/handlers/

test-scenarios:
	@echo "🧪 Testing scenario endpoints..."
	go test -v -run "TestScenario" ./internal/api/handlers/

# Performance testing
test-load:
	@echo "📈 Running load tests..."
	@echo "⚠️  This requires 'hey' tool: go install github.com/rakyll/hey@latest"
	hey -n 1000 -c 10 http://localhost:9000/health

# Watch tests (requires entr: brew install entr)
test-watch:
	@echo "👀 Watching for changes and running tests..."
	find . -name "*.go" | entr -c make test

# Generate test mocks (if using mockery)
generate-mocks:
	@echo "🤖 Generating mocks..."
	mockery --all --output ./internal/mocks

# Setup development environment
setup-dev:
	@echo "⚙️ Setting up development environment..."
	go mod download
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/rakyll/hey@latest
	@echo "✅ Development environment ready!"

# Run all checks (for CI)
ci: deps lint test test-race
	@echo "✅ All CI checks passed!"