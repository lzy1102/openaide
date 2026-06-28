# Prompt Architecture

> 版本: v3.2.0 | 更新: 2026-06-26

## 设计原则

**指令预算有限**。研究表明 thinking 模型在 ~150-200 条指令后遵守率开始均匀衰减——加一条新规则，已有规则的遵守率也下降。非 thinking 模型衰减更快（Claude Code internal research, 2025）。

因此设计目标：**L0 ≤ 30 条核心原则，L3 只补充不重复**。

## 分层模型

```
L0: 宪法      (~400 tokens, always loaded, cached)
  只有"违反就无法正常工作"的原则
  - 接地协议（Grounding Protocol）
  - 工具安全红线
  - 确定性标注规则
  - 核心交互约定

L1: 项目上下文 (~200 tokens, auto-generated)
  - 工作目录、Git 分支
  - CLAUDE.md 等规则文件
  - RepoMap 符号地图
  - 语言特定约定

L2: 技能注入  (~100-500 tokens, SkillActor 动态注入)
  - 匹配到的技能的 prompt

L3: 任务适配  (~300-500 tokens, per-query dynamic)
  - coding: 4 阶段工作流（只讲差异化步骤）
  - review: 防误报规则 + 输出格式
  - think: 探索深度自适应
  - general: 空（L0+L1 足够）

L4: 知识注入  (~50-200 tokens, RAG)
  - 知识库检索到的相关文档

L5: 上次反思  (~100 tokens, LLMReflection result)
  - 质量评分 + 改进建议
```

## 层间协议

1. **L0 是唯一真理源**。所有通用规则（读后写、不猜测、验证）只在 L0 出现
2. **L3 只加差异化内容**。不能重复 L0 的规则，不能引入新标签体系
3. **确定性标注全局统一**。只用 `[verified] [inferred] [assumed]`，L3 不再定义自己的标签
4. **每条指令有且仅有一处定义**。宁可少说，不说两遍

## 参考文献

- Anthropic. "Architecture and Production Patterns of Autonomous Coding Agents." ZenML LLMOps Database, 2025.
- Arize AI. "CLAUDE.md: Best Practices Learned from Optimizing Claude Code with Prompt Learning." 2025.
- Aider. "Separating code reasoning and editing." aider.chat, 2024.
- Cline. System prompt source. github.com/cline/cline, 2025.

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-06-26 | 初始版本：L0 剪枝至 30 条，L3 去重，标签统一 |
