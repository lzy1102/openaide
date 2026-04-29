#!/bin/bash
#
# OpenAIDE 卸载脚本
# 用法: sudo ./uninstall.sh
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}       OpenAIDE 卸载程序               ${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}请使用 sudo 运行此脚本${NC}"
    exit 1
fi

echo -e "${YELLOW}⚠️  这将删除 OpenAIDE 的所有文件和配置!${NC}"
read -p "确认卸载? (yes/no): " confirm

if [ "$confirm" != "yes" ]; then
    echo "已取消卸载"
    exit 0
fi

# 停止服务
echo -e "${BLUE}[1/5] 停止服务...${NC}"
systemctl stop openaide 2>/dev/null || true
systemctl disable openaide 2>/dev/null || true
echo "  完成"

# 删除二进制
echo -e "${BLUE}[2/5] 删除二进制文件...${NC}"
rm -f /usr/local/bin/openaide-server
rm -f /usr/local/bin/openaide
echo "  完成"

# 删除 systemd 服务
echo -e "${BLUE}[3/5] 删除 systemd 服务...${NC}"
rm -f /lib/systemd/system/openaide.service
systemctl daemon-reload
echo "  完成"

# 删除数据和配置
echo -e "${BLUE}[4/5] 删除数据和配置...${NC}"
read -p "是否删除配置文件和数据? (yes/no): " delete_data
if [ "$delete_data" = "yes" ]; then
    rm -rf /opt/openaide
    rm -rf /var/log/openaide
    echo "  已删除所有数据"
else
    echo "  保留数据在 /opt/openaide/"
fi

# 完成
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}       卸载完成!                       ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
