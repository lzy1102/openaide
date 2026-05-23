# OpenAIDE | AI Agent 内核平台

基于全新架构的 AI Agent 内核，采用分层设计：Kernel → Orchestration → API → Infrastructure。

---

## 快速开始

### 1. 一键部署（推荐）

从 GitHub 最新 Release 下载并部署：

```bash
# 使用 curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | sudo bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | sudo bash
```

部署完成后：
- 服务运行在 `http://localhost:8080`
- 配置文件：`~/.openaide/config.yaml`
- 日志：`~/.openaide/logs/server.log`
- 默认管理员：`admin / admin123`

### 2. 本地编译部署

```bash
# 克隆代码到用户目录
git clone https://github.com/lzy1102/openaide.git ~/.openaide
cd ~/.openaide

# 运行安装脚本（本地编译模式）
bash install.sh --local
```

### 3. Docker 部署

```bash
cd ~/.openaide/backend
docker-compose up -d
```

---

## 配置说明

配置文件位置：`~/.openaide/config.yaml`（支持 JSON/YAML）

首次部署会自动创建默认配置，**必须修改 API Key** 才能使用。

### 最小可用配置

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "mode": "server"
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
openaide-cli

# 启动/停止服务（systemd 或手动运行 openaide-server）
openaide-server
```

---

## 项目结构

```
~/.openaide/
├── bin/
│   ├── openaide-server      # API 服务器
│   └── openaide-cli         # 命令行客户端
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
1. 编译 `openaide-server` 和 `openaide-cli`
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

- **22个内置工具**: 文件读写/执行命令/列目录/搜索/Git深度(diff/log/blame)/知识库/代码符号索引
- **联网搜索**: web_search/web_fetch/ai_search (DuckDuckGo，无需API Key)
- **浏览器自动化**: Headless Chrome (browser_navigate/extract/screenshot/click/fill)
- **Diff编辑**: 精确search/replace + 行号替换
- **多模态Vision**: base64图片→Vision API格式 (OpenAI+Anthropic)
- **5个内置Skill + 自动进化**: 代码审查/Git提交/调试/重构/解释（自动检测触发），重复模式自动生成新技能
- **模型智能路由**: 按任务类型自动选择provider/model（代码/搜索/翻译/对话）
- **DeepPlan 深度规划**: 研究→方案对比→选择→计划→TDD执行→测试→验收，自反思闭环（验收不通过自动返工）
- **多Agent Team + 隔离子Agent**: 分析员→程序员→审查员→执行者，独立会话上下文隔离
- **LLM全决策引擎**: 17项硬编码规则替换，角色分配/风险评估/技能检测等全部由LLM判断
- **OpenCode 配置兼容**: 自动发现 opencode.json，导入MCP服务器+项目指令
- **插件市场**: `openaide plugins [list|search|install]` CLI命令
- **LLM反思+跨会话学习**: 每轮LLM结构化反思(function calling)→eval→insights持久化→后续对话自动注入历史经验
- **知识库**: 质量门控自动抽取(score>0.6)+提示词注入 + LLM Embedding语义搜索 + 使用反馈闭环(自动评估有效性)
- **3级记忆**: L1/L2/L3 + LLM Embedding语义搜索 + TF-IDF向量搜索 + 文本匹配三级降级
- **LLM上下文压缩**: LLM驱动的语义摘要 + 待解决问题提取，NovelCompressor降级
- **会话检查点**: 文件JSON，崩溃可恢复会话进度
- **事件持久化**: 事件总线持久化到磁盘，支持回放
- **LLM网关**: OpenAI+Anthropic原生+DeepSeek(thinking)+文本向量化(Embedding)+故障转移
- **Embedder接口**: 可扩展的文本向量化接口，支持配置独立 embedding model
- **MCP协议**: JSON-RPC stdio，外部工具生态接入
- **WebSocket**: 双向流式+心跳
- **插件系统**: Claude Code 官方格式兼容 (skills/MCP/hooks)，`./data/plugins/` 即装即用，自动发现 SKILL.md/.mcp.json/hooks.json
- **TUI**: Bubbletea (AltScreen可选) + 流式渲染 + 思考过程显示
- **认证**: JWT默认关闭，OPENAIDE_AUTH=true开启

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
