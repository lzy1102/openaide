package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

// ============ Styles ============

var (
	sUser  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C8EBF")).Bold(true).PaddingLeft(1)
	sAI    = lipgloss.NewStyle().Foreground(lipgloss.Color("#82B74B")).Bold(true).PaddingLeft(1)
	sThink = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Italic(true).PaddingLeft(3)
	sTool  = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A859"))
	sErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	sInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	sBar   = lipgloss.NewStyle().Background(lipgloss.Color("#1a1a2e")).Foreground(lipgloss.Color("#AAAAAA")).Padding(0, 1)
	sInput = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#444444")).Padding(0, 1)
)

// ============ Messages ============

type streamChunk struct {
	content, thinking string
	done              bool
	tokens, toolCnt   int
	err               error
}

// ============ Model ============

type model struct {
	app       *infra.Application
	program   *tea.Program
	messages  []message
	input     string
	streaming bool
	think     string
	ai        string
	tokens    int
	tools     int
	err       error
	width     int
}

type message struct{ role, content string }

// ============ Entry ============

func main() {
	args := os.Args[1:]

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "update", "upgrade":
			cmdUpdate(args[1:])
			return
		case "version", "-v", "--version":
			fmt.Println("OpenAIDE CLI  dev")
			fmt.Println("用法: openaide [update|version|help]")
			return
		case "help", "-h", "--help":
			fmt.Println("OpenAIDE CLI")
			fmt.Println("  openaide          启动交互对话 (默认)")
			fmt.Println("  openaide update    更新到最新版本")
			fmt.Println("  openaide version   显示版本信息")
			fmt.Println("  openaide help      显示此帮助")
			fmt.Println()
			fmt.Println("聊天内快捷键:")
			fmt.Println("  Ctrl+C  退出")
			fmt.Println("  /clear  清除上下文")
			return
		}
	}

	// 加载配置 + 启动
	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	m := &model{app: app, width: 100}
	p := tea.NewProgram(m,
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	) // 显式绑定stdin/stdout，修复SSH下无法输入的问题
	m.program = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ============ Update ============

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.streaming {
			if msg.String() == "ctrl+c" {
				m.streaming = false
				if m.ai != "" {
					m.messages = append(m.messages, message{role: "assistant", content: formatThink(m.think) + m.ai})
				}
				m.ai = ""
				m.think = ""
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			return m, tea.Quit
		case "enter":
			if m.input == "" {
				return m, nil
			}
			if m.input == "/clear" {
				m.messages = nil
				m.input = ""
				m.tokens = 0
				m.tools = 0
				return m, nil
			}
			query := m.input
			m.input = ""
			m.messages = append(m.messages, message{role: "user", content: query})
			m.streaming = true
			m.ai = ""
			m.think = ""
			m.err = nil
			go doStream(m.program, m.app, query)
			return m, nil

		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
		default:
			s := msg.String()
			if len(s) == 1 && s[0] >= 32 {
				m.input += s
			}
		}

	case streamChunk:
		if msg.err != nil {
			m.err = msg.err
			m.streaming = false
			return m, nil
		}
		if msg.done {
			m.streaming = false
			m.tokens = msg.tokens
			m.tools = msg.toolCnt
			if m.ai != "" {
				m.messages = append(m.messages, message{role: "assistant", content: formatThink(m.think) + m.ai})
			}
			m.ai = ""
			m.think = ""
			return m, nil
		}
		if msg.content != "" {
			m.ai += msg.content
		}
		if msg.thinking != "" {
			m.think += msg.thinking
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
	}

	return m, nil
}

// ============ View ============

func (m *model) View() string {
	if m.width == 0 {
		m.width = 100
	}
	var sb strings.Builder

	// Messages
	var rendered []string
	if m.streaming {
		if m.think != "" {
			rendered = append(rendered, sThink.Render("... "+trunc(m.think, 200)))
		}
		rendered = append(rendered, sAI.Render(m.ai))
	}
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.role == "user" {
			rendered = append(rendered, sUser.Render("▸ "+msg.content))
		} else {
			rendered = append(rendered, sAI.Render(msg.content))
		}
	}
	for i := len(rendered) - 1; i >= 0; i-- {
		sb.WriteString(rendered[i] + "\n")
	}

	// Status
	status := ""
	if m.tools > 0 {
		status += sTool.Render(fmt.Sprintf("🔧%d ", m.tools))
	}
	if m.tokens > 0 {
		status += sInfo.Render(fmt.Sprintf("⚡%d ", m.tokens))
	}
	if m.streaming {
		status += sInfo.Render("● ")
	}
	if status != "" {
		sb.WriteString(sBar.Render(status) + "\n")
	}
	if m.err != nil {
		sb.WriteString(sErr.Render("✗ "+m.err.Error()) + "\n")
	}

	// Input
	prompt := "> "
	if m.streaming {
		prompt = "⏳ "
	}
	cursor := ""
	if !m.streaming {
		cursor = "│"
	}
	sb.WriteString(sInput.Render(prompt + m.input + cursor))

	return sb.String()
}

// ============ Streaming ============

func doStream(p *tea.Program, app *infra.Application, query string) {
	ctx := context.Background()
	stream, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", query, kernel.QueryOptions{})
	if err != nil {
		p.Send(streamChunk{err: err, done: true})
		return
	}

	totalTools := 0
	totalTokens := 0

	for chunk := range stream {
		if chunk.Error != nil {
			p.Send(streamChunk{err: chunk.Error, done: true})
			return
		}
		if chunk.Done {
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}
			p.Send(streamChunk{done: true, tokens: totalTokens, toolCnt: totalTools})
			return
		}
		if len(chunk.ToolCalls) > 0 {
			totalTools += len(chunk.ToolCalls)
		}
		p.Send(streamChunk{content: chunk.Content, thinking: chunk.ReasoningContent})
		time.Sleep(5 * time.Millisecond)
	}
}

// ============ Helpers ============

func formatThink(think string) string {
	if think == "" {
		return ""
	}
	return sThink.Render("[思考] "+think) + "\n"
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}

// ============ Update ============

// processMultimodal 处理多模态输入：检测图片文件路径、base64粘贴、剪贴板
func processMultimodal(input string) string {
	// 1. 检测文件路径（.png .jpg .gif .webp .bmp）
	lower := strings.ToLower(input)
	exts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp"}
	for _, ext := range exts {
		if strings.Contains(lower, ext) {
			// 提取路径部分
			fields := strings.Fields(input)
			var paths []string
			var textParts []string
			for _, f := range fields {
				lf := strings.ToLower(f)
				isPath := false
				for _, e := range exts {
					if strings.HasSuffix(lf, e) || (strings.Contains(lf, e) && strings.Contains(f, "/")) {
						isPath = true
						break
					}
				}
				if isPath {
					paths = append(paths, f)
				} else {
					textParts = append(textParts, f)
				}
			}
			if len(paths) > 0 {
				// 读取图片并转base64
				var images []string
				for _, p := range paths {
					data, err := os.ReadFile(p)
					if err == nil && len(data) < 10*1024*1024 {
						b64 := base64Encode(data)
						ext := strings.ToLower(p[strings.LastIndex(p, ".")+1:])
						if ext == "jpg" { ext = "jpeg" }
						images = append(images, fmt.Sprintf("data:image/%s;base64,%s", ext, b64))
					}
				}
				if len(images) > 0 {
					return strings.Join(textParts, " ") + "\n" + strings.Join(images, "\n")
				}
			}
			break
		}
	}

	// 2. 检测 base64 粘贴 (data:image/...;base64,...)
	if strings.Contains(input, "data:image/") && strings.Contains(input, ";base64,") {
		return input // already formatted
	}

	return input
}

func base64Encode(data []byte) string {
	// Use encoding/base64 which is already imported
	return encode(data)
}

func encode(data []byte) string {
	const b64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte
	for i := 0; i < len(data); i += 3 {
		b := []byte{data[i], 0, 0}
		if i+1 < len(data) { b[1] = data[i+1] }
		if i+2 < len(data) { b[2] = data[i+2] }
		result = append(result,
			b64[b[0]>>2],
			b64[((b[0]&3)<<4)|(b[1]>>4)],
		)
		if i+1 < len(data) {
			result = append(result, b64[((b[1]&15)<<2)|(b[2]>>6)])
		} else {
			result = append(result, '=')
			result = append(result, '=')
			break
		}
		if i+2 < len(data) {
			result = append(result, b64[b[2]&63])
		} else {
			result = append(result, '=')
			break
		}
	}
	return string(result)
}

func cmdUpdate(args []string) {
	fmt.Println("▶ OpenAIDE 更新")
	installDir := os.Getenv("HOME") + "/.openaide"
	script := filepath.Join(installDir, "scripts", "update.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		script = filepath.Join(installDir, "install.sh")
		if _, err := os.Stat(script); os.IsNotExist(err) {
			fmt.Println("错误: 未找到更新脚本")
			os.Exit(1)
		}
	}

	cmdArgs := []string{script}
	for _, arg := range args {
		if arg == "--local" || arg == "-l" {
			cmdArgs = append(cmdArgs, "--local")
		}
	}

	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("\n更新失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\n✓ 更新完成!")
}
