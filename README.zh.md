# OpenAIDE — AI Agent 内核

[![Build](https://github.com/lzy1102/openaide/actions/workflows/build-deploy.yml/badge.svg)](https://github.com/lzy1102/openaide/actions)
[![npm](https://img.shields.io/npm/v/@lzy1102/openaide)](https://www.npmjs.com/package/@lzy1102/openaide)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](https://go.dev/)

[English](README.md) | [中文](#)

**Go 实现的 AI Agent 内核——知识积累、自主进化、每次任务都在变聪明。** SQLite · 向量 ANN · 40 工具 · 24 语言 LSP

> 反思 → 知识精炼 → 技能提取 —— 越用越聪明。

---

## 快速开始

```bash
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
openaide setup
openaide
```

## 安装

```bash
# curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# npm
npm install -g @lzy1102/openaide

# Windows
curl -o install.bat https://raw.githubusercontent.com/lzy1102/openaide/master/install.bat && install.bat
```

## 使用

```bash
openaide                # 交互式 REPL
openaide "修复这个 bug"  # 一次性问答
openaide -c             # 恢复上次会话
openaide setup          # 配置向导
openaide server         # 启动 API 服务
```

## 架构

```
openaide server (API)    openaide (REPL)
         ↓                     ↓
    orchestration/         agent/kernel/
    Plan → Execute         ReAct 循环
         ↓                     ↓
    llm/   tools/   memory/   knowledge/
    40+ 工具 · 向量 ANN · SQLite
```

## 核心能力

- **自主学习**: 每次任务后反思、提取技能、积累知识——越用越聪明
- **多 Agent 协作**: 分析师 → 编码 → 审查 → 执行，会话隔离
- **深度规划**: 研究 → 提案 → 选择 → 计划 → 执行——多轮深度思考
- **MemGPT 记忆**: Agent 主动管理记忆——归档、检索、核心事实
- **Claude 插件兼容**: 直接使用 Claude Code 生态的 skills、MCP、hooks
- **40+ 内置工具**: 文件、Git、Web、浏览器、LSP、桌面控制

## 配置

`~/.openaide/config.yaml`:

```yaml
llm:
  api_key: sk-xxx
  model: deepseek-v4-pro
  execution_model: deepseek-v4-flash
```

## 参与贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## License

MIT License — 见 [LICENSE](LICENSE)。
