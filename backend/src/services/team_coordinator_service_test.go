package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"openaide/backend/src/models"
)

// setupTestDBForTeam 设置测试数据库
func setupTestDBForTeam(t *testing.T) *gorm.DB {
	// 使用唯一的数据库文件名确保每个测试都有独立的数据库
	dsn := fmt.Sprintf("file:test_%d.db?cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	require.NoError(t, err)

	// 自动迁移
	err = db.AutoMigrate(
		&models.Team{},
		&models.TeamMember{},
		&models.TeamEvent{},
		&models.TeamMessage{},
		&models.ProgressReport{},
		&models.TaskProgress{},
		&models.TaskAssignment{},
	)
	require.NoError(t, err)

	// 手动创建任务表（排除 GORM 不支持的切片字段）
	db.Exec(`DROP TABLE IF EXISTS tasks`)
	db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		team_id TEXT,
		title TEXT NOT NULL,
		description TEXT,
		type TEXT,
		priority TEXT,
		status TEXT,
		complexity INTEGER DEFAULT 0,
		estimated INTEGER DEFAULT 0,
		parent_task_id TEXT,
		assigned_to TEXT,
		created_by TEXT,
		created_at DATETIME,
		updated_at DATETIME,
		started_at DATETIME,
		completed_at DATETIME,
		context_metadata TEXT,
		retry_count INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3
	)`)

	// 清理函数
	t.Cleanup(func() {
		db.Exec("DROP TABLE IF EXISTS tasks")
		db.Exec("DROP TABLE IF EXISTS teams")
		db.Exec("DROP TABLE IF EXISTS team_members")
		db.Exec("DROP TABLE IF EXISTS team_events")
		db.Exec("DROP TABLE IF EXISTS team_messages")
		db.Exec("DROP TABLE IF EXISTS progress_reports")
		db.Exec("DROP TABLE IF EXISTS task_progresses")
		db.Exec("DROP TABLE IF EXISTS task_assignments")
	})

	return db
}

// TestTeamCoordinatorService_CreateTeam 测试创建团队
func TestTeamCoordinatorService_CreateTeam(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	team, err := service.CreateTeam("Test Team", "A test team", nil)

	require.NoError(t, err)
	assert.NotNil(t, team)
	assert.NotEmpty(t, team.ID)
	assert.Equal(t, "Test Team", team.Name)
	assert.Equal(t, "A test team", team.Description)
	assert.True(t, team.Enabled)
	_ = team.Config
}

// TestTeamCoordinatorService_AddMember 测试添加成员
func TestTeamCoordinatorService_AddMember(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, err := service.CreateTeam("Test Team", "Description", nil)
	require.NoError(t, err)

	// 添加成员
	member, err := service.AddMember(
		team.ID,
		"TestMember",
		"developer",
		"agent",
		[]string{"coding", "review"},
		map[string]interface{}{"model": "gpt-4"},
	)

	require.NoError(t, err)
	assert.NotNil(t, member)
	assert.NotEmpty(t, member.ID)
	assert.Equal(t, team.ID, member.TeamID)
	assert.Equal(t, "TestMember", member.Name)
	assert.Equal(t, "developer", member.Role)
	assert.Equal(t, "active", member.Availability)
}

// TestTeamCoordinatorService_CreateTask 测试创建任务
func TestTeamCoordinatorService_CreateTask(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, err := service.CreateTeam("Test Team", "Description", nil)
	require.NoError(t, err)

	// 创建任务
	task, err := service.CreateTask(
		team.ID,
		"workflow-123",
		"Test Task",
		"A test task",
		"development",
		"high",
		"system",
		map[string]interface{}{"input_data": "test"},
	)

	require.NoError(t, err)
	assert.NotNil(t, task)
	assert.NotEmpty(t, task.ID)
	assert.Equal(t, team.ID, task.TeamID)
	assert.Equal(t, "Test Task", task.Title)
	assert.Equal(t, "pending", task.Status)
}

// TestTeamCoordinatorService_AssignTask 测试分配任务
func TestTeamCoordinatorService_AssignTask(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "TestMember", "developer", "agent", []string{}, nil)

	// 创建任务
	task, _ := service.CreateTask(
		team.ID,
		"",
		"Test Task",
		"Description",
		"development",
		"high",
		"system",
		nil,
	)

	// 分配任务
	err := service.AssignTask(task.ID, member.ID)
	require.NoError(t, err)

	// 验证任务状态
	updatedTask, _ := service.GetTask(task.ID)
	assert.Equal(t, member.ID, updatedTask.AssignedTo)
	assert.Equal(t, "in_progress", updatedTask.Status)
}

// TestTeamCoordinatorService_UpdateTaskStatus 测试更新任务状态
func TestTeamCoordinatorService_UpdateTaskStatus(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和任务
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	task, _ := service.CreateTask(team.ID, "", "Test", "Desc", "dev", "high", "system", nil)

	// 更新为完成状态
	err := service.UpdateTaskStatus(task.ID, "completed", "")
	require.NoError(t, err)

	// 验证更新
	updatedTask, _ := service.GetTask(task.ID)
	assert.Equal(t, "completed", updatedTask.Status)
	assert.NotNil(t, updatedTask.CompletedAt)
	assert.True(t, updatedTask.Result.Success)
}

// TestTeamCoordinatorService_TrackProgress 测试跟踪进度
func TestTeamCoordinatorService_TrackProgress(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member1, _ := service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)
	member2, _ := service.AddMember(team.ID, "Member2", "tester", "agent", []string{}, nil)

	// 创建多个任务
	task1, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)
	task2, _ := service.CreateTask(team.ID, "", "Task2", "Desc", "dev", "medium", "system", nil)
	task3, _ := service.CreateTask(team.ID, "", "Task3", "Desc", "test", "low", "system", nil)

	// 分配任务
	service.AssignTask(task1.ID, member1.ID)
	service.AssignTask(task2.ID, member1.ID)
	service.AssignTask(task3.ID, member2.ID)

	// 更新一些任务状态
	service.UpdateTaskStatus(task1.ID, "completed", "")
	service.UpdateTaskProgress(task2.ID, 50)

	// 跟踪进度
	progress, err := service.TrackProgress(team.ID)
	require.NoError(t, err)

	assert.Equal(t, 3, progress.TotalTasks)
	assert.Equal(t, 1, progress.CompletedTasks)
	assert.Equal(t, 2, progress.InProgressTasks) // task2 和 task3 都是 in_progress
	assert.Equal(t, 0, progress.PendingTasks)

	// 验证成员统计
	assert.Equal(t, 2, progress.TasksByMember[member1.ID].AssignedTasks)
	assert.Equal(t, 1, progress.TasksByMember[member2.ID].AssignedTasks)
}

// TestTeamCoordinatorService_GenerateReport 测试生成报告
func TestTeamCoordinatorService_GenerateReport(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和任务
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "TestMember", "developer", "agent", []string{}, nil)
	task1, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)
	task2, _ := service.CreateTask(team.ID, "", "Task2", "Desc", "dev", "low", "system", nil)

	service.AssignTask(task1.ID, member.ID)
	service.UpdateTaskStatus(task1.ID, "completed", "")
	service.AssignTask(task2.ID, member.ID)

	// 生成报告
	periodEnd := time.Now()
	periodStart := periodEnd.Add(-24 * time.Hour)
	report, err := service.GenerateReport(team.ID, "system", periodStart, periodEnd)

	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.NotEmpty(t, report.ID)
	assert.Equal(t, team.ID, report.TeamID)
	assert.NotEmpty(t, report.Summary)
	assert.Equal(t, 2, report.TaskStatus.Total)
	assert.Equal(t, 1, report.TaskStatus.Completed)
	assert.Len(t, report.MemberStats, 1)
}

// TestTeamCoordinatorService_NotifyMember 测试通知成员
func TestTeamCoordinatorService_NotifyMember(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "TestMember", "developer", "agent", []string{}, nil)

	// 发送通知
	err := service.NotifyMember(team.ID, member.ID, "test_notification", map[string]interface{}{
		"message": "Hello",
	})
	require.NoError(t, err)

	// 验证消息
	messages, total, err := service.ListMessages(team.ID, member.ID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, messages, 1)
	assert.Equal(t, "test_notification", messages[0].Type)
}

// TestTeamCoordinatorService_SaveAndLoadTeamConfig 测试保存和加载团队配置
func TestTeamCoordinatorService_SaveAndLoadTeamConfig(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Original Team", "Description", nil)
	_, _ = service.AddMember(team.ID, "Member1", "developer", "agent", []string{"coding"}, nil)
	task, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)

	// 保存配置
	config, err := service.SaveTeamConfig(team.ID)
	require.NoError(t, err)
	assert.Equal(t, team.ID, config.Team.ID)
	assert.Len(t, config.Members, 1)
	assert.Len(t, config.Tasks, 1)

	// 从配置恢复新团队
	newTeam, err := service.LoadTeamConfig(config)
	require.NoError(t, err)
	assert.NotEqual(t, team.ID, newTeam.ID)
	assert.Equal(t, "Original Team", newTeam.Name)

	// 验证恢复的成员
	members, _ := service.ListMembers(newTeam.ID)
	assert.Len(t, members, 1)
	assert.Equal(t, "Member1", members[0].Name)

	// 验证恢复的任务
	tasks, total, _ := service.ListTasks(newTeam.ID, "", "", 1, 10)
	assert.Equal(t, int64(1), total)
	assert.NotEqual(t, task.ID, tasks[0].ID) // 新任务应该有新ID
	assert.Equal(t, "Task1", tasks[0].Title)
}

// TestTeamCoordinatorService_ExportImportTeamConfig 测试导入导出
func TestTeamCoordinatorService_ExportImportTeamConfig(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Export Team", "Description", nil)
	service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)

	// 导出配置
	data, err := service.ExportTeamConfig(team.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.Contains(t, string(data), "Export Team")

	// 导入配置
	importedTeam, err := service.ImportTeamConfig(data)
	require.NoError(t, err)
	assert.NotNil(t, importedTeam)
	assert.Equal(t, "Export Team", importedTeam.Name)
}

// TestTeamCoordinatorService_RetryTask 测试重试任务
func TestTeamCoordinatorService_RetryTask(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和任务
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	task, _ := service.CreateTask(team.ID, "", "Test Task", "Desc", "dev", "high", "system", nil)

	// 先设置为失败
	service.UpdateTaskStatus(task.ID, "failed", "temporary error")

	// 重试任务（注意：RetryTask 是异步的）
	err := service.RetryTask(task.ID)
	require.NoError(t, err)

	// 等待异步操作完成
	time.Sleep(100 * time.Millisecond)

	// 验证任务状态（executeTask 会模拟完成）
	updatedTask, _ := service.GetTask(task.ID)
	// executeTask 会将状态设置为 completed（因为 success = true）
	assert.Equal(t, "completed", updatedTask.Status)
	assert.Equal(t, 1, updatedTask.RetryCount)
}

// TestTeamCoordinatorService_Events 测试事件系统
func TestTeamCoordinatorService_Events(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Test Team", "Description", nil)

	// 订阅事件
	eventCh := service.SubscribeToEvents(team.ID)
	defer service.UnsubscribeFromEvents(team.ID, eventCh)

	// 触发事件
	service.AddMember(team.ID, "NewMember", "developer", "agent", []string{}, nil)

	// 等待事件
	select {
	case event := <-eventCh:
		assert.Equal(t, "member_added", event.Type)
		assert.Equal(t, team.ID, event.TeamID)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

// TestTeamCoordinatorService_GetEvents 测试获取事件历史
func TestTeamCoordinatorService_GetEvents(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Test Team", "Description", nil)

	// 生成一些事件
	service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)
	service.AddMember(team.ID, "Member2", "tester", "agent", []string{}, nil)

	// 获取事件
	events, err := service.GetEvents(team.ID, "member_added", 10)
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

// TestTeamCoordinatorService_ListTasks 测试列出任务
func TestTeamCoordinatorService_ListTasks(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)

	// 创建任务
	task1, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)
	task2, _ := service.CreateTask(team.ID, "", "Task2", "Desc", "dev", "low", "system", nil)
	service.AssignTask(task1.ID, member.ID)
	service.UpdateTaskStatus(task1.ID, "completed", "")

	// 测试过滤
	pendingTasks, total, _ := service.ListTasks(team.ID, "pending", "", 1, 10)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, task2.ID, pendingTasks[0].ID)

	completedTasks, total, _ := service.ListTasks(team.ID, "completed", "", 1, 10)
	assert.Equal(t, int64(1), total)
	assert.Equal(t, task1.ID, completedTasks[0].ID)

	memberTasks, total, _ := service.ListTasks(team.ID, "", member.ID, 1, 10)
	assert.Equal(t, int64(1), total) // 只有 task1 被分配给 member
	assert.Equal(t, task1.ID, memberTasks[0].ID)
	_ = memberTasks
}

// TestTeamCoordinatorService_MemberStatus 测试成员状态管理
func TestTeamCoordinatorService_MemberStatus(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)

	// 检查初始状态
	status := service.GetMemberStatus(member.ID)
	assert.Equal(t, "active", status)

	// 更新状态
	err := service.UpdateMemberStatus(member.ID, "busy")
	require.NoError(t, err)

	// 验证更新
	status = service.GetMemberStatus(member.ID)
	assert.Equal(t, "busy", status)
}

// TestTeamCoordinatorService_RemoveMember 测试移除成员
func TestTeamCoordinatorService_RemoveMember(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)

	// 分配任务给成员
	task, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)
	service.AssignTask(task.ID, member.ID)

	// 尝试移除有活动任务的成员（应该失败）
	err := service.RemoveMember(team.ID, member.ID)
	assert.Error(t, err)

	// 完成任务后移除
	service.UpdateTaskStatus(task.ID, "completed", "")
	err = service.RemoveMember(team.ID, member.ID)
	require.NoError(t, err)

	// 验证成员已移除
	_, err = service.GetMember(member.ID)
	assert.Error(t, err)
}

// TestTeamCoordinatorService_BroadcastMessage 测试广播消息
func TestTeamCoordinatorService_BroadcastMessage(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和多个成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)
	service.AddMember(team.ID, "Member2", "tester", "agent", []string{}, nil)
	service.AddMember(team.ID, "Member3", "reviewer", "agent", []string{}, nil)

	// 广播消息
	content := map[string]interface{}{"announcement": "Team meeting at 3pm"}
	err := service.BroadcastMessage(team.ID, "coordinator", "broadcast", content)
	require.NoError(t, err)

	// 验证每个成员都收到消息
	messages, total, _ := service.ListMessages(team.ID, "", 1, 100)
	assert.Equal(t, int64(3), total)
	assert.Len(t, messages, 3)
}

// TestTeamCoordinatorService_StartStop 测试服务启动和停止
func TestTeamCoordinatorService_StartStop(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 启动服务
	err := service.Start(ctx)
	require.NoError(t, err)

	// 创建一些活跃任务
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)
	service.CreateTask(team.ID, "", "Task2", "Desc", "dev", "high", "system", nil)

	// 停止服务
	err = service.Stop()
	require.NoError(t, err)
}

// TestTeamCoordinatorService_UpdateTaskProgress 测试更新任务进度
func TestTeamCoordinatorService_UpdateTaskProgress(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和任务
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	task, _ := service.CreateTask(team.ID, "", "Task1", "Desc", "dev", "high", "system", nil)

	// 更新进度
	err := service.UpdateTaskProgress(task.ID, 50)
	require.NoError(t, err)

	// 验证进度记录
	var progresses []models.TaskProgress
	db.Where("task_id = ?", task.ID).Find(&progresses)
	assert.Len(t, progresses, 1)
	assert.Equal(t, 50, progresses[0].Percent)
}

// TestTeamCoordinatorService_MarkMessageRead 测试标记消息已读
func TestTeamCoordinatorService_MarkMessageRead(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队、成员和消息
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	member, _ := service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)
	service.NotifyMember(team.ID, member.ID, "test", map[string]interface{}{"msg": "test"})

	// 获取消息
	messages, _, _ := service.ListMessages(team.ID, member.ID, 1, 1)
	require.Len(t, messages, 1)

	// 标记为已读
	err := service.MarkMessageRead(messages[0].ID)
	require.NoError(t, err)

	// 验证状态
	message, _ := service.GetMessage(messages[0].ID)
	assert.Equal(t, "read", message.Status)
	assert.NotNil(t, message.ReadAt)
}

// TestTeamCoordinatorService_ListTeams 测试列出团队
func TestTeamCoordinatorService_ListTeams(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建多个团队
	service.CreateTeam("Team1", "Desc1", nil)
	service.CreateTeam("Team2", "Desc2", nil)
	service.CreateTeam("Team3", "Desc3", nil)

	// 列出团队
	teams, err := service.ListTeams()
	require.NoError(t, err)
	assert.Len(t, teams, 3)
}

// TestTeamCoordinatorService_GetTeam 测试获取团队
func TestTeamCoordinatorService_GetTeam(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	createdTeam, _ := service.CreateTeam("Test Team", "Description", nil)

	// 获取团队
	retrievedTeam, err := service.GetTeam(createdTeam.ID)
	require.NoError(t, err)
	assert.Equal(t, createdTeam.ID, retrievedTeam.ID)
	assert.Equal(t, "Test Team", retrievedTeam.Name)
}

// TestTeamCoordinatorService_ListMembers 测试列出成员
func TestTeamCoordinatorService_ListMembers(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队和成员
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	service.AddMember(team.ID, "Member1", "developer", "agent", []string{}, nil)
	service.AddMember(team.ID, "Member2", "tester", "agent", []string{}, nil)

	// 列出成员
	members, err := service.ListMembers(team.ID)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

// TestTeamCoordinatorService_ListReports 测试列出报告
func TestTeamCoordinatorService_ListReports(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队
	team, _ := service.CreateTeam("Test Team", "Description", nil)

	// 生成报告
	periodEnd := time.Now()
	periodStart := periodEnd.Add(-24 * time.Hour)
	service.GenerateReport(team.ID, "system", periodStart, periodEnd)
	service.GenerateReport(team.ID, "system", periodStart, periodEnd)

	// 列出报告
	reports, total, err := service.ListReports(team.ID, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, reports, 2)
}

// TestTeamCoordinatorService_GetReport 测试获取报告
func TestTeamCoordinatorService_GetReport(t *testing.T) {
	db := setupTestDBForTeam(t)
	service := NewTeamCoordinatorService(db, "")

	// 创建团队并生成报告
	team, _ := service.CreateTeam("Test Team", "Description", nil)
	periodEnd := time.Now()
	periodStart := periodEnd.Add(-24 * time.Hour)
	createdReport, _ := service.GenerateReport(team.ID, "system", periodStart, periodEnd)

	// 获取报告
	retrievedReport, err := service.GetReport(createdReport.ID)
	require.NoError(t, err)
	assert.Equal(t, createdReport.ID, retrievedReport.ID)
	assert.NotEmpty(t, retrievedReport.Summary)
}
