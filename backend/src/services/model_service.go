package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ModelService 模型服务
type ModelService struct {
	db          *gorm.DB
	cache       *CacheService
	llmClients  map[string]llm.LLMClient
	clientsMu   sync.RWMutex
}

// NewModelService 创建模型服务实例
func NewModelService(db *gorm.DB, cache *CacheService) *ModelService {
	return &ModelService{
		db:         db,
		cache:      cache,
		llmClients: make(map[string]llm.LLMClient),
	}
}

// CreateModel 创建模型
func (s *ModelService) CreateModel(model *models.Model) error {
	model.ID = uuid.New().String()
	model.Status = "enabled"
	model.CreatedAt = time.Now()
	model.UpdatedAt = time.Now()
	return s.db.Create(model).Error
}

// UpdateModel 更新模型
func (s *ModelService) UpdateModel(model *models.Model) error {
	model.UpdatedAt = time.Now()

	// 清除客户端缓存,以便重新初始化
	s.clientsMu.Lock()
	delete(s.llmClients, model.ID)
	s.clientsMu.Unlock()

	return s.db.Save(model).Error
}

// DeleteModel 删除模型
func (s *ModelService) DeleteModel(id string) error {
	// 清除客户端缓存
	delete(s.llmClients, id)
	return s.db.Where("id = ?", id).Delete(&models.Model{}).Error
}

// GetModel 获取模型 (通过 ID 或名称)
func (s *ModelService) GetModel(idOrName string) (*models.Model, error) {
	var model models.Model

	// 先按 UUID 查询
	if len(idOrName) == 36 && strings.Contains(idOrName, "-") {
		if err := s.db.First(&model, "id = ?", idOrName).Error; err == nil {
			return &model, nil
		}
	}

	// 按 name 查询
	if err := s.db.Where("name = ?", idOrName).First(&model).Error; err == nil {
		return &model, nil
	}

	return nil, fmt.Errorf("model not found: %s", idOrName)
}

// ListModels 列出所有模型
func (s *ModelService) ListModels() ([]models.Model, error) {
	var models []models.Model
	err := s.db.Find(&models).Error
	return models, err
}

// ListEnabledModels 列出所有已启用的模型
func (s *ModelService) ListEnabledModels() ([]models.Model, error) {
	var models []models.Model
	err := s.db.Where("status = ?", "enabled").Find(&models).Error
	return models, err
}

// GetDefaultModel 获取默认模型（第一个 enabled 的 LLM 模型）
func (s *ModelService) GetDefaultModel() (*models.Model, error) {
	var model models.Model
	err := s.db.Where("status = ? AND type = ?", "enabled", "llm").Order("priority DESC, created_at ASC").First(&model).Error
	if err != nil {
		// 无 llm 类型则取任意 enabled
		err = s.db.Where("status = ?", "enabled").Order("priority DESC, created_at ASC").First(&model).Error
		if err != nil {
			return nil, fmt.Errorf("no enabled model available")
		}
	}
	return &model, nil
}

// EnableModel 启用模型
func (s *ModelService) EnableModel(id string) error {
	model, err := s.GetModel(id)
	if err != nil {
		return err
	}
	model.Status = "enabled"
	model.UpdatedAt = time.Now()
	return s.db.Save(model).Error
}

// DisableModel 禁用模型
func (s *ModelService) DisableModel(id string) error {
	model, err := s.GetModel(id)
	if err != nil {
		return err
	}
	model.Status = "disabled"
	model.UpdatedAt = time.Now()
	return s.db.Save(model).Error
}

// CreateModelInstance 创建模型实例
func (s *ModelService) CreateModelInstance(modelID string, config map[string]interface{}) (*models.ModelInstance, error) {
	model, err := s.GetModel(modelID)
	if err != nil {
		return nil, err
	}

	instance := &models.ModelInstance{
		ID:         uuid.New().String(),
		ModelID:    modelID,
		ModelName:  model.Name,
		Config:     config,
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = s.db.Create(instance).Error
	return instance, err
}

// ExecuteModel 执行模型（带缓存）
func (s *ModelService) ExecuteModel(instanceID string, parameters map[string]interface{}) (*models.ModelExecution, error) {
	// 构建缓存键
	paramsJSON, _ := json.Marshal(parameters)
	cacheKey := fmt.Sprintf("model:execute:%s:%x", instanceID, md5.Sum(paramsJSON))

	// 尝试从缓存获取
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.(*models.ModelExecution), nil
	}

	var instance models.ModelInstance
	err := s.db.First(&instance, instanceID).Error
	if err != nil {
		return nil, err
	}

	model, err := s.GetModel(instance.ModelID)
	if err != nil {
		return nil, err
	}

	execution := &models.ModelExecution{
		ID:         uuid.New().String(),
		ModelID:    model.ID,
		ModelName:  model.Name,
		InstanceID: instanceID,
		Parameters: parameters,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	// 执行模型逻辑
	result, err := s.executeModelLogic(model, &instance, parameters)
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		execution.EndedAt = time.Now()
		s.db.Create(execution)
		return execution, err
	}

	execution.Status = "completed"
	if resultMap, ok := result.(map[string]interface{}); ok {
		execution.Result = models.JSONMap(resultMap)
	}
	execution.EndedAt = time.Now()
	s.db.Create(execution)

	// 更新缓存，设置过期时间为30分钟
	s.cache.Set(cacheKey, execution, 30*time.Minute)
	return execution, nil
}

// executeModelLogic 执行模型逻辑
func (s *ModelService) executeModelLogic(model *models.Model, instance *models.ModelInstance, parameters map[string]interface{}) (interface{}, error) {
	switch model.Type {
	case "llm":
		return s.executeLLM(model, parameters)
	case "embedding":
		return s.executeEmbedding(model, parameters)
	default:
		return nil, fmt.Errorf("model type %s not implemented", model.Type)
	}
}

// executeLLM 执行LLM模型
func (s *ModelService) executeLLM(model *models.Model, parameters map[string]interface{}) (interface{}, error) {
	prompt, ok := parameters["prompt"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'prompt' is required")
	}

	// 获取或创建 LLM 客户端
	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// 构建聊天请求
	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}

	// 如果有历史消息,添加到请求中
	if history, ok := parameters["history"].([]llm.Message); ok {
		messages = append(history, messages...)
	}

	// 获取参数
	temperature := 0.7
	if t, ok := parameters["temperature"].(float64); ok {
		temperature = t
	}

	maxTokens := 2048
	if mt, ok := parameters["max_tokens"].(float64); ok {
		maxTokens = int(mt)
	}

	// 调用 LLM API
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req := &llm.ChatRequest{
		Model:       model.Name,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	resp, err := client.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("LLM API call failed: %w", err)
	}

	// 构建响应
	result := map[string]interface{}{
		"response": resp.Choices[0].Message.Content,
		"model":    resp.Model,
		"provider": model.Provider,
	}

	// 添加 token 使用情况
	if resp.Usage != nil {
		result["usage"] = map[string]int{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		}
	}

	return result, nil
}

// executeEmbedding 执行Embedding模型
func (s *ModelService) executeEmbedding(model *models.Model, parameters map[string]interface{}) (interface{}, error) {
	text, ok := parameters["text"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'text' is required")
	}

	// 模拟嵌入向量 (实际应该调用 embedding API)
	embedding := make([]float64, 10)
	for i := range embedding {
		embedding[i] = float64(i) * 0.1
	}

	return map[string]interface{}{
		"embedding": embedding,
		"text":      text,
		"model":     model.Name,
		"provider":  model.Provider,
	}, nil
}

// Chat 发送聊天请求 (对话服务专用)
func (s *ModelService) Chat(modelID string, messages []llm.Message, options map[string]interface{}) (*llm.ChatResponse, error) {
	var model *models.Model
	var err error

	if modelID != "" {
		model, err = s.GetModel(modelID)
		if err != nil {
			return nil, fmt.Errorf("model not found: %w", err)
		}
	} else {
		model, err = s.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no enabled model available: %w", err)
		}
	}

	if model.Status != "enabled" {
		return nil, fmt.Errorf("model is disabled")
	}

	// 获取 LLM 客户端
	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// 构建请求
	req := &llm.ChatRequest{
		Model:    model.Name,
		Messages: messages,
	}

	// 应用选项
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

	// 调用 API
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	return client.Chat(ctx, req)
}

// ChatWithTools 带工具定义的聊天请求 (用于 tool calling)
func (s *ModelService) ChatWithTools(modelID string, messages []llm.Message, tools []llm.ToolDefinition, options map[string]interface{}) (*llm.ChatResponse, error) {
	var model *models.Model
	var err error

	if modelID != "" {
		model, err = s.GetModel(modelID)
		if err != nil {
			return nil, fmt.Errorf("model not found: %w", err)
		}
	} else {
		model, err = s.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no enabled model available: %w", err)
		}
	}

	if model.Status != "enabled" {
		return nil, fmt.Errorf("model is disabled")
	}

	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	req := &llm.ChatRequest{
		Model:    model.Name,
		Messages: messages,
		Tools:    tools,
	}

	// 应用选项
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

	// 工具调用通常需要更长的超时
	timeout := 120 * time.Second
	if t, ok := options["timeout"].(int); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return client.Chat(ctx, req)
}

// ChatStream 发送流式聊天请求
func (s *ModelService) ChatStream(modelID string, messages []llm.Message, options map[string]interface{}) (<-chan llm.ChatStreamChunk, error) {
	var model *models.Model
	var err error

	if modelID != "" {
		model, err = s.GetModel(modelID)
		if err != nil {
			return nil, fmt.Errorf("model not found: %w", err)
		}
	} else {
		model, err = s.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no enabled model available: %w", err)
		}
	}

	if model.Status != "enabled" {
		return nil, fmt.Errorf("model is disabled")
	}

	// 获取 LLM 客户端
	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// 构建请求
	req := &llm.ChatRequest{
		Model:    model.Name,
		Messages: messages,
	}

	// 应用选项
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

	// 调用 API
	// 使用更长的超时时间，因为模型在 CPU 上运行可能需要几分钟
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)

	// 注意: 调用者需要负责在适当的时候调用 cancel()
	// 这里我们使用 context.Background() 的一个变体来避免资源泄漏
	go func() {
		<-ctx.Done()
		cancel()
	}()

	return client.ChatStream(ctx, req)
}

// getLLMClient 获取或创建 LLM 客户端
func (s *ModelService) getLLMClient(model *models.Model) (llm.LLMClient, error) {
	// 检查缓存
	s.clientsMu.RLock()
	if client, ok := s.llmClients[model.ID]; ok {
		s.clientsMu.RUnlock()
		return client, nil
	}
	s.clientsMu.RUnlock()

	// 确定提供商类型
	var provider llm.ProviderType
	switch model.Provider {
	// 原有提供商
	case "openai":
		provider = llm.ProviderOpenAI
	case "anthropic", "claude":
		provider = llm.ProviderAnthropic
	case "glm", "zhipu":
		provider = llm.ProviderGLM

	// 国内大模型
	case "qwen", "tongyi", "dashscope":
		provider = llm.ProviderQwen
	case "ernie", "wenxin", "baidu":
		provider = llm.ProviderErnie
	case "hunyuan", "tencent":
		provider = llm.ProviderHunyuan
	case "spark", "xunfei":
		provider = llm.ProviderSpark
	case "moonshot", "kimi":
		provider = llm.ProviderMoonshot
	case "baichuan":
		provider = llm.ProviderBaichuan
	case "minimax":
		provider = llm.ProviderMiniMax
	case "deepseek":
		provider = llm.ProviderDeepSeek

	// 国际大模型
	case "gemini", "google":
		provider = llm.ProviderGemini
	case "mistral":
		provider = llm.ProviderMistral
	case "cohere":
		provider = llm.ProviderCohere
	case "groq":
		provider = llm.ProviderGroq
	case "openrouter":
		provider = llm.ProviderOpenAI

	// 本地模型
	case "ollama", "local":
		provider = llm.ProviderOllama
	case "vllm":
		provider = llm.ProviderVLLM

	default:
		return nil, fmt.Errorf("unsupported provider: %s", model.Provider)
	}

	// 创建客户端配置
	// 默认超时 300 秒，给 CPU 运行的大模型足够时间
	// 使用 model.Config 中的 model 名称（如果有），否则使用 model.Name
	modelName := model.Name
	if model.Config != nil {
		if cfgModel, ok := model.Config["model"].(string); ok && cfgModel != "" {
			modelName = cfgModel
		}
	}

	config := &llm.ClientConfig{
		Provider:   provider,
		APIKey:     model.APIKey,
		BaseURL:    model.BaseURL,
		Model:      modelName,
		Timeout:    300,
		MaxRetries: 3,
		RetryDelay: 1000,
	}

	// 应用模型特定配置
	if model.Config != nil {
		if timeout, ok := model.Config["timeout"].(float64); ok {
			config.Timeout = int(timeout)
		}
		if maxRetries, ok := model.Config["max_retries"].(float64); ok {
			config.MaxRetries = int(maxRetries)
		}
		if retryDelay, ok := model.Config["retry_delay"].(float64); ok {
			config.RetryDelay = int(retryDelay)
		}
	}

	// 创建客户端
	client, err := llm.NewClient(config)
	if err != nil {
		return nil, err
	}

	// 缓存客户端
	s.clientsMu.Lock()
	s.llmClients[model.ID] = client
	s.clientsMu.Unlock()

	return client, nil
}

// GetModelExecutions 获取模型执行历史
func (s *ModelService) GetModelExecutions(modelID string) ([]models.ModelExecution, error) {
	var executions []models.ModelExecution
	err := s.db.Where("model_id = ?", modelID).Order("started_at DESC").Find(&executions).Error
	return executions, err
}

// ValidateModelConfig 验证模型配置
func (s *ModelService) ValidateModelConfig(model *models.Model) error {
	// 检查必需字段
	if model.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if model.Type == "" {
		return fmt.Errorf("model type is required")
	}
	if model.Provider == "" {
		return fmt.Errorf("model provider is required")
	}

	// 本地模型标识
	localProviders := map[string]bool{
		"ollama": true,
		"local":  true,
		"vllm":   true,
	}

	// 检查 LLM 模型的 API Key (本地模型可选)
	if model.Type == "llm" && model.APIKey == "" && !localProviders[model.Provider] {
		return fmt.Errorf("API key is required for LLM models")
	}

	// 验证提供商
	switch model.Provider {
	// 原有提供商
	case "openai", "anthropic", "claude", "glm", "zhipu":
		// 有效提供商

	// 国内大模型
	case "qwen", "tongyi", "dashscope", "ernie", "wenxin", "baidu",
		"hunyuan", "tencent", "spark", "xunfei", "moonshot", "kimi",
		"baichuan", "minimax", "deepseek":
		// 有效提供商

	// 国际大模型
	case "gemini", "google", "mistral", "cohere", "groq", "openrouter":
		// 有效提供商

	// 本地模型
	case "ollama", "local", "vllm":
		// 有效提供商 (本地模型不强制要求 API Key)

	default:
		return fmt.Errorf("unsupported provider: %s", model.Provider)
	}

	return nil
}

// GetLLMClient 获取默认 LLM 客户端
func (s *ModelService) GetLLMClient() llm.LLMClient {
	var model models.Model
	result := s.db.Where("type = ? AND status = ?", "llm", "enabled").First(&model)
	if result.Error != nil {
		return nil
	}

	client, err := s.getLLMClient(&model)
	if err != nil {
		return nil
	}

	return client
}
