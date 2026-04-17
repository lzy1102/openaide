# OpenAIDE CLI Makefile

.PHONY: all build run install clean fmt lint help

# 版本号
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# 二进制文件名
BINARY_NAME=openaide

# 默认目标
all: build

# 编译
build:
	@echo "🔨 Building CLI..."
	go build -ldflags "-s -w -X main.Version=$(VERSION)" -o bin/$(BINARY_NAME) .

# 运行
run: build
	@echo "🚀 Running CLI..."
	./bin/$(BINARY_NAME)

# 安装到系统
install: build
	@echo "📦 Installing to /usr/local/bin..."
	sudo cp bin/$(BINARY_NAME) /usr/local/bin/
	@echo "✅ Installed. Run 'openaide' to start."

# 安装到用户目录
install-local: build
	@echo "📦 Installing to ~/.local/bin..."
	mkdir -p ~/.local/bin
	cp bin/$(BINARY_NAME) ~/.local/bin/
	@echo "✅ Installed. Add ~/.local/bin to your PATH if not already."

# 清理
clean:
	@echo "🧹 Cleaning..."
	rm -rf bin/
	go clean

# 代码格式化
fmt:
	@echo "✨ Formatting code..."
	go fmt ./...

# 代码检查
lint:
	@echo "🔍 Running linter..."
	@which golangci-lint > /dev/null || go install github.com/golangci-golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# 跨平台编译
build-all:
	@echo "🔨 Building for all platforms..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/$(BINARY_NAME)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o bin/$(BINARY_NAME)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o bin/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o bin/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/$(BINARY_NAME)-windows-amd64.exe .
	@echo "✅ All binaries built in bin/"

# 帮助
help:
	@echo "OpenAIDE CLI - Available commands:"
	@echo ""
	@echo "  make build        - Build CLI binary"
	@echo "  make run          - Build and run CLI"
	@echo "  make install      - Install to /usr/local/bin (requires sudo)"
	@echo "  make install-local- Install to ~/.local/bin"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make fmt          - Format code"
	@echo "  make lint         - Run linter"
	@echo "  make build-all    - Build for all platforms"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make install"
	@echo "  make build-all VERSION=v1.0.0"
	@echo ""
