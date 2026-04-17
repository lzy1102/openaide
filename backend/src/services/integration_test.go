package services

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// setupTestDBForIntegration 创建测试数据库
func setupTestDBForIntegration() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// 自动迁移
	err = db.AutoMigrate(
		&models.Skill{},
		&models.SkillParameter{},
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// TestIntegration_ToolExecution 测试工具执行流程
func TestIntegration_ToolExecution(t *testing.T) {
	db, err := setupTestDBForIntegration()
	assert.NoError(t, err)

	// 初始化服务
	cacheService := NewCacheService()
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	toolService := NewToolService(db, cacheService, loggerService, nil)

	// 执行时间工具
	ctx := context.Background()
	params := map[string]interface{}{}
	result, err := toolService.registry.builtin["get_current_time"].Execute(ctx, params)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// 验证结果包含时间信息
	resultMap, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Contains(t, resultMap, "datetime")
}

// TestIntegration_SkillExecution 测试技能执行流程
func TestIntegration_SkillExecution(t *testing.T) {
	db, err := setupTestDBForIntegration()
	assert.NoError(t, err)

	// 初始化服务
	cacheService := NewCacheService()
	modelService := NewModelService(db, cacheService)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	skillService := NewSkillService(db, modelService, loggerService)

	// 创建测试技能
	skill := &models.Skill{
		Name:        "test_skill",
		Description: "Test skill",
		Category:    "test",
		Version:     "1.0",
		Author:      "test",
		Enabled:     true,
		Triggers:    models.JSONSlice{"test", "testing"},
	}

	err = skillService.CreateSkill(skill)
	assert.NoError(t, err)

	// 测试技能匹配
	result := skillService.MatchSkill("test this")
	assert.NotNil(t, result)
	assert.Equal(t, "test_skill", result.Skill.Name)
}
