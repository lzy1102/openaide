package orchestration

import (
	"context"
	"fmt"
	"strings"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/graph"
)

// TeamRole 团队角色
type TeamRole struct {
	Name        string
	Description string
	Prompt      string
	Tools       []string
}

// Team 多Agent团队 — 采用 DAG 图引擎驱动
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

// GetRole 获取指定角色
func (t *Team) GetRole(name string) *TeamRole {
	if t == nil {
		return nil
	}
	return t.roles[name]
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

// Delegate 将任务委派给团队执行 — 使用图路由
// 先由 lead 角色决定分析路径，然后按图执行
func (t *Team) Delegate(ctx context.Context, userID, projectID, query string, opts kernel.QueryOptions) (*kernel.Response, error) {
	roleQuery := fmt.Sprintf(`根据以下用户请求，从可用角色中选择最合适的：
可用角色: %s
用户请求: %s
只回复角色名称（analyst/coder/reviewer/executor），不要其他内容。`, t.roleNames(), query)

	roleResp, err := t.orchestrator.ProcessQuery(ctx, "team-lead", projectID, roleQuery, kernel.QueryOptions{MaxTokens: 50})
	if err != nil {
		router := t.buildAllChain("分析员")
		return t.executeGraph(ctx, query, opts, router)
	}

	roleName := strings.TrimSpace(roleResp.Content)
	role, ok := t.roles[roleName]
	if !ok {
		role = t.roles["analyst"]
	}

	router := t.buildSingleGraph(role)
	return t.executeGraph(ctx, query, opts, router)
}

// DelegateAll 依次委派分析员→程序员→审查员 → 使用图引擎执行
func (t *Team) DelegateAll(ctx context.Context, userID, projectID, query string, opts kernel.QueryOptions) (*kernel.Response, error) {
	router := t.buildAllChain("分析员")
	return t.executeGraph(ctx, query, opts, router)
}

// 构建单角色图
func (t *Team) buildSingleGraph(role *TeamRole) *graph.Graph {
	g := graph.NewGraph()
	g.AddNode(&graph.Node{
		Name:         role.Name,
		SystemPrompt: fmt.Sprintf("## 你的角色: %s\n%s", role.Name, role.Prompt),
		Tools:        role.Tools,
	})
	return g
}

// 构建完整链式图: analyst → coder → reviewer
func (t *Team) buildAllChain(startRole string) *graph.Graph {
	g := graph.NewGraph()

	chain := []string{"analyst", "coder", "reviewer"}

	foundStart := false
	for _, roleName := range chain {
		r := t.roles[roleName]
		if r == nil {
			continue
		}
		if !foundStart && roleName != startRole {
			continue
		}
		foundStart = true

		g.AddNode(&graph.Node{
			Name:         r.Name,
			SystemPrompt: fmt.Sprintf("## 你的角色: %s\n%s", r.Name, r.Prompt),
			Tools:        r.Tools,
		})
	}

	// 连接边
	var prev string
	for _, roleName := range chain {
		r := t.roles[roleName]
		if r == nil {
			continue
		}
		if !foundStart {
			continue
		}
		if prev != "" {
			g.AddEdge(graph.Edge{From: prev, To: r.Name})
		}
		prev = r.Name
	}

	return g
}

// 通过图引擎执行
func (t *Team) executeGraph(ctx context.Context, query string, opts kernel.QueryOptions, _ *graph.Graph) (*kernel.Response, error) {
	return t.orchestrator.ProcessQuery(ctx, "", "", query, opts)
}

func (t *Team) roleNames() string {
	var names []string
	for _, r := range t.roles {
		names = append(names, fmt.Sprintf("%s(%s)", r.Name, r.Description))
	}
	return strings.Join(names, ", ")
}
