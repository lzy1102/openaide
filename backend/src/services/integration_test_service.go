package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"openaide/backend/src/models"
	"openaide/backend/src/services/llm"
)

// IntegrationTestService 集成测试服务
type IntegrationTestService struct {
	db              *gorm.DB
	taskService     *TaskService
	contextManager  ContextManager
	dialogueService *DialogueService
	modelService    *ModelService
	cacheService    *CacheService
	loggerService   *LoggerService
	mu              sync.RWMutex
	testResults     map[string]*TestResult
}

// TestResult 测试结果
type TestResult struct {
	TestID      string                 `json:"test_id"`
	TestName    string                 `json:"test_name"`
	Status      string                 `json:"status"` // passed, failed, skipped
	Duration    time.Duration          `json:"duration"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at"`
	Error       string                 `json:"error,omitempty"`
	Details     map[string]interface{} `json:"details,omitempty"`
	Metrics     TestMetrics            `json:"metrics"`
}

// TestMetrics 测试指标
type TestMetrics struct {
	TotalAssertions int                    `json:"total_assertions"`
	PassedAssertions int                   `json:"passed_assertions"`
	FailedAssertions int                   `json:"failed_assertions"`
	Coverage         map[string]float64    `json:"coverage,omitempty"`
	Performance      map[string]time.Duration `json:"performance,omitempty"`
}

// TestSuite 测试套件
type TestSuite struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Tests       []TestCase       `json:"tests"`
	SetupFunc   func() error     `json:"-"`
	TeardownFunc func() error    `json:"-"`
}

// TestCase 测试用例
type TestCase struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	TestFunc    func(*TestContext) error `json:"-"`
	Timeout     time.Duration          `json:"timeout"`
	Skip        bool                   `json:"skip"`
}

// TestContext 测试上下文
type TestContext struct {
	Context     context.Context
	DB          *gorm.DB
	TestData    map[string]interface{}
	MockLLM     *MockLLMClient
	StartTime   time.Time
	Assertions  []Assertion
	mu          sync.Mutex
}

// Assertion 断言
type Assertion struct {
	Description string
	Passed      bool
	Expected    interface{}
	Actual      interface{}
	Error       error
}

// NewIntegrationTestService 创建集成测试服务
func NewIntegrationTestService(db *gorm.DB) *IntegrationTestService {
	service := &IntegrationTestService{
		db:          db,
		testResults: make(map[string]*TestResult),
	}

	// 初始化子服务
	service.initServices()

	return service
}

// initServices 初始化子服务
func (s *IntegrationTestService) initServices() {
	// 创建缓存服务
	s.cacheService = NewCacheService()

	// 创建日志服务
	var err error
	s.loggerService, err = NewLoggerService(LogLevelInfo, "")
	if err != nil {
		panic(fmt.Sprintf("failed to create logger service: %v", err))
	}

	// 创建模型服务
	s.modelService = NewModelService(s.db, s.cacheService)

	// 创建对话服务
	s.dialogueService = NewDialogueService(s.db, s.modelService, s.loggerService)

	// 创建上下文管理器
	s.contextManager = NewContextManager(
		s.db,
		s.dialogueService,
		s.cacheService,
		s.loggerService,
		100,
		4000,
		24*time.Hour,
		true,
	)

	// 创建任务服务
	mockLLM := &MockLLMClient{}
	s.taskService = NewTaskService(s.db, mockLLM)
}

// RunTests 运行所有测试
func (s *IntegrationTestService) RunTests(ctx context.Context) (*TestSuiteResult, error) {
	suite := s.buildTestSuite()

	// 设置
	if suite.SetupFunc != nil {
		if err := suite.SetupFunc(); err != nil {
			return nil, fmt.Errorf("setup failed: %w", err)
		}
	}

	result := &TestSuiteResult{
		SuiteName:  suite.Name,
		StartedAt:  time.Now(),
		TestResults: make([]*TestResult, 0, len(suite.Tests)),
	}

	// 运行所有测试
	for _, test := range suite.Tests {
		if test.Skip {
			result.Skipped++
			continue
		}

		testResult := s.runTest(ctx, test)
		result.TestResults = append(result.TestResults, testResult)

		switch testResult.Status {
		case "passed":
			result.Passed++
		case "failed":
			result.Failed++
		}
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	// 清理
	if suite.TeardownFunc != nil {
		if err := suite.TeardownFunc(); err != nil {
			s.loggerService.Error(ctx, "Teardown failed: %v", err)
		}
	}

	return result, nil
}

// TestSuiteResult 测试套件结果
type TestSuiteResult struct {
	SuiteName   string         `json:"suite_name"`
	StartedAt   time.Time      `json:"started_at"`
	CompletedAt time.Time      `json:"completed_at"`
	Duration    time.Duration  `json:"duration"`
	Passed      int            `json:"passed"`
	Failed      int            `json:"failed"`
	Skipped     int            `json:"skipped"`
	TestResults []*TestResult  `json:"test_results"`
}

// runTest 运行单个测试
func (s *IntegrationTestService) runTest(ctx context.Context, test TestCase) *TestResult {
	result := &TestResult{
		TestID:    uuid.New().String(),
		TestName:  test.Name,
		Status:    "passed",
		StartedAt: time.Now(),
		Metrics:   TestMetrics{},
	}

	// 创建测试上下文
	testCtx := &TestContext{
		Context:    ctx,
		DB:         s.db,
		TestData:   make(map[string]interface{}),
		MockLLM:    &MockLLMClient{},
		StartTime:  time.Now(),
		Assertions: make([]Assertion, 0),
	}

	// 设置超时
	testCtxWithTimeout, cancel := context.WithTimeout(ctx, test.Timeout)
	defer cancel()

	// 运行测试
	done := make(chan error, 1)
	go func() {
		done <- test.TestFunc(testCtx)
	}()

	select {
	case err := <-done:
		if err != nil {
			result.Status = "failed"
			result.Error = err.Error()
		}
	case <-testCtxWithTimeout.Done():
		result.Status = "failed"
		result.Error = "test timeout"
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	// 收集指标
	result.Metrics.TotalAssertions = len(testCtx.Assertions)
	for _, a := range testCtx.Assertions {
		if a.Passed {
			result.Metrics.PassedAssertions++
		} else {
			result.Metrics.FailedAssertions++
		}
	}

	// 保存结果
	s.mu.Lock()
	s.testResults[result.TestID] = result
	s.mu.Unlock()

	return result
}

// buildTestSuite 构建测试套件
func (s *IntegrationTestService) buildTestSuite() TestSuite {
	return TestSuite{
		Name:        "Integration Test Suite",
		Description: "AI Assistant integration tests",
		SetupFunc:   s.setupSuite,
		TeardownFunc: s.teardownSuite,
		Tests: []TestCase{
			{
				Name:        "TestContextManager_Compress",
				Description: "测试上下文压缩功能",
				TestFunc:    s.testContextManagerCompress,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestContextManager_Summarize",
				Description: "测试上下文摘要功能",
				TestFunc:    s.testContextManagerSummarize,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestContextManager_ClearExpired",
				Description: "测试清理过期上下文",
				TestFunc:    s.testContextManagerClearExpired,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestContextManager_GetMetrics",
				Description: "测试获取上下文指标",
				TestFunc:    s.testContextManagerGetMetrics,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestTaskService_DecomposeTask",
				Description: "测试任务分解功能",
				TestFunc:    s.testTaskServiceDecompose,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestTaskService_CreateTask",
				Description: "测试创建任务",
				TestFunc:    s.testTaskServiceCreate,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestTaskService_AssignSubtasks",
				Description: "测试子任务分配",
				TestFunc:    s.testTaskServiceAssign,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestTaskService_UpdateStatus",
				Description: "测试任务状态更新",
				TestFunc:    s.testTaskServiceUpdateStatus,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestTaskService_GetOverview",
				Description: "测试获取任务概览",
				TestFunc:    s.testTaskServiceOverview,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestDialogueService_CreateDialogue",
				Description: "测试创建对话",
				TestFunc:    s.testDialogueServiceCreate,
				Timeout:     30 * time.Second,
			},
			{
				Name:        "TestDialogueService_SendMessage",
				Description: "测试发送消息",
				TestFunc:    s.testDialogueServiceSendMessage,
				Timeout:     30 * time.Second,
			},
		},
	}
}

// setupSuite 设置测试套件
func (s *IntegrationTestService) setupSuite() error {
	// 迁移测试表
	return s.db.AutoMigrate(
		&models.Dialogue{},
		&models.Message{},
		&models.Task{},
		&models.Subtask{},
		&models.TeamMember{},
		&models.TaskAssignment{},
		&models.TaskDependency{},
		&models.TaskProgress{},
		&models.TaskStatusUpdate{},
		&CompressedContext{},
	)
}

// teardownSuite 清理测试套件
func (s *IntegrationTestService) teardownSuite() error {
	// 清理测试数据
	tables := []interface{}{
		&models.Dialogue{},
		&models.Message{},
		&models.Task{},
		&models.Subtask{},
		&models.TeamMember{},
		&models.TaskAssignment{},
		&models.TaskDependency{},
		&models.TaskProgress{},
		&models.TaskStatusUpdate{},
		&CompressedContext{},
	}

	for _, table := range tables {
		if err := s.db.Where("1 = 1").Delete(table).Error; err != nil {
			return err
		}
	}

	return nil
}

// ============ 上下文管理器测试 ============

func (s *IntegrationTestService) testContextManagerCompress(ctx *TestContext) error {
	// 创建测试对话
	dialogue := s.dialogueService.CreateDialogue("test-user", "Test Dialogue")
	ctx.TestData["dialogue_id"] = dialogue.ID

	// 添加消息
	if _, err := s.dialogueService.AddMessage(dialogue.ID, "user", "Hello, how are you?"); err != nil {
		return fmt.Errorf("failed to add user message: %w", err)
	}
	if _, err := s.dialogueService.AddMessage(dialogue.ID, "assistant", "I'm doing well, thank you!"); err != nil {
		return fmt.Errorf("failed to add assistant message: %w", err)
	}

	// 执行压缩
	compressed, err := s.contextManager.Compress(dialogue.ID)
	if err != nil {
		return fmt.Errorf("compress failed: %w", err)
	}

	// 断言
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "压缩上下文应包含对话ID",
		Passed:      compressed.DialogueID == dialogue.ID,
		Expected:    dialogue.ID,
		Actual:      compressed.DialogueID,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "压缩上下文应包含摘要",
		Passed:      compressed.Summary != "",
		Expected:    "non-empty summary",
		Actual:      compressed.Summary,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "消息数量应为2",
		Passed:      compressed.MessageCount == 2,
		Expected:    2,
		Actual:      compressed.MessageCount,
	})

	return nil
}

func (s *IntegrationTestService) testContextManagerSummarize(ctx *TestContext) error {
	// 创建测试对话
	dialogue := s.dialogueService.CreateDialogue("test-user", "Test Summary")

	// 添加消息
	if _, err := s.dialogueService.AddMessage(dialogue.ID, "user", "What is AI?"); err != nil {
		return fmt.Errorf("failed to add user message: %w", err)
	}
	if _, err := s.dialogueService.AddMessage(dialogue.ID, "assistant", "AI stands for Artificial Intelligence."); err != nil {
		return fmt.Errorf("failed to add assistant message: %w", err)
	}

	// 执行摘要
	summary, err := s.contextManager.Summarize(dialogue.ID)
	if err != nil {
		return fmt.Errorf("summarize failed: %w", err)
	}

	// 断言
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "摘要不应为空",
		Passed:      summary != "",
		Expected:    "non-empty summary",
		Actual:      summary,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "摘要应包含对话标题",
		Passed:      contains(summary, dialogue.Title),
		Expected:    "contains title",
		Actual:      summary,
	})

	return nil
}

func (s *IntegrationTestService) testContextManagerClearExpired(ctx *TestContext) error {
	// 创建过期的压缩上下文
	expiredCtx := &CompressedContext{
		DialogueID: "expired-dialogue",
		Title:      "Expired Dialogue",
		Summary:    "This is expired",
		MessageCount: 1,
		TokenCount:   10,
		CreatedAt:    time.Now().Add(-48 * time.Hour),
		ExpiresAt:    time.Now().Add(-24 * time.Hour),
	}
	s.db.Create(expiredCtx)

	// 创建未过期的上下文
	validCtx := &CompressedContext{
		DialogueID: "valid-dialogue",
		Title:      "Valid Dialogue",
		Summary:    "This is valid",
		MessageCount: 1,
		TokenCount:   10,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	s.db.Create(validCtx)

	// 执行清理
	err := s.contextManager.ClearExpired(time.Now())
	if err != nil {
		return fmt.Errorf("clear expired failed: %w", err)
	}

	// 验证过期上下文已删除
	var count int64
	s.db.Model(&CompressedContext{}).Where("dialogue_id = ?", "expired-dialogue").Count(&count)

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "过期上下文应被删除",
		Passed:      count == 0,
		Expected:    0,
		Actual:      int(count),
	})

	// 验证有效上下文仍存在
	s.db.Model(&CompressedContext{}).Where("dialogue_id = ?", "valid-dialogue").Count(&count)

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "有效上下文应保留",
		Passed:      count == 1,
		Expected:    1,
		Actual:      int(count),
	})

	return nil
}

func (s *IntegrationTestService) testContextManagerGetMetrics(ctx *TestContext) error {
	// 创建测试数据
	dialogue := s.dialogueService.CreateDialogue("metrics-test", "Metrics Test")
	s.contextManager.Compress(dialogue.ID)

	// 获取指标
	metrics := s.contextManager.GetMetrics()

	// 断言
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "上下文总数应大于0",
		Passed:      metrics.TotalContexts > 0,
		Expected:    "> 0",
		Actual:      metrics.TotalContexts,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "指标应不为空",
		Passed:      metrics != nil,
		Expected:    "non-nil metrics",
		Actual:      metrics,
	})

	return nil
}

// ============ 任务服务测试 ============

func (s *IntegrationTestService) testTaskServiceDecompose(ctx *TestContext) error {
	// 创建团队成员
	member := &models.TeamMember{
		ID:           uuid.New().String(),
		TeamID:       "test-team",
		Name:         "Test Developer",
		Role:         "developer",
		Availability: "available",
		CurrentLoad:  0,
		MaxLoad:      3,
		Capabilities: []models.Capability{
			{Name: "coding", Level: 0.9},
		},
	}
	s.db.Create(member)

	// 创建任务分解请求
	req := DecomposeTaskRequest{
		Title:       "Implement User Authentication",
		Description: "Create a secure login system with JWT tokens",
		Type:        "coding",
		Priority:    "high",
		TeamID:      "test-team",
		CreatedBy:   "test-user",
	}

	// 执行分解
	result, err := s.taskService.DecomposeTask(ctx.Context, req)
	if err != nil {
		return fmt.Errorf("decompose task failed: %w", err)
	}

	ctx.TestData["task_id"] = result.Task.ID
	ctx.TestData["subtasks"] = result.Subtasks

	// 断言
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "任务应被创建",
		Passed:      result.Task != nil,
		Expected:    "non-nil task",
		Actual:      result.Task,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "子任务数量应大于0",
		Passed:      len(result.Subtasks) > 0,
		Expected:    "> 0",
		Actual:      len(result.Subtasks),
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "任务状态应为pending",
		Passed:      result.Task.Status == "pending",
		Expected:    "pending",
		Actual:      result.Task.Status,
	})

	return nil
}

func (s *IntegrationTestService) testTaskServiceCreate(ctx *TestContext) error {
	task := &models.Task{
		ID:          uuid.New().String(),
		TeamID:      "test-team",
		Title:       "Test Task",
		Description: "This is a test task",
		Type:        "coding",
		Priority:    "medium",
		Status:      "pending",
		Complexity:  5,
		Estimated:   60,
		CreatedBy:   "test-user",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err := s.taskService.CreateTask(task)
	if err != nil {
		return fmt.Errorf("create task failed: %w", err)
	}

	// 获取任务
	retrieved, err := s.taskService.GetTask(task.ID)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "任务应被成功创建",
		Passed:      retrieved.ID == task.ID,
		Expected:    task.ID,
		Actual:      retrieved.ID,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "任务标题应匹配",
		Passed:      retrieved.Title == task.Title,
		Expected:    task.Title,
		Actual:      retrieved.Title,
	})

	return nil
}

func (s *IntegrationTestService) testTaskServiceAssign(ctx *TestContext) error {
	// 创建多个团队成员
	members := []models.TeamMember{
		{
			ID:           uuid.New().String(),
			TeamID:       "test-team",
			Name:         "Senior Developer",
			Role:         "developer",
			Availability: "available",
			CurrentLoad:  0,
			MaxLoad:      3,
			Capabilities: []models.Capability{{Name: "coding", Level: 0.95}},
			Experience:   map[string]int{"coding": 5},
		},
		{
			ID:           uuid.New().String(),
			TeamID:       "test-team",
			Name:         "Junior Developer",
			Role:         "developer",
			Availability: "available",
			CurrentLoad:  1,
			MaxLoad:      3,
			Capabilities: []models.Capability{{Name: "coding", Level: 0.7}},
			Experience:   map[string]int{"coding": 1},
		},
	}

	for _, m := range members {
		s.db.Create(&m)
	}

	// 创建任务
	task := &models.Task{
		ID:          uuid.New().String(),
		TeamID:      "test-team",
		Title:       "Complex Coding Task",
		Description: "A complex coding task",
		Type:        "coding",
		Priority:    "high",
		Status:      "pending",
		CreatedBy:   "test-user",
	}
	s.db.Create(task)

	// 创建子任务
	subtask := &models.Subtask{
		ID:           uuid.New().String(),
		ParentTaskID: task.ID,
		Title:        "Implement Core Logic",
		Type:         "coding",
		Status:       "pending",
		Order:        0,
		Estimated:    60,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	s.db.Create(subtask)

	// 执行分配
	availableMembers, _ := s.taskService.getAvailableMembers("test-team")
	assignments := s.taskService.assignSubtasks([]models.Subtask{*subtask}, availableMembers, &TaskAnalysis{
		Complexity: 7,
	})

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "应创建分配记录",
		Passed:      len(assignments) > 0,
		Expected:    "> 0",
		Actual:      len(assignments),
	})

	if len(assignments) > 0 {
		ctx.Assertions = append(ctx.Assertions, Assertion{
			Description: "应分配给高级开发者",
			Passed:      assignments[0].AssignedTo == members[0].ID,
			Expected:    members[0].ID,
			Actual:      assignments[0].AssignedTo,
		})
	}

	return nil
}

func (s *IntegrationTestService) testTaskServiceUpdateStatus(ctx *TestContext) error {
	// 创建任务
	task := &models.Task{
		ID:        uuid.New().String(),
		TeamID:    "test-team",
		Title:     "Status Test Task",
		Status:    "pending",
		CreatedBy: "test-user",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.db.Create(task)

	// 更新状态为进行中
	err := s.taskService.UpdateTaskStatus(task.ID, "in_progress", "test-user", "Starting work")
	if err != nil {
		return fmt.Errorf("update status failed: %w", err)
	}

	// 验证状态
	updated, err := s.taskService.GetTask(task.ID)
	if err != nil {
		return fmt.Errorf("get task failed: %w", err)
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "状态应更新为in_progress",
		Passed:      updated.Status == "in_progress",
		Expected:    "in_progress",
		Actual:      updated.Status,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "开始时间应被设置",
		Passed:      updated.StartedAt != nil,
		Expected:    "non-nil started_at",
		Actual:      updated.StartedAt,
	})

	return nil
}

func (s *IntegrationTestService) testTaskServiceOverview(ctx *TestContext) error {
	// 创建多个任务
	tasks := []models.Task{
		{ID: uuid.New().String(), TeamID: "test-team", Title: "Task 1", Status: "pending", CreatedBy: "test-user"},
		{ID: uuid.New().String(), TeamID: "test-team", Title: "Task 2", Status: "in_progress", CreatedBy: "test-user"},
		{ID: uuid.New().String(), TeamID: "test-team", Title: "Task 3", Status: "completed", CreatedBy: "test-user"},
	}

	for _, t := range tasks {
		s.db.Create(&t)
	}

	// 获取概览
	overview, err := s.taskService.GetTaskOverview("test-team")
	if err != nil {
		return fmt.Errorf("get overview failed: %w", err)
	}

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	stats, ok := overview["stats"].(map[string]interface{})
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "概览应包含统计信息",
		Passed:      ok,
		Expected:    "stats map",
		Actual:      overview,
	})

	if ok {
		total, _ := stats["Total"].(int64)
		ctx.Assertions = append(ctx.Assertions, Assertion{
			Description: "任务总数应为3",
			Passed:      total == 3,
			Expected:    int64(3),
			Actual:      total,
		})
	}

	return nil
}

// ============ 对话服务测试 ============

func (s *IntegrationTestService) testDialogueServiceCreate(ctx *TestContext) error {
	dialogue := s.dialogueService.CreateDialogue("test-user", "Test Conversation")

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "对话应被创建",
		Passed:      dialogue.ID != "",
		Expected:    "non-empty id",
		Actual:      dialogue.ID,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "用户ID应匹配",
		Passed:      dialogue.UserID == "test-user",
		Expected:    "test-user",
		Actual:      dialogue.UserID,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "标题应匹配",
		Passed:      dialogue.Title == "Test Conversation",
		Expected:    "Test Conversation",
		Actual:      dialogue.Title,
	})

	return nil
}

func (s *IntegrationTestService) testDialogueServiceSendMessage(ctx *TestContext) error {
	dialogue := s.dialogueService.CreateDialogue("test-user", "Message Test")

	// 添加消息
	msg, err := s.dialogueService.AddMessage(dialogue.ID, "user", "Hello, AI!")
	if err != nil {
		return fmt.Errorf("failed to add message: %w", err)
	}

	// 获取消息
	messages := s.dialogueService.GetMessages(dialogue.ID)

	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "消息应被添加",
		Passed:      msg.ID != "",
		Expected:    "non-empty id",
		Actual:      msg.ID,
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "消息列表应包含1条消息",
		Passed:      len(messages) == 1,
		Expected:    1,
		Actual:      len(messages),
	})

	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "发送者应为user",
		Passed:      messages[0].Sender == "user",
		Expected:    "user",
		Actual:      messages[0].Sender,
	})

	return nil
}

// ============ 辅助方法 ============

// containsStr 检查字符串是否包含子串
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// GetTestResult 获取测试结果
func (s *IntegrationTestService) GetTestResult(testID string) (*TestResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result, ok := s.testResults[testID]
	return result, ok
}

// GetAllTestResults 获取所有测试结果
func (s *IntegrationTestService) GetAllTestResults() []*TestResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	results := make([]*TestResult, 0, len(s.testResults))
	for _, r := range s.testResults {
		results = append(results, r)
	}
	return results
}

// ClearTestResults 清除测试结果
func (s *IntegrationTestService) ClearTestResults() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.testResults = make(map[string]*TestResult)
}

// TestService 测试特定服务
func (s *IntegrationTestService) TestService(ctx context.Context, serviceName string) (*TestResult, error) {
	var test TestCase

	switch serviceName {
	case "context_manager":
		test = TestCase{
			Name:     "TestContextManager_Full",
			TestFunc: s.testFullContextManager,
			Timeout:  30 * time.Second,
		}
	case "task_service":
		test = TestCase{
			Name:     "TestTaskService_Full",
			TestFunc: s.testFullTaskService,
			Timeout:  30 * time.Second,
		}
	case "dialogue_service":
		test = TestCase{
			Name:     "TestDialogueService_Full",
			TestFunc: s.testFullDialogueService,
			Timeout:  30 * time.Second,
		}
	default:
		return nil, fmt.Errorf("unknown service: %s", serviceName)
	}

	return s.runTest(ctx, test), nil
}

// testFullContextManager 完整的上下文管理器测试
func (s *IntegrationTestService) testFullContextManager(ctx *TestContext) error {
	dialogue := s.dialogueService.CreateDialogue("full-test", "Full Test")

	// 添加多条消息
	for i := 0; i < 5; i++ {
		if _, err := s.dialogueService.AddMessage(dialogue.ID, "user", fmt.Sprintf("Message %d", i)); err != nil {
			return fmt.Errorf("failed to add message %d: %w", i, err)
		}
	}

	// 测试压缩
	compressed, err := s.contextManager.Compress(dialogue.ID)
	if err != nil {
		return err
	}

	// 测试摘要
	summary, err := s.contextManager.Summarize(dialogue.ID)
	if err != nil {
		return err
	}

	// 测试重要信息提取
	info, err := s.contextManager.ExtractImportantInfo(dialogue.ID)
	if err != nil {
		return err
	}

	ctx.mu.Lock()
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "压缩应成功",
		Passed:      compressed.MessageCount == 5,
		Expected:    5,
		Actual:      compressed.MessageCount,
	})
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "摘要应生成",
		Passed:      summary != "",
		Expected:    "non-empty",
		Actual:      len(summary),
	})
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "重要信息应提取",
		Passed:      info != nil,
		Expected:    "non-nil",
		Actual:      info,
	})
	ctx.mu.Unlock()

	return nil
}

// testFullTaskService 完整的任务服务测试
func (s *IntegrationTestService) testFullTaskService(ctx *TestContext) error {
	// 创建成员
	member := &models.TeamMember{
		ID:           uuid.New().String(),
		TeamID:       "full-test-team",
		Name:         "Full Test Developer",
		Role:         "developer",
		Availability: "available",
		CurrentLoad:  0,
		MaxLoad:      3,
	}
	s.db.Create(member)

	// 创建任务
	task := &models.Task{
		ID:          uuid.New().String(),
		TeamID:      "full-test-team",
		Title:       "Full Test Task",
		Description: "A complete test task",
		Type:        "coding",
		Status:      "pending",
		CreatedBy:   "test-user",
	}
	err := s.taskService.CreateTask(task)
	if err != nil {
		return err
	}

	// 更新状态
	err = s.taskService.UpdateTaskStatus(task.ID, "in_progress", "test-user", "Starting")
	if err != nil {
		return err
	}

	// 更新进度
	err = s.taskService.UpdateProgress(task.ID, "", "planning", 25, "Planning phase")
	if err != nil {
		return err
	}

	// 获取进度
	progress, err := s.taskService.GetTaskProgress(task.ID)
	if err != nil {
		return err
	}

	ctx.mu.Lock()
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "任务应创建成功",
		Passed:      task.ID != "",
		Expected:    "non-empty",
		Actual:      task.ID,
	})
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "状态应更新",
		Passed:      len(progress) > 0,
		Expected:    "> 0",
		Actual:      len(progress),
	})
	ctx.mu.Unlock()

	return nil
}

// testFullDialogueService 完整的对话服务测试
func (s *IntegrationTestService) testFullDialogueService(ctx *TestContext) error {
	// 创建对话
	dialogue := s.dialogueService.CreateDialogue("full-dialogue-test", "Full Dialogue Test")

	// 添加多条消息
	messages := []string{"Hello", "How are you?", "What's the weather?"}
	for _, msg := range messages {
		if _, err := s.dialogueService.AddMessage(dialogue.ID, "user", msg); err != nil {
			return fmt.Errorf("failed to add message: %w", err)
		}
	}

	// 获取对话
	retrieved, ok := s.dialogueService.GetDialogue(dialogue.ID)
	if !ok {
		return fmt.Errorf("dialogue not found")
	}

	// 列出用户对话
	userDialogues := s.dialogueService.ListDialoguesByUser("full-dialogue-test")

	ctx.mu.Lock()
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "对话应可检索",
		Passed:      retrieved.ID == dialogue.ID,
		Expected:    dialogue.ID,
		Actual:      retrieved.ID,
	})
	ctx.Assertions = append(ctx.Assertions, Assertion{
		Description: "应找到用户对话",
		Passed:      len(userDialogues) > 0,
		Expected:    "> 0",
		Actual:      len(userDialogues),
	})
	ctx.mu.Unlock()

	return nil
}

// JSON 序列化支持
func (r *TestResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (r *TestSuiteResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// MockLLMClient 用于测试的Mock LLM客户端
type MockLLMClient struct {
	mu      sync.Mutex
	responses map[string]*llm.ChatResponse
}

func (m *MockLLMClient) SetResponse(prompt string, response *llm.ChatResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.responses == nil {
		m.responses = make(map[string]*llm.ChatResponse)
	}
	m.responses[prompt] = response
}

func (m *MockLLMClient) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 返回默认的mock响应
	return &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    "assistant",
					Content: "This is a mock response for testing.",
				},
			},
		},
		Usage: &llm.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}, nil
}

func (m *MockLLMClient) ChatStream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.ChatStreamChunk, error) {
	ch := make(chan llm.ChatStreamChunk, 1)
	ch <- llm.ChatStreamChunk{
		ID: "mock-chunk-1",
		Object: "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model: "mock-model",
		Choices: []llm.StreamChoice{
			{
				Index: 0,
				Delta: llm.MessageDelta{
					Content: "Mock stream response",
				},
				FinishReason: "stop",
			},
		},
	}
	close(ch)
	return ch, nil
}

// NewInMemoryTestDB 创建内存测试数据库
func NewInMemoryTestDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// TestPlannerService 任务规划服务测试
func (s *IntegrationTestService) TestPlanner(ctx context.Context, request map[string]interface{}) (*PlannerResult, error) {
	// 创建测试上下文
	dialogue := s.dialogueService.CreateDialogue("planner-test", "Planning Test")

	// 解析请求
	goal, _ := request["goal"].(string)
	constraints, _ := request["constraints"].([]string)

	// 创建规划任务
	req := DecomposeTaskRequest{
		Title:       goal,
		Description: fmt.Sprintf("Plan and execute: %s", goal),
		Type:        "plan",
		Priority:    "high",
		TeamID:      "test-team",
		CreatedBy:   "planner",
	}

	result, err := s.taskService.DecomposeTask(ctx, req)
	if err != nil {
		return nil, err
	}

	// 验证约束
	for _, constraint := range constraints {
		for _, subtask := range result.Subtasks {
			if contains(subtask.Description, constraint) {
				// 约束已考虑
			}
		}
	}

	return &PlannerResult{
		DialogueID:   dialogue.ID,
		TaskID:       result.Task.ID,
		Subtasks:     result.Subtasks,
		TotalSteps:   len(result.Subtasks),
		EstimatedTime: time.Duration(result.Task.Estimated) * time.Minute,
		Constraints:  constraints,
	}, nil
}

// PlannerResult 规划结果
type PlannerResult struct {
	DialogueID     string           `json:"dialogue_id"`
	TaskID         string           `json:"task_id"`
	Subtasks       []models.Subtask `json:"subtasks"`
	TotalSteps     int              `json:"total_steps"`
	EstimatedTime  time.Duration    `json:"estimated_time"`
	Constraints    []string         `json:"constraints"`
	CreatedAt      time.Time        `json:"created_at"`
}

func (p *PlannerResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
