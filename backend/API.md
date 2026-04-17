# OpenAIDE API Documentation

Complete API reference for the OpenAIDE backend service.

## Table of Contents

- [Authentication](#authentication)
- [Chat & Dialogue](#chat--dialogue)
- [Orchestration](#orchestration)
- [Multi-Agent & Teams](#multi-agent--teams)
- [Knowledge Base & RAG](#knowledge-base--rag)
- [Memory System](#memory-system)
- [Thinking & Reasoning](#thinking--reasoning)
- [Tools & Skills](#tools--skills)
- [MCP](#mcp)
- [Plugins & Automation](#plugins--automation)
- [Tasks & Workflows](#tasks--workflows)
- [Models & Planning](#models--planning)
- [Scheduler](#scheduler)
- [Sandbox & Code](#sandbox--code)
- [Voice](#voice)
- [Prompt Templates](#prompt-templates)
- [Usage Analytics](#usage-analytics)
- [Learning & Feedback](#learning--feedback)
- [Context & Extraction](#context--extraction)
- [Confirmations & Channels](#confirmations--channels)
- [Events](#events)
- [WebSocket](#websocket)
- [Error Responses](#error-responses)

---

## Authentication

Most endpoints require authentication. Include your API token in the request header:

```
Authorization: Bearer <your_token>
```

### Local Mode

For local deployment without authentication:

```bash
export OPENAIDE_LOCAL_MODE=true
```

### Register

**Endpoint:** `POST /api/auth/register`

```json
{
  "username": "user1",
  "password": "password123",
  "email": "user@example.com"
}
```

### Login

**Endpoint:** `POST /api/auth/login`

```json
{
  "username": "user1",
  "password": "password123"
}
```

**Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "...",
  "user": { "id": "...", "username": "user1" }
}
```

### Refresh Token

**Endpoint:** `POST /api/auth/refresh`

### Profile & Sessions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/profile` | Get user profile |
| PUT | `/api/profile` | Update profile |
| POST | `/api/change-password` | Change password |
| GET | `/api/sessions` | List sessions |
| DELETE | `/api/sessions/:id` | Logout session |
| POST | `/api/logout` | Logout all |
| GET | `/api/api-keys` | List API keys |
| POST | `/api/api-keys` | Create API key |
| DELETE | `/api/api-keys/:id` | Revoke API key |

### Admin

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/admin/users` | List users |
| GET | `/api/admin/users/:id` | Get user |
| PUT | `/api/admin/users/:id` | Update user |
| DELETE | `/api/admin/users/:id` | Delete user |

---

## Chat & Dialogue

### Send Chat Message

**Endpoint:** `POST /api/chat`

```json
{
  "model_id": "gpt-4",
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "options": {"temperature": 0.7}
}
```

### Streaming Chat

**Endpoint:** `POST /api/chat/stream`

Returns SSE stream.

### Chat with Tool Calling

**Endpoint:** `POST /api/chat/tools`

```json
{
  "model_id": "gpt-4",
  "user_id": "user_123",
  "dialogue_id": "dlg_456",
  "content": "Search for Go tutorials"
}
```

### Auto-Routed Streaming Chat

**Endpoint:** `POST /api/chat/route`

Automatically selects the best model based on content.

### Chat with Planning

**Endpoint:** `POST /api/chat/plan`

```json
{
  "user_id": "user_123",
  "dialogue_id": "dlg_456",
  "content": "Build a REST API with Go"
}
```

### Model Route Info

**Endpoint:** `GET /api/chat/route-info?content=...`

### Dialogue Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/dialogues` | List all dialogues |
| GET | `/api/dialogues/user/:userID` | List user's dialogues |
| POST | `/api/dialogues` | Create dialogue |
| GET | `/api/dialogues/:id` | Get dialogue |
| PUT | `/api/dialogues/:id` | Update dialogue |
| DELETE | `/api/dialogues/:id` | Delete dialogue |
| GET | `/api/dialogues/:id/messages` | Get messages |
| POST | `/api/dialogues/:id/messages` | Send message |
| POST | `/api/dialogues/:id/stream` | Streaming message (SSE) |
| DELETE | `/api/dialogues/:id/messages` | Clear messages |

---

## Orchestration

### Process Message (Full Pipeline)

**Endpoint:** `POST /api/orchestration/process`

```json
{
  "user_message": "Build a web application with user authentication",
  "user_id": "user_123"
}
```

**Response:**
```json
{
  "session_id": "sess_abc",
  "status": "confirming",
  "analysis": { "task_type": "coding", "complexity": "high" },
  "proposal": { "team": [...], "plan": [...] }
}
```

### Analyze Task Only

**Endpoint:** `POST /api/orchestration/analyze`

### Generate Team Plan Only

**Endpoint:** `POST /api/orchestration/plan`

### Session Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/orchestration/sessions` | List user sessions |
| GET | `/api/orchestration/:session_id` | Get session status |
| GET | `/api/orchestration/:session_id/progress` | Get execution progress |
| POST | `/api/orchestration/:session_id/action` | approve / reject / adjust |
| POST | `/api/orchestration/:session_id/cancel` | Cancel session |

### Team Templates

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/orchestration/templates` | List templates (coding_team, research_team, creative_team, analysis_team, fullstack_team, mixed_team) |
| GET | `/api/orchestration/templates/:name` | Get template details |

---

## Multi-Agent & Teams

### Multi-Agent Collaboration

**Endpoint:** `POST /api/agents/collaborate`

```json
{
  "request": "Design and implement a REST API",
  "max_rounds": 3
}
```

### Run Single Agent

**Endpoint:** `POST /api/agents/run`

```json
{
  "role": "architect",
  "input": "Design the database schema for a blog platform"
}
```

### Get Agent Roles

**Endpoint:** `GET /api/agents/roles`

### Team Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/teams` | Create team |
| GET | `/api/teams` | List teams |
| GET | `/api/teams/:id` | Get team |
| DELETE | `/api/teams/:id` | Delete team |
| GET | `/api/teams/:id/status` | Get team status |
| POST | `/api/teams/:id/agents` | Add member |
| GET | `/api/teams/:id/agents` | List members |
| DELETE | `/api/teams/:id/agents/:agentId` | Remove member |
| PUT | `/api/teams/:id/agents/:agentId/status` | Update member status |
| POST | `/api/teams/:id/agents/:agentId/heartbeat` | Update heartbeat |
| POST | `/api/teams/:id/tasks` | Create task |
| GET | `/api/teams/:id/tasks` | List tasks |
| PUT | `/api/teams/:id/tasks/:taskId/assign` | Assign task |
| POST | `/api/teams/:id/tasks/:taskId/decompose` | Decompose task |
| GET | `/api/teams/:id/progress` | Get progress |

### Agent Registration

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/teams/agents/register` | Register agent |
| POST | `/api/teams/agents/:agentId/unregister` | Unregister agent |
| GET | `/api/teams/agents` | List agents |
| GET | `/api/teams/agents/:agentId` | Get agent status |

---

## Knowledge Base & RAG

### Knowledge Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/knowledge` | List knowledge entries |
| POST | `/api/knowledge` | Create knowledge entry |
| GET | `/api/knowledge/:id` | Get knowledge |
| PUT | `/api/knowledge/:id` | Update knowledge |
| DELETE | `/api/knowledge/:id` | Delete knowledge |
| POST | `/api/knowledge/search` | Semantic search |
| POST | `/api/knowledge/hybrid-search` | Hybrid search |

### Category Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/knowledge/categories` | List categories |
| POST | `/api/knowledge/categories` | Create category |
| GET | `/api/knowledge/categories/:id` | Get category |
| DELETE | `/api/knowledge/categories/:id` | Delete category |

### Document Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/documents` | List documents |
| POST | `/api/documents/import` | Import document |
| GET | `/api/documents/:id` | Get document |
| DELETE | `/api/documents/:id` | Delete document |

### RAG Query

**Endpoint:** `POST /api/rag/query`

```json
{
  "query": "How to implement JWT authentication?",
  "model": "gpt-4",
  "top_k": 5
}
```

### Streaming RAG

**Endpoint:** `POST /api/rag/stream`

Returns SSE stream.

---

## Memory System

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/memory` | Create memory |
| GET | `/api/memory/user/:id` | Get user's memories |
| GET | `/api/memory/search?user_id=...&keyword=...` | Search memories |
| PUT | `/api/memory/:id` | Update memory |
| DELETE | `/api/memory/:id` | Delete memory |
| POST | `/api/memory/adjust-priority` | Trigger priority decay |

**Create Memory Request:**
```json
{
  "user_id": "user_123",
  "content": "User prefers concise code explanations",
  "memory_type": "preference",
  "category": "habit",
  "importance": 4,
  "tags": ["preference", "code", "style"]
}
```

Memory types: `fact`, `preference`, `procedure`, `context`

---

## Thinking & Reasoning

### Thoughts CRUD

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/thinking/thoughts` | List all thoughts |
| POST | `/api/thinking/thoughts` | Create thought |
| GET | `/api/thinking/thoughts/:id` | Get thought |
| DELETE | `/api/thinking/thoughts/:id` | Delete thought |

### Corrections

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/thinking/thoughts/:id/corrections` | Create correction |
| GET | `/api/thinking/thoughts/:id/corrections` | List corrections |
| POST | `/api/thinking/corrections/:id/resolve` | Resolve correction |
| DELETE | `/api/thinking/corrections/:id` | Delete correction |

### Reasoning Modes

**Chain-of-Thought:** `POST /api/thinking/cot`

**Multi-Step Reasoning:** `POST /api/thinking/multi-step`

**Tree-of-Thought:** `POST /api/thinking/tree-of-thought`

### Visualization

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/thinking/visualizations/:id?type=tree` | Get reasoning visualization |
| GET | `/api/thinking/timelines/:id` | Get reasoning timeline |

---

## Tools & Skills

### Tool Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/tools` | List tools |
| GET | `/api/tools/definitions` | Get LLM tool definitions |
| GET | `/api/tools/:name` | Get tool details |
| POST | `/api/tools` | Register tool |
| POST | `/api/tools/execute` | Execute tool |

**Execute Tool Request:**
```json
{
  "name": "web_search",
  "arguments": { "query": "Go concurrency patterns" },
  "dialogue_id": "dlg_123",
  "message_id": "msg_456"
}
```

Built-in tools include time, weather, web search, code execution, file I/O, HTTP requests, JSON parsing, and shell command execution. MCP tools are merged into the same tool definition pool when MCP servers are connected.

### Skill Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/skills` | List skills |
| POST | `/api/skills` | Create skill |
| GET | `/api/skills/categories` | List skill categories |
| POST | `/api/skills/match` | Auto-match skill to content |
| POST | `/api/skills/execute-matched` | Match and execute |
| GET | `/api/skills/:id` | Get skill |
| PUT | `/api/skills/:id` | Update skill |
| DELETE | `/api/skills/:id` | Delete skill |
| POST | `/api/skills/:id/enable` | Enable skill |
| POST | `/api/skills/:id/disable` | Disable skill |
| POST | `/api/skills/:id/execute` | Execute skill |
| GET | `/api/skills/:id/parameters` | List parameter definitions |
| POST | `/api/skills/:id/parameters` | Create parameter definition |
| PUT | `/api/skills/:id/parameters/:paramId` | Update parameter definition |
| DELETE | `/api/skills/:id/parameters/:paramId` | Delete parameter definition |
| GET | `/api/skills/:id/executions` | List execution history |

Skills now support parameter-definition-driven execution: required validation, default value filling, type normalization, natural-language parameter extraction, tool binding, and execution tracking. In the routed chat flow, extracted parameters are also injected into `skill_context` for prompt composition.

---

## MCP

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/mcp/servers` | List MCP servers |
| POST | `/api/mcp/servers` | Add and connect MCP server |
| DELETE | `/api/mcp/servers/:id` | Remove MCP server |
| GET | `/api/mcp/servers/:id/tools` | List tools exposed by a server |
| POST | `/api/mcp/servers/:id/refresh` | Refresh tool discovery |
| POST | `/api/mcp/servers/:id/reconnect` | Reconnect server |

MCP support currently covers server management, initialize handshake, tool discovery, and tool invocation over `stdio` and `sse`. Connected MCP tools are exposed to the LLM through the same tool-calling framework as built-in tools.

---

## Plugins & Automation

### Plugin Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/plugins` | List plugins |
| POST | `/api/plugins` | Create plugin |
| GET | `/api/plugins/:id` | Get plugin |
| PUT | `/api/plugins/:id` | Update plugin |
| DELETE | `/api/plugins/:id` | Delete plugin |
| POST | `/api/plugins/install` | Install plugin |
| POST | `/api/plugins/:id/enable` | Enable plugin |
| POST | `/api/plugins/:id/disable` | Disable plugin |
| POST | `/api/plugins/:id/instances` | Create instance |
| POST | `/api/plugins/instances/:id/execute` | Execute instance |

### Automation

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/automation/executions` | List executions |
| POST | `/api/automation/executions` | Create execution |
| GET | `/api/automation/executions/:id` | Get execution |
| DELETE | `/api/automation/executions/:id` | Delete execution |
| POST | `/api/automation/executions/:id/execute` | Execute |

---

## Tasks & Workflows

### Task Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/tasks` | Create task |
| GET | `/api/tasks` | List tasks |
| GET | `/api/tasks/:id` | Get task |
| PUT | `/api/tasks/:id` | Update task |
| DELETE | `/api/tasks/:id` | Delete task |
| PATCH | `/api/tasks/:id/status` | Update status |
| POST | `/api/tasks/decompose` | Decompose task |
| POST | `/api/tasks/progress` | Update progress |
| GET | `/api/tasks/:id/progress` | Get progress |
| GET | `/api/tasks/overview` | Task overview |
| POST | `/api/tasks/:id/retry` | Retry failed task |
| POST | `/api/tasks/:id/summary` | Generate summary |
| POST | `/api/tasks/:id/cancel` | Cancel task |
| POST | `/api/tasks/:id/reassign` | Reassign task |
| GET | `/api/tasks/:id/can-start` | Check if task can start |
| PATCH | `/api/tasks/subtasks/:id/status` | Update subtask status |

### Workflow Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/workflows` | List workflows |
| POST | `/api/workflows` | Create workflow |
| GET | `/api/workflows/:id` | Get workflow |
| PUT | `/api/workflows/:id` | Update workflow |
| DELETE | `/api/workflows/:id` | Delete workflow |
| POST | `/api/workflows/:id/instances` | Create instance |
| POST | `/api/workflows/instances/:id/execute` | Execute instance |

---

## Models & Planning

### Model Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/models` | List models |
| POST | `/api/models` | Create model |
| GET | `/api/models/:id` | Get model |
| PUT | `/api/models/:id` | Update model |
| DELETE | `/api/models/:id` | Delete model |
| POST | `/api/models/:id/enable` | Enable model |
| POST | `/api/models/:id/disable` | Disable model |
| POST | `/api/models/:id/instances` | Create instance |
| POST | `/api/models/instances/:id/execute` | Execute model |

### Planning

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/plan/execute` | Execute plan |
| GET | `/api/plan/:sessionId` | Get plan status |
| POST | `/api/plan/cancel` | Cancel plan |

---

## Scheduler

### Scheduled Tasks

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/scheduler/tasks` | List tasks |
| POST | `/api/scheduler/tasks` | Create task |
| GET | `/api/scheduler/tasks/:id` | Get task |
| POST | `/api/scheduler/tasks/:id/pause` | Pause task |
| POST | `/api/scheduler/tasks/:id/resume` | Resume task |
| DELETE | `/api/scheduler/tasks/:id` | Delete task |
| POST | `/api/scheduler/tasks/:id/execute` | Execute now |
| GET | `/api/scheduler/tasks/:id/executions` | Execution history |

### Reminders

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/scheduler/reminders` | List reminders |
| POST | `/api/scheduler/reminders` | Create reminder |
| POST | `/api/scheduler/reminders/:id/snooze` | Snooze reminder |
| DELETE | `/api/scheduler/reminders/:id` | Cancel reminder |

---

## Sandbox & Code

### Sandbox

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/sandbox/status` | Sandbox status |
| GET | `/api/sandbox/languages` | Supported languages |
| POST | `/api/sandbox/execute` | Execute code |

```json
{
  "language": "python",
  "code": "print('Hello, World!')"
}
```

### Code Executions

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/code/executions` | List executions |
| POST | `/api/code/executions` | Create execution |
| GET | `/api/code/executions/:id` | Get execution |
| DELETE | `/api/code/executions/:id` | Delete execution |
| POST | `/api/code/executions/:id/execute` | Execute |

---

## Voice

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/voice/status` | Voice service status |
| POST | `/api/voice/tts` | Text-to-speech |
| POST | `/api/voice/stt` | Speech-to-text |

**TTS Request:**
```json
{
  "text": "Hello, this is a test."
}
```

**STT Request:**
```json
{
  "audio": "base64_encoded_audio",
  "format": "wav"
}
```

---

## Prompt Templates

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/prompt-templates` | List templates |
| POST | `/api/prompt-templates` | Create template |
| GET | `/api/prompt-templates/:id` | Get template |
| PUT | `/api/prompt-templates/:id` | Update template |
| DELETE | `/api/prompt-templates/:id` | Delete template |
| POST | `/api/prompt-templates/:id/render` | Render template |
| GET | `/api/prompt-templates/name/:name` | Get by name |
| POST | `/api/prompt-templates/name/:name/render` | Render by name |
| POST | `/api/prompt-templates/:id/versions` | Create version |
| GET | `/api/prompt-templates/name/:name/versions` | Version history |
| POST | `/api/prompt-templates/extract-variables` | Extract variables |
| POST | `/api/prompt-templates/export` | Export templates |
| POST | `/api/prompt-templates/import` | Import templates |

---

## Usage Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/usage/stats?period=month` | Usage statistics |
| GET | `/api/usage/daily?date=2026-01-01` | Daily usage |
| GET | `/api/usage/monthly?year=2026&month=1` | Monthly usage |
| GET | `/api/usage/history?start_date=...&end_date=...` | Usage history |
| GET | `/api/usage/budget` | Get budget |
| POST | `/api/usage/budget` | Set budget |
| GET | `/api/usage/pricing` | Model pricing |
| POST | `/api/usage/pricing` | Update pricing |

**Set Budget Request:**
```json
{
  "monthly_budget": 100.00,
  "daily_budget": 5.00,
  "thresholds": [50, 80, 100],
  "alert_email": "user@example.com"
}
```

---

## Learning & Feedback

### Learning

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/learning/learn` | Learn from feedback |
| GET | `/api/learning/preferences` | Get user preferences |
| POST | `/api/learning/preferences/learn` | Learn preferences |
| POST | `/api/learning/workflows/:id/optimize` | Optimize workflow |
| POST | `/api/learning/evaluate` | Evaluate learning effect |
| GET | `/api/learning/prompts/recommended` | Recommended prompts |
| POST | `/api/learning/prompts/:id/apply` | Apply prompt optimization |
| POST | `/api/learning/interactions` | Record interaction |
| GET | `/api/learning/insights` | Learning insights |
| GET | `/api/learning/records` | Learning records |
| GET | `/api/learning/optimizations/prompts` | Prompt optimizations |
| GET | `/api/learning/optimizations/workflows` | Workflow optimizations |
| POST | `/api/learning/optimizations/workflows/:id/apply` | Apply workflow optimization |
| GET | `/api/learning/interactions/history` | Interaction history |

### Feedback

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/feedback` | Create feedback |
| GET | `/api/feedback/task/:id` | Get task feedback |
| GET | `/api/feedback/average/:type` | Average rating |

---

## Context & Extraction

### Context Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/context/compress` | Compress context |
| POST | `/api/context/summarize` | Summarize context |
| GET | `/api/context/metrics` | Get metrics |
| DELETE | `/api/context/expired` | Clear expired |
| GET | `/api/context/dialogue/:id` | Get dialogue context |

### Knowledge Extraction

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/extraction/extract` | Extract from dialogue |
| POST | `/api/extraction/auto` | Auto-extract |
| POST | `/api/extraction/batch` | Batch extract |
| GET | `/api/extraction/config` | Get extraction config |
| PUT | `/api/extraction/config` | Update extraction config |

---

## Confirmations & Channels

### Confirmations

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/confirmations` | List confirmations |
| POST | `/api/confirmations` | Create confirmation |
| GET | `/api/confirmations/:id` | Get confirmation |
| DELETE | `/api/confirmations/:id` | Delete confirmation |
| POST | `/api/confirmations/:id/confirm` | Confirm task |
| POST | `/api/confirmations/:id/reject` | Reject task |

### Channels

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/channels` | Channel status |
| GET | `/api/channels/enabled` | Enabled channels |

---

## Events

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/events?topic=...` | List events |
| GET | `/api/events/stats` | Event statistics |
| GET | `/api/events/:id` | Get event |
| POST | `/api/events/publish` | Publish event |

```json
{
  "topic": "tool",
  "type": "tool_completed",
  "source": "tool_calling",
  "data": { "tool_name": "web_search" }
}
```

---

## WebSocket

| Endpoint | Description |
|----------|-------------|
| `/ws` | WebSocket connection |
| `/api/ws/stats` | WebSocket statistics |
| `/api/ws/broadcast` | Broadcast message |
| `/api/ws/send/:user_id` | Send to user |
| `/api/ws/notify/task/:id` | Notify task |
| `/api/ws/dialogue/stream` | Streaming dialogue |

---

## Error Responses

All endpoints may return error responses:

```json
{
  "error": "Error message description"
}
```

Common HTTP status codes:
- `400` Bad Request - Invalid request parameters
- `401` Unauthorized - Missing or invalid authentication
- `404` Not Found - Resource not found
- `500` Internal Server Error - Server error
