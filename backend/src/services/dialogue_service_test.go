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

// setupTestDBForDialogue 创建测试数据库
func setupTestDBForDialogue() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, err
	}

	// 自动迁移
	err = db.AutoMigrate(
		&models.Dialogue{},
		&models.Message{},
	)
	if err != nil {
		return nil, err
	}

	return db, nil
}

// TestDialogueService_CreateDialogue 测试创建对话
func TestDialogueService_CreateDialogue(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	userID := "test_user"
	title := "Test Dialogue"

	dialogue := dialogueService.CreateDialogue(userID, title)
	assert.NotEmpty(t, dialogue.ID)
	assert.Equal(t, userID, dialogue.UserID)
	assert.Equal(t, title, dialogue.Title)

	// 验证对话是否创建成功
	var savedDialogue models.Dialogue
	err = db.First(&savedDialogue, "id = ?", dialogue.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, dialogue.ID, savedDialogue.ID)
}

// TestDialogueService_ListDialogues 测试列出对话
func TestDialogueService_ListDialogues(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	// 创建测试对话
	dialogueService.CreateDialogue("user1", "Dialogue 1")
	dialogueService.CreateDialogue("user1", "Dialogue 2")
	dialogueService.CreateDialogue("user2", "Dialogue 3")

	// 测试列出所有对�?
	dialogues := dialogueService.ListDialogues()
	assert.GreaterOrEqual(t, len(dialogues), 3)

	// 测试列出特定用户的对�?
	user1Dialogues := dialogueService.ListDialoguesByUser("user1")
	assert.GreaterOrEqual(t, len(user1Dialogues), 2)
	for _, d := range user1Dialogues {
		assert.Equal(t, "user1", d.UserID)
	}
}

// TestDialogueService_GetDialogue 测试获取对话
func TestDialogueService_GetDialogue(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	// 创建测试对话
	dialogue := dialogueService.CreateDialogue("user1", "Test Dialogue")

	// 测试获取对话
	foundDialogue, found := dialogueService.GetDialogue(dialogue.ID)
	assert.True(t, found)
	assert.Equal(t, dialogue.ID, foundDialogue.ID)

	// 测试获取不存在的对话
	_, notFound := dialogueService.GetDialogue("non_existent_id")
	assert.False(t, notFound)
}

// TestDialogueService_UpdateDialogue 测试更新对话
func TestDialogueService_UpdateDialogue(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	// 创建测试对话
	dialogue := dialogueService.CreateDialogue("user1", "Old Title")

	// 测试更新对话
	newTitle := "New Title"
	updatedDialogue, found := dialogueService.UpdateDialogue(dialogue.ID, newTitle)
	assert.True(t, found)
	assert.Equal(t, newTitle, updatedDialogue.Title)

	// 验证更新是否成功
	var savedDialogue models.Dialogue
	err = db.First(&savedDialogue, "id = ?", dialogue.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, newTitle, savedDialogue.Title)
}

// TestDialogueService_DeleteDialogue 测试删除对话
func TestDialogueService_DeleteDialogue(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)
	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	// 创建测试对话
	dialogue := dialogueService.CreateDialogue("user1", "Test Dialogue")

	// 测试删除对话
	err = dialogueService.DeleteDialogue(dialogue.ID)
	assert.NoError(t, err)

	// 验证对话是否删除成功
	var savedDialogue models.Dialogue
	err = db.First(&savedDialogue, "id = ?", dialogue.ID).Error
	assert.Error(t, err)
}

// TestDialogueService_SendMessage 测试发送消息（使用真实模型调用，需要配置模型）
func TestDialogueService_SendMessage(t *testing.T) {
	db, err := setupTestDBForDialogue()
	assert.NoError(t, err)

	// 创建带有模拟 LLM 客户端的模型服务
	modelService := NewModelService(&config.Config{Models: []config.ModelConfig{}}, nil, db)

	loggerService, _ := NewLoggerService(LogLevelInfo, "")
	dialogueService := NewDialogueService(db, modelService, loggerService)

	// 创建测试对话
	dialogue := dialogueService.CreateDialogue("user1", "Test Dialogue")

	// 测试发送消�?- 由于需要真实模型，这里只测试消息添加逻辑
	// 先手动添加用户消�?
	userMsg, err := dialogueService.AddMessage(dialogue.ID, "user", "Hello")
	assert.NoError(t, err)
	assert.NotEmpty(t, userMsg.ID)
	assert.Equal(t, "Hello", userMsg.Content)

	// 验证消息是否创建成功
	messages := dialogueService.GetMessages(dialogue.ID)
	assert.GreaterOrEqual(t, len(messages), 1)
}


