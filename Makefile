# qBittorrent WebUI Makefile

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
WEB_DIR = web
INTERNAL_WEB_DIR = internal/web

# Go build flags
LDFLAGS = -ldflags "-X github.com/autogrr/rui/internal/buildinfo.Version=$(VERSION)"

.PHONY: all build frontend backend dev dev-backend dev-frontend dev-expose clean test help themes-fetch themes-clean lint lint-full lint-json lint-fix fmt modern deps docs-dev docs-build templ-generate templ-watch tailwind-ui tailwind-ui-watch

# Default target
all: build

# Build both frontend and backend
build: frontend backend

build/docker:
	@echo "Building docker image..."
	docker build -t ghcr.io/autogrr/rui:dev -f distrib/docker/Dockerfile . --build-arg  GIT_TAG=$(GIT_TAG) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(VERSION)

build/dockerx:
	docker buildx build -t ghcr.io/autogrr/rui:dev -f distrib/docker/Dockerfile . --build-arg GIT_TAG=$(GIT_TAG) --build-arg GIT_COMMIT=$(GIT_COMMIT) --build-arg VERSION=$(VERSION) --platform=linux/amd64,linux/arm64 --pull --load

# Fetch premium themes from private repository
themes-fetch:
	@echo "Fetching premium themes..."
	@if [ -n "$$THEMES_REPO_TOKEN" ]; then \
		rm -rf .themes-temp && \
		git clone --depth=1 --filter=blob:none --sparse \
			https://$$THEMES_REPO_TOKEN@github.com/autogrr/rui-premium-themes.git .themes-temp && \
		cd .themes-temp && git sparse-checkout set --cone themes && cd .. && \
		mkdir -p $(WEB_DIR)/src/themes/premium && \
		cp .themes-temp/themes/*.css $(WEB_DIR)/src/themes/premium/ && \
		rm -rf .themes-temp && \
		echo "Premium themes fetched successfully"; \
	else \
		echo "THEMES_REPO_TOKEN not set, skipping premium themes"; \
	fi

# Clean premium themes
themes-clean:
	@echo "Cleaning premium themes..."
	rm -rf $(WEB_DIR)/src/themes/premium

# Build frontend
frontend: themes-fetch
	@echo "Building frontend..."
	cd $(WEB_DIR) && pnpm install && pnpm build
	@echo "Copying frontend assets..."
	rm -rf $(INTERNAL_WEB_DIR)/dist
	cp -r $(WEB_DIR)/dist $(INTERNAL_WEB_DIR)/

# Build backend
backend: templ-generate
	@echo "Building backend..."
	go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/rui

# Development mode - run both frontend and backend
dev:
	@echo "Starting development mode..."
	@make -j 2 dev-backend dev-frontend

# Run backend with hot reload (requires air)
dev-backend:
	@echo "Starting backend development server..."
	air -c .air.toml

# Run frontend development server
dev-frontend:
	@echo "Starting frontend development server..."
	cd $(WEB_DIR) && pnpm dev

# Development mode with frontend exposed on 0.0.0.0
dev-expose:
	@echo "Starting development mode with frontend exposed on 0.0.0.0..."
	@make -j 2 dev-backend dev-frontend-expose

# Run frontend development server exposed on 0.0.0.0
dev-frontend-expose:
	@echo "Starting frontend development server (exposed on 0.0.0.0)..."
	cd $(WEB_DIR) && pnpm dev --host

# Clean build artifacts
clean: themes-clean
	@echo "Cleaning..."
	rm -rf $(WEB_DIR)/dist $(INTERNAL_WEB_DIR)/dist $(BINARY_NAME) $(BUILD_DIR)
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

# Format changed code only (fast, for iteration)
fmt:
	@echo "Formatting changed Go code..."
	@gofiles=$$({ git diff --name-only --diff-filter=d; git diff --name-only --cached --diff-filter=d; } | sort -u | grep '\.go$$' || true); \
		if [ -n "$$gofiles" ]; then echo "$$gofiles" | xargs gofmt -w; fi
	@echo "Formatting changed frontend code..."
	@webfiles=$$({ git diff --name-only --diff-filter=d -- '$(WEB_DIR)/'; git diff --name-only --cached --diff-filter=d -- '$(WEB_DIR)/'; } | sort -u | sed 's|^$(WEB_DIR)/||' | grep -E '\.(ts|tsx|js|jsx)$$' || true); \
		if [ -n "$$webfiles" ]; then cd $(WEB_DIR) && echo "$$webfiles" | xargs pnpm eslint --fix; fi

# Lint code (changed files only - fast feedback for AI iteration)
lint:
	@echo "Linting changed Go code..."
	golangci-lint run --new-from-merge-base=develop --timeout=5m
	@echo "Linting frontend..."
	cd $(WEB_DIR) && pnpm lint

# Full lint (entire codebase - use before commits/PRs)
lint-full:
	@echo "Linting entire Go codebase..."
	golangci-lint run --timeout=10m
	@echo "Linting frontend..."
	cd $(WEB_DIR) && pnpm lint

# Lint with JSON output (for AI agent consumption)
lint-json:
	@echo "Generating lint report..."
	golangci-lint run --new-from-merge-base=main --output.json.path=./lint-report.json --timeout=5m || true
	@echo "Lint report saved to lint-report.json"

# Lint with auto-fix where possible
lint-fix:
	@echo "Running linters with auto-fix..."
	golangci-lint run --fix --timeout=10m
	cd $(WEB_DIR) && pnpm lint --fix

# Modernize Go code (interface{} -> any, etc)
modern:
	@echo "Modernizing Go code..."
	go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest -fix -test ./...

# Install development dependencies
deps:
	@echo "Installing development dependencies..."
	go mod download
	cd $(WEB_DIR) && pnpm install

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
	@echo "  make build          - Build both frontend and backend"
	@echo "  make frontend       - Build frontend only"
	@echo "  make backend        - Build backend only"
	@echo "  make build/docker   - Build Docker image"
	@echo ""
	@echo "Development:"
	@echo "  make dev            - Run development servers (air + pnpm dev)"
	@echo "  make dev-backend    - Run backend with hot reload"
	@echo "  make dev-frontend   - Run frontend development server"
	@echo "  make dev-expose     - Run frontend dev server exposed on 0.0.0.0"
	@echo ""
	@echo "Testing:"
	@echo "  make test           - Run all tests with race detection"
	@echo "  make test-openapi   - Validate OpenAPI specification"
	@echo ""
	@echo "Linting:"
	@echo "  make lint           - Lint changed files only (fast, for iteration)"
	@echo "  make lint-full      - Lint entire codebase"
	@echo "  make lint-json      - Generate JSON lint report for AI agents"
	@echo "  make lint-fix       - Auto-fix linting issues where possible"
	@echo ""
	@echo "Formatting:"
	@echo "  make fmt            - Format changed files only (fast, for iteration)"
	@echo "  make modern         - Modernize Go code (interface{} -> any)"
	@echo ""
	@echo "Documentation:"
	@echo "  make docs-dev       - Run documentation development server"
	@echo "  make docs-build     - Build documentation for production"
	@echo ""
	@echo "Other:"
	@echo "  make themes-fetch   - Fetch premium themes from private repository"
	@echo "  make themes-clean   - Clean premium themes"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo "  make help           - Show this help message"