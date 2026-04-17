# OpenAIDE Backend

OpenAIDE is an intelligent assistant project supporting multi-model LLM integration, thinking & reasoning, auto correction, continuous learning, and knowledge base features.

## Table of Contents

- [Features](#features)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
- [API Documentation](#api-documentation)
- [Tech Stack](#tech-stack)
- [Supported LLM Providers](#supported-llm-providers)

## Features

### Core Capabilities

| Module | File | Status |
|--------|------|--------|
| LLM Multi-model Integration | `services/llm/` | ✅ |
| Thinking & Reasoning | `services/thinking_service.go` | ✅ |
| Auto Correction | `services/correction_service.go` | ✅ |
| Continuous Learning | `services/learning_service.go` | ✅ |
| Workflow Engine | `services/workflow_service.go` | ✅ |

### Knowledge Base System

| Module | File | Status |
|--------|------|--------|
| Knowledge Data Model | `models/knowledge.go` | ✅ |
| Vector Embedding | `services/embedding_service.go` | ✅ |
| Knowledge Service | `services/knowledge_service.go` | ✅ |
| RAG Service | `services/rag_service.go` | ✅ |
| Knowledge Extraction | `services/knowledge_extraction_service.go` | ✅ |
| Document Import | `services/document_service.go` | ✅ |

### Core Enhancement

| Module | File | Status |
|--------|------|--------|
| Task Decomposition | `services/task_decompose_service.go` | ✅ |
| Context Management | `services/context_manager.go` | ✅ |
| Team Coordination | `services/team_coordinator.go` | ✅ |
| Integration Testing | `services/integration_test.go` | ✅ |

### New Features (Latest)

| Module | Description | Status |
|--------|-------------|--------|
| **Function Calling** | Tool execution system for LLM-powered function calls | ✅ |
| **Prompt Templates** | Template management with versioning and variables | ✅ |
| **Usage Analytics** | Usage statistics and cost tracking | ✅ |
| **Task Scheduler** | Cron-based scheduled tasks and reminders | ✅ |

## Project Structure

```
backend/src/
├── models/
│   ├── knowledge.go          # Knowledge model
│   ├── task.go               # Task model
│   ├── team.go               # Team model
│   ├── thinking.go           # Thinking model
│   ├── tool.go               # Tool/Function Calling model
│   ├── prompt_template.go    # Prompt template model
│   ├── usage.go              # Usage tracking model
│   └── scheduler.go          # Scheduler model
├── services/
│   ├── llm/                  # LLM clients
│   │   ├── llm_client.go     # Unified interface
│   │   ├── openai_client.go
│   │   ├── anthropic_client.go
│   │   └── glm_client.go
│   ├── thinking_service.go   # Thinking & reasoning
│   ├── correction_service.go # Auto correction
│   ├── learning_service.go   # Continuous learning
│   ├── workflow_service.go   # Workflow engine
│   ├── knowledge_service.go  # Knowledge service
│   ├── embedding_service.go  # Vector generation
│   ├── rag_service.go        # RAG service
│   ├── task_decompose_service.go  # Task decomposition
│   ├── context_manager.go    # Context management
│   ├── team_coordinator.go   # Team coordination
│   ├── tool_service.go       # Tool execution service
│   ├── prompt_template_service.go  # Template service
│   ├── usage_service.go      # Usage tracking service
│   └── scheduler_service.go  # Scheduler service
└── handlers/
    ├── task_handler.go       # Task API
    ├── tool_handler.go       # Tool API
    ├── prompt_template_handler.go  # Template API
    ├── usage_handler.go      # Usage API
    └── scheduler_handler.go  # Scheduler API
```

## Getting Started

### Prerequisites

- Go 1.21+
- SQLite

### Installation

```bash
# Install dependencies
go mod download

# Run the server
go run cmd/main.go
```

### Local Mode

For local use without authentication:

```bash
# Enable local mode
export OPENAIDE_LOCAL_MODE=true

# Run server
go run cmd/main.go
```

In local mode:
- No registration or login required
- All APIs accessible without authentication
- Runs with admin privileges automatically

### Configuration

Configuration is loaded from environment variables or config files.

## API Documentation

### Task Management

- `POST /api/tasks/decompose` - Decompose task
- `GET /api/tasks` - List tasks
- `GET /api/tasks/:id` - Get task details
- `PATCH /api/tasks/:id/status` - Update status

### Knowledge Base

- `POST /api/knowledge` - Create knowledge
- `GET /api/knowledge/search` - Search knowledge
- `POST /api/documents/import` - Import document

### RAG

- `POST /api/rag/query` - RAG query
- `POST /api/rag/stream` - Streaming RAG

### Function Calling (Tools)

- `GET /api/tools` - List all tools
- `GET /api/tools/definitions` - Get tool definitions for LLM
- `GET /api/tools/:name` - Get tool details
- `POST /api/tools` - Create/register a tool
- `POST /api/tools/execute` - Execute a tool

### Prompt Templates

- `GET /api/prompt-templates` - List templates
- `POST /api/prompt-templates` - Create template
- `GET /api/prompt-templates/:id` - Get template
- `PUT /api/prompt-templates/:id` - Update template
- `DELETE /api/prompt-templates/:id` - Delete template
- `POST /api/prompt-templates/:id/render` - Render template
- `GET /api/prompt-templates/name/:name` - Get template by name
- `POST /api/prompt-templates/name/:name/render` - Render by name
- `POST /api/prompt-templates/:id/versions` - Create new version
- `GET /api/prompt-templates/name/:name/versions` - Get version history
- `POST /api/prompt-templates/extract-variables` - Extract variables
- `POST /api/prompt-templates/export` - Export templates
- `POST /api/prompt-templates/import` - Import templates

### Usage Analytics

- `GET /api/usage/stats` - Get usage statistics
- `GET /api/usage/daily` - Get daily usage
- `GET /api/usage/monthly` - Get monthly usage
- `GET /api/usage/history` - Get usage history
- `GET /api/usage/budget` - Get user budget
- `POST /api/usage/budget` - Set user budget
- `GET /api/usage/pricing` - Get model pricing
- `POST /api/usage/pricing` - Update model pricing

### Task Scheduler

#### Scheduled Tasks
- `GET /api/scheduler/tasks` - List tasks
- `POST /api/scheduler/tasks` - Create task
- `GET /api/scheduler/tasks/:id` - Get task details
- `POST /api/scheduler/tasks/:id/pause` - Pause task
- `POST /api/scheduler/tasks/:id/resume` - Resume task
- `DELETE /api/scheduler/tasks/:id` - Delete task
- `POST /api/scheduler/tasks/:id/execute` - Execute now
- `GET /api/scheduler/tasks/:id/executions` - Get execution history

#### Reminders
- `GET /api/scheduler/reminders` - List reminders
- `POST /api/scheduler/reminders` - Create reminder
- `POST /api/scheduler/reminders/:id/snooze` - Snooze reminder
- `DELETE /api/scheduler/reminders/:id` - Cancel reminder

## Tech Stack

- **Backend**: Go + Gin + GORM + SQLite
- **LLM**: Multi-provider support (see below)
- **Vector**: text-embedding-ada-002
- **Frontend**: HTML/CSS/JavaScript

## Supported LLM Providers

The system now supports 17+ LLM providers through a unified interface.

### Chinese LLMs (国内大模型)

| Provider | Type | Client File | Default Model |
|----------|------|-------------|---------------|
| 通义千问 (Qwen) | OpenAI Compatible | `qwen_client.go` | qwen-turbo |
| 文心一言 (ERNIE) | Custom API | `ernie_client.go` | ernie-bot-4 |
| 混元 (Hunyuan) | OpenAI Compatible | `hunyuan_client.go` | hunyuan-lite |
| 星火 (Spark) | Custom API | `spark_client.go` | spark-v3.5 |
| Moonshot (Kimi) | OpenAI Compatible | `moonshot_client.go` | moonshot-v1-8k |
| 百川 (Baichuan) | OpenAI Compatible | `baichuan_client.go` | Baichuan2-Turbo |
| MiniMax | OpenAI Compatible | `minimax_client.go` | abab5.5-chat |
| DeepSeek | OpenAI Compatible | `deepseek_client.go` | deepseek-chat |
| **智谱 GLM** | OpenAI Compatible | `glm_client.go` | **glm-5** |

> **GLM Latest Models**: glm-5 (旗舰), glm-4.7 (编程增强), glm-4-plus, glm-4-flash

### International LLMs (国际大模型)

| Provider | Type | Client File |
|----------|------|-------------|
| OpenAI | Native | `openai_client.go` |
| Anthropic (Claude) | Native | `anthropic_client.go` |
| Google Gemini | Custom API | `gemini_client.go` |
| Mistral AI | OpenAI Compatible | `mistral_client.go` |
| Cohere | Custom API | `cohere_client.go` |
| Groq | OpenAI Compatible | `groq_client.go` |

### Local Models (本地模型)

| Provider | Type | Client File |
|----------|------|-------------|
| Ollama | OpenAI Compatible | `ollama_client.go` |
| vLLM | OpenAI Compatible | `vllm_client.go` |

## Architecture

```
services/llm/
├── llm_client.go              # Unified LLMClient interface & factory
├── openai_compatible_client.go # Base class for OpenAI-compatible APIs
├── openai_client.go           # OpenAI native client
├── anthropic_client.go        # Anthropic/Claude client
├── glm_client.go              # 智谱 GLM client
├── [provider]_client.go       # Other provider clients
└── llm_client_test.go         # Tests
```

## License

MIT License
