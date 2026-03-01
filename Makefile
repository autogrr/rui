# rui Makefile

# Load .env file if it exists (silently)
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse HEAD 2> /dev/null)
GIT_TAG := $(shell git describe --abbrev=0 --tags)
BINARY_NAME = rui
BUILD_DIR = build

# Go build flags
LDFLAGS = -ldflags "-X github.com/autogrr/rui/internal/buildinfo.Version=$(VERSION)"

.PHONY: all build backend dev dev-backend clean test test-openapi help lint lint-full lint-json lint-fix fmt modern deps docs-dev docs-build templ-generate templ-watch tailwind-ui tailwind-ui-watch build/docker build/dockerx

# Default target
all: build

# Build (templ generate + Go binary)
build: backend

build/docker:
@echo "Building docker image..."
docker build -t ghcr.io/autogrr/rui:dev -f distrib/docker/Dockerfile . --build-arg GIT_TAG=$(GIT_TAG) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(VERSION)

build/dockerx:
docker buildx build -t ghcr.io/autogrr/rui:dev -f distrib/docker/Dockerfile . --build-arg GIT_TAG=$(GIT_TAG) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(VERSION) --platform=linux/amd64,linux/arm64 --pull --load

# Build backend
backend: templ-generate
@echo "Building backend..."
go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/rui

# Development mode with hot reload (requires air)
dev: dev-backend

# Run backend with hot reload (requires air)
dev-backend:
@echo "Starting backend development server..."
air -c .air.toml

# Clean build artifacts
clean:
@echo "Cleaning..."
rm -rf $(BINARY_NAME) $(BUILD_DIR)
@echo "Cleaning templ-generated files..."
find internal/ui -name '*_templ.go' -delete

# Generate templ templates for the server-rendered UI (required before building backend)
templ-generate:
@echo "Generating templ templates..."
templ generate ./internal/ui/...

# Watch and regenerate templ templates on file change
templ-watch:
@echo "Watching templ templates..."
templ generate --watch ./internal/ui/...

# Compile Tailwind CSS for the server-rendered UI
# Uses pnpm when available, otherwise falls back to npm (works on NixOS via nix-store node).
NIX_NODE ?= $(firstword $(wildcard /nix/store/*-nodejs-22.*/bin/node) $(wildcard /nix/store/*-nodejs-24.*/bin/node))
NODE_BIN  = $(if $(shell command -v node 2>/dev/null),node,$(NIX_NODE))
NPM_BIN   = $(if $(shell command -v pnpm 2>/dev/null),pnpm,$(if $(shell command -v npm 2>/dev/null),npm,$(dir $(NODE_BIN))npm))

tailwind-ui:
@echo "Building Tailwind CSS for server-rendered UI..."
cd internal/ui/css && PATH="$(dir $(NODE_BIN)):$$PATH" $(NPM_BIN) install && PATH="$(dir $(NODE_BIN)):$$PATH" $(NPM_BIN) run build

# Watch Tailwind CSS for the server-rendered UI
tailwind-ui-watch:
@echo "Watching Tailwind CSS for server-rendered UI..."
cd internal/ui/css && PATH="$(dir $(NODE_BIN)):$$PATH" $(NPM_BIN) install && PATH="$(dir $(NODE_BIN)):$$PATH" $(NPM_BIN) run dev

# Run tests
test:
@echo "Running tests..."
go test -race -count=3 -v ./...

# Validate OpenAPI specification
test-openapi:
@echo "Validating OpenAPI specification..."
go test -v ./internal/web/swagger

# Format changed Go code only (fast, for iteration)
fmt:
@echo "Formatting changed Go code..."
@gofiles=$$({ git diff --name-only --diff-filter=d; git diff --name-only --cached --diff-filter=d; } | sort -u | grep '\.go$$' || true); \
if [ -n "$$gofiles" ]; then echo "$$gofiles" | xargs gofmt -w; fi

# Lint code (changed files only - fast feedback for AI iteration)
lint:
@echo "Linting changed Go code..."
golangci-lint run --new-from-merge-base=develop --timeout=5m

# Full lint (entire codebase - use before commits/PRs)
lint-full:
@echo "Linting entire Go codebase..."
golangci-lint run --timeout=10m

# Lint with JSON output (for AI agent consumption)
lint-json:
@echo "Generating lint report..."
golangci-lint run --new-from-merge-base=main --output.json.path=./lint-report.json --timeout=5m || true
@echo "Lint report saved to lint-report.json"

# Lint with auto-fix where possible
lint-fix:
@echo "Running linters with auto-fix..."
golangci-lint run --fix --timeout=10m

# Modernize Go code (interface{} -> any, etc)
modern:
@echo "Modernizing Go code..."
go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix -test ./...

# Install development dependencies
deps:
@echo "Installing development dependencies..."
go mod download

# Documentation development server
docs-dev:
@echo "Starting documentation development server..."
cd documentation && pnpm start

# Build documentation
docs-build:
@echo "Building documentation..."
cd documentation && pnpm build

# Help
help:
@echo "Available targets:"
@echo ""
@echo "Build:"
@echo "  make build            - Build (templ generate + Go binary)"
@echo "  make backend          - Build Go binary (runs templ generate)"
@echo "  make build/docker     - Build Docker image"
@echo ""
@echo "Development:"
@echo "  make dev              - Run backend with hot reload (air)"
@echo "  make dev-backend      - Run backend with hot reload (air)"
@echo ""
@echo "Server-rendered UI:"
@echo "  make templ-generate   - Compile *.templ to *_templ.go"
@echo "  make templ-watch      - Watch + regenerate templ on change"
@echo "  make tailwind-ui      - Compile Tailwind CSS for templ UI"
@echo "  make tailwind-ui-watch- Watch Tailwind CSS for templ UI"
@echo ""
@echo "Testing:"
@echo "  make test             - Run all tests with race detection"
@echo "  make test-openapi     - Validate OpenAPI specification"
@echo ""
@echo "Linting:"
@echo "  make lint             - Lint changed files only (fast)"
@echo "  make lint-full        - Lint entire codebase"
@echo "  make lint-json        - Generate JSON lint report"
@echo "  make lint-fix         - Auto-fix linting issues"
@echo ""
@echo "Formatting:"
@echo "  make fmt              - Format changed Go files only"
@echo "  make modern           - Modernize Go code (interface{} -> any)"
@echo ""
@echo "Documentation:"
@echo "  make docs-dev         - Run documentation development server"
@echo "  make docs-build       - Build documentation for production"
@echo ""
@echo "Other:"
@echo "  make clean            - Clean build artifacts"
@echo "  make deps             - Install Go dependencies"
@echo "  make help             - Show this help message"
