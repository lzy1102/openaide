package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dashboardModel 仪表盘 bubbletea Model
type dashboardModel struct {
	dialogues  []Dialogue
	models     []Model
	apiURL     string
	version    string
	cursor     int
	quitted    bool
	action     DashboardAction
	width      int
	height     int
	err        string
	loaded     bool
	loadCmd    tea.Cmd
}

// NewDashboard 创建仪表盘
func NewDashboard(dialogues []Dialogue, models []Model, apiURL, version string) dashboardModel {
	return dashboardModel{
		dialogues: dialogues,
		models:    models,
		apiURL:    apiURL,
		version:   version,
		cursor:    0,
		loaded:    true,
	}
}

func (m dashboardModel) Init() tea.Cmd {
	return nil
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.action = DashboardAction{Action: "exit"}
			m.quitted = true
			return m, tea.Quit
		case "n":
			m.action = DashboardAction{Action: "chat"}
			m.quitted = true
			return m, tea.Quit
		case "m":
			m.action = DashboardAction{Action: "select_model"}
			m.quitted = true
			return m, tea.Quit
		case "c":
			m.action = DashboardAction{Action: "config"}
			m.quitted = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.dialogues)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.dialogues) {
				m.action = DashboardAction{
					Action:     "chat",
					DialogueID: m.dialogues[m.cursor].ID,
				}
			} else {
				m.action = DashboardAction{Action: "chat"}
			}
			m.quitted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	if m.quitted {
		return ""
	}

	// 计算面板宽度
	panelWidth := 36
	if m.width > 120 {
		panelWidth = 40
	}

	// 左栏: 对话列表
	dialoguePanel := m.renderDialoguePanel(panelWidth)

	// 中栏: 模型状态
	modelPanel := m.renderModelPanel(panelWidth)

	// 右栏: 系统信息
	infoPanel := m.renderInfoPanel(panelWidth)

	// 合并三栏
	main := lipgloss.JoinHorizontal(lipgloss.Top, dialoguePanel, modelPanel, infoPanel)

	// 底栏快捷键
	footer := m.renderFooter()

	// 组合
	return fmt.Sprintf("%s\n%s", main, footer)
}

func (m dashboardModel) renderDialoguePanel(width int) string {
	var s strings.Builder

	title := StylePanelTitle.Render("💬 Dialogues")
	s.WriteString(title)
	s.WriteString("\n")

	if len(m.dialogues) == 0 {
		s.WriteString(StyleMuted.Render("  No dialogues yet"))
		s.WriteString("\n")
	} else {
		for i, d := range m.dialogues {
			cursor := " "
			if i == m.cursor {
				cursor = StylePrimary.Render("▶")
			}
			title := d.Title
			if len(title) > 24 {
				title = title[:24] + "…"
			}
			s.WriteString(fmt.Sprintf(" %s %s\n", cursor, StyleBold.Render(title)))
			s.WriteString(StyleMuted.Render(fmt.Sprintf("   %s\n", d.ID[:12]+"…")))
		}
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted).
		Padding(1, 2).
		Width(width)

	return style.Render(s.String())
}

func (m dashboardModel) renderModelPanel(width int) string {
	var s strings.Builder

	title := StylePanelTitle.Render("🤖 Models")
	s.WriteString(title)
	s.WriteString("\n")

	if len(m.models) == 0 {
		s.WriteString(StyleMuted.Render("  No models available"))
		s.WriteString("\n")
	} else {
		// 按 provider 分组
		providers := make(map[string][]Model)
		for _, model := range m.models {
			providers[model.Provider] = append(providers[model.Provider], model)
		}

		for provider, models := range providers {
			s.WriteString(StyleAccent.Render("  " + provider))
			s.WriteString("\n")
			for _, model := range models {
				status := "●"
				if model.Status == "enabled" {
					status = StyleSuccess.Render("●")
				} else {
					status = StyleMuted.Render("●")
				}
				name := model.Name
				if len(name) > 20 {
					name = name[:20] + "…"
				}
				s.WriteString(fmt.Sprintf("    %s %s\n", status, name))
			}
		}
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted).
		Padding(1, 2).
		Width(width)

	return style.Render(s.String())
}

func (m dashboardModel) renderInfoPanel(width int) string {
	var s strings.Builder

	title := StylePanelTitle.Render("⚙ System")
	s.WriteString(title)
	s.WriteString("\n")

	s.WriteString(fmt.Sprintf("  %s %s\n", StyleMuted.Render("API:"), StyleBannerValue.Render(m.apiURL)))
	s.WriteString(fmt.Sprintf("  %s %s\n", StyleMuted.Render("Version:"), StyleBannerValue.Render(m.version)))
	s.WriteString(fmt.Sprintf("  %s %s\n", StyleMuted.Render("Dialogues:"), StyleBannerValue.Render(fmt.Sprintf("%d", len(m.dialogues)))))
	enabledCount := 0
	for _, model := range m.models {
		if model.Status == "enabled" {
			enabledCount++
		}
	}
	s.WriteString(fmt.Sprintf("  %s %s\n", StyleMuted.Render("Models:"), StyleBannerValue.Render(fmt.Sprintf("%d/%d enabled", enabledCount, len(m.models)))))

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted).
		Padding(1, 2).
		Width(width)

	return style.Render(s.String())
}

func (m dashboardModel) renderFooter() string {
	footer := lipgloss.NewStyle().
		Padding(1, 2).
		Foreground(ColorMuted)

	keys := []struct {
		key  string
		desc string
	}{
		{"Enter", "Open"},
		{"n", "New chat"},
		{"m", "Model"},
		{"c", "Config"},
		{"q", "Exit"},
	}

	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %s",
			StyleHelpKey.Render("["+k.key+"]"),
			StyleHelpDesc.Render(k.desc)))
	}

	// 居中
	sep := StyleMuted.Render("  │  ")
	line := strings.Join(parts, sep)
	return footer.Render(line)
}

// RunDashboard 运行仪表盘（独立 tea.Program）
func RunDashboard(apiURL, version string) DashboardAction {
	// 获取数据
	dialogues, _ := FetchDialogues(apiURL, "cli-user")
	models, _ := FetchModels(apiURL)

	if version == "" {
		version = "dev"
	}

	m := NewDashboard(dialogues, models, apiURL, version)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Printf("%s Error: %v\n", StyleError.Render("Error:"), err)
		return DashboardAction{Action: "exit"}
	}

	if final, ok := result.(dashboardModel); ok {
		return final.action
	}

	return DashboardAction{Action: "exit"}
}

// init functions to ensure time import is used
var _ = time.Now
