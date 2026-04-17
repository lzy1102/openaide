package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

func TestWorkflowService(t *testing.T) {
	// 创建内存数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// 自动迁移
	db.AutoMigrate(&models.Workflow{}, &models.WorkflowStep{}, &models.WorkflowInstance{}, &models.StepInstance{})

	// 创建服务实例（传入nil作为LLM客户端，因为这个测试不需要它）
	service := NewWorkflowService(db, nil)

	// 测试创建工作流
	t.Run("CreateWorkflow", func(t *testing.T) {
		steps := []models.WorkflowStep{
			{
				ID:          "1",
				Name:        "Step 1",
				Description: "First step",
				Type:        "skill",
				
				Parameters:  map[string]interface{}{"param1": "value1"},
			},
		}

		workflow := service.CreateWorkflow("Test Workflow", "Test description", steps)
		if workflow.ID == "" {
			t.Error("Expected workflow ID to be set")
		}
		if workflow.Name != "Test Workflow" {
			t.Errorf("Expected workflow name to be 'Test Workflow', got '%s'", workflow.Name)
		}
	})

	// 测试获取工作流
	t.Run("GetWorkflow", func(t *testing.T) {
		steps := []models.WorkflowStep{
			{
				ID:          "1",
				Name:        "Step 1",
				Description: "First step",
				Type:        "skill",
				
				Parameters:  map[string]interface{}{"param1": "value1"},
			},
		}

		workflow := service.CreateWorkflow("Test Workflow", "Test description", steps)
		retrieved, found := service.GetWorkflow(workflow.ID)
		if !found {
			t.Error("Expected to find workflow")
		}
		if retrieved.ID != workflow.ID {
			t.Errorf("Expected workflow ID to be '%s', got '%s'", workflow.ID, retrieved.ID)
		}
	})

	// 测试列出工作流
	t.Run("ListWorkflows", func(t *testing.T) {
		steps := []models.WorkflowStep{
			{
				ID:          "1",
				Name:        "Step 1",
				Description: "First step",
				Type:        "skill",
				
				Parameters:  map[string]interface{}{"param1": "value1"},
			},
		}

		service.CreateWorkflow("Test Workflow 1", "Test description 1", steps)
		service.CreateWorkflow("Test Workflow 2", "Test description 2", steps)

		workflows := service.ListWorkflows()
		if len(workflows) < 2 {
			t.Errorf("Expected at least 2 workflows, got %d", len(workflows))
		}
	})

	// 测试更新工作流
	t.Run("UpdateWorkflow", func(t *testing.T) {
		steps := []models.WorkflowStep{
			{
				ID:          "1",
				Name:        "Step 1",
				Description: "First step",
				Type:        "skill",
				
				Parameters:  map[string]interface{}{"param1": "value1"},
			},
		}

		workflow := service.CreateWorkflow("Test Workflow", "Test description", steps)
		updated, found := service.UpdateWorkflow(workflow.ID, "Updated Workflow", "Updated description", steps)
		if !found {
			t.Error("Expected to find workflow")
		}
		if updated.Name != "Updated Workflow" {
			t.Errorf("Expected workflow name to be 'Updated Workflow', got '%s'", updated.Name)
		}
	})

	// 测试删除工作流
	t.Run("DeleteWorkflow", func(t *testing.T) {
		steps := []models.WorkflowStep{
			{
				ID:          "1",
				Name:        "Step 1",
				Description: "First step",
				Type:        "skill",
				
				Parameters:  map[string]interface{}{"param1": "value1"},
			},
		}

		workflow := service.CreateWorkflow("Test Workflow", "Test description", steps)
		deleted := service.DeleteWorkflow(workflow.ID)
		if !deleted {
			t.Error("Expected workflow to be deleted")
		}
		_, found := service.GetWorkflow(workflow.ID)
		if found {
			t.Error("Expected workflow to not be found after deletion")
		}
	})
}
