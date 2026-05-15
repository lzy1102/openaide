# OpenAIDE 项目构建脚本

.PHONY: all build clean help

# 版本号
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# 默认目标
all: build

# 构建后端
build:
	@echo "Building OpenAIDE..."
	cd backend && $(MAKE) build

# 运行测试
test:
	@echo "Running tests..."
	cd backend && $(MAKE) test

# 清理
clean:
	@echo "Cleaning..."
	cd backend && $(MAKE) clean

# 帮助
help:
	@echo "OpenAIDE - Build commands:"
	@echo ""
	@echo "  make build    - Build backend server and CLI"
	@echo "  make test     - Run tests"
	@echo "  make clean    - Clean build artifacts"
	@echo ""
	@echo "See backend/Makefile for more commands"
