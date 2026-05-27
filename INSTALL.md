# OpenAIDE 安装指南 | Installation Guide

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Prerequisites

- **Go 1.25+** (no CGO required, pure Go)
- **Docker** (optional, for containerized deployment)

### Build

```bash
cd backend

# Install dependencies
make deps

# Build server and CLI
make build
# Output: bin/openaide-server, bin/openaide

# Or build directly
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide-server ./cmd/server
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide ./cmd/cli
```

### Run

```bash
# Start the API server
cd backend && make run
# Server runs on http://localhost:8080

# Or use the compiled binary
./bin/openaide-server -config ~/.openaide/config.yaml

# Interactive CLI (forces direct mode, no API server)
./bin/openaide
```

### Configuration

Config file: `~/.openaide/config.yaml` (or `.json`). Supports both YAML and JSON via file extension detection.

**Minimal config:**

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"

llm:
  default_provider: "openai"
  providers:
    - name: "openai"
      type: "openai"
      base_url: "https://api.openai.com/v1"
      api_key: "sk-your-api-key"
      default_model: "gpt-4o-mini"
      timeout: 60
      enabled: true

kernel:
  max_rounds: 10
  max_tokens: 4000

log:
  level: "info"
  format: "json"
```

**Provider types:** `openai` or `openai-compatible` — all providers go through the OpenAI-compatible protocol. Supports OpenAI, DeepSeek, Qwen (阿里云百炼), Ollama, and any OpenAI-compatible API.

**DeepSeek-specific options** (per provider):
```yaml
providers:
  - name: "deepseek"
    type: "openai"
    base_url: "https://api.deepseek.com/v1"
    api_key: "sk-your-key"
    default_model: "deepseek-v4-pro"
    thinking: true
    reasoning_effort: "max"  # low/medium/high/max (Agent 推荐 max)
    json_mode: false
    enabled: true
```

**Server modes:**
- `server` — starts the HTTP API server
- `direct` — no API server (used by CLI)

### API Endpoints

| Route | Method | Description |
|-------|--------|-------------|
| `/api/v1/chat` | POST | Chat (sync) |
| `/api/v1/chat/stream` | POST | Chat (SSE streaming) |
| `/api/v1/sessions` | GET/POST | List / Create sessions |
| `/api/v1/sessions/{id}` | GET/DELETE | Get history / Delete session |
| `/api/v1/memory/search?q=` | GET | Search memory |
| `/api/v1/tools` | GET | List tools |
| `/api/v1/stats` | GET | System stats |
| `/health` | GET | Health check |

No authentication required (auth is planned but not yet implemented).

### Test

```bash
cd backend

# Run all tests
make test
# or: go test -v ./internal/...

# Run with coverage
make test-coverage

# Run a single package
go test -v ./internal/kernel/...
```

### Lint & Format

```bash
cd backend
make fmt
make lint       # requires golangci-lint
```

### Docker

```bash
cd backend

# Build image
make docker-build

# Start
make docker-run

# Stop
make docker-stop

# Or use docker-compose directly
docker-compose up -d
```

### GitHub Actions CI/CD

Push to `master` triggers: build → test → package.  
Push a `v*` tag triggers: build → test → GitHub Release with binaries.

---

<a name="中文"></a>
## 中文

### 环境要求

- **Go 1.25+**（无需 CGO，纯 Go）
- **Docker**（可选，用于容器化部署）

### 编译

```bash
cd backend

# 安装依赖
make deps

# 编译服务端和 CLI
make build
# 输出: bin/openaide-server, bin/openaide

# 或直接编译
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide-server ./cmd/server
CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/openaide ./cmd/cli
```

### 运行

```bash
# 启动 API 服务
cd backend && make run
# 服务运行在 http://localhost:8080

# 或使用编译好的二进制
./bin/openaide-server -config ~/.openaide/config.yaml

# 交互式 CLI（强制 direct 模式，不启动 API 服务）
./bin/openaide
```

### 配置

配置文件：`~/.openaide/config.yaml`（或 `.json`），通过文件扩展名自动识别格式。

**最小配置：**

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"

llm:
  default_provider: "openai"
  providers:
    - name: "openai"
      type: "openai"
      base_url: "https://api.openai.com/v1"
      api_key: "sk-your-api-key"
      default_model: "gpt-4o-mini"
      timeout: 60
      enabled: true

kernel:
  max_rounds: 10
  max_tokens: 4000

log:
  level: "info"
  format: "json"
```

**支持的提供商类型：** `openai` 或 `openai-compatible`。所有提供商统一通过 OpenAI 兼容协议接入。支持 OpenAI、DeepSeek、通义千问、Ollama 及任何 OpenAI 兼容 API。

**DeepSeek 特有参数**（按 provider 配置）：
```yaml
providers:
  - name: "deepseek"
    type: "openai"
    base_url: "https://api.deepseek.com/v1"
    api_key: "sk-your-key"
    default_model: "deepseek-v4-pro"
    thinking: true
    reasoning_effort: "max"  # Agent 推荐 max
    json_mode: false
    enabled: true
```

**Server 运行模式：**
- `server` — 启动 HTTP API 服务
- `direct` — 不启动 API（CLI 使用）

### API 端点

| 路由 | 方法 | 说明 |
|------|------|------|
| `/api/v1/chat` | POST | 同步聊天 |
| `/api/v1/chat/stream` | POST | 流式聊天 (SSE) |
| `/api/v1/sessions` | GET/POST | 列出/创建会话 |
| `/api/v1/sessions/{id}` | GET/DELETE | 获取历史/删除会话 |
| `/api/v1/memory/search?q=` | GET | 搜索记忆 |
| `/api/v1/tools` | GET | 工具列表 |
| `/api/v1/stats` | GET | 系统统计 |
| `/api/v1/metrics` | GET | 运行时指标 |
| `/api/v1/config` | GET/PUT | 读写配置 |
| `/api/v1/projects` | GET/POST | 项目列表/创建 |
| `/api/v1/projects/{id}` | GET/PUT/DELETE | 单个项目操作 |
| `/api/v1/channels` | GET | 渠道列表 |
| `/api/v1/auth/register` | POST | 注册 |
| `/api/v1/auth/login` | POST | 登录 |
| `/ws` | GET | WebSocket |
| `/health` | GET | 健康检查 |

JWT 认证默认关闭，开启后在请求头中携带 `Authorization: Bearer <token>`。

### 测试

```bash
cd backend
make test                      # 运行全部测试
make test-coverage             # 测试+覆盖率报告
go test -v ./internal/kernel/...  # 单个包
```

### 代码检查

```bash
cd backend
make fmt     # 格式化
make lint    # Lint（需安装 golangci-lint）
```

### Docker

```bash
cd backend
make docker-build    # 构建镜像
make docker-run      # 启动容器
make docker-stop     # 停止容器
# 或直接用 docker-compose up -d
```

### GitHub Actions CI/CD

推送到 `master`：构建 → 测试 → 打包。  
推送 `v*` 标签：构建 → 测试 → 创建 GitHub Release。

---

## 项目结构 | Project Structure

```
backend/
├── cmd/
│   ├── server/main.go        # API 服务器入口
│   ├── cli/main.go            # 交互式 REPL 入口
│   └── cli/                    # REPL (repl.go, repl_output.go, utils.go)
├── internal/
│   ├── infra/   (4 files)     # DI 容器
│   ├── kernel/  (18 files)    # Agent 内核 (process/stream/prompt + enhancements)
│   ├── llm/    (7 files)      # LLM 网关: 多提供商 + Anthropic + OpenAI
│   ├── tools/  (10 files)     # 22 个工具按领域拆分
│   ├── memory/                 # 3 级文件记忆 + 语义搜索
│   ├── orchestration/          # 编排器 + Planner + Team
│   ├── api/                    # REST + SSE + WebSocket
│   ├── plugin/                 # 插件管理 + Claude 格式解析
│   ├── compress/               # LLM 语义压缩
│   ├── event/                  # 事件总线
│   ├── channel/                # 外部消息渠道
│   ├── mcp/                    # MCP 协议
│   └── (config, git, index, identity, knowledge, feedback, auth, lang)
├── Makefile
├── Dockerfile
└── docker-compose.yml
```

## 与旧版文档的差异 | Differences from Legacy Docs

旧版文档描述的设计阶段架构已全部更新。当前实际状态：

| 项目 | 当前状态 |
|------|----------|
| HTTP 框架 | `net/http` 标准库 (非 Gin) |
| 数据存储 | 文件 JSON + 内存 (非 GORM/SQLite) |
| 端口 | 8080 |
| JWT 认证 | 已实现 |
| REPL | lmorg/readline + glamour + pterm (非 Bubble Tea) |
| 22 个内置工具 | 已实现 |
| MCP 协议 | 已实现 (stdio) |
| 插件系统 (Claude 格式) | 已实现 |
| Anthropic 原生 API | 已实现 |
| WebSocket | 已实现 |
| DeepPlan 深度规划 | 已实现 |
| ProjectMind 持续学习 | 已实现 |
| LLM 语义压缩 | 已实现 |
| 外部渠道 (Webhook/飞书/Telegram) | 已实现 |
