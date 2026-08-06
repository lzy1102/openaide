# OpenAIDE 文档中心

> 日期: 2026-08-06

---

## 文档导航

### 快速开始

| 文档 | 说明 | 阅读时间 |
|------|------|----------|
| [QUICKSTART.md](QUICKSTART.md) | 安装、配置、基础使用 | 10 分钟 |
| [DEPLOYMENT.md](DEPLOYMENT.md) | 本地/服务器/Docker 部署 | 15 分钟 |

### 架构设计

| 文档 | 说明 | 阅读时间 |
|------|------|----------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | 总体架构、分层设计、请求生命周期、扩展机制 | 30 分钟 |
| [MODULES.md](MODULES.md) | 模块职责、依赖关系、数据流 | 45 分钟 |
| [MIGRATION.md](MIGRATION.md) | 迁移计划、Phase 分解、风险缓解 | 20 分钟 |

### 可视化

| 文档 | 说明 |
|------|------|
| [flowcharts.html](flowcharts.html) | 交互式流程图（6 个核心流程） |

---

## 阅读顺序建议

### 方式一：快速了解（30 分钟）

```
QUICKSTART.md → flowcharts.html
```

适合：想快速了解项目功能和用法

### 方式二：深入理解（2 小时）

```
ARCHITECTURE.md → MODULES.md → MIGRATION.md
```

适合：需要理解设计原理和实现细节

### 方式三：准备开发（3 小时）

```
ARCHITECTURE.md → MODULES.md → MIGRATION.md → DEPLOYMENT.md
```

适合：准备参与开发或二次扩展

---

## 核心设计决策

| 决策 | 说明 | 文档位置 |
|------|------|----------|
| **SQLite 纯 Go 存储** | WAL 模式 + modernc.org/sqlite，CGO_ENABLED=0 | ARCHITECTURE.md 技术栈 |
| **混合运行模式** | REPL(默认) + API 服务器模式，单一 Go 二进制 | ARCHITECTURE.md 仓库结构 |
| **ReAct 内核** | 统一查询分析 + ReAct 循环 + 反思沉淀作为核心 | ARCHITECTURE.md 核心数据流 |
| **Actor/CSP 无锁** | 有状态模块单 goroutine + channel 通信，零锁核心路径 | ARCHITECTURE.md 关键架构决策 |
| **分层提示词 L0-L6** | 稳定前缀缓存友好 + 动态尾部按需注入 | ARCHITECTURE.md 核心模块详解 1 |
| **成本感知双模型** | reasoning/execution 路由，flash 执行 / pro 推理 | ARCHITECTURE.md 核心模块详解 2 |
| **LLM 语义压缩** | LLM 压缩 + NovelCompressor 降级，token 预算截断 | MODULES.md 模块清单（压缩） |
| **生态兼容扩展** | MCP + Claude 插件/Skill/Hooks 自动发现 | ARCHITECTURE.md 核心模块详解 6 |

---

## 文档状态

| 文档 | 状态 | 最后更新 |
|------|------|----------|
| ARCHITECTURE.md | 已更新 | 2026-08-06 |
| MODULES.md | 已更新 | 2026-08-06 |
| QUICKSTART.md | 已更新 | 2026-05-27 |
| DEPLOYMENT.md | 初稿完成 | 2026-05-15 |
| USAGE.md | 已更新 | 2026-08-04 |

---

## 贡献指南

如需修改文档：

1. 确保与现有文档风格一致
2. 更新目录（如有新增章节）
3. 更新本文档的文档状态表格
4. 提交时注明文档变更范围

---

> **提示**: 所有文档使用 Markdown 格式，可用任何文本编辑器打开。流程图使用 Mermaid 语法，可在 [Mermaid Live Editor](https://mermaid.live) 在线预览。
