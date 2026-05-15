# OpenAIDE 架构设计文档

> 版本: v3.0.0-draft  
> 日期: 2026-05-15  
> 状态: 设计评审中

---

## 目录

1. [设计哲学](#1-设计哲学)
2. [总体架构](#2-总体架构)
3. [分层详解](#3-分层详解)
   - 3.1 [内核层 (Agent Kernel)](#31-内核层-agent-kernel)
     - 3.1.1 [核心组件](#311-核心组件)
     - 3.1.2 [Agent 定义](#312-agent-定义)
     - 3.1.3 [ReAct 引擎](#313-react-引擎)
     - 3.1.4 [记忆系统](#314-记忆系统)
     - 3.1.5 [工具系统](#315-工具系统)
     - 3.1.6 [增强能力层](#316-增强能力层)
   - 3.2 [编排层 (Orchestration)](#32-编排层-orchestration)
   - 3.3 [API 层 (API Layer)](#33-api-层-api-layer)
     - 3.3.1 [职责边界](#331-职责边界)
     - 3.3.2 [核心端点](#332-核心端点)
     - 3.3.3 [增强能力相关端点](#333-增强能力相关端点)
     - 3.3.4 [对话处理流程](#334-对话处理流程)
   - 3.4 [基础设施层 (Infrastructure)](#34-基础设施层-infrastructure)
4. [模块职责矩阵](#4-模块职责矩阵)
5. [数据流设计](#5-数据流设计)
6. [接口契约](#6-接口契约)
7. [事件系统设计](#7-事件系统设计)
8. [项目目录结构](#8-项目目录结构)
9. [现有服务映射](#9-现有服务映射)
10. [迁移路线图](#10-迁移路线图)
11. [附录](#11-附录)

---

## 1. 设计哲学

### 1.1 核心原则

| 原则 | 含义 | 反模式（现有问题） |
|------|------|-------------------|
| **内核唯一** | 所有 AI 智能收敛到 Agent Kernel | 80+ 服务平铺，智能分散在各处 |
| **外壳轻薄** | API 层只负责连接用户和内核 | handlers 里包含业务逻辑 |
| **编排可选** | 简单任务直接走内核，复杂任务才启用编排 | 所有请求都经过复杂编排 |
| **接口先行** | 先定义接口，再实现 | 直接写实现，后期补接口 |
| **依赖倒置** | 内核不依赖外壳，外壳依赖内核 | app.go 直接引用所有服务 |

### 1.2 架构愿景

```
用户输入 → [API 外壳] → [编排层(可选)] → [Agent 内核] → LLM/工具
                ↑                ↑              ↑
                │                │              │
           会话管理          任务规划        思考/记忆/工具
           权限校验          多 Agent 调度    模型调用
           流式传输          工作流执行
```

### 1.3 与现有架构对比

```
现有架构 (问题)                          目标架构 (解决)
┌─────────────────────────┐            ┌─────────────────────────┐
│  handlers/ (40+ 文件)    │            │  api/handlers/ (精简)    │
│  ├─ chat_handler.go     │            │  ├─ chat.go             │
│  ├─ agent_handler.go    │            │  ├─ agent.go            │
│  ├─ tool_handler.go     │            │  ├─ session.go          │
│  ├─ ... (36 more)       │            │  └─ ... (少量)          │
│  (包含业务逻辑)           │            │  (只负责输入输出转换)      │
└──────────┬──────────────┘            └──────────┬──────────────┘
           │                                       │
┌──────────▼──────────────┐            ┌──────────▼──────────────┐
│  services/ (80+ 文件)    │            │  kernel/ (核心)          │
│  ├─ agent_executor.go   │            │  ├─ reactor.go          │
│  ├─ agent_router.go     │            │  ├─ memory.go           │
│  ├─ tool_calling_svc.go │            │  ├─ tool_registry.go    │
│  ├─ dialogue_svc.go     │            │  └─ state_machine.go    │
│  ├─ memory_svc.go       │            │                         │
│  ├─ workflow_svc.go     │            │  orchestration/ (可选)   │
│  ├─ ... (70+ more)      │            │  ├─ planner.go          │
│  (职责混乱，循环依赖)      │            │  ├─ team.go             │
│                          │            │  └─ workflow.go         │
└─────────────────────────┘            └─────────────────────────┘
```

---

## 2. 总体架构

### 2.1 分层架构图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              客户端层 (Clients)                               │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐    │
│  │   Web UI     │  │  Terminal    │  │  REST API    │  │  Go SDK      │    │
│  │  (frontend/) │  │  (terminal/) │  │  (external)  │  │  (sdk/go/)   │    │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘    │
└─────────┼─────────────────┼─────────────────┼─────────────────┼────────────┘
          │                 │                 │                 │
          └─────────────────┴─────────────────┴─────────────────┘
                                    │
┌───────────────────────────────────▼─────────────────────────────────────────┐
│                           API 层 (API Layer)                                 │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  HTTP Server (Gin) │ WebSocket │ SSE │ Auth │ Rate Limit │ Session  │    │
│  │  职责: 协议转换、会话管理、权限校验、流式传输                              │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└───────────────────────────────────┬─────────────────────────────────────────┘
                                    │
                    ┌───────────────┴───────────────┐
                    │                               │
                    ▼                               ▼
┌─────────────────────────────────┐   ┌─────────────────────────────────────┐
│      编排层 (Orchestration)      │   │         内核层 (Agent Kernel)        │
│  ┌───────────────────────────┐  │   │  ┌───────────────────────────────┐  │
│  │  Planner   - 任务规划      │  │   │  │  Reactor   - ReAct 引擎       │  │
│  │  Team      - 多 Agent 协作 │  │   │  │  Memory    - 记忆系统          │  │
│  │  Workflow  - 工作流引擎    │  │   │  │  ToolReg   - 工具注册表        │  │
│  └───────────────────────────┘  │   │  │  State     - 状态机            │  │
│                                 │   │  └───────────────────────────────┘  │
│  触发条件: 复杂任务、多步骤、协作  │   │                                     │
│  简单任务绕过编排，直达内核        │   │  触发条件: 所有 AI 请求必经         │
└─────────────────────────────────┘   └─────────────────────────────────────┘
                    │                               │
                    └───────────────┬───────────────┘
                                    │
┌───────────────────────────────────▼─────────────────────────────────────────┐
│                         LLM 网关 (LLM Gateway)                               │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │  OpenAI  │ │ Anthropic│ │ DeepSeek │ │  Gemini  │ │  Ollama  │  ...    │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
│  职责: 统一接口、模型路由、负载均衡、故障转移、用量统计                          │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
┌───────────────────────────────────▼─────────────────────────────────────────┐
│                        基础设施层 (Infrastructure)                            │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │
│  │  SQLite  │ │  Redis   │ │  Vector  │ │  Event   │ │  Logger  │          │
│  │  (GORM)  │ │  (Cache) │ │  (HNSW)  │ │  (Bus)   │ │  (slog)  │          │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘          │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 关键设计决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 内核位置 | `internal/kernel/` | 不对外暴露，防止外部直接依赖 |
| 编排层位置 | `internal/orchestration/` | 可选增强，内核不依赖编排 |
| API 层位置 | `internal/api/` | 可替换为 gRPC/GraphQL |
| LLM 层位置 | `internal/llm/` | 保留现有设计，精简接口 |
| 工具层位置 | `internal/tools/` | 统一 MCP + Native 工具 |
| 记忆层位置 | `internal/memory/` | 多级记忆统一抽象 |

---

## 3. 分层详解

### 3.1 内核层 (Agent Kernel)

> **定位**: 项目的唯一智能核心。所有 AI 能力收敛于此。  
> **原则**: 内核不依赖任何上层模块，上层通过接口调用内核。

#### 3.1.1 核心组件

```
kernel/
├── agent.go           # Agent 定义：角色、配置、能力
├── reactor.go         # ReAct 引擎：思考 → 行动 → 观察 循环
├── memory.go          # 记忆系统接口（抽象）
├── tool_registry.go   # 工具注册表接口（抽象）
├── state_machine.go   # Agent 状态机
├── events.go          # 内核事件定义
├── reflection.go      # 自我反思：执行后评估与改进
├── learning.go        # 学习进化：从反馈中优化策略
├── pattern.go         # 模式检测：发现用户行为模式
├── correction.go      # 自动纠错：输出质量评估与修正
├── alignment.go       # 价值对齐：输出安全与价值观检查
└── multimodal.go      # 多模态：图像/音频/文档处理（可选）
```

#### 3.1.2 Agent 定义

一个 Agent 是**可配置的智能实体**：

```go
type Agent struct {
    ID       string      // 唯一标识
    Name     string      // 名称
    Role     string      // 角色描述（system prompt）
    
    // 能力配置
    ModelID     string      // 默认模型
    Tools       []string    // 允许使用的工具（空=全部）
    MaxRounds   int         // 最大 ReAct 轮次
    Temperature float64     // 创造性参数
    
    // 记忆配置
    MemoryEnabled bool      // 是否启用记忆
    ContextWindow int       // 上下文窗口大小
    
    // 状态
    State AgentState        // 当前状态
}
```

#### 3.1.3 ReAct 引擎

ReAct 循环是内核的核心执行逻辑：

```
用户输入
    │
    ▼
┌─────────────┐
│   THINK     │ ◄── 分析意图、制定计划、检索记忆
│  (思考)     │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    ACT      │ ◄── 调用工具 / 生成代码 / 调用 API
│   (行动)    │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│  OBSERVE    │ ◄── 获取工具结果 / 执行反馈
│  (观察)     │
└──────┬──────┘
       │
       ▼
   完成？ ──YES──► 生成最终回复
       │
      NO
       │
       └──────────► 回到 THINK（下一轮）
```

**状态机**:

```
          ┌─────────┐
          │  IDLE   │ ◄── 空闲状态
          └────┬────┘
               │ 收到请求
               ▼
          ┌─────────┐
          │THINKING │ ◄── 分析意图、检索记忆
          └────┬────┘
               │
               ▼
          ┌─────────┐
          │ ACTING  │ ◄── 调用工具/生成内容
          └────┬────┘
               │
               ▼
          ┌─────────┐
          │OBSERVING│ ◄── 等待结果
          └────┬────┘
               │
      ┌────────┴────────┐
      │                 │
      ▼                 ▼
 ┌─────────┐      ┌─────────┐
 │COMPLETE │      │  FAILED │
 │(完成)   │      │ (失败)  │
 └─────────┘      └─────────┘
```

#### 3.1.4 记忆系统

三级记忆架构：

| 层级 | 名称 | 存储 | 作用 | 实现 |
|------|------|------|------|------|
| L1 | 工作记忆 | 内存 | 当前对话上下文 | `Dialogue.Messages` |
| L2 | 短期记忆 | SQLite | 近期对话摘要 | `ContextEngine.Compress()` |
| L3 | 长期记忆 | SQLite + Vector | 用户偏好、事实、流程 | `MemoryService` + `RAGService` |

记忆检索流程：

```
用户输入
    │
    ▼
┌─────────────────┐
│ 1. 提取关键词    │
│ 2. 向量检索 L3   │ ──► 相关记忆（相似度 > 0.7）
│ 3. 查询 L2 摘要  │ ──► 近期对话摘要
│ 4. 组装 L1 上下文│ ──► 当前对话消息
└─────────────────┘
    │
    ▼
注入到 System Prompt
    │
    ▼
发送给 LLM
```

#### 3.1.5 工具系统

工具统一注册表：

```
tools/
├── registry.go        # 工具注册表（核心）
├── executor.go        # 工具执行器（沙箱）
├── mcp.go             # MCP 客户端适配器
└── native/            # 内置工具
    ├── bash.go        # 执行 shell 命令
    ├── file.go        # 文件读写
    ├── code.go        # 代码执行
    ├── search.go      # 网络搜索
    └── http.go        # HTTP 请求
```

工具调用流程：

```
LLM 返回 tool_calls
        │
        ▼
┌───────────────┐
│  权限检查      │ ◄── Agent.Tools 白名单
│  参数校验      │ ◄── JSON Schema 校验
└───────┬───────┘
        │
        ▼
┌───────────────┐
│  工具路由      │
│  MCP 工具 ──► MCP 客户端 ──► 外部服务
│  Native 工具 ──► 本地执行器 ──► 沙箱执行
└───────┬───────┘
        │
        ▼
┌───────────────┐
│  结果格式化    │ ◄── 转换为 LLM 可理解的文本
│  错误处理      │ ◄── 失败时返回错误信息
└───────┬───────┘
        │
        ▼
  追加到对话上下文
        │
        ▼
  进入下一轮 ReAct
```

#### 3.1.6 增强能力层（内核扩展）

内核不仅包含核心 ReAct 循环，还具备以下增强能力：

**自我反思 (Reflection)**
- 每轮 ReAct 后评估输出质量
- 发现事实错误、逻辑漏洞、表达不清
- 生成改进建议并影响下一轮思考
- 触发条件：可配置（每轮/任务完成/出错时/评分低时）

**学习进化 (Learning)**
- 从用户反馈（评分、评论）中学习偏好
- 从执行结果中总结经验
- 调整响应风格、工具选择、模型偏好
- 渐进式学习，低置信度时不应用

**模式检测 (Pattern)**
- 检测用户重复的工作流模式
- 发现用户行为偏好（时间、主题、工具）
- 基于模式建议优化或自动化

**自动纠错 (Correction)**
- 评估输出质量（事实、逻辑、语言、安全）
- 自动修正错误（高置信度时）
- 记录变更历史

**价值对齐 (Alignment)**
- 检查输出是否符合预设价值观
- 支持自定义准则和规则
- 分层对齐：系统级 + 用户级
- 透明报告对齐调整

**多模态 (Multimodal，可选)**
- 处理图像输入（OCR、对象检测、场景理解）
- 处理音频输入（语音转文字）
- 处理文档输入（PDF、Word 解析）
- 生成图像/音频输出

```
增强能力触发流程：

用户输入
    │
    ▼
┌─────────────────┐
│ 1. 多模态感知   │ ◄── 处理图像/音频/文档
│    (可选)       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 价值对齐检查 │ ◄── 输入安全检查
│    (Alignment)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. ReAct 核心   │ ◄── 思考 → 行动 → 观察
│    (Reactor)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 自我反思     │ ◄── 评估输出质量
│    (Reflection) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 自动纠错     │ ◄── 修正错误（高置信度）
│    (Correction) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 6. 价值对齐检查 │ ◄── 输出安全检查
│    (Alignment)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 7. 学习进化     │ ◄── 记录反馈，优化策略
│    (Learning)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 8. 模式检测     │ ◄── 检测行为模式
│    (Pattern)    │
└─────────────────┘
```

### 3.2 编排层 (Orchestration)

> **定位**: 可选增强层。简单任务绕过，复杂任务启用。  
> **原则**: 编排层依赖内核，内核不依赖编排层。

#### 3.2.1 触发条件

| 场景 | 处理方式 | 示例 |
|------|----------|------|
| 简单对话 | 绕过编排，直达内核 | "你好"、"解释概念" |
| 单工具调用 | 绕过编排，直达内核 | "查天气"、"执行命令" |
| 多步骤任务 | 启用 Planner | "帮我搭建一个博客" |
| 多 Agent 协作 | 启用 Team | "前端和后端分别实现" |
| 固定流程 | 启用 Workflow | "每日数据报表生成" |

#### 3.2.2 Planner（任务规划器）

将复杂目标拆解为可执行步骤：

```
用户: "帮我搭建一个 Go + React 的博客系统"

Planner 拆解:
┌─────────────────────────────────────────┐
│ Step 1: 设计数据库 schema               │
│   └─ Agent: backend-dev (coding 模型)   │
│                                         │
│ Step 2: 实现后端 API                    │
│   └─ Agent: backend-dev                 │
│   └─ 依赖: Step 1                       │
│                                         │
│ Step 3: 设计前端页面                    │
│   └─ Agent: frontend-dev (coding 模型)  │
│   └─ 依赖: Step 1                       │
│                                         │
│ Step 4: 实现前端组件                    │
│   └─ Agent: frontend-dev                │
│   └─ 依赖: Step 3                       │
│                                         │
│ Step 5: 联调测试                        │
│   └─ Agent: qa-engineer                 │
│   └─ 依赖: Step 2, Step 4               │
└─────────────────────────────────────────┘
```

#### 3.2.3 Team（多 Agent 协作）

```
┌─────────────────────────────────────────┐
│           Team Orchestrator             │
│                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ Agent A │  │ Agent B │  │ Agent C │ │
│  │ (前端)   │  │ (后端)   │  │ (测试)   │ │
│  └────┬────┘  └────┬────┘  └────┬────┘ │
│       │            │            │      │
│       └────────────┼────────────┘      │
│                    │                   │
│              ┌─────┴─────┐             │
│              │ Coordinator│            │
│              │ (协调者)   │            │
│              └───────────┘             │
└─────────────────────────────────────────┘
```

#### 3.2.4 Workflow（工作流引擎）

预定义的可重复执行流程：

```yaml
# 示例：代码审查工作流
workflow:
  name: code_review
  trigger: pr_created
  steps:
    - name: analyze_diff
      agent: code-analyzer
      action: analyze_git_diff
      
    - name: check_security
      agent: security-expert
      action: scan_vulnerabilities
      condition: "${steps.analyze_diff.has_code_changes}"
      
    - name: review_performance
      agent: performance-expert
      action: check_performance
      
    - name: generate_report
      agent: report-writer
      action: summarize_reviews
      depends_on: [analyze_diff, check_security, review_performance]
```

### 3.3 API 层 (API Layer)

> **定位**: 轻薄的外壳。只负责协议转换和会话管理。  
> **原则**: 不包含任何业务逻辑，所有智能委托给内核。

#### 3.3.1 职责边界

| API 层做 | API 层不做 |
|----------|-----------|
| HTTP 请求解析 | 不决定用什么模型 |
| 参数校验 | 不拆解任务 |
| 认证授权 | 不选择工具 |
| 会话管理（创建/销毁/关联） | 不管理记忆（只传 SessionID） |
| 流式传输（SSE/WebSocket） | 不执行业务逻辑 |
| 错误格式化 | |

#### 3.3.2 核心端点

```
POST   /api/v3/chat              # 对话（流式/非流式）
POST   /api/v3/chat/stream       # 流式对话（SSE）
GET    /api/v3/chat/:id          # 获取对话详情
DELETE /api/v3/chat/:id          # 删除对话

POST   /api/v3/agents            # 创建 Agent
GET    /api/v3/agents            # 列出 Agent
GET    /api/v3/agents/:id        # 获取 Agent
PUT    /api/v3/agents/:id        # 更新 Agent
DELETE /api/v3/agents/:id        # 删除 Agent

POST   /api/v3/agents/:id/run    # 执行 Agent（单次任务）

POST   /api/v3/tasks             # 创建复杂任务（启用编排）
GET    /api/v3/tasks/:id         # 获取任务状态
DELETE /api/v3/tasks/:id         # 取消任务

POST   /api/v3/sessions          # 创建会话
GET    /api/v3/sessions/:id      # 获取会话
DELETE /api/v3/sessions/:id      # 删除会话

WS     /api/v3/ws                # WebSocket 实时通信

GET    /health                   # 健康检查
GET    /metrics                  # Prometheus 指标
```

#### 3.3.3 增强能力相关端点

```
POST   /api/v3/feedback           # 提交反馈（触发学习）
GET    /api/v3/feedback/:id      # 获取反馈详情

GET    /api/v3/patterns          # 获取检测到的模式
POST   /api/v3/patterns/:id/apply # 应用模式优化

GET    /api/v3/reflections/:execution_id  # 获取执行反思

GET    /api/v3/alignment/guidelines       # 获取价值准则
POST   /api/v3/alignment/guidelines       # 添加价值准则
PUT    /api/v3/alignment/guidelines/:id   # 更新价值准则

POST   /api/v3/multimodal/upload          # 上传多模态文件
GET    /api/v3/multimodal/:id/result      # 获取多模态处理结果
```

#### 3.3.4 对话处理流程

```
用户 POST /api/v3/chat
        │
        ▼
┌─────────────────┐
│ 1. 解析请求      │
│    {message,    │
│     session_id, │
│     agent_id,   │
│     stream}     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 认证授权      │
│    校验 JWT      │
│    检查权限      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. 获取/创建会话 │
│    SessionMgr   │
│    GetOrCreate  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 获取 Agent   │
│    AgentMgr     │
│    Get(agentID) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 调用内核      │
│    agent.Execute │
│    (所有智能)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 6. 流式/批量返回 │
│    stream? SSE   │
│    : JSON       │
└─────────────────┘
```

### 3.4 基础设施层 (Infrastructure)

#### 3.4.1 存储策略

| 数据类型 | 存储 | 理由 |
|----------|------|------|
| 对话消息 | SQLite (GORM) | 结构化、事务、关系查询 |
| 用户配置 | SQLite | 小数据量、结构化 |
| 缓存 | 内存/Redis | 高性能、TTL |
| 向量 | HNSW (内存) | 近似最近邻搜索 |
| 日志 | 文件 + slog | 持久化、可轮转 |
| 事件 | 内存 EventBus | 轻量、异步 |

#### 3.4.2 LLM 网关

```
┌─────────────────────────────────────────┐
│           LLM Gateway                   │
│                                         │
│  输入: ChatRequest {model, messages...} │
│                                         │
│  1. 模型路由 ──► 根据 modelID 选择客户端 │
│  2. 能力匹配 ──► 根据任务选择最佳模型    │
│  3. 负载均衡 ──► 多 key 轮询            │
│  4. 故障转移 ──► 主模型失败时降级        │
│  5. 用量统计 ──► 记录 token 消耗        │
│  6. 响应缓存 ──► 相同请求直接返回        │
│                                         │
│  输出: ChatResponse                     │
└─────────────────────────────────────────┘
```

---

## 4. 模块职责矩阵

### 4.1 模块 ↔ 职责对照表

| 模块 | 核心职责 | 不做什么 | 依赖 |
|------|----------|----------|------|
| **kernel.Agent** | 执行用户请求、管理状态 | 不处理 HTTP、不管理会话 | llm.Gateway, memory.System, tools.Registry |
| **kernel.Reactor** | ReAct 循环实现 | 不直接调用 LLM（通过 Gateway） | Agent, Memory, Tools |
| **kernel.Memory** | 记忆存储与检索接口 | 不实现具体存储（由 impl 提供） | - |
| **kernel.ToolReg** | 工具注册与发现 | 不执行工具（由 executor 执行） | - |
| **orchestration.Planner** | 任务拆解 | 不执行步骤（委托给 Agent） | kernel.Agent |
| **orchestration.Team** | 多 Agent 调度 | 不替代 Agent 执行 | kernel.Agent, Planner |
| **orchestration.Workflow** | 预定义流程执行 | 不处理动态任务 | kernel.Agent |
| **api.Server** | HTTP 服务、路由 | 不执行业务逻辑 | kernel, orchestration |
| **api.SessionMgr** | 会话生命周期 | 不存储消息内容（只存元数据） | infra.DB |
| **llm.Gateway** | 模型路由与调用 | 不管理对话上下文 | infra.Cache |
| **llm.Providers** | 各厂商 API 适配 | 不决定调用哪个模型 | - |
| **memory.Impl** | 记忆的具体实现 | 不决定何时记忆（由 Reactor 决定） | infra.DB, Vector |
| **tools.Executor** | 工具执行沙箱 | 不注册工具（由 Registry 管理） | - |
| **infra.DB** | 数据库连接 | 不定义模型（由 models/ 定义） | - |
| **infra.Cache** | 缓存操作 | 不决定缓存策略 | - |

### 4.2 依赖关系图

```
                    ┌─────────────┐
                    │   Clients   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  API Layer  │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌────────────┐  ┌────────────┐  ┌────────────┐
    │  Session   │  │   Agent    │  │  Planner   │
    │   Manager  │  │   Manager  │  │   (可选)   │
    └──────┬─────┘  └──────┬─────┘  └──────┬─────┘
           │               │               │
           └───────────────┼───────────────┘
                           │
                    ┌──────▼──────┐
                    │Agent Kernel │
                    │  ┌───────┐  │
                    │  │Reactor│  │
                    │  └───┬───┘  │
                    │      │      │
                    │  ┌───┴───┐  │
                    │  │Memory │  │
                    │  │ToolReg│  │
                    │  └───┬───┘  │
                    └──────┼──────┘
                           │
                    ┌──────▼──────┐
                    │ LLM Gateway │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │  Providers  │
                    └─────────────┘
                           │
                    ┌──────▼──────┐
                    │Infrastructure│
                    └─────────────┘
```

**关键约束**: 箭头方向 = 依赖方向。内核不依赖 API 层，API 层依赖内核。

---

## 5. 数据流设计

### 5.1 简单对话数据流

```
用户输入: "你好"

┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  Client │───►│   API   │───►│  Agent  │───►│   LLM   │───►│  Client │
│         │    │  Layer  │    │  Kernel │    │ Gateway │    │         │
└─────────┘    └─────────┘    └─────────┘    └─────────┘    └─────────┘
     │              │              │              │              │
     │ POST /chat   │              │              │              │
     │─────────────►│              │              │              │
     │              │ Execute()    │              │              │
     │              │─────────────►│              │              │
     │              │              │ Chat()       │              │
     │              │              │─────────────►│              │
     │              │              │              │ HTTP API     │
     │              │              │              │─────────────►
     │              │              │              │◄─────────────
     │              │              │              │  Response    │
     │              │              │◄─────────────│              │
     │              │              │  Message     │              │
     │              │◄─────────────│              │              │
     │              │  Event       │              │              │
     │◄─────────────│              │              │              │
     │  JSON/SSE    │              │              │              │
     │              │              │              │              │
```

### 5.2 工具调用数据流

```
用户输入: "查一下北京天气"

┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  Client │───►│   API   │───►│  Agent  │───►│   LLM   │───►│  Client │
└─────────┘    └─────────┘    └────┬────┘    └─────────┘    └─────────┘
                                   │
                                   │ 1. LLM 返回 tool_call
                                   │    {name: "weather", args: {city: "北京"}}
                                   │
                                   ▼
                            ┌─────────────┐
                            │  ToolReg    │
                            │  Lookup     │
                            └──────┬──────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │  Executor   │
                            │  HTTP Call  │
                            └──────┬──────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │  Weather    │
                            │   API       │
                            └──────┬──────┘
                                   │
                                   │ 2. 返回结果
                                   │    {"temp": 25, "condition": "晴"}
                                   │
                                   ▼
                            ┌─────────────┐
                            │  Reactor    │
                            │  (下一轮)   │
                            └──────┬──────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │     LLM     │
                            │  (总结结果)  │
                            └──────┬──────┘
                                   │
                                   ▼
                            ┌─────────────┐
                            │   Client    │
                            │  "北京今天晴天，25度"
                            └─────────────┘
```

### 5.3 复杂任务编排数据流

```
用户输入: "帮我搭建一个博客系统"

┌─────────┐    ┌─────────┐    ┌─────────┐    ┌─────────┐
│  Client │───►│   API   │───►│ Planner │───►│  Team   │
└─────────┘    └─────────┘    └────┬────┘    └────┬────┘
                                   │              │
                                   │ 拆解为步骤    │ 分配 Agent
                                   ▼              ▼
                            ┌─────────────────────────────┐
                            │         Steps               │
                            │  1. 设计数据库               │
                            │  2. 实现后端 API             │
                            │  3. 实现前端页面             │
                            │  4. 联调测试                 │
                            └─────────────────────────────┘
                                   │
                                   ▼
                            ┌─────────────────────────────┐
                            │      Agent Kernel (x3)       │
                            │  ┌─────────┐ ┌─────────┐   │
                            │  │Agent A  │ │Agent B  │   │
                            │  │(后端)   │ │(前端)   │   │
                            │  └────┬────┘ └────┬────┘   │
                            │       └─────┬─────┘        │
                            │             ▼              │
                            │      ┌─────────┐           │
                            │      │ 结果聚合  │           │
                            │      └────┬────┘           │
                            └───────────┼────────────────┘
                                        │
                                        ▼
                            ┌─────────────────────────────┐
                            │         Client              │
                            │  "博客系统已搭建完成..."      │
                            └─────────────────────────────┘
```

---

## 6. 接口契约

### 6.1 Agent Kernel 接口

```go
// internal/kernel/interfaces.go

package kernel

import "context"

// Agent 是内核的核心抽象
type Agent interface {
    // Execute 执行用户请求，返回事件流
    Execute(ctx context.Context, req *ExecuteRequest) (<-chan Event, error)
    
    // State 获取当前状态
    State() AgentState
    
    // ID 获取 Agent ID
    ID() string
}

// ExecuteRequest 执行请求
type ExecuteRequest struct {
    SessionID   string                 // 会话ID（用于记忆关联）
    Message     string                 // 用户输入
    Context     map[string]interface{} // 附加上下文
    Tools       []string               // 允许使用的工具（空=全部）
    ModelHint   string                 // 模型偏好标签
    Stream      bool                   // 是否流式
}

// Event 是内核输出的统一事件
type Event interface {
    Type() EventType
    Timestamp() time.Time
}

type EventType string

const (
    EventTypeThinking   EventType = "thinking"    // 推理过程
    EventTypeToolCall   EventType = "tool_call"   // 工具调用
    EventTypeToolResult EventType = "tool_result" // 工具结果
    EventTypeMessage    EventType = "message"     // 最终消息
    EventTypeError      EventType = "error"       // 错误
    EventTypeComplete   EventType = "complete"    // 完成
)

// ThinkingEvent 推理事件
type ThinkingEvent struct {
    Content string
    ts      time.Time
}

func (e ThinkingEvent) Type() EventType      { return EventTypeThinking }
func (e ThinkingEvent) Timestamp() time.Time { return e.ts }

// ToolCallEvent 工具调用事件
type ToolCallEvent struct {
    ToolName  string
    Arguments map[string]interface{}
    ts        time.Time
}

func (e ToolCallEvent) Type() EventType      { return EventTypeToolCall }
func (e ToolCallEvent) Timestamp() time.Time { return e.ts }

// ToolResultEvent 工具结果事件
type ToolResultEvent struct {
    ToolName string
    Result   string
    Success  bool
    ts       time.Time
}

func (e ToolResultEvent) Type() EventType      { return EventTypeToolResult }
func (e ToolResultEvent) Timestamp() time.Time { return e.ts }

// MessageEvent 消息事件
type MessageEvent struct {
    Content string
    Done    bool
    ts      time.Time
}

func (e MessageEvent) Type() EventType      { return EventTypeMessage }
func (e MessageEvent) Timestamp() time.Time { return e.ts }

// AgentState Agent 状态
type AgentState string

const (
    AgentStateIdle      AgentState = "idle"
    AgentStateThinking  AgentState = "thinking"
    AgentStateActing    AgentState = "acting"
    AgentStateObserving AgentState = "observing"
    AgentStateComplete  AgentState = "complete"
    AgentStateFailed    AgentState = "failed"
)
```

### 6.2 记忆系统接口

```go
// internal/memory/interfaces.go

package memory

import "context"

// System 记忆系统接口
type System interface {
    // Recall 根据查询检索相关记忆
    Recall(ctx context.Context, query string, sessionID string, limit int) ([]Memory, error)
    
    // Store 存储新记忆
    Store(ctx context.Context, m Memory) error
    
    // Forget 删除记忆
    Forget(ctx context.Context, memoryID string) error
    
    // Summarize 生成对话摘要
    Summarize(ctx context.Context, sessionID string) (string, error)
}

// Memory 记忆条目
type Memory struct {
    ID        string
    Content   string
    Type      MemoryType
    Source    string      // 来源：对话/工具/用户
    SessionID string
    CreatedAt time.Time
    Metadata  map[string]interface{}
}

type MemoryType string

const (
    MemoryTypeFact      MemoryType = "fact"      // 事实
    MemoryTypePreference MemoryType = "preference" // 偏好
    MemoryTypeProcedure MemoryType = "procedure" // 流程
    MemoryTypeSummary   MemoryType = "summary"   // 摘要
)
```

### 6.3 工具系统接口

```go
// internal/tools/interfaces.go

package tools

import "context"

// Registry 工具注册表
type Registry interface {
    // Register 注册工具
    Register(tool Tool) error
    
    // Get 获取工具定义
    Get(name string) (Tool, bool)
    
    // List 列出所有工具
    List() []Tool
    
    // ListByNames 按名称列表获取工具
    ListByNames(names []string) []Tool
}

// Tool 工具定义
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema
    Handler     Handler
}

// Handler 工具执行函数
type Handler func(ctx context.Context, args map[string]interface{}) (Result, error)

// Result 工具执行结果
type Result struct {
    Content string
    Format  string // text/json/markdown
    Error   error
}
```

### 6.4 编排层接口

```go
// internal/orchestration/interfaces.go

package orchestration

import (
    "context"
    "openaide/backend/internal/kernel"
)

// Planner 任务规划器
type Planner interface {
    // Decompose 将目标拆解为可执行步骤
    Decompose(ctx context.Context, goal string, agent *kernel.Agent) ([]Step, error)
}

// Step 执行步骤
type Step struct {
    ID          string
    Name        string
    Description string
    AgentID     string      // 执行该步骤的 Agent
    DependsOn   []string    // 依赖的步骤ID
    Input       map[string]interface{}
}

// Team 多 Agent 协作
type Team interface {
    // Execute 执行团队任务
    Execute(ctx context.Context, task *Task) (*TeamResult, error)
    
    // AddAgent 添加 Agent
    AddAgent(agent *kernel.Agent, role string) error
    
    // RemoveAgent 移除 Agent
    RemoveAgent(agentID string) error
}

// Task 团队任务
type Task struct {
    ID          string
    Goal        string
    Steps       []Step
    Agents      map[string]*kernel.Agent
    MaxRounds   int
}

// TeamResult 团队执行结果
type TeamResult struct {
    TaskID     string
    Success    bool
    Output     string
    StepResults map[string]StepResult
    Duration   time.Duration
}
```

### 6.5 LLM 网关接口

```go
// internal/llm/gateway.go

package llm

import "context"

// Gateway LLM 网关
type Gateway interface {
    // Chat 发送聊天请求
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    
    // ChatStream 流式聊天
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
    
    // RouteModel 根据能力标签选择模型
    RouteModel(capability string) (string, error)
    
    // RegisterProvider 注册提供商
    RegisterProvider(name string, client LLMClient) error
}

// Capability 模型能力标签
type Capability string

const (
    CapabilityFast      Capability = "fast"      // 快速响应
    CapabilityReasoning Capability = "reasoning" // 深度推理
    CapabilityCoding    Capability = "coding"    // 代码生成
    CapabilityLong      Capability = "long"      // 长上下文
    CapabilityCheap     Capability = "cheap"     // 低成本
)
```

### 6.6 增强能力接口

```go
// internal/kernel/reflection.go

package kernel

import "context"

// Reflector 自我反思器
type Reflector interface {
    Reflect(ctx context.Context, execution *Execution) (*Reflection, error)
    Evaluate(ctx context.Context, query, output string) (*Evaluation, error)
    Improve(ctx context.Context, reflection *Reflection) (*Improvement, error)
}

type Reflection struct {
    QualityScore float64
    Issues       []Issue
    Improvements []string
    Confidence   float64
    Learned      bool
}

type Issue struct {
    Type        string  // factual/logical/linguistic/safety
    Severity    string  // critical/high/medium/low
    Description string
    Location    string
}
```

```go
// internal/kernel/learning.go

package kernel

import "context"

// Learner 学习器
type Learner interface {
    LearnFromFeedback(ctx context.Context, feedback *Feedback) error
    LearnFromExecution(ctx context.Context, execution *Execution) error
    GetLearnedPreference(ctx context.Context, userID string) (*Preference, error)
    AdaptStrategy(ctx context.Context, agent *Agent) error
}

type Feedback struct {
    UserID    string
    TaskID    string
    Rating    int
    Comment   string
    Tags      []string
    Timestamp time.Time
}

type Preference struct {
    UserID         string
    ResponseStyle  string
    PreferredTools []string
    AvoidedTopics  []string
    Confidence     float64
}
```

```go
// internal/kernel/alignment.go

package kernel

import "context"

// Aligner 价值对齐器
type Aligner interface {
    // Check 检查是否合规（支持权限和绕过）
    Check(ctx context.Context, input string, scope CheckScope) (*AlignmentResult, error)
    
    // Align 对输出进行对齐调整
    Align(ctx context.Context, output string, scope CheckScope) (string, error)
    
    // AddGuideline 添加准则（需要权限）
    AddGuideline(ctx context.Context, guideline *Guideline, ownerID string) error
    
    // RemoveGuideline 删除准则（需要权限）
    RemoveGuideline(ctx context.Context, guidelineID string, userID string) error
    
    // BypassCheck 绕过检查（需要高权限）
    BypassCheck(ctx context.Context, userID string, reason string) (BypassToken, error)
}

// CheckScope 检查范围（决定用哪些规则）
type CheckScope struct {
    UserID      string       // 用户ID（决定用户级规则）
    AgentID     string       // AgentID（决定Agent级规则）
    Level       CheckLevel   // 检查级别
    BypassToken BypassToken  // 绕过令牌（可选）
}

// CheckLevel 检查级别
type CheckLevel string

const (
    CheckLevelNone     CheckLevel = "none"     // 不检查
    CheckLevelMinimal  CheckLevel = "minimal"  // 最小检查（仅安全）
    CheckLevelStandard CheckLevel = "standard" // 标准检查（默认）
    CheckLevelStrict   CheckLevel = "strict"   // 严格检查
)

// UserRole 用户角色
type UserRole string

const (
    UserRoleGuest   UserRole = "guest"   // 访客：强制严格检查
    UserRoleUser    UserRole = "user"    // 普通用户：标准检查
    UserRolePremium UserRole = "premium" // 高级用户：可降级检查
    UserRoleAdmin   UserRole = "admin"   // 管理员：可自定义规则
    UserRoleSystem  UserRole = "system"  // 系统：可完全关闭
)

// Guideline 价值准则
type Guideline struct {
    ID           string         // 唯一标识
    Name         string         // 名称
    Description  string         // 描述
    Rules        []Rule         // 规则列表
    Priority     int            // 优先级
    Enabled      bool           // 是否启用
    OwnerID      string         // 所有者（system/user/agent）
    Scope        GuidelineScope // 适用范围
    RequiredRole UserRole       // 最低需要角色才能修改
}

// GuidelineScope 准则适用范围
type GuidelineScope string

const (
    GuidelineScopeSystem GuidelineScope = "system" // 系统级：所有用户
    GuidelineScopeUser   GuidelineScope = "user"   // 用户级：仅该用户
    GuidelineScopeAgent  GuidelineScope = "agent"  // Agent级：仅该Agent
)

// Rule 规则
type Rule struct {
    ID         string     // 唯一标识
    Name       string     // 名称
    Type       string     // keyword/regex/semantic
    Pattern    string     // 匹配模式
    Action     RuleAction // 动作
    Severity   string     // critical/high/medium/low
    Bypassable bool       // 是否可绕过（高权限用户）
}

// RuleAction 规则动作
type RuleAction string

const (
    RuleActionBlock   RuleAction = "block"   // 拦截
    RuleActionWarn    RuleAction = "warn"    // 警告（记录但放行）
    RuleActionRewrite RuleAction = "rewrite" // 重写
    RuleActionSkip    RuleAction = "skip"    // 跳过（仅记录）
    RuleActionNotify  RuleAction = "notify"  // 通知管理员
)

// AlignmentResult 对齐结果
type AlignmentResult struct {
    Compliant    bool        // 是否合规
    Level        CheckLevel  // 检查级别
    Violations   []Violation // 违规列表
    Suggestions  []string    // 建议
    Bypassed     bool        // 是否被绕过
    BypassReason string      // 绕过原因
}

// Violation 违规
type Violation struct {
    RuleID      string     // 规则ID
    RuleName    string     // 规则名称
    Severity    string     // 严重程度
    Location    string     // 位置
    Description string     // 描述
    Action      RuleAction // 动作
    Bypassable  bool       // 是否可绕过
}

// BypassToken 绕过令牌
type BypassToken struct {
    Token     string    // 令牌
    UserID    string    // 用户ID
    ExpiresAt time.Time // 过期时间
    Scope     []string  // 可绕过的规则ID
    Reason    string    // 绕过原因
}
```

```go
// internal/kernel/multimodal.go

package kernel

import "context"

// MultimodalProcessor 多模态处理器
type MultimodalProcessor interface {
    ProcessImage(ctx context.Context, image []byte) (*ImageUnderstanding, error)
    ProcessAudio(ctx context.Context, audio []byte) (*AudioUnderstanding, error)
    ProcessDocument(ctx context.Context, doc []byte, mimeType string) (*DocumentUnderstanding, error)
    GenerateImage(ctx context.Context, prompt string) ([]byte, error)
    GenerateAudio(ctx context.Context, text string) ([]byte, error)
}

type ImageUnderstanding struct {
    Description string
    Objects     []DetectedObject
    Text        string
    Context     string
}

type DetectedObject struct {
    Name        string
    BoundingBox [4]float64
    Confidence  float64
}
```

---

## 7. 事件系统设计

### 7.1 设计目标

事件系统是内核与外部世界通信的**唯一通道**，支撑以下能力：

| 能力 | 说明 |
|------|------|
| **实时反馈** | 流式输出、思考过程可视化 |
| **可观测性** | 调试、监控、日志 |
| **可扩展性** | 插件、Hook、自定义处理 |
| **持久化** | 会话恢复、审计追溯、回放 |
| **多消费者** | 前端、日志、监控、审计并行接收 |

### 7.2 事件类型层级

```
┌─────────────────────────────────────────┐
│           事件类型层级                    │
│                                         │
│  系统事件 (System Events)               │
│  ├── SessionStarted    - 会话开始       │
│  ├── SessionEnded      - 会话结束       │
│  ├── AgentSelected     - Agent 已选择   │
│  └── Error             - 系统错误       │
│                                         │
│  ReAct 事件 (ReAct Events)              │
│  ├── ThinkingStarted   - 开始思考       │
│  ├── Thinking          - 思考内容       │
│  ├── ThinkingEnded     - 思考结束       │
│  ├── ToolCallStarted   - 开始调用工具   │
│  ├── ToolCall          - 工具调用详情   │
│  ├── ToolCallEnded     - 工具调用结束   │
│  ├── ToolResult        - 工具结果       │
│  ├── MessageStarted    - 开始生成消息   │
│  ├── Message           - 消息内容（流式）│
│  └── MessageEnded      - 消息生成结束   │
│                                         │
│  增强能力事件 (Enhancement Events)      │
│  ├── ReflectionStarted - 开始反思       │
│  ├── Reflection        - 反思结果       │
│  ├── Correction        - 纠错结果       │
│  ├── AlignmentCheck    - 对齐检查       │
│  └── LearningUpdate    - 学习更新       │
│                                         │
│  状态事件 (State Events)                │
│  ├── StateChanged      - 状态变更       │
│  ├── Progress          - 进度更新       │
│  ├── Cancelled         - 已取消         │
│  └── Timeout           - 超时           │
│                                         │
│  元数据事件 (Meta Events)               │
│  ├── TokenUsage        - Token 消耗     │
│  ├── Cost              - 成本           │
│  ├── Latency           - 延迟           │
│  └── MemoryAccess      - 记忆访问       │
│                                         │
└─────────────────────────────────────────┘
```

### 7.3 事件接口定义

```go
// internal/kernel/events.go

package kernel

import "time"

// Event 基础事件接口
type Event interface {
    Type() EventType
    Timestamp() time.Time
    SessionID() string
    Round() int
    Step() string
    Metadata() map[string]interface{}
}

// EventType 事件类型
type EventType string

// 系统事件
const (
    EventTypeSessionStarted EventType = "session_started"
    EventTypeSessionEnded   EventType = "session_ended"
    EventTypeAgentSelected  EventType = "agent_selected"
    EventTypeError          EventType = "error"
)

// ReAct 事件
const (
    EventTypeThinkingStarted EventType = "thinking_started"
    EventTypeThinking        EventType = "thinking"
    EventTypeThinkingEnded   EventType = "thinking_ended"
    EventTypeToolCallStarted EventType = "tool_call_started"
    EventTypeToolCall        EventType = "tool_call"
    EventTypeToolCallEnded   EventType = "tool_call_ended"
    EventTypeToolResult      EventType = "tool_result"
    EventTypeMessageStarted  EventType = "message_started"
    EventTypeMessage         EventType = "message"
    EventTypeMessageEnded    EventType = "message_ended"
)

// 增强能力事件
const (
    EventTypeReflectionStarted EventType = "reflection_started"
    EventTypeReflection        EventType = "reflection"
    EventTypeCorrection        EventType = "correction"
    EventTypeAlignmentCheck    EventType = "alignment_check"
    EventTypeLearningUpdate    EventType = "learning_update"
)

// 状态事件
const (
    EventTypeStateChanged EventType = "state_changed"
    EventTypeProgress     EventType = "progress"
    EventTypeCancelled    EventType = "cancelled"
    EventTypeTimeout      EventType = "timeout"
)

// 元数据事件
const (
    EventTypeTokenUsage   EventType = "token_usage"
    EventTypeCost         EventType = "cost"
    EventTypeLatency      EventType = "latency"
    EventTypeMemoryAccess EventType = "memory_access"
)
```

### 7.4 事件持久化

#### 存储架构

```
实时流 ──► 内存缓冲区 ──► 批量写入
    │          │              │
    │          ▼              ▼
    │    ┌─────────┐    ┌─────────┐
    │    │  Ring   │    │ SQLite  │
    │    │ Buffer  │    │ (events)│
    │    │ (100条) │    │         │
    │    └─────────┘    └─────────┘
    │          │              │
    ▼          ▼              ▼
┌─────────┐ ┌─────────┐  ┌─────────┐
│  SSE    │ │  File   │  │  Replay │
│ Stream  │ │ (归档)  │  │ Consumer│
└─────────┘ └─────────┘  └─────────┘
```

#### 数据模型

```go
// internal/infra/event_store.go

package infra

import "time"

// StoredEvent 持久化事件
type StoredEvent struct {
    ID          string                 `gorm:"primaryKey"`
    SessionID   string                 `gorm:"index"`
    ExecutionID string                 `gorm:"index"`
    Type        string                 `gorm:"index"`
    Round       int
    Step        string
    Timestamp   time.Time              `gorm:"index"`
    Data        map[string]interface{} `gorm:"serializer:json"`
    Metadata    map[string]interface{} `gorm:"serializer:json"`
    Duration    int64
    CreatedAt   time.Time
}

// EventStore 事件存储接口
type EventStore interface {
    Append(ctx context.Context, event *StoredEvent) error
    AppendBatch(ctx context.Context, events []*StoredEvent) error
    GetBySession(ctx context.Context, sessionID string, limit int) ([]*StoredEvent, error)
    GetByExecution(ctx context.Context, executionID string) ([]*StoredEvent, error)
    GetByType(ctx context.Context, eventType string, limit int) ([]*StoredEvent, error)
    GetByTimeRange(ctx context.Context, start, end time.Time) ([]*StoredEvent, error)
    Replay(ctx context.Context, sessionID string, handler EventHandler) error
    Purge(ctx context.Context, before time.Time) error
    PurgeBySession(ctx context.Context, sessionID string) error
}
```

#### 批量写入策略

```go
// EventBuffer 事件缓冲区
type EventBuffer struct {
    buffer        []*StoredEvent
    capacity      int
    flushInterval time.Duration
    store         EventStore
    mu            sync.Mutex
}

func (b *EventBuffer) Append(event *StoredEvent) {
    b.mu.Lock()
    b.buffer = append(b.buffer, event)
    shouldFlush := len(b.buffer) >= b.capacity
    b.mu.Unlock()
    if shouldFlush {
        b.Flush()
    }
}

func (b *EventBuffer) Flush() error {
    b.mu.Lock()
    events := make([]*StoredEvent, len(b.buffer))
    copy(events, b.buffer)
    b.buffer = b.buffer[:0]
    b.mu.Unlock()
    if len(events) == 0 {
        return nil
    }
    return b.store.AppendBatch(context.Background(), events)
}
```

#### 回放功能

```go
// ReplayOptions 回放选项
type ReplayOptions struct {
    Speed     float64       // 回放速度 (1.0 = 实时)
    Filter    []string      // 只回放指定类型
    SkipDelay bool          // 跳过延迟
    StartFrom time.Time     // 从指定时间开始
    EndAt     time.Time     // 到指定时间结束
}

func (s *SQLiteEventStore) ReplayWithOptions(
    ctx context.Context,
    sessionID string,
    opts ReplayOptions,
    handler EventHandler,
) error {
    events, err := s.GetBySession(ctx, sessionID, 10000)
    if err != nil {
        return err
    }
    var lastTimestamp time.Time
    for _, event := range events {
        if len(opts.Filter) > 0 && !contains(opts.Filter, event.Type) {
            continue
        }
        if !opts.StartFrom.IsZero() && event.Timestamp.Before(opts.StartFrom) {
            continue
        }
        if !opts.EndAt.IsZero() && event.Timestamp.After(opts.EndAt) {
            continue
        }
        if !opts.SkipDelay && !lastTimestamp.IsZero() {
            delay := event.Timestamp.Sub(lastTimestamp)
            delay = time.Duration(float64(delay) / opts.Speed)
            time.Sleep(delay)
        }
        lastTimestamp = event.Timestamp
        if err := handler(ctx, event); err != nil {
            return err
        }
    }
    return nil
}
```

### 7.5 事件订阅（多消费者）

#### 发布订阅架构

```
┌─────────────────────────────────────────┐
│           事件总线 (Event Bus)            │
│                                         │
│  ┌─────────────┐                        │
│  │   Publisher │                        │
│  │   (内核)     │                        │
│  └──────┬──────┘                        │
│         │ Publish(event)                 │
│         ▼                                │
│  ┌─────────────────────────────────┐    │
│  │         Event Bus               │    │
│  │  ┌───────────────────────────┐  │    │
│  │  │  Topic: "session.*"       │  │    │
│  │  │  Topic: "react.*"         │  │    │
│  │  │  Topic: "enhancement.*"   │  │    │
│  │  │  Topic: "meta.*"          │  │    │
│  │  └───────────────────────────┘  │    │
│  └──────┬──────┬──────┬──────┬────┘    │
│         │      │      │      │          │
│    ┌────┘      │      │      └────┐     │
│    ▼           ▼      ▼           ▼     │
│ ┌──────┐   ┌──────┐ ┌──────┐  ┌──────┐ │
│ │ SSE  │   │Logger│ │Metrics│ │Audit │ │
│ │Consumer│  │Consumer│ │Consumer│ │Consumer│ │
│ │(前端)  │  │(日志) │ │(监控) │ │(审计) │ │
│ └──────┘   └──────┘ └──────┘  └──────┘ │
│                                         │
└─────────────────────────────────────────┘
```

#### 接口定义

```go
// internal/infra/event_bus.go

package infra

import "context"

// EventBus 事件总线
type EventBus interface {
    Publish(ctx context.Context, topic string, event Event) error
    PublishSync(ctx context.Context, topic string, event Event) error
    Subscribe(topic string, handler EventHandler) Subscription
    SubscribePattern(pattern string, handler EventHandler) Subscription
    Unsubscribe(sub Subscription)
    Close() error
}

// Subscription 订阅
type Subscription struct {
    ID      string
    Topic   string
    Handler EventHandler
    Active  bool
}

// EventHandler 事件处理器
type EventHandler func(ctx context.Context, event Event) error
```

#### 消费者示例

```go
// 1. SSE 消费者（前端实时推送）
func NewSSEConsumer(w http.ResponseWriter) EventHandler {
    return func(ctx context.Context, event Event) error {
        data, _ := json.Marshal(event)
        fmt.Fprintf(w, "data: %s\n\n", data)
        w.(http.Flusher).Flush()
        return nil
    }
}

// 2. 日志消费者
func NewLoggerConsumer(logger Logger) EventHandler {
    return func(ctx context.Context, event Event) error {
        logger.Info("event",
            "type", event.Type(),
            "session", event.SessionID(),
            "round", event.Round(),
        )
        return nil
    }
}

// 3. 指标消费者（Prometheus）
func NewMetricsConsumer() EventHandler {
    return func(ctx context.Context, event Event) error {
        switch event.Type() {
        case EventTypeThinking:
            thinkingDuration.Observe(event.Duration())
        case EventTypeToolCall:
            toolCallCounter.Inc()
        case EventTypeTokenUsage:
            tokenUsageCounter.Add(float64(event.TotalTokens()))
        }
        return nil
    }
}

// 4. 审计消费者
func NewAuditConsumer(store EventStore) EventHandler {
    return func(ctx context.Context, event Event) error {
        if !isSensitiveEvent(event.Type()) {
            return nil
        }
        stored := &StoredEvent{
            ID:        generateID(),
            SessionID: event.SessionID(),
            Type:      string(event.Type()),
            Timestamp: event.Timestamp(),
            Data:      event.Metadata(),
        }
        return store.Append(ctx, stored)
    }
}
```

### 7.6 事件过滤

#### 过滤接口

```go
// internal/infra/event_filter.go

package infra

// EventFilter 事件过滤器
type EventFilter interface {
    Match(event Event) bool
}

// FilterBuilder 过滤器构建器
type FilterBuilder struct {
    conditions []FilterCondition
}

type FilterCondition struct {
    Field    string
    Operator string
    Value    interface{}
}

func (b *FilterBuilder) TypeIn(types ...string) *FilterBuilder {
    b.conditions = append(b.conditions, FilterCondition{
        Field: "type", Operator: "in", Value: types,
    })
    return b
}

func (b *FilterBuilder) SessionID(sessionID string) *FilterBuilder {
    b.conditions = append(b.conditions, FilterCondition{
        Field: "session_id", Operator: "eq", Value: sessionID,
    })
    return b
}

func (b *FilterBuilder) Build() EventFilter {
    return &compositeFilter{conditions: b.conditions}
}
```

#### 过滤订阅

```go
// 前端只关注当前会话的消息和工具事件
filter := NewFilterBuilder().
    SessionID("sess_001").
    TypeIn("message", "thinking", "tool_call", "tool_result", "error").
    Build()
sub := eventBus.SubscribeWithFilter("session.*", filter, handler)

// 开发者只关注错误和性能
filter := NewFilterBuilder().
    TypeIn("error", "latency", "token_usage").
    Build()
sub := eventBus.SubscribeWithFilter("metrics.*", filter, handler)

// 安全只关注对齐和绕过
filter := NewFilterBuilder().
    TypeIn("alignment_check", "reflection", "learning_update").
    Build()
sub := eventBus.SubscribeWithFilter("audit.*", filter, handler)
```

### 7.7 事件压缩

#### 压缩策略

| 策略 | 适用场景 | 压缩率 |
|------|----------|--------|
| 字段裁剪 | 前端不需要的字段 | 30-50% |
| 批量发送 | 高频小事件 | 40-60% |
| Delta 编码 | 相似事件 | 50-70% |
| 二进制编码 | 全量压缩 | 70-90% |

#### 压缩实现

```go
// internal/infra/event_compression.go

package infra

// EventCompressor 事件压缩器
type EventCompressor interface {
    Compress(event Event) ([]byte, error)
    Decompress(data []byte) (Event, error)
}

// FieldPruner 字段裁剪
type FieldPruner struct {
    keepFields map[string]bool
}

func (p *FieldPruner) Prune(event Event) Event {
    metadata := event.Metadata()
    pruned := make(map[string]interface{})
    for k, v := range metadata {
        if p.keepFields[k] {
            pruned[k] = v
        }
    }
    return &PrunedEvent{base: event, metadata: pruned}
}

// 前端只需要这些字段
var FrontendFields = map[string]bool{
    "content":       true,
    "tool_name":     true,
    "arguments":     true,
    "result":        true,
    "success":       true,
    "done":          true,
    "quality_score": true,
}

// BatchCompressor 批量压缩
type BatchCompressor struct {
    buffer   []Event
    maxSize  int
    maxDelay time.Duration
}

func (c *BatchCompressor) Add(event Event) ([]Event, bool) {
    c.buffer = append(c.buffer, event)
    if len(c.buffer) >= c.maxSize {
        return c.flush(), true
    }
    return nil, false
}

func (c *BatchCompressor) flush() []Event {
    events := make([]Event, len(c.buffer))
    copy(events, c.buffer)
    c.buffer = c.buffer[:0]
    return events
}

// BinaryCompressor 二进制编码（MessagePack）
type BinaryCompressor struct{}

func (c *BinaryCompressor) Compress(event Event) ([]byte, error) {
    return msgpack.Marshal(event)
}

func (c *BinaryCompressor) Decompress(data []byte) (Event, error) {
    var event StoredEvent
    err := msgpack.Unmarshal(data, &event)
    return &event, err
}
```

#### 智能压缩选择

```go
// 根据消费者选择压缩策略
func SelectCompression(consumer string) EventCompressor {
    switch consumer {
    case "frontend":
        return NewChainCompressor(
            NewFieldPruner(FrontendFields),
            NewDeltaCompressor(),
            NewBinaryCompressor(),
        )
    case "logger":
        return NewNoopCompressor()
    case "metrics":
        return NewBatchCompressor(100, 5*time.Second)
    case "audit":
        return NewNoopCompressor()
    default:
        return NewBinaryCompressor()
    }
}
```

### 7.8 事件系统架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        事件系统架构                              │
│                                                                  │
│  ┌─────────────┐                                                │
│  │   Kernel    │                                                │
│  │  (Publisher)│                                                │
│  └──────┬──────┘                                                │
│         │ Event                                                  │
│         ▼                                                        │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    Event Bus                             │    │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐   │    │
│  │  │ Filter  │  │ Compress│  │  Route  │  │ Persist │   │    │
│  │  │ (过滤)   │  │ (压缩)   │  │ (路由)   │  │ (持久化) │   │    │
│  │  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘   │    │
│  └───────┼────────────┼────────────┼────────────┼────────┘    │
│          │            │            │            │               │
│     ┌────┴────┐  ┌────┴────┐  ┌────┴────┐  ┌────┴────┐       │
│     ▼         ▼  ▼         ▼  ▼         ▼  ▼         ▼       │
│  ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐      │
│  │ SSE  │ │Logger│ │Metrics│ │Audit │ │Replay│ │Train │      │
│  │Consumer│ │Consumer│ │Consumer│ │Consumer│ │Consumer│ │Consumer│      │
│  │(前端)  │ │(日志) │ │(监控) │ │(审计) │ │(回放) │ │(训练) │      │
│  └──────┘ └──────┘ └──────┘ └──────┘ └──────┘ └──────┘      │
│                                                                  │
│  存储层:                                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐                        │
│  │ SQLite  │  │  File   │  │  Ring   │                        │
│  │ (热数据)│  │ (归档)  │  │ Buffer  │                        │
│  │ 7天     │  │ 90天    │  │ (实时)  │                        │
│  └─────────┘  └─────────┘  └─────────┘                        │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 8. 项目目录结构

### 7.1 目标目录结构（混合模式）

```
openaide/
├── cmd/
│   └── openaide/
│       └── main.go                      # 统一入口：根据子命令分发
│
├── internal/                            # 私有代码（不对外暴露）
│   │
│   ├── kernel/                          # ████ Agent 内核 ████
│   │   ├── agent.go                     # Agent 定义与配置
│   │   ├── reactor.go                   # ReAct 执行引擎
│   │   ├── state_machine.go             # 状态机
│   │   ├── events.go                    # 事件定义
│   │   └── config.go                    # 内核配置
│   │
│   ├── memory/                          # 记忆系统
│   │   ├── interfaces.go                # 记忆接口
│   │   ├── short_term.go                # 工作记忆（对话上下文）
│   │   ├── long_term.go                 # 长期记忆（SQLite）
│   │   ├── vector.go                    # 向量记忆（RAG）
│   │   ├── embedding.go                 # 嵌入服务
│   │   └── compressor.go                # 上下文压缩
│   │
│   ├── tools/                           # 工具系统
│   │   ├── interfaces.go                # 工具接口
│   │   ├── registry.go                  # 工具注册表
│   │   ├── executor.go                  # 工具执行器
│   │   ├── mcp.go                       # MCP 客户端适配
│   │   └── native/                      # 内置工具
│   │       ├── bash.go
│   │       ├── file.go
│   │       ├── code.go
│   │       ├── search.go
│   │       └── http.go
│   │
│   ├── llm/                             # LLM 网关
│   │   ├── gateway.go                   # 网关核心
│   │   ├── client.go                    # 统一客户端接口
│   │   ├── router.go                    # 模型路由
│   │   └── providers/                   # 各提供商实现
│   │       ├── openai.go
│   │       ├── anthropic.go
│   │       ├── deepseek.go
│   │       ├── gemini.go
│   │       ├── ollama.go
│   │       └── ...
│   │
│   ├── orchestration/                   # 编排层（可选）
│   │   ├── interfaces.go                # 编排接口
│   │   ├── planner.go                   # 任务规划器
│   │   ├── team.go                      # 多 Agent 团队
│   │   ├── workflow.go                  # 工作流引擎
│   │   └── scheduler.go                 # 调度器
│   │
│   ├── api/                             # HTTP API 层（可选）
│   │   ├── server.go                    # HTTP 服务器
│   │   ├── router.go                    # 路由注册
│   │   ├── middleware/                  # 中间件
│   │   │   ├── auth.go
│   │   │   ├── cors.go
│   │   │   ├── rate_limit.go
│   │   │   └── request_id.go
│   │   ├── handlers/                    # 处理器
│   │   │   ├── chat.go                  # 对话
│   │   │   ├── agent.go                 # Agent 管理
│   │   │   ├── session.go               # 会话管理
│   │   │   ├── task.go                  # 任务管理
│   │   │   └── system.go                # 系统接口
│   │   └── websocket.go                 # WebSocket
│   │
│   ├── cli/                             # 命令行交互（直接模式）
│   │   ├── chat.go                      # 对话命令
│   │   ├── config.go                    # 配置命令
│   │   └── interactive.go               # 交互式终端
│   │
│   ├── tui/                             # TUI 界面（可选）
│   │   ├── app.go                       # TUI 应用
│   │   ├── views/                       # 视图组件
│   │   └── styles/                      # 样式定义
│   │
│   └── infra/                           # 基础设施
│       ├── config.go                    # 配置管理
│       ├── database.go                  # 数据库
│       ├── cache.go                     # 缓存
│       ├── vector.go                    # 向量存储
│       ├── event_bus.go                 # 事件总线
│       └── logger.go                    # 日志
│
├── pkg/                                 # 可复用公共库
│   ├── errors/
│   ├── logger/
│   └── utils/
│
├── configs/
│   └── config.yaml                      # 默认配置文件
│
├── docs/                                # 文档
│   ├── ARCHITECTURE.md                  # 本文件
│   ├── API.md                           # API 文档
│   └── DEPLOYMENT.md                    # 部署文档
│
├── examples/                            # 示例
│   └── skills/
│
├── scripts/                             # 脚本
│   ├── install.sh
│   └── deploy.sh
│
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

### 7.2 运行模式

| 命令 | 模式 | 说明 |
|------|------|------|
| `openaide chat` | **直接模式（默认）** | 直接对话，无需启动 server |
| `openaide server` | **服务器模式** | 启动 HTTP API，供远程连接 |
| `openaide tui` | **TUI 模式** | 启动 TUI 界面（连接本地或远程） |
| `openaide config` | **配置命令** | 查看和修改配置 |

### 7.3 三种模式架构

#### 直接模式（默认）

```
用户 -> CLI -> Kernel -> LLM/Tools
```

```go
func runDirectMode() {
    cfg := config.Load()
    kernel := kernel.New(cfg)
    cli := cli.New(kernel)
    cli.Run()  // 交互式对话
}
```

#### 服务器模式

```
用户 -> HTTP Client -> API Server -> Kernel -> LLM/Tools
```

```go
func runServerMode() {
    cfg := config.Load()
    kernel := kernel.New(cfg)
    server := api.NewServer(kernel)
    server.Start(":8080")
}
```

#### TUI 模式

```
用户 -> TUI -> HTTP Client -> API Server (本地或远程)
```

```go
func runTUIMode() {
    client := api.NewClient("http://localhost:8080")
    tui := tui.New(client)
    tui.Run()
}
```

### 7.4 与现有目录对比

| 现有路径 | 目标路径 | 说明 |
|----------|----------|------|
| `backend/cmd/server/` | `cmd/openaide/` | 统一入口 |
| `backend/internal/` | `internal/` | 合并到根目录 |
| `terminal/` | `internal/tui/` + `internal/cli/` | TUI 和 CLI 合并到 internal |
| `backend/src/app.go` | `internal/api/server.go` | 应用容器 → API 服务器 |
| `backend/src/router.go` | `internal/api/router.go` | 路由注册 |
| `backend/src/handlers/` | `internal/api/handlers/` | HTTP 处理器 |
| `backend/src/services/agent_executor.go` | `internal/kernel/reactor.go` | Agent 执行引擎 |
| `backend/src/services/agent_router.go` | `internal/llm/router.go` | 模型路由 |
| `backend/src/services/tool_calling_service.go` | `internal/kernel/reactor.go` + `internal/tools/` | 工具调用 → 内核 + 工具系统 |
| `backend/src/services/dialogue_service.go` | `internal/memory/short_term.go` | 对话管理 → 工作记忆 |
| `backend/src/services/memory_service.go` | `internal/memory/long_term.go` | 记忆服务 → 长期记忆 |
| `backend/src/services/context_engine.go` | `internal/memory/compressor.go` | 上下文引擎 → 压缩器 |
| `backend/src/services/llm/` | `internal/llm/providers/` | LLM 客户端保留 |
| `backend/src/services/orchestration/` | `internal/orchestration/` | 编排保留 |
| `backend/src/services/workflow_service.go` | `internal/orchestration/workflow.go` | 工作流服务 |
| `backend/src/services/storage/` | `internal/infra/` | 存储 → 基础设施 |
| `backend/src/models/` | `internal/models/` | 数据模型 |
| `backend/src/config/` | `internal/infra/config.go` | 配置管理 |
| `backend/src/middleware/` | `internal/api/middleware/` | 中间件 |

### 7.2 与现有目录对比

| 现有路径 | 目标路径 | 说明 |
|----------|----------|------|
| `backend/src/app.go` | `backend/internal/api/server.go` | 应用容器 → API 服务器 |
| `backend/src/router.go` | `backend/internal/api/router.go` | 路由注册 |
| `backend/src/handlers/` | `backend/internal/api/handlers/` | HTTP 处理器 |
| `backend/src/services/agent_executor.go` | `backend/internal/kernel/reactor.go` | Agent 执行引擎 |
| `backend/src/services/agent_router.go` | `backend/internal/llm/router.go` | 模型路由 |
| `backend/src/services/tool_calling_service.go` | `backend/internal/kernel/reactor.go` + `internal/tools/` | 工具调用 → 内核 + 工具系统 |
| `backend/src/services/dialogue_service.go` | `backend/internal/memory/short_term.go` | 对话管理 → 工作记忆 |
| `backend/src/services/memory_service.go` | `backend/internal/memory/long_term.go` | 记忆服务 → 长期记忆 |
| `backend/src/services/context_engine.go` | `backend/internal/memory/compressor.go` | 上下文引擎 → 压缩器 |
| `backend/src/services/llm/` | `backend/internal/llm/providers/` | LLM 客户端保留 |
| `backend/src/services/orchestration/` | `backend/internal/orchestration/` | 编排保留 |
| `backend/src/services/workflow_service.go` | `backend/internal/orchestration/workflow.go` | 工作流服务 |
| `backend/src/services/storage/` | `backend/internal/infra/` | 存储 → 基础设施 |
| `backend/src/models/` | `backend/internal/models/` | 数据模型 |
| `backend/src/config/` | `backend/internal/infra/config.go` | 配置管理 |
| `backend/src/middleware/` | `backend/internal/api/middleware/` | 中间件 |

---

## 8. 现有服务映射

### 8.1 服务归类表

现有 `services/` 下约 80+ 文件，按新架构归类：

#### 归入 kernel/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `agent_executor.go` | `kernel/reactor.go` | ReAct 引擎 |
| `agent_roles.go` | `kernel/agent.go` | Agent 角色定义 |
| `react_state.go` | `kernel/state_machine.go` | ReAct 状态 |

#### 归入 memory/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `memory_service.go` | `memory/long_term.go` | 长期记忆 |
| `memory_embedding_service.go` | `memory/embedding.go` | 嵌入服务 |
| `memory_extraction_service.go` | `memory/extractor.go` | 记忆提取 |
| `memory_index_service.go` | `memory/vector.go` | 向量索引 |
| `context_engine.go` | `memory/compressor.go` | 上下文压缩 |
| `context_manager.go` | `memory/short_term.go` | 上下文管理 |
| `persistent_memory_service.go` | `memory/persistent.go` | 持久化记忆 |
| `rag_service.go` | `memory/rag.go` | RAG 检索 |
| `structured_compaction_service.go` | `memory/compaction.go` | 结构化压缩 |

#### 归入 tools/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `tool_service.go` | `tools/registry.go` | 工具注册 |
| `tool_calling_service.go` | `tools/executor.go` | 工具执行 |
| `tool_call_guardrail.go` | `tools/guardrail.go` | 工具安全 |
| `tool_validator.go` | `tools/validator.go` | 工具校验 |
| `smart_tool_selector.go` | `tools/selector.go` | 工具选择 |
| `mcp/` | `tools/mcp.go` | MCP 客户端 |
| `dev_tools.go` | `tools/native/code.go` | 开发工具 |
| `extended_tools.go` | `tools/native/` | 扩展工具 |

#### 归入 llm/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `llm/llm_client.go` | `llm/client.go` | 统一接口 |
| `llm/openai_client.go` | `llm/providers/openai.go` | OpenAI |
| `llm/anthropic_client.go` | `llm/providers/anthropic.go` | Anthropic |
| `llm/deepseek_client.go` | `llm/providers/deepseek.go` | DeepSeek |
| `llm/gemini_client.go` | `llm/providers/gemini.go` | Gemini |
| `llm/ollama_client.go` | `llm/providers/ollama.go` | Ollama |
| `llm/...` | `llm/providers/...` | 其他提供商 |
| `model_service.go` | `llm/gateway.go` | 模型管理 |
| `model_router.go` | `llm/router.go` | 模型路由 |
| `token_estimator.go` | `llm/token.go` | Token 估算 |
| `token_limit_service.go` | `llm/token.go` | Token 限制 |
| `token_budget.go` | `llm/budget.go` | Token 预算 |
| `cost_optimizer.go` | `llm/optimizer.go` | 成本优化 |
| `embedding_service.go` | `llm/embedding.go` | 嵌入服务 |

#### 归入 orchestration/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `orchestration/team_orchestrator.go` | `orchestration/team.go` | 团队编排 |
| `orchestration/task_analyzer.go` | `orchestration/planner.go` | 任务分析 |
| `orchestration/team_planner.go` | `orchestration/planner.go` | 团队规划 |
| `orchestration/project_manager.go` | `orchestration/project.go` | 项目管理 |
| `orchestration/confirmation_flow.go` | `orchestration/confirmation.go` | 确认流程 |
| `workflow_service.go` | `orchestration/workflow.go` | 工作流 |
| `workflow_state_machine.go` | `orchestration/workflow.go` | 工作流状态 |
| `workflow_scheduler.go` | `orchestration/scheduler.go` | 工作流调度 |
| `plan_service.go` | `orchestration/planner.go` | 计划服务 |
| `structured_planner.go` | `orchestration/planner.go` | 结构化规划 |
| `consensus_planner.go` | `orchestration/planner.go` | 共识规划 |
| `plan_review_service.go` | `orchestration/review.go` | 计划评审 |
| `replanning_engine.go` | `orchestration/replan.go` | 重新规划 |
| `request_orchestrator.go` | `orchestration/orchestrator.go` | 请求编排 |
| `orchestration_service.go` | `orchestration/service.go` | 编排服务 |
| `team_coordinator.go` | `orchestration/team.go` | 团队协调 |
| `team_coordinator_service.go` | `orchestration/team.go` | 团队协调服务 |
| `multi_agent_service.go` | `orchestration/team.go` | 多 Agent |
| `delegation_service.go` | `orchestration/delegation.go` | 任务委托 |

#### 归入 api/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `handlers/*_handler.go` | `api/handlers/*.go` | HTTP 处理器 |
| `middleware/` | `api/middleware/` | 中间件 |

#### 归入 infra/

| 现有文件 | 新位置 | 说明 |
|----------|--------|------|
| `storage/database.go` | `infra/database.go` | 数据库 |
| `storage/cache.go` | `infra/cache.go` | 缓存 |
| `storage/vector_store.go` | `infra/vector.go` | 向量存储 |
| `storage/memory_vector_store.go` | `infra/vector.go` | 内存向量 |
| `storage/ledis_cache.go` | `infra/cache.go` | Ledis 缓存 |
| `config/config.go` | `infra/config.go` | 配置 |
| `logger/logger.go` | `infra/logger.go` | 日志 |
| `event_bus.go` | `infra/event_bus.go` | 事件总线 |
| `rate_limiter.go` | `infra/rate_limit.go` | 限流器 |

#### 删除或合并

| 现有文件 | 处理方式 | 理由 |
|----------|----------|------|
| `workflow_service2.go` | ✅ 已删除 | 未使用 |
| `skill_service.go` + `skill_discovery_service.go` + `skill_evolution_service.go` + `skill_import_service.go` | 合并为 `kernel/skill.go` | 技能管理收敛到内核 |
| `thinking_service.go` + `cot_reasoning_service.go` + `self_reflection_service.go` | 合并到 `kernel/reactor.go` | 思考/推理是内核能力 |
| `dialogue_service.go` + `enhanced_dialogue_service.go` | 合并到 `memory/short_term.go` | 对话是工作记忆 |
| `automation_service.go` | 合并到 `orchestration/workflow.go` | 自动化 = 工作流 |
| `sandbox_service.go` | 合并到 `tools/executor.go` | 沙箱 = 工具执行 |
| `code_service.go` + `code_search_service.go` + `doc_gen_service.go` + `format_service.go` + `test_gen_service.go` + `dependency_service.go` + `deploy_service.go` | 合并为 `tools/native/code.go` | 代码工具收敛 |
| `guardian_service.go` + `guardian_service_test.go` | 合并到 `tools/guardrail.go` | 安全守卫 |
| `pattern_detector.go` + `pattern_detector_service.go` | 合并到 `kernel/pattern.go` | 模式检测 |
| `correction_service.go` | 合并到 `kernel/reactor.go` | 纠错是内核能力 |
| `deep_interview_service.go` | 删除 | 未使用 |
| `capability_gap_service.go` | 删除 | 未使用 |
| `usage_service.go` | 简化保留 | 用量统计 |
| `voice_service.go` | 保留 | 语音功能 |
| `feishu_service.go` | 保留 | 飞书集成 |
| `channel_service.go` | 保留 | 渠道管理 |
| `plugin_service.go` | 保留 | 插件系统 |
| `prompt_service.go` + `prompt_template_service.go` | 合并 | 提示词管理 |
| `knowledge_service.go` + `knowledge_extraction_service.go` + `document_service.go` | 合并到 `memory/knowledge.go` | 知识管理 |
| `learning_service.go` + `task_service.go` + `scheduler_service.go` | 保留/简化 | 学习/任务/调度 |
| `confirmation_service.go` | 合并到 `orchestration/confirmation.go` | 确认流程 |
| `feedback_service.go` + `user_feedback_collector.go` | 合并 | 反馈收集 |
| `session_branch_service.go` | 合并到 `api/session.go` | 会话分支 |
| `exec_policy_service.go` | 合并到 `tools/guardrail.go` | 执行策略 |
| `hook_engine.go` + `post_hook_service.go` | 合并 | Hook 系统 |
| `integration_test_service.go` + `integration_test.go` | 保留 | 集成测试 |
| `base_service.go` | 删除 | 基类反模式 |
| `api_types.go` | 合并到 `api/types.go` | API 类型 |
| `hnsw_index.go` + `hnsw_vector_store.go` + `persistent_hnsw.go` + `vector_manager.go` | 合并到 `infra/vector.go` | HNSW 向量 |
| `ollama_embedding_service.go` | 合并到 `llm/embedding.go` | Ollama 嵌入 |
| `local_knowledge_first.go` | 删除 | 未使用 |
| `llm_budget_manager.go` | 合并到 `llm/budget.go` | 预算管理 |
| `smart_cache_service.go` | 合并到 `infra/cache.go` | 智能缓存 |
| `key_info_extractor.go` | 合并到 `memory/extractor.go` | 关键信息 |
| `progressive_skill_loader.go` | 删除 | 未使用 |
| `slash_command.go` | 保留 | 斜杠命令 |

---

## 9. 迁移路线图

### 9.1 Phase 1: 内核提取（Week 1-3）

**目标**: 建立 `internal/kernel/` 包，将 Agent 核心能力收敛

| 周 | 任务 | 产出 |
|----|------|------|
| W1 | 创建目录结构，定义接口 | `kernel/interfaces.go` |
| W1 | 重构 AgentExecutor → Reactor | `kernel/reactor.go` |
| W1 | 定义 Agent 实体 | `kernel/agent.go` |
| W2 | 实现状态机 | `kernel/state_machine.go` |
| W2 | 定义事件系统 | `kernel/events.go` |
| W2 | 编写内核单元测试 | `kernel/*_test.go` |
| W3 | 集成测试：内核独立运行 | 测试通过 |
| W3 | 文档更新 | 接口文档 |

**风险**: AgentExecutor 被多处引用，需要逐步迁移。
**缓解**: 保留现有实现，新增内核实现，通过接口兼容过渡。

### 9.2 Phase 2: 记忆系统重构（Week 3-5）

**目标**: 建立 `internal/memory/` 包，统一记忆管理

| 周 | 任务 | 产出 |
|----|------|------|
| W3 | 定义记忆接口 | `memory/interfaces.go` |
| W3 | 迁移 MemoryService | `memory/long_term.go` |
| W4 | 迁移 ContextEngine | `memory/compressor.go` |
| W4 | 迁移 RAGService | `memory/rag.go` |
| W4 | 统一向量存储 | `memory/vector.go` |
| W5 | 集成测试 | 测试通过 |

### 9.3 Phase 3: 工具系统重构（Week 5-6）

**目标**: 建立 `internal/tools/` 包，统一工具管理

| 周 | 任务 | 产出 |
|----|------|------|
| W5 | 定义工具接口 | `tools/interfaces.go` |
| W5 | 迁移 ToolService | `tools/registry.go` |
| W5 | 迁移 ToolCallingService | `tools/executor.go` |
| W6 | 迁移 MCP 客户端 | `tools/mcp.go` |
| W6 | 迁移内置工具 | `tools/native/*.go` |

### 9.4 Phase 4: LLM 层精简（Week 6-7）

**目标**: 精简 `internal/llm/`，建立网关

| 周 | 任务 | 产出 |
|----|------|------|
| W6 | 定义网关接口 | `llm/gateway.go` |
| W6 | 迁移 ModelService | `llm/gateway.go` |
| W7 | 迁移模型路由 | `llm/router.go` |
| W7 | 保留提供商实现 | `llm/providers/*.go` |

### 9.5 Phase 5: 编排层精简（Week 7-8）

**目标**: 精简 `internal/orchestration/`，统一规划器

| 周 | 任务 | 产出 |
|----|------|------|
| W7 | 定义编排接口 | `orchestration/interfaces.go` |
| W7 | 合并 Planner | `orchestration/planner.go` |
| W8 | 迁移 TeamOrchestrator | `orchestration/team.go` |
| W8 | 迁移 WorkflowService | `orchestration/workflow.go` |

### 9.6 Phase 6: API 层重构（Week 8-9）

**目标**: 精简 handlers，建立轻薄外壳

| 周 | 任务 | 产出 |
|----|------|------|
| W8 | 创建 API 目录结构 | `api/` |
| W8 | 迁移核心 handlers | `api/handlers/chat.go` |
| W9 | 迁移剩余 handlers | `api/handlers/*.go` |
| W9 | 统一中间件 | `api/middleware/*.go` |

### 9.7 Phase 7: 基础设施迁移（Week 9-10）

**目标**: 建立 `internal/infra/`

| 周 | 任务 | 产出 |
|----|------|------|
| W9 | 迁移数据库 | `infra/database.go` |
| W9 | 迁移缓存 | `infra/cache.go` |
| W10 | 迁移向量存储 | `infra/vector.go` |
| W10 | 迁移事件总线 | `infra/event_bus.go` |

### 9.8 Phase 8: 集成与清理（Week 10-12）

**目标**: 整合测试，清理旧代码

| 周 | 任务 | 产出 |
|----|------|------|
| W10 | 整合所有模块 | 编译通过 |
| W11 | 端到端测试 | 测试通过 |
| W11 | 性能测试 | 基准数据 |
| W12 | 删除旧 services/ 目录 | 清理完成 |
| W12 | 文档更新 | 完整文档 |

### 9.9 迁移总览

```
Week:  1    2    3    4    5    6    7    8    9    10   11   12
       ├────┴────┤    ├────┴────┤    ├────┤    ├────┤    ├────┴────┤
       │ 内核提取 │    │ 记忆重构 │    │工具 │    │LLM │    │ 编排精简 │
       │         │    │         │    │重构 │    │精简 │    │         │
       └─────────┘    └─────────┘    └────┘    └────┘    └─────────┘
                                              ├────┴────┤    ├────┴────┤
                                              │ API 重构 │    │ 集成清理 │
                                              │         │    │         │
                                              └─────────┘    └─────────┘
```

---

## 10. 附录

### 10.1 术语表

| 术语 | 英文 | 定义 |
|------|------|------|
| Agent | Agent | 具有角色、能力、记忆的智能实体 |
| ReAct | ReAct | Reasoning + Acting，推理-行动循环 |
| 内核 | Kernel | Agent 的核心执行引擎 |
| 编排 | Orchestration | 多步骤/多 Agent 的任务调度 |
| 工作流 | Workflow | 预定义的自动化流程 |
| 记忆 | Memory | 信息的存储与检索系统 |
| 工具 | Tool | Agent 可调用的外部能力 |
| MCP | Model Context Protocol | 模型上下文协议 |
| RAG | Retrieval-Augmented Generation | 检索增强生成 |
| SSE | Server-Sent Events | 服务器推送事件 |
| 自我反思 | Reflection | 对执行结果进行质量评估和改进 |
| 学习进化 | Learning | 从反馈中优化 Agent 策略 |
| 模式检测 | Pattern Detection | 发现用户行为和工作流模式 |
| 自动纠错 | Correction | 评估并修正输出错误 |
| 价值对齐 | Alignment | 确保输出符合价值观和安全准则 |
| 多模态 | Multimodal | 处理图像、音频、文档等多种输入 |
| 纯文件存储 | Pure File Storage | Markdown + JSON + 二进制向量，零数据库依赖 |
| 零 CGO | Zero CGO | 所有存储纯 Go 实现，禁止 CGO 绑定 |
| 混合运行模式 | Hybrid Runtime | 直接模式(默认) + 服务器模式(可选) + TUI 模式(可选) |
| 上下文压缩 | Context Compression | 借鉴小说结构的对话摘要技术 |
| 小说式压缩 | Novel Compression | 章节/人物/伏笔/闪回多维度压缩 |

### 10.2 参考架构

- [OpenAI Assistants API](https://platform.openai.com/docs/assistants)
- [LangChain Agent](https://python.langchain.com/docs/modules/agents/)
- [AutoGPT Architecture](https://github.com/Significant-Gravitas/AutoGPT)
- [CrewAI Multi-Agent](https://docs.crewai.com/)

### 10.3 决策记录

| 日期 | 决策 | 理由 | 替代方案 |
|------|------|------|----------|
| 2026-05-12 | 内核放在 `internal/` | 不对外暴露，防止外部依赖 | `pkg/kernel/`（对外暴露） |
| 2026-05-12 | 编排层可选 | 简单任务不需要编排开销 | 所有任务都经过编排 |
| 2026-05-14 | 纯文件存储 | Markdown + JSON + 二进制向量，零数据库依赖 | SQLite（需 CGO/外部依赖） |
| 2026-05-12 | 保留 HNSW | 内存向量，高性能 | Pinecone/Milvus（外部依赖） |
| 2026-05-12 | 事件流返回 | 支持流式、工具调用可视化 | 只返回最终结果 |
| 2026-05-13 | 增强能力作为内核扩展 | 自我反思/学习/对齐是 Agent 必备能力 | 作为独立服务 |
| 2026-05-13 | 多模态作为可选模块 | 资源消耗大，非所有场景需要 | 强制启用 |
| 2026-05-13 | 价值对齐可配置 | 不同用户/场景价值观不同 | 固定准则 |
| 2026-05-13 | 价值对齐权限分级 | 高权限用户需要更大自由度 | 统一规则 |
| 2026-05-13 | 价值对齐可绕过 | 特定场景（如学术研究）需要绕过 | 不可绕过 |
| 2026-05-13 | 异步反思 | 不阻塞主流程，降低延迟 | 同步反思 |
| 2026-05-14 | 上下文压缩融合架构 | 结合小说类比 + MemGPT + RAG + Claude 上下文检索 + LangChain 父文档 | 单一方案 |
| 2026-05-14 | 四层混合压缩 | Layer1 小说结构 + Layer2 内存分离 + Layer3 RAG 检索 + Layer4 动态预算 | 单层压缩 |
| 2026-05-14 | 纯文件存储架构 | Markdown + JSON 索引 + 二进制向量，零数据库依赖 | SQLite + 外部服务 |
| 2026-05-14 | 零 CGO 原则 | 所有存储纯 Go 实现，禁止 CGO | 使用 CGO 绑定 |
| 2026-05-14 | 混合运行模式 | 直接模式(默认) + 服务器模式(可选) + TUI 模式(可选) | 前后端强制分离 |

### 10.4 增强能力实施计划

#### Phase 1: 已有能力迁移（Week 3-4）

| 周 | 任务 | 来源文件 | 目标文件 |
|----|------|----------|----------|
| W3 | 迁移自我反思 | `self_reflection_service.go` | `kernel/reflection.go` |
| W3 | 迁移思维链推理 | `cot_reasoning_service.go` | 整合到 `kernel/reactor.go` |
| W4 | 迁移学习进化 | `learning_service.go` | `kernel/learning.go` |
| W4 | 迁移模式检测 | `pattern_detector.go` | `kernel/pattern.go` |
| W4 | 迁移自动纠错 | `correction_service.go` | `kernel/correction.go` |

#### Phase 2: 新增能力（Week 5-6）

| 周 | 任务 | 文件 | 说明 |
|----|------|------|------|
| W5 | 实现价值对齐 | `kernel/alignment.go` | 新增，可配置准则 |
| W5 | 实现安全守卫 | `kernel/guardrail.go` | 增强现有 guardian |
| W6 | 实现多模态（可选） | `kernel/multimodal.go` | 新增，插件化 |

#### Phase 3: 联动优化（Week 6-7）

| 周 | 任务 | 说明 |
|----|------|------|
| W6 | 反思与学习联动 | 反思结果自动触发学习 |
| W6 | 纠错与反思结合 | 纠错前先做反思评估 |
| W7 | 模式自动优化 | 检测到的模式自动建议工作流 |
| W7 | 对齐与学习结合 | 从对齐违规中学习用户偏好 |

### 10.5 增强能力难点与解决方案

| 能力 | 主要难点 | 解决方案 |
|------|----------|----------|
| **自我反思** | 额外 LLM 调用增加延迟和成本 | 异步执行 + 可配置触发条件 |
| **学习进化** | 数据积累慢，个性化与通用性平衡 | 渐进式学习 + 置信度机制 |
| **模式检测** | 误报率高，随机行为被误认为模式 | 最小样本阈值 + 置信度过滤 |
| **自动纠错** | 纠错本身可能出错，过度纠错 | 置信度阈值 + 用户确认 |
| **价值对齐** | 价值观主观，过度限制创造力 | 可配置准则 + 分层对齐 + 权限分级 + 可绕过 |
| **多模态** | 资源消耗大，模型依赖多 | 可选模块 + 插件化 + 渐进支持 |

### 10.6 性能基准指标

| 指标 | 目标值 | 测试条件 | 说明 |
|------|--------|----------|------|
| **首次响应时间** | < 500ms | 本地运行，直接模式 | 从用户输入到首字返回 |
| **完整响应时间** | < 3s | 单次 LLM 调用，无工具 | 简单问答场景 |
| **工具调用延迟** | < 100ms | 本地工具，无网络 | 文件操作、命令执行 |
| **MCP 工具延迟** | < 500ms | 本地 MCP 服务 | 外部工具通过本地协议 |
| **上下文压缩时间** | < 200ms | 100 轮对话压缩 | 生成章节摘要 |
| **记忆检索时间** | < 50ms | L3 长期记忆，1000 条 | JSON 索引查询 |
| **向量检索时间** | < 100ms | 10000 条向量，Top-5 | HNSW 内存索引 |
| **事件处理吞吐** | > 10000/s | 内存模式，无持久化 | 事件总线分发 |
| **事件持久化吞吐** | > 1000/s | JSON 文件批量写入 | 批量刷盘模式 |
| **内存占用** | < 512MB | 空闲状态 | 仅内核 + 基础服务 |
| **内存占用** | < 2GB | 活跃会话，100 轮对话 | 含上下文和记忆 |
| **磁盘占用** | < 100MB | 新安装，无历史 | 仅二进制 + 配置 |
| **磁盘占用** | < 1GB/年 | 正常使用 | 会话 + 记忆 + 日志 |
| **启动时间** | < 1s | 直接模式 | 从命令到可交互 |
| **启动时间** | < 3s | 服务器模式 | 含 HTTP 服务启动 |

### 10.7 大型项目性能基准

| 指标 | 目标值 | 测试条件 | 说明 |
|------|--------|----------|------|
| **全量索引** | < 30s | 10K 文件，Go 项目 | 首次索引 |
| **增量更新** | < 3s | 100 文件变更 | 日常开发 |
| **变更检测** | < 1s | 10K 文件 | 指纹对比 |
| **符号查询** | < 50ms | 内存命中 | 函数/类型查找 |
| **跨模块搜索** | < 200ms | 5 个模块 | Workspace 范围 |
| **索引内存上限** | < 256MB | 可配置 | LRU 缓存控制 |
| **索引磁盘占用** | < 50MB | 10K 文件 | 纯 JSON 存储 |
| **上下文构建** | < 500ms | 大项目查询 | 分层加载 |
| **大项目内存** | < 1GB | 10K 文件项目活跃 | 含索引缓存 |

---

> 本文档为设计草案，欢迎评审和补充。  
> 如有疑问或建议，请在讨论区提出。
