package services

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/config"
	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// ModelService 模型服务 - 模型配置从配置文件加载，不依赖数据库
type ModelService struct {
	models     []config.ModelConfig // 从配置文件加载的模型列表
	modelsMu   sync.RWMutex
	cfg        *config.Config       // 配置引用，用于回写
	cache      *CacheService
	db         *gorm.DB             // 仅用于 model_instances / model_executions
	llmClients map[string]llm.LLMClient
	clientsMu  sync.RWMutex
}

// NewModelService 创建模型服务实例
func NewModelService(cfg *config.Config, cache *CacheService, db *gorm.DB) *ModelService {
	svc := &ModelService{
		models:     make([]config.ModelConfig, 0),
		cfg:        cfg,
		cache:      cache,
		db:         db,
		llmClients: make(map[string]llm.LLMClient),
	}
	// 从配置加载模型
	svc.ReloadModels()
	return svc
}

// ReloadModels 从配置文件重新加载模型列表
func (s *ModelService) ReloadModels() {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	if s.cfg != nil && len(s.cfg.Models) > 0 {
		s.models = make([]config.ModelConfig, len(s.cfg.Models))
		copy(s.models, s.cfg.Models)
	} else {
		s.models = make([]config.ModelConfig, 0)
	}
}

// saveConfig 保存配置到文件（线程安全）
func (s *ModelService) saveConfig() error {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	if s.cfg == nil {
		return fmt.Errorf("config is nil")
	}
	// 同步内存中的模型列表到配置
	s.cfg.Models = make([]config.ModelConfig, len(s.models))
	copy(s.cfg.Models, s.models)
	return config.Save(s.cfg)
}

// configToModel 将 config.ModelConfig 转换为 models.Model（用于兼容现有接口）
func configToModel(cfg config.ModelConfig, id string) *models.Model {
	return &models.Model{
		ID:          id,
		Name:        cfg.Name,
		Description: cfg.Description,
		Type:        cfg.Type,
		Provider:    cfg.Provider,
		Version:     cfg.Version,
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		Config:      models.JSONMap(cfg.Config),
		Status:      cfg.Status,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// modelToConfig 将 models.Model 转换为 config.ModelConfig
func modelToConfig(m *models.Model) config.ModelConfig {
	var cfgMap map[string]interface{}
	if m.Config != nil {
		cfgMap = map[string]interface{}(m.Config)
	}
	return config.ModelConfig{
		Name:        m.Name,
		Description: m.Description,
		Type:        m.Type,
		Provider:    m.Provider,
		Version:     m.Version,
		APIKey:      m.APIKey,
		BaseURL:     m.BaseURL,
		Config:      cfgMap,
		Status:      m.Status,
	}
}

// generateModelID 根据模型名称生成稳定的 ID
func generateModelID(name string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(name)))[:12]
}

// CreateModel 创建模型（追加到配置文件）
func (s *ModelService) CreateModel(model *models.Model) error {
	if model.Name == "" {
		return fmt.Errorf("model name is required")
	}

	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	// 检查是否已存在
	for _, m := range s.models {
		if m.Name == model.Name {
			return fmt.Errorf("model with name '%s' already exists", model.Name)
		}
	}

	if model.Status == "" {
		model.Status = "enabled"
	}
	if model.Type == "" {
		model.Type = "llm"
	}

	cfg := modelToConfig(model)
	s.models = append(s.models, cfg)

	// 回写配置文件
	if s.cfg != nil {
		s.cfg.Models = make([]config.ModelConfig, len(s.models))
		copy(s.cfg.Models, s.models)
		if err := config.Save(s.cfg); err != nil {
			// 回滚
			s.models = s.models[:len(s.models)-1]
			return fmt.Errorf("failed to save config: %w", err)
		}
	}

	model.ID = generateModelID(model.Name)
	model.CreatedAt = time.Now()
	model.UpdatedAt = time.Now()
	return nil
}

// UpdateModel 更新模型（修改配置文件）
func (s *ModelService) UpdateModel(model *models.Model) error {
	if model.ID == "" && model.Name == "" {
		return fmt.Errorf("model id or name is required")
	}

	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	// 查找并更新
	found := false
	for i, m := range s.models {
		match := false
		if model.ID != "" && generateModelID(m.Name) == model.ID {
			match = true
		}
		if model.Name != "" && m.Name == model.Name {
			match = true
		}
		if match {
			if model.Description != "" {
				s.models[i].Description = model.Description
			}
			if model.Type != "" {
				s.models[i].Type = model.Type
			}
			if model.Provider != "" {
				s.models[i].Provider = model.Provider
			}
			if model.Version != "" {
				s.models[i].Version = model.Version
			}
			if model.APIKey != "" {
				s.models[i].APIKey = model.APIKey
			}
			if model.BaseURL != "" {
				s.models[i].BaseURL = model.BaseURL
			}
			if model.Config != nil {
				s.models[i].Config = map[string]interface{}(model.Config)
			}
			if model.Status != "" {
				s.models[i].Status = model.Status
			}
			found = true

			// 清除客户端缓存
			s.clientsMu.Lock()
			delete(s.llmClients, generateModelID(m.Name))
			s.clientsMu.Unlock()
			break
		}
	}

	if !found {
		return fmt.Errorf("model not found: %s", model.ID)
	}

	// 回写配置文件
	if s.cfg != nil {
		s.cfg.Models = make([]config.ModelConfig, len(s.models))
		copy(s.cfg.Models, s.models)
		return config.Save(s.cfg)
	}
	return nil
}

// DeleteModel 删除模型（从配置文件移除）
func (s *ModelService) DeleteModel(id string) error {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	found := false
	newModels := make([]config.ModelConfig, 0, len(s.models))
	for _, m := range s.models {
		mid := generateModelID(m.Name)
		if mid == id || m.Name == id {
			found = true
			// 清除客户端缓存
			s.clientsMu.Lock()
			delete(s.llmClients, mid)
			s.clientsMu.Unlock()
			continue
		}
		newModels = append(newModels, m)
	}

	if !found {
		return fmt.Errorf("model not found: %s", id)
	}

	s.models = newModels

	// 回写配置文件
	if s.cfg != nil {
		s.cfg.Models = make([]config.ModelConfig, len(s.models))
		copy(s.cfg.Models, s.models)
		return config.Save(s.cfg)
	}
	return nil
}

// GetModel 获取模型 (通过 ID 或名称)
func (s *ModelService) GetModel(idOrName string) (*models.Model, error) {
	s.modelsMu.RLock()
	defer s.modelsMu.RUnlock()

	for _, m := range s.models {
		mid := generateModelID(m.Name)
		if mid == idOrName || m.Name == idOrName {
			return configToModel(m, mid), nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", idOrName)
}

// ListModels 列出所有模型
func (s *ModelService) ListModels() ([]models.Model, error) {
	s.modelsMu.RLock()
	defer s.modelsMu.RUnlock()

	result := make([]models.Model, 0, len(s.models))
	for _, m := range s.models {
		result = append(result, *configToModel(m, generateModelID(m.Name)))
	}
	return result, nil
}

// ListEnabledModels 列出所有已启用的模型
func (s *ModelService) ListEnabledModels() ([]models.Model, error) {
	s.modelsMu.RLock()
	defer s.modelsMu.RUnlock()

	result := make([]models.Model, 0)
	for _, m := range s.models {
		if m.Status == "enabled" {
			result = append(result, *configToModel(m, generateModelID(m.Name)))
		}
	}
	return result, nil
}

// GetDefaultModel 获取默认模型（第一个 enabled 的 LLM 模型）
func (s *ModelService) GetDefaultModel() (*models.Model, error) {
	s.modelsMu.RLock()
	defer s.modelsMu.RUnlock()

	// 先找 enabled 的 llm
	for _, m := range s.models {
		if m.Status == "enabled" && m.Type == "llm" {
			return configToModel(m, generateModelID(m.Name)), nil
		}
	}

	// 再找任意 enabled
	for _, m := range s.models {
		if m.Status == "enabled" {
			return configToModel(m, generateModelID(m.Name)), nil
		}
	}

	return nil, fmt.Errorf("no enabled model available")
}

// EnableModel 启用模型
func (s *ModelService) EnableModel(id string) error {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	for i, m := range s.models {
		if generateModelID(m.Name) == id || m.Name == id {
			s.models[i].Status = "enabled"
			if s.cfg != nil {
				s.cfg.Models = make([]config.ModelConfig, len(s.models))
				copy(s.cfg.Models, s.models)
				return config.Save(s.cfg)
			}
			return nil
		}
	}
	return fmt.Errorf("model not found: %s", id)
}

// DisableModel 禁用模型
func (s *ModelService) DisableModel(id string) error {
	s.modelsMu.Lock()
	defer s.modelsMu.Unlock()

	for i, m := range s.models {
		if generateModelID(m.Name) == id || m.Name == id {
			s.models[i].Status = "disabled"
			if s.cfg != nil {
				s.cfg.Models = make([]config.ModelConfig, len(s.models))
				copy(s.cfg.Models, s.models)
				return config.Save(s.cfg)
			}
			return nil
		}
	}
	return fmt.Errorf("model not found: %s", id)
}

// CreateModelInstance 创建模型实例（仍使用数据库）
func (s *ModelService) CreateModelInstance(modelID string, cfg map[string]interface{}) (*models.ModelInstance, error) {
	model, err := s.GetModel(modelID)
	if err != nil {
		return nil, err
	}

	instance := &models.ModelInstance{
		ID:        uuid.New().String(),
		ModelID:   modelID,
		ModelName: model.Name,
		Config:    cfg,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = s.db.Create(instance).Error
	return instance, err
}

// ExecuteModel 执行模型（带缓存）
func (s *ModelService) ExecuteModel(instanceID string, parameters map[string]interface{}) (*models.ModelExecution, error) {
	paramsJSON, _ := json.Marshal(parameters)
	cacheKey := fmt.Sprintf("model:execute:%s:%x", instanceID, md5.Sum(paramsJSON))

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

	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	messages := []llm.Message{
		{Role: "user", Content: prompt},
	}
	if history, ok := parameters["history"].([]llm.Message); ok {
		messages = append(history, messages...)
	}

	temperature := 0.7
	if t, ok := parameters["temperature"].(float64); ok {
		temperature = t
	}
	maxTokens := 2048
	if mt, ok := parameters["max_tokens"].(float64); ok {
		maxTokens = int(mt)
	}

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

	result := map[string]interface{}{
		"response": resp.Choices[0].Message.Content,
		"model":    resp.Model,
		"provider": model.Provider,
	}
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

	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	req := &llm.ChatRequest{
		Model:    model.Name,
		Messages: messages,
	}
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

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
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

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

	client, err := s.getLLMClient(model)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	req := &llm.ChatRequest{
		Model:    model.Name,
		Messages: messages,
	}
	if temp, ok := options["temperature"].(float64); ok {
		req.Temperature = temp
	}
	if maxTokens, ok := options["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}
	if system, ok := options["system"].(string); ok {
		req.System = system
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	go func() {
		<-ctx.Done()
		cancel()
	}()

	return client.ChatStream(ctx, req)
}

// getLLMClient 获取或创建 LLM 客户端
func (s *ModelService) getLLMClient(model *models.Model) (llm.LLMClient, error) {
	s.clientsMu.RLock()
	if client, ok := s.llmClients[model.ID]; ok {
		s.clientsMu.RUnlock()
		return client, nil
	}
	s.clientsMu.RUnlock()

	var provider llm.ProviderType
	switch model.Provider {
	case "openai":
		provider = llm.ProviderOpenAI
	case "anthropic", "claude":
		provider = llm.ProviderAnthropic
	case "glm", "zhipu":
		provider = llm.ProviderGLM
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
	case "ollama", "local":
		provider = llm.ProviderOllama
	case "vllm":
		provider = llm.ProviderVLLM
	default:
		return nil, fmt.Errorf("unsupported provider: %s", model.Provider)
	}

	modelName := model.Name
	if model.Config != nil {
		if cfgModel, ok := model.Config["model"].(string); ok && cfgModel != "" {
			modelName = cfgModel
		}
	}

	cfg := &llm.ClientConfig{
		Provider:   provider,
		APIKey:     model.APIKey,
		BaseURL:    model.BaseURL,
		Model:      modelName,
		Timeout:    300,
		MaxRetries: 3,
		RetryDelay: 1000,
	}

	if model.Config != nil {
		if timeout, ok := model.Config["timeout"].(float64); ok {
			cfg.Timeout = int(timeout)
		}
		if maxRetries, ok := model.Config["max_retries"].(float64); ok {
			cfg.MaxRetries = int(maxRetries)
		}
		if retryDelay, ok := model.Config["retry_delay"].(float64); ok {
			cfg.RetryDelay = int(retryDelay)
		}
	}

	client, err := llm.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	s.clientsMu.Lock()
	s.llmClients[model.ID] = client
	s.clientsMu.Unlock()

	return client, nil
}

// GetModelExecutions 获取模型执行历史（仍使用数据库）
func (s *ModelService) GetModelExecutions(modelID string) ([]models.ModelExecution, error) {
	var executions []models.ModelExecution
	err := s.db.Where("model_id = ?", modelID).Order("started_at DESC").Find(&executions).Error
	return executions, err
}

// ValidateModelConfig 验证模型配置
func (s *ModelService) ValidateModelConfig(model *models.Model) error {
	if model.Name == "" {
		return fmt.Errorf("model name is required")
	}
	if model.Type == "" {
		return fmt.Errorf("model type is required")
	}
	if model.Provider == "" {
		return fmt.Errorf("model provider is required")
	}

	localProviders := map[string]bool{
		"ollama": true,
		"local":  true,
		"vllm":   true,
	}

	if model.Type == "llm" && model.APIKey == "" && !localProviders[model.Provider] {
		return fmt.Errorf("API key is required for LLM models")
	}

	switch model.Provider {
	case "openai", "anthropic", "claude", "glm", "zhipu",
		"qwen", "tongyi", "dashscope", "ernie", "wenxin", "baidu",
		"hunyuan", "tencent", "spark", "xunfei", "moonshot", "kimi",
		"baichuan", "minimax", "deepseek",
		"gemini", "google", "mistral", "cohere", "groq", "openrouter",
		"ollama", "local", "vllm":
		// valid
	default:
		return fmt.Errorf("unsupported provider: %s", model.Provider)
	}

	return nil
}

// GetLLMClient 获取默认 LLM 客户端
func (s *ModelService) GetLLMClient() llm.LLMClient {
	model, err := s.GetDefaultModel()
	if err != nil {
		return nil
	}

	client, err := s.getLLMClient(model)
	if err != nil {
		return nil
	}

	return client
}
