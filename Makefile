.PHONY: build test run clean deps fmt lint

# Variables
BINARY_NAME=analyzer
BUILD_DIR=bin
MAIN_PATH=cmd/analyzer/main.go

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run property-based tests with more iterations
test-property:
	@echo "Running property-based tests..."
	@go test -v ./... -args -gopter.minSuccessfulTests=100

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -rf data/*
	@rm -rf reports/*
	@rm -rf pending/*

# Run the application in dry-run mode
test-run: build
	@echo "Running in dry-run mode..."
	@./$(BUILD_DIR)/$(BINARY_NAME) --mode daily --dry-run --verbose

# Run the application
run: build
	@./$(BUILD_DIR)/$(BINARY_NAME) --mode daily

# Create necessary directories
setup-dirs:
	@mkdir -p data reports pending logs configs/known-issues

# Development setup
dev-setup: deps setup-dirs
	@cp configs/config.example.yaml configs/config.yaml
	@echo "✅ Development environment set up"
	@echo "📝 Please edit configs/config.yaml with your OpenSearch settings"