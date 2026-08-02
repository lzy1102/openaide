# OpenAIDE 测试计划

> 版本: v0.2.0-56-g9c8e615 | 目标: 手动验证所有核心功能

## 测试前准备

```bash
# 确认版本
~/.openaide/bin/openaide --version
# 预期: OpenAIDE CLI v0.2.0-56-g9c8e615

# 备份旧数据（可选）
mv ~/.openaide/data ~/.openaide/data.bak 2>/dev/null
```

---

## 第一轮：冒烟测试（5分钟）

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| S1 | 版本 | `openaide --version` | 显示版本号 | ☐ |
| S2 | 帮助 | `openaide --help` | 显示所有命令，含 `server` | ☐ |
| S3 | 简单问答 | `openaide "hello"` | 正常回复，无报错 | ☐ |
| S4 | 中文问答 | `openaide "你好"` | 用中文回复 | ☐ |
| S5 | 服务启动 | `openaide server` → Ctrl+C | 启动成功，Ctrl+C 优雅退出 | ☐ |

---

## 第二轮：CLI 模式（15分钟）

### one-shot 模式

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| C1 | 代码问题 | `openaide "用Go写一个hello world"` | 输出可运行代码 | ☐ |
| C2 | 读文件 | `openaide OPENAIDE.md "总结核心规则"` | 读取文件并总结 | ☐ |
| C3 | 指定模型 | `openaide --model deepseek-v4-flash "hi"` | 使用 flash 模型 | ☐ |
| C4 | 超时保护 | `openaide "写10万行代码"` | 300s 超时退出，不卡死 | ☐ |
| C5 | Ctrl+C | 执行大任务，按 Ctrl+C | 优雅退出 | ☐ |

### REPL 模式

| # | 操作 | 命令/输入 | 预期 | ✓ |
|---|------|----------|------|---|
| R1 | 进入 | `openaide` | 显示提示符 | ☐ |
| R2 | 基本对话 | `hello` | 正常回复 | ☐ |
| R3 | 多轮对话 | 连续问 3 个相关问题 | 上下文连贯 | ☐ |
| R4 | 恢复会话 | `openaide -c` | 恢复上次对话 | ☐ |
| R5 | 工具触发 | `读取 OPENAIDE.md` | 显示 ⚙ read_file | ☐ |
| R6 | 审批弹窗 | `创建 /tmp/test.txt 写入 hello` | 弹出 Allow/Deny | ☐ |
| R7 | 拒绝审批 | 选 Deny | 工具被拒绝 | ☐ |
| R8 | 全部允许 | 选 Allow All | 后续不再弹窗 | ☐ |
| R9 | /clear | 输入 `/clear` | 清空当前会话 | ☐ |
| R10 | 退出 | `exit` 或 Ctrl+D | 正常退出 | ☐ |

---

## 第三轮：Server 模式（10分钟）

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| W1 | 启动 | `openaide server &` | 后台启动，监听端口 | ☐ |
| W2 | 健康检查 | `curl localhost:8080/health` | 200 OK | ☐ |
| W3 | 首页 | 浏览器访问 `http://localhost:8080` | 显示 Web 前端 | ☐ |
| W4 | 聊天 | 浏览器中发消息 | 正常回复 | ☐ |
| W5 | API 聊天 | `curl -X POST localhost:8080/api/v1/chat -H 'Content-Type: application/json' -d '{"query":"hi"}'` | JSON 响应 | ☐ |
| W6 | API 流式 | `curl -N -X POST localhost:8080/api/v1/chat/stream -H 'Content-Type: application/json' -d '{"query":"hi"}'` | SSE 流式输出 | ☐ |
| W7 | 会话列表 | `curl localhost:8080/api/v1/sessions` | 返回会话列表 | ☐ |
| W8 | 工具列表 | `curl localhost:8080/api/v1/tools` | 返回工具定义 | ☐ |

---

## 第四轮：核心 Agent 能力（15分钟）

### 工具调用

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| T1 | 读文件 | `openaide "读取 README.md"` | 调用 read_file | ☐ |
| T2 | 搜索文件 | `openaide "查找所有 Go 文件"` | 调用 search_files | ☐ |
| T3 | 写文件 | `openaide "创建 /tmp/hello.go 写入 hello world"` | 写入成功 | ☐ |
| T4 | diff编辑 | `openaide "把 /tmp/hello.go 的 world 改成 openaide"` | diff_edit 成功 | ☐ |
| T5 | git状态 | `openaide "git status"` | 调用 git_status | ☐ |
| T6 | web搜索 | `openaide "搜索 Go 1.26 新特性"` | 调用 web_search | ☐ |

### 任务类型

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| M1 | coding | `openaide "fix: add comment to /tmp/hello.go"` | 编码模式，用工具修改 | ☐ |
| M2 | review | `openaide "review OPENAIDE.md for bugs"` | 审查模式，输出问题列表 | ☐ |
| M3 | think | `openaide "解释 Go 的 CSP 模型"` | 思考模式，分析性回答 | ☐ |
| M4 | debugging | `openaide "有个bug: /tmp/hello.go 的 main 函数不工作"` | 调试模式，先读文件再分析 | ☐ |

### 幻觉预防（关键）

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| H1 | 不编造 | `openaide "OPENAIDE.md 第500行是什么"` | 先读文件再回答，不猜测 | ☐ |
| H2 | 不截断 | `openaide "修改 README.md 的标题"` | 展示完整 diff，不用 "..." | ☐ |
| H3 | 验证API | `openaide "用 encoding/xml 包解析"` | 先 grep 检查 go.mod → 发现不存在 → 说没有 | ☐ |
| H4 | 编辑前重读 | `openaide "在 OPENAIDE.md 第一行加注释"` | 编辑前先 read_file | ☐ |

---

## 第五轮：Team 模式（10分钟）

| # | 操作 | 输入 | 预期 | ✓ |
|---|------|------|------|---|
| TM1 | 进入REPL | `openaide` | 提示符 | ☐ |
| TM2 | /analyst | `/analyst 分析 kernel.go 架构` | analyst 角色回复 | ☐ |
| TM3 | /coder | `/coder 在 /tmp/test.go 写个 hello` | coder 角色写代码 | ☐ |
| TM4 | /reviewer | `/reviewer 审查 /tmp/test.go` | reviewer 角色审查 | ☐ |
| TM5 | /team | `/team 分析项目结构` | 多角色协作 | ☐ |
| TM6 | /auto | `/auto` | 切回自动模式 | ☐ |

---

## 第六轮：持久化与恢复（5分钟）

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| P1 | 会话保存 | 在 REPL 中对话，然后 exit | 会话自动保存 | ☐ |
| P2 | 会话恢复 | `openaide -c` | 恢复上次对话 | ☐ |
| P3 | 会话列表 | `openaide sessions` | 列出所有会话 | ☐ |
| P4 | 知识积累 | 连续 3 次问 "解释 Go 接口" | 第 4 次引用之前的回答 | ☐ |

---

## 第七轮：稳定性（5分钟）

| # | 操作 | 命令 | 预期 | ✓ |
|---|------|------|------|---|
| ST1 | 错误API Key | 临时改错 config.yaml 的 api_key | 显示 401 友好提示 | ☐ |
| ST2 | 并发请求 | 两个终端同时 `openaide "hi"` | 各自独立，不冲突 | ☐ |
| ST3 | 空输入 | `openaide ""` | 不崩溃 | ☐ |
| ST4 | 特殊字符 | `openaide "测试 %s %d ${PATH}"` | 正常处理 | ☐ |
| ST5 | 连续快速 | 连续 5 个 one-shot 请求 | 不丢请求，不崩溃 | ☐ |

---

## 测试结果

| 轮次 | 名称 | 总数 | 通过 | 失败 | 
|------|------|------|------|------|
| 1 | 冒烟测试 | 5 | | |
| 2 | CLI 模式 | 15 | | |
| 3 | Server 模式 | 8 | | |
| 4 | 核心 Agent | 14 | | |
| 5 | Team 模式 | 6 | | |
| 6 | 持久化恢复 | 4 | | |
| 7 | 稳定性 | 5 | | |
| **总计** | | **57** | | |

---

## 测试后清理

```bash
# 停止 server
kill %1 2>/dev/null

# 清理测试文件
rm -f /tmp/hello.go /tmp/test.txt /tmp/test.go

# 恢复数据（如果备份了）
# mv ~/.openaide/data ~/.openaide/data.new && mv ~/.openaide/data.bak ~/.openaide/data
```
