# OpenAIDE 架构设计文档

> 版本: v3.0.0-final
> 日期: 2026-05-17
> 状态: 全部功能完成

## 实现进度

| 层次 | 完成度 | 说明 |
|------|--------|------|
| 内核层 (kernel) | ✅ 100% | ReAct循环、流式、并行工具、反思/学习/模式检测、Skill系统 |
| LLM 层 (llm) | ✅ 100% | 网关+故障转移、OpenAI兼容、Anthropic原生、DeepSeek特殊支持 |
| 工具层 (tools) | ✅ 100% | 22个工具（读写/执行/搜索/Git/知识库/符号/浏览器/多模态） |
| 记忆层 (memory) | ✅ 100% | 3级记忆、文件持久化、TF-IDF向量语义搜索 |
| 编排层 (orchestration) | ✅ 100% | DeepPlan管线（研究→方案→选择→计划）、Team多Agent（隔离上下文）、DAG执行、LLM智能路由 |
| API 层 (api) | ✅ 100% | REST + SSE + WebSocket + JWT认证 + CORS |
| 基础设施层 (infra) | ✅ 100% | DI容器、JSON/YAML配置、日志、质量门控 |
| 知识层 (knowledge) | ✅ 100% | 文档CRUD、文本搜索、注入提示词、质量门控自动抽取 |
| 前端 (frontend) | ⚠️ 50% | SPA框架完成，未与后端API完整联调 |

## 技术栈

- **Go 1.25**, 纯标准库 HTTP (非Gin)
- **文件JSON存储** (非GORM)
- **lipgloss + bubbletea** TUI
- **gorilla/websocket** 双向通信
- **TF-IDF** 向量语义搜索
- **JWT HMAC-SHA256** 认证

## 模块清单 (17/17 已接入)

| 模块 | 文件 | 功能 |
|------|------|------|
| kernel | 22个 | Agent核心、ReAct、反思、学习、模式、Skill、会话存储、提示词管理 |
| llm | 7个 | 网关、OpenAI兼容、Anthropic原生、Embedding、缓存 |
| tools | 10个 | 22个工具注册+实现（按领域拆分文件） |
| memory | 3个 | 3级记忆、TF-IDF向量搜索、语义搜索 |
| orchestration | 3个 | 编排器、Planner、Team多Agent |
| api | 3个 | REST、SSE、WebSocket、JWT中间件 |
| infra | 4个 | DI容器、LLM网关工厂、Kernel工厂、Channel工厂 |
| plugin | 2个 | 插件管理、Claude Code格式解析 (.claude-plugin/skills/MCP/hooks) |
| config | 2个 | JSON/YAML配置 |
| auth | 1个 | JWT签发/验证/中间件 |
| knowledge | 1个 | 文档CRUD、搜索、提示词注入 |
| feedback | 1个 | 质量门控评分 |
| event | 1个 | 事件总线、持久化 (max 10k events) |
| git | 2个 | git status/diff/log/blame |
| index | 3个 | 代码符号索引 |
| identity | 1个 | 项目类型检测 |
| compress | 2个 | LLM语义压缩 + SimpleCompressor降级 |
| channel | 5个 | 外部消息渠道 (Webhook/飞书/Telegram) |
| mcp | 2个 | MCP协议 (stdio传输, 30s超时) |
| skill | (kernel内) | 5个内置技能 + Claude SKILL.md兼容 + 自动进化 |

## 全部完成

| 功能 | 状态 |
|------|------|
| 多Agent Team | ✅ analyst/coder/reviewer/executor |
| DAG工作流 | ✅ 有向无环图并行执行 |
| 插件系统 | ✅ Claude Code 官方格式兼容 (skills/MCP/hooks) |
| 多模态 | ✅ read_image → OpenAI Vision + Anthropic 自动转换 |
| 智能路由 | ✅ LLM 自动选择角色管线 + 子任务分配 |
| 隔离子Agent | ✅ 独立会话执行，主Agent上下文不污染 |
| 深度规划 | ✅ Research→Propose→Select→Plan→Execute→Test→Review |
| LLM决策引擎 | ✅ 17项硬编码规则替换为LLM判断 |
| DeepPlan深度规划 | ✅ Research→Propose→Select→Plan→Execute→Test→Review |
| 自反思闭环 | ✅ 审查检测[需要返工]→自动修复→再审查(最多2次) |
| 隔离子Agent | ✅ 独立会话执行，主Agent上下文不受污染 |
| OpenCode配置兼容 | ✅ 自动发现opencode.json MCP+instructions |
| 插件市场 | ✅ openaide plugins list/search/install |
| API限流 | ✅ token bucket 20req/s |
| 持续学习(ProjectMind) | ✅ 跨会话代码地图/风险/约定/策略统计 |
| 知识库+ProjectMind统一 | ✅ 结构化事实自动同步语义搜索知识库 |
| 自适应规划深度 | ✅ LLM分类任务复杂度→三级分流(simple/moderate/complex) |
| 并子Agent | ✅ 按依赖分组→组内并行执行 |
| 语法高亮 | ✅ chroma/Monokai TUI代码块渲染 |
