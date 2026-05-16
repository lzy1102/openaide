# OpenAIDE 模块职责分工文档

> 版本: v3.0.0-final
> 日期: 2026-05-17

## 模块清单 (全部已接入)

| 模块 | 路径 | 文件数 | 核心职责 |
|------|------|--------|----------|
| **内核** | `internal/kernel/` | 10 | Agent智能核心（ReAct+反思+学习+Skill） |
| **LLM** | `internal/llm/` | 3 | 模型网关+OpenAI兼容+Anthropic原生 |
| **工具** | `internal/tools/` | 2 | 9个工具注册+真实实现 |
| **记忆** | `internal/memory/` | 2 | 3级记忆+TF-IDF向量搜索 |
| **编排** | `internal/orchestration/` | 2 | 编排器+Planner任务拆分 |
| **API** | `internal/api/` | 3 | REST+SSE+WebSocket+JWT |
| **基础设施** | `internal/infra/` | 1 | DI容器+日志初始化 |
| **配置** | `internal/config/` | 2 | JSON/YAML配置管理 |
| **认证** | `internal/auth/` | 1 | JWT签发/验证/中间件 |
| **知识库** | `internal/knowledge/` | 1 | 文档CRUD+搜索+注入 |
| **反馈** | `internal/feedback/` | 1 | 质量门控评分(工具+用户+反思) |
| **事件** | `internal/event/` | 1 | 事件总线+持久化 |
| **Git** | `internal/git/` | 2 | git status/diff/log |
| **索引** | `internal/index/` | 3 | 代码符号索引+工作区扫描 |
| **身份** | `internal/identity/` | 1 | 项目类型检测 |
| **压缩** | `internal/compress/` | 1 | 小说式上下文压缩(章节摘要+悬念钩子) |
| **Entry** | `cmd/server/`, `cmd/cli/` | 2 | API服务器入口、Bubbletea TUI CLI |

## 依赖关系

```
cmd/cli (TUI)  ──→ infra ──→ kernel ──→ llm/tools/memory
cmd/server      ──→ infra ──→ api ──→ orchestration ──→ kernel
                                      └── auth/knowledge/feedback
```

## 未实现模块

| 模块 | 说明 |
|------|------|
| Skill独立模块 | 当前在kernel/skill.go内，未独立成包 |
| Plugin插件系统 | 未开始 |
| 多Agent协作 | Planner已做拆分，Team模式未做 |
| 多模态 | 图片/音频处理未做 |
