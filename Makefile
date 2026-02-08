.PHONY: build build-release clean run dev test test-coverage lint fmt-check fmt vet tidy install docker-build docker-run deps verify check release help build-linux build-darwin build-windows build-all

# Default target
.DEFAULT_GOAL := build

# Binary name
BINARY_NAME=litmuschaos-mcp-server
BINARY_PATH=./bin/$(BINARY_NAME)

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Build flags
BUILD_FLAGS=-ldflags="-s -w"
BUILD_FLAGS_RELEASE=-ldflags="-s -w" -trimpath

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p bin
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_PATH) .
	@echo "Binary built at $(BINARY_PATH)"

## build-release: Build optimized release binary
build-release:
	@echo "Building release $(BINARY_NAME)..."
	@mkdir -p bin
	$(GOBUILD) $(BUILD_FLAGS_RELEASE) -o $(BINARY_PATH) .
	@echo "Release binary built at $(BINARY_PATH)"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf bin/
	@echo "Clean complete"

## run: Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	$(BINARY_PATH)

## dev: Run with file watching for development
dev:
	@echo "Starting development server..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Air not found. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Running without hot reload..."; \
		$(MAKE) run; \
	fi

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linter
lint:
	@echo "Running linter..."
	@go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

## fmt-check: Check code formatting
fmt-check:
	@echo "Checking code formatting..."
	@$(GOFMT) -l .

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@echo "Code formatted"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## tidy: Tidy modules
tidy:
	@echo "Tidying modules..."
	$(GOMOD) tidy
	@echo "Modules tidied"

## install: Install the binary
install: build-release
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BINARY_PATH) $(GOPATH)/bin/ 2>/dev/null || cp $(BINARY_PATH) /usr/local/bin/
	@echo "$(BINARY_NAME) installed"

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t litmuschaos-mcp-server:latest .
	@echo "Docker image built"

## docker-run: Run Docker container
docker-run: docker-build
	@echo "Running Docker container..."
	docker run --rm -it \
		-e CHAOS_CENTER_ENDPOINT \
		-e LITMUS_PROJECT_ID \
		-e LITMUS_ACCESS_TOKEN \
		litmuschaos-mcp-server:latest

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded"

## verify: Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify
	@echo "Dependencies verified"

## check: Run all checks (fmt-check, vet, lint, test)
check: fmt-check vet lint test
	@echo "All checks passed"

## release: Prepare release build
release: clean check build-release
	@echo "Release ready: $(BINARY_PATH)"

## help: Show this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

# Cross-compilation targets
## build-linux: Build for Linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS_RELEASE) -o bin/$(BINARY_NAME)-linux-amd64 .
	@echo "Linux binary built at bin/$(BINARY_NAME)-linux-amd64"

## build-darwin: Build for macOS
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p bin
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS_RELEASE) -o bin/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(BUILD_FLAGS_RELEASE) -o bin/$(BINARY_NAME)-darwin-arm64 .
	@echo "macOS binaries built"

## build-windows: Build for Windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p bin
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(BUILD_FLAGS_RELEASE) -o bin/$(BINARY_NAME)-windows-amd64.exe .
	@echo "Windows binary built at bin/$(BINARY_NAME)-windows-amd64.exe"

## build-all: Build for all platforms
build-all: build-linux build-darwin build-windows
	@echo "All platform binaries built"
