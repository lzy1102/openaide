# Hermes Agent 特性融合技术规格

## 概述

将 [Hermes Agent](https://github.com/NousResearch/hermes-agent) 的核心架构优势融合到 OpenAIDE 项目中,提升上下文管理、技能系统和记忆架构的可扩展性。

## 架构变更总览

### 阶段 1: Context Engine 可插拔接口 + 压缩增强

#### 1.1 Context Engine 抽象接口

**目标**: 将硬编码的 `contextManager` 改为可插拔的 `ContextEngine` 接口,支持多种压缩/摘要策略。

**新增文件**: `backend/src/services/context_engine.go`

**接口设计**:

```go
// ContextEngine 可插拔上下文引擎接口
type ContextEngine interface {
    // Compress 压缩对话历史,返回压缩后的上下文
    Compress(ctx context.Context, dialogueID string) (*CompressedContext, error)
    
    // Summarize 生成对话摘要
    Summarize(ctx context.Context, dialogueID string) (string, error)
    
    // ExtractImportantInfo 提取重要信息
    ExtractImportantInfo(ctx context.Context, dialogueID string) (map[string]interface{}, error)
    
    // ClearExpired 清理过期上下文
    ClearExpired(before time.Time) error
    
    // GetMetrics 获取上下文指标
    GetMetrics() *ContextMetrics
    
    // Name 返回引擎名称 (用于日志和配置)
    Name() string
}

// CompressionMode 压缩模式类型
type CompressionMode string

const (
    CompressionModeAggressive  CompressionMode = "aggressive"   // 激进压缩,丢失部分细节
    CompressionModeBalanced    CompressionMode = "balanced"     // 平衡模式(默认)
    CompressionModeLossless    CompressionMode = "lossless"     // 无损压缩,保留tool-call边界
    CompressionModeSmart       CompressionMode = "smart"        // 智能模式,基于内容自适应
)

// CompressionConfig 压缩配置
type CompressionConfig struct {
    Mode               CompressionMode
    MaxTokens          int           // 最大token数
    KeepLastN          int           // 保留最近N条消息不压缩
    PreserveToolCalls  bool          // 是否保留tool-call边界
    FallbackToSummary  bool          // 压缩失败时是否回退到摘要
}
```

**Hermes 5种压缩模式实现**:

| 模式 | 说明 | 适用场景 |
|------|------|---------|
| Aggressive | 激进摘要,保留关键信息 | 上下文严重超限时 |
| Balanced | 平衡摘要+关键tool-call保留 | 日常对话(默认) |
| Lossless | 保留所有tool-call边界 | 代码/调试任务 |
| Smart | 基于内容复杂度自适应 | 通用场景 |
| Fallback | 摘要失败兜底,至少告知模型"有内容被删了" | 容错机制 |

#### 1.2 原有 ContextManager 迁移

**变更文件**: `backend/src/services/context_manager.go`

- `contextManager` 结构体改为 `DefaultContextEngine`,实现 `ContextEngine` 接口
- 移除 `ContextManager` 接口,统一使用 `ContextEngine`
- 增强 `Compress` 方法: 添加工具调用边界感知、压缩质量验证

#### 1.3 压缩增强功能

**新增压缩策略**:

1. **孤立 tool results 清理**: 当 tool_result 没有对应的 tool_call 时,安全移除
2. **缺失 tool_calls 插入 stub**: 当 tool_call 没有对应 result 时,插入占位符
3. **摘要失败兜底**: 压缩失败时注入 `[以下内容已被压缩以节省空间]` 标记
4. **Tool-call 边界保留**: 确保 tool_call + tool_result 成对保留,不被截断拆分

### 阶段 2: 技能渐进式披露

#### 2.1 Skill 模型扩展

**变更文件**: `backend/src/models/skill.go`

**新增字段**:

```go
type Skill struct {
    // ... 现有字段保持不变 ...
    
    // 渐进式披露字段 (Hermes Agent 标准)
    Level0Summary    string    `json:"level0_summary" gorm:"type:text"`     // Level 0: 概要(~3000 tokens)
    Level1Content    string    `json:"level1_content" gorm:"type:text"`     // Level 1: 完整内容
    Level2References JSONSlice `json:"level2_references" gorm:"type:json"`  // Level 2: 参考材料路径
    
    // 技能自我进化字段
    UsageCount       int       `json:"usage_count" gorm:"default:0"`        // 使用次数
    SuccessRate      float64   `json:"success_rate"`                         // 成功率
    LastEvolvedAt    *time.Time `json:"last_evolved_at"`                     // 最后进化时间
    AutoEvolved      bool      `json:"auto_evolved" gorm:"default:false"`   // 是否自动进化
}
```

#### 2.2 渐进式加载逻辑

**变更文件**: `backend/src/services/skill_service.go`

**匹配流程**:

1. **初始匹配** → 仅加载 `Level0Summary` (约3000 tokens)
2. **确认执行** → 加载 `Level1Content` + 必要工具定义
3. **深度执行** → 按需加载 `Level2References` (参考文件/文档)

**Token 节省效果**: 相比一次性加载全部内容,预计减少 ~60% 系统提示词 token 消耗。

#### 2.3 技能自我进化机制

**变更文件**: `backend/src/services/skill_evolution_service.go`

**触发条件**:
- 技能执行成功次数 >= 5 次
- 成功率 >= 80%
- 距上次进化时间 >= 7 天

**进化动作**:
- 根据执行历史优化 `SystemPromptOverride`
- 补充 `Level2References` 参考材料
- 更新 `triggers` 关键词 (基于成功匹配的用户输入)

### 阶段 3: Memory Provider 插件化

#### 3.1 Memory Provider 接口增强

**变更文件**: `backend/src/services/memory_store.go`

**新增接口方法**:

```go
// MemoryProvider 记忆提供者插件接口 (扩展)
type MemoryProvider interface {
    MemoryStore // 继承基础存储接口
    
    // Initialize 初始化连接/客户端
    Initialize(config map[string]interface{}) error
    
    // Name 返回提供者名称
    Name() string
    
    // SemanticSearch 语义搜索 (向量相似度)
    SemanticSearch(ctx context.Context, userID, query string, limit int) ([]models.Memory, error)
    
    // BatchUpsert 批量更新/插入
    BatchUpsert(ctx context.Context, memories []models.Memory) error
    
    // Close 释放资源
    Close() error
}
```

#### 3.2 注册表模式

**新增结构**: `MemoryProviderRegistry`

```go
type MemoryProviderRegistry struct {
    providers map[string]func() MemoryProvider
    active    MemoryProvider
    mu        sync.RWMutex
}
```

**注册示例**:

```go
// 注册内置 GORM 提供者
registry.Register("gorm", func() MemoryProvider {
    return &GormMemoryProvider{}
})

// 未来可扩展 Redis/向量库 等提供者
// registry.Register("redis", func() MemoryProvider { return &RedisMemoryProvider{} })
// registry.Register("milvus", func() MemoryProvider { return &MilvusMemoryProvider{} })
```

### 阶段 4: 基于活动的超时 + 后台任务通知

#### 4.1 活动跟踪机制

**变更文件**: `backend/src/services/websocket_service.go`

**新增结构**:

```go
// ActivityTracker 基于活动的超时跟踪器
type ActivityTracker struct {
    mu               sync.RWMutex
    lastActivity     map[string]time.Time // sessionID -> 最后活动时间
    activityTimeout  time.Duration
    onTimeout        func(sessionID string)
}

// RecordActivity 记录活动 (工具调用/消息发送)
func (t *ActivityTracker) RecordActivity(sessionID string)

// ShouldTimeout 是否应超时
func (t *ActivityTracker) ShouldTimeout(sessionID string) bool
```

#### 4.2 后台任务通知

**新增模型**: `backend/src/models/task.go` 扩展

```go
type Task struct {
    // ... 现有字段 ...
    
    // 通知相关字段
    NotifyOnComplete bool      `json:"notify_on_complete"`
    NotifyWebhook    string    `json:"notify_webhook,omitempty"`
    CompletedAt      *time.Time `json:"completed_at,omitempty"`
}
```

**WebSocket 推送事件**:

```json
{
  "event": "task_completed",
  "task_id": "xxx",
  "status": "completed",
  "result": {...},
  "completed_at": "2026-04-17T..."
}
```

## 数据库迁移

### Migration 1: Skill 渐进式披露字段

```sql
ALTER TABLE skills ADD COLUMN level0_summary TEXT;
ALTER TABLE skills ADD COLUMN level1_content TEXT;
ALTER TABLE skills ADD COLUMN level2_references JSON;
ALTER TABLE skills ADD COLUMN usage_count INT DEFAULT 0;
ALTER TABLE skills ADD COLUMN success_rate FLOAT DEFAULT 0;
ALTER TABLE skills ADD COLUMN last_evolved_at TIMESTAMP;
ALTER TABLE skills ADD COLUMN auto_evolved BOOLEAN DEFAULT FALSE;
```

### Migration 2: 对话血缘追踪 (预留)

```sql
-- 为未来会话压缩血缘追踪预留
ALTER TABLE dialogues ADD COLUMN parent_dialogue_id VARCHAR(255);
ALTER TABLE dialogues ADD COLUMN compression_level INT DEFAULT 0;
ALTER TABLE dialogues ADD COLUMN lineage JSON;
```

## 兼容性要求

- **向后兼容**: 所有新增字段必须有默认值或为 nullable
- **GORM AutoMigrate**: 使用 `db.AutoMigrate()` 确保新字段自动创建
- **API 响应**: 新增字段在 JSON 响应中默认不暴露敏感内部状态
- **配置开关**: 每个新特性可通过 `server_config.json` 开关控制

## 测试策略

| 测试类型 | 覆盖范围 |
|---------|---------|
| 单元测试 | ContextEngine 接口实现、压缩算法、参数强制转换 |
| 集成测试 | Skill 渐进式加载流程、Memory Provider 切换 |
| E2E 测试 | 完整的 skill 匹配 → 执行 → 进化流程 |
