# OpenAIDE 部署指南

> 版本: v3.0.0-draft
> 日期: 2026-05-15
> 适用场景: 本地部署、服务器部署、Docker 部署

---

## 目录

1. [部署模式概述](#1-部署模式概述)
2. [本地部署](#2-本地部署)
3. [服务器部署](#3-服务器部署)
4. [Docker 部署](#4-docker-部署)
5. [数据管理](#5-数据管理)
6. [配置参考](#6-配置参考)
7. [故障排除](#7-故障排除)

---

## 1. 部署模式概述

OpenAIDE 支持三种运行模式：

| 模式 | 命令 | 适用场景 | 特点 |
|------|------|----------|------|
| **直接模式** | `openaide` | 本地开发（默认） | 零依赖，即开即用 |
| **服务器模式** | `openaide server` | 团队共享 | HTTP API，多客户端 |
| **TUI 模式** | `openaide tui` | 终端爱好者 | 富文本界面 |

### 1.1 架构图

```
┌─────────────────────────────────────────┐
│           直接模式（默认）                │
│  ┌─────────┐    ┌─────────────────────┐ │
│  │  终端   │───►│   OpenAIDE 内核     │ │
│  └─────────┘    │  (AI Agent + 记忆)   │ │
│                 └─────────────────────┘ │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│           服务器模式                     │
│  ┌─────────┐    ┌─────────┐   ┌──────┐ │
│  │ 终端 1  │───►│         │   │      │ │
│  ├─────────┤    │  HTTP   │◄──┤ 内核 │ │
│  │ 终端 2  │───►│  API    │   │      │ │
│  ├─────────┤    │ 服务    │   └──────┘ │
│  │ Web UI  │───►│         │            │
│  └─────────┘    └─────────┘            │
└─────────────────────────────────────────┘
```

---

## 2. 本地部署

### 2.1 安装

```bash
# 方式一：Go 安装
go install github.com/openaide/openaide@latest

# 方式二：二进制安装（Linux/macOS）
curl -fsSL https://openaide.dev/install.sh | bash

# 方式三：二进制安装（Windows）
iwr -useb https://openaide.dev/install.ps1 | iex
```

### 2.2 配置

首次运行自动创建配置目录：

```bash
# 启动并自动配置
openaide

# 或手动编辑配置
mkdir -p ~/.openaide
cat > ~/.openaide/config.yaml << 'EOF'
provider: openai
api_key: sk-your-key-here
model: gpt-4o
EOF
```

### 2.3 验证

```bash
openaide --version
# openaide v3.0.0

# 测试对话
echo "你好" | openaide
```

### 2.4 数据位置

| 数据类型 | 路径 | 说明 |
|----------|------|------|
| 配置 | `~/.openaide/config.yaml` | 用户配置 |
| 会话 | `~/.openaide/sessions/` | 对话历史 |
| 记忆 | `~/.openaide/memory/` | 长期记忆 |
| 知识库 | `~/.openaide/knowledge/` | 知识文件 |
| 日志 | `~/.openaide/logs/` | 运行日志 |

---

## 3. 服务器部署

### 3.1 单机部署

```bash
# 1. 下载二进制
wget https://github.com/openaide/openaide/releases/download/v3.0.0/openaide-linux-amd64
chmod +x openaide-linux-amd64

# 2. 创建配置
mkdir -p /etc/openaide
cat > /etc/openaide/config.yaml << 'EOF'
provider: openai
api_key: sk-your-key-here
model: gpt-4o

# 服务器配置
server:
  host: 0.0.0.0
  port: 8080
  auth:
    enabled: true
    token: your-api-token
EOF

# 3. 启动服务
./openaide-linux-amd64 server --config /etc/openaide/config.yaml

# 4. 后台运行（systemd）
sudo tee /etc/systemd/system/openaide.service << 'EOF'
[Unit]
Description=OpenAIDE Server
After=network.target

[Service]
Type=simple
User=openaide
ExecStart=/usr/local/bin/openaide server --config /etc/openaide/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now openaide
```

### 3.2 使用 Nginx 反向代理

```nginx
server {
    listen 80;
    server_name openaide.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 3.3 多实例部署（负载均衡）

```yaml
# docker-compose.yml
version: '3.8'

services:
  openaide-1:
    image: openaide/openaide:v3.0.0
    volumes:
      - ./config.yaml:/app/config.yaml
      - openaide-data:/app/.openaide
    environment:
      - OPENAIDE_SERVER_PORT=8081

  openaide-2:
    image: openaide/openaide:v3.0.0
    volumes:
      - ./config.yaml:/app/config.yaml
      - openaide-data:/app/.openaide
    environment:
      - OPENAIDE_SERVER_PORT=8082

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - openaide-1
      - openaide-2

volumes:
  openaide-data:
```

---

## 4. Docker 部署

### 4.1 快速启动

```bash
docker run -d \
  --name openaide \
  -p 8080:8080 \
  -v ~/.openaide:/root/.openaide \
  -e OPENAIDE_API_KEY=sk-your-key \
  openaide/openaide:v3.0.0 server
```

### 4.2 Docker Compose（完整版）

```yaml
version: '3.8'

services:
  openaide:
    image: openaide/openaide:v3.0.0
    container_name: openaide
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - openaide-data:/app/.openaide
      - ./workspace:/workspace
    environment:
      - OPENAIDE_PROVIDER=openai
      - OPENAIDE_API_KEY=${OPENAIDE_API_KEY}
      - OPENAIDE_MODEL=gpt-4o
      - OPENAIDE_SERVER_HOST=0.0.0.0
      - OPENAIDE_SERVER_PORT=8080
    command: server

volumes:
  openaide-data:
    driver: local
```

### 4.3 构建自定义镜像

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY . .
RUN go build -o openaide ./cmd/openaide

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/openaide .

# 数据卷
VOLUME ["/root/.openaide"]

EXPOSE 8080
ENTRYPOINT ["./openaide"]
CMD ["server"]
```

```bash
docker build -t my-openaide .
docker run -d -p 8080:8080 -v ~/.openaide:/root/.openaide my-openaide
```

---

## 5. 数据管理

### 5.1 备份

```bash
# 备份所有数据
tar czf openaide-backup-$(date +%Y%m%d).tar.gz ~/.openaide/

# 仅备份会话
tar czf sessions-backup.tar.gz ~/.openaide/sessions/

# 自动备份脚本（crontab）
# 每天凌晨 3 点备份
0 3 * * * tar czf /backup/openaide-$(date +\%Y\%m\%d).tar.gz ~/.openaide/
```

### 5.2 迁移

```bash
# 导出数据
openaide export --output ./openaide-export.tar.gz

# 导入数据
openaide import --input ./openaide-export.tar.gz

# 跨机器迁移
rsync -avz ~/.openaide/ user@new-server:~/.openaide/
```

### 5.3 清理

```bash
# 清理旧会话（保留最近 30 天）
openaide cleanup --sessions --older-than 30d

# 清理所有日志
openaide cleanup --logs

# 重置所有数据（谨慎使用）
openaide reset --all
```

---

## 6. 配置参考

### 6.1 完整配置示例

```yaml
# ~/.openaide/config.yaml

# LLM 配置
provider: openai
api_key: sk-xxxxxxxxxxxxxxxxxxxxxxxx
model: gpt-4o
base_url: ""           # 自定义 API 地址（可选）
proxy: ""              # HTTP 代理（可选）
timeout: 60            # 请求超时（秒）

# 内核配置
kernel:
  max_iterations: 10   # 最大思考轮数
  temperature: 0.7     #  creativity
  auto_compress: true  # 自动上下文压缩

# 记忆配置
memory:
  max_context: 4000    # 最大上下文 token 数
  summary_interval: 10 # 每 N 轮生成摘要

# 权限配置
permission:
  level: standard      # strict/standard/relaxed
  dangerous_tools:     # 危险工具确认
    - file_delete
    - command_exec

# 服务器配置（仅 server 模式）
server:
  host: 127.0.0.1
  port: 8080
  auth:
    enabled: false
    token: ""

# 日志配置
log:
  level: info          # debug/info/warn/error
  file: ""             # 日志文件路径（空则只输出控制台）
```

### 6.2 环境变量映射

| 环境变量 | 配置项 | 示例 |
|----------|--------|------|
| `OPENAIDE_PROVIDER` | `provider` | `openai` |
| `OPENAIDE_API_KEY` | `api_key` | `sk-xxx` |
| `OPENAIDE_MODEL` | `model` | `gpt-4o` |
| `OPENAIDE_PROXY` | `proxy` | `http://127.0.0.1:7890` |
| `OPENAIDE_SERVER_HOST` | `server.host` | `0.0.0.0` |
| `OPENAIDE_SERVER_PORT` | `server.port` | `8080` |

---

## 7. 故障排除

### 7.1 服务无法启动

```bash
# 检查端口占用
lsof -i :8080

# 检查配置格式
openaide config validate

# 查看详细日志
openaide server --log-level debug
```

### 7.2 内存占用过高

```bash
# 限制上下文大小
openaide config set memory.max_context 2000

# 启用自动压缩
openaide config set kernel.auto_compress true

# 清理旧会话
openaide cleanup --sessions --older-than 7d
```

### 7.3 API 连接失败

```bash
# 测试网络连通性
curl -I https://api.openai.com

# 检查代理设置
openaide config show | grep proxy

# 切换提供商
openaide config set provider aliyun
```

### 7.4 数据丢失恢复

```bash
# 检查备份
ls -la ~/.openaide/backups/

# 从备份恢复
tar xzf openaide-backup-20260515.tar.gz -C ~/

# 检查文件权限
ls -la ~/.openaide/
```

---

## 附录

### A. 系统要求

| 场景 | CPU | 内存 | 磁盘 |
|------|-----|------|------|
| 个人使用 | 2 核 | 4GB | 1GB |
| 小团队 | 4 核 | 8GB | 10GB |
| 大团队 | 8 核+ | 16GB+ | 50GB+ |

### B. 端口说明

| 端口 | 用途 | 可配置 |
|------|------|--------|
| 8080 | HTTP API | 是 |

### C. 健康检查

```bash
# HTTP 健康检查
curl http://localhost:8080/health

# 预期响应
{"status":"ok","version":"3.0.0"}
```

---

> **安全提示**: 
> - 生产环境务必启用 API 认证
> - 定期备份数据
> - 不要将 API 密钥提交到代码仓库
