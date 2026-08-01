package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/config"
)

type onboardStep int

const (
	obLang onboardStep = iota
	obFocus
	obCustom
	obStyle
	obDone
)

type onboardWriteMsg struct {
	err error
}

// onboardModel 首次引导模型
type onboardModel struct {
	step       onboardStep
	cfg        *config.Config
	promptsDir string
	zh         bool
	langIdx    int
	focusIdx   int
	styleIdx   int
	input      textinput.Model
	customText string
	identity   string
	style      string
	err        error
}

var onboardFocusOptions = []string{"gp", "prog", "write", "research", "teach", "biz", "custom"}
var onboardStyleOptions = []string{"concise", "detailed", "balanced"}

func initialOnboardModel(cfg *config.Config, promptsDir string) onboardModel {
	zh := cfg.Lang == "zh"
	step := obFocus
	if cfg.Lang == "" {
		step = obLang
		zh = true
	}
	ti := textinput.New()
	ti.CharLimit = 300
	ti.Width = 60
	return onboardModel{
		step:       step,
		cfg:        cfg,
		promptsDir: promptsDir,
		zh:         zh,
		input:      ti,
	}
}

func (m onboardModel) Init() tea.Cmd { return nil }

func (m onboardModel) texts() *onboardText {
	if m.zh {
		return &zhText
	}
	return &enText
}

func (m onboardModel) focusLabel(idx int) string {
	t := m.texts()
	key := onboardFocusOptions[idx]
	switch key {
	case "gp":
		return t.gp
	case "prog":
		return t.prog
	case "write":
		return t.write
	case "research":
		return t.research
	case "teach":
		return t.teach
	case "biz":
		return t.biz
	default:
		return t.custom
	}
}

func (m onboardModel) styleLabel(idx int) string {
	t := m.texts()
	key := onboardStyleOptions[idx]
	switch key {
	case "concise":
		return t.concise
	case "detailed":
		return t.detailed
	default:
		return t.balanced
	}
}

func (m onboardModel) View() string {
	var b strings.Builder
	t := m.texts()

	switch m.step {
	case obLang:
		b.WriteString(styleLogo.Render(" OpenAIDE ") + "\n\n")
		b.WriteString("  " + t.welcome + "\n\n")
		b.WriteString("  Language / 语言\n\n")
		for i, opt := range []string{"中文", "English"} {
			marker := "  "
			label := opt
			if i == m.langIdx {
				marker = "❯ "
				label = styleSelected.Render(opt)
			}
			b.WriteString(marker + label + "\n")
		}
	case obFocus:
		b.WriteString(styleLogo.Render(" OpenAIDE ") + "\n\n")
		b.WriteString("  " + t.profile + "\n\n")
		b.WriteString("  " + t.focus + "\n")
		for i := range onboardFocusOptions {
			marker := "  "
			label := m.focusLabel(i)
			if i == m.focusIdx {
				marker = "❯ "
				label = styleSelected.Render(label)
			}
			b.WriteString(marker + label + "\n")
		}
	case obCustom:
		b.WriteString("  " + t.focus + "\n\n")
		if m.zh {
			b.WriteString(styleInfo.Render("  用几句话描述你理想的助手：") + "\n\n")
		} else {
			b.WriteString(styleInfo.Render("  Describe your ideal assistant in a few sentences:") + "\n\n")
		}
		b.WriteString(m.input.View() + "\n")
	case obStyle:
		b.WriteString("  " + t.focus + "\n")
		b.WriteString(styleDim.Render("  → "+m.focusLabel(m.focusIdx)) + "\n\n")
		b.WriteString("  " + t.respStyle + "\n")
		for i := range onboardStyleOptions {
			marker := "  "
			label := m.styleLabel(i)
			if i == m.styleIdx {
				marker = "❯ "
				label = styleSelected.Render(label)
			}
			b.WriteString(marker + label + "\n")
		}
	case obDone:
		b.WriteString(styleLogo.Render(" OpenAIDE ") + "\n\n")
		if m.err != nil {
			b.WriteString(styleError.Render("  ✗ Failed to save: "+m.err.Error()) + "\n")
			b.WriteString(styleDim.Render("  Default profile will be used.") + "\n")
		} else {
			b.WriteString(styleSuccess.Render("  "+fmt.Sprintf(t.saved, m.promptsDir)) + "\n")
			b.WriteString("  " + t.editHint + "\n")
		}
		b.WriteString("\n" + styleDim.Render("  按任意键退出 / Press any key to exit"))
	}

	b.WriteString("\n\n" + styleDim.Render(" ↑/↓ 选择 · Enter 确认 · Ctrl+C 跳过"))
	return styleBox.Render(b.String())
}

func (m onboardModel) advance() (tea.Model, tea.Cmd) {
	switch m.step {
	case obLang:
		if m.langIdx == 1 {
			m.zh = false
		} else {
			m.zh = true
		}
		m.cfg.Lang = "zh"
		if !m.zh {
			m.cfg.Lang = "en"
		}
		m.cfg.Save(defaultConfigPath())
		m.step = obFocus
	case obFocus:
		if onboardFocusOptions[m.focusIdx] == "custom" {
			m.step = obCustom
			m.input.Focus()
		} else {
			m.step = obStyle
		}
	case obCustom:
		m.customText = strings.TrimSpace(m.input.Value())
		m.step = obStyle
	case obStyle:
		m.style = onboardStyleOptions[m.styleIdx]
		m.identity = buildIdentity(fmt.Sprintf("%d", m.focusIdx+1), m.customText, m.zh)
		m.step = obDone
		return m, func() tea.Msg {
			err := writeUserTemplates(m.promptsDir+"/user", m.identity, m.style, m.zh)
			return onboardWriteMsg{err: err}
		}
	}
	return m, nil
}

func (m onboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case onboardWriteMsg:
		m.err = msg.err
		m.step = obDone
		return m, nil
	case tea.WindowSizeMsg:
		return m, nil
	case tea.KeyMsg:
		if m.step == obDone {
			return m, tea.Quit
		}
		if m.step == obCustom && m.input.Focused() {
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
			if m.step == obLang {
				m.langIdx = (m.langIdx + 1) % 2
			} else if m.step == obFocus {
				m.focusIdx = (m.focusIdx + len(onboardFocusOptions) - 1) % len(onboardFocusOptions)
			} else if m.step == obStyle {
				m.styleIdx = (m.styleIdx + len(onboardStyleOptions) - 1) % len(onboardStyleOptions)
			}
		case "down", "j":
			if m.step == obLang {
				m.langIdx = (m.langIdx + 1) % 2
			} else if m.step == obFocus {
				m.focusIdx = (m.focusIdx + 1) % len(onboardFocusOptions)
			} else if m.step == obStyle {
				m.styleIdx = (m.styleIdx + 1) % len(onboardStyleOptions)
			}
		case "enter":
			return m.advance()
		}
	}
	return m, nil
}

// runOnboardingTUI 运行首次引导（bubbletea TUI）
func runOnboardingTUI(cfg *config.Config, promptsDir string) {
	m := initialOnboardModel(cfg, promptsDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("\n  ✗ Onboarding error: %v\n", err)
	}
}
