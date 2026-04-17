# OpenAIDE Backend Makefile

.PHONY: all build run test clean install deps fmt lint config dev
.PHONY: docker-build docker-run docker-stop docker-logs docker-shell
.PHONY: docker-compose-up docker-compose-down docker-compose-logs
.PHONY: docker-clean docker-push

# 版本号
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# 默认目标
all: deps build

# 安装依赖
deps:
	@echo "📦 Installing dependencies..."
	go mod tidy
	go mod download

# 编译
build:
	@echo "🔨 Building..."
	go build -o bin/openaide-server ./src

# 运行
run:
	@echo "🚀 Starting server..."
	go run ./src/main.go

# 运行测试
test:
	@echo "🧪 Running tests..."
	go test -v ./...

# 运行测试并生成覆盖率
test-coverage:
	@echo "📊 Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# 清理
clean:
	@echo "🧹 Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean

# 代码格式化
fmt:
	@echo "✨ Formatting code..."
	go fmt ./...

# 代码检查
lint:
	@echo "🔍 Running linter..."
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run ./...

# 创建配置文件
config:
	@if [ ! -f config.json ]; then \
		echo "📝 Creating config.json from example..."; \
		cp config.example.json config.json; \
		echo "⚠️  Please edit config.json and add your API keys!"; \
	else \
		echo "✅ config.json already exists"; \
	fi

# 开发模式 (热重载需要安装 air)
dev:
	@which air > /dev/null || go install github.com/cosmtrek/air@latest
	air

# ===========================================
# Docker 命令
# ===========================================

# 构建 Docker 镜像
docker-build:
	@echo "🐳 Building Docker image (version: $(VERSION))..."
	docker build -t openaide:$(VERSION) -t openaide:latest .

# 运行 Docker 容器
docker-run:
	@echo "🐳 Running Docker container..."
	docker run -d --name openaide \
		-p 8081:8081 \
		-v $(PWD)/data:/app/data \
		-v $(PWD)/config.json:/app/config.json:ro \
		--restart unless-stopped \
		openaide:latest
	@echo "✅ Container started. API available at http://localhost:8081"

# 停止 Docker 容器
docker-stop:
	@echo "🛑 Stopping Docker container..."
	docker stop openaide 2>/dev/null || true
	docker rm openaide 2>/dev/null || true
	@echo "✅ Container stopped and removed"

# 查看 Docker 日志
docker-logs:
	@echo "📋 Showing Docker logs (Ctrl+C to exit)..."
	docker logs -f openaide

# 进入 Docker 容器
docker-shell:
	@echo "🐚 Entering Docker container..."
	docker exec -it openaide /bin/sh

# ===========================================
# Docker Compose 命令
# ===========================================

# 启动所有服务 (开发环境)
docker-compose-up:
	@echo "🚀 Starting services with Docker Compose..."
	docker-compose up -d
	@echo "✅ Services started"
	@echo "   API: http://localhost:8081"
	@echo "   Health: http://localhost:8081/health"

# 启动所有服务 (生产环境)
docker-compose-prod:
	@echo "🚀 Starting production services..."
	docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
	@echo "✅ Production services started"

# 停止所有服务
docker-compose-down:
	@echo "🛑 Stopping all services..."
	docker-compose down
	@echo "✅ Services stopped"

# 查看所有服务日志
docker-compose-logs:
	@echo "📋 Showing all service logs (Ctrl+C to exit)..."
	docker-compose logs -f

# 重启所有服务
docker-compose-restart:
	@echo "🔄 Restarting all services..."
	docker-compose restart
	@echo "✅ Services restarted"

# 查看服务状态
docker-compose-ps:
	@echo "📊 Service status:"
	docker-compose ps

# ===========================================
# Docker 清理命令
# ===========================================

# 清理 Docker 资源
docker-clean:
	@echo "🧹 Cleaning Docker resources..."
	docker-compose down -v --remove-orphans 2>/dev/null || true
	docker rmi openaide:latest openaide:$(VERSION) 2>/dev/null || true
	@echo "✅ Docker resources cleaned"

# 完全清理 (包括未使用的镜像和卷)
docker-prune:
	@echo "⚠️  This will remove all unused Docker resources!"
	@read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		docker system prune -a -f --volumes; \
	fi

# ===========================================
# Docker 推送命令 (用于 CI/CD)
# ===========================================

# 推送镜像到仓库
docker-push:
	@echo "📤 Pushing Docker image..."
	docker tag openaide:$(VERSION) $(REGISTRY)/openaide:$(VERSION)
	docker tag openaide:$(VERSION) $(REGISTRY)/openaide:latest
	docker push $(REGISTRY)/openaide:$(VERSION)
	docker push $(REGISTRY)/openaide:latest
	@echo "✅ Image pushed to $(REGISTRY)"

# ===========================================
# 帮助
# ===========================================

help:
	@echo "OpenAIDE Backend - Available commands:"
	@echo ""
	@echo "Development:"
	@echo "  make deps          - Install dependencies"
	@echo "  make build         - Build the server"
	@echo "  make run           - Run the server"
	@echo "  make test          - Run tests"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make fmt           - Format code"
	@echo "  make lint          - Run linter"
	@echo "  make config        - Create config.json from example"
	@echo "  make dev           - Run in development mode (requires air)"
	@echo ""
	@echo "Docker (single container):"
	@echo "  make docker-build  - Build Docker image"
	@echo "  make docker-run    - Run Docker container"
	@echo "  make docker-stop   - Stop and remove container"
	@echo "  make docker-logs   - Show container logs"
	@echo "  make docker-shell  - Enter container shell"
	@echo ""
	@echo "Docker Compose:"
	@echo "  make docker-compose-up      - Start all services (dev)"
	@echo "  make docker-compose-prod    - Start all services (prod)"
	@echo "  make docker-compose-down    - Stop all services"
	@echo "  make docker-compose-logs    - Show all logs"
	@echo "  make docker-compose-restart - Restart all services"
	@echo "  make docker-compose-ps      - Show service status"
	@echo ""
	@echo "Docker Cleanup:"
	@echo "  make docker-clean  - Remove containers and images"
	@echo "  make docker-prune  - Remove all unused Docker resources"
	@echo ""
	@echo "Variables:"
	@echo "  VERSION=xxx        - Set version tag (default: git tag or 'latest')"
	@echo "  REGISTRY=xxx       - Set Docker registry for push"
	@echo ""
	@echo "Examples:"
	@echo "  make docker-build VERSION=v1.0.0"
	@echo "  make docker-push REGISTRY=ghcr.io/myuser"
	@echo ""
