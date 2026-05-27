# OpenAIDE 使用指南

## 安装

### 下载预编译二进制

从 [GitHub Releases](https://github.com/lzy1102/openaide/releases) 下载对应平台：

| 平台 | 文件名 |
|------|--------|
| Linux x86_64 | `openaide-linux-amd64` |
| Linux ARM64 | `openaide-linux-arm64` |
| Windows x86_64 | `openaide-windows-amd64.exe` |
| macOS x86_64 | `openaide-darwin-amd64` |
| macOS ARM64 (M1/M2/M3) | `openaide-darwin-arm64` |

```bash
# Linux/macOS
chmod +x openaide-linux-amd64
sudo mv openaide-linux-amd64 /usr/local/bin/openaide

# Windows (PowerShell)
move openaide-windows-amd64.exe C:\Windows\System32\openaide.exe
```

### 从源码编译

```bash
git clone https://github.com/lzy1102/openaide.git
cd openaide/backend

# 当前平台
make build

# 全平台交叉编译
make build-all
```

## 配置

### 1. 创建配置文件

```bash
mkdir -p ~/.openaide
```

`~/.openaide/config.yaml`:

```yaml
llm:
  providers:
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key-here
      default_model: deepseek-v4-pro
      timeout: 300
      thinking: true
      reasoning_effort: max
      enabled: true
    - name: deepseek-flash
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key-here
      default_model: deepseek-v4-flash
      timeout: 120
      enabled: true
  model_routing:
    reasoning: deepseek-v4-pro   # 分析/审查任务
    execution: deepseek-v4-flash # 执行/编码任务
kernel:
  max_rounds: 30
  min_rounds: 8
log:
  level: info
  lang: zh
```

### 2. 首次运行

```bash
openaide
```

系统提示词自动写入 `~/.openaide/data/prompts/system.md`，可随时编辑。

## 基本使用

### REPL 模式（默认）

```bash
openaide              # 启动交互式 REPL
openaide -c           # 恢复上次会话
openaide "你的问题"    # 一次性问答
```

### REPL 命令

| 命令 | 说明 |
|------|------|
| `/help` | 显示帮助 |
| `/model` | 查看当前模型 |
| `/model <name>` | 切换到指定模型 |
| `/lang zh/en` | 切换语言 |
| `/sessions` | 会话列表 |
| `/session <id>` | 切换会话 |
| `/clear` | 清屏 |
| `/log` | 最近日志 |
| `/handoff` | 保存会话状态 |
| `/exit`, `/quit`, `/q` | 退出 |

### 编程命令

| 命令 | 说明 |
|------|------|
| `/analyst <task>` | 分析任务 (reasoning model) |
| `/coder <task>` | 编码任务 (execution model) |
| `/reviewer <task>` | 审查任务 (reasoning model) |
| `/executor <task>` | 执行/验证 (execution model) |
| `/team <task>` | 完整团队链 |

### 快捷键

| 快捷键 | 功能 |
|--------|------|
| `Ctrl+C` | 中断流式输出（不退出） |
| `Ctrl+D` | 空行退出 |
| `Ctrl+R` | 搜索历史 |
| `↑` / `↓` | 浏览历史 |
| `Tab` | 补全命令或文件路径 |
| `Alt+Enter` | 多行输入（打开 $EDITOR） |

## 高级功能

### 项目规则文件

在项目根目录创建 `OPENAIDE.md`：

```markdown
## Build Commands
- build: make build
- test: make test

## Code Conventions
- Use snake_case for Go
- Tests alongside source

## Forbidden
- Never modify migrations/
```

每次查询时自动加载。也支持 CLAUDE.md、CODEBUDDY.md、CONVENTIONS.md、.cursor/rules/ 等兼容格式。

### 排除文件

创建 `.openaideignore`（gitignore 格式）：

```
node_modules/
dist/
*.log
```

### 多 Agent 团队

复杂任务自动拆分为子 Agent 协作：
- PreviewPlan 判断复杂度
- 简单任务（1 个子任务）→ 直接 ReAct
- 中等任务（2-3 个）→ 交互确认后执行
- 复杂任务（4+ 个）→ 深度研究 → 方案选择 → 执行

### 成本优化

```
规划/分析/审查 → deepseek-v4-pro（深度推理）
编码/执行/总结 → deepseek-v4-flash（快速便宜）
技能检测/轮次估算 → flash（轻量任务）
```

### 子 Agent 模型路由

| 角色 | 模型 | 原因 |
|------|------|------|
| analyst | reasoning (pro) | 需要深度思考 |
| coder | execution (flash) | 按方案执行，无需推理 |
| reviewer | reasoning (pro) | 需要质量判断 |
| executor | execution (flash) | 跑测试，无需推理 |

## 日志

```bash
# 查看日志
tail -f ~/.openaide/logs/openaide.log

# 实时过滤 LLM 调用
tail -f ~/.openaide/logs/openaide.log | grep "LLM chat"
```

## 常见问题

**Q: 怎么换 API Key？**
编辑 `~/.openaide/config.yaml`，修改 `api_key` 字段。

**Q: 怎么用其他模型（OpenAI、Claude）？**
在 config.yaml 添加 provider，修改 `type` 和 `base_url`。

**Q: 提示词怎么改？**
编辑 `~/.openaide/data/prompts/system.md`。空文件或删除后重启会自动写入默认值。

**Q: 会话存在哪里？**
`~/.openaide/data/sessions/`。最多保留 50 条消息，完整历史在 checkpoint 中。
