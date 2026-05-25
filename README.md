# OpenAIDE | AI Agent 内核平台

Go 编写的 AI Agent 内核，支持 CLI（REPL + TUI）和 HTTP API Server。

---

## 快速开始

```bash
git clone https://github.com/lzy1102/openaide.git
cd openaide/backend
make build

# 配置 API Key
mkdir -p ~/.openaide && cp config.example.yaml ~/.openaide/config.yaml
# 编辑 ~/.openaide/config.yaml，填入你的 API Key

# REPL 模式（默认）
./bin/openaide

# TUI 模式
./bin/openaide --tui

# 恢复上次会话
./bin/openaide -c

# One-shot
./bin/openaide "fix this bug"

# 启动 API 服务器
./bin/openaide-server
```

---

## 配置

`~/.openaide/config.yaml`:

```yaml
llm:
  providers:
    - name: deepseek
      default_model: deepseek-v4-pro
      api_key: sk-xxx
      timeout: 300
    - name: deepseek-flash
      default_model: deepseek-v4-flash
      api_key: sk-xxx
      timeout: 120
  model_routing:
    reasoning: deepseek-v4-pro   # analyst, reviewer
    execution: deepseek-v4-flash # coder, executor, synthesis
kernel:
  max_rounds: 30
  min_rounds: 8
log:
  level: info
  },
  "llm": {
    "default_provider": "openai",
    "providers": [
      {
        "name": "openai",
        "type": "openai",
        "base_url": "https://api.openai.com/v1",
        "api_key": "sk-your-real-api-key",
        "default_model": "gpt-4o-mini",
        "timeout": 60,
        "enabled": true
      }
    ]
  }
}
```

### 配置项说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server.host` | 监听地址 | `0.0.0.0` |
| `server.port` | 监听端口 | `8080` |
| `server.mode` | 运行模式：`server`/`direct` | `server` |
| `llm.default_provider` | 默认 LLM 提供商名称 | `""` |
| `llm.providers` | 提供商列表 | `[]` |
| `memory.data_dir` | 记忆数据目录 | `./data/memory` |
| `kernel.max_rounds` | 最大 ReAct 轮数 | `10` |
| `log.level` | 日志级别：`debug`/`info`/`warn`/`error` | `info` |

### 支持的 LLM 提供商

| 提供商 | type | 示例 base_url |
|--------|------|--------------|
| OpenAI | `openai` | `https://api.openai.com/v1` |
| Anthropic | `anthropic` | `https://api.anthropic.com` |
| DeepSeek | `openai` | `https://api.deepseek.com/v1` |
| 阿里云百炼 | `openai` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| 本地 Ollama | `openai` | `http://localhost:11434/v1` |

> 所有 OpenAI 兼容接口统一使用 `type: "openai"`。`type: "openai-compatible"` 仍可向后兼容。|

---

## 部署方式对比

| 方式 | 适用场景 | 命令 |
|------|----------|------|
| **一键脚本** | 快速部署，生产环境 | `curl ... \| sudo bash` |
| **本地编译** | 开发调试，定制修改 | `sudo bash install.sh --local` |
| **GitHub Release** | 指定版本，离线环境 | `VERSION=v1.0.0 curl ... \| bash` |
| **Docker** | 容器化部署 | `docker-compose up -d` |

---

## 服务管理

```bash
# 查看日志
tail -f ~/.openaide/logs/server.log

# 使用 CLI
openaide

# 启动/停止服务（systemd 或手动运行 openaide-server）
openaide-server
```

---

## 项目结构

```
~/.openaide/
├── bin/
│   ├── openaide-server      # API 服务器
│   └── openaide         # 命令行客户端
├── config.json              # 用户配置
├── data/                    # 用户数据
│   ├── memory/              # 记忆
│   ├── sessions/            # 会话
│   └── knowledge/           # 知识库
├── logs/                    # 日志
├── backend/                 # 源代码 (如本地编译)
│   ├── cmd/
│   ├── internal/
│   └── ...
├── docs/                    # 文档
├── scripts/
│   └── update.sh            # 更新脚本
├── install.sh               # 安装脚本
└── README.md
```

---

## GitHub Actions

每次推送到 `master` 分支会自动：
1. 编译 `openaide-server` 和 `openaide`
2. 运行测试
3. 打包为 `openaide-linux-amd64.tar.gz`

打标签 `v*` 会自动创建 GitHub Release 并上传二进制文件。

```bash
# 发布新版本
git tag v1.0.0
git push origin v1.0.0
```

---

## 核心能力

### 智能增强（Aider 风格）
- **RepoMap 符号地图**: 项目符号自动提取注入 prompt，LLM 一眼看清代码结构
- **Lint/Repair 循环**: 代码修改后自动 lint → 错误反馈 LLM 修复 → 循环直到干净
- **Architect/Editor 模型路由**: analyst/reviewer→推理模型，coder/executor→执行模型
- **预算注入**: LLM 感知剩余轮次，主动收敛而非突然截断
- **ProjectMind 持续学习**: 跨会话积累项目知识，越用越聪明

### 核心架构
- **REPL 默认模式**: rich readline + markdown 渲染 + 代码高亮 + tab 补全
- **TUI 模式** (`--tui`): Bubble Tea 组件化 TUI
- **22个内置工具**: 文件读写/执行命令/Git深度/知识库/代码符号索引
- **多 Agent Team**: /analyst /coder /reviewer /executor /team，独立会话+模型路由
- **DeepPlan 深度规划**: 研究→方案对比→选择→计划→执行→自修正闭环
- **LLM 全决策引擎**: 角色分配/风险评估/技能检测全部由 LLM 判断
- **工具并发安全分区**: 只读工具并行，写工具串行
- **会话检查点**: 文件 JSON，崩溃可恢复

### 扩展
- **MCP 协议**: JSON-RPC stdio，外部工具生态
- **插件系统**: Claude Code 官方格式兼容 (skills/MCP/hooks)
- **Web 前端 + API Server**: HTTP/WebSocket/SSE

## CLI

```bash
# Interactive chat
openaide                          # Bubbletea TUI
openaide <prompt>                 # One-shot mode
openaide <file.go> <prompt>       # File + prompt
openaide -c                       # Continue last session
openaide -y                       # Auto-approve all tool calls
openaide --model <name>           # Override model
openaide --verbose                # Debug logging
openaide -o json                  # Structured JSON output

# Subcommands
openaide sessions                 # List sessions
openaide plugins                  # List installed plugins
openaide plugins search           # Search available plugins
openaide plugins install <url>    # Install plugin from GitHub
openaide update                   # Update
openaide version                  # Version
openaide help                     # Show help

# Slash commands (inside TUI)
  /help           Show help
  /clear          Clear chat messages
  /model [name]   Show/switch current model
  /<skill-name>   Activate a skill (builtin or from plugins)

# Keybindings (inside TUI)
  Ctrl+C / Ctrl+D    Quit / stop streaming
  Ctrl+S             Open session list
  F1 / Ctrl+H        Show help
  ↑ / ↓              Input history
  PgUp / PgDown      Scroll chat
```

## API 端点

| 路由 | 说明 |
|------|------|
| `POST /api/v1/chat` | 聊天 (需认证) |
| `POST /api/v1/chat/stream` | 流式聊天 SSE (需认证) |
| `POST /api/v1/auth/register` | 注册 |
| `POST /api/v1/auth/login` | 登录获取JWT |
| `GET /api/v1/sessions` | 会话列表 (需认证) |
| `GET /api/v1/sessions/{id}` | 会话历史 (需认证) |
| `GET /api/v1/memory/search?q=xxx` | 记忆搜索 (需认证) |
| `GET /api/v1/tools` | 工具列表 (需认证) |
| `GET /api/v1/stats` | 系统统计 (需认证) |
| `GET /ws` | WebSocket (需认证) |
| `GET /health` | 健康检查 |

---

## License

MIT License
