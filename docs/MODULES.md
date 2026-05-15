# OpenAIDE 模块职责分工文档

> 版本: v3.0.0-draft  
> 日期: 2026-05-15  
> 状态: 设计评审中

---

## 目录

1. [模块总览](#1-模块总览)
2. [内核层 (kernel/)](#2-内核层-kernel)
3. [记忆层 (memory/)](#3-记忆层-memory)
4. [工具层 (tools/)](#4-工具层-tools)
5. [LLM 层 (llm/)](#5-llm-层-llm)
6. [编排层 (orchestration/)](#6-编排层-orchestration)
7. [API 层 (api/)](#7-api-层-api)
8. [基础设施层 (infra/)](#8-基础设施层-infra)
9. [事件系统 (infra/event*)](#9-事件系统-infraevent)
10. [终端会话隔离设计](#10-终端会话隔离设计)
11. [技能系统 (skill/)](#11-技能系统-skill)
12. [身份系统 (identity/)](#12-身份系统-identity)
13. [知识库系统 (knowledge/)](#13-知识库系统-knowledge)
14. [上下文压缩系统 (compress/)](#14-上下文压缩系统-compress)
15. [模块间依赖关系](#15-模块间依赖关系)
16. [增强能力联动设计](#16-增强能力联动设计)
17. [关键设计约束](#17-关键设计约束)
18. [版本兼容性设计](#18-版本兼容性设计)
19. [CLI 自动补全](#19-cli-自动补全)
20. [更新检查器](#20-更新检查器)
21. [CLI 命令设计](#21-cli-命令设计)
22. [纯文件存储架构](#22-纯文件存储架构)
23. [Git 集成与代码索引](#23-git-集成与代码索引系统)
24. [多模态支持](#24-多模态支持系统)
25. [插件系统](#25-插件系统设计)
26. [Skill 系统](#26-skill-系统设计)

---

## 1. 模块总览

### 1.1 模块清单

| 模块 | 路径 | 核心职责 | 文件数(预估) | 优先级 |
|------|------|----------|-------------|--------|
| **内核** | `internal/kernel/` | Agent 智能核心（含增强能力） | 12-15 | P0 |
| **记忆** | `internal/memory/` | 多级记忆系统 | 6-10 | P0 |
| **工具** | `internal/tools/` | 工具注册与执行 | 8-12 | P0 |
| **LLM** | `internal/llm/` | 模型网关与提供商 | 20+ | P0 |
| **编排** | `internal/orchestration/` | 任务规划与协作 | 5-8 | P1 |
| **API** | `internal/api/` | HTTP 接口与传输（可选） | 8-12 | P1 |
| **CLI** | `internal/cli/` | 命令行交互（直接模式） | 3-5 | P0 |
| **TUI** | `internal/tui/` | TUI 界面（可选） | 5-8 | P2 |
| **基础设施** | `internal/infra/` | 存储与配置 | 6-8 | P1 |

### 1.2 模块分层图

#### 直接模式（默认）

```
┌─────────────────────────────────────────────────────────────┐
│                         CLI 层                               │
│   ┌─────────┐ ┌─────────┐ ┌─────────┐                     │
│   │  Chat   │ │ Config  │ │Interactive│                    │
│   └────┬────┘ └────┬────┘ └────┬────┘                     │
└────────┼───────────┼───────────┼───────────────────────────┘
         │           │           │
         └───────────┴─────┬─────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
              ▼            ▼            ▼
       ┌──────────┐ ┌──────────┐ ┌──────────┐
       │ 编排层    │ │ 内核层    │ │ 记忆层    │
       │(可选增强) │ │(智能核心) │ │(信息存储) │
       └────┬─────┘ └────┬─────┘ └────┬─────┘
            │            │            │
            └────────────┼────────────┘
                         │
              ┌──────────┴──────────┐
              │                     │
              ▼                     ▼
       ┌──────────┐         ┌──────────┐
       │ 工具层    │         │ LLM 层   │
       │(能力扩展) │         │(模型调用) │
       └──────────┘         └──────────┘
              │                     │
              └──────────┬──────────┘
                         │
              ┌──────────▼──────────┐
              │    基础设施层        │
              │  DB │ Cache │ Vector │
              └─────────────────────┘
```

#### 服务器模式

```
┌─────────────────────────────────────────────────────────────┐
│                         API 层                               │
│   ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐        │
│   │ Server  │ │Handlers │ │Middleware│ │WebSocket│        │
│   └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘        │
└────────┼───────────┼───────────┼───────────┼──────────────┘
         │           │           │           │
         └───────────┴─────┬─────┴───────────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
              ▼            ▼            ▼
       ┌──────────┐ ┌──────────┐ ┌──────────┐
       │ 编排层    │ │ 内核层    │ │ 记忆层    │
       │(可选增强) │ │(智能核心) │ │(信息存储) │
       └────┬─────┘ └────┬─────┘ └────┬─────┘
            │            │            │
            └────────────┼────────────┘
                         │
              ┌──────────┴──────────┐
              │                     │
              ▼                     ▼
       ┌──────────┐         ┌──────────┐
       │ 工具层    │         │ LLM 层   │
       │(能力扩展) │         │(模型调用) │
       └──────────┘         └──────────┘
              │                     │
              └──────────┬──────────┘
                         │
              ┌──────────▼──────────┐
              │    基础设施层        │
              │  DB │ Cache │ Vector │
              └─────────────────────┘
```

---

## 2. 内核层 (kernel/)

### 2.1 职责定义

**一句话**: Agent 的智能核心，负责思考、决策、行动。

**详细职责**:
- 接收用户请求，执行 ReAct 循环
- 管理 Agent 状态机（idle → thinking → acting → observing → complete）
- 协调记忆检索、工具调用、LLM 对话
- 生成事件流（thinking/tool_call/tool_result/message）
- 处理错误和异常，优雅降级

### 2.2 文件清单

```
kernel/
├── interfaces.go          # 核心接口定义（Agent, Reactor, Event）
├── agent.go               # Agent 实体定义与配置
├── reactor.go             # ReAct 引擎实现
├── state_machine.go       # Agent 状态机
├── events.go              # 事件类型定义
├── config.go              # 内核配置
├── reflection.go          # 自我反思：执行后评估与改进
├── learning.go            # 学习进化：从反馈中优化策略
├── pattern.go             # 模式检测：发现用户行为模式
├── correction.go          # 自动纠错：输出质量评估与修正
├── alignment.go           # 价值对齐：输出安全与价值观检查
├── multimodal.go          # 多模态：图像/音频/文档处理（可选）
└── reactor_test.go        # 单元测试
```

### 2.3 核心接口

```go
// Agent 智能实体接口
type Agent interface {
    Execute(ctx context.Context, req *ExecuteRequest) (<-chan Event, error)
    State() AgentState
    ID() string
}

// Reactor ReAct 引擎接口
type Reactor interface {
    React(ctx context.Context, agent *Agent, req *ExecuteRequest) error
    Think(ctx context.Context, agent *Agent, input string) (*Thought, error)
    Act(ctx context.Context, agent *Agent, thought *Thought) (*Action, error)
    Observe(ctx context.Context, agent *Agent, action *Action) (*Observation, error)
}
```

### 2.4 输入输出

**输入**:
- `ExecuteRequest`: 用户消息、会话ID、工具白名单、模型偏好

**输出**:
- `Event` 通道: thinking → tool_call → tool_result → message → complete

### 2.5 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `memory.System` | 检索相关记忆 | 是 |
| `tools.Registry` | 获取可用工具定义 | 是（可为空） |
| `llm.Gateway` | 调用 LLM | 是 |
| `infra.Logger` | 记录执行日志 | 是 |

### 2.6 增强能力子模块

#### 2.6.1 自我反思 (reflection.go)

**职责**: 对执行结果进行质量评估和改进建议

| 方法 | 说明 | 触发时机 |
|------|------|----------|
| `Reflect` | 对完整执行进行反思 | 任务完成后 |
| `Evaluate` | 评估单条输出质量 | 每轮 ReAct 后 |
| `Improve` | 生成改进建议 | 反思后 |

**输出**: `Reflection` 结构体（质量评分、问题列表、改进建议、置信度）

**难点**: 额外 LLM 调用增加延迟  
**解决**: 异步执行 + 可配置触发条件

#### 2.6.2 学习进化 (learning.go)

**职责**: 从用户反馈和执行结果中学习，优化 Agent 策略

| 方法 | 说明 |
|------|------|
| `LearnFromFeedback` | 从用户评分/评论中学习 |
| `LearnFromExecution` | 从执行结果中总结经验 |
| `GetLearnedPreference` | 获取学习到的用户偏好 |
| `AdaptStrategy` | 根据学习调整 Agent 策略 |

**输出**: `Preference` 结构体（响应风格、偏好工具、回避主题、置信度）

**难点**: 数据积累慢，个性化与通用性平衡  
**解决**: 渐进式学习 + 置信度机制

#### 2.6.3 模式检测 (pattern.go)

**职责**: 检测用户行为模式和工作流模式

| 方法 | 说明 |
|------|------|
| `DetectWorkflowPatterns` | 检测重复的工作流模式 |
| `DetectUserPatterns` | 检测用户行为偏好 |
| `SuggestOptimization` | 基于模式建议优化 |

**输出**: `WorkflowPattern` / `UserPattern` 结构体

**难点**: 误报率高  
**解决**: 最小样本阈值 + 置信度过滤

#### 2.6.4 自动纠错 (correction.go)

**职责**: 评估输出质量并自动修正错误

| 方法 | 说明 |
|------|------|
| `Evaluate` | 评估输出质量（多维度） |
| `Correct` | 自动纠错（高置信度时） |
| `ShouldCorrect` | 判断是否需要纠错 |

**输出**: `EvaluationResult` / `CorrectionResult` 结构体

**难点**: 纠错本身可能出错  
**解决**: 置信度阈值 + 用户确认

#### 2.6.5 价值对齐 (alignment.go)

**职责**: 检查输出是否符合预设价值观和安全准则，支持权限分级和绕过机制

| 方法 | 说明 | 权限要求 |
|------|------|----------|
| `Check` | 检查是否合规（支持权限和绕过） | 无 |
| `Align` | 对输出进行对齐调整 | 无 |
| `AddGuideline` | 添加价值准则 | RequiredRole |
| `RemoveGuideline` | 删除价值准则 | RequiredRole |
| `BypassCheck` | 绕过检查（生成 BypassToken） | Premium+ |

**核心设计**:

```
┌─────────────────────────────────────────┐
│           对齐系统权限模型               │
│                                         │
│  角色层级:                              │
│  Guest ──► User ──► Premium ──► Admin ──► System │
│    │        │         │          │         │       │
│    │        │         │          │         │       │
│    ▼        ▼         ▼          ▼         ▼       ▼
│  Strict   Standard  Minimal    Custom    Custom   None  │
│  (强制)   (默认)    (可降级)   (可修改)  (可修改)  (关闭) │
│                                         │
│  绕过机制:                              │
│  - Premium+ 可绕过非安全规则            │
│  - 需要理由 + 生成 BypassToken          │
│  - 记录绕过日志                         │
│                                         │
│  规则层级:                              │
│  系统级 ──► 用户级 ──► Agent 级         │
│  (不可删)   (可自定义)  (可自定义)       │
└─────────────────────────────────────────┘
```

**检查级别**:

| 级别 | 说明 | 适用角色 | 检查范围 |
|------|------|----------|----------|
| `none` | 不检查 | System | 无 |
| `minimal` | 仅安全规则 | Premium+ | critical 级别 |
| `standard` | 标准检查（默认） | User | critical + high |
| `strict` | 严格检查 | Guest | 全部规则 |

**规则动作**:

| 动作 | 说明 | 适用场景 |
|------|------|----------|
| `block` | 拦截 | 严重违规 |
| `warn` | 警告（记录但放行） | 轻微违规 |
| `rewrite` | 重写 | 可修正的违规 |
| `skip` | 跳过（仅记录） | 低优先级 |
| `notify` | 通知管理员 | 需要人工审核 |

**输出**: `AlignmentResult` 结构体（是否合规、违规列表、建议、是否绕过）

**难点**: 价值观主观、过度限制创造力、不同用户需要不同自由度  
**解决**: 可配置准则 + 分层对齐 + 权限分级 + 可绕过机制

#### 2.6.6 多模态 (multimodal.go，可选)

**职责**: 处理图像、音频、文档等多模态输入

| 方法 | 说明 |
|------|------|
| `ProcessImage` | 图像理解（OCR、对象检测） |
| `ProcessAudio` | 音频理解（语音转文字） |
| `ProcessDocument` | 文档解析 |
| `GenerateImage` | 图像生成 |
| `GenerateAudio` | 音频生成 |

**输出**: `ImageUnderstanding` / `AudioUnderstanding` 结构体

**难点**: 资源消耗大，模型依赖多  
**解决**: 可选模块 + 插件化 + 渐进支持

### 2.7 不做什么

- ❌ 不处理 HTTP 请求
- ❌ 不管理用户会话
- ❌ 不直接连接数据库
- ❌ 不执行文件 I/O（委托给 tools.Executor）

---

## 3. 记忆层 (memory/)

### 3.1 职责定义

**一句话**: 管理 Agent 的记忆，支持多级存储与检索。

**详细职责**:
- 工作记忆：管理当前对话上下文
- 短期记忆：生成和检索对话摘要
- 长期记忆：存储用户偏好、事实、流程
- 向量记忆：支持语义检索（RAG）
- 记忆压缩：上下文过长时自动压缩

### 3.2 文件清单

```
memory/
├── interfaces.go          # 记忆系统接口
├── short_term.go          # 工作记忆（对话上下文）
├── long_term.go           # 长期记忆（SQLite）
├── vector.go              # 向量记忆（HNSW）
├── embedding.go           # 嵌入服务
├── compressor.go          # 上下文压缩
├── extractor.go           # 关键信息提取
├── rag.go                 # RAG 检索增强
└── knowledge.go           # 知识库管理
```

### 3.3 核心接口

```go
// System 记忆系统接口
type System interface {
    // Recall 检索记忆
    Recall(ctx context.Context, query string, sessionID string, limit int) ([]Memory, error)
    
    // Store 存储记忆
    Store(ctx context.Context, m Memory) error
    
    // Summarize 生成摘要
    Summarize(ctx context.Context, sessionID string) (string, error)
    
    // Compress 压缩上下文
    Compress(ctx context.Context, messages []Message, maxTokens int) ([]Message, error)
}

// Memory 记忆条目
type Memory struct {
    ID        string
    Content   string
    Type      MemoryType    // fact/preference/procedure/summary
    Source    string        // dialogue/tool/user
    SessionID string
    Embedding []float32     // 向量表示
    CreatedAt time.Time
}
```

### 3.4 三级记忆架构

```
┌─────────────────────────────────────────┐
│           L1: 工作记忆                   │
│  ┌─────────────────────────────────┐   │
│  │ 当前对话消息（最近 N 条）          │   │
│  │ 存储: 内存                        │   │
│  │ 作用: 直接注入 LLM 上下文          │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
                    │
                    ▼ 消息过多时
┌─────────────────────────────────────────┐
│           L2: 短期记忆                   │
│  ┌─────────────────────────────────┐   │
│  │ 对话摘要（压缩后）                │   │
│  │ 存储: SQLite                      │   │
│  │ 作用: 保留关键信息，减少 token     │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
                    │
                    ▼ 重要信息提取
┌─────────────────────────────────────────┐
│           L3: 长期记忆                   │
│  ┌─────────────────────────────────┐   │
│  │ 用户偏好、事实、流程              │   │
│  │ 存储: Markdown + 二进制向量文件   │   │
│  │ 作用: 跨会话持久化                 │   │
│  │ 检索: 语义相似度搜索               │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
```

### 3.5 记忆检索流程

```
用户输入: "帮我写个 Python 爬虫"
        │
        ▼
┌─────────────────────────────────────────┐
│ 1. 提取关键词: "Python", "爬虫"          │
│ 2. 向量检索 L3: 找到用户之前学过的       │
│    Python 相关记忆                       │
│ 3. 查询 L2: 获取近期对话摘要             │
│ 4. 组装 L1: 当前对话消息                 │
└─────────────────────────────────────────┘
        │
        ▼
注入到 System Prompt:
"用户之前学习过 Python 基础，偏好使用 requests 库..."
        │
        ▼
发送给 LLM
```

### 3.6 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `infra.DB` | 存储长期记忆 | 是 |
| `infra.Vector` | 向量存储与检索 | 否（可降级为关键词匹配） |
| `llm.Gateway` | 生成摘要和嵌入 | 是 |
| `infra.Logger` | 记录操作日志 | 是 |

### 3.7 详细设计决策

#### 3.7.1 记忆衰减策略

**决策**: 混合策略 — 按类型设置衰减率 + 用户主动遗忘

```go
type MemoryEntry struct {
    Content      string
    Type         MemoryType
    Confidence   float64
    LastAccessed time.Time
    AccessCount  int
    DecayRate    float64
}
```

| 记忆类型 | 衰减率 | 说明 |
|----------|--------|------|
| 用户偏好 | 0.001/天 | 几乎不衰减 |
| 技术事实 | 0.01/天 | 缓慢衰减 |
| 临时信息 | 0.1/天 | 快速衰减 |
| 已确认过时 | 标记删除 | 用户明确说"我不再用 Python 了" |

**好处**: 自动清理噪声、保留重要偏好、灵活可控
**不足**: 衰减参数需要调优、可能误衰减重要信息
**改进**: 引入访问频率和置信度加权，高频访问的记忆衰减慢

#### 3.7.2 记忆冲突处理

**决策**: 时间优先 + 保留历史版本（最近 5 个）

```go
type MemoryVersion struct {
    Content    string
    Timestamp  time.Time
    Confidence float64
    Source     string
}

type MemoryEntry struct {
    Current MemoryVersion
    History []MemoryVersion // 保留最近 5 个
}
```

**场景**: 用户 3 个月前说"用 VS Code"，昨天说"切换到 JetBrains"
- 当前有效: "用 JetBrains"
- 历史保留: ["用 VS Code" @ 3个月前]

**好处**: 保留最新偏好、可追溯历史、支持变化分析
**不足**: 存储增加、检索时需要处理版本
**改进**: 检测到冲突时，低置信度记忆自动降级而非删除

#### 3.7.3 上下文压缩策略

**决策**: 分层压缩 — 概要 + 摘要 + 近期完整

```
原始消息: [M1, M2, M3, M4, M5, M6, M7, M8, M9, M10]
           │                    │              │
           │                    │              └── 保留完整（最近3条）
           │                    └───────────────── 生成摘要（中间4条）
           └────────────────────────────────────── 生成概要（最早3条）

压缩后: [概要] + [摘要] + [M8, M9, M10]
```

**好处**: 平衡信息保留和 token 限制、近期细节完整
**不足**: 摘要可能丢失细节、压缩增加延迟
**改进**: 压缩前识别关键消息（如用户明确指令），优先保留

#### 3.7.4 记忆粒度设计

**决策**: 混合粒度

| 信息类型 | 粒度 | 存储位置 |
|----------|------|----------|
| 用户偏好 | 提取事实 | L3 长期记忆 |
| 项目背景 | 提取事实 | L3 长期记忆 |
| 当前任务 | 原始对话 | L1 工作记忆 |
| 历史上下文 | 摘要 | L2 短期记忆 |

**提取示例**:
```
对话: "我喜欢用 Python，特别是 requests 库做爬虫"
提取: [
  {"type": "preference", "content": "喜欢 Python"},
  {"type": "preference", "content": "偏好 requests 库"},
  {"type": "fact", "content": "有爬虫经验"}
]
```

**好处**: 存储高效、检索精准、层次分明
**不足**: 提取质量依赖 LLM、可能遗漏信息
**改进**: 使用专门的轻量级提取模型，降低 LLM 调用成本

#### 3.7.5 跨会话关联

**决策**: 用户级记忆 + 主题标签 + 语义检索排序

```go
type Memory struct {
    UserID    string
    SessionID string
    TopicTags []string  // 主题标签（自动提取）
    Content   string
    Embedding []float32
}
```

**检索排序权重**:
1. 语义相似度（基础分）
2. 主题匹配 ×1.5
3. 同会话 ×1.3
4. 时间衰减（旧记忆降权）

**好处**: 自然跨会话关联、主题精准匹配、灵活排序
**不足**: 主题提取增加开销、标签质量影响关联效果
**改进**: 主题标签 + 时间衰减 + 语义相似度三重排序

### 3.8 关键改进方向

| 方向 | 说明 | 优先级 |
|------|------|--------|
| 衰减优化 | 访问频率和置信度加权，避免误衰减 | P1 |
| 冲突降级 | 低置信度记忆自动降级而非删除 | P2 |
| 压缩保关键 | 识别关键消息优先保留 | P1 |
| 轻量提取 | 使用轻量级模型替代 LLM 提取 | P2 |
| 三重排序 | 主题 + 时间 + 语义综合排序 | P1 |

### 3.9 不做什么

- ❌ 不决定何时记忆（由 Reactor 触发）
- ❌ 不直接调用 LLM（通过 Gateway）
- ❌ 不管理对话生命周期

---

## 4. 工具层 (tools/)

### 4.1 职责定义

**一句话**: 管理 Agent 可调用的工具，提供注册、发现、执行能力。

**详细职责**:
- 工具注册表：统一管理所有可用工具
- 工具执行器：安全执行工具调用
- MCP 适配：对接外部 MCP 服务
- 内置工具：提供常用原生工具
- 权限控制：Agent 级别的工具白名单
- 参数校验：JSON Schema 校验

### 4.2 文件清单

```
tools/
├── interfaces.go          # 工具接口定义
├── registry.go            # 工具注册表
├── executor.go            # 工具执行器（沙箱）
├── guardrail.go           # 工具安全守卫
├── validator.go           # 参数校验
├── selector.go            # 智能工具选择
├── mcp.go                 # MCP 客户端适配
└── native/                # 内置工具
    ├── bash.go            # Shell 命令执行
    ├── file.go            # 文件读写
    ├── code.go            # 代码执行
    ├── search.go          # 网络搜索
    └── http.go            # HTTP 请求
```

### 4.3 核心接口

```go
// Registry 工具注册表
type Registry interface {
    Register(tool Tool) error
    Get(name string) (Tool, bool)
    List() []Tool
    ListByNames(names []string) []Tool
}

// Tool 工具定义
type Tool struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema
    Handler     Handler
    Category    string    // native/mcp/external
    Dangerous   bool      // 是否需要确认
}

// Handler 工具执行函数
type Handler func(ctx context.Context, args map[string]interface{}) (Result, error)

// Result 执行结果
type Result struct {
    Content string
    Format  string // text/json/markdown
    Error   error
}

// Executor 工具执行器
type Executor interface {
    Execute(ctx context.Context, toolName string, args map[string]interface{}) (Result, error)
    Validate(toolName string, args map[string]interface{}) error
}
```

### 4.4 工具分类

| 类别 | 示例 | 来源 | 安全级别 |
|------|------|------|----------|
| **Native** | bash, file, code, search | 内置 | 高（需确认） |
| **MCP** | filesystem, fetch, git | 外部 MCP | 中（MCP 协议） |
| **Custom** | 用户自定义 API | 用户配置 | 低（需审核） |

### 4.5 工具调用流程

```
LLM 返回 tool_calls
        │
        ▼
┌─────────────────┐
│ 1. 权限检查      │ ◄── Agent.Tools 白名单
│ 2. 参数校验      │ ◄── JSON Schema
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. 工具路由      │
│ Native ──► 本地执行器
│ MCP ──► MCP 客户端 ──► 外部服务
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 沙箱执行      │ ◄── 限制资源、超时控制
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 结果格式化    │ ◄── 转换为 LLM 可理解文本
│ 6. 错误处理      │ ◄── 失败返回错误信息
└────────┬────────┘
         │
         ▼
  追加到对话上下文
```

### 4.6 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `infra.Logger` | 记录工具调用 | 是 |
| `infra.Config` | 读取工具配置 | 是 |
| `llm.Gateway` | 智能工具选择 | 否 |

### 4.7 详细设计决策

#### 4.7.1 失败重试策略

**决策**: 智能判断 + 3次渐进退避重试

```go
type RetryPolicy struct {
    MaxRetries      int
    RetryableErrors []string
    Backoff         time.Duration
}
```

| 可重试错误 | 不可重试错误 |
|------------|--------------|
| 网络超时 | 权限拒绝 |
| 服务不可用 | 参数非法 |
| 连接重置 | 资源不存在 |
| 速率限制 | 安全策略拦截 |

**好处**: 容错性强、资源可控、渐进退避降低压力
**不足**: 延迟累积、非幂等操作可能重复执行
**改进**: 为工具标记幂等性，非幂等操作不重试

#### 4.7.2 危险工具确认

**决策**: 分级确认 + 同步阻塞（支持批量确认）

| 危险级别 | 示例 | 确认策略 |
|----------|------|----------|
| Low | cat, ls | 无需确认 |
| Medium | rm file, write | 需要确认 |
| High | rm -rf, format | 需要确认 + 理由说明 |

**好处**: 安全性高、分级灵活、可配置、可审计
**不足**: 打断体验、超时处理复杂、批量确认繁琐
**改进**: 支持异步确认 + 状态保持，批量确认模式

#### 4.7.3 MCP 热更新

**决策**: 定时刷新（5分钟）+ 事件通知 + 优雅下线

```go
func (c *MCPClient) StartDiscovery(ctx context.Context) {
    c.discover(ctx) // 立即发现
    ticker := time.NewTicker(5 * time.Minute)
    // ... 定时刷新
}
```

**好处**: 零停机、自动感知、事件驱动、解耦
**不足**: 发现延迟、状态不一致、依赖 MCP 服务可用性
**改进**: 支持管理后台主动刷新、工具删除延迟 1 分钟生效

#### 4.7.4 工具结果缓存

**决策**: 只读操作缓存（带 TTL），写操作绝不缓存

| 场景 | 缓存策略 | TTL |
|------|----------|-----|
| 天气查询 | 缓存 | 5分钟 |
| 文件读取 | 缓存 | 1分钟 |
| 数据库查询 | 可选 | 自定义 |
| 写操作 | 不缓存 | - |

**好处**: 减少延迟、降低成本、提升吞吐
**不足**: 数据过期、缓存穿透、缓存雪崩、副作用误判
**改进**: 增加随机抖动 TTL、单飞请求防穿透、缓存预热

#### 4.7.5 并发执行

**决策**: 支持并行，信号量限制最大并发 3，支持依赖图执行

```go
func (e *Executor) ExecuteBatch(ctx context.Context, calls []ToolCall) []Result {
    sem := make(chan struct{}, 3) // 最大并发 3
    // ... WaitGroup 收集结果
}
```

**好处**: 提升效率、资源可控、结果完整
**不足**: 上下文隔离、错误处理复杂、依赖关系、资源竞争
**改进**: 支持 DAG 依赖图执行，按拓扑排序分层并行

### 4.8 不做什么

- ❌ 不决定调用哪个工具（由 LLM/Reactor 决定）
- ❌ 不管理工具的业务逻辑（由 Handler 实现）
- ❌ 不持久化工具结果（由调用方处理）

---

## 5. LLM 层 (llm/)

### 5.1 职责定义

**一句话**: 统一的大语言模型调用网关，支持多提供商、路由、负载均衡。

**详细职责**:
- 统一接口：所有 LLM 调用通过同一接口
- 提供商适配：支持 17+ 提供商
- 模型路由：根据能力标签选择模型
- 负载均衡：多 key 轮询
- 故障转移：主模型失败时降级
- 用量统计：记录 token 消耗
- 响应缓存：相同请求直接返回

### 5.2 文件清单

```
llm/
├── gateway.go             # LLM 网关核心
├── client.go              # 统一客户端接口（OpenAI 兼容）
├── router.go              # 模型路由
├── health.go              # 提供商健康检查
├── token.go               # Token 估算与限制
├── budget.go              # 预算管理
├── optimizer.go           # 成本优化
├── embedding.go           # 嵌入服务
├── provider_config.go     # 提供商配置管理
└── providers/             # 核心提供商实现（仅保留最常用的 3-5 个）
    ├── openai.go          # OpenAI 原生（作为基准）
    ├── anthropic.go       # Claude 系列
    └── ollama.go          # 本地模型
```

**其他提供商通过配置文件注册，无需代码实现**：
```yaml
# providers.yaml
providers:
  - name: deepseek
    base_url: https://api.deepseek.com/v1
    api_key: ${DEEPSEEK_API_KEY}
    models:
      deepseek-chat: deepseek-chat
      deepseek-coder: deepseek-coder
  - name: gemini
    base_url: https://generativelanguage.googleapis.com/v1beta
    api_key: ${GEMINI_API_KEY}
    models:
      gemini-pro: gemini-pro
```

### 5.3 核心接口

```go
// Gateway LLM 网关
type Gateway interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
    RouteModel(capability string) (string, error)
    RegisterProvider(name string, client LLMClient) error
}

// LLMClient 提供商客户端接口
type LLMClient interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
}

// ChatRequest 聊天请求
type ChatRequest struct {
    Messages      []Message
    Model         string
    Temperature   float64
    MaxTokens     int
    Tools         []ToolDefinition
    ToolChoice    interface{}
    Stream        bool
}

// Capability 模型能力标签
type Capability string

const (
    CapabilityFast      Capability = "fast"
    CapabilityReasoning Capability = "reasoning"
    CapabilityCoding    Capability = "coding"
    CapabilityLong      Capability = "long"
    CapabilityCheap     Capability = "cheap"
)
```

### 5.4 模型路由策略

```
用户请求 ──► Gateway.RouteModel("coding")
                    │
                    ▼
            ┌───────────────┐
            │ 能力标签匹配   │
            │ coding ──►    │
            │ 1. Claude 3.5 │
            │ 2. GPT-4o     │
            │ 3. DeepSeek   │
            └───────┬───────┘
                    │
                    ▼
            ┌───────────────┐
            │ 负载均衡       │
            │ 轮询可用 key   │
            └───────┬───────┘
                    │
                    ▼
            ┌───────────────┐
            │ 故障转移       │
            │ 失败 ──► 下一个 │
            └───────┬───────┘
                    │
                    ▼
            调用具体提供商
```

### 5.5 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `infra.Cache` | 响应缓存 | 否 |
| `infra.Config` | 读取模型配置 | 是 |
| `infra.Logger` | 记录调用日志 | 是 |

### 5.6 详细设计决策

#### 5.6.1 提供商注册方式

**原设计**: 17+ 提供商全部代码内置
**问题**: 维护成本高、二进制膨胀、更新滞后

**改进: OpenAI 兼容 + 配置化注册**

```go
type ProviderConfig struct {
    Name     string            `yaml:"name"`
    BaseURL  string            `yaml:"base_url"`
    APIKey   string            `yaml:"api_key"`
    Models   map[string]string `yaml:"models"` // 内部名 -> 提供商名
}

type Gateway struct {
    providers map[string]*ProviderClient // 动态加载
}

func (g *Gateway) LoadProviders(configPath string) error {
    configs, err := loadProviderConfigs(configPath)
    if err != nil {
        return err
    }
    
    for _, cfg := range configs {
        client := NewOpenAICompatibleClient(cfg.BaseURL, cfg.APIKey)
        g.providers[cfg.Name] = client
    }
    return nil
}
```

**好处**: 新增提供商只需改配置、代码量减少 70%、社区可贡献配置
**不足**: 非 OpenAI 兼容的提供商仍需代码适配

#### 5.6.2 故障转移策略

**原设计**: 硬编码降级链
**问题**: 不够灵活、无法根据实时可用性调整

**改进: 动态健康检查 + 用户配置**

```go
type HealthChecker struct {
    providers map[string]*ProviderHealth
}

type ProviderHealth struct {
    Name        string
    LastChecked time.Time
    Latency     time.Duration
    SuccessRate float64
    IsHealthy   bool
}

func (h *HealthChecker) SelectBest(candidates []string) string {
    var best string
    var bestScore float64
    
    for _, name := range candidates {
        health := h.providers[name]
        if !health.IsHealthy {
            continue
        }
        
        score := health.SuccessRate * (1.0 / (1.0 + float64(health.Latency.Milliseconds())))
        if score > bestScore {
            bestScore = score
            best = name
        }
    }
    return best
}
```

**好处**: 实时感知提供商状态、自动选择最优、用户可自定义降级链

#### 5.6.3 Token 预算管理

**原设计**: 简单日/月限制
**问题**: 缺少实时控制、无法按项目/用户分配

**改进: 多级预算 + 实时控制**

```go
type BudgetHierarchy struct {
    Global  *Budget
    User    *Budget
    Project *Budget
    Session *Budget
}

type Budget struct {
    Limit     int64
    Used      int64
    AlertAt   float64
    HardLimit bool
}

func (b *BudgetManager) CheckAndReserve(usage TokenUsage, scope BudgetScope) error {
    for _, budget := range b.hierarchy.GetChain(scope) {
        if budget.Used+usage.Total > budget.Limit {
            if budget.HardLimit {
                return fmt.Errorf("%s budget exceeded", budget.Name)
            }
            b.alert(budget, usage)
        }
        budget.Used += usage.Total
    }
    return nil
}
```

**好处**: 全局到会话级精细控制、软硬限制结合、实时告警

### 5.7 不做什么

- ❌ 不管理对话上下文（由 memory 管理）
- ❌ 不决定何时调用（由 Reactor 触发）
- ❌ 不执行业务逻辑（只负责模型调用）

---

## 6. 编排层 (orchestration/)

### 6.1 职责定义

**一句话**: 可选增强层，处理复杂任务的多步骤规划和多 Agent 协作。

**详细职责**:
- 任务规划：将复杂目标拆解为可执行步骤
- 多 Agent 协作：创建团队、分配任务、聚合结果
- 工作流执行：预定义流程的自动化执行
- 任务调度：管理步骤依赖和执行顺序

### 6.2 触发条件

| 场景 | 是否启用编排 | 示例 |
|------|-------------|------|
| 简单对话 | ❌ 否 | "你好" |
| 单工具调用 | ❌ 否 | "查天气" |
| 多步骤任务 | ✅ 是 | "搭建博客系统" |
| 多 Agent 协作 | ✅ 是 | "前端后端分别实现" |
| 固定流程 | ✅ 是 | "每日数据报表" |

### 6.3 文件清单

```
orchestration/
├── interfaces.go          # 编排接口定义
├── planner.go             # 任务规划器
├── team.go                # 多 Agent 团队
├── workflow.go            # 工作流引擎
├── scheduler.go           # 调度器
├── project.go             # 项目管理
├── confirmation.go        # 确认流程
├── review.go              # 计划评审
└── replan.go              # 重新规划
```

### 6.4 核心接口

```go
// Planner 任务规划器
type Planner interface {
    Decompose(ctx context.Context, goal string, agent *kernel.Agent) ([]Step, error)
}

// Team 多 Agent 协作
type Team interface {
    Execute(ctx context.Context, task *Task) (*TeamResult, error)
    AddAgent(agent *kernel.Agent, role string) error
    RemoveAgent(agentID string) error
}

// Workflow 工作流引擎
type Workflow interface {
    Execute(ctx context.Context, workflowID string, input map[string]interface{}) (*WorkflowResult, error)
    Register(template *WorkflowTemplate) error
}
```

### 6.5 编排流程

```
用户: "帮我搭建一个博客系统"
        │
        ▼
┌─────────────────┐
│ 1. Planner      │
│    拆解为步骤   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. Team         │
│    创建 Agent   │
│    分配角色     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. Scheduler    │
│    按依赖执行   │
│    Step 1 ──► Step 2 ──► ...
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 每个 Step    │
│    调用 kernel.Agent.Execute()
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 结果聚合     │
│    返回给用户   │
└─────────────────┘
```

### 6.6 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `kernel.Agent` | 执行每个步骤 | 是 |
| `llm.Gateway` | 规划时调用 LLM | 是 |
| `infra.DB` | 持久化计划 | 否 |

### 6.7 详细设计决策

#### 6.7.1 编排触发机制

**原设计**: 基于关键词匹配
**问题**: 误判率高、无法根据历史行为学习

**改进: 多维度评估 + 学习优化**

```go
type ComplexityEstimator struct {
    llm llm.Gateway
}

type Factor struct {
    Name   string
    Score  float64
    Weight float64
}

func (e *ComplexityEstimator) Estimate(ctx context.Context, req *ExecuteRequest) (TaskComplexity, error) {
    factors := []Factor{
        {Name: "message_length", Score: float64(len(req.Message)) / 100.0, Weight: 0.2},
        {Name: "tool_count", Score: float64(len(req.Tools)) * 2.0, Weight: 0.3},
        {Name: "has_steps", Score: boolScore(containsSteps(req.Message)), Weight: 0.2},
        {Name: "historical", Score: e.getHistoricalComplexity(req.SessionID), Weight: 0.3},
    }
    
    total := 0.0
    for _, f := range factors {
        total += f.Score * f.Weight
    }
    
    switch {
    case total < 2.0:
        return ComplexitySimple, nil
    case total < 5.0:
        return ComplexitySingleTool, nil
    default:
        return ComplexityMultiStep, nil
    }
}

func (e *ComplexityEstimator) getHistoricalComplexity(sessionID string) float64 {
    // 查询该用户过去 10 次任务的实际复杂度
    // 如果用户经常做复杂任务，降低触发阈值
}
```

**好处**: 多维度综合评估、学习用户习惯、减少误判
**不足**: 需要积累历史数据、初始阶段可能不准确

#### 6.7.2 规划生成策略

**原设计**: 每次调用 LLM 生成规划
**问题**: 成本高、延迟大、相似任务重复生成

**改进: 规划模板 + 缓存复用**

```go
type PlanCache struct {
    cache map[string]*CachedPlan
}

type CachedPlan struct {
    Template []Step
    HitCount int
    LastUsed time.Time
}

func (p *Planner) Decompose(ctx context.Context, goal string) ([]Step, error) {
    summary := p.summarizeGoal(goal)
    
    if cached := p.planCache.Get(summary); cached != nil {
        return p.adaptTemplate(cached.Template, goal)
    }
    
    steps, err := p.generatePlan(ctx, goal)
    if err != nil {
        return nil, err
    }
    
    p.planCache.Set(summary, steps)
    return steps, nil
}

func (p *Planner) summarizeGoal(goal string) string {
    keywords := extractKeywords(goal)
    sort.Strings(keywords)
    return strings.Join(keywords, "_")
}
```

**好处**: 相似任务复用规划、减少 LLM 调用、响应更快
**不足**: 需要维护缓存、模板适配可能不够精准

#### 6.7.3 多 Agent 协调机制

**原设计**: 简单并行执行，无协调
**问题**: 可能重复工作、结果冲突、无共享上下文

**改进: 共享黑板 + 协调器**

```go
type Blackboard struct {
    mu      sync.RWMutex
    entries map[string]*BlackboardEntry
}

type BlackboardEntry struct {
    Key       string
    Value     interface{}
    AgentID   string
    Timestamp time.Time
}

type Coordinator struct {
    blackboard *Blackboard
    agents     map[string]*Agent
}

func (c *Coordinator) Execute(ctx context.Context, task *Task) (*TeamResult, error) {
    c.blackboard.Set("goal", task.Goal, "coordinator")
    c.blackboard.Set("context", task.Context, "coordinator")
    
    var wg sync.WaitGroup
    for id, agent := range c.agents {
        wg.Add(1)
        go func(agentID string, a *Agent) {
            defer wg.Done()
            
            for step := range a.ExecuteSteps(ctx, task) {
                c.blackboard.Set(step.OutputKey, step.Result, agentID)
                
                if conflict := c.detectConflict(step); conflict != nil {
                    c.resolveConflict(conflict)
                }
            }
        }(id, agent)
    }
    wg.Wait()
    
    return c.aggregateResults(), nil
}

func (c *Coordinator) detectConflict(step Step) *Conflict {
    for key, entry := range c.blackboard.entries {
        if key == step.OutputKey && entry.AgentID != step.AgentID {
            return &Conflict{
                Key:    key,
                ValueA: entry.Value,
                ValueB: step.Result,
                AgentA: entry.AgentID,
                AgentB: step.AgentID,
            }
        }
    }
    return nil
}
```

**好处**: Agent 间共享上下文、自动冲突检测、结果可仲裁
**不足**: 实现复杂、协调开销、死锁风险

### 6.8 业界参考

| 项目 | LLM 层特点 | 编排层特点 |
|------|-----------|-----------|
| **LangChain** | 链式调用，Provider 抽象 | Agent 循环，Tool 绑定 |
| **AutoGen** | 多 Agent 对话 | 群聊模式，人工介入 |
| **CrewAI** | 角色定义 | 任务委托，流程控制 |
| **Dify** | 可视化配置 Provider | 工作流画布，条件分支 |
| **FastGPT** | 知识库 RAG | 可视化工作流 |

### 6.9 不做什么

- ❌ 不替代内核执行（只负责调度）
- ❌ 不处理简单任务（直接走内核）
- ❌ 不管理用户会话

---

## 7. API 层 (api/)

### 7.1 职责定义

**一句话**: 轻薄的外壳，负责协议转换和会话管理。

**详细职责**:
- HTTP 服务：Gin 服务器启动与管理
- 路由注册：RESTful API 端点
- 中间件：认证、CORS、限流、请求ID
- 处理器：解析请求、调用内核、格式化响应
- WebSocket：实时双向通信
- 会话管理：创建、销毁、关联会话

### 7.2 文件清单

```
api/
├── server.go              # HTTP 服务器
├── router.go              # 路由注册
├── websocket.go           # WebSocket 处理
├── middleware/            # 中间件
│   ├── auth.go
│   ├── cors.go
│   ├── rate_limit.go
│   └── request_id.go
└── handlers/              # 处理器
    ├── chat.go            # 对话
    ├── agent.go           # Agent 管理
    ├── session.go         # 会话管理
    ├── task.go            # 任务管理
    └── system.go          # 系统接口
```

### 7.3 核心接口

```go
// Server HTTP 服务器
type Server interface {
    Start(addr string) error
    Shutdown(ctx context.Context) error
    RegisterRoutes(r *gin.Engine)
}

// Handler HTTP 处理器
type Handler interface {
    RegisterRoutes(api *gin.RouterGroup)
}
```

### 7.4 请求处理流程

```
用户 POST /api/v3/chat
        │
        ▼
┌─────────────────┐
│ 1. 解析请求      │
│    JSON ──► Struct
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 认证授权      │
│    JWT 校验     │
│    权限检查     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. 获取会话      │
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
│    SSE / JSON   │
└─────────────────┘
```

### 7.5 API 端点

```
POST   /api/v3/chat              # 对话
POST   /api/v3/chat/stream       # 流式对话
GET    /api/v3/chat/:id          # 获取对话
DELETE /api/v3/chat/:id          # 删除对话

POST   /api/v3/agents            # 创建 Agent
GET    /api/v3/agents            # 列出 Agent
GET    /api/v3/agents/:id        # 获取 Agent
PUT    /api/v3/agents/:id        # 更新 Agent
DELETE /api/v3/agents/:id        # 删除 Agent
POST   /api/v3/agents/:id/run    # 执行 Agent

POST   /api/v3/tasks             # 创建任务
GET    /api/v3/tasks/:id         # 获取任务
DELETE /api/v3/tasks/:id         # 取消任务

POST   /api/v3/sessions          # 创建会话
GET    /api/v3/sessions/:id      # 获取会话
DELETE /api/v3/sessions/:id      # 删除会话

WS     /api/v3/ws                # WebSocket

GET    /health                   # 健康检查
GET    /metrics                  # 指标
```

### 7.6 本地 Agent 安全设计

OpenAIDE 是**本地运行**的 AI Agent，安全设计遵循**最小够用**原则，避免企业级系统的过度设计。

#### 7.6.1 认证简化：系统用户识别

```go
type LocalIdentity struct {
    UserID    string    `json:"user_id"`     // 系统用户名哈希
    HomeDir   string    `json:"home_dir"`    // ~/.openaide/
    CreatedAt time.Time `json:"created_at"`
}

func IdentifyUser() (*LocalIdentity, error) {
    user, err := user.Current()
    if err != nil {
        return nil, err
    }
    return &LocalIdentity{
        UserID:  hash(user.Username + user.Uid),
        HomeDir: filepath.Join(user.HomeDir, ".openaide"),
    }, nil
}
```

**删除**：JWT、OAuth、mTLS、API Key（本地单用户无需复杂认证）

#### 7.6.2 授权简化：危险操作确认

```go
type PermissionLevel int

const (
    PermSafe PermissionLevel = iota   // 安全操作：直接执行
    PermWarn                           // 警告操作：记录日志
    PermDangerous                      // 危险操作：需要确认
)

var DangerousOperations = []string{
    "rm -rf", "format", "DROP TABLE", "sudo",
}
```

**删除**：RBAC、ABAC（本地单用户无需角色权限系统）

#### 7.6.3 限流简化：并发控制

```go
type LocalLimiter struct {
    maxConcurrent int
    semaphore     chan struct{}
}

func NewLocalLimiter(max int) *LocalLimiter {
    return &LocalLimiter{
        maxConcurrent: max,
        semaphore:     make(chan struct{}, max),
    }
}
```

**删除**：四层令牌桶限流（本地资源独享，无需限流）

#### 7.6.4 审计简化：关键操作日志

```go
type LocalAuditLog struct {
    File *os.File
}

func (l *LocalAuditLog) Record(operation string, detail string) {
    if !isImportant(operation) {
        return
    }
    timestamp := time.Now().Format("2006-01-02 15:04:05")
    logLine := fmt.Sprintf("[%s] %s: %s\n", timestamp, operation, detail)
    l.File.WriteString(logLine)
}
```

**删除**：全量审计记录、保留策略（本地只需记录关键操作）

#### 7.6.5 传输安全

**删除**：HTTPS/TLS（本地 localhost 无需加密传输）

**保留**：文件权限控制

```go
func SecureFile(path string, data []byte) error {
    err := os.WriteFile(path, data, 0600) // 仅所有者可读写
    if err != nil {
        return err
    }
    return os.Chmod(path, 0600)
}
```

#### 7.6.6 安全中间件（本地版）

```go
func SetupLocalMiddleware(router *gin.Engine) {
    router.Use(middleware.RequestID())              // 调试追踪
    router.Use(middleware.RequestValidation())      // 输入验证
    router.Use(middleware.InputSanitization())      // 输入消毒
    router.Use(middleware.DangerousOperationConfirm()) // 危险操作确认
    router.Use(middleware.SimpleAuditLog())         // 关键操作审计
}
```

#### 7.6.7 安全设计对比

| 模块 | 企业级设计 | 本地 Agent 简化 | 减少复杂度 |
|------|-----------|----------------|-----------|
| 认证 | JWT + OAuth + mTLS | 系统用户识别 | 90% |
| 授权 | RBAC + ABAC | 危险操作确认 | 85% |
| 限流 | 四层令牌桶 | 信号量并发控制 | 80% |
| 审计 | 全量记录 + 保留策略 | 关键操作本地日志 | 75% |
| 传输 | HTTPS + TLS 1.3 | 无需（localhost） | 100% |
| 存储加密 | AES-256-GCM | 文件权限 0600 | 80% |

#### 7.6.8 安全底线（不可省略）

1. **危险操作确认**（`rm -rf` 等必须确认）
2. **输入长度限制**（防内存溢出）
3. **文件权限控制**（`0600` 保护配置）
4. **关键操作审计**（记录危险操作）
5. **价值对齐检查**（已讨论，保留）

### 7.7 依赖模块

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `kernel.Agent` | 执行用户请求 | 是 |
| `orchestration.*` | 处理复杂任务 | 否 |
| `infra.DB` | 会话持久化 | 是 |
| `infra.Logger` | 请求日志 | 是 |

### 7.8 不做什么

- ❌ 不决定用什么模型
- ❌ 不拆解任务
- ❌ 不选择工具
- ❌ 不管理记忆（只传 SessionID）
- ❌ 不执行业务逻辑
- ❌ 不做企业级认证授权（本地单用户）
- ❌ 不做 DDoS 防护（本地无此风险）
- ❌ 不做传输加密（localhost 通信）

---

## 8. 基础设施层 (infra/)

### 8.1 职责定义

**一句话**: 提供底层基础设施支持，包括存储、缓存、配置、日志。

**详细职责**:
- 数据库：SQLite 连接与 GORM 管理
- 缓存：内存/Redis 缓存操作
- 向量存储：HNSW 向量索引
- 配置：读取和管理应用配置（极简设计）
- 日志：结构化日志记录
- 事件总线：内存异步事件分发
- 限流：请求频率控制

### 8.2 文件清单

```
infra/
├── config.go              # 配置管理
├── database.go            # 数据库连接
├── cache.go               # 缓存
├── vector.go              # 向量存储
├── event_bus.go           # 内存事件总线（轻量分发）
├── event_store.go         # 事件持久化存储
├── event_sub.go           # 事件订阅管理
├── event_filter.go        # 事件过滤引擎
├── event_compress.go      # 事件压缩器
├── logger.go              # 日志
└── rate_limit.go          # 限流
```

### 8.3 核心接口

```go
// Database 数据库接口
type Database interface {
    DB() *gorm.DB
    Migrate(models ...interface{}) error
    Close() error
}

// Cache 缓存接口
type Cache interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{}, ttl time.Duration)
    Delete(key string)
}

// VectorStore 向量存储接口
type VectorStore interface {
    Add(id string, embedding []float32, metadata map[string]interface{}) error
    Search(embedding []float32, limit int) ([]SearchResult, error)
    Delete(id string) error
}

// EventBus 内存事件总线接口（轻量分发）
type EventBus interface {
    Publish(topic string, data interface{})
    Subscribe(topic string, handler EventHandler)
}

// EventStore 事件持久化存储接口
type EventStore interface {
    Append(ctx context.Context, events []Event) error
    Read(ctx context.Context, streamID string, fromVersion int) ([]Event, error)
    Replay(ctx context.Context, streamID string, handler func(Event) error) error
    Snapshot(ctx context.Context, streamID string, version int, state interface{}) error
}

// EventSubscriber 事件订阅管理接口
type EventSubscriber interface {
    Subscribe(filter EventFilter, handler EventHandler) SubscriptionID
    Unsubscribe(id SubscriptionID)
    Dispatch(ctx context.Context, event Event) error
}

// EventFilter 事件过滤接口
type EventFilter interface {
    Match(event Event) bool
}

// EventCompressor 事件压缩接口
type EventCompressor interface {
    Compress(events []Event) ([]byte, error)
    Decompress(data []byte) ([]Event, error)
    SelectStrategy(eventType string) CompressionStrategy
}

// Logger 日志接口
type Logger interface {
    Info(msg string, args ...interface{})
    Error(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Debug(msg string, args ...interface{})
}
```

### 8.4 存储策略

| 数据类型 | 存储 | 技术 | 理由 |
|----------|------|------|------|
| 对话消息 | **Markdown 文件** | `*.md` + `index.json` | **人类可读，零依赖** |
| 用户配置 | **YAML 文件** | `gopkg.in/yaml.v3` | **简单直观，一眼看懂** |
| 缓存 | KV | **内存** | **零外部依赖，高性能** |
| 向量 | ANN | HNSW | 近似最近邻 |
| 日志 | 文件 | slog | 持久化、轮转 |
| 实时事件 | 内存 | Channel | 轻量、异步 |
| 持久化事件 | 结构化 | SQLite + 文件 | 可回放、审计 |

### 8.5 配置管理（极简设计）

OpenAIDE 配置遵循**极简原则**：8 个核心项，一眼看懂。

#### 8.5.1 配置文件位置

| 层级 | 路径 | 说明 |
|------|------|------|
| **全局配置** | `~/.openaide/config.yaml` | 用户级默认配置 |
| **项目配置** | `.openaide/config.yaml` | 项目级覆盖配置（可选） |

#### 8.5.2 配置结构

```go
type Config struct {
    Model        string   `yaml:"model"`          // 模型名称
    APIKey       string   `yaml:"api_key"`        // API 密钥
    Language     string   `yaml:"language"`       // 语言：zh/en
    Theme        string   `yaml:"theme"`          // 主题：dark/light
    ShowThinking bool     `yaml:"show_thinking"`  // 显示思考过程
    AutoConfirm  bool     `yaml:"auto_confirm"`   // 自动确认安全操作
    Identity     string   `yaml:"identity"`       // 项目身份
    Tools        []string `yaml:"tools"`          // 启用工具列表
}
```

#### 8.5.3 全局配置示例 (`~/.openaide/config.yaml`)

```yaml
model: deepseek-chat        # 默认模型
api_key: ${DEEPSEEK_KEY}    # API 密钥（环境变量）
language: zh                # 语言：zh/en
theme: dark                 # 主题：dark/light
show_thinking: false        # 显示思考过程
auto_confirm: false         # 自动确认安全操作
```

#### 8.5.4 项目配置示例 (`.openaide/config.yaml`)

```yaml
identity: "Go后端专家"      # 项目身份
tools: [bash, file, go]     # 启用工具
```

#### 8.5.5 配置加载逻辑

```go
func GetConfig() *Config {
    // 1. 加载全局配置
    global, _ := Load("~/.openaide/config.yaml")
    
    // 2. 加载项目配置（如果存在）
    project, _ := Load(".openaide/config.yaml")
    
    // 3. 合并：项目配置覆盖全局配置
    if project != nil {
        if project.Model != "" {
            global.Model = project.Model
        }
        if project.Identity != "" {
            global.Identity = project.Identity
        }
        if len(project.Tools) > 0 {
            global.Tools = project.Tools
        }
    }
    
    return global
}
```

#### 8.5.6 配置对比

| 原设计 | 极简版 |
|--------|--------|
| 6 个嵌套结构体 | 1 个扁平结构体 |
| 三层配置（系统/用户/项目） | 两层配置（全局/项目） |
| 20+ 配置项 | 8 个核心配置项 |
| 复杂校验 | 简单类型检查 |
| 命令行工具 | 直接编辑文件 |

#### 8.5.7 配置底线

1. **YAML 格式**（人类可读）
2. **环境变量支持**（`$API_KEY`）
3. **两层合并**（全局 + 项目覆盖）
4. **8 个核心项**（一眼看懂）

### 8.5 事件系统模块

事件系统是基础设施层的核心子系统，负责 Agent 全生命周期的事件管理。

#### 8.5.1 事件类型层级

| 类别 | 事件类型 | 说明 | 持久化 |
|------|----------|------|--------|
| **System** | `system.start`, `system.stop`, `system.error` | 系统生命周期 | 是 |
| **ReAct** | `react.think`, `react.act`, `react.observe` | ReAct 循环事件 | 是 |
| **Enhancement** | `enhancement.reflect`, `enhancement.learn` | 增强能力事件 | 是 |
| **State** | `state.change`, `state.snapshot` | 状态变更 | 是 |
| **Meta** | `meta.heartbeat`, `meta.metric` | 元数据 | 否 |

#### 8.5.2 事件持久化 (event_store.go)

**职责**: 将事件持久化到存储，支持回放和快照

| 方法 | 说明 |
|------|------|
| `Append` | 批量追加事件 |
| `Read` | 按版本号读取事件 |
| `Replay` | 回放事件流 |
| `Snapshot` | 创建状态快照 |

**存储架构**:

```
┌─────────────────────────────────────────┐
│           事件持久化架构                 │
│                                         │
│  写入路径:                              │
│  事件 ──► 内存缓冲区 ──► 批量写入       │
│              │           SQLite/文件     │
│              ▼                          │
│         定时刷盘(1s)                    │
│                                         │
│  读取路径:                              │
│  回放请求 ──► 读取 JSON 文件 ──► 按序分发  │
│                                         │
│  快照路径:                              │
│  每1000事件 ──► 创建快照 ──► 单独存储   │
└─────────────────────────────────────────┘
```

#### 8.5.3 事件订阅 (event_sub.go)

**职责**: 管理多消费者订阅，支持按需分发

| 特性 | 说明 |
|------|------|
| 多消费者 | 同一事件可分发到多个消费者 |
| 独立消费 | 每个消费者有自己的消费位置 |
| 背压控制 | 消费者慢时自动降速 |
| 容错 | 消费者失败不影响其他消费者 |

**消费者示例**:

```go
// 日志消费者
sub.Subscribe(FilterAll(), func(e Event) {
    logger.Info("event", "type", e.Type, "data", e.Data)
})

// 指标消费者
sub.Subscribe(FilterType("meta.metric"), func(e Event) {
    metrics.Record(e.Data)
})

// 审计消费者
sub.Subscribe(FilterCategory("react", "enhancement"), func(e Event) {
    audit.Log(e)
})
```

#### 8.5.4 事件过滤 (event_filter.go)

**职责**: 支持消费者按需接收事件

| 过滤器 | 说明 | 示例 |
|--------|------|------|
| `FilterAll` | 接收所有事件 | 日志、审计 |
| `FilterType` | 按类型过滤 | 只接收 metric |
| `FilterCategory` | 按类别过滤 | 只接收 react 事件 |
| `FilterSource` | 按来源过滤 | 只接收特定 Agent |
| `FilterComposite` | 组合过滤 | type=react AND source=agent-1 |

**使用示例**:

```go
// 组合过滤
filter := NewFilterBuilder().
    Category("react").
    Type("react.think").
    Source("agent-1").
    Build()

sub.Subscribe(filter, handler)
```

#### 8.5.5 事件压缩 (event_compress.go)

**职责**: 减少事件传输和存储开销

| 策略 | 适用场景 | 压缩率 |
|------|----------|--------|
| `None` | 小事件、低延迟 | 0% |
| `Gzip` | 大文本事件 | 60-80% |
| `Zstd` | 高吞吐场景 | 50-70% |
| `Delta` | 状态变更事件 | 80-90% |
| `Smart` | 自动选择最优策略 | 自适应 |

**智能选择逻辑**:

```go
func SelectStrategy(event Event) CompressionStrategy {
    size := len(event.Data)
    switch {
    case size < 1KB:
        return None
    case event.Type == "state.change":
        return Delta
    case size > 100KB:
        return Zstd
    default:
        return Gzip
    }
}
```

#### 8.5.6 事件系统依赖

| 依赖 | 用途 | 是否必须 |
|------|------|----------|
| `infra.Database` | 事件持久化 | 是 |
| `infra.Cache` | 订阅缓存 | 否 |
| `infra.Logger` | 事件日志 | 是 |

### 8.6 依赖模块

无。基础设施层是最底层，不依赖任何其他模块。

### 8.7 可观测性（本地简化版）

OpenAIDE 作为本地 Agent，可观测性设计遵循**够用即可**原则，避免企业级监控系统的过度设计。

#### 8.7.1 监控指标（本地内存指标）

```go
type LocalMetrics struct {
    ChatCount       int64         // 总对话次数
    ChatDuration    time.Duration // 平均对话耗时
    TokenConsumed   int64         // 总 Token 消耗
    ToolCallCount   int64         // 工具调用次数
    ToolFailCount   int64         // 工具失败次数
    LLMLatency      time.Duration // LLM 平均延迟
    MemoryUsage     int64         // 内存占用（MB）
    ErrorCount      int64         // 错误次数
    LastError       string        // 最近错误
}

func (m *LocalMetrics) Snapshot() string {
    return fmt.Sprintf(`
📊 OpenAIDE 运行指标
─────────────────────
对话次数:    %d
Token 消耗:  %d
工具调用:    %d (失败 %d)
LLM 延迟:    %v
内存占用:    %d MB
错误次数:    %d
─────────────────────
`, m.ChatCount, m.TokenConsumed, m.ToolCallCount, m.ToolFailCount,
   m.LLMLatency, m.MemoryUsage, m.ErrorCount)
}
```

**删除**：Prometheus + Grafana（本地无需外部监控系统）

#### 8.7.2 日志设计（分级日志）

```go
type Logger struct {
    Level  LogLevel
    Output io.Writer
}

type LogLevel int

const (
    LevelDebug LogLevel = iota
    LevelInfo
    LevelWarn
    LevelError
)

func (l *Logger) log(level string, format string, args ...interface{}) {
    timestamp := time.Now().Format("2006-01-02 15:04:05")
    msg := fmt.Sprintf(format, args...)
    fmt.Fprintf(l.Output, "[%s] %s: %s\n", timestamp, level, msg)
}
```

**日志文件结构**：
```
~/.openaide/logs/
├── openaide.log      # 主日志
├── error.log         # 错误日志（单独记录）
└── chat/             # 对话日志（按日期）
    ├── 2026-05-14.log
    └── 2026-05-15.log
```

**删除**：ELK Stack、结构化日志 JSON 格式（本地纯文本即可）

#### 8.7.3 链路追踪（简化 TraceSpan）

```go
type TraceSpan struct {
    Name      string
    StartTime time.Time
    EndTime   time.Time
    Children  []*TraceSpan
}

func (s *TraceSpan) String() string {
    var sb strings.Builder
    s.print(&sb, 0)
    return sb.String()
}

func (s *TraceSpan) print(sb *strings.Builder, depth int) {
    indent := strings.Repeat("  ", depth)
    sb.WriteString(fmt.Sprintf("%s%s: %v\n", indent, s.Name, s.Duration()))
    for _, child := range s.Children {
        child.print(sb, depth+1)
    }
}
```

**输出示例**：
```
ChatRequest: 2.5s
  Think: 500ms
  ToolCall(weather): 1.2s
    HTTPRequest: 800ms
  GenerateResponse: 800ms
```

**删除**：OpenTelemetry + Jaeger（本地无需分布式追踪）

#### 8.7.4 健康检查

```go
type HealthCheck struct {
    Name     string
    Check    func() error
    Critical bool
}

var DefaultChecks = []HealthCheck{
    {Name: "llm_connection", Check: checkLLMConnection, Critical: true},
    {Name: "memory_storage", Check: checkMemoryStorage, Critical: true},
    {Name: "tool_registry", Check: checkToolRegistry, Critical: false},
}
```

**删除**：K8s Probes（本地无容器编排）

#### 8.7.5 调试模式

```go
type DebugMode struct {
    Enabled       bool
    ShowThinking  bool   // 显示思考过程
    ShowToolCalls bool   // 显示工具调用详情
    ShowTokens    bool   // 显示 Token 消耗
    ShowLatency   bool   // 显示延迟
    ShowMemory    bool   // 显示记忆访问
}

func (d *DebugMode) PrintEvent(event Event) {
    if !d.Enabled {
        return
    }
    switch event.Type() {
    case EventTypeThinking:
        if d.ShowThinking {
            fmt.Printf("🤔 思考: %s\n", event.Content())
        }
    case EventTypeToolCall:
        if d.ShowToolCalls {
            fmt.Printf("🔧 工具: %s(%v)\n", event.ToolName(), event.Arguments())
        }
    case EventTypeTokenUsage:
        if d.ShowTokens {
            fmt.Printf("📊 Token: %d\n", event.TotalTokens())
        }
    }
}
```

#### 8.7.6 性能分析（标准库 pprof）

```go
type Profiler struct {
    enabled bool
    file    *os.File
}

func (p *Profiler) StartCPUProfile(path string) error {
    if !p.enabled {
        return nil
    }
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    p.file = f
    return pprof.StartCPUProfile(f)
}

func (p *Profiler) StopCPUProfile() {
    if !p.enabled {
        return
    }
    pprof.StopCPUProfile()
    p.file.Close()
}
```

#### 8.7.7 可观测性汇总

```go
type Observability struct {
    Metrics  *LocalMetrics
    Logger   *Logger
    Tracer   *TraceSpan
    Health   *HealthChecker
    Debug    *DebugMode
    Profiler *Profiler
}

func (o *Observability) StatusReport() string {
    return fmt.Sprintf(`
%s
%s
健康状态: %s
调试模式: %v
`, o.Metrics.Snapshot(), o.Tracer.String(), o.Health.CheckAll().Status, o.Debug.Enabled)
}
```

#### 8.7.8 可观测性设计对比

| 模块 | 企业级设计 | 本地 Agent 简化 | 减少复杂度 |
|------|-----------|----------------|-----------|
| 监控 | Prometheus + Grafana | 内存指标结构体 | 90% |
| 日志 | ELK Stack | 分级日志 + 文件 | 85% |
| 链路追踪 | OpenTelemetry + Jaeger | 简单 TraceSpan | 90% |
| 健康检查 | K8s Probes | 简单函数检查 | 80% |
| 性能分析 | APM 工具 | pprof 标准库 | 70% |

#### 8.7.9 可观测性底线

1. **错误日志**（必须记录，方便排查）
2. **关键指标**（Token 消耗、延迟）
3. **健康检查**（LLM 连接、存储可用）
4. **调试模式**（开发时查看内部状态）

### 8.8 不做什么

- ❌ 不定义业务模型（由 models/ 定义）
- ❌ 不决定缓存策略（由调用方决定）
- ❌ 不处理业务逻辑
- ❌ 不定义事件语义（由 kernel 定义事件类型）
- ❌ 不做企业级监控（Prometheus/Grafana）
- ❌ 不做分布式追踪（OpenTelemetry）
- ❌ 不做容器健康检查（K8s Probes）

---

## 9. 事件系统 (infra/event*)

事件系统是基础设施层的核心子系统，为整个 Agent 架构提供事件驱动能力。详细设计见 [ARCHITECTURE.md 第7章](ARCHITECTURE.md#7-事件系统设计)。

### 9.1 职责定位

**一句话**: 统一的事件管理平台，负责事件的持久化、订阅、过滤和压缩。

**详细职责**:
- 事件持久化：将事件写入存储，支持回放和快照
- 事件订阅：管理多消费者，支持独立消费位置
- 事件过滤：支持消费者按需接收事件
- 事件压缩：减少传输和存储开销

### 9.2 模块归属

事件系统属于 **基础设施层 (infra/)**，但为所有上层模块提供服务：

```
┌─────────────────────────────────────────┐
│           事件系统服务范围               │
│                                         │
│  kernel ──► 生成事件 (react/think/act)  │
│     │                                   │
│     ▼                                   │
│  infra.EventStore ──► 持久化            │
│  infra.EventSubscriber ──► 分发         │
│  infra.EventFilter ──► 过滤             │
│  infra.EventCompressor ──► 压缩         │
│     │                                   │
│     ▼                                   │
│  api ──► 消费事件 (SSE/WebSocket)       │
│  memory ──► 消费事件 (学习/模式)        │
│  orchestration ──► 消费事件 (调度)      │
└─────────────────────────────────────────┘
```

### 9.3 关键设计要点

| 设计点 | 说明 |
|--------|------|
| 持久化 | SQLite + 文件混合存储，支持批量写入和快照 |
| 订阅 | 多消费者独立消费，背压控制，容错 |
| 过滤 | 类型/类别/来源/组合过滤，Builder 模式 |
| 压缩 | 智能策略选择（None/Gzip/Zstd/Delta） |

---

## 10. 终端会话隔离设计

### 10.1 设计目标

OpenAIDE 终端工具需要支持：
- **项目隔离**: 不同文件夹 = 不同项目
- **会话连续**: 再次进入同一文件夹，自动继续上次对话
- **用户隔离**: 同一台电脑不同系统用户自动隔离
- **零配置**: 用户无感知，自然使用

### 10.2 核心设计: 文件夹即项目

```bash
# 用户无感知，自然使用
cd ~/projects/blog
openaide chat          # 自动识别为 blog 项目

cd ~/projects/company-backend
openaide chat          # 自动识别为 company-backend 项目
```

### 10.3 存储结构

```
~/.openaide/                    # 用户级全局配置（按系统用户隔离）
├── config.yaml                 # 用户偏好（模型、语言、编码风格）
└── projects/                   # 项目记忆
    ├── blog_abc123/            # 项目唯一ID（基于路径哈希）
    │   ├── memory.db           # 项目级记忆（代码结构、讨论历史）
    │   └── sessions/
    │       └── latest.json     # 自动继续上次对话
    └── company-backend_def456/
        ├── memory.db
        └── sessions/
```

### 10.4 实现逻辑

```go
type ProjectContext struct {
    RootDir     string // 项目根目录（执行 openaide 的目录）
    ProjectID   string // 项目唯一标识
    MemoryPath  string // ~/.openaide/projects/<id>/memory.db
    SessionPath string // ~/.openaide/projects/<id>/sessions/
}

func (c *CLI) GetProjectContext() (*ProjectContext, error) {
    // 1. 获取当前工作目录
    cwd, err := os.Getwd()
    if err != nil {
        return nil, err
    }
    
    // 2. 生成项目唯一ID（基于绝对路径哈希）
    absPath, _ := filepath.Abs(cwd)
    projectID := fmt.Sprintf("%s_%s", 
        filepath.Base(absPath),
        hashString(absPath)[:6])
    
    // 3. 确定用户级存储根目录
    userHome, _ := os.UserHomeDir()
    baseDir := filepath.Join(userHome, ".openaide", "projects")
    
    return &ProjectContext{
        RootDir:     cwd,
        ProjectID:   projectID,
        MemoryPath:  filepath.Join(baseDir, projectID, "memory.db"),
        SessionPath: filepath.Join(baseDir, projectID, "sessions"),
    }, nil
}
```

### 10.5 自动继续会话

```go
func (c *CLI) ResumeOrCreateSession(project *ProjectContext) (*Session, error) {
    latestPath := filepath.Join(project.SessionPath, "latest.json")
    
    if _, err := os.Stat(latestPath); os.IsNotExist(err) {
        // 首次进入，创建新会话
        return c.createSession(project)
    }
    
    // 自动继续上次对话（无需询问）
    return c.loadSession(latestPath)
}
```

**交互示例**:

```bash
# 第一次进入
$ cd ~/projects/blog
$ openaide chat
> 创建新项目 "blog"，开始新对话

# 第二次进入（自动继续）
$ cd ~/projects/blog
$ openaide chat
> 继续上次对话（数据库设计）...

# 切换项目
$ cd ~/projects/company-backend
$ openaide chat
> 切换到项目 "company-backend"，加载项目上下文
```

### 10.6 多用户隔离

```go
func (c *CLI) GetUserID() string {
    // 优先使用系统用户名
    if user := os.Getenv("USER"); user != "" {
        return user
    }
    if user := os.Getenv("USERNAME"); user != "" {
        return user
    }
    // 回退到机器名
    hostname, _ := os.Hostname()
    return hostname
}
```

**隔离效果**:

| 场景 | 行为 |
|------|------|
| 同一用户，不同文件夹 | 不同项目，独立记忆 |
| 同一用户，同一文件夹 | 继续上次对话 |
| 不同用户，同一文件夹 | 完全隔离，各自独立 |

### 10.7 项目配置

```yaml
# ~/.openaide/projects/blog_abc123/config.yaml
project:
  name: "个人博客"
  description: "基于 Go + React 的博客系统"
  tech_stack: ["go", "react", "postgresql"]
  
agent:
  model: "deepseek-chat"
  temperature: 0.7
  
session:
  auto_resume: true      # 自动继续上次对话
  max_history: 100       # 保留最近 100 条消息
```

### 10.8 与后端架构的对应关系

| 终端概念 | 后端对应 |
|----------|----------|
| 用户（系统用户） | User |
| 项目（文件夹） | Project |
| 会话（latest.json） | Session |
| 项目记忆（memory.db） | Project Memory（L3 长期记忆） |
| 用户偏好（config.yaml） | User Preference（用户级共享） |

### 10.9 关键设计决策

| 决策 | 方案 | 理由 |
|------|------|------|
| 项目识别 | 文件夹路径哈希 | 自然、零配置、符合开发习惯 |
| 用户隔离 | 系统用户名 | 无需登录、自动隔离 |
| 会话继续 | 自动继续（不询问） | 减少交互、体验流畅 |
| 存储位置 | `~/.openaide/` | 标准 XDG 规范、易于清理 |
| 项目配置 | 本地 YAML 文件 | 可版本控制、团队协作 |

---

## 11. 技能系统 (skill/)

### 11.1 职责定位

**一句话**: 面向用户的技能封装，支持语义触发、多轮交互、技能组合和自我进化。

**详细职责**:
- 技能定义与管理（CRUD）
- 智能触发（语义匹配 + 上下文感知）
- 多轮交互（状态机驱动）
- 技能组合（管道编排）
- 自我进化（失败分析 + 自动优化）
- 工具联动（自动注入所需工具）

### 11.2 现有实现分析

当前代码位于 `backend/src/models/skill.go` 和 `backend/src/services/skill_service.go`。

**现有功能**:
- 基础 CRUD
- 关键词触发匹配
- LLM 执行
- 参数提取
- 5 个内置技能（translation, code_review, summarization, data_analysis, daily_report）

### 11.3 改进设计

#### 11.3.1 触发机制改进

**原设计**: 仅关键词匹配
**问题**: "翻译" 会误匹配 "不要翻译"，无上下文感知

**改进: 语义匹配 + 上下文感知**

```go
type SkillMatcher struct {
    embeddingSvc EmbeddingService
    llmSvc       LLMService
}

func (m *SkillMatcher) Match(ctx context.Context, content string, context *ConversationContext) (*SkillMatchResult, error) {
    // 1. 快速关键词预筛选
    candidates := m.keywordFilter(content)
    
    // 2. 语义相似度排序
    contentEmbedding := m.embeddingSvc.Embed(content)
    for _, skill := range candidates {
        for _, trigger := range skill.Triggers {
            triggerEmbedding := m.embeddingSvc.Embed(trigger)
            similarity := cosineSimilarity(contentEmbedding, triggerEmbedding)
            if similarity > 0.85 {
                return &SkillMatchResult{Skill: skill, Confidence: similarity}, nil
            }
        }
    }
    
    // 3. LLM 语义理解（高精度但慢）
    if len(candidates) > 0 {
        return m.llmMatch(ctx, content, candidates)
    }
    
    return nil, nil
}
```

**好处**: 减少误匹配、支持语义相似表达、上下文感知

#### 11.3.2 技能组合（管道编排）

**原设计**: 单技能独立执行
**问题**: "审查代码并翻译" 只能执行一个技能

**改进: 技能管道**

```go
type SkillPipeline struct {
    steps []PipelineStep
}

type PipelineStep struct {
    Skill     *Skill
    InputFrom string // "user" | "step:N"
    OutputTo  string // "user" | "context:key"
}

func (s *SkillService) BuildPipeline(matchedSkills []*SkillMatchResult) (*SkillPipeline, error) {
    pipeline := &SkillPipeline{}
    for i, match := range matchedSkills {
        step := PipelineStep{
            Skill:     match.Skill,
            InputFrom: "user",
            OutputTo:  "user",
        }
        if i > 0 {
            step.InputFrom = fmt.Sprintf("step:%d", i-1)
        }
        pipeline.steps = append(pipeline.steps, step)
    }
    return pipeline, nil
}

func (s *SkillService) ExecutePipeline(ctx context.Context, pipeline *SkillPipeline, content string) (string, error) {
    var lastOutput = content
    for _, step := range pipeline.steps {
        result, err := s.ExecuteSkillWithContent(ctx, step.Skill, lastOutput, "")
        if err != nil {
            return "", err
        }
        lastOutput = s.extractOutput(result)
    }
    return lastOutput, nil
}
```

**好处**: 支持复杂任务、技能复用、灵活组合

#### 11.3.3 多轮交互（状态机）

**原设计**: 一次性执行
**问题**: `daily_report` 无法追问用户今天做了什么

**改进: 技能状态机**

```go
type SkillState string

const (
    SkillStateIdle       SkillState = "idle"
    SkillStateCollecting SkillState = "collecting"
    SkillStateExecuting  SkillState = "executing"
    SkillStateCompleted  SkillState = "completed"
)

type SkillSession struct {
    SkillID       string
    State         SkillState
    CollectedData map[string]interface{}
    MissingParams []string
}

func (s *SkillService) ExecuteWithState(ctx context.Context, session *SkillSession, userInput string) (*SkillResponse, error) {
    switch session.State {
    case SkillStateIdle:
        missing := s.getMissingParams(session.SkillID)
        if len(missing) > 0 {
            session.State = SkillStateCollecting
            session.MissingParams = missing
            return &SkillResponse{Type: "question", Content: s.buildQuestion(missing[0])}, nil
        }
        
    case SkillStateCollecting:
        session.CollectedData[session.MissingParams[0]] = userInput
        session.MissingParams = session.MissingParams[1:]
        if len(session.MissingParams) > 0 {
            return &SkillResponse{Type: "question", Content: s.buildQuestion(session.MissingParams[0])}, nil
        }
        session.State = SkillStateExecuting
        return s.executeWithData(ctx, session)
    }
    return nil, fmt.Errorf("invalid state: %s", session.State)
}
```

**好处**: 支持参数收集、自然对话流、用户体验好

#### 11.3.4 技能自我进化

**原设计**: 仅记录成功率
**问题**: 无法自动优化技能表现

**改进: 失败分析 + 自动优化**

```go
type SkillEvolution struct {
    db  *gorm.DB
    llm LLMService
}

func (e *SkillEvolution) AnalyzeExecutions(skillID string) (*EvolutionSuggestion, error) {
    executions, err := e.getRecentExecutions(skillID, 100)
    if err != nil {
        return nil, err
    }
    
    failures := filterFailed(executions)
    failurePatterns := e.extractPatterns(failures)
    feedbacks := e.getUserFeedbacks(skillID)
    commonComplaints := e.extractComplaints(feedbacks)
    
    prompt := fmt.Sprintf(`分析技能 "%s" 的改进点：
失败模式: %v
用户反馈: %v
请给出提示词优化、参数调整、触发词扩展建议。`, skillID, failurePatterns, commonComplaints)
    
    resp, err := e.llm.Chat(prompt)
    // 解析建议
}

func (e *SkillEvolution) ApplyEvolution(skillID string, suggestion *EvolutionSuggestion) error {
    skill, err := e.getSkill(skillID)
    if err != nil {
        return err
    }
    skill.SystemPromptOverride = suggestion.ImprovedPrompt
    skill.Triggers = append(skill.Triggers, suggestion.NewTriggers...)
    skill.LastEvolvedAt = time.Now()
    skill.AutoEvolved = true
    return e.db.Save(skill).Error
}
```

**好处**: 自动优化、持续改进、减少人工维护

#### 11.3.5 工具联动

**原设计**: 技能与工具系统割裂
**问题**: 技能声明需要工具，但无绑定机制

**改进: 工具自动注入**

```go
type Skill struct {
    RequiredTools  []string
    ToolBindings   []ToolBinding
}

type ToolBinding struct {
    ToolName   string
    SkillParam string
    AutoExecute bool
}

func (s *SkillService) PrepareTools(skill *Skill) ([]Tool, error) {
    var tools []Tool
    for _, toolName := range skill.RequiredTools {
        tool, err := s.toolRegistry.Get(toolName)
        if err != nil {
            return nil, fmt.Errorf("required tool '%s' not available", toolName)
        }
        tools = append(tools, tool)
    }
    return tools, nil
}

func (s *SkillService) ExecuteWithTools(ctx context.Context, skill *Skill, content string) (string, error) {
    tools, err := s.PrepareTools(skill)
    if err != nil {
        return "", err
    }
    systemPrompt := skill.SystemPromptOverride + "\n\n可用工具:\n"
    for _, tool := range tools {
        systemPrompt += fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description)
    }
    return s.llmSvc.ChatWithTools(ctx, systemPrompt, content, tools)
}
```

**好处**: 技能自动获取所需工具、减少配置、执行更智能

### 11.4 技能系统总结

| 改进点 | 原设计问题 | 改进方案 |
|--------|-----------|----------|
| 触发机制 | 仅关键词匹配 | 语义匹配 + 上下文感知 |
| 技能组合 | 单技能独立 | 管道编排，多技能串联 |
| 多轮交互 | 一次性执行 | 状态机，支持参数收集 |
| 自我进化 | 仅成功率统计 | 失败分析 + 自动优化 |
| 工具联动 | 技能与工具割裂 | 工具绑定 + 自动注入 |

---

## 12. 模块间依赖关系

### 12.1 依赖矩阵

| 模块 ↓ / 依赖 → | kernel | memory | tools | llm | orchestration | api | infra |
|----------------|--------|--------|-------|-----|---------------|-----|-------|
| **kernel** | - | ✅ | ✅ | ✅ | ❌ | ❌ | ✅ |
| **memory** | ❌ | - | ❌ | ✅ | ❌ | ❌ | ✅ |
| **tools** | ❌ | ❌ | - | ❌ | ❌ | ❌ | ✅ |
| **llm** | ❌ | ❌ | ❌ | - | ❌ | ❌ | ✅ |
| **orchestration** | ✅ | ❌ | ❌ | ✅ | - | ❌ | ✅ |
| **api** | ✅ | ❌ | ❌ | ❌ | ✅ | - | ✅ |
| **infra** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | - |

**图例**:
- ✅ 直接依赖
- ❌ 不依赖

### 12.2 依赖方向图

```
                    ┌─────────────┐
                    │    API      │
                    └──────┬──────┘
                           │
           ┌───────────────┼───────────────┐
           │               │               │
           ▼               ▼               ▼
    ┌──────────┐    ┌──────────┐    ┌──────────┐
    │ 编排层    │    │ 内核层    │    │ (其他)   │
    │(可选)     │    │(核心)     │    │          │
    └────┬─────┘    └────┬─────┘    └──────────┘
         │               │
         │    ┌──────────┼──────────┐
         │    │          │          │
         │    ▼          ▼          ▼
         │ ┌──────┐  ┌──────┐  ┌──────┐
         └►│memory│  │tools │  │ llm  │
           └──┬───┘  └──────┘  └──┬───┘
              │                   │
              └─────────┬─────────┘
                        │
              ┌─────────▼─────────┐
              │     infra         │
              │  DB │ Cache │ ... │
              └───────────────────┘
```

**关键约束**:
1. 依赖方向始终向下（上层依赖下层）
2. 内核不依赖编排层（编排可选）
3. 基础设施层不依赖任何其他模块
4. 同层模块间尽量避免依赖（如 memory 不依赖 tools）
5. 增强能力子模块间可相互依赖（如 reflection 依赖 learning）

---

## 12. 身份系统 (identity/)

### 12.1 职责定位

**一句话**: 项目级身份管理，支持 LLM 驱动的交互式身份确定。

**详细职责**:
- 项目与身份绑定
- LLM 驱动的交互式身份确定
- 身份配置管理（CRUD）
- 身份切换与继承

### 12.2 核心设计

#### 12.2.1 身份与项目绑定

```go
type ProjectIdentity struct {
    ProjectID   string
    IdentityID  string
    Config      IdentityConfig
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

#### 12.2.2 LLM 驱动的交互式身份确定

```go
type LLMIdentityConfigurator struct {
    llm LLMService
    db  *gorm.DB
}

type IdentityConfigSession struct {
    ProjectID      string
    ProjectFiles   []string
    ProjectContent string
    Messages       []Message
    CurrentIdentity *Identity
    Step           ConfigStep
}
```

**流程**:
1. **分析项目**: LLM 分析项目文件，识别技术栈/领域
2. **生成猜测**: LLM 生成初始身份猜测（项目类型、用户角色、置信度）
3. **交互确认**: LLM 与用户对话，提问澄清（2-3轮）
4. **生成配置**: LLM 综合对话生成完整身份配置
5. **保存激活**: 保存到项目配置，立即生效

**示例交互**:
```
🤖 我分析了这个项目的内容：
📁 项目类型：Python数据分析项目
👤 猜测身份：量化研究员/股票分析师
🎯 置信度：75%

为了更准确地为你服务，我想确认几个问题：
1. 你更关注实盘交易还是策略研究？
2. 你主要交易A股、港股还是美股？
3. 你偏好技术分析还是基本面分析？

> 1. 实盘交易 2. A股 3. 技术分析

✅ 身份已确定：「A股技术派交易员」📈
```

#### 12.2.3 身份配置内容

```go
type Identity struct {
    ID          string
    Name        string
    Description string
    Icon        string
    Prompt      string        // 系统提示词
    Tools       []string      // 可用工具
    Skills      []string      // 关联技能
    Preferences map[string]string // 偏好设置
}
```

### 12.3 关键设计决策

| 决策 | 方案 | 理由 |
|------|------|------|
| 绑定粒度 | 项目级 | 同一用户不同项目不同身份 |
| 确定方式 | LLM 驱动交互 | 动态适应，灵活确认 |
| 配置时机 | 新建项目时 | 首次使用即确定 |
| 自定义 | 支持完全自定义 | 满足特殊需求 |
| 复用 | 保存为模板 | 方便其他项目使用 |

---

## 13. 知识库系统 (knowledge/)

### 13.1 职责定位

**一句话**: 统一管理项目知识，支持自动积累、智能检索和动态应用。

**详细职责**:
- 知识自动提取与积累
- 知识存储与索引
- 知识检索与匹配
- 知识应用与注入

### 13.2 知识积累

#### 13.2.1 自动提取来源

| 来源 | 提取方式 | 示例 |
|------|----------|------|
| **项目文件** | 代码分析、文档解析 | 技术栈、架构设计、API 定义 |
| **对话历史** | LLM 提取关键信息 | 用户偏好、决策记录、解决方案 |
| **工具执行** | 结果结构化存储 | 命令输出、查询结果、错误日志 |
| **用户反馈** | 显式标记 + 隐式学习 | 点赞内容、常用操作、纠错记录 |
| **外部导入** | 文件上传、URL 抓取 | 技术文档、规范标准、参考资料 |

#### 13.2.2 提取流程

```go
type KnowledgeExtractor struct {
    llm LLMService
    db  *gorm.DB
}

// 从对话中提取知识
func (e *KnowledgeExtractor) ExtractFromDialogue(ctx context.Context, dialogueID string) ([]Knowledge, error) {
    // 1. 获取对话内容
    messages := e.getDialogueMessages(dialogueID)
    
    // 2. 使用 LLM 提取关键信息
    prompt := fmt.Sprintf(`从以下对话中提取有价值的知识：

%s

提取规则：
1. 技术决策（为什么选这个方案）
2. 解决方案（问题如何解决）
3. 用户偏好（喜欢的风格、工具）
4. 项目信息（技术栈、架构）
5. 常见错误（踩过的坑）

输出 JSON 数组：
[
  {"type": "decision", "content": "...", "confidence": 0.9},
  {"type": "solution", "content": "...", "confidence": 0.8}
]`, formatMessages(messages))
    
    resp, err := e.llm.Chat(ctx, &ChatRequest{Messages: []Message{{Role: "user", Content: prompt}}})
    // 解析并保存
}

// 从代码中提取知识
func (e *KnowledgeExtractor) ExtractFromCode(ctx context.Context, filePath string, content string) ([]Knowledge, error) {
    prompt := fmt.Sprintf(`分析以下代码文件，提取项目知识：

文件: %s

%s

提取：
1. 技术栈（语言、框架、库）
2. 架构模式（MVC、微服务、分层）
3. 关键配置（数据库、缓存、消息队列）
4. API 定义（接口、参数、返回值）
5. 业务逻辑（核心功能、规则）

输出 JSON`, filePath, content)
    
    resp, err := e.llm.Chat(ctx, &ChatRequest{Messages: []Message{{Role: "user", Content: prompt}}})
    // 解析并保存
}
```

#### 13.2.3 知识类型

```go
type KnowledgeType string

const (
    KnowledgeTypeTechStack    KnowledgeType = "tech_stack"     // 技术栈
    KnowledgeTypeArchitecture KnowledgeType = "architecture"   // 架构设计
    KnowledgeTypeDecision     KnowledgeType = "decision"       // 技术决策
    KnowledgeTypeSolution     KnowledgeType = "solution"       // 解决方案
    KnowledgeTypePreference   KnowledgeType = "preference"     // 用户偏好
    KnowledgeTypeAPI          KnowledgeType = "api"            // API 定义
    KnowledgeTypeError        KnowledgeType = "error"          // 常见错误
    KnowledgeTypePattern      KnowledgeType = "pattern"        // 代码模式
    KnowledgeTypeConfig       KnowledgeType = "config"         // 配置信息
    KnowledgeTypeBusiness     KnowledgeType = "business"       // 业务逻辑
)

type Knowledge struct {
    ID          string
    ProjectID   string
    Type        KnowledgeType
    Content     string
    Source      string        // 来源（文件/对话/工具）
    Confidence  float64       // 置信度
    Embedding   []float32     // 向量表示
    CreatedAt   time.Time
    UpdatedAt   time.Time
    AccessCount int           // 访问次数
}
```

### 13.3 知识应用

#### 13.3.1 检索方式

```go
type KnowledgeRetriever struct {
    vectorStore VectorStore
    db          *gorm.DB
}

// 混合检索
func (r *KnowledgeRetriever) Retrieve(ctx context.Context, query string, projectID string, limit int) ([]Knowledge, error) {
    // 1. 向量检索（语义相似）
    queryEmbedding := r.embedQuery(query)
    vectorResults := r.vectorStore.Search(queryEmbedding, limit*2)
    
    // 2. 关键词检索（精确匹配）
    keywordResults := r.keywordSearch(query, projectID, limit*2)
    
    // 3. RRF 融合排序
    combined := r.reciprocalRankFusion(vectorResults, keywordResults)
    
    // 4. 过滤项目
    var filtered []Knowledge
    for _, k := range combined {
        if k.ProjectID == projectID || k.ProjectID == "" { // "" 表示通用知识
            filtered = append(filtered, k)
        }
    }
    
    // 5. 返回 Top N
    if len(filtered) > limit {
        filtered = filtered[:limit]
    }
    return filtered, nil
}
```

#### 13.3.2 注入 Prompt

```go
func (s *PromptService) InjectKnowledge(ctx context.Context, prompt string, query string, projectID string) (string, error) {
    // 1. 检索相关知识
    knowledge := s.knowledgeRetriever.Retrieve(ctx, query, projectID, 5)
    
    if len(knowledge) == 0 {
        return prompt, nil
    }
    
    // 2. 格式化知识
    var kbParts []string
    kbParts = append(kbParts, "## 项目相关知识（请在回复时参考）")
    
    for i, k := range knowledge {
        kbParts = append(kbParts, fmt.Sprintf("### %d. %s\n%s", i+1, k.Type, k.Content))
    }
    
    // 3. 注入到 Prompt
    knowledgeBlock := strings.Join(kbParts, "\n\n")
    return prompt + "\n\n" + knowledgeBlock, nil
}
```

#### 13.3.3 应用场景

| 场景 | 知识应用 | 效果 |
|------|----------|------|
| 代码生成 | 注入项目技术栈和架构规范 | 生成的代码符合项目风格 |
| 调试辅助 | 注入常见错误和解决方案 | 快速定位问题 |
| 代码审查 | 注入项目编码规范 | 审查标准统一 |
| 架构设计 | 注入现有架构信息 | 设计兼容现有系统 |
| 问答 | 注入项目背景知识 | 回答更准确 |

### 13.4 知识积累策略

#### 13.4.1 自动积累触发条件

```go
type AccumulationTrigger struct {
    AfterDialogue   bool    // 对话结束后提取
    AfterToolCall   bool    // 工具调用后提取
    AfterFileChange bool    // 文件变更后提取
    OnSchedule      string  // 定时提取（cron）
    MinConfidence   float64 // 最低置信度
}

func (e *KnowledgeExtractor) ShouldExtract(event Event) bool {
    switch event.Type {
    case "dialogue.completed":
        return e.trigger.AfterDialogue
    case "tool.executed":
        return e.trigger.AfterToolCall
    case "file.changed":
        return e.trigger.AfterFileChange
    }
    return false
}
```

#### 13.4.2 积累流程

```
事件触发（对话结束/工具执行/文件变更）
    │
    ▼
提取知识（LLM 分析内容）
    │
    ▼
质量评估（置信度检查）
    │
    ├── 置信度 < 阈值 ──► 丢弃
    │
    └── 置信度 >= 阈值
            │
            ▼
    去重检查（相似度比较）
            │
            ├── 已存在相似知识 ──► 合并/更新
            │
            └── 新知识
                    │
                    ▼
            生成向量嵌入
                    │
                    ▼
            保存到知识库
                    │
                    ▼
            更新索引
```

### 13.5 知识库优化设计

#### 13.5.1 Token 消耗优化

**问题 1: 每次都用 LLM 提取知识**

**优化: 本地规则提取 + LLM 兜底**

```go
type LocalExtractor struct {
    rules []ExtractionRule
}

type ExtractionRule struct {
    Pattern       *regexp.Regexp
    KnowledgeType KnowledgeType
    Transform     func(matches []string) string
}

// 预定义规则（零 Token 成本）
var defaultRules = []ExtractionRule{
    {
        Pattern: regexp.MustCompile(`(?i)我用?\s+(\w+)(?:\s*\+\s*(\w+))?`),
        KnowledgeType: KnowledgeTypeTechStack,
        Transform: func(m []string) string {
            return fmt.Sprintf("技术栈: %s", strings.Join(m[1:], ", "))
        },
    },
    {
        Pattern: regexp.MustCompile(`(?i)我喜欢?用?\s+(.+)`),
        KnowledgeType: KnowledgeTypePreference,
        Transform: func(m []string) string {
            return fmt.Sprintf("偏好: %s", m[1])
        },
    },
}

func (e *KnowledgeExtractor) SmartExtract(ctx context.Context, dialogue []Message) ([]Knowledge, error) {
    var allKnowledge []Knowledge
    
    // 1. 本地规则提取（零 Token 成本）
    for _, msg := range dialogue {
        if msg.Role == "user" {
            local := e.localExtractor.Extract(msg.Content)
            allKnowledge = append(allKnowledge, local...)
        }
    }
    
    // 2. 如果本地提取不足，再用 LLM 补充
    if len(allKnowledge) < 2 {
        llmKnowledge, err := e.llmExtract(ctx, dialogue)
        if err != nil {
            return nil, err
        }
        allKnowledge = append(allKnowledge, llmKnowledge...)
    }
    
    return allKnowledge, nil
}
```

**效果**: 80% 的知识通过本地规则提取，Token 消耗降低 70%

**问题 2: 知识注入 Prompt 太长**

**优化: 知识摘要 + 按需注入**

```go
type KnowledgeCompressor struct{}

func (c *KnowledgeCompressor) Compress(knowledge []Knowledge, maxTokens int) string {
    if len(knowledge) == 0 {
        return ""
    }
    
    // 按类型分组并合并
    grouped := groupByType(knowledge)
    
    var parts []string
    parts = append(parts, "[项目上下文]")
    
    for typ, items := range grouped {
        if len(items) == 1 {
            parts = append(parts, fmt.Sprintf("%s: %s", typ, items[0].Content))
        } else {
            merged := c.mergeSameType(items)
            parts = append(parts, fmt.Sprintf("%s: %s", typ, merged))
        }
    }
    
    return strings.Join(parts, "; ")
}

// 按需注入：只注入与意图相关的知识
func (r *KnowledgeRetriever) RetrieveRelevant(ctx context.Context, query string, projectID string, limit int) ([]Knowledge, error) {
    intent := r.extractIntent(query)
    relevantTypes := r.mapIntentToTypes(intent)
    return r.searchByTypes(ctx, query, projectID, relevantTypes, limit)
}
```

**效果**: 知识注入从 500 Token 降到 100 Token，减少 80%

**问题 3: 重复检索相同知识**

**优化: 会话级缓存**

```go
type KnowledgeCache struct {
    mu        sync.RWMutex
    cache     map[string]*CachedKnowledge
    sessionKB map[string][]Knowledge
}

func (c *KnowledgeCache) GetOrRetrieve(sessionID, query string, retriever func() ([]Knowledge, error)) ([]Knowledge, error) {
    // 1. 检查会话级知识库
    c.mu.RLock()
    if sessionKB, ok := c.sessionKB[sessionID]; ok {
        localMatch := c.localMatch(sessionKB, query)
        if len(localMatch) > 0 {
            c.mu.RUnlock()
            return localMatch, nil
        }
    }
    c.mu.RUnlock()
    
    // 2. 检查全局缓存
    queryHash := hashQuery(query)
    c.mu.RLock()
    if cached, ok := c.cache[queryHash]; ok && time.Since(cached.Timestamp) < cached.TTL {
        c.mu.RUnlock()
        return cached.Knowledge, nil
    }
    c.mu.RUnlock()
    
    // 3. 执行检索并缓存
    knowledge, err := retriever()
    if err != nil {
        return nil, err
    }
    
    c.mu.Lock()
    c.cache[queryHash] = &CachedKnowledge{
        Knowledge: knowledge,
        Timestamp: time.Now(),
        TTL:       5 * time.Minute,
    }
    c.sessionKB[sessionID] = append(c.sessionKB[sessionID], knowledge...)
    c.mu.Unlock()
    
    return knowledge, nil
}
```

**效果**: 同一会话内重复查询减少 90% 的检索开销

#### 13.5.2 智能提升优化

**优化 1: 知识图谱**

```go
type KnowledgeGraph struct {
    nodes map[string]*KnowledgeNode
    edges map[string][]*KnowledgeEdge
}

type KnowledgeEdge struct {
    From     string
    To       string
    Relation string // "depends_on", "uses", "solved_by"
    Strength float64
}

// 检索时考虑关联知识
func (r *KnowledgeRetriever) RetrieveWithContext(ctx context.Context, query string, projectID string) ([]Knowledge, error) {
    direct := r.search(query, projectID)
    
    var related []Knowledge
    for _, k := range direct {
        neighbors := r.graph.GetNeighbors(k.ID, 2)
        for _, n := range neighbors {
            related = append(related, n.Knowledge)
        }
    }
    
    return r.deduplicate(append(direct, related...)), nil
}
```

**效果**: Agent 理解知识间的关联，回答更有上下文

**优化 2: 知识效果追踪**

```go
type KnowledgeFeedback struct {
    KnowledgeID string
    Query       string
    Used        bool
    Helpful     bool
    Timestamp   time.Time
}

func (s *KnowledgeService) RecordFeedback(knowledgeID string, helpful bool) {
    s.db.Model(&KnowledgeFeedback{}).
        Where("knowledge_id = ?", knowledgeID).
        Update("helpful", helpful)
    
    if helpful {
        s.db.Model(&Knowledge{}).
            Where("id = ?", knowledgeID).
            Update("effectiveness", gorm.Expr("effectiveness + 0.1"))
    }
}

// 定期清理低效知识
func (s *KnowledgeService) CleanupLowValueKnowledge() error {
    return s.db.Where("effectiveness < 0.3").
        Where("updated_at < ?", time.Now().AddDate(0, -3, 0)).
        Delete(&Knowledge{}).Error
}
```

**优化 3: 知识预测预加载**

```go
type KnowledgePredictor struct {
    llm LLMService
}

func (p *KnowledgePredictor) PredictNeeds(ctx context.Context, dialogue []Message, projectID string) ([]Knowledge, error) {
    recentTopics := p.extractTopics(dialogue)
    
    prompt := fmt.Sprintf(`基于对话主题 %v，预测用户接下来可能需要什么信息`, recentTopics)
    
    resp, err := p.llm.Chat(ctx, &ChatRequest{Messages: []Message{{Role: "user", Content: prompt}}})
    // 解析预测结果，预加载相关知识
}
```

**效果**: 主动预测用户需求，提前准备知识

### 13.6 知识库优化总结

| 优化点 | 原问题 | 优化方案 | Token 节省 | 智能提升 |
|--------|--------|----------|-----------|----------|
| **本地规则提取** | 每次都用 LLM | 规则匹配 + LLM 兜底 | 70% | ⭐⭐⭐ |
| **知识压缩注入** | 全文注入 | 摘要 + 按需注入 | 80% | ⭐⭐ |
| **会话级缓存** | 每次都检索 | 缓存复用 | 90% | ⭐ |
| **知识图谱** | 孤立条目 | 关联关系 | - | ⭐⭐⭐⭐ |
| **效果追踪** | 无反馈 | 使用追踪 + 清理 | 20% | ⭐⭐⭐ |
| **预测预加载** | 被动检索 | 主动预测 | - | ⭐⭐⭐⭐⭐ |

---

## 14. 上下文压缩系统 (compress/)

### 14.1 职责定位

**一句话**: 像管理小说书籍一样管理对话上下文，支持多层摘要、伏笔追踪、多视角压缩。

**详细职责**:
- 对话分层摘要（目录→章节→段落）
- 关键信息标记（伏笔追踪）
- 多角色视角压缩
- 智能闪回机制
- 长对话分卷管理
- 并行任务插叙

### 14.2 现有实现分析

当前代码位于 `backend/src/services/tool_calling_service.go` 和 `backend/src/services/memory_service.go`。

**现有三层压缩**:
1. **工具输出压缩**: 硬截断 150 字符
2. **LLM 摘要压缩**: 单层摘要，保留最近 10 条
3. **记忆截断**: 简单截断到 Token 限制

### 14.3 优化设计

#### 14.3.1 三层摘要结构（小说目录类比）

```go
type DialogueBook struct {
    Title       string    // 项目身份 + 当前任务（封面）
    TOC         []string  // 目录（章节列表）
    Chapters    []Chapter // 章节（分层消息）
    Epilogue    string    // 知识库注入（后记）
}

type Chapter struct {
    ID          int
    Title       string    // 一句话摘要（20字）
    Summary     string    // 简要摘要（100字）
    Detail      string    // 详细摘要（300字）
    Messages    []Message // 原始消息（仅近期保留）
    KeyInfos    []KeyInfo // 关键信息（伏笔）
    Importance  float64   // 重要性评分
}
```

**效果**: 像翻书一样快速定位历史对话，目录层只消耗 50 Token

#### 14.3.2 伏笔追踪系统

```go
type KeyInfo struct {
    Type       string // "decision", "error", "config", "todo"
    Content    string
    Chapter    int
    IsResolved bool
}

// 生成"未解决事项"提示
func (b *DialogueBook) BuildCliffhangerPrompt() string {
    var unresolved []KeyInfo
    for _, ch := range b.Chapters {
        for _, info := range ch.KeyInfos {
            if !info.IsResolved {
                unresolved = append(unresolved, info)
            }
        }
    }
    
    // 输出: "❌ 未解决错误: ...", "⏳ 待办事项: ..."
}
```

**效果**: 像小说追踪伏笔一样追踪未解决问题，Agent 不会遗忘

#### 14.3.3 多视角压缩

```go
type MultiPerspectiveBook struct {
    UserPerspective  Perspective // 关注需求和目标
    AgentPerspective Perspective // 关注行动和结果
    ToolPerspective  Perspective // 关注工具输出
}

// 从用户视角压缩
func (b *MultiPerspectiveBook) CompressFromUserView(messages []Message) string {
    return fmt.Sprintf(`用户视角:
- 需求: ...
- 目标: ...`)
}

// 从 Agent 视角压缩
func (b *MultiPerspectiveBook) CompressFromAgentView(messages []Message) string {
    return fmt.Sprintf(`Agent视角:
- 已执行: ...
- 结果: ...`)
}
```

**效果**: 不同角色有不同记忆，Agent 清楚自己的定位

#### 14.3.4 智能闪回机制

```go
type Flashback struct {
    Trigger    string  // 触发关键词
    Chapter    int     // 闪回到哪一章
    Content    string  // 闪回内容
    Importance float64
}

// 检测是否需要闪回
func (e *FlashbackEngine) DetectFlashbackNeed(query string) []Flashback {
    // 1. 查询包含之前的关键信息
    // 2. 相似错误复发
    // 3. 未解决事项被提及
}
```

**效果**: 像小说闪回一样提醒关键历史，错误复发时自动提醒

#### 14.3.5 长对话分卷

```go
type Volume struct {
    ID       int
    Title    string    // 卷标题
    Theme    string    // 主题
    Chapters []Chapter
    Summary  string    // 卷摘要
}

// 超过 50 章分一卷
func (b *DialogueBook) SplitVolumes() []Volume {
    // 分卷管理
}
```

**效果**: 超长对话分卷管理，像《哈利波特》一样清晰

#### 14.3.6 并行任务插叙

```go
type ParallelThread struct {
    ID       string
    Name     string
    ParentID string
    Messages []Message
    Summary  string
    Status   string // "active", "paused", "completed"
}

// 切换话题时创建插叙线程
func (b *DialogueBook) CreateThread(parentID, threadName string) *ParallelThread {
    // 保存原话题状态
}

// 切回时插入"插叙摘要"
func (b *DialogueBook) SwitchToMainThread(threadID string) string {
    return fmt.Sprintf(`【插叙结束：%s】
- 状态: %s
- 进展: %s`, thread.Name, thread.Status, thread.Summary)
}
```

**效果**: 用户切换话题时状态保存，切回时自动提醒

### 14.4 业界方案对比

| 方案 | 代表项目 | 优点 | 缺点 | 我们的优势 |
|------|----------|------|------|-----------|
| **简单截断** | 多数开源项目 | 实现简单 | 丢失上下文 | ✅ 语义保留 |
| **LLM 摘要** | LangChain | 语义完整 | Token 成本高 | ✅ 分层摘要，成本低 |
| **滑动窗口** | 早期 GPT 应用 | 简单高效 | 遗忘旧信息 | ✅ 伏笔追踪，不遗忘 |
| **向量检索** | RAG 系统 | 精准检索 | 无时间线 | ✅ 时间线 + 关联 |
| **层次化记忆** | MemGPT | 结构化 | 实现复杂 | ✅ 小说类比，直观 |

### 14.5 业界方案对比

| 方案 | 代表项目 | 优点 | 缺点 | 我们的优势 |
|------|----------|------|------|-----------|
| **简单截断** | 多数开源项目 | 实现简单 | 丢失上下文 | ✅ 语义保留 |
| **LLM 摘要** | LangChain | 语义完整 | Token 成本高 | ✅ 分层摘要，成本低 |
| **滑动窗口** | 早期 GPT 应用 | 简单高效 | 遗忘旧信息 | ✅ 伏笔追踪，不遗忘 |
| **向量检索** | RAG 系统 | 精准检索 | 无时间线 | ✅ 时间线 + 关联 |
| **层次化记忆** | MemGPT | 结构化 | 实现复杂 | ✅ 小说类比，直观 |

### 14.6 融合架构设计

#### 14.6.1 融合方案概述

将我们的**小说类比法**与业界最佳实践融合，形成四层混合架构：

```
┌─────────────────────────────────────────────────────────────┐
│                    融合上下文压缩架构                          │
│                                                              │
│  Layer 1: 小说结构层 (DialogueBook)                          │
│  ├── 目录 (TOC)        → 快速导航                             │
│  ├── 章节 (Chapter)    → 分层摘要                             │
│  ├── 伏笔 (KeyInfo)    → 未解决事项追踪                        │
│  └── 插叙 (Thread)     → 多任务并行                           │
│                                                              │
│  Layer 2: MemGPT 内存分离                                    │
│  ├── Main Context      → 当前对话窗口                         │
│  ├── External Memory   → 历史对话存储                         │
│  └── Page Fault        → 按需检索                             │
│                                                              │
│  Layer 3: RAG 检索增强                                       │
│  ├── 混合搜索 (向量 + 关键词)                                  │
│  ├── 重排序 (Rerank)   → 精准排序                             │
│  └── 父文档 (Parent Doc) → 保留完整上下文                       │
│                                                              │
│  Layer 4: 动态预算分配                                       │
│  ├── 系统提示  20%                                           │
│  ├── 知识注入  30%                                           │
│  ├── 近期对话  30%                                           │
│  └── 检索记忆  20%                                           │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

#### 14.6.2 融合实现策略

**策略 1: 小说结构 + MemGPT 内存分离**

```go
type FusionMemoryManager struct {
    // Layer 1: 小说结构（我们的创新）
    book *DialogueBook
    
    // Layer 2: MemGPT 风格内存分离
    mainContext    []Message     // 当前窗口（最近 10 轮）
    externalMemory *EventStore   // 历史事件存储
    
    // Layer 3: RAG 检索
    vectorStore    VectorStore   // 向量存储
    retriever      *RAGRetriever // 检索器
    
    // Layer 4: 预算管理
    budget         *TokenBudget  // Token 预算分配
}

func (m *FusionMemoryManager) BuildContext(ctx context.Context, sessionID string, query string) (*Context, error) {
    // 1. 获取 Main Context（MemGPT 风格）
    mainContext := m.getMainContext(sessionID)
    
    // 2. 检查是否需要检索（Page Fault）
    if m.needsRetrieval(query, mainContext) {
        // 3. RAG 检索（Claude 风格 Contextual Retrieval）
        retrieved := m.retrieve(ctx, query, sessionID)
        
        // 4. 小说结构补充（我们的创新）
        cliffhangers := m.book.BuildCliffhangerPrompt()
        flashbacks := m.book.DetectFlashbackNeed(query)
        
        // 5. 动态预算分配
        return m.budget.Allocate(mainContext, retrieved, cliffhangers, flashbacks)
    }
    
    // 不需要检索，直接返回 Main Context + 小说结构
    return m.budget.Allocate(mainContext, nil, m.book.BuildCliffhangerPrompt(), nil)
}
```

**策略 2: 上下文检索 + 父文档**

```go
type ContextualChunk struct {
    Content         string    // 原始内容
    Contextualized  string    // 带上下文的版本（Claude 风格）
    ParentDoc       string    // 父文档完整内容（LangChain 风格）
    Chapter         int       // 所属章节（小说结构）
    KeyInfo         []KeyInfo // 关联伏笔
}

func (r *RAGRetriever) RetrieveWithContext(ctx context.Context, query string) ([]ContextualChunk, error) {
    // 1. 向量检索 top-k
    candidates := r.vectorStore.Search(query, 20)
    
    // 2. 重排序
    reranked := r.reranker.Rerank(query, candidates, 10)
    
    // 3. 获取父文档（LangChain Parent Document）
    for i, chunk := range reranked {
        parent := r.getParentDocument(chunk.ID)
        reranked[i].ParentDoc = parent.Content
    }
    
    // 4. 添加上下文（Claude Contextual Retrieval）
    for i, chunk := range reranked {
        contextualized := r.addContext(chunk)
        reranked[i].Contextualized = contextualized
    }
    
    return reranked, nil
}
```

**策略 3: 动态预算分配**

```go
type TokenBudget struct {
    TotalBudget int // 例如 8000 tokens
}

func (b *TokenBudget) Allocate(main []Message, retrieved []ContextualChunk, cliffhangers string, flashbacks []Flashback) *Context {
    budget := &Context{}
    remaining := b.TotalBudget
    
    // 1. 系统提示 20%
    systemTokens := int(float64(b.TotalBudget) * 0.2)
    budget.SystemPrompt = b.buildSystemPrompt(systemTokens)
    remaining -= systemTokens
    
    // 2. 知识注入 30%（RAG 检索结果）
    knowledgeTokens := int(float64(b.TotalBudget) * 0.3)
    budget.Knowledge = b.compressRetrieved(retrieved, knowledgeTokens)
    remaining -= len(b.tokenizer.Encode(budget.Knowledge))
    
    // 3. 近期对话 30%（MemGPT Main Context）
    dialogueTokens := int(float64(b.TotalBudget) * 0.3)
    budget.RecentDialogue = b.truncateMessages(main, dialogueTokens)
    remaining -= len(b.tokenizer.Encode(budget.RecentDialogue))
    
    // 4. 检索记忆 20%（小说结构补充）
    memoryTokens := remaining
    budget.Memory = b.buildMemoryPrompt(cliffhangers, flashbacks, memoryTokens)
    
    return budget
}
```

#### 14.6.3 融合优势

| 层面 | 我们的方案 | 业界方案 | 融合优势 |
|------|-----------|----------|----------|
| **结构** | 小说类比（直观） | MemGPT 内存分离（高效） | 直观 + 高效 |
| **检索** | 时间线 + 关联 | RAG 向量检索（精准） | 精准 + 有时间线 |
| **上下文** | 多视角压缩 | Claude 上下文检索（完整） | 完整 + 多视角 |
| **存储** | 章节分层 | LangChain 父文档（不丢失） | 分层 + 不丢失 |
| **预算** | 固定分配 | 动态调整（灵活） | 灵活 + 可控 |

#### 14.6.4 融合实施路径

```
Phase 1: 基础层（Week 1-2）
├── 实现 DialogueBook 结构
├── 实现 MemGPT 风格 Main Context
└── 实现 TokenBudget 分配

Phase 2: 检索层（Week 3-4）
├── 集成向量检索
├── 实现重排序
└── 实现父文档获取

Phase 3: 融合层（Week 5-6）
├── 实现 Page Fault 检测
├── 实现上下文检索
└── 实现动态预算分配

Phase 4: 优化层（Week 7-8）
├── 实现小说结构补充（伏笔/闪回）
├── 实现多视角压缩
└── 性能调优
```

### 14.7 优化总结

| 优化点 | 小说类比 | 当前缺失 | 改进效果 |
|--------|----------|----------|----------|
| **三层摘要** | 目录→章节→段落 | 只有单层摘要 | 快速定位，节省 Token |
| **伏笔追踪** | 未解决线索标记 | 平等对待所有信息 | 不遗忘关键问题 |
| **多视角** | 不同角色不同记忆 | 一视同仁 | 角色定位清晰 |
| **闪回机制** | 关键时刻回顾 | 线性压缩 | 关联历史上下文 |
| **分卷结构** | 长篇分卷 | 单一线性 | 长对话管理 |
| **插叙机制** | 并行线索 | 单线程 | 多任务切换 |
| **MemGPT 分离** | 主内存 + 外存 | 无分离 | 高效内存管理 |
| **RAG 检索** | 按需查资料 | 无检索 | 精准回忆 |
| **父文档** | 完整档案 | 只有摘要 | 不丢失细节 |
| **动态预算** | 按需分配篇幅 | 固定分配 | 灵活高效 |

---

## 15. 增强能力联动设计

### 15.1 能力触发链

```
用户输入
    │
    ▼
┌─────────────────┐
│ 1. 多模态感知   │ ◄── Multimodal（可选）
│    (可选)       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 价值对齐检查 │ ◄── Alignment（输入检查）
│    (Alignment)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. ReAct 核心   │ ◄── Reactor（核心）
│    (Reactor)    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 自我反思     │ ◄── Reflection（质量评估）
│    (Reflection) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. 自动纠错     │ ◄── Correction（错误修正）
│    (Correction) │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 6. 价值对齐检查 │ ◄── Alignment（输出检查）
│    (Alignment)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 7. 学习进化     │ ◄── Learning（策略优化）
│    (Learning)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 8. 模式检测     │ ◄── Pattern（行为分析）
│    (Pattern)    │
└─────────────────┘
```

### 15.2 能力间依赖关系

| 能力 | 依赖 |
|------|------|
| Reflection | Reactor（获取执行结果） |
| Correction | Reflection（获取评估结果） |
| Learning | Reflection（获取反思结果）、Correction（获取纠错记录） |
| Pattern | Learning（获取用户偏好） |
| Alignment | Reactor（获取输出） |
| Multimodal | Reactor（作为输入源） |

### 15.3 可配置性

每个增强能力都是**可选启用**的：

```go
type AgentConfig struct {
    // 核心能力（必须）
    ReactorEnabled bool // true
    
    // 增强能力（可选）
    ReflectionEnabled  bool // 默认 true
    LearningEnabled    bool // 默认 true
    CorrectionEnabled  bool // 默认 true
    AlignmentEnabled   bool // 默认 true
    PatternEnabled     bool // 默认 false（数据积累慢）
    MultimodalEnabled  bool // 默认 false（资源消耗大）
}
```

---

## 16. 关键设计约束

### 16.1 接口隔离

```go
// ✅ 正确：内核只依赖接口，不依赖实现
type Reactor struct {
    memory  memory.System      // 接口
    tools   tools.Registry     // 接口
    llm     llm.Gateway        // 接口
    logger  infra.Logger       // 接口
}

// ❌ 错误：直接依赖具体实现
type Reactor struct {
    memory  *memory.GormStore   // 具体实现
    tools   *tools.ToolService  // 具体实现
    llm     *llm.OpenAIClient  // 具体实现
}
```

### 16.2 依赖注入

```go
// ✅ 正确：通过构造函数注入依赖
func NewReactor(
    memory memory.System,
    tools tools.Registry,
    llm llm.Gateway,
    logger infra.Logger,
) *Reactor {
    return &Reactor{memory, tools, llm, logger}
}

// ❌ 错误：全局变量或内部创建
var reactor = &Reactor{} // 全局变量

func NewReactor() *Reactor {
    return &Reactor{
        memory: memory.NewGormStore(), // 内部创建
    }
}
```

### 16.3 错误处理

```go
// ✅ 正确：包装错误，添加上下文
func (r *Reactor) React(ctx context.Context, req *ExecuteRequest) error {
    thought, err := r.Think(ctx, req)
    if err != nil {
        return fmt.Errorf("reactor think failed: %w", err)
    }
    // ...
}

// ❌ 错误：吞掉错误
func (r *Reactor) React(ctx context.Context, req *ExecuteRequest) error {
    thought, _ := r.Think(ctx, req) // 忽略错误
    // ...
}
```

### 16.4 并发安全

```go
// ✅ 正确：使用 sync.RWMutex 保护共享状态
type Registry struct {
    mu    sync.RWMutex
    tools map[string]Tool
}

func (r *Registry) Get(name string) (Tool, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    t, ok := r.tools[name]
    return t, ok
}

// ❌ 错误：无锁访问共享状态
type Registry struct {
    tools map[string]Tool // 无锁，并发不安全
}
```

### 16.5 测试策略

#### 16.5.1 测试金字塔

```
        ┌─────────┐
        │  E2E    │  5%  (端到端)
        │  测试   │
       ┌┴─────────┴┐
       │  集成测试  │  15% (模块间)
       │           │
      ┌┴───────────┴┐
      │   单元测试    │  80% (核心逻辑)
      │              │
      └──────────────┘
```

#### 16.5.2 单元测试规范

```go
// reactor_test.go
package kernel

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

func TestReactor_Execute(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        mockLLM  func(m *mockLLM)
        want     string
        wantErr  bool
        errCode  ErrorCode
    }{
        {
            name:  "简单对话",
            input: "你好",
            mockLLM: func(m *mockLLM) {
                m.On("Chat", mock.Anything).Return("你好！有什么可以帮助？", nil)
            },
            want: "你好！有什么可以帮助？",
        },
        {
            name:  "工具调用",
            input: "查北京天气",
            mockLLM: func(m *mockLLM) {
                m.On("Chat", mock.Anything).Return("", ErrToolCall)
            },
            wantErr: true,
            errCode: CodeToolFailed,
        },
        {
            name:  "LLM 超时",
            input: "复杂问题",
            mockLLM: func(m *mockLLM) {
                m.On("Chat", mock.Anything).Return("", context.DeadlineExceeded)
            },
            wantErr: true,
            errCode: CodeLLMTimeout,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := new(mockLLM)
            tt.mockLLM(m)
            r := NewReactor(WithLLM(m))
            
            got, err := r.Execute(context.Background(), tt.input)
            
            if tt.wantErr {
                assert.Error(t, err)
                var appErr *AppError
                assert.True(t, errors.As(err, &appErr))
                assert.Equal(t, tt.errCode, appErr.Code)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

#### 16.5.3 集成测试规范

```go
// integration_test.go
package integration

func TestEndToEnd_Chat(t *testing.T) {
    app := NewTestApp(t)
    defer app.Shutdown()
    
    resp, err := app.Chat("你好")
    
    assert.NoError(t, err)
    assert.Contains(t, resp, "你好")
    
    history := app.GetHistory()
    assert.Len(t, history, 2) // 用户 + AI
}
```

#### 16.5.4 测试覆盖率目标

| 模块 | 单元测试 | 集成测试 | 覆盖率目标 |
|------|----------|----------|-----------|
| kernel | ✅ | ✅ | 85% |
| memory | ✅ | ✅ | 80% |
| tools | ✅ | ⚠️ | 75% |
| llm | ✅ | ⚠️ | 70% |
| cli | ⚠️ | ✅ | 60% |
| api | ⚠️ | ✅ | 60% |
| infra | ✅ | ⚠️ | 70% |

#### 16.5.5 CI/CD 测试流程

```yaml
# .github/workflows/test.yml
name: Test

on: [push, pull_request]

jobs:
  unit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test ./... -race -coverprofile=coverage.out
      - run: go tool cover -func=coverage.out | grep total
      
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test ./... -tags=integration -run Integration
      
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go test ./e2e/... -tags=e2e
```

### 16.6 性能基准

#### 16.6.1 核心性能指标

| 指标 | 目标值 | 测试方法 | 优先级 |
|------|--------|----------|--------|
| **首 Token 延迟** | < 500ms | 从请求到第一个 Token | P0 |
| **完整响应时间** | < 3s | 简单对话端到端 | P0 |
| **工具调用延迟** | < 2s | 单次工具执行 | P0 |
| **记忆检索延迟** | < 100ms | 向量检索 | P1 |
| **并发请求数** | > 10 | 同时处理对话数 | P1 |
| **内存占用** | < 512MB | 单实例常驻内存 | P0 |
| **启动时间** | < 1s | 从命令到可交互 | P0 |
| **上下文压缩率** | > 80% | 压缩后 Token 节省 | P1 |

#### 16.6.2 基准测试代码

```go
// benchmark_test.go
package benchmark

import (
    "testing"
    "time"
)

func BenchmarkReactor_Execute(b *testing.B) {
    reactor := NewTestReactor()
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := reactor.Execute(ctx, "你好")
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkMemory_Recall(b *testing.B) {
    memory := NewTestMemory()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := memory.Recall("查询历史")
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkContextCompression(b *testing.B) {
    compressor := NewTestCompressor()
    messages := generateLargeContext(100) // 100 条消息
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := compressor.Compress(messages, 4000)
        if err != nil {
            b.Fatal(err)
        }
    }
}

// 端到端性能测试
func TestEndToEnd_Performance(t *testing.T) {
    app := NewTestApp(t)
    
    // 测试简单对话
    t.Run("简单对话", func(t *testing.T) {
        start := time.Now()
        _, err := app.Chat("你好")
        elapsed := time.Since(start)
        
        assert.NoError(t, err)
        assert.Less(t, elapsed, 3*time.Second, "简单对话应 < 3s")
    })
    
    // 测试工具调用
    t.Run("工具调用", func(t *testing.T) {
        start := time.Now()
        _, err := app.Chat("查北京天气")
        elapsed := time.Since(start)
        
        assert.NoError(t, err)
        assert.Less(t, elapsed, 5*time.Second, "工具调用应 < 5s")
    })
    
    // 测试长上下文
    t.Run("长上下文", func(t *testing.T) {
        for i := 0; i < 50; i++ {
            app.Chat(fmt.Sprintf("消息 %d", i))
        }
        
        start := time.Now()
        _, err := app.Chat("总结上文")
        elapsed := time.Since(start)
        
        assert.NoError(t, err)
        assert.Less(t, elapsed, 5*time.Second, "长上下文应 < 5s")
    })
}
```

#### 16.6.3 性能监控

```go
type PerformanceMonitor struct {
    metrics map[string]*Metric
}

type Metric struct {
    Name      string
    Count     int64
    TotalTime time.Duration
    MaxTime   time.Duration
    MinTime   time.Duration
}

func (pm *PerformanceMonitor) Record(name string, duration time.Duration) {
    m := pm.metrics[name]
    if m == nil {
        m = &Metric{Name: name, MinTime: duration}
        pm.metrics[name] = m
    }
    
    m.Count++
    m.TotalTime += duration
    if duration > m.MaxTime {
        m.MaxTime = duration
    }
    if duration < m.MinTime {
        m.MinTime = duration
    }
}

func (pm *PerformanceMonitor) Report() string {
    var sb strings.Builder
    sb.WriteString("📊 性能报告\n")
    sb.WriteString("─────────────────────\n")
    
    for _, m := range pm.metrics {
        avg := m.TotalTime / time.Duration(m.Count)
        sb.WriteString(fmt.Sprintf("%s: 平均 %v, 最大 %v, 最小 %v, 次数 %d\n",
            m.Name, avg, m.MaxTime, m.MinTime, m.Count))
    }
    
    return sb.String()
}
```

#### 16.6.4 性能优化检查清单

| 检查项 | 优化前 | 优化后 | 方法 |
|--------|--------|--------|------|
| LLM 连接池 | 每次新建 | 复用连接 | `http.Client` 复用 |
| 向量检索 | 全量扫描 | HNSW 索引 | 近似最近邻 |
| 记忆加载 | 全量加载 | 按需加载 | 分页 + 缓存 |
| 上下文压缩 | LLM 摘要 | 本地规则 | 减少 LLM 调用 |
| 工具结果 | 重复执行 | 缓存结果 | TTL 缓存 |

#### 16.6.5 性能目标

| 指标 | 目标 | 说明 |
|------|------|------|
| 首 token 延迟 | < 500ms | 从请求到第一个 token |
| 工具调用延迟 | < 2s | 单次工具调用 |
| 记忆检索延迟 | < 100ms | 向量检索 |
| 并发请求 | > 10 | 同时处理的对话数 |
| 内存占用 | < 512MB | 单实例 |

### 16.7 增强能力性能影响

| 能力 | 额外延迟 | 额外成本 | 优化策略 |
|------|----------|----------|----------|
| Reflection | +200-500ms | +1 LLM 调用 | 异步执行 |
| Learning | 无（后台） | 无 | 批量处理 |
| Correction | +100-300ms | +0.5 LLM 调用 | 高置信度时跳过 |
| Alignment | +50-100ms | +0.2 LLM 调用 | 缓存规则结果 |
| Pattern | 无（后台） | 无 | 定时任务 |
| Multimodal | +500ms-2s | 模型调用 | 可选启用 |

---

## 17. 插件系统 (plugin/)

### 17.1 插件类型

| 类型 | 说明 | 示例 |
|------|------|------|
| **工具插件** | 扩展工具能力 | 数据库查询、API 调用 |
| **LLM 插件** | 扩展模型支持 | 新提供商适配 |
| **记忆插件** | 扩展存储方式 | 外部数据库（可选配置） |
| **事件插件** | 扩展事件处理 | 日志收集、监控 |
| **UI 插件** | 扩展界面 | 自定义主题 |

### 17.2 插件接口

```go
// plugin.go
package plugin

// Plugin 插件接口
type Plugin interface {
    Name() string
    Version() string
    Init(config map[string]interface{}) error
    Close() error
}

// ToolPlugin 工具插件
type ToolPlugin interface {
    Plugin
    GetTools() []Tool
}

// LLMPlugin LLM 插件
type LLMPlugin interface {
    Plugin
    GetProvider() LLMProvider
}

// MemoryPlugin 记忆插件
type MemoryPlugin interface {
    Plugin
    GetStore() MemoryStore
}
```

### 17.3 插件加载

```go
type PluginManager struct {
    plugins map[string]Plugin
    tools   *tools.Registry
    llm     *llm.Gateway
}

func (pm *PluginManager) Load(dir string) error {
    entries, err := os.ReadDir(dir)
    if err != nil {
        return err
    }
    
    for _, entry := range entries {
        if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".so") {
            if err := pm.loadPlugin(filepath.Join(dir, entry.Name())); err != nil {
                log.Printf("加载插件失败 %s: %v", entry.Name(), err)
            }
        }
    }
    
    return nil
}

func (pm *PluginManager) loadPlugin(path string) error {
    p, err := plugin.Open(path)
    if err != nil {
        return err
    }
    
    create, err := p.Lookup("Create")
    if err != nil {
        return err
    }
    
    pluginInstance := create.(func() Plugin)()
    
    if err := pluginInstance.Init(nil); err != nil {
        return err
    }
    
    pm.plugins[pluginInstance.Name()] = pluginInstance
    
    switch p := pluginInstance.(type) {
    case ToolPlugin:
        for _, tool := range p.GetTools() {
            pm.tools.Register(tool)
        }
    case LLMPlugin:
        pm.llm.RegisterProvider(p.GetProvider())
    }
    
    return nil
}
```

### 17.4 插件示例

```go
// myplugin.go
package main

import (
    "github.com/lzy1102/openaide/plugin"
)

type MyPlugin struct{}

func (p *MyPlugin) Name() string { return "my-plugin" }
func (p *MyPlugin) Version() string { return "1.0.0" }

func (p *MyPlugin) Init(config map[string]interface{}) error {
    return nil
}

func (p *MyPlugin) Close() error {
    return nil
}

func (p *MyPlugin) GetTools() []plugin.Tool {
    return []plugin.Tool{
        {
            Name:        "my_tool",
            Description: "我的自定义工具",
            Handler:     myHandler,
        },
    }
}

func myHandler(args map[string]interface{}) (string, error) {
    return "执行成功", nil
}

func Create() plugin.Plugin {
    return &MyPlugin{}
}
```

### 17.5 插件配置

```yaml
# ~/.openaide/config.yaml
plugins:
  enabled: [my-plugin, db-plugin]
  directory: ~/.openaide/plugins
  
  my-plugin:
    api_key: xxx
    
  db-plugin:
    connection: postgres://localhost/mydb
```

### 17.6 插件管理命令

```bash
# 列出插件
openaide plugin list

# 安装插件
openaide plugin install https://github.com/user/my-plugin

# 启用插件
openaide plugin enable my-plugin

# 禁用插件
openaide plugin disable my-plugin

# 卸载插件
openaide plugin uninstall my-plugin
```

---

## 18. 版本兼容性

### 18.1 API 版本管理

```go
// api/version.go
package api

const (
    APIVersionV1 = "v1"
    APIVersionV2 = "v2"
    APIVersionV3 = "v3"
)

type VersionRouter struct {
    versions map[string]*gin.RouterGroup
}

func (vr *VersionRouter) Register(v string, r *gin.RouterGroup) {
    vr.versions[v] = r
}

func (vr *VersionRouter) Route(v string) *gin.RouterGroup {
    return vr.versions[v]
}
```

### 18.2 配置版本迁移

```go
func MigrateConfigV1ToV2(old ConfigV1) ConfigV2 {
    return ConfigV2{
        Model: old.Model,
        APIKey: old.APIKey,
        ShowThinking: false,
        AutoConfirm: false,
    }
}
```

---

## 19. CLI 补全

### 19.1 Shell 补全脚本

```bash
# bash completion
_openaide_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    opts="chat server tui config plugin help"
    
    case "${prev}" in
        config)
            opts="get set list reset"
            ;;
        plugin)
            opts="list install enable disable uninstall"
            ;;
        server)
            opts="start stop status"
            ;;
    esac
    
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

complete -F _openaide_completion openaide
```

### 19.2 自动生成补全

```go
func GenerateCompletion(shell string) string {
    switch shell {
    case "bash":
        return generateBashCompletion()
    case "zsh":
        return generateZshCompletion()
    case "fish":
        return generateFishCompletion()
    default:
        return ""
    }
}
```

---

## 20. 更新检查

```go
type Updater struct {
    currentVersion string
    checkURL       string
}

func (u *Updater) CheckUpdate() (*UpdateInfo, error) {
    resp, err := http.Get(u.checkURL)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var info UpdateInfo
    if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
        return nil, err
    }
    
    if info.Version != u.currentVersion {
        return &info, nil
    }
    
    return nil, nil
}

type UpdateInfo struct {
    Version     string `json:"version"`
    DownloadURL string `json:"download_url"`
    ReleaseNote string `json:"release_note"`
}
```

---

## 21. CLI 命令设计（参考 Claude Code）

### 21.1 核心命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `openaide` | 进入当前项目会话（恢复或新建） | `openaide` |
| `openaide -c` | 继续上次会话 | `openaide -c` |
| `openaide -n` | 强制新建会话 | `openaide -n` |
| `/clear` | 清除当前上下文（会话内） | `> /clear` |
| `/exit` | 退出会话（会话内） | `> /exit` |
| `/help` | 显示帮助（会话内） | `> /help` |

### 21.2 会话管理命令

```bash
# 进入项目（自动恢复或新建）
$ cd /my/project
$ openaide

🚀 OpenAIDE
─────────────────────
项目: my-project
会话: 恢复上次 (2026-05-14 10:30)
─────────────────────
> 

# 继续上次会话
$ openaide -c

🚀 OpenAIDE
─────────────────────
项目: my-project
会话: 继续 (2026-05-14 10:30)
─────────────────────
> 

# 强制新建会话
$ openaide -n

🚀 OpenAIDE
─────────────────────
项目: my-project
会话: 新建 (2026-05-14 11:00)
─────────────────────
> 
```

### 21.3 会话内命令（以 `/` 开头）

```bash
> /clear

🧹 上下文已清除
─────────────────────
> 

> /help

📖 OpenAIDE 命令
─────────────────────
/clear     清除当前上下文
/exit      退出会话
/help      显示帮助
/history   显示对话历史
/config    查看当前配置
/tools     查看可用工具
/memory    查看记忆状态
/save      保存当前会话
/load      加载历史会话
```

### 21.4 代码实现

```go
// cmd/openaide/main.go
package main

import (
    "flag"
    "fmt"
    "os"
)

func main() {
    var (
        continueFlag = flag.Bool("c", false, "继续上次会话")
        newFlag      = flag.Bool("n", false, "新建会话")
        configFlag   = flag.Bool("config", false, "查看配置")
        versionFlag  = flag.Bool("version", false, "查看版本")
    )
    flag.Parse()
    
    projectPath, _ := os.Getwd()
    
    switch {
    case *versionFlag:
        printVersion()
    case *configFlag:
        printConfig()
    case *continueFlag:
        runContinue(projectPath)
    case *newFlag:
        runNew(projectPath)
    default:
        runAuto(projectPath)
    }
}

func runAuto(projectPath string) {
    session := sessionManager.GetActiveSession(projectPath)
    
    if session != nil {
        fmt.Println("🚀 OpenAIDE")
        fmt.Println("─────────────────────")
        fmt.Printf("项目: %s\n", getProjectName(projectPath))
        fmt.Printf("会话: 恢复上次 (%s)\n", session.LastTime.Format("2006-01-02 15:04"))
        fmt.Println("─────────────────────")
        runInteractive(session)
    } else {
        runNew(projectPath)
    }
}

func runContinue(projectPath string) {
    session := sessionManager.GetLastSession(projectPath)
    if session == nil {
        fmt.Println("⚠️  没有找到历史会话，新建会话")
        runNew(projectPath)
        return
    }
    
    fmt.Println("🚀 OpenAIDE")
    fmt.Println("─────────────────────")
    fmt.Printf("项目: %s\n", getProjectName(projectPath))
    fmt.Printf("会话: 继续 (%s)\n", session.LastTime.Format("2006-01-02 15:04"))
    fmt.Println("─────────────────────")
    runInteractive(session)
}

func runNew(projectPath string) {
    session := sessionManager.CreateSession(projectPath)
    
    fmt.Println("🚀 OpenAIDE")
    fmt.Println("─────────────────────")
    fmt.Printf("项目: %s\n", getProjectName(projectPath))
    fmt.Printf("会话: 新建 (%s)\n", session.CreatedAt.Format("2006-01-02 15:04"))
    fmt.Println("─────────────────────")
    runInteractive(session)
}

func runInteractive(session *Session) {
    scanner := bufio.NewScanner(os.Stdin)
    
    for {
        fmt.Print("> ")
        if !scanner.Scan() {
            break
        }
        
        input := scanner.Text()
        
        if strings.HasPrefix(input, "/") {
            if handleCommand(input, session) {
                break
            }
            continue
        }
        
        response, err := session.Agent.Chat(input)
        if err != nil {
            fmt.Printf("❌ 错误: %v\n", err)
            continue
        }
        
        fmt.Println(response)
    }
}

func handleCommand(cmd string, session *Session) bool {
    switch cmd {
    case "/clear":
        session.ClearContext()
        fmt.Println("🧹 上下文已清除")
        return false
    case "/exit":
        session.Save()
        fmt.Println("👋 再见")
        return true
    case "/help":
        printHelp()
        return false
    case "/history":
        printHistory(session)
        return false
    case "/config":
        printSessionConfig(session)
        return false
    case "/tools":
        printAvailableTools(session)
        return false
    case "/memory":
        printMemoryStatus(session)
        return false
    case "/save":
        session.Save()
        fmt.Println("💾 会话已保存")
        return false
    default:
        fmt.Printf("❓ 未知命令: %s，输入 /help 查看帮助\n", cmd)
        return false
    }
}
```

### 21.5 会话状态管理

```go
type SessionManager struct {
    db *gorm.DB
}

type Session struct {
    ID        string    `json:"id"`
    ProjectID string    `json:"project_id"`
    Status    string    `json:"status"` // active, paused, closed
    Messages  []Message `json:"messages"`
    CreatedAt time.Time `json:"created_at"`
    LastTime  time.Time `json:"last_time"`
}

func (sm *SessionManager) GetActiveSession(projectPath string) *Session {
    projectID := generateProjectID(projectPath)
    
    var session Session
    result := sm.db.Where("project_id = ? AND status = ?", projectID, "active").
        Order("last_time DESC").
        First(&session)
    
    if result.Error != nil {
        return nil
    }
    
    return &session
}

func (sm *SessionManager) GetLastSession(projectPath string) *Session {
    projectID := generateProjectID(projectPath)
    
    var session Session
    result := sm.db.Where("project_id = ?", projectID).
        Order("last_time DESC").
        First(&session)
    
    if result.Error != nil {
        return nil
    }
    
    return &session
}

func (sm *SessionManager) CreateSession(projectPath string) *Session {
    projectID := generateProjectID(projectPath)
    
    sm.db.Model(&Session{}).
        Where("project_id = ? AND status = ?", projectID, "active").
        Update("status", "paused")
    
    session := &Session{
        ID:        generateSessionID(),
        ProjectID: projectID,
        Status:    "active",
        Messages:  []Message{},
        CreatedAt: time.Now(),
        LastTime:  time.Now(),
    }
    
    sm.db.Create(session)
    return session
}

func (s *Session) ClearContext() {
    s.Messages = []Message{s.Messages[0]} // 保留系统消息
    s.LastTime = time.Now()
}

func (s *Session) Save() {
    s.LastTime = time.Now()
}
```

### 21.6 与 Claude Code 对比

| 功能 | Claude Code | OpenAIDE |
|------|-------------|----------|
| 进入项目 | `claude` | `openaide` |
| 继续会话 | `claude -c` | `openaide -c` |
| 新建会话 | `claude -n` | `openaide -n` |
| 清除上下文 | `/clear` | `/clear` |
| 退出 | `/exit` | `/exit` |
| 查看帮助 | `/help` | `/help` |
| 查看历史 | `/history` | `/history` |
| 查看配置 | - | `/config` |
| 查看工具 | - | `/tools` |
| 查看记忆 | - | `/memory` |
| 保存会话 | - | `/save` |

---

## 22. 纯文件存储架构

### 22.1 设计原则

1. **所有数据存文件**：人类可读，可编辑，可版本控制
2. **索引用 JSON**：快速查询，无外部依赖
3. **向量用二进制文件**：高效存储，内存搜索
4. **零数据库**：不需要 SQLite，不需要 GORM

### 22.2 目录结构

```
~/.openaide/
├── config.yaml              # 配置（YAML）
├── sessions/                # 会话
│   ├── index.json          # 会话索引
│   └── *.md                # 对话记录（Markdown）
├── memory/                  # 长期记忆
│   ├── summary.md          # 摘要
│   └── key_infos.json      # 关键信息索引
└── knowledge/               # 知识库
    ├── index.json          # 知识索引
    ├── embeddings.bin      # 向量数据（二进制）
    └── docs/               # 文档
        ├── *.md
        └── *.txt
```

### 22.3 会话索引（JSON）

```json
{
  "version": "1.0",
  "sessions": [
    {
      "id": "xxx",
      "project_id": "my-project",
      "file_path": "sessions/2026-05-14-xxx.md",
      "created_at": "2026-05-14T10:30:00Z",
      "updated_at": "2026-05-14T11:00:00Z",
      "message_count": 10
    }
  ]
}
```

### 22.4 对话记录（Markdown）

```markdown
# OpenAIDE 会话记录

## 会话信息
- 项目: my-project
- 开始时间: 2026-05-14 10:30

---

## 对话 1

### 用户 (10:30)
你好，帮我写一个 Go 函数

### AI (10:31)
好的，这是一个简单的 Go 函数：

```go
func Hello() string {
    return "Hello, World!"
}
```
```

### 22.5 知识库索引（JSON）

```json
{
  "version": "1.0",
  "documents": [
    {
      "id": "api-design",
      "title": "API设计规范",
      "file_path": "docs/api-design.md",
      "tags": ["api", "design"],
      "embedding_offset": 0,
      "embedding_length": 1536
    }
  ],
  "tags": {
    "api": ["api-design"],
    "design": ["api-design"]
  }
}
```

### 22.6 向量文件（二进制）

```
embeddings.bin
┌─────────────────────────────────────┐
│  Header (16 bytes)                  │
│  - Magic: "VECT" (4 bytes)          │
│  - Version: 1 (4 bytes)             │
│  - Dimension: 1536 (4 bytes)        │
│  - Count: 100 (4 bytes)             │
├─────────────────────────────────────┤
│  Vector 1 (6144 bytes)              │
│  [0.1, -0.2, 0.3, ...]              │
├─────────────────────────────────────┤
│  Vector 2 (6144 bytes)              │
├─────────────────────────────────────┤
│  ...                                │
└─────────────────────────────────────┘
```

### 22.7 代码实现

```go
package storage

type FileStorage struct {
    baseDir string
    mu      sync.RWMutex
}

// 加载会话索引
func (s *FileStorage) loadSessionIndex() (*SessionIndex, error) {
    path := filepath.Join(s.baseDir, "sessions", "index.json")
    data, err := os.ReadFile(path)
    if err != nil {
        if os.IsNotExist(err) {
            return &SessionIndex{Version: "1.0"}, nil
        }
        return nil, err
    }
    
    var index SessionIndex
    if err := json.Unmarshal(data, &index); err != nil {
        return nil, err
    }
    return &index, nil
}

// 保存会话
func (s *FileStorage) SaveConversation(session Session) error {
    // 1. 保存 Markdown
    mdPath := filepath.Join(s.baseDir, "sessions", session.ID+".md")
    if err := s.writeMarkdown(mdPath, session); err != nil {
        return err
    }
    
    // 2. 更新索引
    index, err := s.loadSessionIndex()
    if err != nil {
        return err
    }
    
    entry := SessionEntry{
        ID:           session.ID,
        ProjectID:    session.ProjectID,
        FilePath:     "sessions/" + session.ID + ".md",
        CreatedAt:    session.CreatedAt,
        UpdatedAt:    time.Now(),
        MessageCount: len(session.Messages),
    }
    
    // 更新或添加
    updated := false
    for i, existing := range index.Sessions {
        if existing.ID == session.ID {
            index.Sessions[i] = entry
            updated = true
            break
        }
    }
    if !updated {
        index.Sessions = append(index.Sessions, entry)
    }
    
    // 3. 保存索引
    return s.saveSessionIndex(index)
}

// 向量搜索（内存暴力搜索）
type FileVectorStore struct {
    filePath  string
    dimension int
    vectors   [][]float32
}

func (vs *FileVectorStore) Load() error {
    file, err := os.Open(vs.filePath)
    if err != nil {
        return err
    }
    defer file.Close()
    
    // 读取 Header
    var header struct {
        Magic     [4]byte
        Version   int32
        Dimension int32
        Count     int32
    }
    
    if err := binary.Read(file, binary.LittleEndian, &header); err != nil {
        return err
    }
    
    vs.dimension = int(header.Dimension)
    count := int(header.Count)
    
    // 读取向量到内存
    vs.vectors = make([][]float32, count)
    for i := 0; i < count; i++ {
        vec := make([]float32, vs.dimension)
        if err := binary.Read(file, binary.LittleEndian, vec); err != nil {
            return err
        }
        vs.vectors[i] = vec
    }
    
    return nil
}

func (vs *FileVectorStore) Search(query []float32, k int) []SearchResult {
    var results []SearchResult
    
    for i, vec := range vs.vectors {
        score := cosineSimilarity(query, vec)
        results = append(results, SearchResult{
            Index: i,
            Score: score,
        })
    }
    
    // 排序
    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })
    
    // 返回 top-k
    if len(results) > k {
        results = results[:k]
    }
    
    return results
}
```

### 22.8 性能评估

| 数据量 | 关键词查询 | 向量搜索 | 是否够用 |
|--------|-----------|----------|----------|
| 10 篇 | < 1ms | < 10ms | ✅ |
| 100 篇 | < 5ms | < 50ms | ✅ |
| 1000 篇 | < 20ms | < 500ms | ✅ |

### 22.9 与主流 Agent 对比

| Agent | 存储方式 | 持久化 | 可编辑 |
|-------|----------|--------|--------|
| **Claude Code** | 内存 | ❌ | - |
| **Aider** | Markdown | ✅ | ✅ |
| **Continue** | JSON | ✅ | ✅ |
| **OpenAIDE** | **Markdown + JSON** | **✅** | **✅** |

### 22.10 依赖清单

| 依赖 | 用途 | 大小 |
|------|------|------|
| `go-cache` | 内存缓存 | ~50KB |

**总依赖：1 个库，纯 Go，零外部服务**

---

---

## 23. Git 集成与代码索引系统

> 目标: 让 Agent 理解代码变更、安全提交、项目级代码导航
> 优先级: P0 (Git 集成) / P1 (代码索引)

### 23.1 为什么需要 Git 集成

**当前问题**: Agent 可以执行 `git commit`，但：
- 看不懂 diff 的含义
- 无法生成有意义的提交信息
- 不知道变更影响了哪些代码
- 误操作后无法安全回滚

**对比 Claude Code**:
```
Claude Code: "检测到 3 个文件变更，建议提交信息: fix(auth): 修复登录校验"
OpenAIDE:   "git commit -m 'update'" (无意义提交)
```

### 23.2 Git 集成设计

#### 23.2.1 核心能力

| 能力 | 说明 | 优先级 |
|------|------|--------|
| **状态感知** | 解析 git status，结构化展示变更 | P0 |
| **Diff 解析** | 提取变更的函数/类型/行号 | P0 |
| **提交建议** | 基于 diff 生成 conventional commit | P0 |
| **安全提交** | 预览 + 确认 + 可回滚 | P0 |
| **历史查询** | "这个文件最近谁改的？" | P1 |
| **影响分析** | "改这个接口影响哪些地方？" | P1 |

#### 23.2.2 数据结构

```go
// internal/tools/git_types.go
package tools

// GitStatus 结构化状态
type GitStatus struct {
    Branch      string
    Ahead       int
    Behind      int
    Modified    []FileChange
    Staged      []FileChange
    Untracked   []string
    Conflicts   []string
    Clean       bool
}

type FileChange struct {
    Path       string
    Status     string  // modified, added, deleted, renamed
    Additions  int
    Deletions  int
    Diff       string  // 前 100 行
    Functions  []FuncChange  // 变更的函数
}

type FuncChange struct {
    Name       string
    Type       string  // added, modified, deleted
    OldLine    int
    NewLine    int
    Signature  string  // func (r *Receiver) Name(params) returns
}
```

#### 23.2.3 实现细节

**步骤 1: 调用 git 命令**
```go
// internal/tools/git_executor.go
package tools

import (
    "os/exec"
    "strings"
)

type GitExecutor struct {
    workDir string
}

func (g *GitExecutor) Status() (*GitStatus, error) {
    // git status --porcelain=v2 --branch
    output, err := g.exec("status", "--porcelain=v2", "--branch")
    if err != nil {
        return nil, err
    }
    return g.parseStatus(output)
}

func (g *GitExecutor) Diff(path string) (*FileChange, error) {
    // git diff --unified=3 -- function-context
    output, err := g.exec("diff", "--unified=3", "--", path)
    if err != nil {
        return nil, err
    }
    return g.parseDiff(path, output)
}

func (g *GitExecutor) exec(args ...string) (string, error) {
    cmd := exec.Command("git", args...)
    cmd.Dir = g.workDir
    out, err := cmd.CombinedOutput()
    return string(out), err
}
```

**步骤 2: 解析 diff 提取函数变更**
```go
// internal/tools/git_parser.go
package tools

import (
    "regexp"
    "strings"
)

// 解析 git diff 输出，提取函数级变更
func ParseDiff(diff string) []FuncChange {
    var changes []FuncChange
    
    // 匹配函数定义行: @@ -old,old_len +new,new_len @@ func Name(...)
    hunkRegex := regexp.MustCompile(`@@ -(\d+),\d+ \+(\d+),\d+ @@ (.*)`)
    // 匹配 Go 函数定义
    funcRegex := regexp.MustCompile(`^[+-]?func\s+(?:\([^)]+\)\s+)?(\w+)\s*\(`)
    
    lines := strings.Split(diff, "\n")
    currentHunk := ""
    
    for _, line := range lines {
        if matches := hunkRegex.FindStringSubmatch(line); matches != nil {
            currentHunk = matches[3]
            continue
        }
        
        if matches := funcRegex.FindStringSubmatch(line); matches != nil {
            funcName := matches[1]
            changeType := "modified"
            if strings.HasPrefix(line, "+") {
                changeType = "added"
            } else if strings.HasPrefix(line, "-") {
                changeType = "deleted"
            }
            
            changes = append(changes, FuncChange{
                Name:      funcName,
                Type:      changeType,
                Signature: extractSignature(currentHunk),
            })
        }
    }
    
    return changes
}
```

**步骤 3: 生成提交信息**
```go
// internal/tools/git_commit.go
package tools

import (
    "fmt"
    "strings"
)

// GenerateCommitMessage 基于变更生成提交信息
func GenerateCommitMessage(status *GitStatus, llm LLMClient) (string, error) {
    // 构建 prompt
    var sb strings.Builder
    sb.WriteString("分析以下代码变更，生成符合 Conventional Commits 规范的提交信息。\n\n")
    sb.WriteString("变更摘要:\n")
    
    for _, f := range status.Staged {
        sb.WriteString(fmt.Sprintf("- %s (%s): +%d -%d\n", 
            f.Path, f.Status, f.Additions, f.Deletions))
        for _, fn := range f.Functions {
            sb.WriteString(fmt.Sprintf("  - %s: %s\n", fn.Type, fn.Name))
        }
    }
    
    sb.WriteString("\n要求:\n")
    sb.WriteString("1. 格式: <type>(<scope>): <subject>\n")
    sb.WriteString("2. type: feat, fix, docs, style, refactor, test, chore\n")
    sb.WriteString("3. subject 不超过 50 字\n")
    sb.WriteString("4. 如有必要添加 body 描述\n")
    sb.WriteString("\n只返回提交信息，不要解释。")
    
    return llm.Complete(sb.String())
}

// 简化版：无需 LLM，基于规则生成
func GenerateCommitMessageSimple(status *GitStatus) string {
    // 统计变更类型
    hasFeat := false
    hasFix := false
    hasDocs := false
    
    for _, f := range status.Staged {
        if strings.HasSuffix(f.Path, ".md") {
            hasDocs = true
        }
        for _, fn := range f.Functions {
            if strings.Contains(strings.ToLower(fn.Name), "fix") {
                hasFix = true
            }
        }
    }
    
    // 简单规则判断
    switch {
    case hasFix:
        return fmt.Sprintf("fix: 修复 %s", status.Staged[0].Path)
    case hasDocs:
        return fmt.Sprintf("docs: 更新 %s", status.Staged[0].Path)
    default:
        return fmt.Sprintf("feat: 更新 %s", status.Staged[0].Path)
    }
}
```

**步骤 4: 安全提交流程**
```go
// internal/tools/git_safe_commit.go
package tools

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

// SafeCommit 带确认的安全提交
func SafeCommit(executor *GitExecutor, message string, auto bool) error {
    // 1. 检查是否有暂存变更
    status, err := executor.Status()
    if err != nil {
        return err
    }
    if len(status.Staged) == 0 {
        return fmt.Errorf("没有暂存的变更，请先执行 git add")
    }
    
    // 2. 显示预览
    fmt.Println("=== 提交预览 ===")
    fmt.Printf("提交信息: %s\n", message)
    fmt.Printf("变更文件: %d 个\n", len(status.Staged))
    for _, f := range status.Staged {
        fmt.Printf("  %s (%s): +%d -%d\n", f.Path, f.Status, f.Additions, f.Deletions)
    }
    
    // 3. 显示 diff 摘要
    fmt.Println("\n=== Diff 摘要 ===")
    for _, f := range status.Staged {
        if len(f.Diff) > 0 {
            lines := strings.Split(f.Diff, "\n")
            for i, line := range lines {
                if i >= 20 {
                    fmt.Println("  ...")
                    break
                }
                if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
                    fmt.Printf("  %s\n", line[:min(len(line), 80)])
                }
            }
        }
    }
    
    // 4. 确认
    if !auto {
        fmt.Print("\n确认提交? [Y/n/edit/cancel] ")
        reader := bufio.NewReader(os.Stdin)
        input, _ := reader.ReadString('\n')
        input = strings.TrimSpace(strings.ToLower(input))
        
        switch input {
        case "", "y", "yes":
            // 继续提交
        case "edit", "e":
            fmt.Print("输入新提交信息: ")
            newMsg, _ := reader.ReadString('\n')
            message = strings.TrimSpace(newMsg)
        default:
            return fmt.Errorf("提交已取消")
        }
    }
    
    // 5. 执行提交
    _, err = executor.exec("commit", "-m", message)
    return err
}
```

#### 23.2.4 CLI 命令设计

```bash
# 查看 Git 状态
openaide> /git status
分支: feature/login
状态: 有 3 个文件变更
  M  auth/login.go        +45 -12  (已暂存)
  M  user/model.go        +8  -2   (已暂存)
  M  docs/api.md          +15 -0   (未暂存)

# 生成提交建议
openaide> /git suggest
建议提交信息:
  feat(auth): 添加角色支持，修复登录校验逻辑

  - 在 user 模型添加 Role 字段
  - 修复登录密码校验逻辑
  - 更新 API 文档

# 安全提交
openaide> /git commit
=== 提交预览 ===
提交信息: feat(auth): 添加角色支持，修复登录校验逻辑
变更文件: 3 个
...
确认提交? [Y/n/edit/cancel]
```

---

### 23.3 项目级代码索引

#### 23.3.1 为什么需要索引

**当前问题**: Agent 只能读取单个文件，不知道：
- 这个函数在哪里被调用
- 修改这个接口会影响哪些地方
- 项目有哪些模块、依赖关系如何

**对比 Claude Code**:
```
用户: "谁调用了 GetUser？"
Claude Code: "GetUser 在 3 处被调用: handlers/auth.go:23, handlers/user.go:56..."
OpenAIDE:    "我帮你搜索一下..." (文本搜索，不准确)
```

#### 23.3.2 轻量级索引设计

**核心原则**: 不解析完整 AST，用正则 + 简单语法分析

```go
// internal/index/symbol_index.go
package index

import (
    "encoding/json"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
    "time"
)

// SymbolIndex 符号索引
type SymbolIndex struct {
    Version    string                 `json:"version"`
    ProjectID  string                 `json:"project_id"`
    UpdatedAt  time.Time              `json:"updated_at"`
    
    // 符号表
    Functions  map[string][]Location  `json:"functions"`  // 函数名 -> 位置列表
    Types      map[string][]Location  `json:"types"`      // 类型名 -> 位置列表
    Methods    map[string][]Location  `json:"methods"`    // 方法名 -> 位置列表 (Receiver.Method)
    Imports    map[string][]string    `json:"imports"`    // 包路径 -> 导入它的文件
    
    // 文件元数据
    Files      map[string]FileMeta    `json:"files"`
}

type Location struct {
    File    string `json:"file"`
    Line    int    `json:"line"`
    Column  int    `json:"column"`
    Comment string `json:"comment,omitempty"`  // 文档注释
}

type FileMeta struct {
    Package   string   `json:"package"`
    Imports   []string `json:"imports"`
    Functions []string `json:"functions"`
    Types     []string `json:"types"`
    Size      int64    `json:"size"`
    ModTime   int64    `json:"mod_time"`
}
```

#### 23.3.3 索引构建实现

```go
// internal/index/builder.go
package index

import (
    "bufio"
    "os"
    "path/filepath"
    "regexp"
    "strings"
)

type IndexBuilder struct {
    root      string
    index     *SymbolIndex
    funcRegex *regexp.Regexp
    typeRegex *regexp.Regexp
    methodRegex *regexp.Regexp
    importRegex *regexp.Regexp
    commentRegex *regexp.Regexp
}

func NewIndexBuilder(root string) *IndexBuilder {
    return &IndexBuilder{
        root: root,
        index: &SymbolIndex{
            Version:   "1.0",
            Functions: make(map[string][]Location),
            Types:     make(map[string][]Location),
            Methods:   make(map[string][]Location),
            Imports:   make(map[string][]string),
            Files:     make(map[string]FileMeta),
        },
        // Go 函数定义: func Name(params) returns { 或 func (r *Receiver) Name(params) {
        funcRegex: regexp.MustCompile(`^func\s+(\w+)\s*\(`),
        // Go 方法定义: func (r *Receiver) Name(params) {
        methodRegex: regexp.MustCompile(`^func\s+\([^)]+\)\s+(\w+)\s*\(`),
        // Go 类型定义: type Name struct/interface/func
        typeRegex: regexp.MustCompile(`^type\s+(\w+)\s+(?:struct|interface|func)`),
        // Go 导入: import "path" 或 import ( ... )
        importRegex: regexp.MustCompile(`["']([^"']+)["']`),
        // 文档注释
        commentRegex: regexp.MustCompile(`^//\s*(.+)`),
    }
}

// Build 构建索引
func (b *IndexBuilder) Build() (*SymbolIndex, error) {
    // 遍历所有 .go 文件
    err := filepath.Walk(b.root, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil
        }
        
        // 跳过 vendor、.git、test 文件
        if strings.Contains(path, "vendor/") || 
           strings.Contains(path, ".git/") ||
           strings.HasSuffix(path, "_test.go") {
            return nil
        }
        
        if strings.HasSuffix(path, ".go") {
            b.indexFile(path)
        }
        
        return nil
    })
    
    if err != nil {
        return nil, err
    }
    
    b.index.UpdatedAt = time.Now()
    return b.index, nil
}

func (b *IndexBuilder) indexFile(path string) error {
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer file.Close()
    
    relPath, _ := filepath.Rel(b.root, path)
    meta := FileMeta{
        File: relPath,
    }
    
    scanner := bufio.NewScanner(file)
    lineNum := 0
    lastComment := ""
    
    for scanner.Scan() {
        lineNum++
        line := scanner.Text()
        
        // 提取文档注释
        if matches := b.commentRegex.FindStringSubmatch(line); matches != nil {
            lastComment = matches[1]
            continue
        }
        
        // 提取包名
        if strings.HasPrefix(line, "package ") {
            meta.Package = strings.TrimSpace(strings.TrimPrefix(line, "package "))
            continue
        }
        
        // 提取导入
        if strings.Contains(line, "import") {
            if matches := b.importRegex.FindAllStringSubmatch(line, -1); matches != nil {
                for _, m := range matches {
                    importPath := m[1]
                    meta.Imports = append(meta.Imports, importPath)
                    b.index.Imports[importPath] = append(b.index.Imports[importPath], relPath)
                }
            }
            continue
        }
        
        // 提取函数
        if matches := b.funcRegex.FindStringSubmatch(line); matches != nil {
            funcName := matches[1]
            loc := Location{
                File:    relPath,
                Line:    lineNum,
                Comment: lastComment,
            }
            b.index.Functions[funcName] = append(b.index.Functions[funcName], loc)
            meta.Functions = append(meta.Functions, funcName)
            lastComment = ""
            continue
        }
        
        // 提取方法
        if matches := b.methodRegex.FindStringSubmatch(line); matches != nil {
            methodName := matches[1]
            loc := Location{
                File:    relPath,
                Line:    lineNum,
                Comment: lastComment,
            }
            b.index.Methods[methodName] = append(b.index.Methods[methodName], loc)
            lastComment = ""
            continue
        }
        
        // 提取类型
        if matches := b.typeRegex.FindStringSubmatch(line); matches != nil {
            typeName := matches[1]
            loc := Location{
                File:    relPath,
                Line:    lineNum,
                Comment: lastComment,
            }
            b.index.Types[typeName] = append(b.index.Types[typeName], loc)
            meta.Types = append(meta.Types, typeName)
            lastComment = ""
            continue
        }
    }
    
    b.index.Files[relPath] = meta
    return scanner.Err()
}
```

#### 23.3.4 索引查询实现

```go
// internal/index/query.go
package index

import (
    "fmt"
    "strings"
)

type IndexQuery struct {
    index *SymbolIndex
}

// FindFunction 查找函数定义
func (q *IndexQuery) FindFunction(name string) ([]Location, bool) {
    locs, ok := q.index.Functions[name]
    return locs, ok
}

// FindType 查找类型定义
func (q *IndexQuery) FindType(name string) ([]Location, bool) {
    locs, ok := q.index.Types[name]
    return locs, ok
}

// FindCallers 查找调用者（简化版：文本搜索）
func (q *IndexQuery) FindCallers(funcName string) []Location {
    var callers []Location
    
    // 搜索 "funcName(" 模式
    searchPattern := funcName + "("
    
    for filePath, meta := range q.index.Files {
        content, err := os.ReadFile(filepath.Join(q.index.root, filePath))
        if err != nil {
            continue
        }
        
        lines := strings.Split(string(content), "\n")
        for i, line := range lines {
            // 排除函数定义行（那是被调用者）
            if strings.Contains(line, "func "+funcName) {
                continue
            }
            // 排除注释
            if strings.HasPrefix(strings.TrimSpace(line), "//") {
                continue
            }
            // 查找调用
            if strings.Contains(line, searchPattern) {
                callers = append(callers, Location{
                    File: filePath,
                    Line: i + 1,
                })
            }
        }
    }
    
    return callers
}

// AnalyzeImpact 分析变更影响
func (q *IndexQuery) AnalyzeImpact(funcName string) *ImpactReport {
    report := &ImpactReport{
        Function: funcName,
    }
    
    // 1. 找到函数定义
    if locs, ok := q.FindFunction(funcName); ok {
        report.Definition = locs[0]
    }
    
    // 2. 找到所有调用者
    report.Callers = q.FindCallers(funcName)
    
    // 3. 分析调用者的调用者（二级影响）
    for _, caller := range report.Callers {
        // 提取调用者所在函数名
        callerFunc := q.findEnclosingFunction(caller.File, caller.Line)
        if callerFunc != "" {
            indirect := q.FindCallers(callerFunc)
            report.IndirectCallers = append(report.IndirectCallers, indirect...)
        }
    }
    
    return report
}

type ImpactReport struct {
    Function        string
    Definition      Location
    Callers         []Location
    IndirectCallers []Location
}

func (r *ImpactReport) String() string {
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("影响分析: %s\n", r.Function))
    sb.WriteString(fmt.Sprintf("定义: %s:%d\n", r.Definition.File, r.Definition.Line))
    sb.WriteString(fmt.Sprintf("直接调用: %d 处\n", len(r.Callers)))
    for _, c := range r.Callers {
        sb.WriteString(fmt.Sprintf("  - %s:%d\n", c.File, c.Line))
    }
    sb.WriteString(fmt.Sprintf("间接调用: %d 处\n", len(r.IndirectCallers)))
    return sb.String()
}
```

#### 23.3.5 索引存储与更新

```go
// internal/index/storage.go
package index

import (
    "crypto/md5"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
)

// IndexStorage 索引存储
type IndexStorage struct {
    baseDir string
}

func NewIndexStorage(baseDir string) *IndexStorage {
    return &IndexStorage{
        baseDir: filepath.Join(baseDir, "index"),
    }
}

// Save 保存索引
func (s *IndexStorage) Save(projectRoot string, index *SymbolIndex) error {
    // 项目哈希作为文件名
    projectHash := hashProject(projectRoot)
    path := filepath.Join(s.baseDir, projectHash+".json")
    
    // 确保目录存在
    os.MkdirAll(s.baseDir, 0755)
    
    data, err := json.MarshalIndent(index, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(path, data, 0644)
}

// Load 加载索引
func (s *IndexStorage) Load(projectRoot string) (*SymbolIndex, error) {
    projectHash := hashProject(projectRoot)
    path := filepath.Join(s.baseDir, projectHash+".json")
    
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    
    var index SymbolIndex
    if err := json.Unmarshal(data, &index); err != nil {
        return nil, err
    }
    
    return &index, nil
}

// IsStale 检查索引是否过期
func (s *IndexStorage) IsStale(projectRoot string) bool {
    index, err := s.Load(projectRoot)
    if err != nil {
        return true
    }
    
    // 检查每个文件的修改时间
    for filePath, meta := range index.Files {
        fullPath := filepath.Join(projectRoot, filePath)
        info, err := os.Stat(fullPath)
        if err != nil {
            return true
        }
        if info.ModTime().Unix() > meta.ModTime {
            return true
        }
    }
    
    return false
}

func hashProject(root string) string {
    // 用项目路径的 MD5 作为哈希
    return fmt.Sprintf("%x", md5.Sum([]byte(root)))
}
```

#### 23.3.6 与 Agent 内核集成

```go
// internal/kernel/context.go
package kernel

// 在组装上下文时注入代码索引信息
func (k *Kernel) enrichContext(ctx *Context) error {
    // 1. 检查索引是否存在/过期
    storage := index.NewIndexStorage(k.config.BaseDir)
    
    if storage.IsStale(ctx.ProjectRoot) {
        // 异步重建索引
        go k.rebuildIndex(ctx.ProjectRoot)
    }
    
    // 2. 加载索引
    idx, err := storage.Load(ctx.ProjectRoot)
    if err != nil {
        return nil // 索引不存在也不报错
    }
    
    // 3. 注入相关符号信息到上下文
    query := &index.IndexQuery{Index: idx}
    
    // 如果用户在讨论某个函数，注入其调用链
    if ctx.FocusedSymbol != "" {
        if locs, ok := query.FindFunction(ctx.FocusedSymbol); ok {
            ctx.AddSystemPrompt(fmt.Sprintf(
                "相关代码: %s 定义在 %s:%d",
                ctx.FocusedSymbol, locs[0].File, locs[0].Line,
            ))
            
            // 注入调用者信息
            callers := query.FindCallers(ctx.FocusedSymbol)
            if len(callers) > 0 {
                ctx.AddSystemPrompt(fmt.Sprintf(
                    "被调用位置: %d 处", len(callers),
                ))
            }
        }
    }
    
    return nil
}
```

#### 23.3.7 CLI 交互示例

```bash
# 构建索引
openaide> /index build
正在构建代码索引...
扫描 45 个文件...
发现 128 个函数，32 个类型
索引已保存: ~/.openaide/index/a1b2c3d4.json

# 查询符号
openaide> /index find GetUser
GetUser 定义在:
  - models/user.go:45
    // GetUser 根据 ID 获取用户信息

# 查找调用者
openaide> /index callers GetUser
GetUser 被调用位置:
  - handlers/auth.go:23    LoginHandler
  - handlers/user.go:56    GetProfileHandler
  - services/user.go:89    BatchGetUsers

# 影响分析
openaide> /index impact GetUser
影响分析: GetUser
定义: models/user.go:45
直接调用: 3 处
  - handlers/auth.go:23
  - handlers/user.go:56
  - services/user.go:89
间接调用: 5 处
  - handlers/auth.go:45 (LoginHandler 被路由调用)
  ...

# 智能提示（自动使用索引）
openaide> 帮我修改 GetUser 函数
[Agent 自动查询索引]
Agent: GetUser 在 models/user.go:45 定义，被 3 处调用。
修改时需要注意:
1. handlers/auth.go:23 的 LoginHandler 依赖返回值
2. services/user.go:89 的 BatchGetUsers 批量调用
建议先查看所有调用点，确认接口兼容性。
```

---

### 23.4 性能评估

| 指标 | 目标值 | 测试条件 |
|------|--------|----------|
| **Git 状态解析** | < 100ms | 100 个文件变更 |
| **Diff 解析** | < 50ms | 单文件 100 行变更 |
| **提交建议生成** | < 2s | 使用 LLM |
| **索引构建** | < 5s | 100 个 Go 文件 |
| **索引查询** | < 10ms | 内存加载后 |
| **调用者搜索** | < 500ms | 100 个文件 |
| **索引存储** | < 1MB | 1000 个函数 |

---

### 23.5 与现有架构的融合

```
┌─────────────────────────────────────────┐
│           CLI 层                         │
│  /git status  /git commit  /index find  │
└─────────────┬───────────────────────────┘
              │
┌─────────────▼───────────────────────────┐
│           工具层 (tools/)                │
│  ┌─────────┐  ┌─────────────────────┐  │
│  │ Git     │  │ 代码索引            │  │
│  │ 集成    │  │ (index/)            │  │
│  └────┬────┘  └──────────┬──────────┘  │
│       │                  │             │
│       └────────┬─────────┘             │
│                ▼                       │
│         ┌─────────────┐                │
│         │ Agent 内核   │                │
│         │ (ReAct 循环) │                │
│         └──────┬──────┘                │
│                │                       │
│         ┌──────▼──────┐                │
│         │   LLM 层    │                │
│         └─────────────┘                │
└─────────────────────────────────────────┘
```

---

### 23.6 实现路线图

| 阶段 | 功能 | 时间 | 优先级 |
|------|------|------|--------|
| **Phase 1** | Git 状态解析 + diff 提取 | 3 天 | P0 |
| **Phase 2** | 提交建议生成 + 安全提交 | 2 天 | P0 |
| **Phase 3** | 基础符号索引（函数/类型） | 5 天 | P1 |
| **Phase 4** | 调用者搜索 + 影响分析 | 3 天 | P1 |
| **Phase 5** | 与内核集成（自动注入上下文） | 3 天 | P1 |
| **Phase 6** | AST 精确索引（长期） | 2 周 | P2 |

---

---

## 24. 多模态支持系统

> 目标: 扩展 Agent 输入能力，支持图像、语音、文档
> 优先级: P2
> 状态: 设计阶段

### 24.1 为什么需要多模态

**当前局限**:
- 只能处理文本输入
- 无法分析截图中的错误信息
- 无法理解 UI 设计图
- 无法处理语音指令

**典型场景**:
```
用户: "这个报错是什么意思？" [截图]
OpenAIDE: ❌ 无法处理图片

用户: "帮我看看这个界面设计" [设计图]
OpenAIDE: ❌ 无法处理图片

用户: "用语音说: 帮我写个排序函数"
OpenAIDE: ❌ 无法处理语音
```

### 24.2 设计原则

| 原则 | 说明 |
|------|------|
| **可选模块** | 默认不启用，按需加载 |
| **纯 Go 实现** | 图像解码、音频转码不用 CGO |
| **本地优先** | 优先本地处理，保护隐私 |
| **模型无关** | 支持多种视觉/语音模型 |
| **渐进支持** | 先图像，后语音，再视频 |

### 24.3 架构设计

```
┌─────────────────────────────────────────┐
│           多模态输入层                    │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │  图像   │ │  语音   │ │  文档   │  │
│  │ 输入    │ │ 输入    │ │ 输入    │  │
│  └────┬────┘ └────┬────┘ └────┬────┘  │
└───────┼───────────┼───────────┼───────┘
        │           │           │
        ▼           ▼           ▼
┌─────────────────────────────────────────┐
│           预处理层（纯 Go）               │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │ 图像解码 │ │ 语音转码 │ │ 文档提取 │  │
│  │(image/) │ │(audio/) │ │(doc/)   │  │
│  │- PNG    │ │- WAV    │ │- PDF    │  │
│  │- JPEG   │ │- MP3    │ │- DOCX   │  │
│  │- WebP   │ │- OGG    │ │- PPTX   │  │
│  └────┬────┘ └────┬────┘ └────┬────┘  │
└───────┼───────────┼───────────┼───────┘
        │           │           │
        └───────────┼───────────┘
                    ▼
┌─────────────────────────────────────────┐
│           特征提取层                     │
│  ┌─────────────────────────────────┐   │
│  │  视觉模型（可选）                 │   │
│  │  - 本地: llama.cpp (视觉版)      │   │
│  │  - 云端: GPT-4V, Claude 3       │   │
│  └─────────────────────────────────┘   │
│  ┌─────────────────────────────────┐   │
│  │  语音模型（可选）                 │   │
│  │  - 本地: Whisper.cpp             │   │
│  │  - 云端: Whisper API             │   │
│  └─────────────────────────────────┘   │
└─────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│           统一表示层                     │
│  所有模态转换为文本描述 + 结构化数据      │
│  注入到 ReAct 上下文                     │
└─────────────────────────────────────────┘
```

### 24.4 实现细节

**图像处理模块**:
```go
// internal/multimodal/image.go
package multimodal

import (
    "image"
    _ "image/jpeg"
    _ "image/png"
    _ "image/webp"
    "os"
)

type ImageProcessor struct {
    maxWidth  int
    maxHeight int
}

// Process 处理图像输入
func (p *ImageProcessor) Process(path string) (*ImageContent, error) {
    // 1. 解码图像
    file, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer file.Close()
    
    img, format, err := image.Decode(file)
    if err != nil {
        return nil, err
    }
    
    // 2. 缩放（减少 token 消耗）
    img = p.resize(img)
    
    // 3. 转换为 base64
    base64Data, err := p.toBase64(img, format)
    
    // 4. 提取基本元数据
    bounds := img.Bounds()
    
    return &ImageContent{
        Data:      base64Data,
        Format:    format,
        Width:     bounds.Dx(),
        Height:    bounds.Dy(),
        Size:      p.getFileSize(path),
    }, nil
}

type ImageContent struct {
    Data      string // base64
    Format    string
    Width     int
    Height    int
    Size      int64
    Caption   string // 可选：本地模型生成的描述
}
```

**语音处理模块**:
```go
// internal/multimodal/audio.go
package multimodal

// AudioProcessor 语音转文字
type AudioProcessor struct {
    // 使用 Whisper.cpp（纯 C，通过 CGO 或外部进程）
    // 或调用云端 API
}

func (p *AudioProcessor) Transcribe(path string) (*AudioContent, error) {
    // 1. 解码音频（支持 MP3, WAV, OGG）
    // 2. 转换为 WAV 16kHz（Whisper 要求）
    // 3. 调用 Whisper 模型
    // 4. 返回转录文本 + 时间戳
    
    return &AudioContent{
        Text:      "转录的文本",
        Duration:  10.5,
        Segments: []Segment{
            {Start: 0, End: 3.2, Text: "第一段"},
            {Start: 3.2, End: 10.5, Text: "第二段"},
        },
    }, nil
}
```

**与 LLM 集成**:
```go
// 将多模态内容注入到 LLM 请求
func (k *Kernel) buildMultimodalMessage(content *MultimodalContent) []Message {
    var messages []Message
    
    // 文本部分
    if content.Text != "" {
        messages = append(messages, Message{
            Role:    "user",
            Content: content.Text,
        })
    }
    
    // 图像部分（需要视觉模型支持）
    for _, img := range content.Images {
        messages = append(messages, Message{
            Role: "user",
            ContentParts: []ContentPart{
                {
                    Type:     "image_url",
                    ImageURL: &ImageURL{
                        URL: "data:image/png;base64," + img.Data,
                    },
                },
            },
        })
    }
    
    return messages
}
```

### 24.5 配置与启用

```yaml
# config.yaml
multimodal:
  enabled: true
  
  image:
    enabled: true
    max_size: 5MB        # 最大文件大小
    max_width: 1920      # 最大宽度
    max_height: 1080     # 最大高度
    provider: local      # local 或 openai
    
  audio:
    enabled: true
    max_duration: 60s    # 最大时长
    provider: whisper    # whisper 或 openai
    
  document:
    enabled: true
    formats: [pdf, docx, pptx]
```

### 24.6 优缺点

| 优势 | 劣势 |
|------|------|
| 扩展应用场景 | 增加二进制体积 |
| 提升用户体验 | 需要额外模型资源 |
| 本地处理保护隐私 | 处理速度较慢 |
| 可选模块不影响核心 | 配置复杂度增加 |

---

## 25. 插件系统设计

> 目标: 支持第三方扩展，构建生态
> 优先级: P2
> 状态: 设计阶段

### 25.1 设计目标

- **扩展性**: 用户可自定义功能
- **隔离性**: 插件崩溃不影响核心
- **热更新**: 无需重启加载插件
- **安全性**: 限制插件权限

### 25.2 插件类型

| 类型 | 说明 | 示例 |
|------|------|------|
| **工具插件** | 扩展工具能力 | 数据库查询、API 调用 |
| **LLM 插件** | 扩展模型支持 | 新提供商适配 |
| **记忆插件** | 扩展存储方式 | 外部数据库 |
| **事件插件** | 扩展事件处理 | 日志收集、监控 |
| **UI 插件** | 扩展界面 | 自定义主题 |

### 25.3 插件接口

```go
// internal/plugin/plugin.go
package plugin

// Plugin 插件接口
type Plugin interface {
    // 元数据
    Name() string
    Version() string
    Description() string
    
    // 生命周期
    Init(config map[string]interface{}) error
    Start() error
    Stop() error
    
    // 能力注册
    RegisterTools(registry ToolRegistry) error
    RegisterHooks(hookManager HookManager) error
}

// Hook 机制
type HookManager interface {
    // 注册钩子
    RegisterHook(point HookPoint, handler HookHandler)
    
    // 触发钩子
    ExecuteHook(point HookPoint, ctx context.Context, data interface{}) error
}

type HookPoint string

const (
    HookBeforeRequest  HookPoint = "before_request"
    HookAfterResponse  HookPoint = "after_response"
    HookBeforeToolCall HookPoint = "before_tool_call"
    HookAfterToolCall  HookPoint = "after_tool_call"
    HookOnError        HookPoint = "on_error"
)
```

### 25.4 插件加载机制

```go
// internal/plugin/loader.go
package plugin

import (
    "plugin" // Go 原生 plugin 包（仅 Linux/macOS）
    // 或使用 RPC/HTTP 方式（跨平台）
)

type PluginLoader struct {
    pluginsDir string
    plugins    map[string]Plugin
}

// Load 加载插件
func (l *PluginLoader) Load(name string) (Plugin, error) {
    // 方案一：Go plugin（.so 文件，仅 Unix）
    // p, err := plugin.Open(path)
    
    // 方案二：独立进程 + RPC（推荐，跨平台）
    // 启动独立进程，通过 gRPC/HTTP 通信
    
    // 方案三：WASM（未来方向）
    // 加载 WASM 模块，沙箱执行
    
    return nil, nil
}
```

### 25.5 插件示例

```go
// 示例：数据库查询插件
package main

import (
    "database/sql"
    "fmt"
)

type DBPlugin struct {
    db *sql.DB
}

func (p *DBPlugin) Name() string { return "database" }
func (p *DBPlugin) Version() string { return "1.0.0" }

func (p *DBPlugin) Init(config map[string]interface{}) error {
    dsn := config["dsn"].(string)
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        return err
    }
    p.db = db
    return nil
}

func (p *DBPlugin) RegisterTools(registry ToolRegistry) error {
    registry.Register(Tool{
        Name:        "db_query",
        Description: "执行 SQL 查询",
        Parameters: []Parameter{
            {Name: "sql", Type: "string", Required: true},
        },
        Handler: p.handleQuery,
    })
    return nil
}

func (p *DBPlugin) handleQuery(ctx context.Context, args map[string]interface{}) (string, error) {
    sql := args["sql"].(string)
    rows, err := p.db.QueryContext(ctx, sql)
    // ...
    return result, nil
}
```

---

## 26. Skill 系统设计

> 目标: 封装专业流程，提升 Agent 效率
> 优先级: P1
> 状态: 设计阶段

### 26.1 什么是 Skill

**定义**: Skill 是 Agent 的"专业技能包"，封装了特定领域的知识、工具和流程。

**对比**:
| 概念 | 说明 |
|------|------|
| **Tool** | 单个功能（如：读文件、执行命令）|
| **Skill** | 完整流程（如：代码审查、Bug 修复、API 设计）|
| **Plugin** | 外部扩展（如：数据库连接、第三方 API）|

### 26.2 Skill 结构

```go
// internal/skill/skill.go
package skill

// Skill 技能定义
type Skill struct {
    ID          string
    Name        string
    Description string
    Category    string  // code_review, debugging, refactoring, etc.
    
    // 触发条件
    Triggers []Trigger
    
    // 执行流程
    Workflow Workflow
    
    // 知识库
    Knowledge []string  // 关联的知识文档
    
    // 系统提示词覆盖
    SystemPrompt string
    
    // 工具集
    Tools []string  // 需要的工具列表
    
    // 记忆配置
    Memory MemoryConfig
}

type Trigger struct {
    Type    string  // keyword, intent, pattern, manual
    Pattern string  // 匹配模式
}

type Workflow struct {
    Steps []Step
}

type Step struct {
    Name        string
    Description string
    Action      string      // llm, tool, condition, loop
    Config      interface{} // 步骤配置
}
```

### 26.3 内置 Skill 示例

**代码审查 Skill**:
```yaml
# skills/code_review.yaml
id: code_review
name: 代码审查
description: 自动审查代码变更，发现潜在问题
category: quality

triggers:
  - type: manual
    pattern: "/review"
  - type: intent
    pattern: "审查.*代码"

workflow:
  steps:
    - name: 获取变更
      action: tool
      config:
        tool: git_diff
        args: {staged: true}
    
    - name: 分析变更
      action: llm
      config:
        prompt: |
          审查以下代码变更：
          {{.diff}}
          
          检查：
          1. 潜在 bug
          2. 安全漏洞
          3. 性能问题
          4. 代码规范
    
    - name: 生成报告
      action: llm
      config:
        format: markdown

system_prompt: |
  你是一位资深代码审查员，擅长发现代码中的问题。
  审查时要严格但友善，给出具体改进建议。

tools:
  - git_diff
  - file_read
  - file_write

memory:
  persist_learnings: true  # 记住审查经验
```

### 26.4 Skill 执行流程

```
用户输入: "/review" 或 "帮我审查这段代码"

  │
  ▼
┌─────────────────┐
│ 1. 触发匹配      │ ← 匹配 Skill 的 triggers
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. 加载 Skill   │ ← 读取配置，准备上下文
│    注入知识库    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. 执行工作流    │ ← 按步骤执行
│    - 获取数据    │
│    - LLM 分析    │
│    - 生成结果    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. 学习进化      │ ← 保存经验到记忆
│    - 用户反馈    │
│    - 效果评估    │
└─────────────────┘
```

### 26.5 Skill 与现有系统的关系

```
┌─────────────────────────────────────────┐
│           Skill 层                       │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐  │
│  │代码审查  │ │Bug修复  │ │API设计  │  │
│  │Skill    │ │Skill    │ │Skill    │  │
│  └────┬────┘ └────┬────┘ └────┬────┘  │
│       └───────────┼───────────┘       │
│                   │                   │
│              ┌────┴────┐              │
│              │ Skill   │              │
│              │ 引擎    │              │
│              └────┬────┘              │
└───────────────────┼───────────────────┘
                    │
    ┌───────────────┼───────────────┐
    ▼               ▼               ▼
┌─────────┐   ┌─────────┐   ┌─────────┐
│ 工具层   │   │ LLM 层  │   │ 记忆层  │
│ (tools/)│   │ (llm/)  │   │(memory/)│
└─────────┘   └─────────┘   └─────────┘
```

### 26.6 Skill 自进化机制

```go
// internal/skill/evolution.go
package skill

// 从执行结果中学习，优化 Skill
func (s *Skill) Learn(execution *ExecutionResult, feedback UserFeedback) error {
    // 1. 分析执行效果
    effectiveness := s.evaluateEffectiveness(execution, feedback)
    
    // 2. 提取改进点
    improvements := s.extractImprovements(execution)
    
    // 3. 更新 Skill 配置
    if effectiveness < 0.7 {
        // 效果不佳，调整提示词或流程
        s.adjustPrompt(improvements)
        s.adjustWorkflow(improvements)
    }
    
    // 4. 保存学习记录
    s.saveLearningRecord(LearningRecord{
        SkillID:       s.ID,
        ExecutionID:   execution.ID,
        Effectiveness: effectiveness,
        Improvements:  improvements,
        Timestamp:     time.Now(),
    })
    
    return nil
}
```

### 26.7 三个系统的对比

| 维度 | 多模态 | 插件 | Skill |
|------|--------|------|-------|
| **目的** | 扩展输入能力 | 扩展功能 | 封装专业流程 |
| **用户** | 终端用户 | 开发者 | 终端用户 |
| **开发** | 核心团队 | 第三方 | 核心团队/社区 |
| **复杂度** | 中 | 高 | 中 |
| **优先级** | P2 | P2 | P1 |
| **实现难度** | 中 | 高 | 低 |
| **价值** | 扩展场景 | 生态建设 | 提升效率 |

### 26.8 推荐实现顺序

```
Phase 1 (现在): Skill 系统
  - 实现 Skill 引擎
  - 内置 3-5 个常用 Skill（代码审查、重构、测试生成）
  - 收益：立即提升用户体验

Phase 2 (1-2 周后): 多模态
  - 图像输入支持
  - 语音输入支持（可选）
  - 收益：扩展应用场景

Phase 3 (长期): 插件系统
  - 设计稳定的插件接口
  - 实现插件加载器
  - 收益：生态建设
```

---

> 本文档为设计草案，欢迎评审和补充。
