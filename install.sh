#!/bin/bash
#
# OpenAIDE 安装脚本
# 用法: sudo ./install.sh
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}       OpenAIDE 安装程序               ${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}请使用 sudo 运行此脚本${NC}"
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 目录定义 (符合 FHS 标准)
BIN_DIR="/usr/local/bin"
CONFIG_DIR="/opt/openaide/.openaide"
DATA_DIR="/opt/openaide"
LOG_DIR="/var/log/openaide"

echo -e "${YELLOW}安装目录:${NC}"
echo "  二进制: $BIN_DIR"
echo "  配置:   $CONFIG_DIR"
echo "  数据:   $DATA_DIR"
echo "  日志:   $LOG_DIR"
echo ""

# 1. 创建目录
echo -e "${BLUE}[1/6] 创建目录结构...${NC}"
mkdir -p "$BIN_DIR"
mkdir -p "$CONFIG_DIR"
mkdir -p "$DATA_DIR"
mkdir -p "$LOG_DIR"
echo "  完成"

# 2. 安装二进制
echo -e "${BLUE}[2/6] 安装二进制文件...${NC}"

if [ -f "$SCRIPT_DIR/backend/src/openaide-server" ]; then
    cp "$SCRIPT_DIR/backend/src/openaide-server" "$BIN_DIR/"
    chmod +x "$BIN_DIR/openaide-server"
    echo "  openaide-server -> $BIN_DIR/"
elif [ -f "$SCRIPT_DIR/dist/openaide-server" ]; then
    cp "$SCRIPT_DIR/dist/openaide-server" "$BIN_DIR/"
    chmod +x "$BIN_DIR/openaide-server"
    echo "  openaide-server -> $BIN_DIR/"
else
    echo -e "  ${RED}错误: 找不到 openaide-server 二进制文件${NC}"
    echo "  请先编译: cd backend/src && GOOS=linux GOARCH=amd64 go build -o openaide-server ."
    exit 1
fi

if [ -f "$SCRIPT_DIR/terminal/openaide" ]; then
    cp "$SCRIPT_DIR/terminal/openaide" "$BIN_DIR/"
    chmod +x "$BIN_DIR/openaide"
    echo "  openaide (CLI) -> $BIN_DIR/"
elif [ -f "$SCRIPT_DIR/dist/openaide" ]; then
    cp "$SCRIPT_DIR/dist/openaide" "$BIN_DIR/"
    chmod +x "$BIN_DIR/openaide"
    echo "  openaide (CLI) -> $BIN_DIR/"
else
    echo -e "  ${YELLOW}警告: 找不到 openaide CLI 二进制${NC}"
fi

# 3. 安装配置文件
echo -e "${BLUE}[3/6] 安装配置文件...${NC}"

if [ ! -f "$CONFIG_DIR/config.json" ]; then
    if [ -f "$SCRIPT_DIR/backend/config.example.json" ]; then
        cp "$SCRIPT_DIR/backend/config.example.json" "$CONFIG_DIR/config.json"
        echo "  已创建默认配置文件"
        echo -e "  ${YELLOW}⚠️  请编辑 $CONFIG_DIR/config.json 添加你的 API Keys!${NC}"
    elif [ -f "$SCRIPT_DIR/.openaide.example" ]; then
        cp "$SCRIPT_DIR/.openaide.example" "$CONFIG_DIR/config.json"
        echo "  已创建默认配置文件"
    else
        echo "  创建最小配置..."
        cat > "$CONFIG_DIR/config.json" << 'EOF'
{
  "models": [],
  "default_model": "",
  "feishu": {"enabled": false},
  "voice": {"enabled": false},
  "sandbox": {"enabled": false},
  "embedding": {"enabled": false},
  "context": {
    "compression_enabled": true,
    "compression_mode": "balanced",
    "max_tokens": 8000,
    "keep_last_n": 4,
    "preserve_tool_calls": true,
    "fallback_to_summary": true
  },
  "activity_timeout": "30m"
}
EOF
    fi
else
    echo "  配置文件已存在，保留"
fi

# 4. 复制前端文件
echo -e "${BLUE}[4/6] 复制前端文件...${NC}"
if [ -d "$SCRIPT_DIR/frontend" ]; then
    cp -r "$SCRIPT_DIR/frontend" "$DATA_DIR/" 2>/dev/null || true
    echo "  前端文件已复制"
else
    echo "  前端目录不存在，跳过"
fi

# 5. 创建 systemd 服务
echo -e "${BLUE}[5/6] 创建 systemd 服务...${NC}"

cat > /lib/systemd/system/openaide.service << EOF
[Unit]
Description=OpenAIDE AI Assistant Server
Documentation=https://github.com/openaide/openaide
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/opt/openaide
Environment=PORT=19375
Environment=HOME=/root
ExecStart=/usr/local/bin/openaide-server
Restart=always
RestartSec=5
StandardOutput=append:/var/log/openaide/server.log
StandardError=append:/var/log/openaide/error.log

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable openaide
echo "  服务已创建并启用"

# 6. 完成
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}       安装完成!                       ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "目录布局:"
echo "  /usr/local/bin/openaide-server  - 后端服务"
echo "  /usr/local/bin/openaide         - CLI 工具"
echo "  /opt/openaide/.openaide/config.json - 配置文件"
echo "  /opt/openaide/                  - 数据目录"
echo "  /var/log/openaide/              - 日志目录"
echo ""
echo "下一步:"
echo "  1. 编辑配置: nano /opt/openaide/.openaide/config.json"
echo "  2. 启动服务: systemctl start openaide"
echo "  3. 查看状态: systemctl status openaide"
echo "  4. 使用 CLI: openaide"
echo ""
