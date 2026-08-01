package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ============================================================
// 模拟 OpenAIDE kernel 的流式输出
// 真实集成时替换为 kernel.ProcessStream(ctx, query) 返回的 <-chan StreamChunk
// ============================================================

// chunk 对应 kernel.StreamChunk（简化版）
type chunk struct {
	kind     string // content | thinking | tool_call | tool_done | done
	content  string
	toolName string
}

// mockProcessStream 模拟 kernel 流式响应
// 真实场景：stream, _ := agentKernel.ProcessStream(ctx, query); 然后遍历 stream
func mockProcessStream(ctx context.Context, query string) <-chan chunk {
	ch := make(chan chunk)
	go func() {
		defer close(ch)
		select {
		case <-ctx.Done():
			return
		case <-time.After(80 * time.Millisecond):
		}

		// thinking 块
		send(ctx, ch, chunk{kind: "thinking", content: "分析用户请求：需要读取文件并修复 bug..."})

		// tool_call 块
		send(ctx, ch, chunk{kind: "tool_call", toolName: "read_file", content: `{"path":"main.go"}`})
		time.Sleep(150 * time.Millisecond)
		send(ctx, ch, chunk{kind: "tool_done", toolName: "read_file", content: "读取到 120 行，发现第 45 行有空指针"})

		// 第二轮工具
		send(ctx, ch, chunk{kind: "tool_call", toolName: "diff_edit", content: `{"file":"main.go","line":45}`})
		time.Sleep(120 * time.Millisecond)
		send(ctx, ch, chunk{kind: "tool_done", toolName: "diff_edit", content: "已修复：增加 nil 检查"})

		// 最终回答（流式逐字）
		answer := "已修复 main.go 第 45 行的空指针问题。\n\n修改内容：\n```go\nif cfg != nil {\n    return cfg.Name\n}\n```\n建议补充单元测试覆盖空配置场景。"
		for _, r := range answer {
			select {
			case <-ctx.Done():
				return
			default:
				send(ctx, ch, chunk{kind: "content", content: string(r)})
				time.Sleep(8 * time.Millisecond)
			}
		}

		send(ctx, ch, chunk{kind: "done"})
	}()
	return ch
}

func send(ctx context.Context, ch chan<- chunk, c chunk) {
	select {
	case ch <- c:
	case <-ctx.Done():
	}
}

// ============================================================
// Bubbletea Model
// ============================================================

type model struct {
	viewport  viewport.Model
	textarea  textarea.Model
	spinner   spinner.Model
	width     int
	height    int
	streaming bool
	history   *strings.Builder // 指针：避免 Update 值复制 strings.Builder 触发 copyCheck panic
	cancel    context.CancelFunc
	streamCh  <-chan chunk // 当前流式 channel
}

type chunkMsg chunk

// waitForChunk 订阅 kernel 流式 channel，收到块后转成 tea.Msg
// 这是 Bubbletea 对接 channel 的标准模式
func waitForChunk(ch <-chan chunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return chunkMsg{kind: "done"}
		}
		return chunkMsg(c)
	}
}

func initialModel() model {
	ta := textarea.New()
	ta.Placeholder = "输入问题，Enter 发送，Ctrl+Enter 换行，Ctrl+C 取消/退出"
	ta.Focus()
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)
	vp.SetContent("Bubbletea REPL Demo（不捕获鼠标，Ctrl+Shift+C 复制照常）\n————————————————————————\n\n")

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return model{
		viewport: vp,
		textarea: ta,
		spinner:  sp,
		history:  &strings.Builder{},
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// 上方 viewport 占大部分高度，下方留输入框 + 状态栏
		vpHeight := msg.Height - 5
		if vpHeight < 3 {
			vpHeight = 3
		}
		m.viewport.Height = vpHeight
		m.viewport.Width = msg.Width
		m.textarea.SetWidth(msg.Width)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// OpenAIDE 风格：streaming 时第一次 Ctrl+C 取消请求，idle 时退出
			if m.streaming && m.cancel != nil {
				m.cancel()
				m.appendHistory("\n[已取消]\n")
				m.streaming = false
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			// Enter 发送，Ctrl+Enter / Shift+Enter 换行（textarea 默认行为）
			if m.streaming {
				return m, nil
			}
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			m.textarea.Reset()
			cmd := m.startStream(text)
			return m, cmd
		}

	case chunkMsg:
		// 处理 kernel 流式块
		switch msg.kind {
		case "thinking":
			m.appendHistory(styleThinking.Render("💭 "+msg.content) + "\n")
		case "tool_call":
			m.appendHistory(styleTool.Render(fmt.Sprintf("🔧 %s(%s)", msg.toolName, msg.content)) + "\n")
		case "tool_done":
			m.appendHistory(styleToolDone.Render(fmt.Sprintf("  → %s", msg.content)) + "\n")
		case "content":
			m.appendHistory(msg.content)
		case "done":
			m.appendHistory("\n\n")
			m.streaming = false
			return m, nil
		}
		m.viewport.SetContent(m.history.String())
		m.viewport.GotoBottom()
		if msg.kind != "done" {
			cmds = append(cmds, waitForChunk(m.streamCh))
		}
		return m, tea.Batch(cmds...)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.streaming {
			cmds = append(cmds, cmd)
		}
	}

	// 透传给组件
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// streamCh 保存当前流式 channel（用于 Update 中继续订阅）
// startStream 启动流式响应，返回首次订阅 cmd
// 真实集成时：m.streamCh = agentKernel.ProcessStream(ctx, query)
func (m *model) startStream(text string) tea.Cmd {
	m.appendHistory(styleUser.Render("❯ "+text) + "\n\n")
	m.streaming = true

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streamCh = mockProcessStream(ctx, text)
	return waitForChunk(m.streamCh)
}

// appendHistory 追加并刷新 viewport
func (m *model) appendHistory(s string) {
	m.history.WriteString(s)
	m.viewport.SetContent(m.history.String())
	m.viewport.GotoBottom()
}

func (m model) View() string {
	status := styleStatus.Render("● idle")
	if m.streaming {
		status = styleStreaming.Render(fmt.Sprintf("%s streaming", m.spinner.View()))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.viewport.View(),
		separator,
		status,
		m.textarea.View(),
	)
}

const separator = "——————————————————————————"

var (
	styleThinking = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	styleTool     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleToolDone = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleUser     = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true)
	styleStatus   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleStreaming = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

func main() {
	// 注意：没有 tea.WithMouseCellMotion() —— 鼠标选择/Ctrl+Shift+C 复制照常工作
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Println("运行出错:", err)
	}
}
