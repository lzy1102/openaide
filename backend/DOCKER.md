# Docker 部署指南 | Docker Deployment Guide

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Quick Start

```bash
# Build and run with Docker Compose
make docker-compose-up

# Or manually
docker-compose up -d
```

### Single Container

```bash
# Build image
docker build -t openaide:latest .

# Run container
docker run -d \
  --name openaide \
  -p 19375:19375 \
  -v $(PWD)/data:/app/data \
  -v $(PWD)/config.json:/app/config.json:ro \
  openaide:latest

# View logs
docker logs -f openaide

# Stop container
docker stop openaide && docker rm openaide
```

### Docker Compose

#### Development Environment

```bash
# Start services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

#### Production Environment

```bash
# Start production stack
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# With nginx (optional)
docker-compose -f docker-compose.yml -f docker-compose.prod.yml --profile nginx up -d

# With Ollama (optional)
docker-compose -f docker-compose.yml -f docker-compose.prod.yml --profile ollama up -d
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | 19375 |
| `GIN_MODE` | Gin mode (debug/release) | release |
| `TZ` | Timezone | Asia/Shanghai |
| `OPENAI_API_KEY` | OpenAI API key | - |
| `ANTHROPIC_API_KEY` | Anthropic API key | - |
| `DEEPSEEK_API_KEY` | DeepSeek API key | - |
| `QWEN_API_KEY` | Qwen API key | - |

### Volume Mounts

| Path | Description |
|------|-------------|
| `/app/data` | Persistent data (SQLite database) |
| `/app/config.json` | Configuration file |

### Health Check

```bash
# Check container health
docker inspect --format='{{.State.Health.Status}}' openaide

# Manual health check
curl http://localhost:19375/health
```

### Resource Limits

Production configuration includes:
- CPU limit: 2 cores
- Memory limit: 2GB
- CPU reservation: 0.5 cores
- Memory reservation: 512MB

Adjust in `docker-compose.prod.yml` as needed.

---

<a name="中文"></a>
## 中文

### 快速开始

```bash
# 使用 Docker Compose 构建并运行
make docker-compose-up

# 或手动执行
docker-compose up -d
```

### 单容器部署

```bash
# 构建镜像
docker build -t openaide:latest .

# 运行容器
docker run -d \
  --name openaide \
  -p 19375:19375 \
  -v $(PWD)/data:/app/data \
  -v $(PWD)/config.json:/app/config.json:ro \
  openaide:latest

# 查看日志
docker logs -f openaide

# 停止容器
docker stop openaide && docker rm openaide
```

### Docker Compose 部署

#### 开发环境

```bash
# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

#### 生产环境

```bash
# 启动生产环境栈
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# 包含 Nginx (可选)
docker-compose -f docker-compose.yml -f docker-compose.prod.yml --profile nginx up -d

# 包含 Ollama (可选)
docker-compose -f docker-compose.yml -f docker-compose.prod.yml --profile ollama up -d
```

### 环境变量

| 变量 | 描述 | 默认值 |
|------|------|--------|
| `PORT` | 服务端口 | 19375 |
| `GIN_MODE` | Gin 模式 (debug/release) | release |
| `TZ` | 时区 | Asia/Shanghai |
| `OPENAI_API_KEY` | OpenAI API 密钥 | - |
| `ANTHROPIC_API_KEY` | Anthropic API 密钥 | - |
| `DEEPSEEK_API_KEY` | DeepSeek API 密钥 | - |
| `QWEN_API_KEY` | 通义千问 API 密钥 | - |

### 数据卷挂载

| 路径 | 描述 |
|------|------|
| `/app/data` | 持久化数据 (SQLite 数据库) |
| `/app/config.json` | 配置文件 |

### 健康检查

```bash
# 检查容器健康状态
docker inspect --format='{{.State.Health.Status}}' openaide

# 手动健康检查
curl http://localhost:19375/health
```

### 资源限制

生产环境配置包含：
- CPU 限制: 2 核
- 内存限制: 2GB
- CPU 预留: 0.5 核
- 内存预留: 512MB

可在 `docker-compose.prod.yml` 中按需调整。

### 常用命令

```bash
# 查看服务状态
make docker-compose-ps

# 重启服务
make docker-compose-restart

# 查看日志
make docker-compose-logs

# 进入容器
make docker-shell

# 清理资源
make docker-clean
```

### 故障排查

#### 容器无法启动

```bash
# 查看容器日志
docker logs openaide

# 检查配置文件
docker exec openaide cat /app/config.json
```

#### 端口冲突

```bash
# 修改端口映射
docker run -d -p 8082:19375 ... openaide:latest

# 或使用环境变量
PORT=8082 docker-compose up -d
```

#### 数据持久化

确保 `./data` 目录有正确的权限：

```bash
mkdir -p data
chmod 755 data
```

---

## Dockerfile 说明

### 多阶段构建

```
┌─────────────────────────────────────────┐
│ Stage 1: Builder (golang:1.22-alpine)   │
│  - 安装构建依赖                          │
│  - 下载 Go 模块                          │
│  - 编译静态二进制文件                     │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│ Stage 2: Runtime (alpine:3.19)          │
│  - 最小化运行时镜像                       │
│  - 仅包含必要的运行时依赖                 │
│  - 健康检查配置                          │
└─────────────────────────────────────────┘
```

### 镜像大小优化

- 使用 Alpine 基础镜像
- 多阶段构建减少层数
- 静态链接减少依赖
- 清理缓存和临时文件

### 安全性

- 非 root 用户运行 (可配置)
- 只读配置文件挂载
- 健康检查监控
- 资源限制保护
