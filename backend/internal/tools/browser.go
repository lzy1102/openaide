package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"

	"openaide/backend/internal/kernel"
)

func browserToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_navigate",
				Description: "浏览器导航到URL，等待页面加载",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{"type": "string", "description": "目标URL"},
						"wait_ms": map[string]interface{}{"type": "integer", "description": "等待时间毫秒（默认3000）"},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_extract",
				Description: "提取当前页面文本内容（支持JS渲染后的页面）",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{"type": "string", "description": "CSS选择器（默认body）"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_screenshot",
				Description: "截取当前页面，返回base64图片",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"full_page": map[string]interface{}{"type": "boolean", "description": "是否全页截图"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_click",
				Description: "点击页面元素（CSS选择器）",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{"type": "string", "description": "CSS选择器，如 #submit, .btn, button"},
					},
					"required": []string{"selector"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "browser_fill",
				Description: "在输入框中填写文本",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{"type": "string", "description": "输入框CSS选择器"},
						"value": map[string]interface{}{"type": "string", "description": "要填写的文本"},
					},
					"required": []string{"selector", "value"},
				},
			},
		},
	}
}

// ============ Browser Manager ============

var (
	browserCtx    context.Context
	browserCancel context.CancelFunc
	allocCancel   context.CancelFunc
	browserMu     sync.Mutex
	browserReady  bool
)

// autoInstallChromium 自动检测并安装 Chromium
func autoInstallChromium() error {
	// 1. 检查是否已有 Chrome/Chromium
	for _, name := range []string{"google-chrome", "chromium-browser", "chromium", "chrome"} {
		if _, err := exec.LookPath(name); err == nil {
			return nil
		}
	}

	// 2. 尝试安装
	var cmd *exec.Cmd
	if _, err := exec.LookPath("apt-get"); err == nil {
		cmd = exec.Command("apt-get", "install", "-y", "chromium-browser")
	} else if _, err := exec.LookPath("apk"); err == nil {
		cmd = exec.Command("apk", "add", "chromium")
	} else if _, err := exec.LookPath("yum"); err == nil {
		cmd = exec.Command("yum", "install", "-y", "chromium")
	} else {
		return fmt.Errorf("no package manager found, install chromium manually")
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("auto-install chromium failed: %w", err)
	}
	return nil
}

var browserGlobalEnabled bool

// SetBrowserEnabled 设置浏览器是否启用（由app.go根据配置调用）
func SetBrowserEnabled(enabled bool) {
	browserGlobalEnabled = enabled
}

// ShutdownBrowser 关闭浏览器，释放资源
func ShutdownBrowser() {
	browserMu.Lock()
	defer browserMu.Unlock()
	if !browserReady {
		return
	}
	if browserCancel != nil {
		browserCancel()
	}
	if allocCancel != nil {
		allocCancel()
	}
	browserReady = false
}

// browserEnabled 检查浏览器功能是否启用
func browserEnabled() bool {
	return browserGlobalEnabled || os.Getenv("OPENAIDE_BROWSER") == "true"
}

// initBrowser 惰性初始化浏览器（仅在OPENAIDE_BROWSER=true时）
func initBrowser() error {
	if !browserEnabled() {
		return fmt.Errorf("browser disabled. Set OPENAIDE_BROWSER=true to enable (requires ~500MB RAM)")
	}

	browserMu.Lock()
	defer browserMu.Unlock()

	if browserReady {
		return nil
	}

	if err := autoInstallChromium(); err != nil {
		return err
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("no-first-run", true),
		chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	var aCancel context.CancelFunc
	allocCtx, aCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	allocCancel = aCancel
	browserCtx, browserCancel = chromedp.NewContext(allocCtx)

	// 验证浏览器可用（导航到空白页）
	if err := chromedp.Run(browserCtx, chromedp.Navigate("about:blank")); err != nil {
		browserCancel()
		return fmt.Errorf("browser start failed: %w", err)
	}

	browserReady = true
	return nil
}

// ============ Browser Tools ============

// handleBrowserNavigate 导航到URL
func handleBrowserNavigate(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		URL      string `json:"url"`
		WaitTime int    `json:"wait_ms,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.URL == "" {
		return &kernel.ToolResult{Error: "url is required"}, nil
	}
	if !strings.HasPrefix(args.URL, "http") {
		args.URL = "https://" + args.URL
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	if args.WaitTime == 0 {
		args.WaitTime = 3000
	}
	wait := time.Duration(args.WaitTime) * time.Millisecond

	var title, currentURL string
	err := chromedp.Run(browserCtx,
		chromedp.Navigate(args.URL),
		chromedp.Sleep(wait),
		chromedp.WaitReady("body"),
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("navigate failed: %v", err)}, nil
	}

	return &kernel.ToolResult{
		Content: fmt.Sprintf("✓ Navigated to: %s\nTitle: %s\nURL: %s", args.URL, title, currentURL),
	}, nil
}

// handleBrowserExtract 提取页面文本
func handleBrowserExtract(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Selector string `json:"selector,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Selector == "" {
		args.Selector = "body"
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var text string
	err := chromedp.Run(browserCtx,
		chromedp.WaitVisible(args.Selector),
		chromedp.Text(args.Selector, &text),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("extract failed: %v", err)}, nil
	}

	// 清理文本
	text = strings.TrimSpace(text)
	lines := strings.Split(text, "\n")
	var clean []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			clean = append(clean, l)
		}
	}
	text = strings.Join(clean, "\n")
	if len(text) > 20000 {
		text = text[:20000] + "\n... (truncated)"
	}

	return &kernel.ToolResult{Content: text}, nil
}

// handleBrowserScreenshot 截取页面截图
func handleBrowserScreenshot(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Selector string `json:"selector,omitempty"`
		FullPage bool   `json:"full_page,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var buf []byte
	actions := []chromedp.Action{chromedp.WaitVisible("body")}
	if args.FullPage {
		actions = append(actions, chromedp.FullScreenshot(&buf, 90))
	} else {
		actions = append(actions, chromedp.CaptureScreenshot(&buf))
	}

	if err := chromedp.Run(browserCtx, actions...); err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("screenshot failed: %v", err)}, nil
	}

	b64 := base64.StdEncoding.EncodeToString(buf)
	return &kernel.ToolResult{
		Content: fmt.Sprintf("data:image/png;base64,%s", b64),
	}, nil
}

// handleBrowserClick 点击元素
func handleBrowserClick(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Selector == "" {
		return &kernel.ToolResult{Error: "selector is required"}, nil
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	err := chromedp.Run(browserCtx,
		chromedp.WaitVisible(args.Selector),
		chromedp.Click(args.Selector),
		chromedp.Sleep(1*time.Second),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("click failed: %v", err)}, nil
	}

	return &kernel.ToolResult{Content: fmt.Sprintf("✓ Clicked: %s", args.Selector)}, nil
}

// handleBrowserFill 填写表单
func handleBrowserFill(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Selector string `json:"selector"`
		Value    string `json:"value"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Selector == "" || args.Value == "" {
		return &kernel.ToolResult{Error: "selector and value are required"}, nil
	}

	if err := initBrowser(); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	err := chromedp.Run(browserCtx,
		chromedp.WaitVisible(args.Selector),
		chromedp.SendKeys(args.Selector, args.Value),
	)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("fill failed: %v", err)}, nil
	}

	return &kernel.ToolResult{Content: fmt.Sprintf("✓ Filled '%s' with value", args.Selector)}, nil
}
