# Examples

本目录包含 OpenAIDE 的 **MCP / Plugin / Skill** 三种扩展机制的完整可复制示例。

## 目录结构

```
examples/
├── README.md                          ← 本文件
├── mcp.example.yaml                   ← MCP 服务器配置完整示例（stdio + sse + env）
├── skills/                            ← 单点技能（每个都是最小插件格式）
│   ├── release-version/               ← 完整示例（含字段注释 + 可执行脚本）
│   │   ├── .claude-plugin/plugin.json ← 插件清单（必须，否则不会被发现）
│   │   └── skills/
│   │       └── release-version/
│   │           ├── SKILL.md
│   │           └── scripts/compute_version.sh
│   └── git-cleanup/                   ← 最小可用示例（name + description）
│       ├── .claude-plugin/plugin.json
│       └── skills/
│           └── git-cleanup/
│               └── SKILL.md
└── plugins/
    └── demo-plugin/                   ← 插件容器示例（Claude Code 兼容结构）
        ├── .claude-plugin/plugin.json ← 插件清单（name/version/description/author）
        ├── .mcp.json                  ← 插件自带的 MCP 服务器（自动发现）
        ├── hooks/hooks.json           ← 生命周期钩子（事件 → 命令）
        └── skills/demo-greet/SKILL.md ← 插件自带的技能
```

> **注意**：`DiscoverClaudeSkills` / `DiscoverClaudeMCP` / `DiscoverClaudeHooks` 都只扫描
> 包含 `.claude-plugin/plugin.json` 的目录——任何 skill（包括"单点技能"）都必须以
> 插件目录结构存放才会被加载。

## 怎么用

### Skill（单点能力，推荐优先）

把 `examples/skills/` 下的**整个插件目录**复制到 `~/.openaide/data/plugins/`：

```bash
cp -r examples/skills/* ~/.openaide/data/plugins/
# 单个技能：
cp -r examples/skills/release-version ~/.openaide/data/plugins/
```

技能命中机制（无需手动选择）：
1. 请求进入时，统一查询分析预匹配技能 ID（`kernel_stream.go`）
2. 未预匹配则 LLM 兜底检测（`detectWithLLM`，只回技能 ID 或 `none`）
3. 命中后：SKILL.md 正文注入 system prompt，`allowed-tools` 过滤工具集


### MCP（外部工具接入，配置即用）

```bash
cp examples/mcp.example.yaml /tmp/mcp.patch.yaml
# 把其中 mcp: 段合并进 ~/.openaide/config.yaml 顶层
```

支持两种连接：
- **stdio**：`command` + `args`（本地进程）
- **sse**：`url`（远程 HTTP）

工具注册进 LLM 时带 `mcp_` 前缀（如 `mcp_github_*`）。

### Plugin（打包容器，分发用）

把 `examples/plugins/demo-plugin` 整体复制到插件目录：

```bash
cp -r examples/plugins/demo-plugin ~/.openaide/data/plugins/
```

插件目录包含 `.claude-plugin/plugin.json`（必须）时才会被识别，其内的
skills、.mcp.json、hooks 会被自动发现并加载（`app.go` 启动时扫描）。

## 字段速查

| 机制 | 关键字段 | 作用 |
|---|---|---|
| SKILL.md | `name`（必填） | 技能 ID，匹配返回值 |
| SKILL.md | `description` | 自动匹配依据，务必写清触发场景 |
| SKILL.md | `allowed-tools` | 工具白名单（Claude 工具名，自动映射） |
| SKILL.md | `argument-hint` | 调用参数提示 |
| plugin.json | `name/version/description` | 插件清单 |
| .mcp.json | `{"服务器名": {command/args/env}}` | 插件自带 MCP 服务器 |
| hooks.json | `{"hooks":[{"event","command"}]}` | 生命周期钩子（PreToolUse/PostToolUse/Stop…） |
