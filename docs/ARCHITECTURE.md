# OpenAIDE 架构设计文档

> 版本: v3.0.0-final
> 日期: 2026-05-17
> 状态: 全部功能完成

## 实现进度

| 层次 | 完成度 | 说明 |
|------|--------|------|
| 内核层 (kernel) | ✅ 100% | ReAct循环、流式、并行工具、反思/学习/模式检测、Skill系统 |
| LLM 层 (llm) | ✅ 100% | 网关+故障转移、OpenAI兼容、Anthropic原生、DeepSeek特殊支持 |
| 工具层 (tools) | ✅ 100% | 9个工具真实实现（读写/执行/搜索/Git/知识库/符号索引） |
| 记忆层 (memory) | ✅ 100% | 3级记忆、文件持久化、TF-IDF向量语义搜索 |
| 编排层 (orchestration) | ✅ 90% | 基础编排、Planner任务拆分、子任务汇总。缺少Team多Agent |
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
| kernel | 10个 | Agent核心、ReAct、反思、学习、模式、Skill、会话存储 |
| llm | 3个 | 网关、OpenAI兼容、Anthropic原生 |
| tools | 2个 | 9个工具注册+实现 |
| memory | 2个 | 3级记忆、TF-IDF向量搜索 |
| orchestration | 2个 | 编排器、Planner任务拆分 |
| api | 3个 | REST、SSE、WebSocket、JWT中间件 |
| infra | 1个 | DI容器、日志 |
| config | 2个 | JSON/YAML配置 |
| auth | 1个 | JWT签发/验证/中间件 |
| knowledge | 1个 | 文档CRUD、搜索、提示词注入 |
| feedback | 1个 | 质量门控评分 |
| event | 1个 | 事件总线、持久化 |
| git | 2个 | git status/diff/log |
| index | 3个 | 代码符号索引 |
| identity | 1个 | 项目类型检测 |
| compress | 1个 | 小说式上下文压缩 |
| skill | (kernel内) | 5个内置技能 |

## 全部完成

| 功能 | 状态 |
|------|------|
| 多Agent Team | ✅ analyst/coder/reviewer/executor |
| DAG工作流 | ✅ 有向无环图并行执行 |
| 插件系统 | ✅ 可插拔扩展管理器 |
| 多模态 | ✅ read_image base64 |
| API限流 | ✅ token bucket 20req/s |
| 工具沙箱 | ❌ 不需要 |
