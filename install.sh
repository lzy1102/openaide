#!/bin/bash
#
# OpenAIDE 安装脚本
# 用户级安装，每个用户独立部署到 ~/.openaide
#
# 用法:
#   1. 从 GitHub 最新 Release 安装:
#      curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
#
#   2. 从指定版本安装:
#      VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
#
#   3. 本地编译安装 (代码已在 ~/.openaide):
#      bash ~/.openaide/install.sh --local
#

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 配置
GITHUB_REPO="lzy1102/openaide"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.openaide}"
VERSION="${VERSION:-latest}"
BIN_DIR="$INSTALL_DIR/bin"
DATA_DIR="$INSTALL_DIR/data"
LOG_DIR="$INSTALL_DIR/logs"
CONFIG_FILE="$INSTALL_DIR/config.yaml"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_ok() {
    echo -e "${GREEN}[OK]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查依赖
check_deps() {
    log_info "检查依赖..."
    local missing=()

    if ! command -v curl &> /dev/null && ! command -v wget &> /dev/null; then
        missing+=("curl 或 wget")
    fi
    if ! command -v tar &> /dev/null; then
        missing+=("tar")
    fi

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "缺少依赖: ${missing[*]}"
        exit 1
    fi
    log_ok "依赖检查通过"
}

# 获取最新版本号
get_latest_version() {
    local api_url="https://api.github.com/repos/$GITHUB_REPO/releases/latest"
    local version

    if command -v curl &> /dev/null; then
        version=$(curl -s "$api_url" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    else
        version=$(wget -qO- "$api_url" | grep '"tag_name":' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/')
    fi

    if [ -z "$version" ]; then
        echo ""
    else
        echo "$version"
    fi
}

# 下载 Release
download_release() {
    local ver="$1"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$ver/openaide-linux-amd64.tar.gz"
    local temp_file="/tmp/openaide-$ver.tar.gz"

    log_info "下载 OpenAIDE $ver..."
    log_info "URL: $download_url"

    if command -v curl &> /dev/null; then
        curl -fsSL -o "$temp_file" "$download_url" || {
            log_error "下载失败，请检查版本号和网络"
            exit 1
        }
    else
        wget -qO "$temp_file" "$download_url" || {
            log_error "下载失败，请检查版本号和网络"
            exit 1
        }
    fi

    log_ok "下载完成: $temp_file"
    echo "$temp_file"
}

# 安装二进制文件
install_binaries() {
    local archive="$1"

    log_info "安装到 $INSTALL_DIR..."

    # 创建目录
    mkdir -p "$BIN_DIR" "$DATA_DIR" "$LOG_DIR"

    # 解压
    tar -xzf "$archive" -C "$INSTALL_DIR" --strip-components=0 2>/dev/null || {
        local temp_extract="/tmp/openaide-extract-$$"
        mkdir -p "$temp_extract"
        tar -xzf "$archive" -C "$temp_extract"
        cp -r "$temp_extract"/* "$INSTALL_DIR/" 2>/dev/null || true
        rm -rf "$temp_extract"
    }

    # 确保可执行
    chmod +x "$BIN_DIR"/openaide-server "$BIN_DIR"/openaide-cli 2>/dev/null || true

    # 添加到 PATH (如果不存在)
    local shell_rc=""
    if [ -f "$HOME/.bashrc" ]; then
        shell_rc="$HOME/.bashrc"
    elif [ -f "$HOME/.zshrc" ]; then
        shell_rc="$HOME/.zshrc"
    fi

    if [ -n "$shell_rc" ] && ! grep -q "$BIN_DIR" "$shell_rc" 2>/dev/null; then
        echo "export PATH=\"$BIN_DIR:\$PATH\"" >> "$shell_rc"
        log_ok "已添加 $BIN_DIR 到 PATH (请运行: source $shell_rc)"
    fi

    # 创建快捷命令 openaide
    ln -sf "$BIN_DIR/openaide-cli" "$BIN_DIR/openaide" 2>/dev/null || true

    # 创建系统级软链接 (需要 root)
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-cli" /usr/local/bin/openaide-cli 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    # 立即生效当前会话
    export PATH="$BIN_DIR:$PATH"

    log_ok "二进制文件安装完成"
    log_info "快捷命令: openaide (已可用)"
}

# 初始化配置
init_config() {
    if [ -f "$CONFIG_FILE" ]; then
        log_warn "配置文件已存在，保留现有配置"
        return
    fi

    log_info "创建默认配置文件..."

    cat > "$CONFIG_FILE" << 'EOF'
# ============================================
# OpenAIDE 配置文件
# 格式: YAML (支持注释，一目了然)
# 文档: https://github.com/lzy1102/openaide/blob/master/docs/DEPLOYMENT.md
# ============================================

# 服务器配置
server:
  host: 0.0.0.0          # 监听地址，0.0.0.0 表示所有网卡
  port: 8080             # 服务端口
  mode: server           # 运行模式: server(服务端) / direct(直连) / tui(终端)

# LLM 大模型配置
llm:
  default_provider: deepseek   # 默认使用的提供商
  fallback_enabled: true       # 是否启用故障自动切换

  providers:
    # DeepSeek 示例配置 (OpenAI 兼容格式)
    - name: deepseek
      type: openai-compatible
      base_url: https://api.deepseek.com/v1
      api_key: "sk-请替换为你的DeepSeek API Key"   # <-- 必填: 替换为你的 API Key
      default_model: deepseek-v4-pro
      timeout: 60
      enabled: true

    # 其他提供商示例 (需要时取消注释)
    # - name: openai
    #   type: openai-compatible
    #   base_url: https://api.openai.com/v1
    #   api_key: "sk-你的OpenAI-Key"
    #   default_model: gpt-4o
    #   timeout: 60
    #   enabled: false

    # - name: ollama
    #   type: openai-compatible
    #   base_url: http://localhost:11434/v1
    #   api_key: "ollama"      # Ollama 不需要真实 key，但需占位
    #   default_model: qwen2.5
    #   timeout: 120
    #   enabled: false

# 记忆系统配置
memory:
  data_dir: ./data/memory      # 记忆数据存储目录
  max_items: 10000             # 最大记忆条目数
  compress_threshold: 100      # 触发压缩的上下文轮数

# 工具系统配置
tools:
  enabled: []                  # 启用的工具列表，空数组表示全部启用
  dangerous_tools:             # 危险工具列表，执行前会要求确认
    - execute_command
    - write_file
  max_execution_time: 30       # 工具最大执行时间(秒)

# AI Agent 内核配置
kernel:
  max_rounds: 10               # 单次对话最大 ReAct 轮数
  max_tokens: 4000             # 单次请求最大 Token 数
  system_prompt: "You are a helpful AI assistant."

# 存储配置
storage:
  data_dir: ./data             # 数据根目录
  index_dir: ./data/index      # 代码索引目录

# 日志配置
log:
  level: info                  # 日志级别: debug / info / warn / error
  format: json                 # 日志格式: json / text
EOF

    log_ok "配置文件创建完成: $CONFIG_FILE"
    log_warn "请编辑配置文件，设置你的 API Key!"
}

# 本地编译安装
local_build() {
    log_info "本地编译安装..."

    if [ ! -d "$INSTALL_DIR/backend" ]; then
        log_error "未找到 $INSTALL_DIR/backend 目录"
        log_info "请先克隆代码: git clone https://github.com/$GITHUB_REPO.git $INSTALL_DIR"
        exit 1
    fi

    cd "$INSTALL_DIR/backend"

    # 检查 Go
    if ! command -v go &> /dev/null; then
        log_error "未找到 Go，请先安装 Go 1.25+"
        exit 1
    fi

    log_info "编译服务器..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$BIN_DIR/openaide-server" ./cmd/server

    log_info "编译 CLI..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$BIN_DIR/openaide-cli" ./cmd/cli

    log_info "运行测试..."
    go test ./internal/... || log_warn "部分测试失败"

    # 创建快捷命令和软链接
    ln -sf "$BIN_DIR/openaide-cli" "$BIN_DIR/openaide" 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-cli" /usr/local/bin/openaide-cli 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    log_ok "本地编译完成"
}

# 创建启动脚本
create_start_script() {
    local start_script="$INSTALL_DIR/start.sh"

    cat > "$start_script" << EOF
#!/bin/bash
# OpenAIDE 启动脚本

cd "$INSTALL_DIR"
nohup "$BIN_DIR/openaide-server" -config "$CONFIG_FILE" > "$LOG_DIR/server.log" 2>&1 &
echo "OpenAIDE started on http://localhost:8080"
echo "PID: \$!"
echo "\$!" > "$INSTALL_DIR/openaide.pid"
EOF

    chmod +x "$start_script"

    # 创建停止脚本
    local stop_script="$INSTALL_DIR/stop.sh"
    cat > "$stop_script" << EOF
#!/bin/bash
# OpenAIDE 停止脚本

if [ -f "$INSTALL_DIR/openaide.pid" ]; then
    PID=\$(cat "$INSTALL_DIR/openaide.pid")
    if kill -0 "\$PID" 2>/dev/null; then
        kill "\$PID"
        echo "OpenAIDE stopped (PID: \$PID)"
    else
        echo "OpenAIDE is not running"
    fi
    rm -f "$INSTALL_DIR/openaide.pid"
else
    echo "PID file not found"
fi
EOF

    chmod +x "$stop_script"
    log_ok "启动/停止脚本已创建"
}

# 显示状态
show_status() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  OpenAIDE 安装完成!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "安装目录: $INSTALL_DIR"
    echo "二进制:   $BIN_DIR/openaide-server"
    echo "          $BIN_DIR/openaide-cli"
    echo "          $BIN_DIR/openaide (快捷命令)"
    echo "配置:     $CONFIG_FILE"
    echo "数据:     $DATA_DIR"
    echo "日志:     $LOG_DIR/server.log"
    echo ""
    echo "启动服务:"
    echo "  $INSTALL_DIR/start.sh"
    echo ""
    echo "停止服务:"
    echo "  $INSTALL_DIR/stop.sh"
    echo ""
    echo "查看日志:"
    echo "  tail -f $LOG_DIR/server.log"
    echo ""
    echo "使用 CLI:"
    echo "  openaide"
    echo ""
    echo -e "${YELLOW}重要: 请编辑 $CONFIG_FILE 配置你的 API Key!${NC}"
    echo ""
    echo "如果 openaide 命令找不到，请运行:"
    echo "  export PATH=\"$BIN_DIR:\$PATH\""
}

# 主函数
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  OpenAIDE 用户级安装脚本${NC}"
    echo -e "${BLUE}  安装目录: $INSTALL_DIR${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    check_deps

    # 解析参数
    local use_local=false
    for arg in "$@"; do
        case "$arg" in
            --local|-l)
                use_local=true
                ;;
            --version|-v)
                VERSION="$2"
                shift
                ;;
            --help|-h)
                echo "用法: $0 [选项]"
                echo ""
                echo "选项:"
                echo "  --local, -l      本地编译安装 (代码需在 $INSTALL_DIR)"
                echo "  --version, -v    指定版本号 (默认: latest)"
                echo "  --help, -h       显示帮助"
                echo ""
                echo "环境变量:"
                echo "  INSTALL_DIR      安装目录 (默认: \$HOME/.openaide)"
                echo "  VERSION          版本号 (默认: latest)"
                exit 0
                ;;
        esac
    done

    # 创建安装目录
    mkdir -p "$INSTALL_DIR"

    if [ "$use_local" = true ]; then
        # 本地编译模式
        local_build
    else
        # GitHub Release 模式
        if [ "$VERSION" = "latest" ]; then
            VERSION=$(get_latest_version)
        fi

        # 如果没有发布版本，自动切换到源码编译模式
        if [ -z "$VERSION" ]; then
            log_warn "未找到 GitHub Release，切换到源码编译模式"
            log_info "克隆源码到 $INSTALL_DIR ..."
            if [ -d "$INSTALL_DIR/.git" ]; then
                cd "$INSTALL_DIR" && git pull
            else
                rm -rf "$INSTALL_DIR"
                git clone "https://github.com/$GITHUB_REPO.git" "$INSTALL_DIR"
            fi
            local_build
        else
            log_info "安装版本: $VERSION"

            # 下载
            local archive
            archive=$(download_release "$VERSION")

            # 安装
            install_binaries "$archive"

            # 清理下载文件
            rm -f "$archive"
        fi
    fi

    # 初始化配置
    init_config

    # 创建启动脚本
    create_start_script

    # 显示状态
    show_status
}

main "$@"
