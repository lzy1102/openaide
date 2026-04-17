# 实施任务清单

## 阶段 1: Context Engine 可插拔接口 + 压缩增强

### 任务 1.1: 创建 ContextEngine 接口定义
- [ ] 新建文件: `backend/src/services/context_engine.go`
- [ ] 定义 `ContextEngine` 接口 (Compress/Summarize/ExtractImportantInfo/ClearExpired/GetMetrics/Name)
- [ ] 定义 `CompressionMode` 枚举 (Aggressive/Balanced/Lossless/Smart)
- [ ] 定义 `CompressionConfig` 配置结构体
- [ ] 实现 `DefaultContextEngine` 结构体,将现有 `contextManager` 迁移至此

### 任务 1.2: 增强压缩算法 (Hermes 5模式)
- [ ] 实现 `compressAggressive()` - 激进摘要模式
- [ ] 实现 `compressBalanced()` - 平衡模式(默认)
- [ ] 实现 `compressLossless()` - 无损模式,保留tool-call边界
- [ ] 实现 `compressSmart()` - 智能自适应模式
- [ ] 实现 `fallbackSummary()` - 摘要失败兜底
- [ ] 实现 `cleanIsolatedToolResults()` - 清理孤立tool results
- [ ] 实现 `insertToolCallStubs()` - 缺失tool_calls插入stub

### 任务 1.3: 更新依赖注入
- [ ] 修改 `main.go` 中的服务初始化逻辑
- [ ] 更新 `config.go` 添加压缩模式配置项
- [ ] 确保现有 handler 使用新接口

## 阶段 2: 技能渐进式披露

### 任务 2.1: 扩展 Skill 数据模型
- [ ] 修改 `backend/src/models/skill.go`
- [ ] 添加 `Level0Summary` 字段 (text)
- [ ] 添加 `Level1Content` 字段 (text)
- [ ] 添加 `Level2References` 字段 (JSON)
- [ ] 添加 `UsageCount` 字段 (int, default 0)
- [ ] 添加 `SuccessRate` 字段 (float)
- [ ] 添加 `LastEvolvedAt` 字段 (timestamp)
- [ ] 添加 `AutoEvolved` 字段 (bool, default false)

### 任务 2.2: 实现渐进式加载逻辑
- [ ] 修改 `backend/src/services/skill_service.go`
- [ ] 实现 `GetSkillLevel0()` - 仅返回概要
- [ ] 实现 `GetSkillLevel1()` - 返回完整内容
- [ ] 实现 `GetSkillLevel2()` - 返回参考材料
- [ ] 修改 `MatchSkill()` - 初始匹配只加载Level0
- [ ] 修改 `ExecuteSkillWithContent()` - 执行时加载Level1

### 任务 2.3: 技能自我进化服务
- [ ] 新建文件: `backend/src/services/skill_evolution_service.go`
- [ ] 实现 `EvaluateSkillPerformance()` - 评估技能表现
- [ ] 实现 `EvolveSkill()` - 根据执行历史优化技能
- [ ] 实现 `RunPeriodicEvolution()` - 定时执行进化检查
- [ ] 在 `main.go` 中注册定时任务

## 阶段 3: Memory Provider 插件化

### 任务 3.1: 扩展 MemoryStore 接口
- [ ] 修改 `backend/src/services/memory_store.go`
- [ ] 新增 `MemoryProvider` 接口 (继承 MemoryStore)
- [ ] 添加 `Initialize()` 方法
- [ ] 添加 `Name()` 方法
- [ ] 添加 `SemanticSearch()` 方法
- [ ] 添加 `BatchUpsert()` 方法
- [ ] 添加 `Close()` 方法

### 任务 3.2: 实现提供者注册表
- [ ] 新增结构: `MemoryProviderRegistry`
- [ ] 实现 `Register()` 方法
- [ ] 实现 `GetProvider()` 方法
- [ ] 实现 `SetActiveProvider()` 方法
- [ ] 注册内置 GORM 提供者

### 任务 3.3: GormMemoryProvider 实现
- [ ] 将 `GormMemoryStore` 重构为 `GormMemoryProvider`
- [ ] 实现 `MemoryProvider` 接口所有方法
- [ ] 添加配置文件支持

## 阶段 4: 基于活动的超时 + 后台任务通知

### 任务 4.1: 实现 ActivityTracker
- [ ] 修改 `backend/src/services/websocket_service.go`
- [ ] 新增 `ActivityTracker` 结构体
- [ ] 实现 `RecordActivity()` 方法
- [ ] 实现 `ShouldTimeout()` 方法
- [ ] 实现 `CleanupExpiredSessions()` 方法

### 任务 4.2: 后台任务通知系统
- [ ] 修改 `backend/src/models/task.go` (如存在) 或新建
- [ ] 添加 `NotifyOnComplete` 字段
- [ ] 添加 `NotifyWebhook` 字段
- [ ] 添加 `CompletedAt` 字段
- [ ] 实现 WebSocket 事件推送
- [ ] 实现 HTTP Webhook 回调

### 任务 4.3: 集成到现有服务
- [ ] 修改 `backend/src/services/task_service.go` (如存在)
- [ ] 在任务完成时触发通知
- [ ] 在 WebSocket handler 中注册事件监听
