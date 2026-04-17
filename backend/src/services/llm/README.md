# LLM 客户端实现

## 概述

本目录包含了 OpenAIDE 项目的 LLM (Large Language Model) 客户端实现，支持 **17+** 个主流 LLM 提供商，包括国内大模型、国际大模型和本地部署模型。

## 支持的提供商

### 国内大模型

| Provider | 别名 | 默认模型 | API 格式 |
|----------|------|----------|----------|
| 通义千问 (Qwen) | `qwen`, `tongyi`, `dashscope` | qwen-turbo | OpenAI 兼容 |
| 文心一言 (ERNIE) | `ernie`, `wenxin`, `baidu` | ernie-bot-4 | 百度自定义 |
| 混元 (Hunyuan) | `hunyuan`, `tencent` | hunyuan-lite | 腾讯云签名 |
| 星火 (Spark) | `spark`, `xunfei` | spark-v3.5 | 讯飞自定义 |
| Moonshot (Kimi) | `moonshot`, `kimi` | moonshot-v1-8k | OpenAI 兼容 |
| 百川 (Baichuan) | `baichuan` | Baichuan2-Turbo | OpenAI 兼容 |
| MiniMax | `minimax` | abab5.5-chat | OpenAI 兼容 |
| DeepSeek | `deepseek` | deepseek-chat | OpenAI 兼容 |
| 智谱 GLM | `glm`, `zhipu` | glm-5 | OpenAI 兼容 |

### 国际大模型

| Provider | 别名 | 默认模型 | API 格式 |
|----------|------|----------|----------|
| OpenAI | `openai` | gpt-4 | OpenAI 原生 |
| Anthropic (Claude) | `anthropic`, `claude` | claude-3-sonnet | Anthropic 自定义 |
| Google Gemini | `gemini`, `google` | gemini-pro | Google REST |
| Mistral AI | `mistral` | mistral-large-latest | OpenAI 兼容 |
| Cohere | `cohere` | command | Cohere 自定义 |
| Groq | `groq` | llama2-70b-4096 | OpenAI 兼容 |

### 本地/开源模型

| Provider | 别名 | 默认地址 | 说明 |
|----------|------|----------|------|
| Ollama | `ollama`, `local` | localhost:11434 | 本地模型运行 |
| vLLM | `vllm` | localhost:19375 | 高性能推理 |

## 架构

### 核心接口

```go
// LLMClient 统一的 LLM 客户端接口
type LLMClient interface {
    // Chat 发送聊天请求并返回响应
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

    // ChatStream 发送聊天请求并返回流式响应
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamChunk, error)
}
```

### 实现结构

```
llm/
├── llm_client.go                # 核心接口和类型定义
├── openai_client.go             # OpenAI 原生客户端
├── anthropic_client.go          # Anthropic (Claude) 客户端
├── glm_client.go                # 智谱 AI 客户端
│
├── openai_compatible_client.go  # OpenAI 兼容客户端基类
│
├── qwen_client.go               # 通义千问客户端
├── ernie_client.go              # 文心一言客户端
├── hunyuan_client.go            # 腾讯混元客户端
├── spark_client.go              # 讯飞星火客户端
├── moonshot_client.go           # Moonshot (Kimi) 客户端
├── baichuan_client.go           # 百川客户端
├── minimax_client.go            # MiniMax 客户端
├── deepseek_client.go           # DeepSeek 客户端
│
├── gemini_client.go             # Google Gemini 客户端
├── mistral_client.go            # Mistral AI 客户端
├── cohere_client.go             # Cohere 客户端
├── groq_client.go               # Groq 客户端
│
├── ollama_client.go             # Ollama 本地模型客户端
└── vllm_client.go               # vLLM 客户端
```

## 使用方法

### 1. 创建模型配置 (通过 API)

```bash
# 创建通义千问模型
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

# 创建 DeepSeek 模型
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

# 创建 Ollama 本地模型 (无需 API Key)
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

# 创建智谱 GLM-5 模型
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "glm-5",
    "type": "llm",
    "provider": "glm",
    "api_key": "your-api-key-id.secret",
    "base_url": "https://open.bigmodel.cn/api/paas/v4",
    "config": {
      "model": "glm-5",
      "timeout": 60
    }
  }'

# 创建智谱 GLM-4.7 模型 (编程增强版, Coding Plan)
curl -X POST http://localhost:19375/api/models \
  -H "Content-Type: application/json" \
  -d '{
    "name": "glm-4.7",
    "type": "llm",
    "provider": "glm",
    "api_key": "your-api-key-id.secret",
    "base_url": "https://open.bigmodel.cn/api/coding/paas/v4",
    "config": {
      "model": "glm-4.7",
      "timeout": 60
    }
  }'
```

### 2. 使用 Go 代码创建客户端

```go
import "assistant/backend/src/services/llm"

config := &llm.ClientConfig{
    Provider:   llm.ProviderDeepSeek,
    APIKey:     "your-api-key",
    BaseURL:    "https://api.deepseek.com",
    Model:      "deepseek-chat",
    Timeout:    60,
    MaxRetries: 3,
    RetryDelay: 1000,
}

client, err := llm.NewClient(config)
if err != nil {
    log.Fatal(err)
}
```

### 3. 发送聊天请求

```go
req := &llm.ChatRequest{
    Model: "deepseek-chat",
    Messages: []llm.Message{
        {Role: "system", Content: "You are a helpful assistant."},
        {Role: "user", Content: "你好!"},
    },
    Temperature: 0.7,
    MaxTokens:   2048,
}

resp, err := client.Chat(context.Background(), req)
if err != nil {
    log.Fatal(err)
}

fmt.Println(resp.Choices[0].Message.Content)
```

### 4. 流式响应

```go
chunkChan, err := client.ChatStream(context.Background(), req)
if err != nil {
    log.Fatal(err)
}

for chunk := range chunkChan {
    if chunk.Error != nil {
        log.Printf("Error: %v", chunk.Error)
        break
    }

    if len(chunk.Choices) > 0 {
        fmt.Print(chunk.Choices[0].Delta.Content)
    }
}
```

## API 端点

### 聊天 API

```
POST /api/chat
Content-Type: application/json

{
  "model_id": "model-id",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "options": {
    "temperature": 0.7,
    "max_tokens": 2048
  }
}
```

### 流式聊天 API

```
POST /api/chat/stream
Content-Type: application/json

{
  "model_id": "model-id",
  "messages": [
    {"role": "user", "content": "Hello!"}
  ],
  "options": {
    "temperature": 0.7,
    "max_tokens": 2048
  }
}
```

响应格式: Server-Sent Events (SSE)

## 配置说明

### 客户端配置

| 参数 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| Provider | ProviderType | LLM 提供商类型 | - |
| APIKey | string | API 密钥 | - |
| BaseURL | string | API 基础 URL | 提供商默认 |
| Model | string | 默认模型名称 | - |
| Timeout | int | 请求超时时间(秒) | 60 |
| MaxRetries | int | 最大重试次数 | 3 |
| RetryDelay | int | 重试延迟(毫秒) | 1000 |

### 请求参数

| 参数 | 类型 | 说明 |
|------|------|------|
| Model | string | 模型名称 |
| Messages | []Message | 消息列表 |
| Temperature | float64 | 温度参数 (0.0-2.0) |
| MaxTokens | int | 最大生成 token 数 |
| TopP | float64 | 采样参数 |
| Stop | []string | 停止序列 |
| PresencePenalty | float64 | 存在惩罚 |
| FrequencyPenalty | float64 | 频率惩罚 |

## 各提供商默认 BaseURL

| Provider | BaseURL |
|----------|---------|
| 通义千问 | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| 文心一言 | `https://aip.baidubce.com/rpc/2.0/ai_custom/v1/wenxinworkshop` |
| 混元 | `https://hunyuan.tencentcloudapi.com` |
| 星火 | `https://spark-api.xf-yun.com` |
| Moonshot | `https://api.moonshot.cn/v1` |
| 百川 | `https://api.baichuan-ai.com/v1` |
| MiniMax | `https://api.minimax.chat/v1` |
| DeepSeek | `https://api.deepseek.com` |
| 智谱 GLM | `https://open.bigmodel.cn/api/paas/v4` |
| 智谱 GLM Coding Plan | `https://open.bigmodel.cn/api/coding/paas/v4` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta` |
| Mistral | `https://api.mistral.ai/v1` |
| Cohere | `https://api.cohere.ai/v1` |
| Groq | `https://api.groq.com/openai/v1` |
| Ollama | `http://localhost:11434/v1` |

## 错误处理

客户端实现了完善的错误处理机制:

1. **请求重试**: 自动重试失败的请求
2. **超时控制**: 可配置的请求超时
3. **错误响应解析**: 解析并返回详细的错误信息

## 本地模式

OpenAIDE 支持本地模式，无需用户注册和认证即可使用。启用方式：

```bash
# 设置环境变量启用本地模式
export OPENAIDE_LOCAL_MODE=true

# 启动服务
go run cmd/main.go
```

在本地模式下：
- 所有 API 请求无需携带认证 Token
- 自动以管理员权限运行
- 适合个人本地使用场景

## 注意事项

1. **API Key 安全**: 不要在代码中硬编码 API Key，使用环境变量或配置文件
2. **速率限制**: 注意各提供商的速率限制
3. **成本控制**: 注意 token 使用量，控制成本
4. **流式响应**: 使用流式响应时确保正确关闭连接
5. **本地模型**: Ollama 和 vLLM 等本地模型无需 API Key
6. **智谱 API Key 格式**: 智谱 GLM 的 API Key 格式为 `id.secret`，直接作为 Bearer Token 使用
7. **智谱 Coding Plan**: GLM-4.7/GlM-5 编程增强版使用不同的 base_url `https://open.bigmodel.cn/api/coding/paas/v4`，模型名称不变

## 开发计划

- [x] 添加国内主流大模型支持
- [x] 添加国际主流大模型支持
- [x] 添加本地模型支持 (Ollama, vLLM)
- [x] 添加 GLM-5 / GLM-4.7 最新模型支持
- [x] 本地模式认证绕过
- [ ] 实现 Embedding API 统一接口
- [ ] 添加请求/响应中间件
- [ ] 实现更完善的缓存策略
- [ ] 添加请求批处理支持

## 常见问题

### 智谱 GLM 返回 429 错误

如果返回 `余额不足或无可用资源包,请充值` 错误 (HTTP 429)，说明：
- API Key 配置正确，认证成功
- 账户余额不足，需要充值
- 新用户可在 [智谱开放平台](https://open.bigmodel.cn/) 领取免费额度

### 如何添加新模型

系统支持 OpenAI 兼容格式的 API，添加新模型只需：

1. 在管理界面或通过 API 创建模型配置
2. 设置 `provider` 为对应提供商
3. 设置 `base_url` 为 API 地址
4. 设置 `api_key` 为您的密钥

对于 OpenAI 兼容的 API，可以设置 `provider` 为 `openai` 并指定正确的 `base_url`。
