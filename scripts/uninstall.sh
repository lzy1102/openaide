#!/bin/bash

# OpenAIDE 卸载脚本

set -e

INSTALL_DIR="${INSTALL_DIR:-/opt/openaide}"
BIN_DIR="/usr/local/bin"

echo ""
echo "========================================"
echo "  OpenAIDE 卸载程序"
echo "========================================"
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 sudo 运行此脚本"
    exit 1
fi

# 确认卸载
read -p "确定要卸载 OpenAIDE 吗? [y/N] " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "取消卸载"
    exit 0
fi

# 停止并禁用服务
echo "🛑 停止服务..."
systemctl stop openaide 2>/dev/null || true
systemctl disable openaide 2>/dev/null || true

# 删除服务文件
echo "🗑️  删除服务文件..."
rm -f /etc/systemd/system/openaide.service
systemctl daemon-reload

# 删除符号链接
echo "🔗 删除命令链接..."
rm -f "$BIN_DIR/openaide"

# 询问是否删除数据
read -p "是否删除安装目录 ($INSTALL_DIR)? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🗑️  删除安装目录..."
    rm -rf "$INSTALL_DIR"
else
    echo "📁 保留安装目录: $INSTALL_DIR"
fi

echo ""
echo "========================================"
echo "✅ 卸载完成！"
echo "========================================"
echo ""
