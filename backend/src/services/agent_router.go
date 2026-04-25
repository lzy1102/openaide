package services

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type AgentModelConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type AgentRoutingConfig struct {
	AgentModels  map[string]AgentModelConfig `json:"agentModels"`
	AgentRouting map[string]string           `json:"agentRouting"`
}

type AgentRouter struct {
	mu     sync.RWMutex
	config AgentRoutingConfig
	svc    *ModelService
}

func NewAgentRouter(svc *ModelService) *AgentRouter {
	r := &AgentRouter{
		svc: svc,
		config: AgentRoutingConfig{
			AgentModels:  make(map[string]AgentModelConfig),
			AgentRouting: make(map[string]string),
		},
	}
	r.loadConfig()
	return r
}

func (r *AgentRouter) loadConfig() {
	home, _ := os.UserHomeDir()
	configPaths := []string{}
	if home != "" {
		configPaths = append(configPaths, filepath.Join(home, ".openaide", "agent_routing.json"))
	}
	configPaths = append(configPaths, filepath.Join(".", "agent_routing.json"))

	for _, path := range configPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg AgentRoutingConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			log.Printf("[AgentRouter] Failed to parse config %s: %v", path, err)
			continue
		}
		r.mu.Lock()
		if cfg.AgentModels != nil {
			r.config.AgentModels = cfg.AgentModels
		}
		if cfg.AgentRouting != nil {
			r.config.AgentRouting = cfg.AgentRouting
		}
		r.mu.Unlock()
		return
	}

	r.initDefaultRouting()
}

func (r *AgentRouter) initDefaultRouting() {
	r.mu.Lock()
	defer r.mu.Unlock()

	models, err := r.svc.ListModels()
	if err != nil || len(models) == 0 {
		return
	}

	var fastModel, codeModel, reasoningModel, defaultModel string
	for _, m := range models {
		for _, tag := range m.Tags {
			switch strings.TrimSpace(tag) {
			case "fast":
				if fastModel == "" {
					fastModel = m.ID
				}
			case "code":
				if codeModel == "" {
					codeModel = m.ID
				}
			case "reasoning":
				if reasoningModel == "" {
					reasoningModel = m.ID
				}
			}
		}
		if defaultModel == "" && m.Status == "enabled" {
			defaultModel = m.ID
		}
	}

	if defaultModel == "" && len(models) > 0 {
		defaultModel = models[0].ID
	}

	r.config.AgentRouting = map[string]string{
		"build":    defaultModel,
		"plan":     defaultModel,
		"explore":  defaultModel,
		"general":  defaultModel,
		"default":  defaultModel,
	}

	if reasoningModel != "" {
		r.config.AgentRouting["plan"] = reasoningModel
	}
	if fastModel != "" {
		r.config.AgentRouting["explore"] = fastModel
		r.config.AgentRouting["compact"] = fastModel
	}
	if codeModel != "" {
		r.config.AgentRouting["general"] = codeModel
	}
}

func (r *AgentRouter) Route(agentType string) (string, *AgentModelConfig) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normalized := normalizeAgentName(agentType)

	if modelName, ok := r.config.AgentRouting[normalized]; ok {
		if modelCfg, ok := r.config.AgentModels[modelName]; ok {
			return modelName, &modelCfg
		}
		return modelName, nil
	}

	if modelName, ok := r.config.AgentRouting["default"]; ok {
		if modelCfg, ok := r.config.AgentModels[modelName]; ok {
			return modelName, &modelCfg
		}
		return modelName, nil
	}

	return "", nil
}

func (r *AgentRouter) RouteModelID(agentType string) string {
	modelID, _ := r.Route(agentType)
	return modelID
}

func (r *AgentRouter) GetClientForAgent(agentType string) (string, error) {
	modelID := r.RouteModelID(agentType)
	if modelID == "" {
		return "", fmt.Errorf("no model found for agent: %s", agentType)
	}
	return modelID, nil
}

func (r *AgentRouter) UpdateConfig(config AgentRoutingConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if config.AgentModels != nil {
		r.config.AgentModels = config.AgentModels
	}
	if config.AgentRouting != nil {
		r.config.AgentRouting = config.AgentRouting
	}
}

func (r *AgentRouter) GetConfig() AgentRoutingConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := AgentRoutingConfig{
		AgentModels:  make(map[string]AgentModelConfig),
		AgentRouting: make(map[string]string),
	}
	for k, v := range r.config.AgentModels {
		safe := v
		safe.APIKey = maskKey(v.APIKey)
		result.AgentModels[k] = safe
	}
	for k, v := range r.config.AgentRouting {
		result.AgentRouting[k] = v
	}
	return result
}

func (r *AgentRouter) ListRoutes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range r.config.AgentRouting {
		result[k] = v
	}
	return result
}

func normalizeAgentName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.TrimSpace(name)

	switch name {
	case "build":
		return "build"
	case "plan":
		return "plan"
	case "explore":
		return "explore"
	case "general", "general-purpose":
		return "general"
	case "compact", "summarize":
		return "compact"
	default:
		return name
	}
}

func maskKey(key string) string {
	if len(key) <= 12 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
