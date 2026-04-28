package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"openaide/backend/src/config"
	"openaide/backend/src/models"
)

// TestSkillService_CreateSkill 测试创建技能
func TestSkillService_CreateSkill(t *testing.T) {
	db, err := setupTestDBForSkill()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	skillService := NewSkillService(db, modelService, loggerService)

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
	assert.NotEmpty(t, skill.ID)

	// 验证技能是否创建成功
	var savedSkill models.Skill
	err = db.First(&savedSkill, "name = ?", "test_skill").Error
	assert.NoError(t, err)
	assert.Equal(t, skill.Name, savedSkill.Name)
}

// TestSkillService_ListSkills 测试列出技能
func TestSkillService_ListSkills(t *testing.T) {
	db, err := setupTestDBForSkill()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	skillService := NewSkillService(db, modelService, loggerService)

	// 创建测试技能
	skill1 := &models.Skill{
		Name:        "skill1",
		Description: "Test skill 1",
		Category:    "test",
		Version:     "1.0",
		Author:      "test",
		Enabled:     true,
		Triggers:    models.JSONSlice{"skill1"},
	}
	skill2 := &models.Skill{
		Name:        "skill2",
		Description: "Test skill 2",
		Category:    "test",
		Version:     "1.0",
		Author:      "test",
		Enabled:     true,
		Triggers:    models.JSONSlice{"skill2"},
	}

	err = skillService.CreateSkill(skill1)
	assert.NoError(t, err)
	err = skillService.CreateSkill(skill2)
	assert.NoError(t, err)

	// 测试列出技能
	skills, err := skillService.ListSkills()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(skills), 2)
}

// TestSkillService_ListEnabledSkills 测试列出已启用的技能
func TestSkillService_ListEnabledSkills(t *testing.T) {
	db, err := setupTestDBForSkill()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	skillService := NewSkillService(db, modelService, loggerService)

	// 创建启用和禁用的技能
	enabledSkill := &models.Skill{
		Name:        "enabled_skill",
		Description: "Enabled test skill",
		Category:    "test",
		Version:     "1.0",
		Author:      "test",
		Enabled:     true,
		Triggers:    models.JSONSlice{"enabled"},
	}
	disabledSkill := &models.Skill{
		Name:        "disabled_skill",
		Description: "Disabled test skill",
		Category:    "test",
		Version:     "1.0",
		Author:      "test",
		Enabled:     false,
		Triggers:    models.JSONSlice{"disabled"},
	}

	err = skillService.CreateSkill(enabledSkill)
	assert.NoError(t, err)
	err = skillService.CreateSkill(disabledSkill)
	assert.NoError(t, err)

	// 测试列出已启用的技能
	enabledSkills, err := skillService.ListEnabledSkills()
	assert.NoError(t, err)

	// 验证只有启用的技能被返回
	for _, skill := range enabledSkills {
		assert.True(t, skill.Enabled)
	}
}

// TestSkillService_MatchSkill 测试技能匹配
func TestSkillService_MatchSkill(t *testing.T) {
	db, err := setupTestDBForSkill()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	skillService := NewSkillService(db, modelService, loggerService)

	// 创建测试技能
	skill := &models.Skill{
		Name:        "translation",
		Description: "Translation skill",
		Category:    "language",
		Version:     "1.0",
		Author:      "test",
		Enabled:     true,
		Triggers:    models.JSONSlice{"翻译", "translate"},
	}

	err = skillService.CreateSkill(skill)
	assert.NoError(t, err)

	// 测试匹配技能
	result := skillService.MatchSkill("帮我翻译这句话")
	assert.NotNil(t, result)
	assert.Equal(t, "translation", result.Skill.Name)
}

// TestSkillService_CreateSkillParameter 测试创建技能参数
func TestSkillService_CreateSkillParameter(t *testing.T) {
	db, err := setupTestDBForSkill()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
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
		Triggers:    models.JSONSlice{"test"},
	}

	err = skillService.CreateSkill(skill)
	assert.NoError(t, err)

	// 创建技能参数
	param := &models.SkillParameter{
		SkillID:     skill.ID,
		Name:        "text",
		Description: "Text to process",
		Type:        "string",
		Required:    true,
		Default:     nil,
	}

	err = skillService.CreateSkillParameter(param)
	assert.NoError(t, err)
	assert.NotEmpty(t, param.ID)

	// 验证参数是否创建成功
	params, err := skillService.GetSkillParameters(skill.ID)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(params), 1)
}

// setupTestDBForSkill 设置测试数据库
func setupTestDBForSkill() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// 自动迁移
	err = db.AutoMigrate(
		&models.Skill{},
		&models.SkillParameter{},
		&models.SkillExecution{},
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}
