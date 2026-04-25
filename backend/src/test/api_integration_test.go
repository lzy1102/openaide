package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"openaide/backend/src/handlers"
	"openaide/backend/src/models"
	"openaide/backend/src/services"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupTestDB 创建测试数据库
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	// 自动迁移所有模型
	err = db.AutoMigrate(
		&models.Model{},
		&models.Dialogue{},
		&models.Message{},
		&models.ModelInstance{},
		&models.ModelExecution{},
		&models.Workflow{},
		&models.WorkflowInstance{},
		&models.Tool{},
		&models.ToolExecution{},
		&models.Knowledge{},
		&models.KnowledgeCategory{},
		&models.Document{},
		&models.Memory{},
		&models.ShortTermMemory{},
		&models.Task{},
		&models.Subtask{},
		&models.TaskDependency{},
		&models.TaskAssignment{},
		&models.TaskProgress{},
		&models.Plugin{},
		&models.CodeExecution{},
		&models.AutomationExecution{},
		&models.Confirmation{},
		&models.WorkflowSchedule{},
	)
	require.NoError(t, err)

	return db
}

// setupTestRouter 创建测试路由
func setupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	db := setupTestDB(t)
	cache := services.NewCacheService()
	logger, _ := services.NewLoggerService(services.LogLevelInfo, "")

	// 创建服务
	modelService := services.NewModelService(db, cache)
	dialogueService := services.NewDialogueService(db, modelService, logger)
	workflowService := services.NewWorkflowService(db, modelService.GetLLMClient())
	toolService := services.NewToolService(db, cache, logger, nil)
	embeddingService := services.NewOpenAIEmbeddingService("", "", "", cache)
	vectorManager, _ := services.NewVectorManager("", embeddingService)
	var vectorSvc services.VectorService = vectorManager
	if vectorSvc == nil {
		vectorSvc = services.NewNoopVectorService()
	}
	knowledgeService := services.NewKnowledgeService(db, embeddingService, vectorSvc, cache)
	taskService := services.NewTaskService(db, modelService.GetLLMClient())
	pluginService := services.NewPluginService(db, cache)
	codeService := services.NewCodeService(db)
	automationService := services.NewAutomationService(db)
	confirmationService := services.NewConfirmationService(db)

	_ = handlers.NewKnowledgeHandler(knowledgeService, nil, nil, logger)
	_ = handlers.NewTaskHandler(taskService)

	// 设置API路由
	api := router.Group("/api")
	{
		// 模型API
		models := api.Group("/models")
		{
			models.GET("", func(c *gin.Context) {
				models, err := modelService.ListModels()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, models)
			})

			models.POST("", func(c *gin.Context) {
				var model models.Model
				if err := c.ShouldBindJSON(&model); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				if err := modelService.CreateModel(&model); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, model)
			})

			models.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				model, err := modelService.GetModel(id)
				if err != nil {
					c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, model)
			})

			models.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				var model models.Model
				if err := c.ShouldBindJSON(&model); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				model.ID = id
				if err := modelService.UpdateModel(&model); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, model)
			})

			models.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := modelService.DeleteModel(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Model deleted successfully"})
			})
		}

		// 对话API
		dialogues := api.Group("/dialogues")
		{
			dialogues.POST("", func(c *gin.Context) {
				var req struct {
					UserID string `json:"user_id"`
					Title  string `json:"title"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				dialogue := dialogueService.CreateDialogue(req.UserID, req.Title)
				c.JSON(http.StatusOK, dialogue)
			})

			dialogues.GET("", func(c *gin.Context) {
				dialogues := dialogueService.ListDialogues()
				c.JSON(http.StatusOK, dialogues)
			})

			dialogues.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				dialogue, found := dialogueService.GetDialogue(id)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Dialogue not found"})
					return
				}
				c.JSON(http.StatusOK, dialogue)
			})

			dialogues.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := dialogueService.DeleteDialogue(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Dialogue deleted successfully"})
			})

			dialogues.POST("/:id/messages", func(c *gin.Context) {
				id := c.Param("id")
				var req struct {
					UserID  string `json:"user_id"`
					Content string `json:"content"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				message, err := dialogueService.AddMessage(id, "user", req.Content)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, message)
			})

			dialogues.GET("/:id/messages", func(c *gin.Context) {
				id := c.Param("id")
				messages := dialogueService.GetMessages(id)
				c.JSON(http.StatusOK, messages)
			})
		}

		// 工作流API
		workflows := api.Group("/workflows")
		{
			workflows.GET("", func(c *gin.Context) {
				workflows := workflowService.ListWorkflows()
				c.JSON(http.StatusOK, workflows)
			})

			workflows.POST("", func(c *gin.Context) {
				var req struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				workflow := workflowService.CreateWorkflow(req.Name, req.Description, nil)
				c.JSON(http.StatusOK, workflow)
			})

			workflows.GET("/:id", func(c *gin.Context) {
				id := c.Param("id")
				workflow, found := workflowService.GetWorkflow(id)
				if !found {
					c.JSON(http.StatusNotFound, gin.H{"error": "Workflow not found"})
					return
				}
				c.JSON(http.StatusOK, workflow)
			})

			workflows.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if !workflowService.DeleteWorkflow(id) {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete workflow"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Workflow deleted successfully"})
			})
		}

		// 工具API
		tools := api.Group("/tools")
		{
			tools.GET("", func(c *gin.Context) {
				tools, err := toolService.ListTools()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, tools)
			})

			tools.POST("", func(c *gin.Context) {
				var tool models.Tool
				if err := c.ShouldBindJSON(&tool); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				tool.ID = uuid.New().String()
				tool.CreatedAt = time.Now()
				tool.UpdatedAt = time.Now()
				if err := toolService.RegisterTool(&tool); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, tool)
			})
		}

		// 任务API
		tasks := api.Group("/tasks")
		{
			tasks.GET("", func(c *gin.Context) {
				tasks := taskService.ListTasks()
				c.JSON(http.StatusOK, tasks)
			})

			tasks.POST("", func(c *gin.Context) {
				var req struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				task := taskService.CreateTask(req.Title, req.Description, "medium", time.Now().Add(24*time.Hour))
				c.JSON(http.StatusOK, task)
			})

			tasks.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := taskService.DeleteTask(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Task deleted successfully"})
			})
		}

		// 插件API
		plugins := api.Group("/plugins")
		{
			plugins.GET("", func(c *gin.Context) {
				plugins, err := pluginService.ListPlugins()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, plugins)
			})

			plugins.POST("", func(c *gin.Context) {
				var plugin models.Plugin
				if err := c.ShouldBindJSON(&plugin); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				if err := pluginService.CreatePlugin(&plugin); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, plugin)
			})

			plugins.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				if err := pluginService.DeletePlugin(id); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Plugin deleted successfully"})
			})
		}
	}

	return router, db
}

// TestModelAPI 测试模型API完整流程
func TestModelAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Model", func(t *testing.T) {
		model := models.Model{
			Name:     "test-model",
			Type:     "llm",
			Provider: "test",
			APIKey:   "test-key",
		}

		jsonData, _ := json.Marshal(model)
		req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Model
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test-model", response.Name)
		assert.Equal(t, "enabled", response.Status)
	})

	t.Run("List Models", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var models []models.Model
		err := json.Unmarshal(w.Body.Bytes(), &models)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(models), 1)
	})

	t.Run("Get Model", func(t *testing.T) {
		// 先创建一个模型
		model := models.Model{
			Name:     "get-test-model",
			Type:     "llm",
			Provider: "test",
		}
		jsonData, _ := json.Marshal(model)
		req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdModel models.Model
		json.Unmarshal(w.Body.Bytes(), &createdModel)

		// 获取模型
		req = httptest.NewRequest(http.MethodGet, "/api/models/"+createdModel.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Model
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, createdModel.ID, response.ID)
	})

	t.Run("Update Model", func(t *testing.T) {
		// 先创建一个模型
		model := models.Model{
			Name:     "update-test-model",
			Type:     "llm",
			Provider: "test",
		}
		jsonData, _ := json.Marshal(model)
		req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdModel models.Model
		json.Unmarshal(w.Body.Bytes(), &createdModel)

		// 更新模型
		updateData := models.Model{
			Name: "updated-model-name",
			Type: "llm",
		}
		jsonData, _ = json.Marshal(updateData)
		req = httptest.NewRequest(http.MethodPut, "/api/models/"+createdModel.ID, bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Delete Model", func(t *testing.T) {
		// 先创建一个模型
		model := models.Model{
			Name:     "delete-test-model",
			Type:     "llm",
			Provider: "test",
		}
		jsonData, _ := json.Marshal(model)
		req := httptest.NewRequest(http.MethodPost, "/api/models", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdModel models.Model
		json.Unmarshal(w.Body.Bytes(), &createdModel)

		// 删除模型
		req = httptest.NewRequest(http.MethodDelete, "/api/models/"+createdModel.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, "Model deleted successfully", response["message"])
	})
}

// TestDialogueAPI 测试对话API完整流程
func TestDialogueAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Dialogue", func(t *testing.T) {
		reqBody := map[string]string{
			"user_id": "test-user",
			"title":   "Test Dialogue",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/dialogues", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Dialogue
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "Test Dialogue", response.Title)
	})

	t.Run("List Dialogues", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/dialogues", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var dialogues []models.Dialogue
		err := json.Unmarshal(w.Body.Bytes(), &dialogues)
		require.NoError(t, err)
	})

	t.Run("Get Dialogue", func(t *testing.T) {
		// 先创建对话
		reqBody := map[string]string{
			"user_id": "test-user",
			"title":   "Get Test Dialogue",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/dialogues", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdDialogue models.Dialogue
		json.Unmarshal(w.Body.Bytes(), &createdDialogue)

		// 获取对话
		req = httptest.NewRequest(http.MethodGet, "/api/dialogues/"+createdDialogue.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Dialogue
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, createdDialogue.ID, response.ID)
	})

	t.Run("Add Message", func(t *testing.T) {
		// 先创建对话
		reqBody := map[string]string{
			"user_id": "test-user",
			"title":   "Message Test Dialogue",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/dialogues", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdDialogue models.Dialogue
		json.Unmarshal(w.Body.Bytes(), &createdDialogue)

		// 添加消息
		messageBody := map[string]string{
			"user_id": "test-user",
			"content": "Hello, this is a test message",
		}
		jsonData, _ = json.Marshal(messageBody)
		req = httptest.NewRequest(http.MethodPost, "/api/dialogues/"+createdDialogue.ID+"/messages", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Message
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Hello, this is a test message", response.Content)
		assert.Equal(t, "user", response.Sender)
	})

	t.Run("Get Messages", func(t *testing.T) {
		// 先创建对话和消息
		reqBody := map[string]string{
			"user_id": "test-user",
			"title":   "Get Messages Test",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/dialogues", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdDialogue models.Dialogue
		json.Unmarshal(w.Body.Bytes(), &createdDialogue)

		// 添加几条消息
		for i := 0; i < 3; i++ {
			messageBody := map[string]string{
				"user_id": "test-user",
				"content": "Message " + string(rune('0'+i)),
			}
			jsonData, _ = json.Marshal(messageBody)
			req = httptest.NewRequest(http.MethodPost, "/api/dialogues/"+createdDialogue.ID+"/messages", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
		}

		// 获取消息列表
		req = httptest.NewRequest(http.MethodGet, "/api/dialogues/"+createdDialogue.ID+"/messages", nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var messages []models.Message
		err := json.Unmarshal(w.Body.Bytes(), &messages)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(messages), 3)
	})

	t.Run("Delete Dialogue", func(t *testing.T) {
		// 先创建对话
		reqBody := map[string]string{
			"user_id": "test-user",
			"title":   "Delete Test Dialogue",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/dialogues", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdDialogue models.Dialogue
		json.Unmarshal(w.Body.Bytes(), &createdDialogue)

		// 删除对话
		req = httptest.NewRequest(http.MethodDelete, "/api/dialogues/"+createdDialogue.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]string
		json.Unmarshal(w.Body.Bytes(), &response)
		assert.Equal(t, "Dialogue deleted successfully", response["message"])
	})
}

// TestWorkflowAPI 测试工作流API
func TestWorkflowAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Workflow", func(t *testing.T) {
		reqBody := map[string]string{
			"name":        "test-workflow",
			"description": "Test workflow description",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Workflow
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test-workflow", response.Name)
	})

	t.Run("List Workflows", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var workflows []models.Workflow
		err := json.Unmarshal(w.Body.Bytes(), &workflows)
		require.NoError(t, err)
	})

	t.Run("Delete Workflow", func(t *testing.T) {
		// 先创建工作流
		reqBody := map[string]string{
			"name":        "delete-test-workflow",
			"description": "To be deleted",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdWorkflow models.Workflow
		json.Unmarshal(w.Body.Bytes(), &createdWorkflow)

		// 删除工作流
		req = httptest.NewRequest(http.MethodDelete, "/api/workflows/"+createdWorkflow.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestToolAPI 测试工具API
func TestToolAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Tool", func(t *testing.T) {
		tool := models.Tool{
			Name:        "test-tool",
			Description: "Test tool description",
			Type:        "function",
			Category:    "test",
			Enabled:     true,
			ParametersSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"param1": map[string]interface{}{
						"type": "string",
					},
				},
			},
		}

		jsonData, _ := json.Marshal(tool)
		req := httptest.NewRequest(http.MethodPost, "/api/tools", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Tool
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test-tool", response.Name)
	})

	t.Run("List Tools", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var tools []models.Tool
		err := json.Unmarshal(w.Body.Bytes(), &tools)
		require.NoError(t, err)
	})
}

// TestTaskAPI 测试任务API
func TestTaskAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Task", func(t *testing.T) {
		reqBody := map[string]string{
			"title":       "test-task",
			"description": "Test task description",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Task
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test-task", response.Title)
	})

	t.Run("List Tasks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var tasks []models.Task
		err := json.Unmarshal(w.Body.Bytes(), &tasks)
		require.NoError(t, err)
	})

	t.Run("Delete Task", func(t *testing.T) {
		// 先创建任务
		reqBody := map[string]string{
			"title":       "delete-test-task",
			"description": "To be deleted",
		}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdTask models.Task
		json.Unmarshal(w.Body.Bytes(), &createdTask)

		// 删除任务
		req = httptest.NewRequest(http.MethodDelete, "/api/tasks/"+createdTask.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestPluginAPI 测试插件API
func TestPluginAPI(t *testing.T) {
	router, _ := setupTestRouter(t)

	t.Run("Create Plugin", func(t *testing.T) {
		plugin := models.Plugin{
			Name:        "test-plugin",
			Description: "Test plugin description",
			Version:     "1.0.0",
			Enabled:     true,
		}

		jsonData, _ := json.Marshal(plugin)
		req := httptest.NewRequest(http.MethodPost, "/api/plugins", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response models.Plugin
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
		assert.Equal(t, "test-plugin", response.Name)
	})

	t.Run("List Plugins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/plugins", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var plugins []models.Plugin
		err := json.Unmarshal(w.Body.Bytes(), &plugins)
		require.NoError(t, err)
	})

	t.Run("Delete Plugin", func(t *testing.T) {
		// 先创建插件
		plugin := models.Plugin{
			Name:        "delete-test-plugin",
			Description: "To be deleted",
			Version:     "1.0.0",
		}
		jsonData, _ := json.Marshal(plugin)
		req := httptest.NewRequest(http.MethodPost, "/api/plugins", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var createdPlugin models.Plugin
		json.Unmarshal(w.Body.Bytes(), &createdPlugin)

		// 删除插件
		req = httptest.NewRequest(http.MethodDelete, "/api/plugins/"+createdPlugin.ID, nil)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// TestDeleteSQLFix 测试删除SQL修复
func TestDeleteSQLFix(t *testing.T) {
	router, db := setupTestRouter(t)

	t.Run("Delete Model SQL Fix", func(t *testing.T) {
		// 创建模型
		model := models.Model{
			ID:       uuid.New().String(),
			Name:     "sql-fix-test-model",
			Type:     "llm",
			Provider: "test",
		}
		db.Create(&model)

		// 删除模型
		req := httptest.NewRequest(http.MethodDelete, "/api/models/"+model.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证已删除
		var count int64
		db.Model(&models.Model{}).Where("id = ?", model.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Delete Dialogue SQL Fix", func(t *testing.T) {
		// 创建对话
		dialogue := models.Dialogue{
			ID:     uuid.New().String(),
			UserID: "test-user",
			Title:  "sql-fix-test-dialogue",
		}
		db.Create(&dialogue)

		// 删除对话
		req := httptest.NewRequest(http.MethodDelete, "/api/dialogues/"+dialogue.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证已删除
		var count int64
		db.Model(&models.Dialogue{}).Where("id = ?", dialogue.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Delete Workflow SQL Fix", func(t *testing.T) {
		// 创建工作流
		workflow := models.Workflow{
			ID:          uuid.New().String(),
			Name:        "sql-fix-test-workflow",
			Description: "Test",
		}
		db.Create(&workflow)

		// 删除工作流
		req := httptest.NewRequest(http.MethodDelete, "/api/workflows/"+workflow.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证已删除
		var count int64
		db.Model(&models.Workflow{}).Where("id = ?", workflow.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Delete Task SQL Fix", func(t *testing.T) {
		// 创建任务
		task := models.Task{
			ID:          uuid.New().String(),
			Title:       "sql-fix-test-task",
			Description: "Test",
			Status:      "pending",
		}
		db.Create(&task)

		// 删除任务
		req := httptest.NewRequest(http.MethodDelete, "/api/tasks/"+task.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证已删除
		var count int64
		db.Model(&models.Task{}).Where("id = ?", task.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})

	t.Run("Delete Plugin SQL Fix", func(t *testing.T) {
		// 创建插件
		plugin := models.Plugin{
			ID:      uuid.New().String(),
			Name:    "sql-fix-test-plugin",
			Version: "1.0.0",
		}
		db.Create(&plugin)

		// 删除插件
		req := httptest.NewRequest(http.MethodDelete, "/api/plugins/"+plugin.ID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// 验证已删除
		var count int64
		db.Model(&models.Plugin{}).Where("id = ?", plugin.ID).Count(&count)
		assert.Equal(t, int64(0), count)
	})
}
