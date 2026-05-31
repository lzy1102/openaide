# OpenAIDE Backend Makefile

.PHONY: all build run test clean fmt lint docker-build docker-run docker-stop

# 版本号
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# 默认目标
all: build

# 安装依赖
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Output directory (relative to project root)
BIN_DIR = ../bin

# Compiler flags
LDFLAGS = -s -w -X main.Version=$(VERSION) -X main.BuildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# ── Build ────────────────────────────────────────

build:
	@echo "Building..."
	@rm -rf cmd/server/frontend 2>/dev/null; cp -r ../frontend cmd/server/frontend
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server ./cmd/server
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide ./cmd/cli
	@rm -rf cmd/server/frontend

install: build
	@mkdir -p ~/.openaide/bin
	@cp $(BIN_DIR)/openaide ~/.openaide/bin/openaide
	@cp $(BIN_DIR)/openaide-server ~/.openaide/bin/openaide-server
	@for d in ~/.local/bin /usr/local/bin; do \
		if [ -d "$$d" ]; then \
			cp $(BIN_DIR)/openaide "$$d/openaide" 2>/dev/null || true; \
			echo "Also installed to $$d/openaide"; \
		fi; \
	done
	@echo "Installed to ~/.openaide/bin/"
	@echo "Run: export PATH=\$$HOME/.openaide/bin:\$$PATH"

build-all:
	@rm -rf cmd/server/frontend 2>/dev/null; cp -r ../frontend cmd/server/frontend
	$(MAKE) build-linux-amd64 build-linux-arm64 build-windows-amd64 build-darwin-amd64 build-darwin-arm64
	@rm -rf cmd/server/frontend

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-linux-amd64 ./cmd/cli
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server-linux-amd64 ./cmd/server

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-linux-arm64 ./cmd/cli
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server-linux-arm64 ./cmd/server

build-windows-amd64:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-windows-amd64.exe ./cmd/cli
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server-windows-amd64.exe ./cmd/server

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-darwin-amd64 ./cmd/cli
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server-darwin-amd64 ./cmd/server

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-darwin-arm64 ./cmd/cli
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/openaide-server-darwin-arm64 ./cmd/server

# ── Dev ──────────────────────────────────────────

run:
	go run ./cmd/server

test:
	go test -v ./internal/...

test-coverage:
	go test -v -coverprofile=$(BIN_DIR)/coverage.out ./internal/...
	go tool cover -html=$(BIN_DIR)/coverage.out -o $(BIN_DIR)/coverage.html

clean:
	rm -rf $(BIN_DIR)/
	rm -f coverage.out coverage.html
	find cmd -type f -executable -delete 2>/dev/null || true
	go clean

fmt:
	go fmt ./...

lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# 代码检查
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# Docker 构建
docker-build:
	@echo "Building Docker image..."
	docker build -t openaide:$(VERSION) -t openaide:latest .

# Docker 运行
docker-run:
	@echo "Running Docker container..."
	docker run -d --name openaide \
		-p 8080:8080 \
		-v openaide-data:/opt/openaide/data \
		--restart unless-stopped \
		openaide:latest

# Docker 停止
docker-stop:
	@echo "Stopping Docker container..."
	docker stop openaide 2>/dev/null || true
	docker rm openaide 2>/dev/null || true

# 帮助
help:
	@echo "OpenAIDE Backend - Available commands:"
	@echo ""
	@echo "Development:"
	@echo "  make deps          - Install dependencies"
	@echo "  make build         - Build server and CLI"
	@echo "  make run           - Run the server"
	@echo "  make test          - Run tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make fmt           - Format code"
	@echo "  make lint          - Run linter"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build  - Build Docker image"
	@echo "  make docker-run    - Run Docker container"
	@echo "  make docker-stop   - Stop and remove container"
