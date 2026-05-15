#!/bin/bash

# OpenAIDE Backend 启动脚本

set -e

echo "Starting OpenAIDE Backend..."

# 检查 Go 环境
if ! command -v go &> /dev/null; then
    echo "Go is not installed. Please install Go 1.25+"
    exit 1
fi

# 切换到脚本所在目录
cd "$(dirname "$0")"

# 检查是否需要初始化配置
if [ ! -f "config.json" ] && [ -f "config.example.json" ]; then
    echo "Creating config.json from example..."
    cp config.example.json config.json
    echo "Please edit config.json and add your API keys!"
fi

# 下载依赖
echo "Downloading dependencies..."
go mod tidy

# 编译
echo "Building..."
CGO_ENABLED=0 go build -o bin/openaide-server ./cmd/server

# 启动服务器
echo "Starting server on port 8080..."
./bin/openaide-server
