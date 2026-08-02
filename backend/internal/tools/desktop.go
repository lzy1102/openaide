package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"openaide/backend/internal/kernel"
)

// detectOS returns "linux", "darwin", or "windows"
func detectOS() string { return runtime.GOOS }

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

// ── Cross-platform desktop helpers ─────────────────────────

func screenshotCmd(region string) *exec.Cmd {
	tmpFile := "/tmp/openaide_screenshot.png"
	switch detectOS() {
	case "linux":
		switch region {
		case "active_window":
			if _, err := exec.LookPath("gnome-screenshot"); err == nil {
				return exec.Command("gnome-screenshot", "-w", "-f", tmpFile)
			}
			return exec.Command("scrot", "-u", tmpFile)
		case "selection":
			return exec.Command("gnome-screenshot", "-a", "-f", tmpFile)
		default:
			if _, err := exec.LookPath("gnome-screenshot"); err == nil {
				return exec.Command("gnome-screenshot", "-f", tmpFile)
			}
			return exec.Command("scrot", tmpFile)
		}
	case "darwin":
		switch region {
		case "selection":
			return exec.Command("screencapture", "-i", tmpFile)
		case "active_window":
			return exec.Command("screencapture", "-w", tmpFile)
		default:
			return exec.Command("screencapture", "-x", tmpFile)
		}
	case "windows":
		// PowerShell: capture screen to file
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; Add-Type -AssemblyName System.Drawing; `+
				`$screen = [System.Windows.Forms.Screen]::PrimaryScreen; `+
				`$bmp = New-Object System.Drawing.Bitmap($screen.Bounds.Width, $screen.Bounds.Height); `+
				`$g = [System.Drawing.Graphics]::FromImage($bmp); `+
				`$g.CopyFromScreen(0,0,0,0,$bmp.Size); $bmp.Save('%s'); $g.Dispose(); $bmp.Dispose()`,
			strings.ReplaceAll(tmpFile, "/", "\\"))
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func clickCmd(x, y int, button string, double bool) *exec.Cmd {
	switch detectOS() {
	case "linux":
		args := []string{"mousemove", strconv.Itoa(x), strconv.Itoa(y), "click"}
		btnMap := map[string]string{"left": "1", "middle": "2", "right": "3"}
		btn := btnMap[button]
		if btn == "" {
			btn = "1"
		}
		if double {
			args = append(args, "--repeat", "2", btn)
		} else {
			args = append(args, btn)
		}
		return exec.Command("xdotool", args...)
	case "darwin":
		// AppleScript click at coordinates
		clicks := "click"
		if double {
			clicks = "double click"
		}
		script := fmt.Sprintf(
			`tell application "System Events" to %s at {%d, %d}`,
			clicks, x, y)
		return exec.Command("osascript", "-e", script)
	case "windows":
		// PowerShell + mouse_event via C#
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; `+
				`[System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d,%d); `+
				`Add-Type -MemberDefinition '[DllImport("user32.dll")] public static extern void mouse_event(int dwFlags, int dx, int dy, int dwData, int dwExtraInfo);' `+
				`-Name Win32 -Namespace Mouse; `+
				`[Mouse.Win32]::mouse_event(0x0002,0,0,0,0); [Mouse.Win32]::mouse_event(0x0004,0,0,0,0)`,
			x, y)
		if double {
			script = strings.Replace(script, "0x0004", "0x0004", 1)
			script += `; Start-Sleep -Milliseconds 100; [Mouse.Win32]::mouse_event(0x0002,0,0,0,0); [Mouse.Win32]::mouse_event(0x0004,0,0,0,0)`
		}
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func typeCmd(text string) *exec.Cmd {
	switch detectOS() {
	case "linux":
		return exec.Command("xdotool", "type", "--", text)
	case "darwin":
		script := fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, text)
		return exec.Command("osascript", "-e", script)
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('%s')`,
			text)
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func keyCmd(keys string) *exec.Cmd {
	k := strings.ToLower(keys)
	k = strings.ReplaceAll(k, " ", "+")
	switch detectOS() {
	case "linux":
		k = strings.ReplaceAll(k, "enter", "Return")
		k = strings.ReplaceAll(k, "esc", "Escape")
		return exec.Command("xdotool", "key", k)
	case "darwin":
		// Map common keys to AppleScript key codes
		k = strings.ReplaceAll(k, "enter", "return")
		k = strings.ReplaceAll(k, "esc", "escape")
		k = strings.ReplaceAll(k, "ctrl", "control")
		script := fmt.Sprintf(`tell application "System Events" to keystroke (%s)`, k)
		return exec.Command("osascript", "-e", script)
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('{%s}')`,
			k)
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func scrollCmd(x, y int) *exec.Cmd {
	switch detectOS() {
	case "linux":
		// xdotool: button 4=up, 5=down, 6=left, 7=right
		args := []string{}
		for i := 0; i > y; i-- {
			args = append(args, "click", "4")
		}
		for i := 0; i < y; i++ {
			args = append(args, "click", "5")
		}
		for i := 0; i > x; i-- {
			args = append(args, "click", "6")
		}
		for i := 0; i < x; i++ {
			args = append(args, "click", "7")
		}
		return exec.Command("xdotool", args...)
	case "darwin":
		count := abs(y)
		if count == 0 {
			count = 1
		}
		script := fmt.Sprintf(`repeat %d times\ntell application "System Events" to key code %s\nend repeat`,
			count, map[bool]string{true: "126", false: "125"}[y < 0])
		return exec.Command("osascript", "-e", script)
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -MemberDefinition '[DllImport("user32.dll")] public static extern void mouse_event(int dwFlags, int dx, int dy, int dwData, int dwExtraInfo);' `+
				`-Name Win32 -Namespace Mouse; [Mouse.Win32]::mouse_event(0x0800,0,0,%d,0)`,
			y*120)
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func moveCmd(x, y int) *exec.Cmd {
	switch detectOS() {
	case "linux":
		return exec.Command("xdotool", "mousemove", strconv.Itoa(x), strconv.Itoa(y))
	case "darwin":
		script := fmt.Sprintf(
			`tell application "System Events" to set position of mouse to {%d, %d}`, x, y)
		return exec.Command("osascript", "-e", script)
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Cursor]::Position = New-Object System.Drawing.Point(%d,%d)`,
			x, y)
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func dragCmd(x1, y1, x2, y2 int) *exec.Cmd {
	switch detectOS() {
	case "linux":
		return exec.Command("xdotool", "mousemove", strconv.Itoa(x1), strconv.Itoa(y1),
			"mousedown", "1", "mousemove", strconv.Itoa(x2), strconv.Itoa(y2), "mouseup", "1")
	case "darwin":
		script := fmt.Sprintf(
			`tell application "System Events"
	tell process "Finder"
		set position of mouse to {%d, %d}
		click and drag to {%d, %d}
	end tell
end tell`, x1, y1, x2, y2)
		return exec.Command("osascript", "-e", script)
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; `+
				`Add-Type -MemberDefinition '[DllImport("user32.dll")] public static extern void mouse_event(int dwFlags, int dx, int dy, int dwData, int dwExtraInfo); public static extern bool SetCursorPos(int X, int Y);' `+
				`-Name Win32 -Namespace Mouse; `+
				`[Mouse.Win32]::SetCursorPos(%d,%d); [Mouse.Win32]::mouse_event(0x0002,0,0,0,0); `+
				`[Mouse.Win32]::SetCursorPos(%d,%d); [Mouse.Win32]::mouse_event(0x0004,0,0,0,0)`,
			x1, y1, x2, y2)
		return exec.Command("powershell", "-Command", script)
	}
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// ── Tool handlers ─────────────────────────────────────

func handleDesktopScreenshot(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Region string `json:"region,omitempty"`
	}
	json.Unmarshal([]byte(arguments), &args)

	tmpFile := "/tmp/openaide_screenshot.png"
	cmd := screenshotCmd(args.Region)
	if cmd == nil {
		return &kernel.ToolResult{Error: "screenshot not supported on " + detectOS()}, nil
	}
	if err := cmd.Run(); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("screenshot failed: %v (install gnome-screenshot/scrot/screencapture)", err)}, nil
	}

	data, _ := os.ReadFile(tmpFile)
	b64 := base64.StdEncoding.EncodeToString(data)
	os.Remove(tmpFile)
	return &kernel.ToolResult{Content: fmt.Sprintf("data:image/png;base64,%s", b64)}, nil
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

	cmd := clickCmd(args.X, args.Y, args.Button, args.Double)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop click not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Clicked %s at (%d,%d)", args.Button, args.X, args.Y)}, nil
}

func handleDesktopType(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Text string `json:"text"`
	}
	json.Unmarshal([]byte(arguments), &args)
	cmd := typeCmd(args.Text)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop type not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Typed %d chars", len(args.Text))}, nil
}

func handleDesktopKey(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Keys string `json:"keys"`
	}
	json.Unmarshal([]byte(arguments), &args)
	cmd := keyCmd(args.Keys)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop key not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Pressed: %s", args.Keys)}, nil
}

func handleDesktopScroll(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct{ X, Y int }
	json.Unmarshal([]byte(arguments), &args)
	cmd := scrollCmd(args.X, args.Y)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop scroll not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Scrolled (x:%d y:%d)", args.X, args.Y)}, nil
}

func handleDesktopMove(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct{ X, Y int }
	json.Unmarshal([]byte(arguments), &args)
	cmd := moveCmd(args.X, args.Y)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop move not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Moved to (%d,%d)", args.X, args.Y)}, nil
}

func handleDesktopDrag(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct{ X1, Y1, X2, Y2 int }
	json.Unmarshal([]byte(arguments), &args)
	cmd := dragCmd(args.X1, args.Y1, args.X2, args.Y2)
	if cmd == nil {
		return &kernel.ToolResult{Error: "desktop drag not supported on " + detectOS()}, nil
	}
	cmd.Run()
	return &kernel.ToolResult{Content: fmt.Sprintf("// Dragged (%d,%d)→(%d,%d)", args.X1, args.Y1, args.X2, args.Y2)}, nil
}
