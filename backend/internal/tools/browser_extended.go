package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"openaide/backend/internal/kernel"

	"github.com/chromedp/chromedp"
)

// Extended browser tools: coordinate-based click, scroll, keyboard input
// These enable "computer use" style interaction within a browser tab

func browserExtendedDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_click_at",
				Description: "Click at specific (x,y) coordinates in the browser viewport. Use browser_screenshot to see the page first, then click at the desired position.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x": map[string]interface{}{"type": "integer", "description": "X coordinate in viewport"},
						"y": map[string]interface{}{"type": "integer", "description": "Y coordinate in viewport"},
					},
					"required": []string{"x", "y"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_scroll",
				Description: "Scroll the browser page by (x, y) pixels. Positive y = scroll down, 300 = roughly one page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"y": map[string]interface{}{"type": "integer", "description": "Vertical scroll (positive=down, 300≈1 page)"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_type",
				Description: "Type keyboard input into the browser. Use for filling forms with keyboard or pressing keys like Enter, Escape, Tab.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"text": map[string]interface{}{"type": "string", "description": "Text to type, or key name: Enter, Escape, Tab, ArrowDown, ArrowUp"},
					},
					"required": []string{"text"},
				},
			},
		},
	}
}

func handleBrowserClickAt(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	err := chromedp.Run(browserCtx,
		chromedp.MouseClickXY(float64(args.X), float64(args.Y)),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("click failed: %v", err)}, nil
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("// Clicked at (%d, %d)", args.X, args.Y)}, nil
}

func handleBrowserScroll(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Y int `json:"y,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)
	if args.Y == 0 {
		args.Y = 300 // default: scroll down ~1 page
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	err := chromedp.Run(browserCtx,
		chromedp.EvaluateAsDevTools(
			fmt.Sprintf("window.scrollBy(0, %d)", args.Y), nil,
		),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("scroll failed: %v", err)}, nil
	}
	dir := "down"
	if args.Y < 0 {
		dir = "up"
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("// Scrolled %s by %dpx", dir, int(math.Abs(float64(args.Y))))}, nil
}

func handleBrowserType(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(arguments), &args)

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	// Translate common key names to chromedp key events
	keyMap := map[string]string{
		"Enter": "\r", "Return": "\r", "Escape": "",
		"Tab": "\t", "Backspace": "\b", "Delete": "",
	}

	var actions []chromedp.Action
	if k, ok := keyMap[args.Text]; ok {
		// Special key
		actions = append(actions, chromedp.KeyEvent(k))
	} else if len(args.Text) == 1 {
		actions = append(actions, chromedp.KeyEvent(args.Text))
	} else {
		// Typing text — use keyboard type
		actions = append(actions, chromedp.KeyEvent(args.Text))
	}

	if err := chromedp.Run(browserCtx, actions...); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("type failed: %v", err)}, nil
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("// Typed: %s", args.Text)}, nil
}

// Capture viewport dimensions for coordinate calculations
func getViewportDims(ctx context.Context) (int, int, error) {
	var width, height int
	err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools("window.innerWidth", &width),
		chromedp.EvaluateAsDevTools("window.innerHeight", &height),
	)
	return width, height, err
}

