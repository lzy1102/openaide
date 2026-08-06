---
# 最小可用示例：只需要 name + description 两个字段即可被加载和自动匹配。
# name 必填；description 决定 LLM 何时命中本技能。
name: git-cleanup
description: 清理本地已合并的 git 分支
allowed-tools:
  - Bash
  - Grep
---

用 `git branch --merged` 找出已合并分支，逐条确认后删除。

规则：
- 跳过 main/master 分支，永不删除当前所在分支
- 删除前先向用户列出将要删除的分支，获得确认
- 使用 `git branch -d`（安全删除），不用 `-D`
