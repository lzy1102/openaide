#!/bin/bash

# OpenAIDE Backend 启动脚本
# OpenAIDE Backend Startup Script

set -e

echo "🚀 Starting OpenAIDE Backend..."

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed. Please install Go 1.20+"
    exit 1
fi

# 切换到脚本所在目录
cd "$(dirname "$0")"

# 检查是否需要初始化配置
if [ ! -f "config.json" ] && [ -f "config.example.json" ]; then
    echo "📝 Creating config.json from example..."
    cp config.example.json config.json
    echo "⚠️  Please edit config.json and add your API keys!"
fi

# 下载依赖
echo "📦 Downloading dependencies..."
go mod tidy

# 编译
echo "🔨 Building..."
go build -o openaide-server ./src

# 启动服务器
echo "✅ Starting server on port 19375..."
PORT=${PORT:-19375}
./openaide-server

