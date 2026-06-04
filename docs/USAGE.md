# OpenAIDE 使用指南

## CLI 命令

```bash
openaide              # 启动交互式 REPL
openaide "prompt"     # 一次性问答
openaide -c           # 恢复上一次会话
openaide -y           # 自动审批所有工具操作
openaide --model <n>  # 指定模型
openaide --verbose    # 调试日志
openaide setup        # 交互式配置向导
openaide sessions     # 列出所有会话
openaide --version    # 显示版本
```

## REPL 快捷键

| 快捷键 | 功能 |
|--------|------|
| `↑` / `↓` | 浏览历史命令 |
| `Tab` | 补全命令/文件路径 |
| `Ctrl+R` | 搜索历史 |
| `Ctrl+C` | 中断当前操作 |
| `Ctrl+D` | 退出 (空行) |
| `Alt+Enter` | 多行编辑 ($EDITOR) |
| `@filename` | 读取文件内容拼入提示词 |

## 内置命令

| 命令 | 功能 |
|------|------|
| `/help` | 显示帮助 |
| `/clear` | 清空当前会话，开始新任务 |
| `/model [name]` | 查看或切换模型 |
| `/lang zh\|en` | 切换界面语言 |
| `/log` | 查看最近日志 |
| `/sessions` | 列出所有会话 |
| `/session <id>` | 切换到指定会话 |
| `/handoff` | 保存当前状态 |
| `/exit`, `/quit`, `/q` | 退出 |
| `/undo` | 撤销上一次文件修改 |
| `/init` | 重新运行项目初始化 |

## 团队模式

多 Agent 协作处理复杂任务:

```bash
/analyst    # 分析员 — 只读分析，制定方案
/coder      # 程序员 — 写代码，修改文件
/reviewer   # 审查员 — 审查代码质量和安全
/executor   # 执行者 — 运行测试，验证结果
/team       # 完整团队 — 分析→编码→审查→验证
```

每个角色有独立的工具集和反提示词约束。

## Server 模式

```bash
openaide-server --config ~/.openaide/config.yaml
```

### API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/health` | GET | 健康检查 |
| `/api/v1/chat` | POST | 同步聊天 |
| `/api/v1/chat/stream` | POST | 流式聊天 (SSE) |
| `/api/v1/sessions` | GET | 列出会话 |
| `/api/v1/sessions/{id}` | GET | 会话详情 |
| `/api/v1/tools` | GET | 工具列表 |
| `/api/v1/stats` | GET | 系统状态 |
| `/ws` | GET | WebSocket |

### API 示例

```bash
# 同步请求
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"content":"Explain goroutines","user_id":"user1","project_id":"demo"}'

# 流式请求
curl -X POST http://localhost:8080/api/v1/chat/stream \
  -H "Content-Type: application/json" \
  -d '{"content":"Write hello world in Go"}'
```

## 提示词自定义

创建以下文件覆盖默认提示词:

```
~/.openaide/data/prompts/
├── l0.md           # L0 身份+安全 (全局)
├── l1.md           # L1 项目上下文
├── l3_coding.md    # L3 编码模式
├── l3_review.md    # L3 审查模式
├── l3_teaching.md  # L3 教学模式
├── l3_research.md  # L3 研究模式
├── system.zh.md    # 自定义中文提示词
└── system.en.md    # 自定义英文提示词
```

## 配置文件热更新

修改 `~/.openaide/config.yaml` 后自动重载: `max_rounds`, `max_tokens`, `unsafe_mode`, `log.level`。其他设置需重启。

## 数据目录

```
~/.openaide/data/
├── sessions.db          # 会话 (SQLite)
├── knowledge.db         # 知识库 (SQLite + 向量)
├── memory.db            # 记忆 (SQLite)
├── skills/auto_skills.json  # 自动提取的技能
├── plugins/             # Claude 兼容插件
├── project_mind.json    # 跨会话项目知识
└── traces.jsonl         # 执行追踪
```

## 常见问题

**Q: API Key 报错?** 检查 `config.yaml` 的 `api_key`，确认 `enabled: true`

**Q: 如何切换模型?** `/model` 查看，`/model <name>` 切换

**Q: 如何让 Agent 更聪明?** 创建 `CLAUDE.md` 描述项目，用 `/team` 处理复杂任务，多跑类似任务让蒸馏系统学习

**Q: 数据在哪?** 默认 `~/.openaide/data/`，`storage.data_dir` 配置
