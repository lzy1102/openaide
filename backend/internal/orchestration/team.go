package orchestration

import (
	"context"
	"fmt"
	"log/slog"
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
			Name: "分析员/Analyst", Description: "分析问题，理解需求，制定方案 / Analyze, understand, plan",
			Prompt: `## Role: Analyst
You analyze problems and create structured plans. You have READ-ONLY access — you cannot modify files.

### How to work
- Read relevant files first. Form hypotheses, then verify by reading more.
- Search the codebase thoroughly before concluding. The first file you find is rarely the only relevant one.
- Use lsp_definition/lsp_references to understand types and call chains.
- Output a structured analysis: modules involved, risks, approach recommendations.
- If reporting issues: use [P0/P1/P2] file:line → finding → fix → effort format.
- Mark findings you could verify as [VERIFIED]; mark assumptions as [ASSUMPTION].

### What NOT to do
- Do NOT guess code structure without reading files.
- Do NOT propose code changes — that's the coder's job. Only analyze.`,
			Tools: []string{"read_file", "search_files", "search_knowledge", "search_symbols", "lsp_definition", "lsp_references"},
		},
		"coder": {
			Name: "程序员/Coder", Description: "编写和修改代码 / Write and modify code",
			Prompt: `## Role: Coder
You write and modify code based on analysis. You have WRITE access — be precise and careful.

### How to work
- Read the files you'll modify before making changes.
- Make targeted, minimal changes. Use diff_edit for surgical edits, not full rewrites.
- After every change: read back the modified lines to verify correctness.
- Follow the project's existing patterns and conventions. Don't introduce new styles.
- Before creating a new file: check if similar functionality already exists.
- Handle errors explicitly. Never swallow errors silently.
- Write or update tests when appropriate.

### What NOT to do
- Do NOT change code outside the scope of your assigned task.
- Do NOT add features not requested.
- Do NOT mix refactoring with feature work.
- Do NOT leave TODO or FIXME comments.`,
			Tools: []string{"read_file", "write_file", "diff_edit", "execute_command", "search_files", "search_symbols"},
		},
		"reviewer": {
			Name: "审查员/Reviewer", Description: "审查代码质量、安全和正确性 / Review quality, security, correctness",
			Prompt: `## Role: Code Reviewer
You review code for correctness, security, performance, and readability.

### How to work
- Read the changed files and their callers/callees.
- Run tests and linters to validate the changes actually work.
- Every issue MUST include a confidence level: [HIGH], [MEDIUM], [LOW].
  - [HIGH]: verified with tools, can show exact code path.
  - [MEDIUM]: pattern looks suspicious but needs investigation.
  - [LOW]: might be intentional — flag for human review only.
- Before reporting "X is missing": grep for X first. It may be in callers.
- Look for: SQL injection, XSS, race conditions, nil pointers, resource leaks.
- Output using [P0/P1/P2] file:line → issue → fix → effort format.

### What NOT to do
- Do NOT flag style preferences as issues. Focus on correctness and safety.
- Do NOT report issues without verifying them first.
- Do NOT suggest new features — only review what exists.`,
			Tools: []string{"read_file", "search_files", "execute_command", "search_symbols"},
		},
		"executor": {
			Name: "执行者/Executor", Description: "执行命令、测试、部署 / Run commands, tests, deploy",
			Prompt: `## Role: Executor
You run tests, execute commands, and verify results.

### How to work
- Run the project's test suite first to establish a baseline.
- Execute each verification step and report the output clearly.
- If tests fail: capture the exact error messages and stack traces.
- Verify that the changes produce the expected output.
- Check for regressions: run tests that were passing before.

### What NOT to do
- Do NOT modify code. Your job is to run and verify, not to fix.
- Do NOT skip failing tests — report them.
- Do NOT run destructive commands (rm, drop, format, truncate).`,
			Tools: []string{"execute_command", "git_status", "git_diff", "read_file", "search_files"},
		},
	}
}

// Delegate 将任务委派给团队执行 — 使用图路由
// 先由 lead 角色决定分析路径，然后按图执行
func (t *Team) Delegate(ctx context.Context, userID, projectID, query string, opts kernel.QueryOptions) (*kernel.Response, error) {
	t.GenerateRoles(ctx, query)

	roleQuery := fmt.Sprintf(`从以下角色中选最适合这个任务的:
可用角色: %s

用户请求: %s

只回复角色 ID，不要其他内容。`, t.roleNames(), query)

	resp, err := t.orchestrator.ProcessQuery(ctx, "team-lead", projectID, roleQuery, kernel.QueryOptions{MaxTokens: 50})
	if err != nil {
		// fallback: use first available role
		for _, r := range t.roles {
			return t.executeGraph(ctx, query, opts, t.buildSingleGraph(r))
		}
	}

	roleName := strings.TrimSpace(resp.Content)
	role, ok := t.roles[roleName]
	if !ok {
		// fallback: pick first dynamic role
		for _, r := range t.roles {
			role = r
			break
		}
		if role == nil {
			return nil, fmt.Errorf("no roles available")
		}
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

// 通过图引擎执行，按拓扑序依次运行每个角色作为独立 sub-agent
func (t *Team) executeGraph(ctx context.Context, query string, opts kernel.QueryOptions, g *graph.Graph) (*kernel.Response, error) {
	slog.Debug("Team executeGraph start", "nodes", len(g.Nodes))
	if g == nil || len(g.Nodes) == 0 {
		return t.orchestrator.ProcessQuery(ctx, "", "", query, opts)
	}

	order, err := g.TopoSort()
	if err != nil {
		return nil, fmt.Errorf("topo sort failed: %w", err)
	}

	var previousResults []string
	var lastContent string

	for _, name := range order {
		node := g.Nodes[name]
		slog.Debug("Team executing node", "node", name, "tools", len(node.Tools))
		if node == nil {
			continue
		}

		// 找到匹配的角色名
		var roleName string
		for rn, role := range t.roles {
			if role.Name == name || rn == name {
				roleName = rn
				break
			}
		}
		if roleName == "" {
			roleName = name
		}

		content, err := t.orchestrator.RunSubAgent(ctx, "", "", roleName, query, previousResults)
		if err != nil {
			return nil, fmt.Errorf("role %s failed: %w", name, err)
		}
		previousResults = append(previousResults, fmt.Sprintf("[%s]: %s", name, content))
		lastContent = content
	}

	return &kernel.Response{Content: lastContent}, nil
}

func (t *Team) roleNames() string {
	var names []string
	for _, r := range t.roles {
		names = append(names, fmt.Sprintf("%s(%s)", r.Name, r.Description))
	}
	return strings.Join(names, ", ")
}
