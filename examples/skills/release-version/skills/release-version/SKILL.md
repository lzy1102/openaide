---
# name: 技能唯一 ID，也是匹配后返回给内核的标识。必填，为空则该文件解析失败。
name: release-version
# description: 自动匹配的核心依据。统一查询分析和 LLM 检测都只读它来判断是否命中，
# 必须准确描述"什么时候用这个技能"。建议写明触发场景。
description: 按语义化版本规范执行版本发布，包括更新版本号、生成 changelog、打 git tag
# argument-hint: 可选。给用户/调用方的参数提示，说明调用技能时需要提供什么输入。
argument-hint: 版本号类型（major/minor/patch），如 "patch"
# allowed-tools: 可选。工具白名单——命中后 LLM 只能看到这些工具。
# 使用 Claude 生态工具名，会被自动映射到 OpenAIDE 工具：
#   Read→read_file, Write→write_file, Edit→diff_edit, Glob/Grep→search_files,
#   Bash→execute_command, WebSearch→web_search, WebFetch→web_fetch,
#   Task→execute_command, AskUserQuestion→ask_user, List→list_directory,
#   TodoWrite→todo_write, LSP→lsp_definition
# 未识别或留空 = 不限制工具（LLM 可用全部已注册工具）。
allowed-tools:
  - Read
  - Edit
  - Grep
  - Bash
  - TodoWrite
---

# 版本发布流程

## 步骤
1. 读取当前版本：执行 `git describe --tags --abbrev=0`，解析出当前版本号
2. 根据用户指定的类型（major/minor/patch）计算新版本号
   - major：`X.0.0`（破坏性变更）
   - minor：`X.Y.0`（新功能）
   - patch：`X.Y.Z+1`（修复）
3. 用 Edit 更新 `version.txt` 与 `package.json` 中的版本号
4. 用 Bash 生成 changelog 片段：`git log --oneline <old_tag>..HEAD`
5. 用 Bash 提交并打 tag：`git tag -a v<新版本> -m "release v<新版本>"`
6. 用 TodoWrite 记录完成项

## 规则
- 严格遵循 SemVer：breaking change → major，新功能 → minor，修复 → patch
- 禁止直接 push；tag 打好后询问用户是否推送
- 每次只发布一个版本，发布失败时回滚到步骤 3 之前的状态
