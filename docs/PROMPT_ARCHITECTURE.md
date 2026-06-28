# Prompt Architecture

> 版本: v3.4.0 | 更新: 2026-06-28

## 设计原则

**指令预算有限**。研究表明 thinking 模型在 ~150-200 条指令后遵守率开始均匀衰减。非 thinking 模型衰减更快。

**解决**：L0 包含所有核心规则（~30 条），L3 只发模式信号（2-3 行）。无层间重复。kernel 层面自动验证。

## 分层模型

```
L0: 核心规则   (~600 tokens, always loaded, cached)
  ├── Hard Blocks（8 条绝对禁止）
  ├── Grounding Protocol（4 条接地规则）
  ├── Certainty Labels（3 级确定性标注）
  ├── Coding Workflow（Read→Plan→Execute→Verify，含数据迁移检查）
  ├── Engineering Checklist（测试可运行性、配置、CI/CD、数据兼容、覆盖率）
  ├── Review Mode（审查标签 + 防误报 + 输出格式）
  ├── Debugging Mode（复现→假说→锁定→验证）
  ├── Interaction（5 条交互约定）
  └── Learning & Memory（4 条学习机制）

L1: 项目上下文 (~200 tokens, auto-generated)

L2: 技能注入  (~100-500 tokens, SkillActor 动态注入)

L3: 模式信号 (~50 tokens, per-query dynamic)
  - coding: "Follow Coding Workflow + Engineering Checklist"
  - review: "Follow Review Mode. Output in [P0/P1/P2] format."
  - think: "Use certainty labels. Be specific."
  - debugging: "Follow Debugging Mode."
  - general: 空

L4: 知识注入  (~50-200 tokens, RAG)

L5: 上次反思  (~100 tokens, LLMReflection result)

Auto-Verify: (kernel, post-ReAct)
  - 检测项目测试命令 → 自动执行 → 失败注入回 ReAct（最多 3 轮）
```

## 关键设计决策

### L3 从完整规则降为模式信号

**之前**：L3 包含完整的工作流描述（coding 3210 chars, review 2500 chars），与 L0 有大量重复。

**现在**：L3 每个模式 2-3 行，只发"激活信号"。所有真实规则在 L0。

**原因**：
1. LLM 不"组合"层——L3 的指令会覆盖 L0，而非补充
2. 任务分类不可靠——"why is auth broken?" 可能分到 think 或 coding
3. 层间重复浪费 20% token 预算
4. 研究显示负面约束（Hard Blocks）比正面指引遵守率高 2-3 倍

### Hard Blocks vs 正面指引

**负面约束**（Hard Blocks）产生 70%+ 遵守率：
- "Never claim facts about unread code"
- "Never truncate code with '...'"

**正面指引**（Soft Guidelines）产生 30-40% 遵守率：
- "Be concise"
- "Match existing patterns"

因此 L0 优先使用负面约束格式——提升整体遵守率。

### Engineering Checklist

L0 新增工程检查清单——修改代码时主动检查：
- 测试能否运行？测试值与代码一致？
- strict 模式、硬编码路径、CI/CD
- 数据向后兼容性
- 测试覆盖缺口

这弥补了 LLM "代码逻辑对但工程基础设施坏"的盲区。

### Auto-Verification（kernel 层面）

ReAct 循环结束后、finalizeResponse 之前：
1. 检测项目类型（go.mod → `go test`，package.json → `npm test` 等）
2. 自动执行测试
3. 失败 → 注入用户消息 → 重入 ReAct（最多 3 轮）
4. 通过 → 正常完成

LLM 无法跳过——kernel 强制执行。

## 参考文献

- Anthropic. "Architecture and Production Patterns of Autonomous Coding Agents." ZenML LLMOps Database, 2025.
- arXiv:2604.11088. "Negative constraints more effective than positive directives." 2026.
- Arize AI. "CLAUDE.md: Best Practices Learned from Optimizing Claude Code with Prompt Learning." 2025.
- Aider. "Separating code reasoning and editing." aider.chat, 2024.

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-06-28 | v3.4.0: Engineering Checklist + Auto-Verification。Plan 步骤加数据迁移检查。 |
| 2026-06-26 | v3.2.0: 初始版本。L0 剪枝至 22 条，L3 去重，标签统一。 |
