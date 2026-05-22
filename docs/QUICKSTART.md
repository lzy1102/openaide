# OpenAIDE 快速入门指南

## 1. 安装

```bash
cd backend
make build
# 输出: bin/openaide-server, bin/openaide-cli
```

## 2. 配置

编辑 `~/.openaide/config.yaml`：

```yaml
llm:
  default_provider: "openai"
  providers:
    - name: "openai"
      type: "openai"
      base_url: "https://api.openai.com/v1"
      api_key: "sk-your-api-key"
      default_model: "gpt-4o-mini"
      enabled: true

kernel:
  max_rounds: 10
  max_tokens: 4000
```

## 3. 使用

```bash
# 交互式 TUI（首次运行会进入引导配置身份）
openaide-cli

# 单次执行
openaide-cli "帮我分析这个项目的代码结构"

# 继续上次会话
openaide-cli -c

# One-shot 带文件
openaide-cli main.go "review this function"
```

## 4. TUI 内操作

| 快捷键 | 功能 |
|--------|------|
| Enter | 发送消息 |
| Ctrl+C | 退出 / 停止流式 |
| Ctrl+S | 会话列表 |
| Ctrl+H | 帮助 |
| ↑/↓ | 输入历史 |

| 斜杠命令 | 功能 |
|----------|------|
| /help | 帮助 |
| /clear | 清屏 |
| /model [name] | 查看/切换模型 |
| /<skill> | 激活技能 (builtin 或 plugin) |

## 5. 安装插件

```bash
# 复制 Claude Code 插件到 data/plugins/
cp -r ~/claude-plugins/some-plugin ./data/plugins/

# 启动即发现
openaide-cli
```

## 6. 数据位置

```
./data/                  # 项目数据（默认，可配置）
├── prompts/system.md    # 系统提示词（可编辑）
├── sessions/            # 会话记录
├── memory/              # 记忆
├── plugins/             # 插件
└── knowledge/           # 知识库

~/.openaide/config.yaml  # 全局配置
```

## 下一步

- `CLAUDE.md` — 项目架构和开发指南
- `INSTALL.md` — 详细安装、Docker、CI/CD
- `README.md` — 全部功能列表
