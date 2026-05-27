# OpenAIDE API 文档

Base URL: `http://localhost:8080/api/v1`

## 认证

JWT 认证默认关闭。开启后在请求头中携带：
```
Authorization: Bearer <token>
```

### 注册
**POST** `/api/v1/auth/register`
```json
{"username": "user1", "password": "pass123"}
```

### 登录
**POST** `/api/v1/auth/login`
```json
{"username": "user1", "password": "pass123"}
```
响应: `{"token": "eyJ...", "user": {"id": "...", "username": "user1"}}`

---

## 聊天

### 同步聊天
**POST** `/api/v1/chat`
```json
{
  "message": "Hello",
  "user_id": "user-001",
  "project_id": "default",
  "model": "deepseek-v4-pro",
  "temperature": 0.7,
  "max_tokens": 4000,
  "tools": ["read_file", "write_file"]
}
```
响应:
```json
{
  "content": "Hello! How can I help?",
  "tool_calls": 2,
  "tokens_used": 1500,
  "duration_ms": 3200,
  "model": "deepseek-v4-pro"
}
```

### 流式聊天 (SSE)
**POST** `/api/v1/chat/stream`

请求体同上。响应为 SSE 流 (`text/event-stream`):

| Event | 说明 |
|-------|------|
| `data: {"type":"content","content":"..."}` | 文本内容 |
| `data: {"type":"thinking","content":"..."}` | 推理/思考 |
| `data: {"type":"tool_call","tool_name":"...","tool_call_id":"..."}` | 工具调用开始 |
| `data: {"type":"tool_done","tool_name":"...","tool_result":...}` | 工具执行完成 |
| `data: {"type":"progress","round":3,"total_rounds":10}` | ReAct 进度 |
| `data: {"type":"done","tokens_used":2500}` | 流结束 |
| `data: {"type":"error","error":"..."}` | 错误 |

---

## 会话

### 列表
**GET** `/api/v1/sessions?user_id=user-001&project_id=default&limit=20&offset=0`

响应: `[{"id": "...", "project_id": "...", "user_id": "...", "message_count": 12, "created_at": "...", "updated_at": "..."}]`

### 创建
**POST** `/api/v1/sessions`
```json
{"user_id": "user-001", "project_id": "default"}
```

### 详情
**GET** `/api/v1/sessions/{id}`

响应:
```json
{
  "session": {"id": "...", "project_id": "...", "user_id": "...", "message_count": 5},
  "messages": [{"role": "user", "content": "..."}, {"role": "assistant", "content": "..."}]
}
```

### 删除
**DELETE** `/api/v1/sessions/{id}`

---

## 项目

### 列表
**GET** `/api/v1/projects`

### 创建
**POST** `/api/v1/projects`
```json
{"name": "my-project", "path": "/home/user/project"}
```

### 单个项目
**GET/PUT/DELETE** `/api/v1/projects/{id}`

---

## 配置

### 读取
**GET** `/api/v1/config` → 返回 YAML 文本

### 写入
**PUT** `/api/v1/config` → body 为 YAML 文本

---

## 其他

| 路由 | 方法 | 说明 |
|------|------|------|
| `/api/v1/memory/search?q=xxx` | GET | 记忆语义搜索 |
| `/api/v1/tools` | GET | 工具定义列表 |
| `/api/v1/stats` | GET | 系统统计 |
| `/api/v1/metrics` | GET | 运行时指标 (requests/tokens/errors) |
| `/api/v1/channels` | GET | 外部渠道列表 |
| `/ws` | GET | WebSocket 连接 |
| `/health` | GET | 健康检查 |

## 错误响应

```json
{"error": "error message"}
```

HTTP 状态码: `400` 参数错误, `401` 未认证, `404` 未找到, `500` 服务器错误。
