# OpenAIDE 项目构建脚本

.PHONY: all build clean help
.PHONY: build-backend build-cli build-frontend
.PHONY: install install-system install-standalone

# 版本号
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# 部署目录
DIST_DIR := dist
BIN_DIR := $(DIST_DIR)/bin
FRONTEND_DIR := $(DIST_DIR)/frontend
DATA_DIR := $(DIST_DIR)/data

# 默认目标
all: build

# ===========================================
# 构建命令
# ===========================================

build: build-backend build-cli build-frontend
	@cp .openaide.example $(DIST_DIR)/ 2>/dev/null || true
	@mkdir -p $(DATA_DIR)/{database,knowledge,uploads}
	@cp scripts/install.sh $(DIST_DIR)/ 2>/dev/null || true
	@cp scripts/uninstall.sh $(DIST_DIR)/ 2>/dev/null || true
	@chmod +x $(DIST_DIR)/install.sh $(DIST_DIR)/uninstall.sh 2>/dev/null || true
	@echo ""
	@echo -e "\033[32m✅ 构建完成！\033[0m 输出目录: $(DIST_DIR)/"
	@echo ""
	@echo "dist/"
	@echo "├── bin/"
	@echo "│   ├── openaide          # CLI 工具"
	@echo "│   └── openaide-server   # 后端服务"
	@echo "├── frontend/             # 前端文件"
	@echo "├── data/                 # 数据目录"
	@echo "│   ├── database/         # 数据库"
	@echo "│   ├── knowledge/        # 知识库"
	@echo "│   └── uploads/          # 上传文件"
	@echo "├── .openaide.example     # 配置示例"
	@echo "├── install.sh            # 安装脚本"
	@echo "└── uninstall.sh          # 卸载脚本"
	@echo ""

build-backend:
	@echo -e "\033[33m🔨 构建后端服务...\033[0m"
	@mkdir -p $(BIN_DIR)
	cd backend && CGO_ENABLED=1 go build -ldflags "-s -w -X main.Version=$(VERSION)" -o ../$(BIN_DIR)/openaide-server ./src

build-cli:
	@echo -e "\033[33m🔨 构建 CLI 工具...\033[0m"
	@mkdir -p $(BIN_DIR)
	cd terminal && go build -ldflags "-s -w -X main.Version=$(VERSION)" -o ../$(BIN_DIR)/openaide .

build-frontend:
	@echo -e "\033[33m📦 复制前端文件...\033[0m"
	@mkdir -p $(FRONTEND_DIR)
	@cp -r frontend/* $(FRONTEND_DIR)/ 2>/dev/null || echo "⚠️  前端目录不存在，跳过"

# ===========================================
# 安装命令
# ===========================================

# 默认安装 (FHS 标准布局)
install: build
	@echo -e "\033[36m📦 安装到系统 (FHS 标准布局)...\033[0m"
	cd $(DIST_DIR) && sudo ./install.sh

# 独立安装 (/opt 布局)
install-standalone: build
	@echo -e "\033[36m📦 安装到 /opt (独立布局)...\033[0m"
	cd $(DIST_DIR) && sudo INSTALL_VERSION=standalone ./install.sh

# ===========================================
# 运行命令
# ===========================================

run-backend: build-backend
	@echo -e "\033[36m🚀 启动后端服务...\033[0m"
	./$(BIN_DIR)/openaide-server

run-cli: build-cli
	@echo -e "\033[36m🚀 启动 CLI...\033[0m"
	./$(BIN_DIR)/openaide

# ===========================================
# 打包命令
# ===========================================

package: build
	@echo -e "\033[36m📦 打包发布版本...\033[0m"
	tar -czvf openaide-$(VERSION)-linux-amd64.tar.gz -C $(DIST_DIR) .
	@ls -lh openaide-$(VERSION)-linux-amd64.tar.gz

# ===========================================
# 部署命令
# ===========================================

deploy: package
	@if [ -z "$(HOST)" ]; then \
		echo "❌ 请指定 HOST 参数"; \
		echo "示例: make deploy HOST=root@192.168.1.100"; \
		exit 1; \
	fi
	@echo "📤 上传到 $(HOST)..."
	scp openaide-$(VERSION)-linux-amd64.tar.gz $(HOST):/tmp/
	ssh $(HOST) "cd /tmp && tar -xzf openaide-$(VERSION)-linux-amd64.tar.gz && sudo ./install.sh && rm -rf openaide-*"
	@echo "✅ 部署完成！"

# ===========================================
# 清理命令
# ===========================================

clean:
	@echo -e "\033[31m🧹 清理构建产物...\033[0m"
	rm -rf $(DIST_DIR)
	rm -f openaide-*.tar.gz
	cd backend && go clean
	cd terminal && go clean

# ===========================================
# 帮助
# ===========================================

help:
	@echo ""
	@echo -e "\033[36m╔══════════════════════════════════════════════════════════╗\033[0m"
	@echo -e "\033[36m║              OpenAIDE - 构建命令                          ║\033[0m"
	@echo -e "\033[36m╚══════════════════════════════════════════════════════════╝\033[0m"
	@echo ""
	@echo -e "\033[33m构建:\033[0m"
	@echo "  make build           构建所有组件"
	@echo "  make build-backend   仅构建后端"
	@echo "  make build-cli       仅构建 CLI"
	@echo ""
	@echo -e "\033[33m安装:\033[0m"
	@echo "  make install         安装 (FHS 标准布局)"
	@echo "  make install-standalone  安装 (/opt 独立布局)"
	@echo ""
	@echo -e "\033[33m运行:\033[0m"
	@echo "  make run-backend     启动后端服务"
	@echo "  make run-cli         启动 CLI"
	@echo ""
	@echo -e "\033[33m打包:\033[0m"
	@echo "  make package         打包为 tar.gz"
	@echo "  make deploy HOST=x   部署到服务器"
	@echo ""
	@echo -e "\033[33m清理:\033[0m"
	@echo "  make clean           清理所有构建产物"
	@echo ""
	@echo -e "\033[33m安装后目录结构 (FHS 标准):\033[0m"
	@echo "  /usr/bin/openaide          CLI 命令"
	@echo "  /usr/bin/openaide-server   服务程序"
	@echo "  /etc/openaide/config       配置文件"
	@echo "  /var/lib/openaide/         数据目录"
	@echo "  /var/log/openaide/         日志目录"
	@echo ""
