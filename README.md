# OpenAIDE | AI 智能助手开发平台

[English](#english) | [中文](#中文)

---

<a name="english"></a>
## English

A full-featured AI Agent development platform with multi-model support, multi-agent collaboration, knowledge base RAG, workflow orchestration, and enterprise integrations.

### Features

#### Core Capabilities
- **18+ LLM Providers**: Unified interface for OpenAI, Claude, Gemini, Qwen, DeepSeek, GLM, Ollama, and more
- **Multi-Agent Collaboration**: Role-based agents (architect, developer, reviewer, researcher, PM, tester) with team coordination
- **Intelligent Orchestration**: Automatic task analysis → team planning → confirmation → execution pipeline
- **Knowledge Base & RAG**: Document import, vector embedding, semantic search, retrieval-augmented generation
- **Three-Layer Memory**: Working memory, short-term (dialogue summaries with TTL), long-term (facts/preferences/procedures/context with decay)
- **5-Layer System Prompt**: Base prompt → optimization suggestions → user preferences → memory context → RAG context
- **Thinking & Reasoning**: Chain-of-Thought, Multi-Step Reasoning, Tree-of-Thought with correction system
- **Tool Calling Framework**: HTTP requests, code execution, web search, weather, file I/O, and MCP external tools — with SSRF protection
- **Workflow Engine**: Step-by-step workflow execution with state machine, rollback, and scheduling
- **Plugin System**: Install, enable/disable, configure, and execute plugins
- **Skill System**: Modular skill management with auto-matching, parameter extraction, typed parameter normalization, tool binding, and execution tracking
- **Task Management**: CRUD, decomposition, dependencies (DAG), progress tracking, team assignment
- **Scheduled Tasks**: Cron-based scheduling with webhook and script execution
- **Code Sandbox**: Docker-isolated code execution with multi-language support and safety validation
- **Voice Services**: STT (Whisper) and TTS with configurable providers
- **WebSocket**: Real-time bidirectional communication and streaming dialogue
- **Feishu Integration**: Enterprise messaging bot with card callbacks and `/skill` command execution for matched skills
- **Adaptive Learning**: Feedback collection, preference learning, prompt optimization
- **Usage Analytics**: Token/cost tracking, budget management, threshold alerts via email
- **Authentication**: JWT tokens, API keys, role-based access control

#### Architecture

```
┌──────────────┐     ┌──────────────────────────────────────────────┐     ┌──────────────┐
│  Web / CLI   │────▶│              Backend (Gin)                   │────▶│  18+ LLMs    │
│  / Feishu    │     │  ┌─────┐ ┌──────┐ ┌──────┐ ┌─────────────┐ │     │  Providers   │
└──────────────┘     │  │Auth │ │Chat  │ │Tools │ │ Orchestration│ │     └──────────────┘
                     │  └─────┘ └──────┘ └──────┘ └─────────────┘ │
                     │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐ │     ┌──────────────┐
                     │  │Knowledge│RAG │ │Memory│ │Scheduler│ │────▶│   SQLite     │
                     │  └──────┘ └──────┘ └──────┘ └──────────┘ │     │   + Vectors  │
                     │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐ │     └──────────────┘
                     │  │Teams │ │Skills│ │Plugins│ │ Voice/WS │ │
                     │  └──────┘ └──────┘ └──────┘ └──────────┘ │
                     └──────────────────────────────────────────────┘
```

**Tech Stack:** Go 1.20+ / Gin / GORM / SQLite (modernc, CGO_ENABLED=0)

#### Supported LLM Providers

##### Domestic Chinese LLMs
| Provider | Alias | Models |
|----------|-------|--------|
| 通义千问 (Qwen) | `qwen`, `tongyi` | qwen-turbo, qwen-plus, qwen-max |
| 文心一言 (ERNIE) | `ernie`, `wenxin` | ernie-bot-4, ernie-bot-turbo |
| 混元 (Hunyuan) | `hunyuan` | hunyuan-lite, hunyuan-standard |
| 星火 (Spark) | `spark`, `xunfei` | spark-v3.5, spark-v4.0 |
| Moonshot (Kimi) | `moonshot`, `kimi` | moonshot-v1-8k, moonshot-v1-32k |
| 百川 (Baichuan) | `baichuan` | Baichuan2-Turbo, Baichuan2-53B |
| MiniMax | `minimax` | abab5.5-chat, abab5.5s-chat |
| DeepSeek | `deepseek` | deepseek-chat, deepseek-coder |
| 智谱 GLM | `glm`, `zhipu` | glm-5, glm-4.7, glm-4-plus, glm-4-flash |

##### International LLMs
| Provider | Alias | Models |
|----------|-------|--------|
| OpenAI | `openai` | gpt-4, gpt-4-turbo, gpt-3.5-turbo |
| Anthropic (Claude) | `anthropic`, `claude` | claude-3-opus, claude-3-sonnet, claude-3-haiku |
| Google Gemini | `gemini`, `google` | gemini-pro, gemini-ultra |
| Mistral AI | `mistral` | mistral-large, mistral-medium, mistral-small |
| Cohere | `cohere` | command, command-light |
| Groq | `groq` | llama2-70b-4096, mixtral-8x7b-32768 |

##### Local Models
| Provider | Alias | Description |
|----------|-------|-------------|
| Ollama | `ollama`, `local` | Run models locally (llama2, mistral, etc.) |
| vLLM | `vllm` | High-performance inference server |

### API Endpoints Overview

#### Authentication & User
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/register` | Register new user |
| POST | `/api/auth/login` | User login |
| POST | `/api/auth/refresh` | Refresh access token |
| GET | `/api/profile` | Get user profile |
| PUT | `/api/profile` | Update profile |
| POST | `/api/change-password` | Change password |
| GET/DELETE | `/api/sessions` | Manage sessions |
| CRUD | `/api/api-keys` | API key management |
| CRUD | `/api/admin/users` | Admin user management |

#### Chat & Dialogue
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/chat` | Send chat message |
| POST | `/api/chat/stream` | Streaming chat (SSE) |
| POST | `/api/chat/tools` | Chat with tool calling |
| POST | `/api/chat/route` | Auto-routed streaming chat |
| POST | `/api/chat/plan` | Chat with planning |
| GET | `/api/chat/route-info` | Get model routing info |
| CRUD | `/api/dialogues` | Dialogue management |
| POST | `/api/dialogues/:id/stream` | Streaming dialogue (SSE) |

#### Orchestration
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/orchestration/process` | Start orchestration pipeline |
| POST | `/api/orchestration/analyze` | Analyze task only |
| POST | `/api/orchestration/plan` | Generate team plan only |
| GET | `/api/orchestration/sessions` | List sessions |
| GET | `/api/orchestration/:id` | Get session status |
| POST | `/api/orchestration/:id/action` | Approve/reject/adjust plan |
| GET | `/api/orchestration/:id/progress` | Get execution progress |
| POST | `/api/orchestration/:id/cancel` | Cancel session |
| GET | `/api/orchestration/templates` | List team templates |
| GET | `/api/orchestration/templates/:name` | Get template details |

#### Multi-Agent & Teams
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/agents/collaborate` | Multi-agent collaboration |
| POST | `/api/agents/run` | Run single agent |
| GET | `/api/agents/roles` | Get agent role configs |
| CRUD | `/api/teams` | Team management |
| CRUD | `/api/teams/:id/agents` | Team member management |
| CRUD | `/api/teams/:id/tasks` | Team task management |
| CRUD | `/api/teams/agents` | Agent registration |

#### Knowledge Base & RAG
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/knowledge` | Knowledge entries |
| POST | `/api/knowledge/search` | Semantic search |
| POST | `/api/knowledge/hybrid-search` | Hybrid search |
| CRUD | `/api/knowledge/categories` | Knowledge categories |
| CRUD | `/api/documents` | Document management |
| POST | `/api/documents/import` | Import document |
| POST | `/api/rag/query` | RAG query |
| POST | `/api/rag/stream` | Streaming RAG query |

#### Memory
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/memory` | Create memory |
| GET | `/api/memory/user/:id` | Get user memories |
| GET | `/api/memory/search` | Search memories |
| PUT | `/api/memory/:id` | Update memory |
| DELETE | `/api/memory/:id` | Delete memory |
| POST | `/api/memory/adjust-priority` | Adjust memory priorities |

#### Thinking & Reasoning
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/thinking/thoughts` | Thought management |
| CRUD | `/api/thinking/thoughts/:id/corrections` | Correction management |
| POST | `/api/thinking/cot` | Chain-of-Thought reasoning |
| POST | `/api/thinking/multi-step` | Multi-step reasoning |
| POST | `/api/thinking/tree-of-thought` | Tree-of-Thought reasoning |
| GET | `/api/thinking/visualizations/:id` | Reasoning visualization |
| GET | `/api/thinking/timelines/:id` | Reasoning timeline |

#### Tools & Skills
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/tools` | Tool management |
| POST | `/api/tools/execute` | Execute tool |
| GET | `/api/tools/definitions` | Get LLM tool definitions |
| CRUD | `/api/skills` | Skill management |
| GET | `/api/skills/:id/parameters` | List skill parameter definitions |
| POST | `/api/skills/:id/parameters` | Create skill parameter definition |
| PUT | `/api/skills/:id/parameters/:paramId` | Update skill parameter definition |
| DELETE | `/api/skills/:id/parameters/:paramId` | Delete skill parameter definition |
| GET | `/api/skills/:id/executions` | Get skill execution history |
| POST | `/api/skills/match` | Auto-match skill |
| POST | `/api/skills/execute-matched` | Execute matched skill |
| CRUD | `/api/mcp/servers` | MCP server management |
| GET | `/api/mcp/servers/:id/tools` | List MCP server tools |
| POST | `/api/mcp/servers/:id/refresh` | Refresh MCP tools |
| POST | `/api/mcp/servers/:id/reconnect` | Reconnect MCP server |

#### Plugins & Automation
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/plugins` | Plugin management |
| POST | `/api/plugins/install` | Install plugin |
| POST | `/api/plugins/:id/enable` | Enable plugin |
| POST | `/api/plugins/:id/disable` | Disable plugin |
| CRUD | `/api/automation/executions` | Automation executions |
| POST | `/api/automation/executions/:id/execute` | Execute automation |

#### Tasks & Workflows
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/tasks` | Task management |
| POST | `/api/tasks/decompose` | Decompose task |
| CRUD | `/api/workflows` | Workflow management |
| POST | `/api/workflows/:id/instances` | Create workflow instance |
| POST | `/api/workflows/instances/:id/execute` | Execute workflow |

#### Models & Planning
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/models` | Model management |
| POST | `/api/models/:id/enable` | Enable model |
| POST | `/api/models/:id/disable` | Disable model |
| POST | `/api/plan/execute` | Execute plan |
| GET | `/api/plan/:sessionId` | Get plan status |
| POST | `/api/plan/cancel` | Cancel plan |

#### Scheduler, Sandbox, Voice, Code
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/scheduler/tasks` | Scheduled tasks |
| CRUD | `/api/scheduler/reminders` | Reminders |
| GET | `/api/sandbox/status` | Sandbox status |
| GET | `/api/sandbox/languages` | Supported languages |
| POST | `/api/sandbox/execute` | Execute code in sandbox |
| GET | `/api/voice/status` | Voice service status |
| POST | `/api/voice/tts` | Text-to-speech |
| POST | `/api/voice/stt` | Speech-to-text |
| CRUD | `/api/code/executions` | Code execution records |

#### Other
| Method | Endpoint | Description |
|--------|----------|-------------|
| CRUD | `/api/confirmations` | Confirmation workflows |
| GET/POST | `/api/channels` | Channel management |
| CRUD | `/api/usage/*` | Usage analytics & billing |
| CRUD | `/api/prompt-templates/*` | Prompt templates |
| CRUD | `/api/learning/*` | Adaptive learning |
| CRUD | `/api/feedback/*` | Feedback management |
| CRUD | `/api/context/*` | Context management |
| CRUD | `/api/extraction/*` | Knowledge extraction |
| CRUD | `/api/events/*` | Event bus |
| CRUD | `/api/feishu/*` | Feishu integration |
| WS | `/ws` | WebSocket connection |
| CRUD | `/api/ws/*` | WebSocket management |
| GET | `/health` | Health check |

### Installation

> **Detailed guide**: [INSTALL.md](INSTALL.md)

#### Quick Start

```bash
cd openaide/backend
go mod tidy
go run ./src/main.go
# Server runs on http://localhost:19375
```

#### Build (no CGO required)

```bash
CGO_ENABLED=0 go build -o bin/openaide-server ./src
./bin/openaide-server
```

#### Configuration

Create `~/.openaide/config.json` with your API key:

```json
{
  "models": [{
    "name": "my-model",
    "type": "llm",
    "provider": "openai",
    "api_key": "your-api-key-here",
    "base_url": "https://api.example.com/v1",
    "config": {
      "model": "gpt-4o-mini"
    },
    "status": "enabled"
  }],
  "default_model": "my-model",
  "feishu": {"enabled": false},
  "voice": {"enabled": false},
  "sandbox": {"enabled": false},
  "embedding": {"enabled": false}
}
```

> See `server_config.example.json` for a template.

#### Terminal CLI

```bash
cd openaide/terminal
go run main.go
```

Interactive mode:
```
  ╭─────────────────────────────╮
  │  OpenAIDE CLI               │
  ╰─────────────────────────────╯

    API:   http://localhost:19375/api
    Model: gpt-4o-mini
    Mode:  Streaming

  Type /help for commands, exit or /exit to quit
```

#### Prerequisites
- Go 1.20+
- Docker (optional, for sandbox)
- Python/Node.js (optional, for code execution tools)

---

<a name="中文"></a>
## 中文

一个全功能 AI Agent 开发平台，支持多模型接入、多 Agent 协作、知识库 RAG、工作流编排和企业级集成。

### 功能特性

#### 核心能力
- **18+ 大模型提供商**：统一接口接入 OpenAI、Claude、Gemini、通义千问、DeepSeek、GLM、Ollama 等
- **多 Agent 协作**：角色化 Agent（架构师、开发者、审查员、研究员、PM、测试员）团队协作
- **智能编排**：任务分析 → 团队规划 → 确认审批 → 自动执行的全流程
- **知识库 & RAG**：文档导入、向量嵌入、语义搜索、检索增强生成
- **三层记忆架构**：工作记忆、短期记忆（对话摘要+TTL）、长期记忆（事实/偏好/流程/上下文+衰减机制）
- **5 层 System Prompt**：基础 Prompt → 优化建议 → 用户偏好 → 记忆上下文 → RAG 上下文
- **思考推理**：思维链(CoT)、多步推理、思维树(ToT)、纠正系统、可视化
- **工具调用框架**：HTTP 请求、代码执行、网页搜索、天气查询、文件读写，以及 MCP 外部工具接入，内置 SSRF 防护
- **工作流引擎**：步骤编排、状态机、回滚恢复、调度执行
- **插件系统**：安装、启用/禁用、配置、执行
- **技能系统**：模块化技能管理，支持自动匹配、参数提取、类型归一化、工具绑定与执行追踪
- **任务管理**：CRUD、分解、依赖关系(DAG)、进度跟踪、团队分配
- **定时调度**：Cron 定时任务、Webhook 触发、脚本执行
- **安全沙箱**：Docker 隔离代码执行，多语言支持，安全校验
- **语音服务**：语音转文字(Whisper)、文字转语音，可配置提供商
- **WebSocket**：实时双向通信、流式对话
- **飞书集成**：企业消息机器人、卡片回调，以及 `/skill` 命令的技能匹配与执行
- **自适应学习**：反馈收集、偏好学习、Prompt 自动优化
- **使用量统计**：Token/成本追踪、预算管理、阈值邮件告警
- **认证鉴权**：JWT Token、API Key、角色权限控制

### 系统架构

```
┌──────────────┐     ┌──────────────────────────────────────────────┐     ┌──────────────┐
│  Web / CLI   │────▶│              Backend (Gin)                   │────▶│  18+ LLMs    │
│  / 飞书       │     │  ┌─────┐ ┌──────┐ ┌──────┐ ┌─────────────┐ │     │  提供商      │
└──────────────┘     │  │认证  │ │对话  │ │工具  │ │  智能编排    │ │     └──────────────┘
                     │  └─────┘ └──────┘ └──────┘ └─────────────┘ │
                     │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐ │     ┌──────────────┐
                     │  │知识库 │ │ RAG  │ │记忆  │ │ 定时调度  │ │────▶│   SQLite     │
                     │  └──────┘ └──────┘ └──────┘ └──────────┘ │     │   + 向量存储  │
                     │  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐ │     └──────────────┘
                     │  │团队  │ │技能  │ │插件  │ │ 语音/WS  │ │
                     │  └──────┘ └──────┘ └──────┘ └──────────┘ │
                     └──────────────────────────────────────────────┘
```

**技术栈**：Go 1.20+ / Gin / GORM / SQLite (modernc, CGO_ENABLED=0)

### 安装

> **详细指南**：[INSTALL.md](INSTALL.md)

```bash
cd openaide/backend
go mod tidy
go run ./src/main.go
# 服务运行在 http://localhost:19375
```

#### 零 CGO 编译

```bash
CGO_ENABLED=0 go build -o bin/openaide-server ./src
./bin/openaide-server
```

#### 配置文件

在 `~/.openaide/config.json` 中创建配置：

```json
{
  "models": [{
    "name": "my-model",
    "type": "llm",
    "provider": "openai",
    "api_key": "your-api-key-here",
    "base_url": "https://api.example.com/v1",
    "config": {
      "model": "gpt-4o-mini"
    },
    "status": "enabled"
  }],
  "default_model": "my-model",
  "feishu": {"enabled": false},
  "voice": {"enabled": false},
  "sandbox": {"enabled": false},
  "embedding": {"enabled": false}
}
```

> 参考 `server_config.example.json` 模板文件。

#### 终端 CLI

```bash
cd openaide/terminal
go run main.go
```

交互式模式：
```
  ╭─────────────────────────────╮
  │  OpenAIDE CLI               │
  ╰─────────────────────────────╯

    API:   http://localhost:19375/api
    Model: gpt-4o-mini
    Mode:  Streaming

  Type /help for commands, exit or /exit to quit
```

### API 端点总览

> 完整 API 文档参见 [backend/API.md](backend/API.md)

| 分组 | 路由前缀 | 说明 |
|------|----------|------|
| 认证 | `/api/auth/*` | 注册、登录、Token 刷新 |
| 用户 | `/api/profile` | 用户信息管理 |
| 会话 | `/api/sessions` | 会话管理 |
| 对话 | `/api/dialogues/*` | 对话 CRUD + 消息管理 + 流式 |
| 聊天 | `/api/chat/*` | 普通聊天 / 流式 / 工具调用 / 自动路由 / 规划 |
| MCP | `/api/mcp/*` | MCP Server 管理 / 工具发现 / 重连 / 刷新 |
| 编排 | `/api/orchestration/*` | 智能编排全流程 |
| Agent | `/api/agents/*` | 多 Agent 协作 / 单 Agent 运行 |
| 团队 | `/api/teams/*` | 团队管理 / 成员 / 任务 |
| 知识库 | `/api/knowledge/*` | 知识条目 / 分类 / 搜索 |
| 文档 | `/api/documents/*` | 文档管理 / 导入 |
| RAG | `/api/rag/*` | 检索增强生成 |
| 记忆 | `/api/memory/*` | 三层记忆管理 |
| 推理 | `/api/thinking/*` | CoT / 多步 / 思维树 / 纠正 |
| 工具 | `/api/tools/*` | 工具注册 / 执行 / 定义 |
| 技能 | `/api/skills/*` | 技能管理 / 参数定义 / 匹配 / 执行 / 执行历史 |
| 插件 | `/api/plugins/*` | 插件管理 / 安装 / 启用 |
| 自动化 | `/api/automation/*` | 自动化执行管理 |
| 任务 | `/api/tasks/*` | 任务管理 / 分解 / 进度 |
| 工作流 | `/api/workflows/*` | 工作流编排 / 实例执行 |
| 模型 | `/api/models/*` | 模型管理 / 启用 / 禁用 |
| 规划 | `/api/plan/*` | 规划执行 / 状态 / 取消 |
| 调度 | `/api/scheduler/*` | 定时任务 / 提醒 |
| 沙箱 | `/api/sandbox/*` | 安全代码执行 |
| 语音 | `/api/voice/*` | TTS / STT |
| 代码 | `/api/code/*` | 代码执行记录 |
| 确认 | `/api/confirmations/*` | 审批确认流程 |
| 渠道 | `/api/channels/*` | 多渠道管理 |
| 统计 | `/api/usage/*` | 用量统计 / 预算 / 定价 |
| 模板 | `/api/prompt-templates/*` | Prompt 模板管理 |
| 学习 | `/api/learning/*` | 自适应学习 / 优化 |
| 反馈 | `/api/feedback/*` | 反馈管理 |
| 上下文 | `/api/context/*` | 上下文管理 |
| 提取 | `/api/extraction/*` | 知识提取 |
| 事件 | `/api/events/*` | 事件总线 |
| 飞书 | `/api/feishu/*` | 飞书集成 |
| WS | `/ws`, `/api/ws/*` | WebSocket |
| 健康 | `/health` | 健康检查 |

---

## License | 许可证

MIT License
