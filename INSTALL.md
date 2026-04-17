# OpenAIDE 安装指南 | Installation Guide

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

### Quick Start

#### 1. Prerequisites

- **Go 1.20+** (for backend)
- **Python 3.6+** (for frontend dev server, optional)

#### 2. Install Backend

```bash
# Clone or download the project
cd openaide

# Install backend dependencies
cd backend
go mod tidy

# Copy and edit configuration
cp config.example.json config.json
# Edit config.json and add your API keys

# Run backend server
go run ./src/main.go
# Or build and run
go build -o bin/openaide-server ./src && ./bin/openaide-server
```

**Local Mode (No Authentication):**

For local use without user registration:

```bash
export OPENAIDE_LOCAL_MODE=true
go run ./src/main.go
```

In local mode, all APIs are accessible without authentication tokens.

#### 3. Install CLI (Optional)

```bash
# Build CLI
cd terminal
go build -o bin/openaide main.go

# Move to PATH (optional)
sudo mv bin/openaide /usr/local/bin/

# Or add to your shell profile
export PATH="$PATH:/path/to/openaide/terminal/bin"
```

### Configuration

#### Backend Configuration (`backend/config.json`)

API Keys are configured in the backend `config.json`:

```json
{
  "models": [
    {
      "name": "gpt-4",
      "provider": "openai",
      "api_key": "sk-your-openai-api-key-here",
      "base_url": "https://api.openai.com/v1",
      "status": "enabled"
    },
    {
      "name": "deepseek-chat",
      "provider": "deepseek",
      "api_key": "your-deepseek-api-key-here",
      "base_url": "https://api.deepseek.com",
      "status": "enabled"
    },
    {
      "name": "qwen-turbo",
      "provider": "qwen",
      "api_key": "your-dashscope-api-key-here",
      "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
      "status": "enabled"
    },
    {
      "name": "llama2-local",
      "provider": "ollama",
      "base_url": "http://localhost:11434/v1",
      "status": "enabled"
    },
    {
      "name": "glm-5",
      "provider": "glm",
      "api_key": "your-api-key-id.secret",
      "base_url": "https://open.bigmodel.cn/api/paas/v4",
      "status": "enabled"
    }
  ]
}
```

**Supported Providers:**

| Provider | API Key Required | Base URL |
|----------|------------------|----------|
| `openai` | ✅ | https://api.openai.com/v1 |
| `anthropic` | ✅ | https://api.anthropic.com |
| `deepseek` | ✅ | https://api.deepseek.com |
| `qwen` | ✅ | https://dashscope.aliyuncs.com/compatible-mode/v1 |
| `moonshot` | ✅ | https://api.moonshot.cn/v1 |
| `glm` (智谱) | ✅ | https://open.bigmodel.cn/api/paas/v4 |
| `glm` Coding Plan | ✅ | https://open.bigmodel.cn/api/coding/paas/v4 (glm-4.7, glm-5) |
| `ernie` | ✅ + secret_key | - |
| `ollama` | ❌ | http://localhost:11434/v1 |

#### CLI Configuration (`~/.openaide/config.yaml`)

```bash
# Initialize CLI config
openaide config init
```

Edit `~/.openaide/config.yaml`:

```yaml
api:
  base_url: "http://localhost:19375/api"
  timeout_sec: 30

chat:
  default_model: "gpt-4"
  stream: true
  context_limit: 10

models:
  - id: "gpt-4"
    name: "GPT-4"
    provider: "openai"
  - id: "deepseek-chat"
    name: "DeepSeek Chat"
    provider: "deepseek"
```

### Usage

#### Start Backend Server

```bash
cd backend
./bin/openaide-server
# Server runs on http://localhost:19375
```

#### Use CLI

```bash
# Enter chat (default)
openaide

# Specify model
openaide -m deepseek-chat

# With streaming
openaide -m gpt-4 -s

# Specify API URL
openaide --api http://localhost:19375/api
```

#### CLI Commands

```bash
openaide              # Start chat (default)
openaide models       # List available models
openaide config show  # Show current config
openaide config init  # Initialize config file
openaide --help       # Show help
```

### Docker Deployment

```bash
cd backend

# Build and run with Docker Compose
make docker-compose-up

# Or manually
docker build -t openaide:latest .
docker run -d -p 19375:19375 -v $(PWD)/data:/app/data openaide:latest
```

---

<a name="中文"></a>
## 中文

### 快速开始

#### 1. 环境要求

- **Go 1.20+** (后端)
- **Python 3.6+** (前端开发服务器，可选)

#### 2. 安装后端

```bash
# 克隆或下载项目
cd openaide

# 安装后端依赖
cd backend
go mod tidy

# 复制并编辑配置文件
cp config.example.json config.json
# 编辑 config.json，添加你的 API Keys

# 运行后端服务
go run ./src/main.go
# 或者编译后运行
go build -o bin/openaide-server ./src && ./bin/openaide-server
```

**本地模式 (无需认证):**

本地使用无需注册用户:

```bash
export OPENAIDE_LOCAL_MODE=true
go run ./src/main.go
```

本地模式下，所有 API 无需认证即可访问。

#### 3. 安装 CLI (可选)

```bash
# 编译 CLI
cd terminal
go build -o bin/openaide main.go

# 移动到 PATH (可选)
sudo mv bin/openaide /usr/local/bin/

# 或添加到 shell 配置
export PATH="$PATH:/path/to/openaide/terminal/bin"
```

### 配置

#### 后端配置 (`backend/config.json`)

API Keys 在后端 `config.json` 中配置：

```json
{
  "models": [
    {
      "name": "gpt-4",
      "provider": "openai",
      "api_key": "sk-your-openai-api-key-here",
      "base_url": "https://api.openai.com/v1",
      "status": "enabled"
    },
    {
      "name": "deepseek-chat",
      "provider": "deepseek",
      "api_key": "your-deepseek-api-key-here",
      "base_url": "https://api.deepseek.com",
      "status": "enabled"
    },
    {
      "name": "qwen-turbo",
      "provider": "qwen",
      "api_key": "your-dashscope-api-key-here",
      "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
      "status": "enabled"
    },
    {
      "name": "llama2-local",
      "provider": "ollama",
      "base_url": "http://localhost:11434/v1",
      "status": "enabled"
    },
    {
      "name": "glm-5",
      "provider": "glm",
      "api_key": "your-api-key-id.secret",
      "base_url": "https://open.bigmodel.cn/api/paas/v4",
      "status": "enabled"
    }
  ]
}
```

**支持的提供商：**

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

#### CLI 配置 (`~/.openaide/config.yaml`)

```bash
# 初始化 CLI 配置
openaide config init
```

编辑 `~/.openaide/config.yaml`：

```yaml
api:
  base_url: "http://localhost:19375/api"
  timeout_sec: 30

chat:
  default_model: "gpt-4"
  stream: true
  context_limit: 10

models:
  - id: "gpt-4"
    name: "GPT-4"
    provider: "openai"
  - id: "deepseek-chat"
    name: "DeepSeek Chat"
    provider: "deepseek"
```

### 使用方法

#### 启动后端服务

```bash
cd backend
./bin/openaide-server
# 服务运行在 http://localhost:19375
```

#### 使用 CLI

```bash
# 直接进入聊天（默认）
openaide

# 指定模型
openaide -m deepseek-chat

# 启用流式输出
openaide -m gpt-4 -s

# 指定 API 地址
openaide --api http://localhost:19375/api
```

#### CLI 命令

```bash
openaide              # 开始聊天（默认）
openaide models       # 列出可用模型
openaide config show  # 显示当前配置
openaide config init  # 初始化配置文件
openaide --help       # 显示帮助
```

### Docker 部署

```bash
cd backend

# 使用 Docker Compose 构建并运行
make docker-compose-up

# 或手动构建
docker build -t openaide:latest .
docker run -d -p 19375:19375 -v $(PWD)/data:/app/data openaide:latest
```

---

## Architecture | 架构

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   CLI Client    │────▶│   Backend API   │────▶│   LLM Providers │
│   (openaide)    │     │   (Port 19375)   │     │   (OpenAI, etc) │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │
        │                       │
   ~/.openaide/           backend/config.json
   config.yaml            (API Keys here)
   (API URL only)
```

**Key Points:**
- **API Keys** are stored in `backend/config.json` (server-side)
- **CLI config** (`~/.openaide/config.yaml`) only stores the API URL to connect to the backend
- **CLI** connects to the backend, which handles all LLM API calls
