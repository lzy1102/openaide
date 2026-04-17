package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type settingsStep int

const (
	stepAPIURL settingsStep = iota
	stepTimeout
	stepDefaultModel
	stepStreamToggle
	stepContextLimit
	stepConfirm
)

func (s settingsStep) String() string {
	switch s {
	case stepAPIURL:
		return "API Base URL"
	case stepTimeout:
		return "Timeout (seconds)"
	case stepDefaultModel:
		return "Default Model"
	case stepStreamToggle:
		return "Streaming"
	case stepContextLimit:
		return "Context Limit"
	case stepConfirm:
		return "Confirm"
	}
	return ""
}

// settingsModel 设置向导 bubbletea Model
type settingsModel struct {
	step       settingsStep
	config     Config
	configPath string
	apiURL     string
	models     []Model

	inputs  map[settingsStep]*textinput.Model
	streamOn bool

	focused bool
	err     string
	quitted bool
	saved   bool
}

// NewSettings 创建设置向导
func NewSettings(cfg *Config, configPath, apiURL string) settingsModel {
	m := settingsModel{
		step:       stepAPIURL,
		config:     *cfg,
		configPath: configPath,
		apiURL:     apiURL,
		streamOn:   cfg.Chat.Stream,
		focused:    true,
		inputs:     make(map[settingsStep]*textinput.Model),
	}

	// 初始化各步骤的 textinput
	steps := []settingsStep{stepAPIURL, stepTimeout, stepDefaultModel, stepContextLimit}
	values := map[settingsStep]string{
		stepAPIURL:       cfg.API.BaseURL,
		stepTimeout:      strconv.Itoa(cfg.API.TimeoutSec),
		stepDefaultModel: cfg.Chat.DefaultModel,
		stepContextLimit: strconv.Itoa(cfg.Chat.ContextLimit),
	}
	placeholders := map[settingsStep]string{
		stepAPIURL:       "http://localhost:19375/api",
		stepTimeout:      "30",
		stepDefaultModel: "auto",
		stepContextLimit: "10",
	}

	for _, step := range steps {
		t := textinput.New()
		t.SetValue(values[step])
		t.Placeholder = placeholders[step]
		t.CharLimit = 256
		t.Width = 50
		m.inputs[step] = &t
	}

	// 聚焦第一个输入
	m.inputs[stepAPIURL].Focus()

	return m
}

func (m settingsModel) Init() tea.Cmd {
	return nil
}

func (m settingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitted = true
			return m, tea.Quit
		}

		switch m.step {
		case stepAPIURL, stepTimeout, stepDefaultModel, stepContextLimit:
			return m.updateInputStep(msg)
		case stepStreamToggle:
			return m.updateStreamStep(msg)
		case stepConfirm:
			return m.updateConfirmStep(msg)
		}
	}

	return m, nil
}

func (m settingsModel) updateInputStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	currentStep := m.step

	*m.inputs[currentStep], cmd = m.inputs[currentStep].Update(msg)

	switch msg.String() {
	case "enter":
		// 验证输入
		switch currentStep {
		case stepTimeout:
			if _, err := strconv.Atoi(m.inputs[currentStep].Value()); err != nil {
				m.err = "Please enter a valid number"
				return m, nil
			}
		case stepContextLimit:
			if _, err := strconv.Atoi(m.inputs[currentStep].Value()); err != nil {
				m.err = "Please enter a valid number"
				return m, nil
			}
		}

		m.err = ""
		m.inputs[currentStep].Blur()

		// 移到下一步
		nextStep := currentStep + 1
		if nextStep <= stepConfirm {
			m.step = nextStep
			if nextStep != stepStreamToggle && nextStep != stepConfirm {
				m.inputs[nextStep].Focus()
			}
		}
	case "tab":
		// 同 enter
		m.inputs[currentStep].Blur()
		nextStep := currentStep + 1
		if nextStep <= stepConfirm {
			m.step = nextStep
			if nextStep != stepStreamToggle && nextStep != stepConfirm {
				m.inputs[nextStep].Focus()
			}
		}
	}

	return m, cmd
}

func (m settingsModel) updateStreamStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		m.streamOn = true
		m.step = stepContextLimit
		m.inputs[stepContextLimit].Focus()
	case "n":
		m.streamOn = false
		m.step = stepContextLimit
		m.inputs[stepContextLimit].Focus()
	}
	return m, nil
}

func (m settingsModel) updateConfirmStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		// 保存配置
		m.config.API.BaseURL = m.inputs[stepAPIURL].Value()
		if t, err := strconv.Atoi(m.inputs[stepTimeout].Value()); err == nil {
			m.config.API.TimeoutSec = t
		}
		m.config.Chat.DefaultModel = m.inputs[stepDefaultModel].Value()
		m.config.Chat.Stream = m.streamOn
		if c, err := strconv.Atoi(m.inputs[stepContextLimit].Value()); err == nil {
			m.config.Chat.ContextLimit = c
		}

		if err := SaveConfig(m.configPath, &m.config); err != nil {
			m.err = "Failed to save: " + err.Error()
			return m, nil
		}

		m.saved = true
		m.quitted = true
		return m, tea.Quit

	case "n", "esc", "q":
		m.quitted = true
		return m, tea.Quit

	case "r":
		// 重来
		m.step = stepAPIURL
		m.err = ""
		m.inputs[stepAPIURL].Focus()
	}

	return m, nil
}

func (m settingsModel) View() string {
	if m.quitted {
		return ""
	}

	var s strings.Builder

	// 标题
	s.WriteString(StyleTitle.Render("  OpenAIDE Settings Wizard"))
	s.WriteString("\n\n")

	// 步骤指示器
	steps := []settingsStep{stepAPIURL, stepTimeout, stepDefaultModel, stepStreamToggle, stepContextLimit, stepConfirm}
	for i, step := range steps {
		label := step.String()
		if step == m.step {
			s.WriteString(lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fmt.Sprintf("  [%d] %s", i+1, label)))
		} else if int(step) < int(m.step) {
			s.WriteString(StyleSuccess.Render(fmt.Sprintf("  [%d] %s ✓", i+1, label)))
		} else {
			s.WriteString(StyleMuted.Render(fmt.Sprintf("  [%d] %s", i+1, label)))
		}
		s.WriteString("\n")
	}
	s.WriteString("\n")

	// 当前步骤内容
	s.WriteString(lipgloss.NewStyle().Foreground(ColorMuted).Render("─" + strings.Repeat("─", 50)))
	s.WriteString("\n\n")

	switch m.step {
	case stepAPIURL, stepTimeout, stepDefaultModel, stepContextLimit:
		s.WriteString(StyleBold.Render("  " + m.step.String()))
		s.WriteString("\n\n")
		s.WriteString("  " + m.inputs[m.step].View())
		s.WriteString("\n\n")
		s.WriteString(StyleMuted.Render("  Tab to accept • Enter to continue • Esc to cancel"))

	case stepStreamToggle:
		s.WriteString(StyleBold.Render("  Enable streaming responses?"))
		s.WriteString("\n\n")
		s.WriteString("  [y] Yes   [n] No")
		if m.streamOn {
			s.WriteString("     " + StyleSuccess.Render("(currently: on)"))
		} else {
			s.WriteString("     " + StyleMuted.Render("(currently: off)"))
		}

	case stepConfirm:
		s.WriteString(StyleBold.Render("  Configuration Summary"))
		s.WriteString("\n\n")
		s.WriteString(fmt.Sprintf("    API URL:       %s\n", StyleBannerValue.Render(m.inputs[stepAPIURL].Value())))
		s.WriteString(fmt.Sprintf("    Timeout:       %s seconds\n", StyleBannerValue.Render(m.inputs[stepTimeout].Value())))
		s.WriteString(fmt.Sprintf("    Default Model: %s\n", StyleBannerValue.Render(m.inputs[stepDefaultModel].Value())))
		streamStr := "off"
		if m.streamOn {
			streamStr = "on"
		}
		s.WriteString(fmt.Sprintf("    Streaming:     %s\n", StyleBannerValue.Render(streamStr)))
		s.WriteString(fmt.Sprintf("    Context Limit: %s\n", StyleBannerValue.Render(m.inputs[stepContextLimit].Value())))
		s.WriteString(fmt.Sprintf("    Config Path:   %s\n", StyleBannerValue.Render(m.configPath)))
		s.WriteString("\n")
		s.WriteString(StyleMuted.Render("  [y] Save  •  [n] Cancel  •  [r] Restart"))
	}

	if m.err != "" {
		s.WriteString("\n\n")
		s.WriteString(StyleError.Render("  " + m.err))
	}

	s.WriteString("\n")
	return s.String()
}

// RunSettings 运行设置向导（独立 tea.Program）
func RunSettings(cfg *Config, configPath string) SettingsResult {
	// 如果没有 configPath，使用默认路径
	if configPath == "" {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			configPath = homeDir + "/.openaide/config.yaml"
		}
	}

	apiURL := ""
	if cfg != nil {
		apiURL = cfg.API.BaseURL
	}

	m := NewSettings(cfg, configPath, apiURL)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Printf("%s Error: %v\n", StyleError.Render("Error:"), err)
		return SettingsResult{}
	}

	if final, ok := result.(settingsModel); ok {
		return SettingsResult{
			Saved:  final.saved,
			Config: &final.config,
		}
	}

	return SettingsResult{}
}
