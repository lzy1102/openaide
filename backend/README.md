# OpenAIDE Backend

Go 编写的 AI Agent 内核。分层架构：Kernel → Orchestration → API → Infrastructure。

## 快速开始

```bash
cd backend
make build
# 输出: bin/openaide-server, bin/openaide

# 启动 API 服务
make run
# 服务运行在 http://localhost:8080

# 交互式 REPL
./bin/openaide
```

## 项目结构

```
backend/
├── cmd/
│   ├── server/main.go     # API 服务器入口
│   └── cli/                # REPL 交互式 CLI
│       ├── main.go         # 入口、参数解析
│       ├── repl.go         # REPL 主循环 (lmorg/readline + glamour)
│       ├── repl_output.go  # Markdown 渲染、pterm 样式
│       └── utils.go        # 工具函数
├── internal/
│   ├── infra/              # DI 容器 (Application)
│   ├── kernel/             # Agent 内核 (ReAct、流式、反思、Skill、提示词)
│   ├── llm/                # LLM 网关 (OpenAI 兼容 + Anthropic 原生)
│   ├── tools/              # 22 个内置工具 (文件/命令/Git/知识库/符号/浏览器)
│   ├── memory/             # 3 级文件记忆 + 语义搜索
│   ├── orchestration/      # 编排器 + DeepPlan + Team 多 Agent
│   ├── api/                # REST + SSE + WebSocket + JWT
│   ├── plugin/             # Claude Code 插件格式兼容
│   ├── mcp/                # MCP 协议 (stdio)
│   ├── compress/           # LLM 语义压缩
│   ├── knowledge/          # 知识库
│   ├── event/              # 事件总线
│   ├── channel/            # 外部渠道 (Webhook/飞书/Telegram)
│   ├── config/             # YAML/JSON 配置
│   ├── auth/               # JWT 认证
│   ├── lang/               # 多语言 (zh/en)
│   └── projectmind/        # 持续学习 (代码地图/风险/约定)
├── Makefile
├── Dockerfile
└── docker-compose.yml
```

## 技术栈

- **Go 1.25**, 纯标准库 `net/http` (非 Gin)
- **文件 JSON 存储** (非 GORM/SQLite)
- **REPL**: lmorg/readline v4 + glamour (Markdown) + pterm (富文本)
- **LLM**: OpenAI 兼容协议 + Anthropic 原生 API
- **WebSocket**: gorilla/websocket
- **认证**: JWT HMAC-SHA256

## 配置

`~/.openaide/config.yaml`:

```yaml
llm:
  providers:
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-xxx
      default_model: deepseek-v4-pro
      timeout: 300
      thinking: true
      reasoning_effort: max
    - name: deepseek-flash
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-xxx
      default_model: deepseek-v4-flash
      timeout: 120
  model_routing:
    reasoning: deepseek-v4-pro   # analyst, reviewer
    execution: deepseek-v4-flash # coder, executor, synthesis
kernel:
  max_rounds: 30
  min_rounds: 8
log:
  level: info
  lang: zh
```

**Architect/Editor 模型路由**: analyst/reviewer → reasoning model (pro)，coder/executor → execution model (flash)。

## API 端点

| 路由 | 方法 | 说明 |
|------|------|------|
| `/api/v1/chat` | POST | 同步聊天 |
| `/api/v1/chat/stream` | POST | 流式聊天 (SSE) |
| `/api/v1/sessions` | GET/POST | 会话列表 / 创建 |
| `/api/v1/sessions/{id}` | GET/DELETE | 会话详情 / 删除 |
| `/api/v1/memory/search?q=` | GET | 记忆搜索 |
| `/api/v1/tools` | GET | 工具列表 |
| `/api/v1/stats` | GET | 系统统计 |
| `/api/v1/metrics` | GET | 运行时指标 |
| `/api/v1/config` | GET/PUT | 读写配置 |
| `/api/v1/projects` | GET/POST | 项目列表 / 创建 |
| `/api/v1/projects/{id}` | GET/PUT/DELETE | 单个项目操作 |
| `/api/v1/channels` | GET | 渠道列表 |
| `/api/v1/auth/register` | POST | 注册 |
| `/api/v1/auth/login` | POST | 登录 |
| `/ws` | GET | WebSocket |
| `/health` | GET | 健康检查 |

## 测试

```bash
make test
make test-coverage
go test -v ./internal/kernel/...
```

## License

MIT
