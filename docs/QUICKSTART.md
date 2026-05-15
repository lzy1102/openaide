# OpenAIDE 快速入门指南

> 版本: v3.0.0-draft
> 日期: 2026-05-15
> 预计阅读时间: 10 分钟

---

## 目录

1. [安装](#1-安装)
2. [首次配置](#2-首次配置)
3. [基础使用](#3-基础使用)
4. [常用命令](#4-常用命令)
5. [工作流示例](#5-工作流示例)
6. [故障排除](#6-故障排除)

---

## 1. 安装

### 1.1 系统要求

| 项目 | 要求 |
|------|------|
| 操作系统 | Windows 10+/macOS 12+/Linux |
| Go 版本 | 1.21+ |
| 内存 | 4GB+ |
| 磁盘 | 1GB+ |

### 1.2 安装方式

#### 方式一：Go 安装（推荐）

```bash
go install github.com/openaide/openaide@latest
```

#### 方式二：二进制下载

```bash
# Linux/macOS
curl -fsSL https://openaide.dev/install.sh | bash

# Windows
iwr -useb https://openaide.dev/install.ps1 | iex
```

#### 方式三：Docker

```bash
docker run -it --rm \
  -v ~/.openaide:/root/.openaide \
  -v $(pwd):/workspace \
  openaide/openaide:latest
```

### 1.3 验证安装

```bash
openaide --version
# 输出: openaide v3.0.0
```

---

## 2. 首次配置

### 2.1 自动引导

首次运行会自动创建配置：

```bash
openaide
```

### 2.2 手动配置

编辑配置文件 `~/.openaide/config.yaml`：

```yaml
# 必需：选择 LLM 提供商
provider: openai

# 必需：API 密钥
api_key: sk-xxxxxxxxxxxxxxxxxxxxxxxx

# 可选：模型选择
model: gpt-4o

# 可选：代理设置
proxy: http://127.0.0.1:7890
```

### 2.3 支持的提供商

| 提供商 | 配置示例 |
|--------|----------|
| OpenAI | `provider: openai` |
| Claude | `provider: anthropic` |
| 阿里云 | `provider: aliyun` |
| 智谱 | `provider: zhipu` |
| 本地 Ollama | `provider: ollama` |
| 自定义 | `provider: custom` |

---

## 3. 基础使用

### 3.1 启动会话

```bash
# 进入项目文件夹，启动新会话
cd ~/my-project
openaide

# 或使用完整路径
openaide /path/to/project
```

### 3.2 对话交互

```
$ openaide
OpenAIDE v3.0.0 | 项目: my-project | 会话: 2026-05-15-abc123

> 帮我分析一下这个项目的代码结构

[AI 分析中...]
这个项目是一个 Go Web 应用，包含以下模块：
- main.go: 入口文件
- handlers/: HTTP 处理器
- models/: 数据模型
...

> 帮我写一个用户登录接口

[AI 生成代码...]
```

### 3.3 会话管理

```bash
# 列出所有会话
openaide --list-sessions

# 恢复指定会话
openaide --resume <session-id>

# 删除会话
openaide --delete-session <session-id>
```

---

## 4. 常用命令

### 4.1 命令速查表

| 命令 | 说明 | 示例 |
|------|------|------|
| `openaide` | 启动新会话 | `openaide` |
| `openaide -c` | 继续上次会话 | `openaide -c` |
| `openaide -p <path>` | 指定项目路径 | `openaide -p ~/project` |
| `/clear` | 清空当前上下文 | 在会话中输入 |
| `/save` | 保存当前会话 | 在会话中输入 |
| `/exit` | 退出会话 | 在会话中输入 |

### 4.2 配置命令

```bash
# 查看当前配置
openaide config show

# 编辑配置
openaide config edit

# 验证配置
openaide config validate

# 切换提供商
openaide config set provider claude
```

### 4.3 知识库命令

```bash
# 添加文件到知识库
openaide knowledge add ./docs/api.md

# 搜索知识库
openaide knowledge search "用户认证流程"

# 列出知识库文件
openaide knowledge list

# 删除知识库文件
openaide knowledge remove api.md
```

---

## 5. 工作流示例

### 5.1 代码审查工作流

```bash
# 1. 进入项目目录
cd ~/my-project

# 2. 启动 OpenAIDE
openaide

# 3. 请求代码审查
> 请审查 src/auth.go 文件，找出潜在的安全问题

# 4. 根据建议修改
# 5. 再次确认
> 帮我确认修复是否完整
```

### 5.2 新功能开发工作流

```bash
# 1. 创建功能分支
git checkout -b feature/user-profile

# 2. 启动 OpenAIDE
openaide

# 3. 描述需求
> 帮我实现用户资料页面，需要支持头像上传和昵称修改

# 4. 迭代开发
> 头像上传需要限制大小为 2MB

# 5. 测试
> 帮我写单元测试

# 6. 提交代码
```

### 5.3 学习工作流

```bash
# 1. 创建学习项目
mkdir ~/learn-rust && cd ~/learn-rust

# 2. 启动 OpenAIDE
openaide

# 3. 开始学习
> 教我 Rust 的所有权概念

# 4. 实践
> 帮我写一个示例程序演示所有权转移

# 5. 保存笔记
> 把今天的学习内容保存到知识库
```

---

## 6. 故障排除

### 6.1 常见问题

#### 问题：无法连接 LLM 提供商

```
Error: connection refused to api.openai.com
```

**解决**：
```bash
# 检查网络连接
ping api.openai.com

# 配置代理
openaide config set proxy http://127.0.0.1:7890

# 或使用国内提供商
openaide config set provider aliyun
```

#### 问题：上下文超出限制

```
Warning: context length approaching limit
```

**解决**：
- 在会话中输入 `/clear` 清空上下文
- 或开启自动压缩：`openaide config set auto_compress true`

#### 问题：权限不足

```
Error: operation not permitted (guardian)
```

**解决**：
```bash
# 查看当前权限级别
openaide config show | grep permission

# 调整权限（谨慎使用）
openaide config set permission_level relaxed
```

### 6.2 获取帮助

```bash
# 查看帮助
openaide --help

# 查看具体命令帮助
openaide config --help

# 查看版本信息
openaide --version
```

### 6.3 日志位置

| 平台 | 路径 |
|------|------|
| Linux/macOS | `~/.openaide/logs/` |
| Windows | `%USERPROFILE%\.openaide\logs\` |

---

## 下一步

- 阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 了解架构设计
- 阅读 [MODULES.md](MODULES.md) 了解模块职责
- 阅读 [DEPLOYMENT.md](DEPLOYMENT.md) 了解部署方案

---

> **提示**: 所有数据存储在 `~/.openaide/` 目录下，可随时备份或迁移。
