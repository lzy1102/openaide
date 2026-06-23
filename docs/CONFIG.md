# OpenAIDE 配置指南

配置文件: `~/.openaide/config.yaml`

## 最小配置

```yaml
llm:
  api_key: sk-your-api-key
  model: deepseek-v4-pro
```

只需填写 `api_key` 即可使用。系统自动识别提供商和 API 地址。

## 完整配置

```yaml
# 全局语言: zh (中文) / en (English)
lang: zh

# ── LLM 配置 ──
llm:
  # 简单格式（推荐）
  api_key: sk-your-api-key
  model: deepseek-v4-pro              # 推理模型
  execution_model: deepseek-v4-flash  # 快速执行模型（可选）
  # base_url: https://api.deepseek.com/v1  # 中转站自填

  # 或高级：多 provider 格式
  # providers:
  #   - name: deepseek
  #     type: openai
  #     base_url: https://api.deepseek.com/v1
  #     api_key: sk-your-api-key
  #     default_model: deepseek-v4-pro
  #     timeout: 300
  #     thinking: true                 # DeepSeek 思考模式
  #     reasoning_effort: max          # 推理深度: min/low/medium/high/max
  #     json_mode: false               # JSON 模式
  #     strict_tools: true             # 严格工具调用
  #     embedding_model: text-embedding-3-small  # 嵌入模型（知识库/记忆用）
  #     headers:                       # 自定义 HTTP 头
  #       X-Custom: value
  #     enabled: true

  # 双模型路由（成本感知）
  model_routing:
    reasoning: deepseek-v4-pro    # analyst/coder/reviewer 用
    execution: deepseek-v4-flash  # executor/classifier 用

# ── 服务器配置 ──
server:
  host: 0.0.0.0
  port: 8080
  mode: direct          # direct / server / tui

# ── 内核配置 ──
kernel:
  max_rounds: 50        # 安全上限（实际由上下文窗口收敛）
  max_tokens: 200000    # 最大上下文窗口
  min_rounds: 8         # 自适应轮次下限
  max_rounds_cap: 50    # 自适应轮次上限
  distill_enabled: true        # 启用自动技能提取
  knowledge_enabled: true      # 启用自动知识入库
  distill_min_queries: 5       # 触发技能提取的最小相似查询数
  distill_similarity: 0.80     # 余弦相似度阈值
  system_prompt: ""            # 自定义系统提示词（覆盖文件）
  unsafe_mode: true            # 安全模式: true=跳过审批

# ── 规划配置 ──
planning:
  enabled: true
  deep_timeout: 300      # 深度分析超时（秒）
  preview_timeout: 30    # 预览超时（秒）

# ── 存储配置 ──
storage:
  data_dir: ~/.openaide/data
  session_store: sqlite   # file / sqlite / memory

# ── 记忆配置 ──
memory:
  data_dir: ~/.openaide/data/memory

# ── 搜索配置 ──
search:
  searxng_url: http://localhost:8888

# ── 工具配置 ──
tools:
  dangerous_tools:       # 需要审批的工具
    - execute_command
    - write_file

# ── 浏览器配置 ──
browser:
  enabled: false

# ── MCP 配置 ──
mcp:
  enabled: false
  servers:
    - id: my-server
      type: stdio         # stdio / sse
      command: npx
      args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
      # env:
      #   KEY: value
    # - id: remote-server
    #   type: sse
    #   url: http://localhost:3001/sse

# ── 渠道配置 ──
channels:
  webhooks: []           # Webhook 渠道配置
  feishu: []             # 飞书机器人配置
  telegram: []           # Telegram 机器人配置
  task_queue:
    worker_count: 4
    queue_size: 128

# ── 日志配置 ──
log:
  level: info            # debug / info / warn / error
  format: json           # json / text
  persist: false         # 持久化 trace/event（默认 true）
```

## 配置说明

### LLM 格式

支持两种写法：

1. **简单格式**（推荐新手）：只需 `api_key` + `model`。系统自动推断 provider 类型和 API 地址。
2. **Providers 格式**（高级）：多 provider 列表 + 双模型路由。每个 provider 可独立配置超时、Header、思考模式等。

自动推断规则：
- 模型名含 `deepseek` → `api.deepseek.com/v1`
- 模型名含 `claude` 或 `anthropic` → `api.anthropic.com`
- 模型名含 `gpt`/`o1`/`o3`/`o4` → `api.openai.com/v1`

### 双模型路由 (Architect/Editor)

系统按任务类型自动选模型：
- **Reasoning** (reasoning model): 分析、审查、反思——需要深度思考的任务
- **Execution** (execution model): 写代码、执行命令、快速分类——需要低成本快速响应的任务

无需配置即可工作——系统使用默认 provider 完成所有任务。

### 内核调优

| 参数 | 默认 | 说明 |
|------|------|------|
| `max_rounds` | 50 | 硬上限，触发预算耗尽回调 |
| `min_rounds` | 8 | 自适应下限 |
| `max_rounds_cap` | 50 | 自适应上限 |
| `distill_enabled` | true | 是否自动从相似查询中提取技能 |
| `knowledge_enabled` | true | 是否自动保存知识点 |
| `unsafe_mode` | true | 跳过工具审批（生产环境建议 false） |

### MCP 协议

支持两种传输：
- **stdio**: 启动本地进程，通过 stdin/stdout JSON-RPC 通信
- **SSE**: 连接远程 MCP 服务器的 Server-Sent Events 端点

MCP 工具自动注册为内核工具，与内置工具统一调用。

## 环境变量

| 变量 | 说明 |
|------|------|
| `LANG` | 界面语言 (`zh_CN.UTF-8` = 中文，其他 = English) |
| `HOME` | 配置和数据目录的父目录 |
| `INSTALL_DIR` | 安装目录 (默认 `~/.openaide`) |
| `VERSION` | 安装时指定版本 |
