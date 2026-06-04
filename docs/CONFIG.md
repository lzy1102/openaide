# OpenAIDE 配置指南

配置文件: `~/.openaide/config.yaml`

## 最小配置

```yaml
llm:
  providers:
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key
      default_model: deepseek-v4-pro
```

只需填写 `api_key` 即可使用。

## 完整配置

```yaml
# 服务器配置 (openaide-server)
server:
  host: "0.0.0.0"
  port: 8080

# LLM 提供商
llm:
  default_provider: "deepseek"
  providers:
    # 推理模型 (分析、审查、反思)
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-xxx
      default_model: deepseek-v4-pro
      timeout: 300
      enabled: true

    # 执行模型 (写代码、跑测试) — 可选，用于 Architect/Editor 路由
    - name: deepseek-flash
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-xxx
      default_model: deepseek-v4-flash
      timeout: 120
      enabled: true

    # OpenAI (可选)
    - name: openai
      type: openai
      base_url: https://api.openai.com/v1
      api_key: sk-xxx
      default_model: gpt-4o-mini
      enabled: false

    # Anthropic Claude (可选)
    - name: claude
      type: anthropic
      base_url: https://api.anthropic.com
      api_key: sk-ant-xxx
      default_model: claude-sonnet-4-20250514
      enabled: false

  # 模型路由 (可选 — 需要两个 provider)
  model_routing:
    reasoning: deepseek-v4-pro    # analyst, reviewer, reflection
    execution: deepseek-v4-flash  # coder, executor, classifier

# 内核配置
kernel:
  max_rounds: 10           # ReAct 最大轮次
  max_tokens: 4000         # 上下文窗口
  system_prompt: ""        # 自定义系统提示词 (留空=默认)
  min_rounds: 5            # 自适应轮次下限
  max_rounds_cap: 30       # 自适应轮次上限
  unsafe_mode: true        # 跳过工具审批 (本地开发用)

  # 模式检测 (蒸馏阈值)
  pattern_min_cluster_size: 8       # 多少相似 query 触发蒸馏
  pattern_similarity_threshold: 0.80 # 聚类余弦相似度

# 存储
storage:
  data_dir: ~/.openaide/data       # 数据目录
  session_store: sqlite             # sqlite / file / memory

# 日志
log:
  level: info           # debug / info / warn / error
  format: text          # text / json
  persist_enabled: true # 是否持久化事件和追踪

# 渠道 (Webhook/飞书/Telegram)
channels:
  task_queue:
    worker_count: 4
    queue_size: 128
  webhooks: []      # Webhook 渠道列表
  feishu: []        # 飞书渠道列表
  telegram: []       # Telegram 渠道列表

# 浏览器 (可选)
browser:
  enabled: false
```

## Provider 类型

| 类型 | 支持的服务 |
|------|-----------|
| `openai` | OpenAI, DeepSeek, Ollama, Qwen, 任何 OpenAI 兼容 API |
| `anthropic` | Anthropic Claude |

## Architect/Editor 模式

配置 `model_routing` 后，系统自动分配:
- **Architect** (reasoning model): 分析任务、代码审查、反思评估
- **Editor** (execution model): 写代码、执行命令、分类判断

无需配置即可工作——系统使用默认 provider 完成所有任务。

## 环境变量

| 变量 | 说明 |
|------|------|
| `LANG` | 界面语言 (`zh_CN.UTF-8` = 中文，其他 = English) |
| `HOME` | 配置和数据目录的父目录 |
| `INSTALL_DIR` | 安装目录 (默认 `~/.openaide`) |
| `VERSION` | 安装时指定版本 |
