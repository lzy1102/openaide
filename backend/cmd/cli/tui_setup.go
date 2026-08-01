package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// setupStep 向导步骤
type setupStep int

const (
	stepLang setupStep = iota
	stepProvider
	stepAPIKey
	stepCustomURL
	stepModel
	stepFlashModel
	stepSearchConfirm
	stepSearchURL
	stepWriting
	stepDone
)

// providerOpt 提供商选项
type providerOpt struct {
	name      string
	baseURL   string
	needModel bool
}

var setupProviders = []providerOpt{
	{name: "DeepSeek", baseURL: "https://api.deepseek.com/v1"},
	{name: "OpenAI", baseURL: "https://api.openai.com/v1"},
	{name: "Anthropic", baseURL: "https://api.anthropic.com"},
	{name: "Ollama (local)", baseURL: "http://localhost:11434/v1", needModel: true},
	{name: "Custom OpenAI-compatible", baseURL: "", needModel: true},
}

type setupDoneMsg struct {
	configPath string
	promptsDir string
	testOK     bool
	err        error
}

// setupModel 配置向导模型
type setupModel struct {
	step       setupStep
	width      int
	langIdx    int
	provIdx    int
	input      textinput.Model
	searchOn   bool
	spinner    spinner.Model
	result     setupResult
	configPath string
	promptsDir string
	testOK     bool
	err        error
}

func initialSetupModel() setupModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styleLogo
	ti := textinput.New()
	ti.CharLimit = 200
	ti.Width = 60
	return setupModel{
		step:    stepLang,
		spinner: s,
		input:   ti,
		result: setupResult{
			lang:       "zh",
			provider:   setupProviders[0].name,
			baseURL:    setupProviders[0].baseURL,
			model:      "deepseek-v4-pro",
			flashModel: "deepseek-v4-flash",
		},
	}
}

func (m setupModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// configureInput 按步骤配置输入框
func (m *setupModel) configureInput(placeholder, value string, mask bool) {
	m.input = textinput.New()
	m.input.CharLimit = 200
	m.input.Width = 60
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.Focus()
	if mask {
		m.input.EchoMode = textinput.EchoPassword
	}
}

func (m setupModel) currentProvider() providerOpt {
	return setupProviders[m.provIdx]
}

// langOptions 语言选项
var setupLangOptions = []string{"中文", "English"}

func (m setupModel) View() string {
	var b strings.Builder
	b.WriteString(styleLogo.Render(" ⚙ OpenAIDE Setup Wizard") + "\n\n")

	switch m.step {
	case stepLang:
		b.WriteString(styleInfo.Render(" 选择界面语言 / Select interface language:") + "\n\n")
		for i, opt := range setupLangOptions {
			marker := "  "
			label := opt
			if i == m.langIdx {
				marker = "❯ "
				label = styleSelected.Render(opt)
			}
			b.WriteString(marker + label + "\n")
		}
	case stepProvider:
		b.WriteString(styleInfo.Render(" 选择 LLM 提供商 / Select LLM provider:") + "\n\n")
		for i, p := range setupProviders {
			marker := "  "
			label := p.name
			if p.needModel {
				label += styleDim.Render(" (需指定模型)")
			}
			if i == m.provIdx {
				marker = "❯ "
				label = styleSelected.Render(p.name)
			}
			b.WriteString(marker + label + "\n")
		}
	case stepAPIKey:
		b.WriteString(styleInfo.Render(fmt.Sprintf(" 输入 API Key（%s）:", m.result.provider)) + "\n\n")
		b.WriteString(m.input.View() + "\n")
		b.WriteString(styleDim.Render(" 提示：Ollama 本地服务可留空。回车确认。") + "\n")
	case stepCustomURL:
		b.WriteString(styleInfo.Render(" 输入 Base URL（自定义提供商）:") + "\n\n")
		b.WriteString(m.input.View() + "\n")
	case stepModel:
		b.WriteString(styleInfo.Render(" 输入主模型（reasoning / 推理）:") + "\n\n")
		b.WriteString(m.input.View() + "\n")
	case stepFlashModel:
		b.WriteString(styleInfo.Render(" 输入执行模型（execution / 快速）:") + "\n\n")
		b.WriteString(m.input.View() + "\n")
	case stepSearchConfirm:
		b.WriteString(styleInfo.Render(" 是否启用 SearXNG 搜索？") + "\n\n")
		for i, opt := range []string{"否", "是"} {
			marker := "  "
			label := opt
			on := (i == 1) == m.searchOn
			if on {
				marker = "❯ "
				label = styleSelected.Render(opt)
			}
			b.WriteString(marker + label + "\n")
		}
	case stepSearchURL:
		b.WriteString(styleInfo.Render(" 输入 SearXNG 地址:") + "\n\n")
		b.WriteString(m.input.View() + "\n")
	case stepWriting:
		b.WriteString(styleInfo.Render(" 正在生成配置…") + "\n\n")
		b.WriteString(m.spinner.View() + " 写入 config.yaml 并测试连接\n")
	case stepDone:
		b.WriteString(styleSuccess.Render(" ✓ 配置完成") + "\n\n")
		if m.err != nil {
			b.WriteString(styleError.Render(" ✗ "+m.err.Error()) + "\n")
		} else {
			b.WriteString(" 配置文件: " + m.configPath + "\n")
			b.WriteString(" 提示词目录: " + m.promptsDir + "\n")
			if m.testOK {
				b.WriteString(styleSuccess.Render(" 连接测试: 通过") + "\n")
			} else if m.result.apiKey != "" {
				b.WriteString(styleWarn.Render(" 连接测试: 失败（可稍后用 openaide 重试）") + "\n")
			}
		}
		b.WriteString(styleDim.Render(" 按任意键退出") + "\n")
	}
	b.WriteString("\n" + styleDim.Render(" ↑/↓ 选择 · Enter 确认 · Ctrl+C 取消"))
	return styleBox.Render(b.String())
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case setupDoneMsg:
		return m.onDone(msg), nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		if m.step == stepDone {
			return m, tea.Quit
		}
		if m.input.Focused() {
			switch msg.String() {
			case "enter":
				return m.advance()
			case "ctrl+c":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.moveSelection(-1)
		case "down", "j":
			m.moveSelection(1)
		case "enter":
			return m.advance()
		}
	}
	return m, nil
}

func (m *setupModel) moveSelection(dir int) {
	switch m.step {
	case stepLang:
		m.langIdx = (m.langIdx + dir + len(setupLangOptions)) % len(setupLangOptions)
	case stepProvider:
		m.provIdx = (m.provIdx + dir + len(setupProviders)) % len(setupProviders)
	case stepSearchConfirm:
		m.searchOn = !m.searchOn
	}
}

// advance 推进向导到下一步
func (m setupModel) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case stepLang:
		m.result.lang = "zh"
		if m.langIdx == 1 {
			m.result.lang = "en"
		}
		m.step = stepProvider
	case stepProvider:
		p := m.currentProvider()
		m.result.provider = p.name
		m.result.baseURL = p.baseURL
		if p.name == "Ollama (local)" {
			m.step = stepModel
		} else {
			m.step = stepAPIKey
			m.configureInput("sk-...", "", true)
		}
	case stepAPIKey:
		m.result.apiKey = strings.TrimSpace(m.input.Value())
		if m.result.baseURL == "" {
			m.step = stepCustomURL
			m.configureInput("https://api.example.com/v1", "https://api.example.com/v1", false)
		} else if m.currentProvider().needModel {
			m.step = stepModel
			m.configureInput("模型名，如 gpt-4o / qwen2.5", m.result.model, false)
		} else {
			m.step = stepSearchConfirm
		}
	case stepCustomURL:
		url := strings.TrimSpace(m.input.Value())
		if url == "" {
			url = "https://api.example.com/v1"
		}
		m.result.baseURL = url
		m.step = stepModel
		m.configureInput("模型名，如 gpt-4o / qwen2.5", m.result.model, false)
	case stepModel:
		model := strings.TrimSpace(m.input.Value())
		if model != "" {
			m.result.model = model
		}
		m.step = stepFlashModel
		m.configureInput("执行模型（可同主模型）", m.result.flashModel, false)
	case stepFlashModel:
		model := strings.TrimSpace(m.input.Value())
		if model != "" {
			m.result.flashModel = model
		}
		m.step = stepSearchConfirm
	case stepSearchConfirm:
		if m.searchOn {
			m.step = stepSearchURL
			m.configureInput("http://localhost:8888", "http://localhost:8888", false)
		} else {
			m.result.searchURL = ""
			m.step = stepWriting
			return m, m.writeAll()
		}
	case stepSearchURL:
		m.result.searchURL = strings.TrimSpace(m.input.Value())
		m.step = stepWriting
		return m, m.writeAll()
	}
	return m, nil
}

// writeAll 写入配置文件并测试连接
func (m setupModel) writeAll() tea.Cmd {
	cfg := m.result
	return func() tea.Msg {
		configPath, promptsDir, err := writeSetupFiles(cfg)
		testOK := false
		if err == nil && cfg.apiKey != "" {
			testOK = testConnection(cfg.baseURL, cfg.apiKey, cfg.model)
		}
		return setupDoneMsg{configPath: configPath, promptsDir: promptsDir, testOK: testOK, err: err}
	}
}

func (m setupModel) onDone(msg setupDoneMsg) tea.Model {
	m.configPath = msg.configPath
	m.promptsDir = msg.promptsDir
	m.testOK = msg.testOK
	m.err = msg.err
	m.step = stepDone
	return m
}

// runSetupTUI 运行配置向导（bubbletea TUI）
func runSetupTUI() {
	m := initialSetupModel()
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Printf("\n  ✗ Setup error: %v\n", err)
		return
	}
	if sm, ok := final.(setupModel); ok {
		if sm.err != nil {
			fmt.Printf("\n  ✗ Setup failed: %v\n", sm.err)
		} else if sm.configPath != "" {
			fmt.Printf("\n  ✓ 配置完成\n")
			fmt.Printf("    配置文件: %s\n", sm.configPath)
			fmt.Printf("    提示词目录: %s\n", sm.promptsDir)
			if sm.testOK {
				fmt.Printf("    连接测试: 通过 ✓\n")
			} else if sm.result.apiKey != "" {
				fmt.Printf("    连接测试: 失败（可稍后用 openaide 重试）\n")
			}
			fmt.Printf("\n    现在运行 openaide 开始使用！\n")
		} else {
			// 用户 Ctrl+C 取消
			if _, err := os.Stat(os.Getenv("HOME") + "/.openaide/config.yaml"); os.IsNotExist(err) {
				fmt.Printf("  已取消，未生成配置。\n")
			}
		}
	}
}
