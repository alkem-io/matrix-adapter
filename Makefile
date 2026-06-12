# Variables
BINARY_NAME=adapter
ENTRYPOINT=cmd/adapter/main.go
BUILD_DIR=bin

# Go commands
GO=go
GOTEST=$(GO) test
GOBUILD=$(GO) build
GOCLEAN=$(GO) clean
GOVET=$(GO) vet
GOFMT=$(GO) fmt

# Default target
.PHONY: all
all: deps fmt lint test build

# Generate OpenAPI spec from Go source.
# Requires the `apispec` binary on PATH. CI installs the org-pinned version
# (see go-ci.yml in antst/alkemio-github-workflows); locally:
#   go install github.com/antst/go-apispec/cmd/apispec@v0.4.16
.PHONY: openapi
openapi:
	@echo "Generating OpenAPI spec..."
	apispec --dir . --output openapi.yaml --config apispec.yaml

# Build the application
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(ENTRYPOINT)

# Run the application
.PHONY: run
run:
	@echo "Running $(BINARY_NAME)..."
	$(GO) run $(ENTRYPOINT)

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Lint the code
.PHONY: lint
lint:
	@echo "Linting..."
	$(GOVET) ./...
	$(GO) tool golangci-lint run

# Lint markdown files
.PHONY: lint-md
lint-md:
	@echo "Linting Markdown files..."
	@npx markdownlint-cli "**/*.md" --ignore node_modules --ignore lib/node_modules

# Format the code
.PHONY: fmt
fmt:
	@echo "Formatting..."
	$(GOFMT) ./...

# Generate all artifacts
.PHONY: generate
generate: generate-go generate-events openapi

# Generate Go code (DTOs, mocks)
.PHONY: generate-go
generate-go:
	@echo "Generating Go code..."
	$(GO) generate ./...

# Generate TypeScript files (events + commands)
.PHONY: generate-events
generate-events:
	@echo "Generating TypeScript files..."
	$(GO) run cmd/gen-events/main.go lib/src

# Serve documentation
.PHONY: doc
doc:
	@echo "Starting documentation server..."
	$(GO) doc -http

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Docker build
.PHONY: docker-build
docker-build:
	@echo "Building Docker image..."
	docker build -t alkemio/matrix-adapter .

# Help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  all            - Run deps, fmt, lint, test, and build"
	@echo "  build          - Build the application binary"
	@echo "  run            - Run the application locally"
	@echo "  test           - Run unit tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  lint           - Run linters (go vet, golangci-lint)"
	@echo "  lint-md        - Lint Markdown files (uses markdownlint-cli)"
	@echo "  fmt            - Format code"
	@echo "  generate       - Run go generate"
	@echo "  doc            - Serve documentation (using go doc -http)"
	@echo "  clean          - Remove build artifacts"
	@echo "  deps           - Download and tidy dependencies"
	@echo "  openapi        - Generate OpenAPI spec from Go source"
	@echo "  docker-build   - Build Docker image"
