package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	ColorPrimary   = lipgloss.Color("#61AFEF")
	ColorSecondary = lipgloss.Color("#98C379")
	ColorAccent    = lipgloss.Color("#C678DD")
	ColorWarning   = lipgloss.Color("#E5C07B")
	ColorError     = lipgloss.Color("#E06C75")
	ColorMuted     = lipgloss.Color("#7F848E")
	ColorDark      = lipgloss.Color("#282C34")
	ColorThinking  = lipgloss.Color("#E8C4FF")
	ColorTool      = lipgloss.Color("#FFB366")
	ColorDimText   = lipgloss.Color("#ABB2BF")
	ColorCyan      = lipgloss.Color("#56B6C2")
	ColorOrange    = lipgloss.Color("#D19A66")
	ColorPink      = lipgloss.Color("#FF7B72")
	ColorTeal      = lipgloss.Color("#3DDBD9")
	ColorLavender  = lipgloss.Color("#CDD6F4")
	ColorSurface   = lipgloss.Color("#1E222A")
	ColorSurfaceDim = lipgloss.Color("#171A22")
	ColorOverlay   = lipgloss.Color("#545D6E")
	ColorBrightWhite = lipgloss.Color("#FFFFFF")
	ColorBrightText  = lipgloss.Color("#F0F0F0")

	BgModel     = lipgloss.Color("#1A3A6B")
	BgTime      = lipgloss.Color("#0D4A4A")
	BgTokens    = lipgloss.Color("#5C4A1A")
	BgThinking  = lipgloss.Color("#4A1A6B")
	BgTool      = lipgloss.Color("#6B4A1A")
	BgSuccess   = lipgloss.Color("#1A5A1A")
	BgError     = lipgloss.Color("#6B1A1A")
	BgStream    = lipgloss.Color("#0D4A5A")
	BgKeyHint   = lipgloss.Color("#3A404A")
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
	StyleTokenCount   = lipgloss.NewStyle().Foreground(ColorDimText)
	StyleCommand      = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	StyleOutput       = lipgloss.NewStyle().Foreground(ColorDimText)
	StyleFilePath     = lipgloss.NewStyle().Foreground(ColorOrange).Underline(true)
	StyleCodeBlock    = lipgloss.NewStyle().
				Foreground(ColorBrightText).
				Background(ColorSurface).
				Padding(0, 2)
)

var (
	StyleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorOverlay)

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
			Foreground(ColorDimText)

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
				Foreground(ColorDimText).
				PaddingLeft(4)

	StyleBannerValue = lipgloss.NewStyle().
				Foreground(ColorSecondary)
)

var (
	BadgeModel = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgModel).
			Padding(0, 1).
			Bold(true)

	BadgeTime = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgTime).
			Padding(0, 1).
			Bold(true)

	BadgeTokens = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgTokens).
			Padding(0, 1).
			Bold(true)

	BadgeThinking = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgThinking).
			Padding(0, 1).
			Bold(true).
			Italic(true)

	BadgeTool = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgTool).
			Padding(0, 1).
			Bold(true)

	BadgeSuccess = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgSuccess).
			Padding(0, 1).
			Bold(true)

	BadgeError = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgError).
			Padding(0, 1).
			Bold(true)

	BadgeStream = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgStream).
			Padding(0, 1).
			Bold(true)

	BadgeWarning = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgTokens).
			Padding(0, 1).
			Bold(true)

	BadgeKeyHint = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgKeyHint).
			Padding(0, 1).
			Bold(true)

	BadgeUser = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgSuccess).
			Padding(0, 1).
			Bold(true)

	BadgeAssistant = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgModel).
			Padding(0, 1).
			Bold(true)
)

var (
	StyleThinkingBlock = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorAccent).
				Padding(0, 1).
				Foreground(ColorLavender)

	StyleToolBlock = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorTool).
			Padding(0, 1)

	StyleErrorBlock = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorError).
			Padding(0, 1).
			Foreground(ColorError)
)

var (
	StyleSectionLabel = lipgloss.NewStyle().
				Foreground(ColorDimText).
				Bold(true)

	StyleSectionValue = lipgloss.NewStyle().
				Foreground(ColorSecondary)

	StyleKeyHint = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(BgKeyHint).
			Padding(0, 1)

	StyleDescHint = lipgloss.NewStyle().
			Foreground(ColorDimText)

	StyleDivider = lipgloss.NewStyle().
			Foreground(ColorOverlay)

	StylePromptArrow = lipgloss.NewStyle().
				Foreground(ColorSecondary).
				Bold(true)

	StylePromptText = lipgloss.NewStyle().
				Foreground(ColorSecondary)
)

func Badge(label string, style lipgloss.Style) string {
	return style.Render(" " + label + " ")
}

func LabeledLine(label, value string) string {
	return fmt.Sprintf("  %s %s", StyleSectionLabel.Render(label), StyleSectionValue.Render(value))
}
