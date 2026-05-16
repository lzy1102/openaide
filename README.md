# OpenAIDE | AI Agent 内核平台

基于全新架构的 AI Agent 内核，采用分层设计：Kernel → Orchestration → API → Infrastructure。

---

## 快速开始

### 1. 一键部署（推荐）

从 GitHub 最新 Release 下载并部署：

```bash
# 使用 curl
curl -fsSL https://raw.githubusercontent.com/lzy1102/openaide/master/scripts/deploy.sh | sudo bash

# 或使用 wget
wget -qO- https://raw.githubusercontent.com/lzy1102/openaide/master/scripts/deploy.sh | sudo bash
```

部署完成后：
- 服务运行在 `http://localhost:8080`
- 配置文件：`/opt/openaide/config.json`
- 日志：`/opt/openaide/logs/server.log`

### 2. 本地编译部署

```bash
# 克隆代码
git clone https://github.com/lzy1102/openaide.git /opt/openaide
cd /opt/openaide

# 运行部署脚本（本地编译模式）
sudo bash scripts/deploy.sh --local
```

### 3. Docker 部署

```bash
cd /opt/openaide/backend
docker-compose up -d
```

---

## 配置说明

配置文件位置：`/opt/openaide/config.json`

首次部署会自动创建默认配置，**必须修改 API Key** 才能使用。

### 最小可用配置

```json
{
  "server": {
    "host": "0.0.0.0",
    "port": 8080,
    "mode": "server"
  },
  "llm": {
    "default_provider": "openai",
    "providers": [
      {
        "name": "openai",
        "type": "openai",
        "base_url": "https://api.openai.com/v1",
        "api_key": "sk-your-real-api-key",
        "default_model": "gpt-4o-mini",
        "timeout": 60,
        "enabled": true
      }
    ]
  }
}
```

### 配置项说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `server.host` | 监听地址 | `0.0.0.0` |
| `server.port` | 监听端口 | `8080` |
| `server.mode` | 运行模式：`server`/`direct` | `server` |
| `llm.default_provider` | 默认 LLM 提供商名称 | `""` |
| `llm.providers` | 提供商列表 | `[]` |
| `memory.data_dir` | 记忆数据目录 | `./data/memory` |
| `tools.dangerous_tools` | 危险工具列表 | `["execute_command", "write_file"]` |
| `kernel.max_rounds` | 最大 ReAct 轮数 | `10` |
| `log.level` | 日志级别：`debug`/`info`/`warn`/`error` | `info` |

### 支持的 LLM 提供商

| 提供商 | type | 示例 base_url |
|--------|------|--------------|
| OpenAI | `openai` | `https://api.openai.com/v1` |
| DeepSeek | `openai-compatible` | `https://api.deepseek.com/v1` |
| 阿里云百炼 | `openai-compatible` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| 本地 Ollama | `openai-compatible` | `http://localhost:11434/v1` |

---

## 部署方式对比

| 方式 | 适用场景 | 命令 |
|------|----------|------|
| **一键脚本** | 快速部署，生产环境 | `curl ... \| sudo bash` |
| **本地编译** | 开发调试，定制修改 | `sudo bash scripts/deploy.sh --local` |
| **GitHub Release** | 指定版本，离线环境 | `VERSION=v1.0.0 curl ... \| bash` |
| **Docker** | 容器化部署 | `docker-compose up -d` |

---

## 服务管理

```bash
# 启动/停止/重启
sudo systemctl start openaide
sudo systemctl stop openaide
sudo systemctl restart openaide

# 查看状态
sudo systemctl status openaide

# 查看日志
sudo journalctl -u openaide -f

# 使用 CLI
openaide-cli
```

---

## 项目结构

```
openaide/
├── backend/
│   ├── cmd/
│   │   ├── server/          # API 服务器入口
│   │   └── cli/             # 命令行客户端
│   ├── internal/
│   │   ├── kernel/          # AI Agent 内核 (ReAct 循环)
│   │   ├── llm/             # LLM 网关 (多提供商)
│   │   ├── tools/           # 工具系统
│   │   ├── memory/          # 记忆系统
│   │   ├── orchestration/   # 编排层
│   │   ├── api/             # RESTful API
│   │   ├── config/          # 配置管理
│   │   ├── git/             # Git 集成
│   │   ├── index/           # 代码索引
│   │   ├── knowledge/       # 知识库
│   │   ├── compress/        # 上下文压缩
│   │   ├── event/           # 事件系统
│   │   └── identity/        # 身份检测
│   ├── config.example.json  # 配置示例
│   └── Dockerfile
├── docs/                    # 架构文档
├── scripts/
│   └── deploy.sh            # 一键部署脚本
└── README.md
```

---

## GitHub Actions

每次推送到 `master` 分支会自动：
1. 编译 `openaide-server` 和 `openaide-cli`
2. 运行测试
3. 打包为 `openaide-linux-amd64.tar.gz`

打标签 `v*` 会自动创建 GitHub Release 并上传二进制文件。

```bash
# 发布新版本
git tag v1.0.0
git push origin v1.0.0
```

---

## API 端点

| 路由 | 说明 |
|------|------|
| `POST /api/v1/chat` | 聊天 |
| `POST /api/v1/chat/stream` | 流式聊天 |
| `GET /api/v1/sessions` | 会话列表 |
| `GET /api/v1/sessions/{id}` | 会话历史 |
| `GET /api/v1/memory/search?q=xxx` | 记忆搜索 |
| `GET /api/v1/tools` | 工具列表 |
| `GET /health` | 健康检查 |

---

## License

MIT License
