# OpenAIDE | AI Agent 内核平台

Go 编写的 AI Agent 内核——**知识积累、自主进化、24 语言 LSP**。提供 REPL + Web + API Server。

> Unlike other agents, OpenAIDE learns from every task. Reflection → Knowledge Refinement → Skill Extraction — it gets smarter with use.

---

## 快速开始

### Linux / macOS

```bash
# 一键安装
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# 交互式配置（推荐）
openaide setup

# 或手动编辑 API Key
vi ~/.openaide/config.yaml

# 启动
openaide
```

### Windows

```cmd
:: 下载 install.bat，双击运行
:: 或命令行：
curl -o install.bat https://raw.githubusercontent.com/lzy1102/openaide/master/install.bat
install.bat
```

详细说明见 [使用指南](docs/USAGE.md)。远程访问见 [远程部署指南](docs/REMOTE_ACCESS.md)。

### 源码编译

```bash
git clone https://github.com/lzy1102/openaide.git
cd openaide/backend
make build
./bin/openaide
```

### 常用命令

```bash
openaide              # REPL 交互模式
openaide setup        # 交互式配置向导（推荐首次使用）
openaide -c           # 恢复上次会话
openaide "问题"        # 一次性问答
openaide --verbose    # 调试模式
```

---

## 配置

`~/.openaide/config.yaml`:

```yaml
# 简单写法（推荐）—— 系统自动识别提供商
llm:
  api_key: sk-xxx              # ← 改成你的 API Key
  model: deepseek-v4-pro       # gpt-4o / claude-sonnet-4-6 / deepseek-v4-pro
  execution_model: deepseek-v4-flash  # 快速模型（可选）
  # base_url: https://api.deepseek.com/v1  # 中转站/自定义地址

lang: zh                       # UI 语言：zh / en


# 高级：双模型路由（Architect/Editor 模式）
# llm:
#   providers:
#     - name: deepseek
#       type: openai
#       base_url: https://api.deepseek.com/v1
#       api_key: sk-xxx
#       default_model: deepseek-v4-pro
#       timeout: 300
#       thinking: true
#       reasoning_effort: max
#     - name: deepseek-flash
#       type: openai
#       base_url: https://api.deepseek.com/v1
#       api_key: sk-xxx
#       default_model: deepseek-v4-flash
#       timeout: 120
#   model_routing:
#     reasoning: deepseek-v4-pro
#     execution: deepseek-v4-flash

search:
  searxng_url: http://localhost:8888  # SearXNG 实例（可选）

log:
  level: info
  persist: false
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
| `kernel.max_rounds` | 最大 ReAct 轮数（安全上限，默认50） | `50` |
| `storage.data_dir` | 数据目录 | `~/.openaide/data` |
| `storage.session_store` | 会话存储：`sqlite`/`file`/`memory` | `sqlite` |
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
| **Web 界面** | 团队共享 | `./bin/openaide-server` → http://localhost:8080 |
| **Windows 安装** | 普通用户 | 双击 `install.bat` |
| **PowerShell 安装** | Windows 高级用户 | `powershell -File install.ps1` |
| **本地编译** | 开发调试 | `make build` |
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
│   └── openaide             # 命令行客户端
├── config.yaml              # 用户配置
├── data/                    # 用户数据 (SQLite)
│   ├── sessions.db          # 会话 (SQLite, WAL)
│   ├── knowledge.db         # 知识库 (SQLite + 向量索引)
│   ├── memory.db            # 记忆 (SQLite + 批量嵌入)
│   ├── plugins/             # Claude Code 格式插件
│   ├── prompts/             # 系统提示词
│   ├── traces.jsonl         # 执行追踪
│   └── checkpoints/         # 会话检查点
├── logs/                    # 日志
├── backend/                 # 源代码 (如本地编译)
│   ├── cmd/
│   ├── internal/
│   └── ...
├── docs/                    # 文档
├── install.sh               # Linux/macOS 安装脚本
├── install.bat              # Windows 批处理安装
├── install.ps1              # Windows PowerShell 安装
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
- **RepoMap 符号地图**: Go AST 解析 + regex 多语言，注入 prompt，LLM 一眼看清代码结构
- **Lint/Repair 循环**: 代码修改后自动 lint → 错误反馈 LLM → 修复 → 循环直到干净（联动 ProjectMind 学习）
- **目标驱动执行**: `-y` 模式全程自动化：Planning → Execution → Verification → Self-correction
- **Architect/Editor 模型路由**: analyst/reviewer→推理模型，coder/executor→执行模型
- **预算注入**: LLM 感知剩余轮次，主动收敛而非突然截断
- **ProjectMind 持续学习**: 跨会话积累项目知识，纠正/衰减/清零，越用越聪明

### 对比其他 Agent

| 能力 | Claude Code | Aider | Codex/Cursor | OpenAIDE |
|------|-------------|-------|-------------|----------|
| 知识积累学习 | ❌ | ❌ | ❌ | ✅ |
| 技能自动提取 | ❌ | ❌ | ❌ | ✅ |
| CSP Actor (零锁) | ❌ | ❌ | ❌ | ✅ |
| 分层提示词 | ❌ | ❌ | ❌ | ✅ |
| 向量 ANN 搜索 | ❌ | ❌ | ❌ | ✅ |
| 24 语言 LSP | ✅ | ✅ | ✅ | ✅ |
| 桌面控制 | ❌ | ❌ | ❌ | ✅ |
| Web 前端 | ❌ | ❌ | ❌ | ✅ |
| 移动端接入 | ❌ | ❌ | ❌ | ✅ |

### 核心架构
- **CSP Actor 模型**: Session/Skill/Memory/Knowledge 全采用 Go 原生 CSP (channel 通信)，50→19 锁
- **SQLite 存储**: 会话/知识/记忆统一存储在 `.db` 文件中（WAL 模式，并发安全）
- **向量搜索**: 内存向量缓存 + 随机投影分桶 ANN（O(n/256) 近似检索）
- **知识积累管线**: 反思打分 → 去重合并 → LLM 精炼 → 结构化存储 → 自动技能提取
- **REPL 模式**: Claude 式审批（允许/全部允许/拒绝）、@file 引用、版本显示
- **Web 前端**: 流式聊天 + 仪表盘 + 模型配置 + 设置管理 + 多语言 + 暗色模式
- **Computer Use**: 桌面截图+点击+键盘+拖拽 + 浏览器坐标操作（Linux/macOS/Windows）
- **34个内置工具**: 文件/Git/命令/搜索/知识库/浏览器/桌面控制/任务管理（跨平台）
- **多 Agent Team**: /analyst /coder /reviewer /executor /team，独立会话+模型路由
- **DeepPlan 深度规划**: 研究→方案对比→选择→计划→执行→自修正闭环
- **LLM 全决策引擎**: 角色分配/风险评估/技能检测全部由 LLM 判断
- **插件系统**: Claude Code 官方格式 (skills/MCP/hooks)，支持热加载
- **MCP 协议**: JSON-RPC stdio，外部工具生态

### Web 前端
- **4 个页面**: 对话、仪表盘、模型配置、设置管理
- **Markdown 渲染**: marked.js + highlight.js 代码高亮
- **流式聊天**: SSE 实时输出 + 思考可视化 + 工具调用展示
- **项目管理**: 注册目录 → AI 基于项目目录分析
- **配置管理**: Web 界面编辑 config.yaml，js-yaml 解析
- **多语言**: 中文/English/日本語/한국어
- **暗色模式**: 自动/手动切换
- **动态 API URL**: 自动适配部署 host:port

### 技能系统（自主学习）
- **自动提炼**: 重复查询/工具序列/错误模式自动生成技能
- **LLM 生成**: 技能内容由 LLM 根据实际上下文生成（非模板）
- **置信度反馈**: 用得好 +0.05，用得差 -0.1，低于 0.3 自动禁用
- **Convention→Skill**: 高置信度规则自动升级为技能
- **allowed-tools**: 技能可声明工具白名单（如 code-review 只读）
- **跨系统协同**: Skill ↔ Learner ↔ ProjectMind ↔ Knowledge 全部联通

### 扩展
- **Computer Use**: 桌面截图+点击+键盘+拖拽，浏览器坐标操作（Linux/macOS/Windows）
- **MCP 协议**: JSON-RPC stdio，外部工具生态
- **插件系统**: Claude Code 官方格式兼容 (skills/MCP/hooks)
- **API Server**: HTTP REST + WebSocket + SSE

## CLI

```bash
# Interactive chat
openaide                          # REPL 交互模式
openaide <prompt>                 # One-shot mode
openaide <file.go> <prompt>       # File + prompt
openaide -c                       # Continue last session
openaide -y                       # 全自动模式：跳过所有确认，目标驱动执行
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

# Slash commands (inside REPL)
  /help           Show help
  /clear          Clear chat messages
  /model [name]   Show/switch current model
  /analyst        Analyze task (reasoning model)
  /coder          Code task (execution model)
  /reviewer       Review code
  /executor       Execute/verify
  /team           Full team pipeline
  /sessions       List sessions
  /init           Generate OPENAIDE.md for current project
  /tree           Browse project file tree
  /status         System health & providers overview
  /lang zh/en     Switch language
  /<skill-name>   Activate a skill (builtin or from plugins)

# Keybindings (inside REPL)
  Ctrl+C / Ctrl+D    Quit / stop streaming
  Ctrl+S             Open session list
  Ctrl+R             Search history
  F1 / Ctrl+H        Show help
  ↑ / ↓              Input history
  PgUp / PgDown      Scroll chat
  Tab                补全命令/文件路径
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
