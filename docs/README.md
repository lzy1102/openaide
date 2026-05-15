# OpenAIDE 文档中心

> 版本: v3.0.0-draft  
> 日期: 2026-05-15  
> 状态: 设计评审中

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
| [ARCHITECTURE.md](ARCHITECTURE.md) | 总体架构、分层设计、数据流 | 30 分钟 |
| [MODULES.md](MODULES.md) | 模块职责、接口定义、实现细节 | 45 分钟 |
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
| **纯文件存储** | Markdown + JSON + 二进制向量，零数据库依赖 | ARCHITECTURE.md 10.3 |
| **零 CGO** | 所有存储纯 Go 实现，禁止 CGO | ARCHITECTURE.md 10.3 |
| **混合运行模式** | 直接模式(默认) + 服务器模式 + TUI 模式 | ARCHITECTURE.md 3.x |
| **ReAct 内核** | Reasoning + Acting 循环作为核心 | ARCHITECTURE.md 3.1.3 |
| **三级记忆** | L1 工作记忆 → L2 短期记忆 → L3 长期记忆 | MODULES.md 3.x |
| **小说式压缩** | 章节/人物/伏笔/闪回多维度压缩 | MODULES.md 14.x |

---

## 文档状态

| 文档 | 状态 | 最后更新 |
|------|------|----------|
| ARCHITECTURE.md | 设计评审中 | 2026-05-15 |
| MODULES.md | 设计评审中 | 2026-05-15 |
| MIGRATION.md | 设计评审中 | 2026-05-15 |
| QUICKSTART.md | 初稿完成 | 2026-05-15 |
| DEPLOYMENT.md | 初稿完成 | 2026-05-15 |
| flowcharts.html | 初稿完成 | 2026-05-15 |

---

## 贡献指南

如需修改文档：

1. 确保与现有文档风格一致
2. 更新目录（如有新增章节）
3. 更新本文档的文档状态表格
4. 提交时注明文档变更范围

---

> **提示**: 所有文档使用 Markdown 格式，可用任何文本编辑器打开。流程图使用 Mermaid 语法，可在 [Mermaid Live Editor](https://mermaid.live) 在线预览。
