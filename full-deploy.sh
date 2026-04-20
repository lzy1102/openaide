#!/bin/bash
# OpenAIDE 完整部署脚本
# 使用方法: 将本地代码同步到服务器后执行 bash /opt/openaide/full-deploy.sh

set -e

echo "========================================="
echo "  OpenAIDE 完整部署"
echo "========================================="

# 1. 清理旧文件
echo "[1/6] 清理旧文件..."
rm -f /opt/openaide/backend/src/services/post_hook_service.go
rm -f /opt/openaide/backend/src/services/enhanced_dialogue_service.go

# 2. 修复 go.mod
echo "[2/6] 修复 go.mod..."
cat > /opt/openaide/backend/go.mod << 'GOMOD'
module openaide/backend

go 1.24

require (
	github.com/gin-gonic/gin v1.12.0
	github.com/glebarez/sqlite v1.11.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/larksuite/oapi-sdk-go/v3 v3.5.3
	github.com/ledisdb/ledisdb v0.0.0-20200510135210-d35789ec47e6
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/robfig/cron/v3 v3.0.1
	github.com/stretchr/testify v1.11.1
	golang.org/x/crypto v0.49.0
	gopkg.in/yaml.v3 v3.0.1
	gorm.io/gorm v1.31.1
)
GOMOD
echo "go.mod 已修复"

# 3. 编译
echo "[3/6] 编译服务..."
cd /opt/openaide/backend/src
/usr/local/go/bin/go mod tidy 2>/dev/null
/usr/local/go/bin/go build -o /opt/openaide/openaide-server . 2>&1 | tail -20
echo "编译完成: $(ls -lh /opt/openaide/openaide-server | awk '{print $5}')"

# 4. 停止旧进程
echo "[4/6] 停止旧服务..."
fuser -k 19375/tcp 2>/dev/null || true
sleep 3

# 5. 启动新服务
echo "[5/6] 启动新服务..."
cd /opt/openaide
nohup ./openaide-server > /opt/openaide/server.log 2>&1 &
echo "服务已启动 PID: $!"
sleep 10

# 6. 验证
echo "[6/6] 验证..."
curl -s http://localhost:19375/health | python3 -m json.tool 2>/dev/null || curl -s http://localhost:19375/health

echo ""
echo "========================================="
echo "  部署完成!"
echo "========================================="
echo "自我进化服务状态:"
grep -E "Self-Evolution|SkillDiscovery|PatternDetector|FeedbackCollector|MemoryExtract|Memory.*semantic" /opt/openaide/server.log || echo "等待定时任务启动..."
echo ""
echo "进程: $(ps aux | grep openaide-server | grep -v grep | awk '{print "PID="$2}')"
echo "日志: tail -f /opt/openaide/server.log"
