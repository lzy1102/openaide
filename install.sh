#!/bin/bash
#
# OpenAIDE 安装脚本
# 支持从 GitHub Release 下载安装或本地编译安装
#
# 用法:
#   1. 从 GitHub 最新 Release 安装 (推荐):
#      curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
#
#   2. 从指定版本安装:
#      VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
#
#   3. 本地编译安装 (代码已在 ~/.openaide):
#      bash ~/.openaide/install.sh --local
#
#   4. 使用本地 tar.gz 包安装:
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

# 检测当前平台
detect_platform() {
    local os arch
    case "$(uname -s)" in
        Linux)  os="linux" ;;
        Darwin) os="darwin" ;;
        *_NT*)  os="windows" ;;  # MSYS2/Cygwin/MinGW
        *)      os="linux" ;;    # fallback
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)            arch="amd64" ;; # fallback
    esac
    echo "$os-$arch"
}

# 下载 Release
download_release() {
    local ver="$1"
    local platform
    platform=$(detect_platform)
    local binary="openaide-$platform"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$ver/$binary"
    local temp_file="/tmp/openaide-$ver"

    log_info "下载 OpenAIDE $ver ($platform)..."
    log_info "URL: $download_url"

    if command -v curl &> /dev/null; then
        curl -fsSL --progress-bar -o "$temp_file" "$download_url" || {
            log_error "下载失败 (平台: $platform, 版本: $ver)"
            log_info "可用的编译平台: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64"
            log_info "如平台不匹配，请使用 --local 从源码编译"
            exit 1
        }
    else
        wget --progress=bar:force -qO "$temp_file" "$download_url" || {
            log_error "下载失败 (平台: $platform, 版本: $ver)"
            exit 1
        }
    fi

    log_ok "下载完成: $temp_file"
    echo "$temp_file"
}

# 安装二进制文件（支持 tar.gz 归档和裸二进制）
install_binary() {
    local source="$1"
    local is_archive="${2:-false}"

    log_info "从本地包安装: $source"

    if [ ! -f "$source" ]; then
        log_error "文件不存在: $source"
        exit 1
    fi

    # 创建目录
    mkdir -p "$BIN_DIR" "$DATA_DIR" "$LOG_DIR"

    if [ "$is_archive" = "true" ]; then
        log_info "解压到 $INSTALL_DIR..."
        tar -xzf "$source" -C "$INSTALL_DIR" --overwrite
    else
        cp "$source" "$BIN_DIR/openaide"
        chmod +x "$BIN_DIR/openaide"
        log_info "安装 CLI: $BIN_DIR/openaide"
    fi

    # 确保可执行
    chmod +x "$BIN_DIR"/openaide-server "$BIN_DIR"/openaide 2>/dev/null || true

    # 创建系统级软链接 (需要 root)
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    # 添加到 PATH
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

    export PATH="$BIN_DIR:$PATH"

    log_ok "安装完成"
}

# 从 GitHub Release 安装
install_from_release() {
    local ver="$1"

    if [ "$ver" = "latest" ]; then
        ver=$(get_latest_version)
        if [ -z "$ver" ]; then
            log_warn "未找到 GitHub Release，切换到源码编译模式"
            local_build
            return
        fi
    fi

    log_info "安装版本: $ver"

    # 下载
    local archive
    archive=$(download_release "$ver")

    # 安装
    install_binary "$archive" "false"

    # 清理
    rm -f "$archive"
}

# 初始化配置
init_config() {
    if [ -f "$CONFIG_FILE" ]; then
        log_warn "配置文件已存在，保留现有配置"
        return
    fi

    log_info "创建默认配置文件..."

    cat > "$CONFIG_FILE" << EOF
# 只需修改下面 llm 的 api_key 即可使用，其他均有默认值

server:
  host: "0.0.0.0"
  port: 8080

llm:
  default_provider: "deepseek"
  providers:
    - name: "deepseek"
      type: "openai"
      base_url: "https://api.deepseek.com/v1"
      api_key: "sk-请替换为你的API Key"     # ← 必填
      default_model: "deepseek-v4-pro"
      enabled: true
    # - name: "openai"
    #   type: "openai"
    #   base_url: "https://api.openai.com/v1"
    #   api_key: "sk-xxx"
    #   default_model: "gpt-4o-mini"
    #   enabled: true
    # - name: "claude"
    #   type: "anthropic"
    #   base_url: "https://api.anthropic.com"
    #   api_key: "sk-ant-xxx"
    #   default_model: "claude-sonnet-4-20250514"
    #   enabled: true

memory:
  data_dir: "$INSTALL_DIR/data/memory"

kernel:
  max_rounds: 10

storage:
  data_dir: "$INSTALL_DIR/data"

browser:
  enabled: false

log:
  level: "info"
EOF

    log_ok "配置文件创建完成: $CONFIG_FILE"
    log_warn "请编辑配置文件，设置你的 API Key!"
}

# 本地编译安装
local_build() {
    log_info "本地编译安装..."

    if [ ! -d "$INSTALL_DIR/backend" ]; then
        log_info "克隆源码到 $INSTALL_DIR ..."
        git clone "https://github.com/$GITHUB_REPO.git" "$INSTALL_DIR" 2>/dev/null || {
            log_error "克隆失败，请检查网络或手动克隆"
            exit 1
        }
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
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$BIN_DIR/openaide" ./cmd/cli

    log_info "运行测试..."
    go test ./internal/... || log_warn "部分测试失败"

    # 创建快捷命令和软链接
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    # 添加到 PATH
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

    export PATH="$BIN_DIR:$PATH"

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

    # 复制更新脚本（如果存在）
    if [ -f "$INSTALL_DIR/scripts/update.sh" ]; then
        chmod +x "$INSTALL_DIR/scripts/update.sh"
    fi

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
    echo "          $BIN_DIR/openaide"
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
    echo ""
    echo "更新到最新版本:"
    echo "  bash $INSTALL_DIR/scripts/update.sh"
}

# 主函数
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  OpenAIDE 安装脚本${NC}"
    echo -e "${BLUE}  安装目录: $INSTALL_DIR${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    check_deps

    # 解析参数
    local use_local=false
    local archive_path=""
    for arg in "$@"; do
        case "$arg" in
            --local|-l)
                use_local=true
                ;;
            --archive|-a)
                archive_path="$2"
                shift
                ;;
            --version|-v)
                VERSION="$2"
                shift
                ;;
            --help|-h)
                echo "用法: $0 [选项]"
                echo ""
                echo "选项:"
                echo "  --local, -l           本地编译安装 (代码需在 $INSTALL_DIR)"
                echo "  --version, -v         指定版本号 (默认: latest)"
                echo "  --help, -h            显示帮助"
                echo ""
                echo "环境变量:"
                echo "  INSTALL_DIR           安装目录 (默认: \$HOME/.openaide)"
                echo "  VERSION               版本号 (默认: latest)"
                echo ""
                echo "示例:"
                echo "  $0                              # 从 GitHub Release 安装最新版"
                echo "  VERSION=v1.0.0 $0               # 安装指定版本"
                echo "  $0 --local                      # 本地编译安装"
                exit 0
                ;;
        esac
    done

    # 创建安装目录
    mkdir -p "$INSTALL_DIR"

    if [ -n "$archive_path" ]; then
        # 从本地存档安装
        install_binary "$archive_path" "true"
    elif [ "$use_local" = true ]; then
        # 本地编译模式
        local_build
    else
        # GitHub Release 模式
        install_from_release "$VERSION"
    fi

    # 初始化配置
    init_config

    # 创建启动脚本
    create_start_script

    # 显示状态
    show_status
}

main "$@"
