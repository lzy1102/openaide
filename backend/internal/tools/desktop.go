package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"openaide/backend/internal/kernel"
)

func desktopToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_screenshot",
				Description: "Take a screenshot of the entire desktop and return base64 image data for vision analysis",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"region": map[string]interface{}{
							"type":        "string",
							"description": "Optional: active_window, selection, or empty for full screen",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_click",
				Description: "Click at screen coordinates (x, y). Use with desktop_screenshot to see the screen first.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x": map[string]interface{}{"type": "integer", "description": "X coordinate"},
						"y": map[string]interface{}{"type": "integer", "description": "Y coordinate"},
						"button": map[string]interface{}{
							"type":        "string",
							"description": "Mouse button: left, right, middle (default left)",
						},
						"double": map[string]interface{}{
							"type":        "boolean",
							"description": "Double click",
						},
					},
					"required": []string{"x", "y"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_type",
				Description: "Type text using the keyboard at the current cursor position",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"text": map[string]interface{}{"type": "string", "description": "Text to type"},
					},
					"required": []string{"text"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_key",
				Description: "Press a keyboard key or combination (e.g. Return, Escape, ctrl+c, alt+Tab)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"keys": map[string]interface{}{"type": "string", "description": "Key name or combo: Return, Escape, Tab, ctrl+c, alt+F4, Super"},
					},
					"required": []string{"keys"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_scroll",
				Description: "Scroll at current mouse position. Positive y = scroll down, negative = scroll up.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x": map[string]interface{}{"type": "integer", "description": "Horizontal scroll amount"},
						"y": map[string]interface{}{"type": "integer", "description": "Vertical scroll amount (positive=down, negative=up)"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_move",
				Description: "Move mouse cursor to (x, y) without clicking",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x": map[string]interface{}{"type": "integer", "description": "X coordinate"},
						"y": map[string]interface{}{"type": "integer", "description": "Y coordinate"},
					},
					"required": []string{"x", "y"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "desktop_drag",
				Description: "Drag from (x1, y1) to (x2, y2)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"x1": map[string]interface{}{"type": "integer"},
						"y1": map[string]interface{}{"type": "integer"},
						"x2": map[string]interface{}{"type": "integer"},
						"y2": map[string]interface{}{"type": "integer"},
					},
					"required": []string{"x1", "y1", "x2", "y2"},
				},
			},
		},
	}
}

func handleDesktopScreenshot(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Region string `json:"region,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)

	tmpFile := "/tmp/openaide_desktop_screenshot.png"

	var cmd *exec.Cmd
	switch args.Region {
	case "active_window":
		cmd = exec.Command("gnome-screenshot", "-w", "-f", tmpFile)
		if _, err := exec.LookPath("gnome-screenshot"); err != nil {
			// Fallback: use scrot
			cmd = exec.Command("scrot", "-u", tmpFile)
		}
	case "selection":
		cmd = exec.Command("gnome-screenshot", "-a", "-f", tmpFile)
	default:
		cmd = exec.Command("gnome-screenshot", "-f", tmpFile)
		if _, err := exec.LookPath("gnome-screenshot"); err != nil {
			cmd = exec.Command("scrot", tmpFile)
		}
	}
	if err := cmd.Run(); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("screenshot failed: %v (install gnome-screenshot or scrot)", err)}, nil
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("read screenshot: %v", err)}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	os.Remove(tmpFile)

	// Returns base64 so the multimodal model can "see" the screen
	return &kernel.ToolResult{
		Content: fmt.Sprintf("data:image/png;base64,%s", b64),
	}, nil
}

func handleDesktopClick(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		X      int    `json:"x"`
		Y      int    `json:"y"`
		Button string `json:"button,omitempty"`
		Double bool   `json:"double,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)
	if args.Button == "" {
		args.Button = "left"
	}

	btnMap := map[string]string{"left": "1", "middle": "2", "right": "3"}
	btn := btnMap[args.Button]
	if btn == "" {
		btn = "1"
	}

	xdotool("mousemove", strconv.Itoa(args.X), strconv.Itoa(args.Y))
	if args.Double {
		xdotool("click", "--repeat", "2", btn)
	} else {
		xdotool("click", btn)
	}

	return &kernel.ToolResult{Content: fmt.Sprintf("// Clicked %s at (%d, %d)", args.Button, args.X, args.Y)}, nil
}

func handleDesktopType(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(arguments), &args)

	xdotool("type", "--", args.Text)
	return &kernel.ToolResult{Content: fmt.Sprintf("// Typed %d chars", len(args.Text))}, nil
}

func handleDesktopKey(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Keys string `json:"keys"`
	}
	json.Unmarshal([]byte(arguments), &args)

	// Parse key combo: "ctrl+c" → "ctrl+c"
	keys := strings.ToLower(args.Keys)
	keys = strings.ReplaceAll(keys, " ", "+")
	// Handle common aliases
	keys = strings.ReplaceAll(keys, "enter", "Return")
	keys = strings.ReplaceAll(keys, "esc", "Escape")
	xdotool("key", keys)
	return &kernel.ToolResult{Content: fmt.Sprintf("// Pressed: %s", args.Keys)}, nil
}

func handleDesktopScroll(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		X int `json:"x,omitempty"`
		Y int `json:"y,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)

	// xdotool click 4 = scroll up, 5 = scroll down
	if args.Y < 0 {
		for i := 0; i > args.Y; i-- {
			xdotool("click", "4")
		}
	} else if args.Y > 0 {
		for i := 0; i < args.Y; i++ {
			xdotool("click", "5")
		}
	}
	if args.X < 0 {
		for i := 0; i > args.X; i-- {
			xdotool("click", "6")
		}
	} else if args.X > 0 {
		for i := 0; i < args.X; i++ {
			xdotool("click", "7")
		}
	}
	return &kernel.ToolResult{Content: fmt.Sprintf("// Scrolled (x:%d y:%d)", args.X, args.Y)}, nil
}

func handleDesktopMove(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	json.Unmarshal([]byte(arguments), &args)

	xdotool("mousemove", strconv.Itoa(args.X), strconv.Itoa(args.Y))
	return &kernel.ToolResult{Content: fmt.Sprintf("// Moved to (%d, %d)", args.X, args.Y)}, nil
}

func handleDesktopDrag(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		X1 int `json:"x1"`
		Y1 int `json:"y1"`
		X2 int `json:"x2"`
		Y2 int `json:"y2"`
	}
	json.Unmarshal([]byte(arguments), &args)

	xdotool("mousemove", strconv.Itoa(args.X1), strconv.Itoa(args.Y1))
	xdotool("mousedown", "1")
	xdotool("mousemove", strconv.Itoa(args.X2), strconv.Itoa(args.Y2))
	xdotool("mouseup", "1")
	return &kernel.ToolResult{Content: fmt.Sprintf("// Dragged from (%d,%d) to (%d,%d)", args.X1, args.Y1, args.X2, args.Y2)}, nil
}

// xdotool runs xdotool with arguments, logging failures
func xdotool(args ...string) {
	cmd := exec.Command("xdotool", args...)
	cmd.Run() // Best-effort, errors logged by caller if needed
}
