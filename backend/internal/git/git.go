package git

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Status Git 仓库状态
type Status struct {
	Branch        string       `json:"branch"`
	Ahead         int          `json:"ahead"`
	Behind        int          `json:"behind"`
	Staged        []FileChange `json:"staged"`
	Unstaged      []FileChange `json:"unstaged"`
	Untracked     []string     `json:"untracked"`
	Conflicts     []string     `json:"conflicts"`
	IsClean       bool         `json:"is_clean"`
}

// FileChange 文件变更
type FileChange struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	Status  string `json:"status"` // A=Added, M=Modified, D=Deleted, R=Renamed, C=Copied, U=Updated
	Score   int    `json:"score,omitempty"`
}

// Diff 文件差异
type Diff struct {
	Path        string   `json:"path"`
	OldPath     string   `json:"old_path,omitempty"`
	Content     string   `json:"content"`
	Additions   int      `json:"additions"`
	Deletions   int      `json:"deletions"`
	IsBinary    bool     `json:"is_binary"`
	IsNewFile   bool     `json:"is_new_file"`
	IsDeleted   bool     `json:"is_deleted"`
	Hunks       []Hunk   `json:"hunks,omitempty"`
}

// Hunk 差异块
type Hunk struct {
	OldStart int    `json:"old_start"`
	OldLines int    `json:"old_lines"`
	NewStart int    `json:"new_start"`
	NewLines int    `json:"new_lines"`
	Lines    []Line `json:"lines"`
}

// Line 差异行
type Line struct {
	Type string `json:"type"` // " "=context, "+"=addition, "-"=deletion
	Text string `json:"text"`
}

// Client Git 客户端
type Client struct {
	workDir string
}

// NewClient 创建 Git 客户端
func NewClient(workDir string) *Client {
	return &Client{workDir: workDir}
}

// Status 获取仓库状态
func (c *Client) Status() (*Status, error) {
	status := &Status{
		Staged:    make([]FileChange, 0),
		Unstaged:  make([]FileChange, 0),
		Untracked: make([]string, 0),
		Conflicts: make([]string, 0),
	}

	// 获取分支信息
	branch, ahead, behind, err := c.getBranchInfo()
	if err != nil {
		return nil, err
	}
	status.Branch = branch
	status.Ahead = ahead
	status.Behind = behind

	// 获取文件状态
	output, err := c.exec("status", "--porcelain")
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		path := line[3:]

		// 解析重命名/复制
		var oldPath string
		if indexStatus == 'R' || indexStatus == 'C' || workTreeStatus == 'R' || workTreeStatus == 'C' {
			parts := strings.SplitN(path, " -> ", 2)
			if len(parts) == 2 {
				oldPath = parts[0]
				path = parts[1]
			}
		}

		// 冲突检测
		if indexStatus == 'U' || workTreeStatus == 'U' ||
			(indexStatus == 'D' && workTreeStatus == 'D') ||
			(indexStatus == 'A' && workTreeStatus == 'A') {
			status.Conflicts = append(status.Conflicts, path)
			continue
		}

		// 暂存区状态
		if indexStatus != ' ' && indexStatus != '?' {
			status.Staged = append(status.Staged, FileChange{
				Path:   path,
				Status: string(indexStatus),
				OldPath: oldPath,
			})
		}

		// 工作区状态
		if workTreeStatus != ' ' {
			if workTreeStatus == '?' {
				status.Untracked = append(status.Untracked, path)
			} else {
				status.Unstaged = append(status.Unstaged, FileChange{
					Path:   path,
					Status: string(workTreeStatus),
					OldPath: oldPath,
				})
			}
		}
	}

	status.IsClean = len(status.Staged) == 0 && len(status.Unstaged) == 0 &&
		len(status.Untracked) == 0 && len(status.Conflicts) == 0

	return status, nil
}

// DiffStaged 获取暂存区差异
func (c *Client) DiffStaged() ([]Diff, error) {
	return c.getDiff("--cached")
}

// DiffUnstaged 获取工作区差异
func (c *Client) DiffUnstaged() ([]Diff, error) {
	return c.getDiff()
}

// DiffFile 获取单个文件差异
func (c *Client) DiffFile(path string, staged bool) (*Diff, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", path)

	output, err := c.exec(args...)
	if err != nil {
		return nil, err
	}

	diffs, err := c.parseDiff(output)
	if err != nil {
		return nil, err
	}

	if len(diffs) > 0 {
		return &diffs[0], nil
	}
	return nil, fmt.Errorf("no diff for file: %s", path)
}

// Commit 提交变更
func (c *Client) Commit(message string) error {
	_, err := c.exec("commit", "-m", message)
	return err
}

// Add 添加文件到暂存区
func (c *Client) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := c.exec(args...)
	return err
}

// Log 获取提交历史
func (c *Client) Log(limit int) ([]Commit, error) {
	format := "%H|%an|%ae|%at|%s"
	output, err := c.exec("log", fmt.Sprintf("-%d", limit), fmt.Sprintf("--pretty=format:%s", format))
	if err != nil {
		return nil, err
	}

	var commits []Commit
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    parts[0],
			Author:  parts[1],
			Email:   parts[2],
			Date:    parts[3],
			Message: parts[4],
		})
	}

	return commits, nil
}

// BranchList 获取分支列表
func (c *Client) BranchList() ([]Branch, error) {
	output, err := c.exec("branch", "-a", "--format=%(refname:short)|%(HEAD)")
	if err != nil {
		return nil, err
	}

	var branches []Branch
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		branches = append(branches, Branch{
			Name:    parts[0],
			Current: parts[1] == "*",
		})
	}

	return branches, nil
}

// IsRepo 检查是否为 Git 仓库
func (c *Client) IsRepo() bool {
	_, err := c.exec("rev-parse", "--git-dir")
	return err == nil
}

// Root 获取仓库根目录
func (c *Client) Root() (string, error) {
	return c.exec("rev-parse", "--show-toplevel")
}

// ============ 内部方法 ============

func (c *Client) getBranchInfo() (string, int, int, error) {
	// 获取当前分支
	branch, err := c.exec("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", 0, 0, err
	}

	// 获取 ahead/behind
	upstream, err := c.exec("rev-parse", "--abbrev-ref", "@{upstream}")
	if err != nil {
		// 没有上游分支
		return branch, 0, 0, nil
	}

	output, err := c.exec("rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", branch, upstream))
	if err != nil {
		return branch, 0, 0, nil
	}

	var ahead, behind int
	fmt.Sscanf(output, "%d\t%d", &ahead, &behind)

	return branch, ahead, behind, nil
}

func (c *Client) getDiff(extraArgs ...string) ([]Diff, error) {
	args := append([]string{"diff"}, extraArgs...)
	output, err := c.exec(args...)
	if err != nil {
		return nil, err
	}

	return c.parseDiff(output)
}

func (c *Client) parseDiff(output string) ([]Diff, error) {
	if output == "" {
		return []Diff{}, nil
	}

	var diffs []Diff
	var currentDiff *Diff
	var currentHunk *Hunk

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		// 新文件开始
		if strings.HasPrefix(line, "diff --git ") {
			if currentDiff != nil {
				diffs = append(diffs, *currentDiff)
			}
			currentDiff = &Diff{Hunks: make([]Hunk, 0)}
			currentHunk = nil
			continue
		}

		if currentDiff == nil {
			continue
		}

		// 旧文件路径
		if strings.HasPrefix(line, "--- a/") {
			currentDiff.OldPath = strings.TrimPrefix(line, "--- a/")
			continue
		}
		if strings.HasPrefix(line, "--- /dev/null") {
			currentDiff.IsNewFile = true
			continue
		}

		// 新文件路径
		if strings.HasPrefix(line, "+++ b/") {
			currentDiff.Path = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if strings.HasPrefix(line, "+++ /dev/null") {
			currentDiff.IsDeleted = true
			continue
		}

		// 二进制文件
		if strings.Contains(line, "Binary files") {
			currentDiff.IsBinary = true
			continue
		}

		// Hunk 头
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
			}
			var oldStart, oldLines, newStart, newLines int
			fmt.Sscanf(line, "@@ -%d,%d +%d,%d @@", &oldStart, &oldLines, &newStart, &newLines)
			currentHunk = &Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Lines:    make([]Line, 0),
			}
			continue
		}

		// 差异行
		if currentHunk != nil && len(line) > 0 {
			lineType := " "
			if line[0] == '+' {
				lineType = "+"
				currentDiff.Additions++
			} else if line[0] == '-' {
				lineType = "-"
				currentDiff.Deletions++
			}
			currentHunk.Lines = append(currentHunk.Lines, Line{
				Type: lineType,
				Text: line[1:],
			})
		}
	}

	if currentHunk != nil && currentDiff != nil {
		currentDiff.Hunks = append(currentDiff.Hunks, *currentHunk)
	}
	if currentDiff != nil {
		diffs = append(diffs, *currentDiff)
	}

	return diffs, nil
}

func (c *Client) exec(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if c.workDir != "" {
		cmd.Dir = c.workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %v (stderr: %s)",
			strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Commit 提交信息
type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

// Branch 分支信息
type Branch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// SuggestCommitMessage 生成提交信息建议
func SuggestCommitMessage(status *Status, diffs []Diff) string {
	var parts []string

	// 根据变更类型确定前缀
	types := make(map[string]int)
	for _, change := range status.Staged {
		switch change.Status {
		case "A":
			types["feat"]++
		case "M":
			types["fix"]++
		case "D":
			types["remove"]++
		case "R":
			types["refactor"]++
		}
	}

	// 选择最常见的类型
	maxCount := 0
	prefix := "chore"
	for t, count := range types {
		if count > maxCount {
			maxCount = count
			prefix = t
		}
	}

	// 根据变更文件生成描述
	if len(status.Staged) == 1 {
		parts = append(parts, fmt.Sprintf("%s: update %s", prefix, filepath.Base(status.Staged[0].Path)))
	} else if len(status.Staged) <= 3 {
		var names []string
		for _, change := range status.Staged {
			names = append(names, filepath.Base(change.Path))
		}
		parts = append(parts, fmt.Sprintf("%s: update %s", prefix, strings.Join(names, ", ")))
	} else {
		parts = append(parts, fmt.Sprintf("%s: update %d files", prefix, len(status.Staged)))
	}

	return strings.Join(parts, "\n")
}
