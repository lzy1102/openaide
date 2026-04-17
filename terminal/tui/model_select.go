package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// modelItem 模型列表项
type modelItem struct {
	model   Model
	current bool
}

func (i modelItem) Title() string {
	prefix := "  "
	if i.current {
		prefix = "● "
	}
	return prefix + i.model.Name
}

func (i modelItem) Description() string {
	status := "disabled"
	if i.model.Status == "enabled" {
		status = "enabled"
	}
	return i.model.Provider + " | " + status
}

func (i modelItem) FilterValue() string {
	return i.model.Name + " " + i.model.Provider
}

// modelSelectModel bubbletea 模型选择器
type modelSelectModel struct {
	list         list.Model
	currentModel string
	quitted      bool
	selected     string
	width, height int
}

// NewModelSelect 创建模型选择器
func NewModelSelect(models []Model, currentModel string) modelSelectModel {
	items := make([]list.Item, 0, len(models))
	for _, m := range models {
		if m.Status != "enabled" {
			continue
		}
		items = append(items, modelItem{model: m, current: m.Name == currentModel})
	}

	if len(items) == 0 {
		items = append(items, modelItem{model: Model{Name: "No models available"}, current: false})
	}

	// 自定义委托样式
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#61AFEF")).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#5C6370"))
	delegate.ShowDescription = true

	l := list.New(items, delegate, 0, 0)
	l.Title = "  Select Model"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	return modelSelectModel{
		list:         l,
		currentModel: currentModel,
	}
}

func (m modelSelectModel) Init() tea.Cmd {
	return nil
}

func (m modelSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width, msg.Height-4)
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitted = true
			return m, tea.Quit
		case "enter":
			if item, ok := m.list.SelectedItem().(modelItem); ok {
				m.selected = item.model.Name
			}
			m.quitted = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m modelSelectModel) View() string {
	if m.quitted {
		return ""
	}
	help := StyleMuted.Render("↑/↓ navigate  •  enter select  •  q/esc cancel")
	return fmt.Sprintf("%s\n\n%s", m.list.View(), help)
}

// RunModelSelect 运行模型选择器（独立 tea.Program）
func RunModelSelect(apiURL, currentModel string) ModelSelectResult {
	models, err := FetchModels(apiURL)
	if err != nil {
		fmt.Printf("%s Failed to fetch models: %v\n", StyleError.Render("Error:"), err)
		return ModelSelectResult{}
	}

	m := NewModelSelect(models, currentModel)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Printf("%s Error: %v\n", StyleError.Render("Error:"), err)
		return ModelSelectResult{}
	}

	if finalModel, ok := result.(modelSelectModel); ok {
		return ModelSelectResult{
			Selected: finalModel.selected,
			Changed:  finalModel.selected != "" && finalModel.selected != currentModel,
		}
	}

	return ModelSelectResult{}
}
