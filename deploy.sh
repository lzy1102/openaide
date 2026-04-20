#!/bin/bash
# OpenAIDE 服务器部署脚本
# 使用方法: 在服务器上执行 bash /opt/openaide/deploy.sh

set -e

echo "========================================="
echo "  OpenAIDE 服务器部署脚本"
echo "========================================="

BACKEND_SRC="/opt/openaide/backend/src"
SERVER_BIN="/opt/openaide/openaide-server"
SERVER_LOG="/opt/openaide/server.log"

# 1. 拉取最新代码
echo ""
echo "[1/6] 拉取最新代码..."
cd "$BACKEND_SRC"
git pull || echo "git pull 失败，跳过"

# 2. 修复 go.mod 版本
echo "[2/6] 修复 go.mod..."
sed -i 's/go 1\.25\.0/go 1.26/' /opt/openaide/backend/go.mod
echo "go.mod 已更新: $(head -3 /opt/openaide/backend/go.mod)"

# 3. 编译
echo "[3/6] 编译服务..."
cd "$BACKEND_SRC"
CGO_ENABLED=0 go build -o "$SERVER_BIN" . 2>&1 | tail -30
echo "编译完成: $(ls -lh $SERVER_BIN | awk '{print $5}')"

# 4. 停止旧进程
echo "[4/6] 停止旧服务..."
OLD_PID=$(pgrep -f openaide-server || true)
if [ -n "$OLD_PID" ]; then
    kill -9 $OLD_PID
    sleep 2
    echo "已停止旧进程 PID: $OLD_PID"
else
    echo "没有运行中的进程"
fi

# 5. 启动新服务
echo "[5/6] 启动新服务..."
cd /opt/openaide
nohup ./openaide-server > "$SERVER_LOG" 2>&1 &
echo "服务已启动，PID: $!"
sleep 5

# 6. 验证
echo "[6/6] 验证服务..."
HEALTH=$(curl -s http://localhost:19375/health 2>/dev/null)
if [ -n "$HEALTH" ]; then
    echo "$HEALTH" | python3 -m json.tool 2>/dev/null || echo "$HEALTH"
else
    echo "健康检查未响应，查看日志:"
    tail -20 "$SERVER_LOG"
fi

echo ""
echo "========================================="
echo "  部署完成!"
echo "========================================="
echo "进程信息:"
ps aux | grep openaide-server | grep -v grep
echo ""
echo "查看日志: tail -f $SERVER_LOG"
echo "健康检查: curl http://localhost:19375/health"
