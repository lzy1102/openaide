# OpenAIDE 快速入门

## 1. 安装

```bash
cd backend
make build
# 输出: bin/openaide server, bin/openaide
```

## 2. 配置

编辑 `~/.openaide/config.yaml`：

```yaml
llm:
  providers:
    - name: deepseek
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key
      default_model: deepseek-v4-pro
      timeout: 300
      thinking: true
      reasoning_effort: max
    - name: deepseek-flash
      type: openai
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-api-key
      default_model: deepseek-v4-flash
      timeout: 120
  model_routing:
    reasoning: deepseek-v4-pro
    execution: deepseek-v4-flash
kernel:
  max_rounds: 30
log:
  level: info
  lang: zh
```

## 3. 使用

```bash
# REPL 交互模式（默认）
openaide

# 一次性问答
openaide "帮我分析这个项目的代码结构"

# 恢复上次会话
openaide -c

# 带文件
openaide main.go "review this function"
```

## 4. REPL 操作

| 快捷键 | 功能 |
|--------|------|
| Enter | 发送 |
| Ctrl+C | 中断流式输出 |
| Ctrl+D | 空行退出 |
| Ctrl+R | 搜索历史 |
| ↑/↓ | 浏览历史 |
| Tab | 补全命令/文件路径 |

| 命令 | 功能 |
|------|------|
| /help | 帮助 |
| /clear | 清屏 |
| /model [name] | 查看/切换模型 |
| /analyst | 分析任务 |
| /coder | 编码任务 |
| /team | 完整团队链 |
| /sessions | 会话列表 |
| /exit, /quit | 退出 |

## 5. 安装插件

```bash
# 复制 Claude Code 插件到 data/plugins/
cp -r ~/claude-plugins/some-plugin ./data/plugins/
openaide  # 启动即发现
```

## 6. 数据位置

```
~/.openaide/
├── config.yaml           # 全局配置
├── data/
│   ├── prompts/system.md # 系统提示词（可编辑）
│   ├── sessions/         # 会话记录
│   ├── memory/           # 记忆
│   ├── plugins/          # 插件
│   └── knowledge/        # 知识库
└── logs/                 # 日志
```

## 下一步

- `README.md` — 全部功能列表
- `backend/API.md` — API 文档
- `docs/USAGE.md` — 详细使用指南
- `docs/ARCHITECTURE.md` — 架构设计
