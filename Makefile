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

## build: Build the server binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server
.PHONY: build

# =============================================================================
# Code Quality Targets
# =============================================================================

## test: Run tests with race detector and coverage
test:
	@echo "Running tests..."
	go test -race -cover ./...
.PHONY: test

## update-golden: Updates Golden Files for tests
update-golden:
	@echo "Updating Golden Test Files..."
	@go test ./internal/api/... --update-golden
.phony: update-golden

## test-e2e: Run Playwright E2E tests
test-e2e: build
	@echo "Running E2E tests..."
	npm run test:e2e
.PHONY: test-e2e

## lint: Run golangci-lint
lint:
	@echo "Running Go linter..."
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...
.PHONY: lint

## lint-css: Lint CSS files
lint-css:
	@echo "Running CSS linter..."
	@npx stylelint "static/css/**/*.css"
.PHONY: lint-css

## lint-js: Lint JavaScript files
lint-js:
	@echo "Running JS linter..."
	@npx eslint "static/js/**/*.js" --no-config-lookup \
		--rule 'no-unused-vars: [warn, {varsIgnorePattern: "^(switchInputMode|copyToClipboard|downloadAsFile|addJoinSpec|removeJoinSpec|updateJoinSpecState|loadExample|loadJoinExample)$$"}]' \
		--rule 'no-undef: off' 2>/dev/null || true
.PHONY: lint-js

## lint-yaml: Lint YAML configuration files
lint-yaml:
	@echo "Linting YAML files..."
	@command -v yamllint >/dev/null 2>&1 || { echo "yamllint not installed: brew install yamllint"; exit 1; }
	yamllint .github/workflows/ cloudbuild.yaml .golangci.yml
.PHONY: lint-yaml

## lint-json: Validate JSON configuration files
lint-json:
	@echo "Validating JSON files..."
	@npx jsonlint --quiet package.json
.PHONY: lint-json

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...
.PHONY: fmt

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...
.PHONY: vet

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	go mod tidy
.PHONY: tidy

## verify-templates: Verify Go templates parse correctly (via build)
verify-templates:
	@echo "Verifying templates..."
	@go build -o /dev/null ./cmd/server
	@echo "Templates OK"
.PHONY: verify-templates

## verify-static: Verify static assets exist and are valid
verify-static:
	@echo "Verifying static assets..."
	@test -f static/css/style.css || { echo "Missing: static/css/style.css"; exit 1; }
	@test -f static/js/app.js || { echo "Missing: static/js/app.js"; exit 1; }
	@echo "Static assets OK"
.PHONY: verify-static

## check: Run all checks before pushing (tidy, fmt, vet, lint, test, build, verify)
check: tidy fmt vet lint lint-css lint-js lint-yaml lint-json test build verify-templates verify-static
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
.PHONY: check

# =============================================================================
# Server Management
# =============================================================================

## run: Run server in foreground (blocking)
run: build
	$(BUILD_DIR)/$(BINARY_NAME)
.PHONY: run

## clean: Remove build artifacts and caches
clean:
	rm -rf $(BUILD_DIR)
	go clean -cache -testcache
.PHONY: clean

## docker-build: Build Docker image
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(BINARY_NAME):$(VERSION) .
.PHONY: docker-build

## start: Start server in background
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
.PHONY: start

## stop: Stop background server
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
.PHONY: stop

## restart: Restart the server
restart: stop start
.PHONY: restart

## status: Check if server is running
status:
	@if [ -f $(PID_FILE) ] && kill -0 $$(cat $(PID_FILE)) 2>/dev/null; then \
		echo "Server running (PID $$(cat $(PID_FILE))) - http://localhost:8080"; \
	else \
		echo "Server not running"; \
		rm -f $(PID_FILE) 2>/dev/null; \
	fi
.PHONY: status

## logs: Tail server logs
logs:
	@if [ -f $(LOG_FILE) ]; then \
		tail -f $(LOG_FILE); \
	else \
		echo "No log file found. Start server with 'make start'"; \
	fi
.PHONY: logs

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
.PHONY: help
