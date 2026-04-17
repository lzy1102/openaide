package services

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// PluginService 插件服务
type PluginService struct {
	db    *gorm.DB
	cache *CacheService
}

// NewPluginService 创建插件服务实例
func NewPluginService(db *gorm.DB, cache *CacheService) *PluginService {
	return &PluginService{db: db, cache: cache}
}

// CreatePlugin 创建插件
func (s *PluginService) CreatePlugin(plugin *models.Plugin) error {
	plugin.ID = uuid.New().String()
	plugin.Status = "installed"
	plugin.CreatedAt = time.Now()
	plugin.UpdatedAt = time.Now()
	return s.db.Create(plugin).Error
}

// UpdatePlugin 更新插件
func (s *PluginService) UpdatePlugin(plugin *models.Plugin) error {
	plugin.UpdatedAt = time.Now()
	return s.db.Save(plugin).Error
}

// DeletePlugin 删除插件
func (s *PluginService) DeletePlugin(id string) error {
	return s.db.Where("id = ?", id).Delete(&models.Plugin{}).Error
}

// GetPlugin 获取插件
func (s *PluginService) GetPlugin(id string) (*models.Plugin, error) {
	var plugin models.Plugin
	err := s.db.First(&plugin, id).Error
	return &plugin, err
}

// ListPlugins 列出所有插件
func (s *PluginService) ListPlugins() ([]models.Plugin, error) {
	var plugins []models.Plugin
	err := s.db.Find(&plugins).Error
	return plugins, err
}

// EnablePlugin 启用插件
func (s *PluginService) EnablePlugin(id string) error {
	plugin, err := s.GetPlugin(id)
	if err != nil {
		return err
	}
	plugin.Status = "enabled"
	plugin.UpdatedAt = time.Now()
	return s.db.Save(plugin).Error
}

// DisablePlugin 禁用插件
func (s *PluginService) DisablePlugin(id string) error {
	plugin, err := s.GetPlugin(id)
	if err != nil {
		return err
	}
	plugin.Status = "disabled"
	plugin.UpdatedAt = time.Now()
	return s.db.Save(plugin).Error
}

// InstallPlugin 安装插件
func (s *PluginService) InstallPlugin(plugin *models.Plugin) error {
	// 这里实现插件安装逻辑
	// 实际项目中，这里应该从远程仓库下载插件并解压到指定目录
	plugin.ID = uuid.New().String()
	plugin.Status = "installed"
	plugin.CreatedAt = time.Now()
	plugin.UpdatedAt = time.Now()
	return s.db.Create(plugin).Error
}

// CreatePluginInstance 创建插件实例
func (s *PluginService) CreatePluginInstance(pluginID string, config map[string]interface{}) (*models.PluginInstance, error) {
	plugin, err := s.GetPlugin(pluginID)
	if err != nil {
		return nil, err
	}

	instance := &models.PluginInstance{
		ID:         uuid.New().String(),
		PluginID:   pluginID,
		PluginName: plugin.Name,
		Config:     config,
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err = s.db.Create(instance).Error
	return instance, err
}

// ExecutePlugin 执行插件（带缓存）
func (s *PluginService) ExecutePlugin(instanceID string, parameters map[string]interface{}) (*models.PluginExecution, error) {
	// 构建缓存键
	paramsJSON, _ := json.Marshal(parameters)
	cacheKey := fmt.Sprintf("plugin:execute:%s:%x", instanceID, md5.Sum(paramsJSON))
	
	// 尝试从缓存获取
	if cached, found := s.cache.Get(cacheKey); found {
		return cached.(*models.PluginExecution), nil
	}
	
	var instance models.PluginInstance
	err := s.db.First(&instance, instanceID).Error
	if err != nil {
		return nil, err
	}

	plugin, err := s.GetPlugin(instance.PluginID)
	if err != nil {
		return nil, err
	}

	execution := &models.PluginExecution{
		ID:         uuid.New().String(),
		PluginID:   plugin.ID,
		PluginName: plugin.Name,
		InstanceID: instanceID,
		Parameters: parameters,
		Status:     "running",
		StartedAt:  time.Now(),
	}

	// 这里实现插件执行逻辑
	// 实际项目中，这里应该调用插件的执行方法
	result, err := s.executePluginLogic(plugin, &instance, parameters)
	if err != nil {
		execution.Status = "failed"
		execution.Error = err.Error()
		execution.EndedAt = time.Now()
		s.db.Create(execution)
		return execution, err
	}

	execution.Status = "completed"
	if resultMap, ok := result.(map[string]interface{}); ok {
		execution.Result = &models.JSONAny{Data: resultMap}
	}
	execution.EndedAt = time.Now()
	s.db.Create(execution)
	
	// 更新缓存，设置过期时间
	// 对于天气和股票等实时数据，设置较短的过期时间
	expiration := 10 * time.Minute
	if plugin.Name == "weather_plugin" || plugin.Name == "stock_plugin" {
		expiration = 5 * time.Minute
	}
	s.cache.Set(cacheKey, execution, expiration)
	return execution, nil
}

// executePluginLogic 执行插件逻辑
func (s *PluginService) executePluginLogic(plugin *models.Plugin, instance *models.PluginInstance, parameters map[string]interface{}) (interface{}, error) {
	// 这里实现具体的插件执行逻辑
	// 实际项目中，这里应该根据插件类型调用不同的执行器
	switch plugin.Name {
	case "example_plugin":
		return s.executeExamplePlugin(parameters)
	case "weather_plugin":
		return s.executeWeatherPlugin(parameters)
	case "stock_plugin":
		return s.executeStockPlugin(parameters)
	default:
		return nil, fmt.Errorf("plugin %s not implemented", plugin.Name)
	}
}

// executeExamplePlugin 执行示例插件
func (s *PluginService) executeExamplePlugin(parameters map[string]interface{}) (interface{}, error) {
	// 示例插件实现
	message, ok := parameters["message"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'message' is required")
	}

	return map[string]string{
		"response": fmt.Sprintf("Plugin response: %s", message),
	}, nil
}

// executeWeatherPlugin 执行天气插件
func (s *PluginService) executeWeatherPlugin(parameters map[string]interface{}) (interface{}, error) {
	// 示例天气插件实现
	city, ok := parameters["city"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'city' is required")
	}

	// 这里应该调用实际的天气API
	// 例如：OpenWeatherMap API
	// apiKey := os.Getenv("WEATHER_API_KEY")
	// url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s", city, apiKey)
	// 由于是示例，返回模拟数据

	return map[string]interface{}{
		"city": city,
		"temperature": 25.5,
		"humidity": 60,
		"description": "晴",
		"wind_speed": 5.2,
	},
	nil
}

// executeStockPlugin 执行股票插件
func (s *PluginService) executeStockPlugin(parameters map[string]interface{}) (interface{}, error) {
	// 示例股票插件实现
	symbol, ok := parameters["symbol"].(string)
	if !ok {
		return nil, fmt.Errorf("parameter 'symbol' is required")
	}

	// 这里应该调用实际的股票API
	// 例如：Alpha Vantage API
	// apiKey := os.Getenv("STOCK_API_KEY")
	// url := fmt.Sprintf("https://www.alphavantage.co/query?function=GLOBAL_QUOTE&symbol=%s&apikey=%s", symbol, apiKey)
	// 由于是示例，返回模拟数据

	return map[string]interface{}{
		"symbol": symbol,
		"price": 150.25,
		"change": 2.5,
		"change_percent": 1.7,
		"volume": 1000000,
	},
	nil
}

// GetPluginExecutions 获取插件执行历史
func (s *PluginService) GetPluginExecutions(pluginID string) ([]models.PluginExecution, error) {
	var executions []models.PluginExecution
	err := s.db.Where("plugin_id = ?", pluginID).Order("started_at DESC").Find(&executions).Error
	return executions, err
}
