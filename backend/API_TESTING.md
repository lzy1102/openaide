# API 测试文档 | API Testing Guide

本文档提供常用 API 的 curl 测试示例。

## 基础 URL

```
http://localhost:19375
```

## 健康检查

```bash
curl http://localhost:19375/health
```

---

## 认证 API

### 用户注册

```bash
curl -X POST http://localhost:19375/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123"
  }'
```

### 用户登录

```bash
curl -X POST http://localhost:19375/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

---

## 模型管理 API

### 列出所有模型

```bash
curl http://localhost:19375/api/models
```

### 创建 OpenAI 模型

```bash
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gpt-4",
    "type": "llm",
    "provider": "openai",
    "api_key": "sk-your-openai-api-key",
    "base_url": "https://api.openai.com/v1",
    "config": {
      "model": "gpt-4",
      "timeout": 60
    }
  }'
```

### 创建 DeepSeek 模型

```bash
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "deepseek-chat",
    "type": "llm",
    "provider": "deepseek",
    "api_key": "your-deepseek-api-key",
    "config": {
      "model": "deepseek-chat"
    }
  }'
```

### 创建通义千问模型

```bash
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "qwen-turbo",
    "type": "llm",
    "provider": "qwen",
    "api_key": "your-dashscope-api-key",
    "base_url": "https://dashscope.aliyuncs.com/compatible-mode/v1",
    "config": {
      "model": "qwen-turbo"
    }
  }'
```

### 创建 Ollama 本地模型 (无需 API Key)

```bash
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "llama2-local",
    "type": "llm",
    "provider": "ollama",
    "base_url": "http://localhost:11434/v1",
    "config": {
      "model": "llama2"
    }
  }'
```

### 启用/禁用模型

```bash
# 启用
curl -X POST http://localhost:19375/api/models/{model_id}/enable

# 禁用
curl -X POST http://localhost:19375/api/models/{model_id}/disable
```

---

## 聊天 API

### 发送聊天消息

```bash
curl -X POST http://localhost:19375/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "model-id-here",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "Hello!"}
    ],
    "options": {
      "temperature": 0.7,
      "max_tokens": 2048
    }
  }'
```

### 流式聊天 (SSE)

```bash
curl -X POST http://localhost:19375/api/chat/stream \
  -H "Content-Type: application/json" \
  -d '{
    "model_id": "model-id-here",
    "messages": [
      {"role": "user", "content": "写一首关于春天的诗"}
    ],
    "options": {
      "temperature": 0.8
    }
  }'
```

---

## 对话 API

### 创建对话

```bash
curl -X POST http://localhost:19375/api/dialogues \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "title": "New Conversation"
  }'
```

### 发送对话消息

```bash
curl -X POST http://localhost:19375/api/dialogues/{dialogue_id}/messages \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "content": "你好，请介绍一下你自己",
    "model_id": "model-id-here"
  }'
```

### 流式对话

```bash
curl -X POST http://localhost:19375/api/dialogues/{dialogue_id}/stream \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "user-123",
    "content": "请详细解释量子计算",
    "model_id": "model-id-here"
  }'
```

---

## 知识库 API

### 创建知识条目

```bash
curl -X POST http://localhost:19375/api/knowledge \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Go 语言基础",
    "content": "Go 是一门静态类型、编译型编程语言...",
    "category": "programming",
    "tags": ["go", "programming", "backend"]
  }'
```

### 搜索知识

```bash
curl -X POST http://localhost:19375/api/knowledge/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "如何学习 Go 语言",
    "limit": 10
  }'
```

### 导入文档

```bash
curl -X POST http://localhost:19375/api/documents/import \
  -F "file=@document.pdf" \
  -F "category=documents"
```

---

## RAG API

### RAG 查询

```bash
curl -X POST http://localhost:19375/api/rag/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "什么是机器学习？",
    "top_k": 5,
    "options": {
      "temperature": 0.7
    }
  }'
```

### 流式 RAG 查询

```bash
curl -X POST http://localhost:19375/api/rag/stream \
  -H "Content-Type: application/json" \
  -d '{
    "query": "请解释深度学习的原理",
    "top_k": 5
  }'
```

---

## 任务管理 API

### 分解任务

```bash
curl -X POST http://localhost:19375/api/tasks/decompose \
  -H "Content-Type: application/json" \
  -d '{
    "title": "开发一个 Web 应用",
    "description": "使用 Go 和 React 开发一个博客系统",
    "complexity": "high"
  }'
```

### 列出任务

```bash
curl http://localhost:19375/api/tasks
```

### 更新任务状态

```bash
curl -X PATCH http://localhost:19375/api/tasks/{task_id}/status \
  -H "Content-Type: application/json" \
  -d '{
    "status": "in_progress"
  }'
```

---

## 上下文管理 API

### 压缩上下文

```bash
curl -X POST http://localhost:19375/api/context/compress \
  -H "Content-Type: application/json" \
  -d '{
    "dialogue_id": "dialogue-id-here"
  }'
```

### 生成摘要

```bash
curl -X POST http://localhost:19375/api/context/summarize \
  -H "Content-Type: application/json" \
  -d '{
    "dialogue_id": "dialogue-id-here"
  }'
```

---

## 知识提取 API

### 从对话提取知识

```bash
curl -X POST http://localhost:19375/api/extraction/extract \
  -H "Content-Type: application/json" \
  -d '{
    "dialogue_id": "dialogue-id-here"
  }'
```

### 自动提取配置

```bash
# 获取配置
curl http://localhost:19375/api/extraction/config

# 更新配置
curl -X PUT http://localhost:19375/api/extraction/config \
  -H "Content-Type: application/json" \
  -d '{
    "auto_extract": true,
    "min_content_length": 100
  }'
```

---

## WebSocket API

### 连接 WebSocket

```javascript
// JavaScript 示例
const ws = new WebSocket('ws://localhost:19375/ws');

ws.onopen = () => {
  console.log('Connected');
  ws.send(JSON.stringify({
    type: 'subscribe',
    channel: 'chat'
  }));
};

ws.onmessage = (event) => {
  console.log('Received:', JSON.parse(event.data));
};
```

### 获取 WebSocket 统计

```bash
curl http://localhost:19375/api/ws/stats
```

---

## 测试脚本

将以下内容保存为 `test_api.sh`：

```bash
#!/bin/bash

BASE_URL="http://localhost:19375"

echo "Testing API endpoints..."

# 1. Health check
echo "1. Health check..."
curl -s $BASE_URL/health | jq .

# 2. Register user
echo "2. Register user..."
curl -s -X POST $BASE_URL/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","email":"test@test.com","password":"test123"}' | jq .

# 3. Login
echo "3. Login..."
TOKEN=$(curl -s -X POST $BASE_URL/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@test.com","password":"test123"}' | jq -r '.access_token')

echo "Token: $TOKEN"

# 4. List models
echo "4. List models..."
curl -s -H "Authorization: Bearer $TOKEN" $BASE_URL/api/models | jq .

echo "Done!"
```
