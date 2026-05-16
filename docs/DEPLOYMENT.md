# OpenAIDE 部署指南

> 版本: v3.0.0
> 日期: 2026-05-16
> 适用场景: 服务器部署、Docker 部署

---

## 目录

1. [目录结构](#1-目录结构)
2. [一键部署](#2-一键部署)
3. [本地编译部署](#3-本地编译部署)
4. [Docker 部署](#4-docker-部署)
5. [配置说明](#5-配置说明)
6. [服务管理](#6-服务管理)
7. [故障排除](#7-故障排除)

---

## 1. 目录结构

OpenAIDE 统一安装在 `/opt/openaide`：

| 路径 | 说明 |
|------|------|
| `/opt/openaide/bin/openaide-server` | API 服务器 |
| `/opt/openaide/bin/openaide-cli` | 命令行客户端 |
| `/opt/openaide/config.json` | 主配置文件 |
| `/opt/openaide/data/` | 数据目录（会话、记忆、知识库） |
| `/opt/openaide/logs/` | 日志目录 |
| `/opt/openaide/scripts/deploy.sh` | 部署脚本 |

---

## 2. 一键部署

### 2.1 从 GitHub Release 部署（推荐）

```bash
# 使用 curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/scripts/deploy.sh | sudo bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/lzy1102/openaide/master/scripts/deploy.sh | sudo bash
```

### 2.2 指定版本部署

```bash
VERSION=v1.0.0 curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/scripts/deploy.sh | sudo bash
```

### 2.3 部署后配置

```bash
# 编辑配置文件，填入 API Key
sudo vim /opt/openaide/config.json

# 重启服务
sudo systemctl restart openaide
```

---

## 3. 本地编译部署

适用于开发调试或修改代码后部署：

```bash
# 1. 克隆代码
git clone https://github.com/lzy1102/openaide.git /opt/openaide
cd /opt/openaide

# 2. 运行部署脚本（本地编译模式）
sudo bash scripts/deploy.sh --local
```

---

## 4. Docker 部署

```bash
cd /opt/openaide/backend

# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f
```

---

## 5. 配置说明

### 5.1 配置文件位置

```
/opt/openaide/config.json
```

### 5.2 最小可用配置

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "mode": "server"
  },
  "llm": {
    "default_provider": "deepseek",
    "providers": [
      {
        "name": "deepseek",
        "type": "openai-compatible",
        "base_url": "https://api.deepseek.com/v1",
        "api_key": "sk-your-real-api-key",
        "default_model": "deepseek-v4-pro",
        "timeout": 60,
        "enabled": true
      }
    ]
  }
}
```

### 5.3 支持的 LLM 提供商

| 提供商 | type | base_url |
|--------|------|----------|
| OpenAI | `openai` | `https://api.openai.com/v1` |
| DeepSeek | `openai-compatible` | `https://api.deepseek.com/v1` |
| 阿里云百炼 | `openai-compatible` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| Ollama (本地) | `openai-compatible` | `http://localhost:11434/v1` |

### 5.4 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `OPENAIDE_HOME` | 数据根目录 | `/opt/openaide` |
| `OPENAIDE_CONFIG` | 配置文件路径 | `/opt/openaide/config.json` |

---

## 6. 服务管理

```bash
# 查看状态
sudo systemctl status openaide

# 启动/停止/重启
sudo systemctl start openaide
sudo systemctl stop openaide
sudo systemctl restart openaide

# 查看日志
sudo journalctl -u openaide -f
sudo tail -f /opt/openaide/logs/server.log

# 使用 CLI
openaide-cli
```

---

## 7. 故障排除

### 7.1 服务无法启动

```bash
# 检查配置文件格式
sudo /opt/openaide/bin/openaide-server -config /opt/openaide/config.json

# 查看详细日志
sudo journalctl -u openaide -n 100
```

### 7.2 API 无响应

```bash
# 检查服务状态
curl http://localhost:8080/health

# 检查端口监听
ss -tlnp | grep 8080
```

### 7.3 LLM 调用失败

```bash
# 测试 LLM 连接
curl https://api.deepseek.com/v1/models \
  -H "Authorization: Bearer your-api-key"
```
