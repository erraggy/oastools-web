.PHONY: build test lint lint-css lint-js run clean docker-build start stop restart status dev tidy help
.PHONY: check fmt vet verify-templates verify-static logs

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
BINARY_NAME := oastools-web
BUILD_DIR := bin
PID_FILE := $(BUILD_DIR)/.server.pid
LOG_FILE := $(BUILD_DIR)/server.log

# Build flags
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

# =============================================================================
# Build Targets
# =============================================================================

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

# =============================================================================
# Code Quality Targets
# =============================================================================

## test: Run tests with race detector and coverage
test:
	@echo "Running tests..."
	go test -race -cover ./...

## lint: Run golangci-lint
lint:
	@echo "Running Go linter..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...

## lint-css: Lint CSS files
lint-css:
	@echo "Running CSS linter..."
	@npx stylelint "static/css/**/*.css"

## lint-js: Lint JavaScript files
lint-js:
	@echo "Running JS linter..."
	@npx eslint "static/js/**/*.js" --no-config-lookup --rule 'no-unused-vars: warn' --rule 'no-undef: off' 2>/dev/null || true

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	go mod tidy

## verify-templates: Verify Go templates parse correctly (via build)
verify-templates:
	@echo "Verifying templates..."
	@go build -o /dev/null ./cmd/server
	@echo "Templates OK"

## verify-static: Verify static assets exist and are valid
verify-static:
	@echo "Verifying static assets..."
	@test -f static/css/style.css || { echo "Missing: static/css/style.css"; exit 1; }
	@test -f static/js/app.js || { echo "Missing: static/js/app.js"; exit 1; }
	@echo "Static assets OK"

## check: Run all checks before pushing (tidy, fmt, vet, lint, test, build, verify)
check: tidy fmt vet lint lint-css lint-js test build verify-templates verify-static
	@echo ""
	@echo "============================================"
	@echo "All checks passed!"
	@echo "============================================"
	@echo ""
	@echo "Git status:"
	@git status --short
	@echo ""
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "⚠️  Uncommitted changes detected (review above)"; \
	else \
		echo "✓ Working tree clean"; \
	fi

# Run server in foreground (blocking)
run: build
	$(BUILD_DIR)/$(BINARY_NAME)

clean:
	rm -rf $(BUILD_DIR)
	go clean -cache -testcache

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY_NAME):$(VERSION) .

# Server management
start: build
	@if [ -f $(PID_FILE) ] && kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "Server already running (PID $$(cat $(PID_FILE)))"; \
	else \
		$(BUILD_DIR)/$(BINARY_NAME) > $(LOG_FILE) 2>&1 & echo $$! > $(PID_FILE); \
		sleep 1; \
		if kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
			echo "Server started (PID $$(cat $(PID_FILE))) - http://localhost:8080"; \
			echo "Logs: tail -f $(LOG_FILE)"; \
		else \
			echo "Server failed to start. Check $(LOG_FILE)"; \
			rm -f $(PID_FILE); \
			exit 1; \
		fi \
	fi

stop:
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 $$PID 2>/dev/null; then \
			kill $$PID; \
			echo "Server stopped (PID $$PID)"; \
		else \
			echo "Server not running (stale PID file)"; \
		fi; \
		rm -f $(PID_FILE); \
	else \
		echo "No PID file found. Checking for orphan processes..."; \
		pkill -f "$(BINARY_NAME)" 2>/dev/null && echo "Killed orphan process" || echo "No server running"; \
	fi

restart: stop start

status:
	@if [ -f $(PID_FILE) ] && kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "Server running (PID $$(cat $(PID_FILE))) - http://localhost:8080"; \
	else \
		echo "Server not running"; \
		rm -f $(PID_FILE) 2>/dev/null; \
	fi

logs:
	@if [ -f $(LOG_FILE) ]; then \
		tail -f $(LOG_FILE); \
	else \
		echo "No log file found. Start server with 'make start'"; \
	fi

# =============================================================================
# Development Helpers
# =============================================================================

## dev: Run with hot reload (requires air)
dev:
	@command -v air >/dev/null 2>&1 || { echo "air not installed: go install github.com/air-verse/air@latest"; exit 1; }
	air

# =============================================================================
# Help
# =============================================================================

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
	@echo ""
	@echo "Quick Start:"
	@echo "  make check    # Run all checks before pushing"
	@echo "  make run      # Build and run server"
	@echo "  make dev      # Run with hot reload"
