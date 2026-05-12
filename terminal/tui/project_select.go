package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type projectItem struct {
	project Project
	current bool
}

func (i projectItem) Title() string {
	prefix := "  "
	if i.current {
		prefix = "● "
	}
	name := i.project.Name
	if i.project.IsDefault {
		name += " (default)"
	}
	return prefix + name
}

func (i projectItem) Description() string {
	desc := i.project.Description
	if desc == "" {
		desc = "No description"
	}
	if i.project.WorkingDir != "" {
		desc += " · " + i.project.WorkingDir
	}
	if i.project.ModelID != "" {
		desc += " · model: " + i.project.ModelID
	}
	return desc
}

func (i projectItem) FilterValue() string {
	return i.project.Name + " " + i.project.Description
}

type projectSelectModel struct {
	list            list.Model
	currentProjectID string
	quitted         bool
	selected        *Project
	width, height   int
}

func NewProjectSelect(projects []Project, currentProjectID string) projectSelectModel {
	items := make([]list.Item, 0, len(projects))
	for _, p := range projects {
		items = append(items, projectItem{project: p, current: p.ID == currentProjectID})
	}

	if len(items) == 0 {
		items = append(items, projectItem{project: Project{Name: "No projects available"}, current: false})
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#C678DD")).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#5C6370"))
	delegate.ShowDescription = true

	l := list.New(items, delegate, 0, 0)
	l.Title = "  Select Project"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	return projectSelectModel{
		list:             l,
		currentProjectID: currentProjectID,
	}
}

func (m projectSelectModel) Init() tea.Cmd {
	return nil
}

func (m projectSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if item, ok := m.list.SelectedItem().(projectItem); ok {
				m.selected = &item.project
			}
			m.quitted = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m projectSelectModel) View() string {
	if m.quitted {
		return ""
	}
	help := StyleMuted.Render("↑/↓ navigate  •  enter select  •  q/esc cancel")
	return fmt.Sprintf("%s\n\n%s", m.list.View(), help)
}

type ProjectSelectResult struct {
	Selected *Project
	Changed  bool
}

func RunProjectSelect(apiURL, currentProjectID string) ProjectSelectResult {
	projects, err := FetchProjects(apiURL)
	if err != nil {
		fmt.Printf("%s Failed to fetch projects: %v\n", StyleError.Render("Error:"), err)
		return ProjectSelectResult{}
	}

	m := NewProjectSelect(projects, currentProjectID)
	p := tea.NewProgram(m, tea.WithAltScreen())

	result, err := p.Run()
	if err != nil {
		fmt.Printf("%s Error: %v\n", StyleError.Render("Error:"), err)
		return ProjectSelectResult{}
	}

	if finalModel, ok := result.(projectSelectModel); ok {
		changed := finalModel.selected != nil && finalModel.selected.ID != currentProjectID
		return ProjectSelectResult{
			Selected: finalModel.selected,
			Changed:  changed,
		}
	}

	return ProjectSelectResult{}
}

func AutoDetectProject(apiURL string) string {
	projects, err := FetchProjects(apiURL)
	if err != nil || len(projects) == 0 {
		return ""
	}

	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	for _, p := range projects {
		if p.WorkingDir != "" && wd == p.WorkingDir {
			return p.ID
		}
	}

	for _, p := range projects {
		if p.WorkingDir != "" && len(wd) > len(p.WorkingDir) && wd[:len(p.WorkingDir)] == p.WorkingDir {
			return p.ID
		}
	}

	return ""
}
