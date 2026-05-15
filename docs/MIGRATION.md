# OpenAIDE 迁移实施计划

> 版本: v3.0.0-draft  
> 日期: 2026-05-15  
> 状态: 设计评审中  
> 预计周期: 15 周（含增强能力）

---

## 目录

1. [迁移策略](#1-迁移策略)
2. [Phase 1: 内核提取 (Week 1-3)](#phase-1-内核提取-week-1-3)
3. [Phase 2: 记忆系统重构 (Week 3-5)](#phase-2-记忆系统重构-week-3-5)
4. [Phase 3: 工具系统重构 (Week 5-6)](#phase-3-工具系统重构-week-5-6)
5. [Phase 4: LLM 层精简 (Week 6-7)](#phase-4-llm-层精简-week-6-7)
6. [Phase 5: 编排层精简 (Week 7-8)](#phase-5-编排层精简-week-7-8)
7. [Phase 6: API 层重构 (Week 8-9)](#phase-6-api-层重构-week-8-9)
8. [Phase 7: 基础设施迁移 (Week 9-10)](#phase-7-基础设施迁移-week-9-10)
9. [Phase 8: 集成与清理 (Week 10-12)](#phase-8-集成与清理-week-10-12)
10. [风险与缓解](#10-风险与缓解)
11. [验收标准](#11-验收标准)
12. [增强能力迁移计划](#12-增强能力迁移计划)

---

## 1. 迁移策略

### 1.1 总体策略: 并行开发 + 渐进替换

```
现有代码 (services/)          新代码 (internal/)
     │                              │
     │  并行开发，逐步替换            │
     │                              │
     ▼                              ▼
┌─────────────┐              ┌─────────────┐
│ 旧服务运行中  │              │ 新模块开发中  │
│ (保持稳定)   │◄────────────►│ (逐步集成)   │
└─────────────┘              └─────────────┘
     │                              │
     │  接口兼容层                    │
     │  (过渡期)                     │
     │                              │
     ▼                              ▼
┌─────────────────────────────────────────┐
│           最终状态：旧代码删除            │
│           新代码完全接管                 │
└─────────────────────────────────────────┘
```

### 1.2 关键原则

| 原则 | 说明 |
|------|------|
| **保持现有功能运行** | 迁移期间，现有服务必须持续可用 |
| **接口兼容过渡** | 新旧系统通过适配器共存 |
| **小步快跑** | 每个 Phase 产出可验证的代码 |
| **测试驱动** | 每步都有对应的测试覆盖 |
| **随时回滚** | 每个阶段都可回退到上一版本 |

### 1.3 分支策略

```
main (稳定分支)
  │
  ├── feature/kernel-phase-1     # Phase 1: 内核提取
  │     │
  │     └── PR ──► main
  │
  ├── feature/memory-phase-2     # Phase 2: 记忆重构
  │     │
  │     └── PR ──► main
  │
  ├── feature/tools-phase-3      # Phase 3: 工具重构
  │     │
  │     └── PR ──► main
  │
  └── ... (后续 Phase)
```

---

## Phase 1: 内核提取 (Week 1-3)

### 目标
建立 `internal/kernel/` 包，将 Agent 核心能力收敛到内核。

### 任务清单

#### Week 1: 目录结构与接口定义

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 1.1 | 创建目录结构 | `backend/internal/kernel/` | P0 |
| 1.2 | 定义 Agent 接口 | `kernel/interfaces.go` | P0 |
| 1.3 | 定义 Reactor 接口 | `kernel/interfaces.go` | P0 |
| 1.4 | 定义 Event 接口 | `kernel/events.go` | P0 |
| 1.5 | 定义状态机 | `kernel/state_machine.go` | P0 |
| 1.6 | 编写接口测试 | `kernel/interfaces_test.go` | P1 |

#### Week 2: Reactor 实现

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 2.1 | 迁移 AgentExecutor → Reactor | `kernel/reactor.go` | P0 |
| 2.2 | 实现 Think 方法 | `kernel/reactor.go` | P0 |
| 2.3 | 实现 Act 方法 | `kernel/reactor.go` | P0 |
| 2.4 | 实现 Observe 方法 | `kernel/reactor.go` | P0 |
| 2.5 | 实现 React 循环 | `kernel/reactor.go` | P0 |
| 2.6 | 编写 Reactor 单元测试 | `kernel/reactor_test.go` | P1 |

#### Week 3: 集成与验证

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 3.1 | 实现 Agent 实体 | `kernel/agent.go` | P0 |
| 3.2 | 创建兼容适配器 | `kernel/adapter.go` | P0 |
| 3.3 | 集成测试 | `kernel/integration_test.go` | P0 |
| 3.4 | 性能基准测试 | `kernel/bench_test.go` | P1 |
| 3.5 | 文档更新 | `docs/ARCHITECTURE.md` | P1 |

### 代码示例

```go
// kernel/interfaces.go
package kernel

import "context"

// Agent 智能实体接口
type Agent interface {
    Execute(ctx context.Context, req *ExecuteRequest) (<-chan Event, error)
    State() AgentState
    ID() string
}

// ExecuteRequest 执行请求
type ExecuteRequest struct {
    SessionID string
    Message   string
    Context   map[string]interface{}
    Tools     []string
    ModelHint string
    Stream    bool
}

// Event 事件接口
type Event interface {
    Type() EventType
    Timestamp() time.Time
}
```

### 验收标准

- [ ] `internal/kernel/` 目录创建完成
- [ ] 所有接口定义通过编译
- [ ] Reactor 单元测试通过
- [ ] 与现有 AgentExecutor 功能等价
- [ ] 代码覆盖率 > 70%

### 风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| AgentExecutor 被多处引用 | 高 | 高 | 保留旧实现，新增适配器 |
| 事件流设计不满足需求 | 中 | 中 | 先实现最小版本，后续迭代 |

---

## Phase 2: 记忆系统重构 (Week 3-5)

### 目标
建立 `internal/memory/` 包，统一记忆管理。

### 任务清单

#### Week 3: 接口定义与长期记忆

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 4.1 | 创建目录结构 | `backend/internal/memory/` | P0 |
| 4.2 | 定义记忆系统接口 | `memory/interfaces.go` | P0 |
| 4.3 | 迁移 MemoryService | `memory/long_term.go` | P0 |
| 4.4 | 实现记忆存储 | `memory/store.go` | P0 |
| 4.5 | 编写单元测试 | `memory/long_term_test.go` | P1 |

#### Week 4: 工作记忆与向量记忆

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 5.1 | 迁移 DialogueService → 工作记忆 | `memory/short_term.go` | P0 |
| 5.2 | 迁移 ContextEngine → 压缩器 | `memory/compressor.go` | P0 |
| 5.3 | 迁移 RAGService → 向量记忆 | `memory/vector.go` | P0 |
| 5.4 | 实现嵌入服务 | `memory/embedding.go` | P0 |
| 5.5 | 编写集成测试 | `memory/integration_test.go` | P1 |

#### Week 5: 集成与验证

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 6.1 | 实现记忆检索流程 | `memory/recall.go` | P0 |
| 6.2 | 实现关键信息提取 | `memory/extractor.go` | P0 |
| 6.3 | 集成测试 | `memory/integration_test.go` | P0 |
| 6.4 | 性能测试 | `memory/bench_test.go` | P1 |

### 代码示例

```go
// memory/interfaces.go
package memory

import "context"

type System interface {
    Recall(ctx context.Context, query string, sessionID string, limit int) ([]Memory, error)
    Store(ctx context.Context, m Memory) error
    Summarize(ctx context.Context, sessionID string) (string, error)
    Compress(ctx context.Context, messages []Message, maxTokens int) ([]Message, error)
}

type Memory struct {
    ID        string
    Content   string
    Type      MemoryType
    Source    string
    SessionID string
    Embedding []float32
    CreatedAt time.Time
}
```

### 验收标准

- [ ] `internal/memory/` 目录创建完成
- [ ] 三级记忆架构实现完成
- [ ] 记忆检索流程测试通过
- [ ] 与现有 MemoryService 功能等价
- [ ] 代码覆盖率 > 75%

---

## Phase 3: 工具系统重构 (Week 5-6)

### 目标
建立 `internal/tools/` 包，统一工具管理。

### 任务清单

#### Week 5: 注册表与执行器

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 7.1 | 创建目录结构 | `backend/internal/tools/` | P0 |
| 7.2 | 定义工具接口 | `tools/interfaces.go` | P0 |
| 7.3 | 迁移 ToolService → 注册表 | `tools/registry.go` | P0 |
| 7.4 | 迁移 ToolCallingService → 执行器 | `tools/executor.go` | P0 |
| 7.5 | 实现安全守卫 | `tools/guardrail.go` | P0 |
| 7.6 | 编写单元测试 | `tools/registry_test.go` | P1 |

#### Week 6: MCP 与内置工具

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 8.1 | 迁移 MCP 客户端 | `tools/mcp.go` | P0 |
| 8.2 | 迁移内置工具 | `tools/native/*.go` | P0 |
| 8.3 | 实现参数校验 | `tools/validator.go` | P0 |
| 8.4 | 实现智能工具选择 | `tools/selector.go` | P1 |
| 8.5 | 集成测试 | `tools/integration_test.go` | P0 |

### 验收标准

- [ ] `internal/tools/` 目录创建完成
- [ ] 工具注册表功能完整
- [ ] 工具执行器安全可控
- [ ] MCP 集成测试通过
- [ ] 代码覆盖率 > 70%

---

## Phase 4: LLM 层精简 (Week 6-7)

### 目标
精简 `internal/llm/`，建立网关。

### 任务清单

#### Week 6: 网关接口

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 9.1 | 创建目录结构 | `backend/internal/llm/` | P0 |
| 9.2 | 定义网关接口 | `llm/gateway.go` | P0 |
| 9.3 | 迁移 ModelService → 网关 | `llm/gateway.go` | P0 |
| 9.4 | 实现模型路由 | `llm/router.go` | P0 |
| 9.5 | 保留提供商实现 | `llm/providers/*.go` | P0 |

#### Week 7: 优化与测试

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 10.1 | 实现负载均衡 | `llm/gateway.go` | P1 |
| 10.2 | 实现故障转移 | `llm/gateway.go` | P1 |
| 10.3 | 实现响应缓存 | `llm/gateway.go` | P1 |
| 10.4 | 迁移 Token 管理 | `llm/token.go` | P0 |
| 10.5 | 集成测试 | `llm/integration_test.go` | P0 |

### 验收标准

- [ ] `internal/llm/` 目录创建完成
- [ ] 网关支持多提供商
- [ ] 模型路由功能完整
- [ ] 与现有 ModelService 功能等价
- [ ] 代码覆盖率 > 70%

---

## Phase 5: 编排层精简 (Week 7-8)

### 目标
精简 `internal/orchestration/`，统一规划器。

### 任务清单

#### Week 7: 规划器

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 11.1 | 创建目录结构 | `backend/internal/orchestration/` | P0 |
| 11.2 | 定义编排接口 | `orchestration/interfaces.go` | P0 |
| 11.3 | 合并 Planner | `orchestration/planner.go` | P0 |
| 11.4 | 迁移 TeamOrchestrator | `orchestration/team.go` | P0 |
| 11.5 | 编写单元测试 | `orchestration/planner_test.go` | P1 |

#### Week 8: 工作流与调度

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 12.1 | 迁移 WorkflowService | `orchestration/workflow.go` | P0 |
| 12.2 | 实现调度器 | `orchestration/scheduler.go` | P0 |
| 12.3 | 实现确认流程 | `orchestration/confirmation.go` | P1 |
| 12.4 | 集成测试 | `orchestration/integration_test.go` | P0 |

### 验收标准

- [ ] `internal/orchestration/` 目录创建完成
- [ ] Planner 功能完整
- [ ] Team 协作测试通过
- [ ] Workflow 执行正常
- [ ] 代码覆盖率 > 75%

---

## Phase 6: API 层重构 (Week 8-9)

### 目标
精简 handlers，建立轻薄外壳。

### 任务清单

#### Week 8: 核心处理器

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 13.1 | 创建目录结构 | `backend/internal/api/` | P0 |
| 13.2 | 迁移服务器启动 | `api/server.go` | P0 |
| 13.3 | 迁移路由注册 | `api/router.go` | P0 |
| 13.4 | 迁移 ChatHandler | `api/handlers/chat.go` | P0 |
| 13.5 | 迁移 AgentHandler | `api/handlers/agent.go` | P0 |
| 13.6 | 迁移中间件 | `api/middleware/*.go` | P0 |

#### Week 9: 剩余处理器

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 14.1 | 迁移 SessionHandler | `api/handlers/session.go` | P0 |
| 14.2 | 迁移 TaskHandler | `api/handlers/task.go` | P0 |
| 14.3 | 迁移 WebSocket | `api/websocket.go` | P0 |
| 14.4 | 集成测试 | `api/integration_test.go` | P0 |
| 14.5 | API 文档更新 | `docs/API.md` | P1 |

### 验收标准

- [ ] `internal/api/` 目录创建完成
- [ ] 所有核心端点可用
- [ ] 中间件功能正常
- [ ] WebSocket 通信正常
- [ ] 代码覆盖率 > 60%

---

## Phase 7: 基础设施迁移 (Week 9-10)

### 目标
建立 `internal/infra/`。

### 任务清单

#### Week 9: 数据库与缓存

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 15.1 | 创建目录结构 | `backend/internal/infra/` | P0 |
| 15.2 | 迁移数据库连接 | `infra/database.go` | P0 |
| 15.3 | 迁移缓存 | `infra/cache.go` | P0 |
| 15.4 | 迁移配置管理 | `infra/config.go` | P0 |
| 15.5 | 迁移日志 | `infra/logger.go` | P0 |

#### Week 10: 向量与事件系统

| 序号 | 任务 | 文件 | 优先级 |
|------|------|------|--------|
| 16.1 | 迁移向量存储 | `infra/vector.go` | P0 |
| 16.2 | 迁移事件总线 | `infra/event_bus.go` | P0 |
| 16.3 | **实现事件持久化** | `infra/event_store.go` | P0 |
| 16.4 | **实现事件订阅管理** | `infra/event_sub.go` | P0 |
| 16.5 | **实现事件过滤引擎** | `infra/event_filter.go` | P1 |
| 16.6 | **实现事件压缩器** | `infra/event_compress.go` | P1 |
| 16.7 | 迁移限流器 | `infra/rate_limit.go` | P0 |
| 16.8 | 集成测试 | `infra/integration_test.go` | P0 |
| 16.9 | **事件系统专项测试** | `infra/event_*_test.go` | P0 |

### 验收标准

- [ ] `internal/infra/` 目录创建完成
- [ ] **纯文件存储引擎可用（Markdown/JSON/二进制向量）**
- [ ] 缓存功能正常（内存实现）
- [ ] 向量存储可用（HNSW 纯 Go 实现）
- [ ] **事件持久化可用（Append/Read/Replay/Snapshot）**
- [ ] **事件订阅可用（多消费者/背压/容错）**
- [ ] **事件过滤可用（类型/类别/来源/组合）**
- [ ] **事件压缩可用（None/Gzip/Zstd/Delta/Smart）**
- [ ] 代码覆盖率 > 70%

---

## Phase 8: 集成与清理 (Week 10-12)

### 目标
整合所有模块，清理旧代码。

### 任务清单

#### Week 10: 模块整合

| 序号 | 任务 | 说明 | 优先级 |
|------|------|------|--------|
| 17.1 | 整合所有 internal/ 包 | 确保编译通过 | P0 |
| 17.2 | 更新 main.go | 使用新架构启动 | P0 |
| 17.3 | 更新 go.mod | 整理依赖 | P0 |
| 17.4 | 端到端测试 | 完整流程测试 | P0 |

#### Week 11: 测试与优化

| 序号 | 任务 | 说明 | 优先级 |
|------|------|------|--------|
| 18.1 | 性能测试 | 基准测试 | P1 |
| 18.2 | 压力测试 | 并发测试 | P1 |
| 18.3 | 修复 Bug | 测试中发现的问题 | P0 |
| 18.4 | 优化性能 | 瓶颈分析 | P1 |

#### Week 12: 清理与文档

| 序号 | 任务 | 说明 | 优先级 |
|------|------|------|--------|
| 19.1 | 删除旧 services/ 目录 | 清理冗余代码 | P0 |
| 19.2 | 删除旧 handlers/ 目录 | 清理冗余代码 | P0 |
| 19.3 | 更新 README.md | 项目说明 | P1 |
| 19.4 | 更新 ARCHITECTURE.md | 架构文档 | P1 |
| 19.5 | 更新 API.md | API 文档 | P1 |
| 19.6 | 编写 DEPLOYMENT.md | 部署文档 | P1 |
| 19.7 | **编写 EVENT_SYSTEM.md** | 事件系统使用文档 | P1 |

### 验收标准

- [ ] 项目编译通过
- [ ] 所有端到端测试通过
- [ ] 性能达到基准要求
- [ ] 旧代码完全清理
- [ ] 文档完整更新

---

## 10. 风险与缓解

### 10.1 风险清单

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| **编译失败** | 高 | 高 | 每个 Phase 结束时确保编译通过 |
| **功能回归** | 高 | 高 | 保留旧实现，通过适配器过渡 |
| **性能下降** | 中 | 中 | 每个 Phase 进行性能基准测试 |
| **循环依赖** | 中 | 高 | 严格遵循依赖方向，接口隔离 |
| **测试覆盖不足** | 中 | 中 | 强制要求每个模块的测试覆盖率 |
| **文档滞后** | 高 | 低 | 每个 Phase 同步更新文档 |

### 10.2 回滚策略

```
发现问题
    │
    ▼
┌─────────────────┐
│ 1. 定位问题     │
│    哪个 Phase   │
│    哪个模块     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 评估影响     │
│    是否阻塞     │
│    其他模块     │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│ 是    │ │ 否    │
│ 阻塞  │ │ 可修复 │
└───┬───┘ └───┬───┘
    │         │
    ▼         ▼
┌───────┐ ┌───────┐
│回滚到 │ │ 修复  │
│上一   │ │ 继续  │
│版本   │ │       │
└───────┘ └───────┘
```

---

## 11. 验收标准

### 11.1 功能验收

| 检查项 | 标准 | 验证方式 |
|--------|------|----------|
| 简单对话 | 正常响应 | 手动测试 |
| 工具调用 | 正确执行 | 手动测试 |
| 流式输出 | SSE 正常 | 手动测试 |
| 记忆检索 | 跨会话记忆 | 手动测试 |
| 多 Agent | 协作完成 | 手动测试 |
| 工作流 | 按步骤执行 | 手动测试 |

### 11.2 性能验收

| 指标 | 目标 | 验证方式 |
|------|------|----------|
| 首 token 延迟 | < 500ms | 基准测试 |
| 工具调用延迟 | < 2s | 基准测试 |
| 记忆检索延迟 | < 100ms | 基准测试 |
| 并发请求 | > 100 | 压力测试 |
| 内存占用 | < 512MB | 监控 |

### 11.3 代码质量验收

| 指标 | 目标 | 验证方式 |
|------|------|----------|
| 单元测试覆盖率 | > 70% | go test -cover |
| 编译无警告 | 0 警告 | go build |
| 无循环依赖 | 0 循环 | go list |
| 代码规范 | gofmt | gofmt -l |

---

## 附录

### A. 迁移总览图

```
Week:  1    2    3    4    5    6    7    8    9    10   11   12
       ├────┴────┤    ├────┴────┤    ├────┤    ├────┤    ├────┴────┤
       │ Phase 1 │    │ Phase 2 │    │ P3 │    │ P4 │    │ Phase 5 │
       │ 内核提取 │    │ 记忆重构 │    │工具│    │LLM │    │ 编排精简 │
       │         │    │         │    │重构│    │精简│    │         │
       └─────────┘    └─────────┘    └────┘    └────┘    └─────────┘
                                              ├────┴────┤    ├────┴────┤
                                              │ Phase 6 │    │ Phase 8 │
                                              │ API 重构 │    │ 集成清理 │
                                              │         │    │         │
                                              └─────────┘    └─────────┘
                                                    ├────┴────┤
                                                    │ Phase 7 │
                                                    │ 基础设施 │
                                                    │ 迁移    │
                                                    └─────────┘
```

### B. 关键里程碑

| 里程碑 | 时间 | 标志 |
|--------|------|------|
| 内核可用 | Week 3 | Reactor 通过测试 |
| 记忆可用 | Week 5 | 三级记忆测试通过 |
| 工具可用 | Week 6 | 工具执行测试通过 |
| LLM 可用 | Week 7 | 多提供商测试通过 |
| 编排可用 | Week 8 | 工作流测试通过 |
| API 可用 | Week 9 | 所有端点测试通过 |
| 基础设施可用 | Week 10 | 存储测试通过 |
| 项目完成 | Week 12 | 端到端测试通过 |
| 增强能力迁移完成 | Week 13 | 反思/学习/模式/纠错测试通过 |
| 新增能力实现完成 | Week 14 | 对齐/多模态测试通过 |
| **事件系统完成** | **Week 14** | **持久化/订阅/过滤/压缩测试通过** |
| 能力联动优化完成 | Week 15 | 完整增强链路测试通过 |

### C. 资源需求

| 资源 | 需求 | 说明 |
|------|------|------|
| 开发人员 | 1-2 人 | Go 后端开发 |
| 测试环境 | 1 台 | 部署测试 |
| LLM API | 多个 key | 测试不同提供商 |
| 时间 | 15 周 | 全职开发（含增强能力和事件系统） |

---

## 12. 增强能力迁移计划

### 12.1 Phase 9: 已有增强能力迁移（Week 12-13）

**目标**: 将现有 services/ 下的增强能力迁移到 kernel/

#### Week 12: 自我反思与思维链

| 序号 | 任务 | 来源文件 | 目标文件 | 优先级 |
|------|------|----------|----------|--------|
| 20.1 | 迁移自我反思服务 | `self_reflection_service.go` | `kernel/reflection.go` | P0 |
| 20.2 | 迁移思维链推理 | `cot_reasoning_service.go` | 整合到 `kernel/reactor.go` | P0 |
| 20.3 | 编写反思单元测试 | `kernel/reflection_test.go` | 新增 | P1 |
| 20.4 | 集成测试 | `kernel/reflection_integration_test.go` | 新增 | P0 |

#### Week 13: 学习、模式与纠错

| 序号 | 任务 | 来源文件 | 目标文件 | 优先级 |
|------|------|----------|----------|--------|
| 21.1 | 迁移学习服务 | `learning_service.go` | `kernel/learning.go` | P0 |
| 21.2 | 迁移模式检测 | `pattern_detector.go` | `kernel/pattern.go` | P0 |
| 21.3 | 迁移自动纠错 | `correction_service.go` | `kernel/correction.go` | P0 |
| 21.4 | 编写单元测试 | `kernel/*_test.go` | 新增 | P1 |
| 21.5 | 集成测试 | `kernel/enhancement_integration_test.go` | 新增 | P0 |

### 12.2 Phase 10: 新增增强能力（Week 13-14）

**目标**: 实现价值对齐和多模态支持

#### Week 13: 价值对齐（可配置权限模型）

| 序号 | 任务 | 文件 | 说明 | 优先级 |
|------|------|------|------|--------|
| 22.1 | 实现价值对齐接口 | `kernel/alignment.go` | 新增，支持权限分级 | P0 |
| 22.2 | 实现角色系统 | `kernel/alignment.go` | Guest/User/Premium/Admin/System | P0 |
| 22.3 | 实现检查级别 | `kernel/alignment.go` | None/Minimal/Standard/Strict | P0 |
| 22.4 | 实现准则管理 | `kernel/alignment.go` | 系统级/用户级/Agent级 | P0 |
| 22.5 | 实现规则动作 | `kernel/alignment.go` | Block/Warn/Rewrite/Skip/Notify | P0 |
| 22.6 | 实现绕过机制 | `kernel/alignment.go` | BypassToken + 理由记录 | P0 |
| 22.7 | 实现输入检查 | `kernel/alignment.go` | 输入安全检查 | P0 |
| 22.8 | 实现输出检查 | `kernel/alignment.go` | 输出安全检查 | P0 |
| 22.9 | 编写单元测试 | `kernel/alignment_test.go` | 权限/绕过/级别测试 | P1 |

#### Week 14: 多模态（可选）

| 序号 | 任务 | 文件 | 说明 | 优先级 |
|------|------|------|------|--------|
| 23.1 | 实现多模态接口 | `kernel/multimodal.go` | 新增 | P1 |
| 23.2 | 实现图像处理 | `kernel/multimodal.go` | OCR + 检测 | P1 |
| 23.3 | 实现音频处理 | `kernel/multimodal.go` | 语音转文字 | P2 |
| 23.4 | 实现文档处理 | `kernel/multimodal.go` | PDF/Word 解析 | P2 |
| 23.5 | 编写单元测试 | `kernel/multimodal_test.go` | 新增 | P1 |

### 12.3 Phase 11: 能力联动优化（Week 14-15）

**目标**: 实现增强能力间的联动，集成事件系统

#### Week 14: 反思与学习联动 + 事件系统集成

| 序号 | 任务 | 说明 | 优先级 |
|------|------|------|--------|
| 24.1 | 反思触发学习 | 反思结果自动触发学习 | P0 |
| 24.2 | 纠错与反思结合 | 纠错前先做反思评估 | P0 |
| 24.3 | 对齐与学习结合 | 从对齐违规中学习用户偏好 | P1 |
| 24.4 | **内核事件接入事件系统** | ReAct 事件写入 EventStore | P0 |
| 24.5 | **增强能力事件接入** | Reflection/Learning 事件写入 | P0 |
| 24.6 | **API 层事件消费** | SSE/WebSocket 消费事件流 | P0 |

#### Week 15: 模式优化 + 事件系统完善

| 序号 | 任务 | 说明 | 优先级 |
|------|------|------|--------|
| 25.1 | 模式自动优化工作流 | 检测到的模式自动建议工作流 | P1 |
| 25.2 | 模式触发学习 | 从模式中提取用户偏好 | P1 |
| 25.3 | **事件回放功能** | 支持会话事件回放 | P1 |
| 25.4 | **事件压缩优化** | 根据事件类型自动选择压缩策略 | P1 |
| 25.5 | 端到端测试 | 完整增强能力链路测试 | P0 |

### 12.4 增强能力迁移总览

```
Week:  12   13   14   15
       ├────┴────┤    ├────┴────┤
       │ Phase 9 │    │ Phase 10│
       │已有能力 │    │新增能力 │
       │ 迁移   │    │ 实现   │
       └─────────┘    └─────────┘
                          ├────┴────┤
                          │ Phase 11│
                          │联动优化 │
                          └─────────┘
```

### 12.5 增强能力验收标准

| 检查项 | 标准 | 验证方式 |
|--------|------|----------|
| 自我反思 | 能识别输出问题 | 单元测试 |
| 学习进化 | 能从反馈中调整 | 集成测试 |
| 模式检测 | 能发现重复模式 | 单元测试 |
| 自动纠错 | 能修正明显错误 | 单元测试 |
| 价值对齐 | 能拦截违规内容 | 单元测试 |
| 价值对齐权限 | 不同角色不同检查级别 | 集成测试 |
| 价值对齐绕过 | Premium+ 可绕过非安全规则 | 集成测试 |
| 多模态 | 能处理图像输入 | 手动测试 |
| 能力联动 | 反思触发学习 | 集成测试 |
| **事件持久化** | **能存储和回放事件** | **集成测试** |
| **事件订阅** | **多消费者独立消费** | **单元测试** |
| **事件过滤** | **按需接收事件** | **单元测试** |
| **事件压缩** | **减少传输和存储** | **基准测试** |
| **融合上下文压缩** | **四层混合压缩正常工作** | **基准测试** |
| **小说结构层** | **DialogueBook 能构建和查询** | **单元测试** |
| **内存分离层** | **Main Context + External Memory 分离** | **单元测试** |
| **RAG 检索层** | **向量检索 + 重排序 + 父文档** | **集成测试** |
| **动态预算层** | **Token 预算按策略分配** | **单元测试** |

### 12.6 增强能力风险

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|----------|
| 反思增加延迟 | 高 | 中 | 异步执行 + 可配置 |
| 学习数据不足 | 中 | 低 | 渐进式 + 默认值 |
| 多模态模型依赖 | 中 | 高 | 可选模块 + 降级 |
| 价值对齐过度限制 | 中 | 中 | 可配置 + 权限分级 + 可绕过 |
| **事件存储膨胀** | **中** | **中** | **快照 + 压缩 + 自动清理** |
| **事件回放性能** | **中** | **中** | **索引 + 快照恢复** |
| **融合架构复杂度** | **中** | **高** | **分层实现，逐层验证** |
| **向量检索延迟** | **中** | **中** | **缓存 + 预加载** |
| **动态预算失衡** | **低** | **中** | **监控 + 自适应调整** |

---

### 12.7 新功能迁移

#### Phase 12: 错误处理与测试 (Week 16)

```
Week 16
├─ Day 1-2: 错误码定义
│   ├─ 定义 ErrorCode 枚举
│   ├─ 实现 AppError 结构
│   └─ 实现错误包装函数
│
├─ Day 3-4: 错误恢复
│   ├─ 实现自动重试
│   ├─ 实现降级策略
│   └─ 实现错误报告
│
└─ Day 5: 测试框架
    ├─ 单元测试基类
    ├─ Mock 工具
    └─ 集成测试环境
```

**验收标准**:
- 所有错误都有错误码
- 关键错误能自动恢复
- 单元测试覆盖率 > 80%

#### Phase 13: 性能优化 (Week 17)

```
Week 17
├─ Day 1-2: 基准测试
│   ├─ 编写 Benchmark
│   ├─ 性能监控
│   └─ 性能报告
│
├─ Day 3-4: 优化实现
│   ├─ 连接池优化
│   ├─ 缓存优化
│   └─ 压缩优化
│
└─ Day 5: 验证
    ├─ 性能测试
    ├─ 对比基准
    └─ 文档更新
```

**验收标准**:
- 首 Token < 500ms
- 工具调用 < 2s
- 内存 < 512MB

#### Phase 14: 插件系统 (Week 18)

```
Week 18
├─ Day 1-2: 插件接口
│   ├─ 定义 Plugin 接口
│   ├─ 定义 ToolPlugin 接口
│   └─ 实现插件加载器
│
├─ Day 3-4: 插件管理
│   ├─ 插件安装
│   ├─ 插件启用/禁用
│   └─ 插件配置
│
└─ Day 5: 示例插件
    ├─ 编写示例
    ├─ 测试插件
    └─ 文档更新
```

**验收标准**:
- 能加载 .so 插件
- 能注册工具
- 有示例插件

#### Phase 15: 版本兼容与更新 (Week 19)

```
Week 19
├─ Day 1-2: 版本管理
│   ├─ API 版本路由
│   ├─ 配置版本迁移
│   └─ 数据版本迁移
│
├─ Day 3-4: CLI 补全
│   ├─ Bash 补全
│   ├─ Zsh 补全
│   └─ Fish 补全
│
└─ Day 5: 更新检查
    ├─ 版本检查
    ├─ 更新提示
    └─ 自动更新
```

**验收标准**:
- 支持 API 版本
- 配置自动迁移
- Shell 补全可用
- 更新检查正常

---

> 本文档为实施计划草案，欢迎评审和补充。
