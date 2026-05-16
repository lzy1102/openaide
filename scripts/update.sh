#!/bin/bash
#
# OpenAIDE 快速更新脚本
# 用于在服务器上快速更新到最新版本
#
# 用法:
#   1. 更新到最新 Release:
#      bash ~/.openaide/scripts/update.sh
#
#   2. 更新到指定版本:
#      VERSION=v1.0.0 bash ~/.openaide/scripts/update.sh
#
#   3. 本地重新编译:
#      bash ~/.openaide/scripts/update.sh --local
#
#   4. 仅重启服务:
#      bash ~/.openaide/scripts/update.sh --restart
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
BACKUP_DIR="$INSTALL_DIR/.backup-$(date +%Y%m%d-%H%M%S)"

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
        log_warn "无法获取最新版本"
        echo ""
    else
        echo "$version"
    fi
}

# 下载 Release
download_release() {
    local ver="$1"
    local download_url="https://github.com/$GITHUB_REPO/releases/download/$ver/openaide-linux-amd64.tar.gz"
    local temp_file="/tmp/openaide-update-$ver.tar.gz"

    log_info "下载 OpenAIDE $ver..."

    if command -v curl &> /dev/null; then
        curl -fsSL -o "$temp_file" "$download_url" || {
            log_error "下载失败"
            exit 1
        }
    else
        wget -qO "$temp_file" "$download_url" || {
            log_error "下载失败"
            exit 1
        }
    fi

    echo "$temp_file"
}

# 备份当前版本
backup_current() {
    log_info "备份当前版本到 $BACKUP_DIR..."
    mkdir -p "$BACKUP_DIR"
    if [ -d "$BIN_DIR" ]; then
        cp -r "$BIN_DIR" "$BACKUP_DIR/"
    fi
    if [ -f "$INSTALL_DIR/config.yaml" ]; then
        cp "$INSTALL_DIR/config.yaml" "$BACKUP_DIR/"
    fi
    log_ok "备份完成"
}

# 从 Release 更新
update_from_release() {
    local ver="$1"

    if [ "$ver" = "latest" ]; then
        ver=$(get_latest_version)
        if [ -z "$ver" ]; then
            log_error "无法获取最新版本号"
            exit 1
        fi
    fi

    log_info "更新到版本: $ver"

    # 下载
    local archive
    archive=$(download_release "$ver")

    # 备份
    backup_current

    # 解压
    log_info "安装新版本..."
    cd "$INSTALL_DIR"
    tar -xzf "$archive" --overwrite
    chmod +x "$BIN_DIR"/openaide-server "$BIN_DIR"/openaide-cli 2>/dev/null || true

    # 创建快捷命令
    ln -sf "$BIN_DIR/openaide-cli" "$BIN_DIR/openaide" 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-cli" /usr/local/bin/openaide-cli 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    # 清理
    rm -f "$archive"

    log_ok "更新完成"
}

# 本地重新编译
update_local() {
    log_info "本地重新编译..."

    if [ ! -d "$INSTALL_DIR/backend" ]; then
        log_error "未找到 $INSTALL_DIR/backend 目录"
        exit 1
    fi

    if ! command -v go &> /dev/null; then
        log_error "未找到 Go"
        exit 1
    fi

    # 备份
    backup_current

    cd "$INSTALL_DIR/backend"

    log_info "编译服务器..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$BIN_DIR/openaide-server" ./cmd/server

    log_info "编译 CLI..."
    CGO_ENABLED=0 go build -ldflags "-s -w" -o "$BIN_DIR/openaide-cli" ./cmd/cli

    # 创建快捷命令
    ln -sf "$BIN_DIR/openaide-cli" "$BIN_DIR/openaide" 2>/dev/null || true

    log_ok "本地编译完成"
}

# 重启服务
restart_service() {
    log_info "重启服务..."

    # 停止旧服务
    if [ -f "$INSTALL_DIR/openaide.pid" ]; then
        local PID
        PID=$(cat "$INSTALL_DIR/openaide.pid")
        if kill -0 "$PID" 2>/dev/null; then
            log_info "停止旧服务 (PID: $PID)..."
            kill "$PID"
            sleep 2
        fi
    fi

    # 启动新服务
    cd "$INSTALL_DIR"
    nohup "$BIN_DIR/openaide-server" -config "$INSTALL_DIR/config.yaml" > "$INSTALL_DIR/logs/server.log" 2>&1 &
    local NEW_PID=$!
    echo "$NEW_PID" > "$INSTALL_DIR/openaide.pid"

    sleep 2
    if kill -0 "$NEW_PID" 2>/dev/null; then
        log_ok "服务已启动 (PID: $NEW_PID)"
    else
        log_error "服务启动失败，查看日志: $INSTALL_DIR/logs/server.log"
        exit 1
    fi
}

# 回滚到上一个版本
rollback() {
    log_info "查找备份..."

    local latest_backup
    latest_backup=$(ls -td "$INSTALL_DIR"/.backup-* 2>/dev/null | head -1)

    if [ -z "$latest_backup" ]; then
        log_error "未找到备份"
        exit 1
    fi

    log_info "回滚到: $latest_backup"

    # 停止服务
    if [ -f "$INSTALL_DIR/openaide.pid" ]; then
        local PID
        PID=$(cat "$INSTALL_DIR/openaide.pid")
        if kill -0 "$PID" 2>/dev/null; then
            kill "$PID"
            sleep 2
        fi
    fi

    # 恢复备份
    cp -r "$latest_backup/bin" "$INSTALL_DIR/"
    if [ -f "$latest_backup/config.yaml" ]; then
        cp "$latest_backup/config.yaml" "$INSTALL_DIR/"
    fi

    # 重新创建软链接
    ln -sf "$BIN_DIR/openaide-cli" "$BIN_DIR/openaide" 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-server" /usr/local/bin/openaide-server 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide-cli" /usr/local/bin/openaide-cli 2>/dev/null || true
    ln -sf "$BIN_DIR/openaide" /usr/local/bin/openaide 2>/dev/null || true

    # 重启服务
    restart_service

    log_ok "回滚完成"
}

# 显示状态
show_status() {
    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  OpenAIDE 状态${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "安装目录: $INSTALL_DIR"
    echo ""

    # 检查服务状态
    if [ -f "$INSTALL_DIR/openaide.pid" ]; then
        local PID
        PID=$(cat "$INSTALL_DIR/openaide.pid")
        if kill -0 "$PID" 2>/dev/null; then
            echo -e "服务状态: ${GREEN}运行中${NC} (PID: $PID)"
        else
            echo -e "服务状态: ${RED}未运行${NC}"
        fi
    else
        echo -e "服务状态: ${YELLOW}未启动${NC}"
    fi

    # 显示版本
    if [ -f "$BIN_DIR/openaide-cli" ]; then
        echo "CLI 路径: $BIN_DIR/openaide-cli"
        ls -lh "$BIN_DIR/openaide-cli" | awk '{print "编译时间:", $6, $7, $8}'
    fi

    # 显示备份
    local backup_count
    backup_count=$(ls -d "$INSTALL_DIR"/.backup-* 2>/dev/null | wc -l)
    echo "备份数量: $backup_count"

    echo ""
}

# 清理旧备份
cleanup_backups() {
    log_info "清理旧备份 (保留最近5个)..."
    ls -td "$INSTALL_DIR"/.backup-* 2>/dev/null | tail -n +6 | xargs rm -rf 2>/dev/null || true
    log_ok "清理完成"
}

# 主函数
main() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}  OpenAIDE 更新工具${NC}"
    echo -e "${BLUE}========================================${NC}"
    echo ""

    # 解析参数
    local use_local=false
    local do_restart=false
    local do_rollback=false
    local do_status=false
    local do_cleanup=false

    for arg in "$@"; do
        case "$arg" in
            --local|-l)
                use_local=true
                ;;
            --restart|-r)
                do_restart=true
                ;;
            --rollback)
                do_rollback=true
                ;;
            --status|-s)
                do_status=true
                ;;
            --cleanup|-c)
                do_cleanup=true
                ;;
            --version|-v)
                VERSION="$2"
                shift
                ;;
            --help|-h)
                echo "用法: $0 [选项]"
                echo ""
                echo "选项:"
                echo "  --local, -l       本地重新编译"
                echo "  --restart, -r     仅重启服务"
                echo "  --rollback        回滚到上一个版本"
                echo "  --status, -s      显示状态"
                echo "  --cleanup, -c     清理旧备份"
                echo "  --version, -v     指定版本号"
                echo "  --help, -h        显示帮助"
                echo ""
                echo "示例:"
                echo "  $0                    # 更新到最新 Release"
                echo "  $0 --local            # 本地重新编译"
                echo "  $0 --restart          # 重启服务"
                echo "  $0 --rollback         # 回滚到备份"
                exit 0
                ;;
        esac
    done

    # 执行操作
    if [ "$do_status" = true ]; then
        show_status
        exit 0
    fi

    if [ "$do_rollback" = true ]; then
        rollback
        exit 0
    fi

    if [ "$do_cleanup" = true ]; then
        cleanup_backups
        exit 0
    fi

    if [ "$do_restart" = true ]; then
        restart_service
        exit 0
    fi

    # 默认: 更新
    if [ "$use_local" = true ]; then
        update_local
    else
        update_from_release "$VERSION"
    fi

    # 重启服务
    restart_service

    # 清理旧备份
    cleanup_backups

    echo ""
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  更新完成!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo "当前版本: $VERSION"
    echo "备份位置: $BACKUP_DIR"
    echo ""
    echo "查看日志:"
    echo "  tail -f $INSTALL_DIR/logs/server.log"
    echo ""
    echo "使用 CLI:"
    echo "  openaide"
    echo ""
}

main "$@"
