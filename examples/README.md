# Examples

本目录包含 OpenAIDE 的**示例插件**。TS 版插件为一个目录/包，通过 `index.ts` 导出 `OpenAIDePlugin` 接口，以同进程 `import()` 动态加载（无子进程）。

## 目录结构

```
examples/
└── plugins/
    └── example-plugin/            ← 完整示例（工具 + 钩子 + 人格）
        ├── index.ts               ← 插件入口（导出 OpenAIDePlugin：tools/hooks/persona）
        ├── openaide.yaml          ← 可选 manifest（name/version/description/persona）
        ├── SYSTEM.md              ← 可选：人格系统提示词
        └── package.json           ← 插件自身依赖
```

## 怎么用

把示例插件目录复制到插件目录并启动：

```bash
# 方式 A：运行时指向 example 目录
#   macOS / Linux（bash）
OPENAIDE_PLUGINS_DIR=examples/plugins openaide
#   Windows（PowerShell）
$env:OPENAIDE_PLUGINS_DIR="examples/plugins"; openaide

# 方式 B：复制到默认插件目录
#   macOS / Linux
cp -r examples/plugins/example-plugin ~/.openaide/plugins/
#   Windows（PowerShell）
Copy-Item -Recurse examples/plugins/example-plugin "$HOME\.openaide\plugins\"
openaide
```

启动后发送 `upper hello`，即可看到工具调用（`example__upper`）、钩子触发与人格注入。

## 字段速查

| 机制 | 关键字段 | 作用 |
|---|---|---|
| tools | `index.ts > tools` | 工具定义，注册为 `<插件名>__<工具名>`，handler 与内核同进程执行 |
| hooks | `index.ts > hooks` | 事件钩子，订阅内核事件（如 `tool.call.ended`） |
| interceptors | `index.ts > interceptors` | 拦截器（策略）：可否决/改写 LLM 请求与工具调用——审批门、脱敏、限流等 |
| providers | `index.ts > providers` | 注册 LLM 后端工厂；`config.llm.provider: <name>` 即切换大脑 |
| persona | `index.ts > persona` 或 `SYSTEM.md` | 人格定义（L0 系统提示词） |
| manifest | `openaide.yaml` | 可选元信息：name/version/description/category/persona/toolAllowlist |
| category | `index.ts` 或 `openaide.yaml` | 分类（轻量元数据，`openaide plugins` 按此分组展示；缺省 uncategorized） |