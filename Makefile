.PHONY: build test lint run clean docker-build start stop restart status dev tidy

VERSION ?= dev
BINARY_NAME := oastools-web
BUILD_DIR := bin
PID_FILE := $(BUILD_DIR)/.server.pid
LOG_FILE := $(BUILD_DIR)/server.log

# Build flags
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

build:
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

test:
	go test -race -cover ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed"; exit 1; }
	golangci-lint run ./...

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

# Development helpers
dev:
	@command -v air >/dev/null 2>&1 || { echo "air not installed: go install github.com/air-verse/air@latest"; exit 1; }
	air

tidy:
	go mod tidy
