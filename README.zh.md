# OpenAIDE — AI Agent 内核

[English](README.md) | [中文](#)

**Go 实现的 AI Agent 内核——知识积累、自主进化、每次任务都在变聪明。** 
CSP Actor · SQLite · 向量 ANN · 40 工具 · 24 语言 LSP

> 反思 → 知识精炼 → 技能提取 —— 越用越聪明。

---

## 快速开始

```bash
# 一键安装
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# 交互式配置向导
openaide setup

# 启动
openaide
```

```bash
# 源码编译
git clone https://github.com/lzy1102/openaide.git
cd openaide && make install
openaide setup
```

---

## 和其他 Agent 的区别

| 能力 | Claude Code | Aider | Cursor/Codex | OpenAIDE |
|------|-------------|-------|-------------|----------|
| 知识积累学习 | ❌ | ❌ | ❌ | ✅ 反思→蒸馏→技能 |
| 技能自动提取 | ❌ | ❌ | ❌ | ✅ 语义聚类+LLM蒸馏 |
| 过程监督 | ❌ | ❌ | ❌ | ✅ 逐步执行评估 |
| 思维树探索 | ❌ | ❌ | ❌ | ✅ 多路径并联+优选 |
| 自奖励学习 | ❌ | ❌ | ❌ | ✅ 自适应评估标准 |
| Agent 记忆管理 | ❌ | ❌ | ❌ | ✅ MemGPT 归档/检索 |
| ACI (工具界面) | ❌ | ❌ | ❌ | ✅ 结构化输出+验证 |
| CSP Actor (零锁) | ❌ | ❌ | ❌ | ✅ |
| 分层提示词 L0-L6 | ❌ | ❌ | ❌ | ✅ 按任务适配 |
| 向量 ANN 搜索 | ❌ | ❌ | ❌ | ✅ 分桶 O(n/256) |
| 桌面控制 | ❌ | ❌ | ❌ | ✅ 跨平台 |
| Web 前端 | ❌ | ❌ | ❌ | ✅ |
| 移动端接入 | ❌ | ❌ | ❌ | ✅ 飞书/Telegram |
| 插件热加载 | ❌ | ❌ | ❌ | ✅ Claude 兼容 |
| 24 语言 LSP | ✅ | ✅ | ✅ | ✅ |

---

## 架构

```
┌──────────────────────────────────────────────┐
│  openaide server (API)    openaide (REPL)     │
├──────────────────────────────────────────────┤
│  infra/Application — DI 容器                  │
├──────────────────────────────────────────────┤
│  orchestration/    api/       channel/       │
│  Plan→Execute      REST API   Feishu/Telegram│
├──────────────────────────────────────────────┤
│  kernel/AgentKernel — ReAct 循环             │
│  ├─ kernel/actor/  (CSP Actor, SafeMap)      │
│  ├─ kernel/trace/  (Tracer, Checkpointer)    │
│  └─ kernel/graph/  (DAG topo sort)           │
├──────────────────────────────────────────────┤
│  llm/      tools/     memory/   knowledge/   │
│  Gateway   40+ tools  SQLite    Vector ANN   │
└──────────────────────────────────────────────┘
```

### 技术栈

- **语言**: Go 1.25+ (纯 Go，CGO_ENABLED=0)
- **存储**: SQLite (WAL 模式) — sessions、knowledge、memory
- **并发**: CSP Actors + SafeMap + atomic.Value
- **LLM**: OpenAI 兼容 + Anthropic 原生 API
- **LSP**: 24 语言通过 stdio JSON-RPC
- **REPL**: lmorg/readline + glamour + pterm

---

## 核心能力

### Agent 能力
- **ReAct 循环**: 推理 → 行动 → 观察 → 重复。主 Agent 带完整分层提示词 (L0-L6)
- **DeepPlan 深度规划**: 研究 → 提案 → 选择 → 计划 → 执行 → 验证
- **多 Agent Team**: 4 个角色 (analyst/coder/reviewer/executor)，含反提示词约束、Mini ReAct 循环、角色限定的工具集、隔离会话
- **MemGPT 记忆管理**: Agent 主动管理自己的记忆——归档、检索、存储核心事实
- **过程监督**: 逐步评估每次执行，识别最佳和最差决策
- **ACI 工具**: Agent 友好的工具输出（前后对比、行号、验证反馈）
- **Architect/Editor 模型路由**: 推理模型做分析，执行模型写代码

### 知识系统——越用越聪明
- **思维树探索**: 决策点多路径并联探索，LLM 评估选最优方案
- **自奖励学习**: 按任务类型自适应评估标准，越用越精确
- **过程监督**: 逐步评估精确定位哪个决策好、哪个需要改进
- **隐式反馈**: LLM 从对话流推断用户满意度，无需打断提问
- **知识精炼**: 去重 → LLM 提取 → 复合评分检索（相关性+重要性+时效性）
- **技能提取**: 重复成功模式 → 自动创建技能
- **ProjectMind**: 跨会话项目知识积累

### 开发工具
- **40 个内置工具**: 文件、Git、Web、知识库、浏览器、桌面、LSP
- **verify_claim**: 报告前验证工具——防止误判
- **批量嵌入**: N 条消息一次 API 调用
- **嵌入缓存**: LRU 缓存的查询嵌入

### 用户体验
- **REPL**: 文件持久化历史、Tab 补全、语法提示
- **Claude 式审批**: 允许 / 全部允许 / 拒绝
- **交互式配置向导**: 语言 → 提供商 → API Key → 模型
- **Web 前端**: 流式聊天、仪表盘、设置
- **远程访问**: Cloudflare Tunnel、frp、VPS、Tailscale

---

## 配置

`~/.openaide/config.yaml`:

```yaml
llm:
 api_key: sk-xxx            # 你的 API Key
 model: deepseek-v4-pro        # gpt-4o / claude-sonnet-4-6
 execution_model: deepseek-v4-flash  # 快速模型（可选）

lang: zh                # UI 语言：zh / en

storage:
 session_store: sqlite         # sqlite / file / memory
```

---

## 命令行

```bash
openaide              # 交互式 REPL
openaide "问题"       # 一次性问答
openaide -c           # 恢复上次会话
openaide -y           # 自动审批所有工具
openaide --model <名称>   # 覆盖模型
openaide --verbose    # 调试日志
openaide setup        # 交互式配置向导
openaide sessions     # 列出会话
openaide plugins      # 列出插件
openaide update       # 更新
openaide server       # 启动 API 服务（Web 模式）
openaide server --config /path/to/config.yaml  # 使用自定义配置启动服务
```

---

## API

| 端点 | 方法 | 说明 |
|------|------|------|
| `/health` | GET | 健康检查 |
| `/metrics` | GET | Prometheus 指标 |
| `/api/v1/chat` | POST | 聊天 |
| `/api/v1/chat/stream` | POST | 流式聊天 (SSE) |
| `/api/v1/sessions` | GET | 会话列表 |
| `/api/v1/sessions/{id}` | GET | 会话历史 |
| `/api/v1/tools` | GET | 工具列表 |
| `/api/v1/stats` | GET | 系统统计 |

---

## 参与贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。

---

## License

MIT License — 见 [LICENSE](LICENSE)。
