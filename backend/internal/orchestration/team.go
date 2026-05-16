package orchestration

import (
	"context"
	"fmt"
	"strings"

	"openaide/backend/internal/kernel"
)

// TeamRole 团队角色
type TeamRole struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Tools       []string `json:"tools"`
}

// Team 多Agent团队
type Team struct {
	orchestrator *Orchestrator
	roles        map[string]*TeamRole
}

// NewTeam 创建团队
func NewTeam(o *Orchestrator) *Team {
	return &Team{
		orchestrator: o,
		roles:        defaultRoles(),
	}
}

func defaultRoles() map[string]*TeamRole {
	return map[string]*TeamRole{
		"analyst": {
			Name: "分析员", Description: "分析问题，理解需求，制定方案",
			Prompt: "你是分析员。分析用户需求，理解问题本质，制定解决方案。输出结构化的分析报告。",
			Tools:  []string{"read_file", "search_files", "search_knowledge", "search_symbols"},
		},
		"coder": {
			Name: "程序员", Description: "编写和修改代码",
			Prompt: "你是程序员。根据分析报告编写或修改代码。遵循最佳实践，处理边界情况。",
			Tools:  []string{"read_file", "write_file", "execute_command", "search_files", "search_symbols"},
		},
		"reviewer": {
			Name: "审查员", Description: "审查代码质量、安全和正确性",
			Prompt: "你是代码审查员。检查代码的正确性、安全性、性能和可读性。输出审查意见。",
			Tools:  []string{"read_file", "search_files", "execute_command"},
		},
		"executor": {
			Name: "执行者", Description: "执行命令、测试、部署",
			Prompt: "你是执行者。运行测试、执行命令、验证结果。报告执行状态。",
			Tools:  []string{"execute_command", "git_status", "read_file"},
		},
	}
}

// Delegate 将任务委派给团队角色执行
func (t *Team) Delegate(ctx context.Context, userID, projectID, query string, opts kernel.QueryOptions) (*kernel.Response, error) {
	// 1. 分析 → 分配角色
	roleQuery := fmt.Sprintf(`根据以下用户请求，从可用角色中选择最合适的：
可用角色: %s
用户请求: %s
只回复角色名称（analyst/coder/reviewer/executor），不要其他内容。`, t.roleNames(), query)

	roleResp, err := t.orchestrator.ProcessQuery(ctx, "team-lead", projectID, roleQuery, kernel.QueryOptions{MaxTokens: 50})
	if err != nil {
		return t.orchestrator.ProcessQuery(ctx, userID, projectID, query, opts)
	}

	roleName := strings.TrimSpace(roleResp.Content)
	role, ok := t.roles[roleName]
	if !ok {
		// 默认分析员
		role = t.roles["analyst"]
	}

	// 2. 用该角色的prompt执行任务
	roleQuery2 := fmt.Sprintf("## 你的角色: %s\n%s\n\n## 用户请求:\n%s\n\n请以你的角色完成此任务。", role.Name, role.Prompt, query)
	if len(role.Tools) > 0 {
		opts.ToolFilter = role.Tools
	}

	result, err := t.orchestrator.ProcessQuery(ctx, userID, projectID, roleQuery2, opts)
	if err != nil {
		return nil, err
	}

	// 3. 打包结果
	result.Content = fmt.Sprintf("[%s] %s", role.Name, result.Content)
	return result, nil
}

// DelegateAll 将任务依次委派给多个角色
func (t *Team) DelegateAll(ctx context.Context, userID, projectID, query string, opts kernel.QueryOptions) (*kernel.Response, error) {
	chain := []string{"analyst", "coder", "reviewer"}
	var results []string
	totalTools := 0

	context := query
	for _, roleName := range chain {
		role := t.roles[roleName]
		roleQuery := fmt.Sprintf("## 你的角色: %s\n%s\n\n## 任务:\n%s\n\n## 前面步骤的结果:\n%s",
			role.Name, role.Prompt, query, strings.Join(results, "\n---\n"))

		rOpts := opts
		rOpts.ToolFilter = role.Tools
		resp, err := t.orchestrator.ProcessQuery(ctx, userID, projectID, roleQuery, rOpts)
		if err != nil {
			results = append(results, fmt.Sprintf("❌ %s: %v", role.Name, err))
			continue
		}
		results = append(results, fmt.Sprintf("## %s\n%s", role.Name, resp.Content))
		totalTools += resp.ToolCalls
		_ = context
	}

	return &kernel.Response{
		Content:   strings.Join(results, "\n\n---\n\n"),
		ToolCalls: totalTools,
	}, nil
}

func (t *Team) roleNames() string {
	var names []string
	for _, r := range t.roles {
		names = append(names, fmt.Sprintf("%s(%s)", r.Name, r.Description))
	}
	return strings.Join(names, ", ")
}
