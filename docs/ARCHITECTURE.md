# OpenAIDE 架构设计

> 日期: 2026-05-27

## 技术栈

- **Go 1.25**, 纯标准库 `net/http` (非 Gin)
- **文件 JSON 存储** (非 GORM/SQLite)
- **REPL**: lmorg/readline v4 + glamour (Markdown) + pterm (富文本)
- **LLM**: OpenAI 兼容协议 + Anthropic 原生 API
- **WebSocket**: gorilla/websocket
- **JWT HMAC-SHA256** 认证

## 分层架构

```
cmd/server (API 服务器)       cmd/cli (交互式 REPL)
         \_________________________/
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
| lang | 1 | 多语言 (zh/en) |

## 核心特性

| 特性 | 说明 |
|------|------|
| **REPL 模式** | lmorg/readline + 历史 + Tab 补全 + glamour Markdown 渲染 + pterm 样式 |
| **Architect/Editor 路由** | analyst/reviewer → reasoning (pro)，coder/executor → execution (flash) |
| **DeepPlan 深度规划** | Research → Propose → Select → Plan → Execute → Review |
| **Team 多 Agent** | /analyst /coder /reviewer /executor /team，独立会话 + 模型路由 |
| **LLM 决策引擎** | 角色分配/风险评估/技能检测/复杂度估算全部由 LLM 判断 |
| **22 个内置工具** | 文件读写/命令执行/Git 深度/知识库/符号索引/浏览器/多模态 |
| **工具并发安全** | 只读工具并行，写工具串行 |
| **预算注入** | LLM 感知剩余轮次，主动收敛 |
| **RepoMap** | 项目符号地图自动注入 prompt |
| **Lint/Repair 循环** | 代码修改后自动 lint → 错误反馈 LLM → 循环直到干净 |
| **ProjectMind** | 跨会话积累项目知识 |
| **插件系统** | Claude Code 官方格式兼容 (skills/MCP/hooks) |
| **MCP 协议** | JSON-RPC stdio，外部工具生态 |
| **Web 前端** | 流式聊天 + 项目管理 + 配置编辑 + 多语言 + 暗色模式 |
| **API Server** | REST + SSE + WebSocket，16 个端点 |
