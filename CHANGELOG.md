# Changelog

本项目的所有重要变更都记录在此文件。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

### Added

- `core`: `buildToolInventory()` —— `[Available Tools]` 段从运行时工具表生成，
  提示词不再硬编码工具名（插件改名不会导致模型调到不存在的工具）。
- `core`: `filterToolsByAllowlist()` 提升为公共实现，`react.ts` 与 `kernel.ts` 共用同一
  白名单口径，保证"列出的工具"与"可调用的工具"永远是同一集合。
- `core`: `SAFETY_FLOOR` —— 恒定注入、人格不可关闭的安全底线。
- `core`: `ToolRegisterOptions` + `ToolExecutor.isDangerous()` —— 危险标记走注册元数据，
  不再靠描述前缀嗅探；标记不进入发给 LLM 的 `tools` 载荷。
- `core`: `KernelConfig.parallelTools` —— 同一轮的工具调用是否并行（默认 `true`）。
- `tools`: `grep_search` 新增 `max_results` 参数；`include` 支持 `*` / `?` / `**` 通配符。
- 测试：`core/test/react.test.ts`、`llm/test/cache.test.ts`、`plugins/test/builtin-personas.test.ts`、
  `tools/test/{builtin,registry,file-tools}.test.ts`。

### Changed

- `core`: 所有 system 消息（`[Task]`、`[ProjectKnowledge]`）前置于历史与用户消息。
  旧实现追加在 user 之后，会被多数模型忽略甚至拒收。
- `core`: 同一轮内多个工具调用并行执行；权限检查与审批拦截链仍串行，避免并发弹窗。
- `core`: 上下文压缩的摘要保留工具链语义 —— 带 `tool_calls` 的 assistant 渲染为
  `assistant [called: name(args)]`，工具结果渲染为 `tool(name) result: ...`。
- `core`: `process` / `processStream` 的公共前置与持久化逻辑收敛为 `prepare()` /
  `persistAssistant()` / `reactConfig()`。
- `cli`: `coder` 人格不再指示 agent 自动执行 `git add -A && git commit`；
  改为先看 `git status`、只暂存自己改动的路径、未经明确要求不提交。
- `llm`: `cache_control` 断点只落在最后一条 system 消息上（旧实现给每条都打）。
- `plugins`: `dangerous` 工具的描述前缀保留（供 LLM 参考），但审批判定改用注册元数据。
- `api`: HTTP 错误码分类 —— 413（payload 过大）/ 400（非法 JSON）/ 499（中止）/ 500（内部故障）。

### Fixed

- `core`: CJK 任务类型检测 —— JS 正则 `\b` 对中文恒不匹配，中文查询此前全部落到 `general`。
- `core`: 工具调用失败时 `error` 现在会写入 tool 消息。此前失败调用的 `content` 为空，
  模型只看到一条空观察，既不知道原因也无法纠正。
- `core`: 轮数耗尽不再"静默完成" —— 归还最后一轮文本并产出 `Error` chunk；
  `process()` 在有错误且无内容时抛错，而非返回空内容。
- `core`: `process()` 抛错后内核不再卡在 `processing` 状态（补 `try/finally`）。
- `tools`: 危险命令黑名单 —— 修掉字面量 `"curl ... | sh"` 永不命中、以及子串匹配误杀
  `firmware` / `confirm` / `transform` / `git add -A` / `npm run format` 的问题；
  改为 token 全等 + 前缀 + 序列 + 正则四层判定。
- `tools`: `diff_edit` 不再把 replacement 里的 `$&` / `$1` / `` $` `` 当替换模式展开。
- `tools`: `diff_edit` 支持 LF↔CRLF 双向归一（Windows 文件此前几乎无法编辑）。
- `tools`: `grep_search` 不再跳过所有点开头条目（`.env` / `.gitignore` / `.github` 是盲区）。
- `cli`: 子代理可见面屏蔽全部 `subagents__*`，阻断无限自我委派（`definitions()` 不暴露 +
  `execute()` 二次拦截）。

### Deprecated

- `core`: `PromptContext.reflection`、`PromptContext.skillMessage`、`reflectionPrompt()`
  已移除 —— 全仓库无调用方；`SKILL.md` 实际被解析为 persona，与 skillMessage 无关。

### Removed

- `core`: 无用的 `toWireMessages(messages, tools)` 第二参数。

## 说明

- `0.3.3` 之前的变更未在本文件中追溯，参见 git 历史。
