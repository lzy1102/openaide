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

# 编译
build:
	@echo "Building..."
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide-server ./cmd/server
	CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide-cli ./cmd/cli

# 运行
run:
	@echo "Starting server..."
	go run ./cmd/server

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./internal/...

# 运行测试并生成覆盖率
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 清理
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean

# 代码格式化
fmt:
	@echo "Formatting..."
	go fmt ./...

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
