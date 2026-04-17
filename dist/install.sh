#!/bin/bash
#
# OpenAIDE 安装脚本
# 遵循 Linux FHS (Filesystem Hierarchy Standard) 标准
#
# 目录布局:
#   /usr/bin/openaide              - CLI 工具
#   /usr/bin/openaide-server       - 服务程序
#   /etc/openaide/                 - 配置目录
#   /var/lib/openaide/             - 数据目录
#   /var/log/openaide/             - 日志目录
#   /usr/share/openaide/           - 静态资源
#

set -e

# 版本信息
VERSION="${VERSION:-latest}"
INSTALL_VERSION="${INSTALL_VERSION:-system}"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║           OpenAIDE 安装程序 v${VERSION}                      ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ 请使用 sudo 运行此脚本${NC}"
    exit 1
fi

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 检测安装模式
if [ "$INSTALL_VERSION" = "standalone" ]; then
    # 独立安装模式 - 所有文件在 /opt/openaide/
    PREFIX="/opt/openaide"
    BIN_DIR="$PREFIX/bin"
    CONFIG_DIR="$PREFIX/etc"
    DATA_DIR="$PREFIX/var/lib"
    LOG_DIR="$PREFIX/var/log"
    SHARE_DIR="$PREFIX/share"
    SYSTEM_BIN="/usr/local/bin"
else
    # 系统安装模式 - 遵循 FHS 标准
    PREFIX=""
    BIN_DIR="/usr/bin"
    CONFIG_DIR="/etc/openaide"
    DATA_DIR="/var/lib/openaide"
    LOG_DIR="/var/log/openaide"
    SHARE_DIR="/usr/share/openaide"
    SYSTEM_BIN="/usr/bin"
fi

echo -e "${YELLOW}📦 安装模式: ${INSTALL_VERSION}${NC}"
echo ""

# ============================================
# 1. 创建目录结构
# ============================================
echo -e "${BLUE}📁 创建目录结构...${NC}"

mkdir -p "$BIN_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"/{database,knowledge,uploads}
mkdir -p "$LOG_DIR"
mkdir -p "$SHARE_DIR"/frontend

# 设置目录权限
if command -v openaide &> /dev/null; then
    # 如果 openaide 用户存在
    chown -R openaide:openaide "$DATA_DIR" "$LOG_DIR" 2>/dev/null || true
fi

echo "   BIN:     $BIN_DIR"
echo "   CONFIG:  $CONFIG_DIR"
echo "   DATA:    $DATA_DIR"
echo "   LOG:     $LOG_DIR"
echo "   SHARE:   $SHARE_DIR"
echo ""

# ============================================
# 2. 安装程序文件
# ============================================
echo -e "${BLUE}📦 安装程序文件...${NC}"

# 安装二进制文件
if [ -f "$SCRIPT_DIR/bin/openaide" ]; then
    cp "$SCRIPT_DIR/bin/openaide" "$BIN_DIR/"
    chmod 755 "$BIN_DIR/openaide"
    echo "   ✓ openaide (CLI)"
fi

if [ -f "$SCRIPT_DIR/bin/openaide-server" ]; then
    cp "$SCRIPT_DIR/bin/openaide-server" "$BIN_DIR/"
    chmod 755 "$BIN_DIR/openaide-server"
    echo "   ✓ openaide-server"
fi

# 安装前端文件
if [ -d "$SCRIPT_DIR/frontend" ]; then
    cp -r "$SCRIPT_DIR/frontend/"* "$SHARE_DIR/frontend/"
    echo "   ✓ frontend"
fi

# ============================================
# 3. 创建配置文件
# ============================================
echo -e "${BLUE}📝 配置文件...${NC}"

# 主配置文件
if [ ! -f "$CONFIG_DIR/config" ]; then
    if [ -f "$SCRIPT_DIR/.openaide.example" ]; then
        cp "$SCRIPT_DIR/.openaide.example" "$CONFIG_DIR/config"
        echo "   ✓ 已创建 $CONFIG_DIR/config"
        echo -e "   ${YELLOW}⚠️  请编辑配置文件添加 API Keys!${NC}"
    fi
else
    echo "   ✓ 配置文件已存在，保留"
fi

# 创建符号链接 (兼容旧路径)
ln -sf "$CONFIG_DIR/config" "$CONFIG_DIR/.openaide" 2>/dev/null || true

# ============================================
# 4. 创建命令链接 (standalone 模式)
# ============================================
if [ "$INSTALL_VERSION" = "standalone" ]; then
    echo -e "${BLUE}🔗 创建命令链接...${NC}"
    ln -sf "$BIN_DIR/openaide" "$SYSTEM_BIN/openaide"
    ln -sf "$BIN_DIR/openaide-server" "$SYSTEM_BIN/openaide-server"
fi

# ============================================
# 5. 创建 systemd 服务
# ============================================
echo -e "${BLUE}⚙️  创建系统服务...${NC}"

cat > /lib/systemd/system/openaide.service << EOF
[Unit]
Description=OpenAIDE AI Assistant Server
Documentation=https://github.com/openaide/openaide
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=$DATA_DIR
Environment="OPENAIDE_CONFIG=$CONFIG_DIR/config"
Environment="OPENAIDE_DATA=$DATA_DIR"
Environment="OPENAIDE_LOG=$LOG_DIR"
Environment="OPENAIDE_LOCAL_MODE=false"
ExecStart=$BIN_DIR/openaide-server
Restart=always
RestartSec=5
StandardOutput=append:$LOG_DIR/server.log
StandardError=append:$LOG_DIR/error.log

# 安全限制
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=$DATA_DIR $LOG_DIR

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable openaide

# ============================================
# 6. 完成
# ============================================
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              ✅ 安装完成！                                ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "目录布局:"
echo "  程序:     $BIN_DIR/"
echo "  配置:     $CONFIG_DIR/config"
echo "  数据:     $DATA_DIR/"
echo "  日志:     $LOG_DIR/"
echo "  前端:     $SHARE_DIR/frontend/"
echo ""
echo "下一步:"
echo -e "  ${YELLOW}1.${NC} 编辑配置文件:"
echo "     sudo nano $CONFIG_DIR/config"
echo ""
echo -e "  ${YELLOW}2.${NC} 启动服务:"
echo "     sudo systemctl start openaide"
echo ""
echo -e "  ${YELLOW}3.${NC} 查看状态:"
echo "     sudo systemctl status openaide"
echo ""
echo -e "  ${YELLOW}4.${NC} 使用 CLI:"
echo "     openaide"
echo ""
echo "API 端点: http://localhost:19375"
echo ""
