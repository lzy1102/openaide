# OpenAIDE 完整功能测试套件

> 版本: v0.2.0+ | 日期: 2026-06-23 | 预计时间: 2-3 小时

---

## 测试环境准备

```bash
# 确保二进制是最新的
make build && cp bin/openaide ~/.openaide/bin/

# 清理测试数据（可选，全新测试体验）
rm -rf ~/.openaide/data/
```

---

## 一、基础功能 (Basics)

### 1.1 版本信息

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| B1 | 版本号 | `openaide --version` | 显示 `OpenAIDE CLI v0.2.0-xxx (built 2026-...)` | ☐ |
| B2 | 帮助 | `openaide --help` | 显示所有命令列表，含 `server` | ☐ |
| B3 | 帮助 (简写) | `openaide -h` | 同 --help | ☐ |
| B4 | 版本 (简写) | `openaide -v` | 同 --version | ☐ |

### 1.2 构建验证

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| B5 | 构建 | `make build` | 成功，输出 `bin/openaide` | ☐ |
| B6 | 单元测试 | `make test` | 26 个包全部 PASS | ☐ |
| B7 | 竞态检测 | `go test -race ./internal/...` | 无竞态报告 | ☐ |
| B8 | 静态分析 | `go vet ./...` (backend 目录) | 无警告 | ☐ |

### 1.3 配置

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| B9 | 配置向导 | `openaide setup` | 交互式：语言→提供商→API Key→模型 | ☐ |
| B10 | 配置热重载 | 修改 `~/.openaide/config.yaml` 的 `max_tokens`，再运行 `openaide` | 新配置生效，无需重启 | ☐ |
| B11 | 无效配置 | 清空 config.yaml 的 api_key，运行 `openaide "hello"` | 显示警告，使用默认配置继续 | ☐ |

---

## 二、CLI 模式 (CLI Modes)

### 2.1 一次性问答 (One-shot)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| C1 | 简单问答 | `openaide "hello"` | 返回问候，不进入 REPL | ☐ |
| C2 | 中文问答 | `openaide "这个项目是做什么的"` | 用中文回答项目功能 | ☐ |
| C3 | 带文件 | `openaide README.md "总结这个文档"` | 读取 README.md 并总结 | ☐ |
| C4 | 多文件 | `openaide README.md README.zh.md "比较两个文档"` | 读取两个文件并比较 | ☐ |
| C5 | 代码生成 | `openaide "写一个 Go 的 hello world"` | 输出可运行的 Go 代码 | ☐ |
| C6 | JSON 输出 | `openaide --output json "hello"` | 输出 JSON 格式 `{"content": "..."}` | ☐ |
| C7 | 指定模型 | `openaide --model deepseek-v4-flash "hi"` | 使用 flash 模型响应 | ☐ |
| C8 | 调试模式 | `openaide --verbose "测试"` | 显示 debug 级别日志 | ☐ |
| C9 | 日志级别 | `openaide --log-level warn "测试"` | 只显示 warn 级别以上日志 | ☐ |
| C10 | 超时 | `openaide "写一个 10 万行代码的项目"` | 300s 超时后退出 | ☐ |
| C11 | Ctrl+C 中断 | 执行 `openaide "大任务"`，按 Ctrl+C | 优雅退出，不残留进程 | ☐ |

### 2.2 交互式 REPL

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| C12 | 进入 REPL | `openaide` | 显示欢迎信息 + 提示符 | ☐ |
| C13 | 基本对话 | 输入 "hello" | 正常回复 | ☐ |
| C14 | 多轮对话 | 连续输入 3 个不同问题 | 上下文连贯 | ☐ |
| C15 | 退出 | 输入 `exit` 或 `quit` 或 Ctrl+D | 正常退出 | ☐ |
| C16 | /clear | 输入 "/clear" | 清空当前会话，开始新会话 | ☐ |
| C17 | 恢复会话 | `openaide -c` | 恢复上次未完成的会话 | ☐ |
| C18 | 自动审批 | `openaide -y "创建 test.txt 写入 hello"` | 自动批准工具操作，不弹出审批 | ☐ |
| C19 | Markdown 渲染 | 在 REPL 中问 "列出 Go 语言的特性" | 代码块带语法高亮，表格格式正确 | ☐ |
| C20 | 工具可见性 | 在 REPL 中问 "读取 CLAUDE.md" | 终端显示 ⚙ read_file 工具调用状态 | ☐ |

---

## 三、Server 模式 (Web Server)

### 3.1 服务启动

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| S1 | 启动服务 | `openaide server` | 服务启动，监听端口 | ☐ |
| S2 | 自定义配置 | `openaide server --config /path/to/config.yaml` | 使用指定配置启动 | ☐ |
| S3 | 端口占用 | 先启动一个 server，再启动第二个 | 第二个报错端口被占用 | ☐ |
| S4 | 优雅关闭 | 启动 server，按 Ctrl+C | 日志显示 "Shutting down..." → "Goodbye!" | ☐ |

### 3.2 Web 前端

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| S5 | 首页加载 | 浏览器访问 `http://localhost:8080` | 显示 Web 前端界面 | ☐ |
| S6 | SPA 路由 | 访问 `http://localhost:8080/chat` | 正确显示聊天页面 | ☐ |
| S7 | 设置页面 | 访问 `http://localhost:8080/settings` | 显示设置页面 | ☐ |
| S8 | 仪表盘 | 访问 `http://localhost:8080/dashboard` | 显示仪表盘 | ☐ |

### 3.3 API 端点

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| S9 | 健康检查 | `curl http://localhost:8080/health` | 返回 200 OK | ☐ |
| S10 | 指标 | `curl http://localhost:8080/metrics` | 返回 Prometheus 格式指标 | ☐ |
| S11 | 同步聊天 | `curl -X POST http://localhost:8080/api/v1/chat -H 'Content-Type: application/json' -d '{"query":"hi"}'` | 返回 200 + JSON 响应 | ☐ |
| S12 | 流式聊天 | `curl -N -X POST http://localhost:8080/api/v1/chat/stream -H 'Content-Type: application/json' -d '{"query":"hi"}'` | SSE 流式输出 | ☐ |
| S13 | 会话列表 | `curl http://localhost:8080/api/v1/sessions` | 返回 JSON 会话列表 | ☐ |
| S14 | 会话详情 | `curl http://localhost:8080/api/v1/sessions/{id}` | 返回特定会话详情 | ☐ |
| S15 | 工具列表 | `curl http://localhost:8080/api/v1/tools` | 返回所有工具定义 | ☐ |
| S16 | 系统统计 | `curl http://localhost:8080/api/v1/stats` | 返回统计信息 | ☐ |
| S17 | CORS | 从不同域发起 API 请求 | 正确返回 CORS 头 | ☐ |

---

## 四、核心 Agent 能力 (Core Agent)

### 4.1 任务类型识别

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A1 | Coding 模式 | `openaide "fix the typo in CLAUDE.md: 'openaide' should be 'openaide'"` | 进入编码模式，使用工具修复 | ☐ |
| A2 | Review 模式 | `openaide "review CLAUDE.md for bugs"` | 进入审查模式，输出 P0/P1/P2 问题 | ☐ |
| A3 | Think 模式 | `openaide "explain how Go's CSP works"` | 进入思考模式，输出分析性回答 | ☐ |
| A4 | General 模式 | `openaide "hello"` | 简短问候回复 | ☐ |

### 4.2 工具调用

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A5 | 读取文件 | `openaide "读取 README.md 的内容"` | 调用 read_file，显示文件内容 | ☐ |
| A6 | 列出目录 | `openaide "列出 backend/internal 下的所有目录"` | 调用 list_directory | ☐ |
| A7 | 搜索文件 | `openaide "查找项目中所有包含 'AgentKernel' 的文件"` | 调用 search_files | ☐ |
| A8 | 写入文件 | `openaide "创建 /tmp/test_openaide.txt 写入 hello world"` | 调用 write_file，文件已创建 | ☐ |
| A9 | Git 状态 | `openaide "用 git status 查看当前状态"` | 调用 git_status，显示状态 | ☐ |
| A10 | Git diff | `openaide "用 git diff 查看最近的改动"` | 调用 git_diff | ☐ |
| A11 | Git log | `openaide "用 git log 看最近 10 条 commit"` | 调用 git_log | ☐ |
| A12 | Web 搜索 | `openaide "搜索 Go 1.26 的新特性"` | 调用 web_search | ☐ |
| A13 | 网页抓取 | `openaide "抓取 https://go.dev 的内容并总结"` | 调用 web_fetch | ☐ |
| A14 | diff_edit | `openaide "把 /tmp/test_openaide.txt 中的 hello 改成 world"` | 调用 diff_edit，显示 before/after | ☐ |
| A15 | 执行命令 | `openaide "运行 ls -la"` | **弹出审批**，批准后执行成功 | ☐ |
| A16 | 审批拒绝 | A15 中选择 "Deny" | 工具被拒绝，显示拒绝原因 | ☐ |
| A17 | 审批全部允许 | A15 中选择 "Allow All" | 后续工具调用自动批准 | ☐ |
| A18 | 并行工具 | `openaide "同时读取 README.md 和 CLAUDE.md"` | 并行调用 read_file，更快完成 | ☐ |
| A19 | 危险工具 | `openaide "执行 rm -rf /"` | 弹出审批，需要确认 | ☐ |

### 4.3 知识库 (Knowledge)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A20 | 知识检索 | `openaide "我们之前讨论过哪些优化？"` | 检索知识库中的历史记录 | ☐ |
| A21 | 知识积累 | 连续问 3 个相关的问题，第 4 个问 "总结我们讨论过的内容" | 知识积累后能回忆之前的讨论 | ☐ |
| A22 | 知识搜索 | 在 REPL 中，AI 使用 search_knowledge 工具 | 返回知识库匹配结果 | ☐ |

### 4.4 记忆管理 (MemGPT)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A23 | 会话记忆 | 在一个会话中告诉 AI 你的名字，然后问 "我叫什么名字" | 记住上下文中的名字 | ☐ |
| A24 | 跨会话记忆 | 会话 A 中告诉 AI 偏好，退出后 `openaide -c` 问 "我的偏好吗" | 恢复会话后记住偏好 | ☐ |
| A25 | 归档 | 在长对话后，AI 使用 `manage_memory(action='archive')` | 对话被归档存储 | ☐ |
| A26 | 检索 | 在新会话中问之前归档的内容 | 能检索到之前归档的信息 | ☐ |
| A27 | 核心记忆 | AI 使用 `manage_memory(action='remember')` 存储事实 | 核心事实持久化 | ☐ |
| A28 | 上下文压缩 | 超过 4000 token 的大对话 | 自动压缩旧消息为摘要 | ☐ |

### 4.5 反思与学习 (Reflection)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A29 | 任务反思 | `openaide "重构 CLAUDE.md，让它更简洁"` | 完成后自动反思，记录质量评分 | ☐ |
| A30 | 技能提取 | 连续 5 次问类似的问题（如 "解释 Go 的接口" → "Go 接口示例" → ...） | distill 触发，自动创建技能 | ☐ |
| A31 | 用户反馈 | 在 REPL 中对回答表示满意 ("good") | 知识权重提升 | ☐ |
| A32 | 用户反馈 (差) | 在 REPL 中对回答表示不满 ("bad") | 知识权重降低 | ☐ |

### 4.6 审查功能

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| A33 | 代码审查 | `openaide "review backend/internal/kernel/kernel.go for bugs"` | 输出 P0/P1/P2 问题，带行号和建议 | ☐ |
| A34 | BUG 检测 | `openaide "review backend/internal/kernel/actor/actor.go"` | 有 BUG 时说清触发条件 | ☐ |
| A35 | 无 bug | `openaide "review backend/internal/kernel/types.go for bugs"` | 无 bug 时说 "No bugs found" | ☐ |

---

## 五、Team 模式 (Multi-Agent Team)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| T1 | 自动 Team | `openaide "设计并实现一个简单的缓存系统"` | 自动激活 Team 模式 | ☐ |
| T2 | /analyst | REPL 中输入 "/analyst 分析 kernel.go 的架构" | analyst 角色分析 | ☐ |
| T3 | /coder | REPL 中输入 "/coder 创建一个 hello.go" | coder 角色编码 | ☐ |
| T4 | /reviewer | REPL 中输入 "/reviewer 审查刚创建的代码" | reviewer 角色审查 | ☐ |
| T5 | /executor | REPL 中输入 "/executor 运行 make test" | executor 角色执行 | ☐ |
| T6 | /team | REPL 中输入 "/team 实现一个 TODO 应用" | 多角色协作完成 | ☐ |
| T7 | /auto | REPL 中输入 "/auto" | 切换回自动模式 | ☐ |
| T8 | 子 Agent 隔离 | Team 模式中观察子 Agent | 子 Agent 有独立会话，主 Agent 只看到结果摘要 | ☐ |
| T9 | 角色生成 | 复杂任务触发 `GenerateRoles` | LLM 自定义角色，非固定 4 角色 | ☐ |

---

## 六、插件系统 (Plugins)

### 6.1 插件管理

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| P1 | 列出插件 | `openaide plugins` | 列出已安装插件 | ☐ |
| P2 | 搜索插件 | `openaide plugins search` | 显示搜索指引 | ☐ |
| P3 | 安装插件 | `openaide plugins install https://github.com/anthropics/claude-plugins-official.git` | 下载并安装 | ☐ |
| P4 | 插件热加载 | 复制一个新插件目录到 `./data/plugins/` | 无需重启，自动发现 | ☐ |

### 6.2 Claude 插件兼容

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| P5 | Skill 注册 | 安装含 `skills/*/SKILL.md` 的插件 | 技能注册为 `/skill-name` | ☐ |
| P6 | MCP 工具 | 配置 `.mcp.json` 的 MCP server | 工具以 `mcp_` 前缀出现在工具列表 | ☐ |
| P7 | Hooks 触发 | 配置 `hooks/hooks.json` | 事件触发时执行对应命令 | ☐ |
| P8 | 插件详情 | `openaide --verbose 2>&1 \| grep plugin` | 日志显示插件发现信息 | ☐ |

---

## 七、会话管理 (Session Management)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| M1 | 列出会话 | `openaide sessions` | 列出历史会话 ID 和预览 | ☐ |
| M2 | 自动创建 | `openaide "测试"` | 自动创建新会话 | ☐ |
| M3 | 会话恢复 | `openaide -c` | 恢复最近非空会话 | ☐ |
| M4 | 会话标题 | 在 REPL 中对话 | 自动生成 3-5 字会话标题 | ☐ |
| M5 | 会话清理 | 系统运行 `CleanupOldSessions` | 自动清理过期会话 | ☐ |
| M6 | 会话持久化 | 退出 REPL 后重启 `openaide -c` | 会话数据完整恢复 | ☐ |
| M7 | 多会话并发 | 两个终端同时使用 openaide | 各自独立会话，不冲突 | ☐ |

---

## 八、自我更新 (Self-Update)

| # | 测试场景 | 命令 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| U1 | 更新检查 | `openaide update` | 检查并下载最新版本 | ☐ |
| U2 | 本地更新 | `openaide update --local` | 使用本地脚本更新 | ☐ |
| U3 | 无更新脚本 | 删除 `~/.openaide/install.sh` 后运行 `openaide update` | 显示 "script not found" | ☐ |

---

## 九、错误处理 (Error Handling)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| E1 | 无效 API Key | 配置错误的 API Key | 显示友好的 401 错误提示 | ☐ |
| E2 | 配额耗尽 | 触发 API quota 耗尽 | 显示 429 提示和重置时间 | ☐ |
| E3 | 网络超时 | 断开网络后执行 `openaide "test"` | 显示超时错误 | ☐ |
| E4 | 无效命令 | `openaide --unknown-flag` | 显示帮助信息 | ☐ |
| E5 | 不存在的文件 | `openaide /nonexistent/file "分析"` | 警告文件不存在，跳过 | ☐ |
| E6 | Context 取消 | 在流式响应中 Ctrl+C | 优雅停止，不崩溃 | ☐ |
| E7 | 工具 panic | 触发工具内部 panic | 捕获 panic，记录日志，继续运行 | ☐ |
| E8 | Actor 停止 | 在 SessionActor 停止后调用 Create | 返回 `ErrActorStopped`，不 panic | ☐ |

---

## 十、并发与性能 (Concurrency)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| F1 | 并发请求 | 同时执行 5 个 `openaide "simple question"` | 各自独立完成，无竞态 | ☐ |
| F2 | 大数据量 | 读取超过 10000 行的文件 | 分页读取，不 OOM | ☐ |
| F3 | 长时间运行 | REPL 中连续对话 20 轮 | 无内存泄漏，无 goroutine 泄漏 | ☐ |
| F4 | 快速连续请求 | 快速连续发送 10 个 one-shot 请求 | 请求排队处理，不丢失 | ☐ |
| F5 | Actor 通道满 | 大量并发 session 操作 | 返回 `ErrActorBusy`，不阻塞 | ☐ |

---

## 十一、数据持久化 (Data Persistence)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| D1 | SQLite 会话 | 检查 `~/.openaide/data/sessions.db` | 包含会话数据 | ☐ |
| D2 | SQLite 知识 | 检查 `~/.openaide/data/knowledge.db` | 包含知识条目 | ☐ |
| D3 | SQLite 记忆 | 检查 `~/.openaide/data/memory.db` | 包含记忆数据 | ☐ |
| D4 | 技能文件 | 检查 `~/.openaide/data/skills/auto_skills.json` | distill 后包含自动技能 | ☐ |
| D5 | 检查点 | 检查 `~/.openaide/data/checkpoints/` | 含检查点 JSON 文件 | ☐ |
| D6 | 追踪日志 | 检查 `~/.openaide/data/traces.jsonl` | 包含执行追踪记录 | ☐ |
| D7 | 自定义 Prompt | 创建 `~/.openaide/data/prompts/user/custom.md` | REPL 中自动加载自定义 prompt | ☐ |

---

## 十二、ProjectMind (跨会话学习)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| PM1 | CodeMap | 多次执行代码相关任务 | 自动记录文件→用途映射 | ☐ |
| PM2 | RiskMap | 多次遇到相同错误 | 标记脆弱区域 | ☐ |
| PM3 | Conventions | 项目积累的编码约定 | 注入到子 Agent prompt | ☐ |
| PM4 | 执行历史 | 检查 `~/.openaide/data/project_mind.json` | 包含最近 50 条执行记录 | ☐ |

---

## 十三、前端组件测试 (Frontend, 浏览器测试)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| W1 | 聊天输入 | 在 Web 前端输入框输入消息 | 发送并显示回复 | ☐ |
| W2 | 流式显示 | 输入复杂问题 | 流式逐字显示回复 | ☐ |
| W3 | Thinking 可视化 | 观察推理过程 | ThinkingVisualizer 组件显示推理步骤 | ☐ |
| W4 | 工具调用显示 | 触发工具调用 | ToolCallDisplay 组件显示工具名和状态 | ☐ |
| W5 | 纠正面板 | 触发自动修正 | CorrectionPanel 显示修正过程 | ☐ |
| W6 | 模板页面 | 访问 Templates 页面 | 显示可用模板 | ☐ |
| W7 | 定时任务 | 访问 Scheduled Tasks | 显示定时任务列表 | ☐ |
| W8 | 模型切换 | 在 Web 前端切换模型 | API 使用切换后的模型 | ☐ |
| W9 | 暗色模式 | 切换暗色模式 | 主题切换为暗色 | ☐ |

---

## 十四、兼容性与集成 (Compatibility)

| # | 测试场景 | 操作 | 预期结果 | 通过 |
|---|---------|------|---------|------|
| I1 | CLAUDE.md 加载 | 在项目根目录放置 `CLAUDE.md`，运行 `openaide "项目规则是什么"` | 回答中包含 CLAUDE.md 内容 | ☐ |
| I2 | OPENAIDE.md | 同 I1，但用 `OPENAIDE.md` | 自动加载 | ☐ |
| I3 | CODEBUDDY.md | 同 I1，但用 `CODEBUDDY.md` | 自动加载 | ☐ |
| I4 | OpenCode 配置 | 项目根目录放置 `opencode.json`，启动 openaide | 自动发现并导入 MCP 服务器 | ☐ |
| I5 | MCP stdio | 配置 stdio MCP server | 工具成功注册 | ☐ |
| I6 | MCP SSE | 配置 SSE MCP server | 工具成功注册 | ☐ |
| I7 | LSP 自动检测 | 在 Go 项目中问 "这个函数在哪里定义" | LSP 跳转定义 | ☐ |
| I8 | 21 语言检测 | 在不同语言项目中运行 `openaide` | 自动检测语言并注入对应约定 | ☐ |

---

## 测试结果汇总

| 分类 | 总数 | 通过 | 失败 | 跳过 |
|------|------|------|------|------|
| 一、基础功能 | 11 | | | |
| 二、CLI 模式 | 20 | | | |
| 三、Server 模式 | 17 | | | |
| 四、核心 Agent | 35 | | | |
| 五、Team 模式 | 9 | | | |
| 六、插件系统 | 8 | | | |
| 七、会话管理 | 7 | | | |
| 八、自我更新 | 3 | | | |
| 九、错误处理 | 8 | | | |
| 十、并发与性能 | 5 | | | |
| 十一、数据持久化 | 7 | | | |
| 十二、ProjectMind | 4 | | | |
| 十三、前端组件 | 9 | | | |
| 十四、兼容性 | 8 | | | |
| **合计** | **151** | | | |

---

## 测试说明

1. **标记方式**: 测试通过后在 ☐ 中打 ✓
2. **失败记录**: 失败的测试记录实际现象和错误日志
3. **跳过记录**: 记录跳过原因（如：无浏览器）
4. **Server 测试**: 先启动 `openaide server &`，测试完后 `kill %1`
5. **清理**: `rm -rf ~/.openaide/data/` 重置环境
