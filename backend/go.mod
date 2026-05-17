module openaide/backend

go 1.26

require (
	golang.org/x/term v0.43.0
	gopkg.in/yaml.v3 v3.0.1
	openaide/backend/internal/infra v0.0.0-00010101000000-000000000000
	openaide/backend/internal/kernel v0.0.0
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/charmbracelet/bubbles v1.0.0 // indirect
	github.com/charmbracelet/bubbletea v1.3.10 // indirect
	github.com/charmbracelet/colorprofile v0.4.1 // indirect
	github.com/charmbracelet/lipgloss v1.1.0 // indirect
	github.com/charmbracelet/x/ansi v0.11.6 // indirect
	github.com/charmbracelet/x/cellbuf v0.0.15 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/chromedp/cdproto v0.0.0-20260321001828-e3e3800016bc // indirect
	github.com/chromedp/chromedp v0.15.1 // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/clipperhouse/displaywidth v0.9.0 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.5.0 // indirect
	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
	github.com/go-json-experiment/json v0.0.0-20260214004413-d219187c3433 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-localereader v0.0.1 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/sys v0.44.0 // indirect
	golang.org/x/text v0.3.8 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	openaide/backend/internal/api v0.0.0 // indirect
	openaide/backend/internal/compress v0.0.0-00010101000000-000000000000 // indirect
	openaide/backend/internal/event v0.0.0-00010101000000-000000000000 // indirect
	openaide/backend/internal/llm v0.0.0 // indirect
	openaide/backend/internal/memory v0.0.0 // indirect
	openaide/backend/internal/orchestration v0.0.0 // indirect
	openaide/backend/internal/tools v0.0.0 // indirect
)

replace (
	openaide/backend/internal/api => ./internal/api
	openaide/backend/internal/compress => ./internal/compress
	openaide/backend/internal/config => ./internal/config
	openaide/backend/internal/event => ./internal/event
	openaide/backend/internal/git => ./internal/git
	openaide/backend/internal/identity => ./internal/identity
	openaide/backend/internal/index => ./internal/index
	openaide/backend/internal/infra => ./internal/infra
	openaide/backend/internal/kernel => ./internal/kernel
	openaide/backend/internal/knowledge => ./internal/knowledge
	openaide/backend/internal/llm => ./internal/llm
	openaide/backend/internal/memory => ./internal/memory
	openaide/backend/internal/orchestration => ./internal/orchestration
	openaide/backend/internal/tools => ./internal/tools
)
