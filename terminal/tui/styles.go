package tui

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary   = lipgloss.Color("#61AFEF")
	ColorSecondary = lipgloss.Color("#98C379")
	ColorAccent    = lipgloss.Color("#C678DD")
	ColorWarning   = lipgloss.Color("#E5C07B")
	ColorError     = lipgloss.Color("#E06C75")
	ColorMuted     = lipgloss.Color("#5C6370")
	ColorDark      = lipgloss.Color("#282C34")
	ColorThinking  = lipgloss.Color("#C678DD")
	ColorTool      = lipgloss.Color("#E5C07B")
	ColorDimText   = lipgloss.Color("#7F848E")
)

var (
	StylePrimary = lipgloss.NewStyle().Foreground(ColorPrimary)
	StyleTitle   = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	StylePrompt  = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	StyleUser    = lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError)
	StyleMuted   = lipgloss.NewStyle().Foreground(ColorMuted)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSecondary)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning)
	StyleAccent  = lipgloss.NewStyle().Foreground(ColorAccent)
	StyleBold    = lipgloss.NewStyle().Bold(true)
)

var (
	StyleThinking     = lipgloss.NewStyle().Foreground(ColorThinking).Italic(true)
	StyleThinkingIcon = lipgloss.NewStyle().Foreground(ColorThinking)
	StyleToolIcon     = lipgloss.NewStyle().Foreground(ColorTool)
	StyleToolName     = lipgloss.NewStyle().Foreground(ColorTool).Bold(true)
	StyleDimText      = lipgloss.NewStyle().Foreground(ColorDimText)
	StyleAssistant    = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	StyleUserLabel    = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	StyleTokenCount   = lipgloss.NewStyle().Foreground(ColorMuted).Faint(true)
)

var (
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted)

	StylePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(1, 2)

	StylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	StyleHelpKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	StyleHelpDesc = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleStatusEnabled  = lipgloss.NewStyle().Foreground(ColorSecondary)
	StyleStatusDisabled = lipgloss.NewStyle().Foreground(ColorMuted)

	StyleListItem = lipgloss.NewStyle().
			Padding(0, 1)

	StyleItemSelected = lipgloss.NewStyle().
				Padding(0, 1).
				Foreground(ColorPrimary).
				Bold(true)
)

var (
	StyleBannerTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				Padding(0, 0, 0, 2)

	StyleBannerInfo = lipgloss.NewStyle().
				Foreground(ColorMuted).
				PaddingLeft(4)

	StyleBannerValue = lipgloss.NewStyle().
				Foreground(ColorSecondary)
)
