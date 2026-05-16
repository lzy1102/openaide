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
- 配置文件：`~/.openaide/config.json`
- 日志：`~/.openaide/logs/server.log`

### 2. 本地编译部署

```bash
# 克隆代码到用户目录
git clone https://github.com/lzy1102/openaide.git ~/.openaide
cd ~/.openaide

# 运行部署脚本（本地编译模式）
bash scripts/deploy.sh --local
```

### 3. Docker 部署

```bash
cd ~/.openaide/backend
docker-compose up -d
```

---

## 配置说明

配置文件位置：`~/.openaide/config.json`

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
# 启动服务
~/.openaide/start.sh

# 停止服务
~/.openaide/stop.sh

# 查看日志
tail -f ~/.openaide/logs/server.log

# 使用 CLI
openaide-cli
```

---

## 项目结构

```
~/.openaide/
├── bin/
│   ├── openaide-server      # API 服务器
│   └── openaide-cli         # 命令行客户端
├── config.json              # 用户配置
├── data/                    # 用户数据
│   ├── memory/              # 记忆
│   ├── sessions/            # 会话
│   └── knowledge/           # 知识库
├── logs/                    # 日志
├── start.sh                 # 启动脚本
├── stop.sh                  # 停止脚本
├── backend/                 # 源代码 (如本地编译)
│   ├── cmd/
│   ├── internal/
│   └── ...
├── docs/                    # 文档
├── scripts/
│   └── deploy.sh            # 安装脚本
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

## 核心能力

- **ReAct Agent**: 思考→行动→观察循环，最大10轮，工具并行执行
- **9个内置工具**: 读写文件、执行命令、列目录、搜索文件、Git状态、知识库搜索/添加、代码符号搜索
- **5个Skill**: 代码审查、Git提交、调试助手、代码重构、代码解释（自动检测触发）
- **任务规划器**: 复杂请求自动拆分+子任务顺序执行+结果汇总
- **反思+学习**: 每轮自动反思评分，反馈到下一轮；学习模式存入insights
- **知识库**: 自动质量门控抽取（score>0.6），下次对话注入相关上下文
- **3级记忆**: L1工作/L2短期/L3长期，TF-IDF向量语义搜索
- **会话持久化**: 文件JSON存储，死机重启完整恢复
- **小说式上下文压缩**: 章节摘要+悬念钩子，超出窗口自动压缩
- **LLM网关**: OpenAI兼容+Anthropic原生+DeepSeek特殊支持(thinking/FIM)，故障转移
- **JWT认证**: HMAC-SHA256，注册/登录/中间件
- **WebSocket**: /ws端点，双向流式+心跳

## CLI

```bash
openaide           # Bubbletea TUI（推荐）
openaide update    # 更新
openaide version   # 版本
```

## API 端点

| 路由 | 说明 |
|------|------|
| `POST /api/v1/chat` | 聊天 (需认证) |
| `POST /api/v1/chat/stream` | 流式聊天 SSE (需认证) |
| `POST /api/v1/auth/register` | 注册 |
| `POST /api/v1/auth/login` | 登录获取JWT |
| `GET /api/v1/sessions` | 会话列表 (需认证) |
| `GET /api/v1/sessions/{id}` | 会话历史 (需认证) |
| `GET /api/v1/memory/search?q=xxx` | 记忆搜索 (需认证) |
| `GET /api/v1/tools` | 工具列表 (需认证) |
| `GET /api/v1/stats` | 系统统计 (需认证) |
| `GET /ws` | WebSocket (需认证) |
| `GET /health` | 健康检查 |

---

## License

MIT License
