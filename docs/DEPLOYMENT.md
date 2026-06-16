# OpenAIDE 部署指南

> 版本: v3.0.0
> 日期: 2026-05-16
> 适用场景: 用户级安装，每个用户独立部署

---

## 目录

1. [目录结构](#1-目录结构)
2. [一键安装](#2-一键安装)
3. [本地编译安装](#3-本地编译安装)
4. [Docker 部署](#4-docker-部署)
5. [配置说明](#5-配置说明)
6. [服务管理](#6-服务管理)
7. [故障排除](#7-故障排除)

---

## 1. 目录结构

OpenAIDE 安装在用户目录 `~/.openaide`，每个用户完全独立：

| 路径 | 说明 |
|------|------|
| `~/.openaide/bin/openaide` | 统一二进制 (CLI + 服务器) |
| `~/.openaide/config.yaml` | 用户配置文件 (YAML 格式，支持注释) |
| `~/.openaide/data/` | 用户数据（会话、记忆、知识库） |
| `~/.openaide/logs/` | 用户日志 |
| `~/.openaide/start.sh` | 启动脚本 |
| `~/.openaide/stop.sh` | 停止脚本 |

---

## 2. 一键安装

### 2.1 从 GitHub Release 安装（推荐）

```bash
# 使用 curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
```

### 2.2 指定版本安装

```bash
VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/install.sh | bash
```

### 2.3 安装后配置

```bash
# 编辑配置文件，填入 API Key
vim ~/.openaide/config.yaml

# 启动服务
~/.openaide/start.sh
```

---

## 3. 本地编译安装

适用于开发调试或修改代码后安装：

```bash
# 1. 克隆代码到用户目录
git clone https://github.com/lzy1102/openaide.git ~/.openaide
cd ~/.openaide

# 2. 运行安装脚本（本地编译模式）
bash install.sh --local
```

---

## 4. Docker 部署

```bash
cd ~/.openaide/backend

# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f
```

---

## 5. 配置说明

### 5.1 配置文件位置

```
~/.openaide/config.yaml
```

### 5.2 最小可用配置

```yaml
# 服务器配置
server:
  host: 0.0.0.0
  port: 8080
  mode: server

# LLM 配置 (必填: 替换 api_key)
llm:
  default_provider: deepseek
  providers:
    - name: deepseek
      type: openai-compatible
      base_url: https://api.deepseek.com/v1
      api_key: "sk-your-real-api-key"
      default_model: deepseek-v4-pro
      timeout: 60
      enabled: true
```

### 5.3 支持的 LLM 提供商

| 提供商 | type | base_url |
|--------|------|----------|
| OpenAI | `openai` | `https://api.openai.com/v1` |
| DeepSeek | `openai-compatible` | `https://api.deepseek.com/v1` |
| 阿里云百炼 | `openai-compatible` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| Ollama (本地) | `openai-compatible` | `http://localhost:11434/v1` |

---

## 6. 服务管理

```bash
# 启动服务
~/.openaide/start.sh

# 停止服务
~/.openaide/stop.sh

# 查看日志
tail -f ~/.openaide/logs/server.log

# 使用 CLI
openaide
# 或
~/.openaide/bin/openaide
```

---

## 7. 故障排除

### 7.1 服务无法启动

```bash
# 检查配置文件格式
~/.openaide/bin/openaide server --config ~/.openaide/config.yaml

# 查看日志
cat ~/.openaide/logs/server.log
```

### 7.2 API 无响应

```bash
# 检查服务状态
curl http://localhost:8080/health

# 检查端口监听
ss -tlnp | grep 8080
```

### 7.3 命令找不到

```bash
# 添加 PATH
export PATH="$HOME/.openaide/bin:$PATH"

# 或永久添加
echo 'export PATH="$HOME/.openaide/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc
```
