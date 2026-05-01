# OpenAIDE 安装指南 | Installation Guide

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Quick Start

#### 1. Prerequisites

- **Go 1.22+** (with CGO for SQLite)
- **gcc** (required for CGO/SQLite compilation)
- **Docker** (optional, for containerized deployment)

#### 2. Install Backend

```bash
cd openaide/backend
go mod tidy

# Copy and edit configuration
cp config.example.json config.json
# Edit config.json and add your API keys

# Run backend server (no CGO required)
go run ./src/main.go
# Server runs on http://localhost:19375
```

#### 3. Build

```bash
go build -o openaide-server ./src
./openaide-server
```

### Configuration

#### Config File Lookup Priority

`OPENAIDE_CONFIG` env → `/app/config.json` (Docker) → `OPENAIDE_HOME/config.json` → `./config.json` → executable directory → `~/.openaide/config.json`

#### Backend Configuration (`config.json`)

```json
{
  "home_dir": "/opt/openaide",
  "storage": {
    "cache": {
      "type": "ledis",
      "default_expiration": 3600,
      "cleanup_interval": 600,
      "data_dir": "/opt/openaide/data/ledis"
    },
    "db": {
      "type": "sqlite",
      "uri": "/opt/openaide/data/db/openaide.db"
    },
    "vector_store": {
      "type": "hnsw",
      "data_dir": "/opt/openaide/data/vectors"
    }
  },
  "models": [
    {
      "name": "deepseek-chat",
      "provider": "deepseek",
      "api_key": "your-deepseek-api-key",
      "base_url": "https://api.deepseek.com",
      "config": { "model": "deepseek-chat", "timeout": 60 },
      "status": "enabled"
    },
    {
      "name": "gpt-4",
      "provider": "openai",
      "api_key": "sk-your-openai-api-key",
      "base_url": "https://api.openai.com/v1",
      "config": { "model": "gpt-4", "timeout": 60 },
      "status": "enabled"
    }
  ],
  "context": {
    "compression_enabled": true,
    "compression_mode": "balanced",
    "max_tokens": 8000,
    "keep_last_n": 4
  },
  "activity_timeout": "30m"
}
```

> See `config.example.json` for the full template with all options.

#### Storage Configuration

| Component | Type | Description |
|-----------|------|-------------|
| **Cache** | `memory` | In-memory cache (default, no persistence) |
| | `ledis` | LedisDB embedded KV store (recommended, Redis-compatible) |
| | `redis` | External Redis server |
| **DB** | `sqlite` | SQLite database (default, pure Go, no CGO) |
| | `postgres` | PostgreSQL database |
| | `mysql` | MySQL database |
| **VectorStore** | `hnsw` | HNSW vector index with JSON persistence (default) |
| | `memory` | In-memory brute-force search (testing only) |

#### Supported LLM Providers

| Provider | API Key Required | Base URL |
|----------|------------------|----------|
| `openai` | ✅ | https://api.openai.com/v1 |
| `anthropic` | ✅ | https://api.anthropic.com |
| `deepseek` | ✅ | https://api.deepseek.com |
| `qwen` | ✅ | https://dashscope.aliyuncs.com/compatible-mode/v1 |
| `moonshot` | ✅ | https://api.moonshot.cn/v1 |
| `glm` (智谱) | ✅ | https://open.bigmodel.cn/api/paas/v4 |
| `ernie` | ✅ + secret_key | - |
| `ollama` | ❌ | http://localhost:11434/v1 |

### Authentication

All API routes (except `/health` and `/api/auth/*`) require JWT authentication.

```bash
# Register a new user
curl -X POST http://localhost:19375/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","email":"admin@example.com","password":"your-password"}'

# Login to get access token
curl -X POST http://localhost:19375/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
# Response: {"code":0,"message":"success","data":{"access_token":"eyJ...","refresh_token":"...","expires_in":86400}}

# Access protected routes with Bearer token
curl http://localhost:19375/api/dialogues \
  -H "Authorization: Bearer eyJ..."
```

**Production**: Set `OPENAIDE_JWT_SECRET` environment variable. If not set, a random secret is generated on each restart (existing sessions will be invalidated).

### API Response Format

All API responses use a unified format:

```json
// Success
{"code": 0, "message": "success", "data": {...}}

// Error
{"code": 400, "message": "error description"}

// Paginated list (supports ?page=1&page_size=20)
{"code": 0, "message": "success", "data": {"items": [...], "total": 100, "page": 1, "page_size": 20, "total_pages": 5}}
```

Every response includes `X-Request-ID` header for request tracing.

### Server Deployment

#### Docker Compose (Recommended)

```bash
cd backend

# Copy environment template
cp .env.example .env
# Edit .env and set JWT_SECRET

# Copy and edit config
cp config.example.json config.json
# Edit config.json and add your API keys

# Start service
docker-compose up -d

# View logs
docker-compose logs -f

# Stop service
docker-compose down
```

#### Manual Deployment (Linux)

```bash
# 1. Build
cd backend
CGO_ENABLED=0 go build -o openaide-server ./src

# 2. Create directories
sudo mkdir -p /opt/openaide/data/{db,vectors,ledis,sessions}
sudo mkdir -p /opt/openaide/logs
sudo mkdir -p /var/log/openaide

# 3. Deploy binary and config
sudo cp openaide-server /opt/openaide/
sudo cp config.json /opt/openaide/

# 4. Create systemd service
sudo cat > /lib/systemd/system/openaide.service << 'EOF'
[Unit]
Description=OpenAIDE AI Assistant Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/openaide
Environment=PORT=19375
Environment=OPENAIDE_HOME=/opt/openaide
Environment=OPENAIDE_JWT_SECRET=your-secret-here
ExecStart=/opt/openaide/openaide-server
Restart=always
RestartSec=5
StandardOutput=append:/var/log/openaide/server.log
StandardError=append:/var/log/openaide/error.log

[Install]
WantedBy=multi-user.target
EOF

# 5. Start service
sudo systemctl daemon-reload
sudo systemctl enable openaide
sudo systemctl start openaide

# 6. Check status
sudo systemctl status openaide
curl http://localhost:19375/health
```

#### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `19375` | Server listen port |
| `OPENAIDE_HOME` | `~/.openaide` | Data directory root |
| `OPENAIDE_CONFIG` | auto-detect | Config file path |
| `OPENAIDE_JWT_SECRET` | auto-generated | JWT signing secret |
| `OPENAIDE_FRONTEND_DIR` | auto-detect | Frontend static files |
| `TZ` | system | Timezone |

### CLI (Optional)

```bash
cd terminal
go build -o openaide main.go

# Use
openaide              # Start chat
openaide -m deepseek  # Specify model
openaide models       # List models
openaide --help       # Show help
```

---

<a name="中文"></a>
## 中文

### 快速开始

#### 1. 环境要求

- **Go 1.22+** (需要 CGO 支持 SQLite)
- **gcc** (CGO/SQLite 编译必需)
- **Docker** (可选，用于容器化部署)

#### 2. 安装后端

```bash
cd openaide/backend
go mod tidy

# 复制并编辑配置文件
cp config.example.json config.json
# 编辑 config.json，添加你的 API Keys

# 运行后端服务（无需 CGO）
go run ./src/main.go
# 服务运行在 http://localhost:19375
```

#### 3. 编译

```bash
CGO_ENABLED=1 go build -o openaide-server ./src
./openaide-server
```

### 配置

#### 配置文件查找优先级

`OPENAIDE_CONFIG` 环境变量 → `/app/config.json` (Docker) → `OPENAIDE_HOME/config.json` → `./config.json` → 可执行文件目录 → `~/.openaide/config.json`

#### 后端配置 (`config.json`)

```json
{
  "home_dir": "/opt/openaide",
  "storage": {
    "cache": {
      "type": "ledis",
      "default_expiration": 3600,
      "cleanup_interval": 600,
      "data_dir": "/opt/openaide/data/ledis"
    },
    "db": {
      "type": "sqlite",
      "uri": "/opt/openaide/data/db/openaide.db"
    },
    "vector_store": {
      "type": "hnsw",
      "data_dir": "/opt/openaide/data/vectors"
    }
  },
  "models": [
    {
      "name": "deepseek-chat",
      "provider": "deepseek",
      "api_key": "你的-deepseek-api-key",
      "base_url": "https://api.deepseek.com",
      "config": { "model": "deepseek-chat", "timeout": 60 },
      "status": "enabled"
    },
    {
      "name": "glm-5",
      "provider": "glm",
      "api_key": "你的-智谱-api-key",
      "base_url": "https://open.bigmodel.cn/api/paas/v4",
      "config": { "model": "glm-5", "timeout": 60 },
      "status": "enabled"
    }
  ],
  "context": {
    "compression_enabled": true,
    "compression_mode": "balanced",
    "max_tokens": 8000,
    "keep_last_n": 4
  },
  "activity_timeout": "30m"
}
```

> 完整配置模板见 `config.example.json`。

#### 存储配置

| 组件 | 类型 | 说明 |
|------|------|------|
| **缓存** | `memory` | 内存缓存（默认，无持久化） |
| | `ledis` | LedisDB 嵌入式 KV 存储（推荐，兼容 Redis） |
| | `redis` | 外部 Redis 服务器 |
| **数据库** | `sqlite` | SQLite 数据库（默认，纯 Go，无需 CGO） |
| | `postgres` | PostgreSQL 数据库 |
| | `mysql` | MySQL 数据库 |
| **向量存储** | `hnsw` | HNSW 向量索引 + JSON 持久化（默认） |
| | `memory` | 内存暴力搜索（仅测试用） |

#### 支持的 LLM 提供商

| 提供商 | 需要 API Key | Base URL |
|--------|--------------|----------|
| `openai` | ✅ | https://api.openai.com/v1 |
| `anthropic` | ✅ | https://api.anthropic.com |
| `deepseek` | ✅ | https://api.deepseek.com |
| `qwen` (通义千问) | ✅ | https://dashscope.aliyuncs.com/compatible-mode/v1 |
| `moonshot` (Kimi) | ✅ | https://api.moonshot.cn/v1 |
| `glm` (智谱) | ✅ | https://open.bigmodel.cn/api/paas/v4 |
| `ernie` (文心一言) | ✅ + secret_key | - |
| `ollama` (本地) | ❌ | http://localhost:11434/v1 |

### 认证

所有 API 路由（除 `/health` 和 `/api/auth/*`）均需要 JWT 认证。

```bash
# 注册新用户
curl -X POST http://localhost:19375/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","email":"admin@example.com","password":"your-password"}'

# 登录获取访问令牌
curl -X POST http://localhost:19375/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
# 响应: {"code":0,"message":"success","data":{"access_token":"eyJ...","refresh_token":"...","expires_in":86400}}

# 使用 Bearer 令牌访问受保护路由
curl http://localhost:19375/api/dialogues \
  -H "Authorization: Bearer eyJ..."
```

**生产环境**：务必设置 `OPENAIDE_JWT_SECRET` 环境变量。未设置时每次重启自动生成新密钥，已登录用户的会话将失效。

### API 响应格式

所有 API 响应使用统一格式：

```json
// 成功
{"code": 0, "message": "success", "data": {...}}

// 错误
{"code": 400, "message": "错误描述"}

// 分页列表（支持 ?page=1&page_size=20 参数）
{"code": 0, "message": "success", "data": {"items": [...], "total": 100, "page": 1, "page_size": 20, "total_pages": 5}}
```

每个响应都包含 `X-Request-ID` 请求追踪头。

### 服务器部署

#### Docker Compose 部署（推荐）

```bash
cd backend

# 复制环境变量模板
cp .env.example .env
# 编辑 .env，设置 JWT_SECRET

# 复制并编辑配置
cp config.example.json config.json
# 编辑 config.json，添加 API Keys

# 启动服务
docker-compose up -d

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down
```

#### 手动部署（Linux）

```bash
# 1. 编译
cd backend
CGO_ENABLED=1 go build -o openaide-server ./src

# 2. 创建目录
sudo mkdir -p /opt/openaide/data/{db,vectors,ledis,sessions}
sudo mkdir -p /opt/openaide/logs
sudo mkdir -p /var/log/openaide

# 3. 部署二进制和配置
sudo cp openaide-server /opt/openaide/
sudo cp config.json /opt/openaide/

# 4. 创建 systemd 服务
sudo cat > /lib/systemd/system/openaide.service << 'EOF'
[Unit]
Description=OpenAIDE AI Assistant Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/openaide
Environment=PORT=19375
Environment=OPENAIDE_HOME=/opt/openaide
Environment=OPENAIDE_JWT_SECRET=你的密钥
ExecStart=/opt/openaide/openaide-server
Restart=always
RestartSec=5
StandardOutput=append:/var/log/openaide/server.log
StandardError=append:/var/log/openaide/error.log

[Install]
WantedBy=multi-user.target
EOF

# 5. 启动服务
sudo systemctl daemon-reload
sudo systemctl enable openaide
sudo systemctl start openaide

# 6. 检查状态
sudo systemctl status openaide
curl http://localhost:19375/health
```

#### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PORT` | `19375` | 服务监听端口 |
| `OPENAIDE_HOME` | `~/.openaide` | 数据目录根路径 |
| `OPENAIDE_CONFIG` | 自动检测 | 配置文件路径 |
| `OPENAIDE_JWT_SECRET` | 自动生成 | JWT 签名密钥 |
| `OPENAIDE_FRONTEND_DIR` | 自动检测 | 前端静态文件目录 |
| `TZ` | 系统时区 | 时区设置 |

### CLI (可选)

```bash
cd terminal
go build -o openaide main.go

# 使用
openaide              # 开始聊天
openaide -m deepseek  # 指定模型
openaide models       # 列出模型
openaide --help       # 显示帮助
```

---

## Architecture | 架构

```
┌──────────────┐     ┌──────────────────────────────────────────┐     ┌─────────────────┐
│  CLI Client  │────▶│           Backend API (Port 19375)       │────▶│  LLM Providers  │
│  (openaide)  │     │                                          │     │  (OpenAI, etc)  │
└──────────────┘     │  ┌─────────┐ ┌────────┐ ┌────────────┐ │     └─────────────────┘
                     │  │  Auth   │ │ Router │ │ Middleware  │ │
┌──────────────┐     │  │ (JWT)   │ │ (Gin)  │ │ CORS/ReqID │ │     ┌─────────────────┐
│  Web Client  │────▶│  └─────────┘ └────────┘ └────────────┘ │     │  Vector Store   │
│  (Frontend)  │     │                                          │────▶│  (HNSW/Memory)  │
└──────────────┘     │  ┌─────────────────────────────────┐    │     └─────────────────┘
                     │  │         Service Layer            │    │
┌──────────────┐     │  │ Dialogue / Memory / Skill / ... │    │     ┌─────────────────┐
│  Feishu Bot  │────▶│  └─────────────────────────────────┘    │────▶│     Cache       │
│  (WebSocket) │     │                                          │     │ (Ledis/Redis)   │
└──────────────┘     │  ┌─────────────────────────────────┐    │     └─────────────────┘
                     │  │       Pluggable Storage          │    │
                     │  │  DB / Cache / VectorStore        │────│────▶  SQLite / PG / MySQL
                     │  └─────────────────────────────────┘    │
                     └──────────────────────────────────────────┘
```

**Key Points:**
- **API Keys** are stored in `config.json` (server-side only)
- **Pluggable Storage**: DB (SQLite/PostgreSQL/MySQL), Cache (Memory/LedisDB/Redis), VectorStore (HNSW/Memory)
- **JWT Authentication**: All routes protected by default, configurable secret via env var
- **Structured Logging**: slog-based logging with component tags
- **Graceful Shutdown**: SIGINT/SIGTERM with 30s timeout
- **Request Tracing**: X-Request-ID auto-generated and propagated
