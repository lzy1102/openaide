package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
)

func webToolDefs() []kernel.ToolDefinition {
	return []kernel.ToolDefinition{
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "web_search",
				Description: "联网搜索，获取最新信息",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string", "description": "搜索关键词"},
						"limit": map[string]interface{}{"type": "integer", "description": "结果数量（默认5）"},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "web_fetch",
				Description: "抓取网页内容，提取正文文本",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{"type": "string", "description": "网页URL"},
						"max_length": map[string]interface{}{"type": "integer", "description": "最大返回长度"},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: kernel.FunctionDef{
				Name:        "ai_search",
				Description: "AI增强搜索：搜索+抓取+分析一步到位",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{"type": "string", "description": "搜索查询"},
						"fetch_pages": map[string]interface{}{"type": "boolean", "description": "是否抓取页面内容"},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}

// handleWebSearch 联网搜索
func handleWebSearch(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query  string `json:"query"`
		Limit  int    `json:"limit,omitempty"`
		Engine string `json:"engine,omitempty"` // duckduckgo, google, bing
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Engine == "" {
		args.Engine = "duckduckgo"
	}

	results, err := searchWeb(ctx, args.Query, args.Limit, args.Engine)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// Search: '%s' (%d results from %s)\n", args.Query, len(results), args.Engine))
	for i, r := range results {
		out.WriteString(fmt.Sprintf("\n## %d. %s\n  URL: %s\n  %s\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return &kernel.ToolResult{Content: out.String()}, nil
}

// handleWebFetch 抓取网页内容
func handleWebFetch(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		URL         string `json:"url"`
		MaxLength   int    `json:"max_length,omitempty"`
		ExtractText bool   `json:"extract_text,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.URL == "" {
		return &kernel.ToolResult{Error: "url is required"}, nil
	}
	if args.MaxLength <= 0 {
		args.MaxLength = 10000
	}
	if !strings.HasPrefix(args.URL, "http") {
		args.URL = "https://" + args.URL
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", args.URL, nil)
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	req.Header.Set("User-Agent", "OpenAIDE/3.0 (Web Fetcher)")

	resp, err := client.Do(req)
	if err != nil {
		return &kernel.ToolResult{Error: fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(args.MaxLength*2)))
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	content := string(body)
	if args.ExtractText || true {
		content = extractText(content)
	}
	if len(content) > args.MaxLength {
		content = content[:args.MaxLength] + "\n... (truncated)"
	}

	return &kernel.ToolResult{Content: fmt.Sprintf("// %s (%d)\n%s", resp.Status, len(content), content)}, nil
}

// handleAISearch AI驱动的智能搜索 — 搜索+抓取+AI分析
func handleAISearch(ctx context.Context, arguments string) (*kernel.ToolResult, error) {
	var args struct {
		Query      string `json:"query"`
		FetchPages bool   `json:"fetch_pages,omitempty"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}
	if args.Query == "" {
		return &kernel.ToolResult{Error: "query is required"}, nil
	}

	// 1. 搜索
	results, err := searchWeb(ctx, args.Query, 5, "duckduckgo")
	if err != nil {
		return &kernel.ToolResult{Error: err.Error()}, nil
	}

	var out strings.Builder
	out.WriteString(fmt.Sprintf("// AI Search: '%s'\n", args.Query))
	out.WriteString(fmt.Sprintf("## 搜索结果 (%d)\n", len(results)))

	for i, r := range results {
		out.WriteString(fmt.Sprintf("%d. **%s**\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet))

		// 2. 抓取页面内容
		if args.FetchPages && i < 3 {
			body, err := fetchPage(ctx, r.URL, 5000)
			if err == nil {
				out.WriteString(fmt.Sprintf("   [页面内容] %s\n", body))
			}
		}
	}

	// 3. 返回结构化结果供LLM分析
	out.WriteString("\n## 分析建议\n请根据以上搜索结果回答用户问题。优先使用官方文档和最新信息。")
	return &kernel.ToolResult{Content: out.String()}, nil
}

// ============ 搜索实现 ============

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchWeb(ctx context.Context, query string, limit int, engine string) ([]searchResult, error) {
	switch engine {
	case "duckduckgo":
		return searchDuckDuckGo(ctx, query, limit)
	default:
		return searchDuckDuckGo(ctx, query, limit)
	}
}

// searchDuckDuckGo 使用 DuckDuckGo HTML 搜索（无需 API Key）
func searchDuckDuckGo(ctx context.Context, query string, limit int) ([]searchResult, error) {
	url := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", strings.ReplaceAll(query, " ", "+"))

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 OpenAIDE/3.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseDuckDuckGoHTML(string(body), limit), nil
}

// parseDuckDuckGoHTML 解析 DuckDuckGo 搜索结果
func parseDuckDuckGoHTML(html string, limit int) []searchResult {
	var results []searchResult

	// 提取结果链接
	linkRe := regexp.MustCompile(`class="result__a"[^>]*href="([^"]*)"[^>]*>([^<]*)<`)
	matches := linkRe.FindAllStringSubmatch(html, limit)
	for _, m := range matches {
		if len(m) >= 3 {
			r := searchResult{URL: cleanURL(m[1]), Title: cleanHTML(m[2])}
			results = append(results, r)
		}
	}

	// 提取摘要
	snippetRe := regexp.MustCompile(`class="result__snippet"[^>]*>([^<]*)<`)
	snippets := snippetRe.FindAllStringSubmatch(html, limit)
	for i, s := range snippets {
		if i < len(results) && len(s) >= 2 {
			results[i].Snippet = cleanHTML(s[1])
		}
	}

	return results
}

// ============ 页面抓取 ============

func fetchPage(ctx context.Context, url string, maxLen int) (string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 OpenAIDE/3.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxLen*2)))
	if err != nil {
		return "", err
	}

	text := extractText(string(body))
	if len(text) > maxLen {
		text = text[:maxLen]
	}
	return text, nil
}

// extractText 从HTML提取纯文本
func extractText(html string) string {
	// 移除 script/style
	html = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	// 移除标签
	html = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(html, " ")
	// 清理空白
	html = regexp.MustCompile(`\s+`).ReplaceAllString(html, " ")
	// 解码常用实体
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	return strings.TrimSpace(html)
}

func cleanURL(u string) string {
	u = strings.ReplaceAll(u, "//duckduckgo.com/l/?uddg=", "")
	u = strings.TrimSpace(u)
	return u
}

func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	return strings.TrimSpace(s)
}
