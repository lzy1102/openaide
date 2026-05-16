#!/bin/bash
set -e
export PATH=/usr/local/go/bin:$PATH
export HOME=/root

echo '[INFO] 解压代码到 /opt/openaide...'
mkdir -p /opt/openaide
tar -xzf /tmp/openaide-src.tar.gz -C /opt/openaide

echo '[INFO] 创建 root 用户安装目录...'
mkdir -p /root/.openaide/bin /root/.openaide/data /root/.openaide/logs /root/.openaide/scripts

echo '[INFO] 编译 CLI...'
cd /opt/openaide/backend
go build -ldflags '-s -w' -o /root/.openaide/bin/openaide-cli ./cmd/cli

echo '[INFO] 编译 Server...'
go build -ldflags '-s -w' -o /root/.openaide/bin/openaide-server ./cmd/server

echo '[INFO] 设置权限...'
chmod +x /root/.openaide/bin/openaide-cli /root/.openaide/bin/openaide-server

echo '[INFO] 创建快捷命令...'
ln -sf /root/.openaide/bin/openaide-cli /root/.openaide/bin/openaide
ln -sf /root/.openaide/bin/openaide-server /usr/local/bin/openaide-server 2>/dev/null || true
ln -sf /root/.openaide/bin/openaide-cli /usr/local/bin/openaide-cli 2>/dev/null || true
ln -sf /root/.openaide/bin/openaide /usr/local/bin/openaide 2>/dev/null || true

echo '[INFO] 复制更新脚本...'
cp /opt/openaide/scripts/update.sh /root/.openaide/scripts/
chmod +x /root/.openaide/scripts/update.sh

echo '[INFO] 创建配置文件...'
if [ ! -f /root/.openaide/config.yaml ]; then
  cp /opt/openaide/backend/config.example.yaml /root/.openaide/config.yaml
fi

echo '[INFO] 启动服务...'
cd /root/.openaide
nohup /root/.openaide/bin/openaide-server -config /root/.openaide/config.yaml > /root/.openaide/logs/server.log 2>&1 &
echo $! > /root/.openaide/openaide.pid
sleep 2

echo ''
echo '========================================'
echo '  安装完成!'
echo '========================================'
echo ''
echo "PID: $(cat /root/.openaide/openaide.pid)"
echo ''
echo '测试命令:'
echo '  openaide help         # 查看帮助'
echo '  openaide update --local   # 以后用这个更新'
echo '  openaide              # 启动聊天'
