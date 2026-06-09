# OpenAIDE 架构设计

> 日期: 2026-05-27

## 技术栈

- **Go 1.25**, 纯标准库 `net/http` (非 Gin)
- **SQLite 存储** (WAL 模式，modernc.org/sqlite 纯 Go，CGO_ENABLED=0)
- **CSP Actor 模型**: 核心状态模块采用单 goroutine + channel 通信，50→19 锁
- **REPL**: lmorg/readline v4 + glamour (Markdown) + pterm (富文本)
- **LLM**: OpenAI 兼容协议 + Anthropic 原生 API
- **WebSocket**: gorilla/websocket
- **JWT HMAC-SHA256** 认证

## 分层架构

```
cmd/server (API 服务器)   cmd/cli (交互式 REPL)   cmd/desktop (桌面应用)
         \___________________________|___________________________/
                     |
              infra/Application    ← DI 容器
              /    |    |     \
         api/  orchestration/  channel/
                     |
              kernel/AgentKernel   ← ReAct 循环核心
              /    |    |     \
        llm/   tools/ memory/  kernel/types
```

## 模块清单

| 模块 | 文件数 | 功能 |
|------|--------|------|
| kernel | 22 | Agent 核心、ReAct、流式、反思、学习、Skill、会话存储、提示词 |
| llm | 7 | 网关、OpenAI 兼容、Anthropic 原生、Embedding、缓存 |
| tools | 10 | 22 个工具 (文件/命令/Git/知识库/符号/浏览器/多模态) |
| memory | 3 | 3 级记忆、TF-IDF 语义搜索 |
| orchestration | 3 | 编排器、DeepPlan (研究→方案→计划)、Team 多 Agent |
| api | 3 | REST、SSE、WebSocket、JWT 中间件 |
| infra | 4 | DI 容器、LLM 工厂、Kernel 工厂、Channel 工厂 |
| plugin | 2 | Claude Code 插件格式兼容 (skills/MCP/hooks) |
| config | 2 | YAML/JSON 配置 |
| auth | 1 | JWT 签发/验证/中间件 |
| knowledge | 1 | 文档 CRUD、搜索、提示词注入 |
| compress | 2 | LLM 语义压缩 + 简单压缩降级 |
| channel | 5 | 外部渠道 (Webhook/飞书/Telegram) |
| mcp | 2 | MCP 协议 (stdio, 30s 超时) |
| projectmind | 3 | 持续学习 (代码地图/风险/约定/策略) |
| eval | 2 | LLM-as-judge 评测框架：语义评判 + 基准任务套件 |
| lang | 1 | 多语言 (zh/en) |

## 核心特性

| 特性 | 说明 |
|------|------|
| **CSP Actor 模型** | Session/Skill/Memory/Knowledge 采用 Go 原生 channel 通信，50→19 锁 |
| **SQLite 存储** | 会话/知识/记忆统一 SQLite WAL，纯 Go 实现 |
| **向量搜索** | 内存向量缓存 + 随机投影分桶 ANN |
| **知识积累** | 反思→去重→LLM精炼→结构化存储→自动技能提取 |
| **REPL 模式** | lmorg/readline + 历史 + Tab 补全 + Claude 式审批 + glamour |
| **Architect/Editor 路由** | analyst/reviewer → reasoning (pro)，coder/executor → execution (flash) |
| **DeepPlan 深度规划** | Research → Propose → Select → Plan → Execute → Review |
| **Team 多 Agent** | /analyst /coder /reviewer /executor /team，独立会话 + 模型路由 |
| **LLM 决策引擎** | 所有决策由 LLM 判断：4 分类任务识别、风险评估、技能检测、LLM-as-Judge 评测。零硬编码关键字匹配 |
| **无限 ReAct** | 无轮次上限，LLM 自己决定何时停止。仅 token 预算兜底，不强制合成 |
| **技能蒸馏** | embedding 聚类 + LLM 聚类双路径。单一 LLM 调用完成质量评估+精炼。零硬编码阈值，全提供商兼容 |
| **编码工作流** | 4 阶段工程师思维模型：评估影响→规划方案→小步执行→自检审查 |
| **多语言** | 自动检测 21 种语言项目规范，注入语言特定约定 |
| **34 个内置工具** | 文件/Git/命令/搜索/知识库/浏览器/桌面/多模态 |
| **插件系统** | Claude Code 官方格式兼容 (skills/MCP/hooks)，热加载 |
| **MCP 协议** | JSON-RPC stdio，外部工具生态 |
| **Web 前端** | 流式聊天 + 项目管理 + 配置编辑 + 多语言 + 暗色模式 |
| **API Server** | REST + SSE + WebSocket + Prometheus /metrics |
