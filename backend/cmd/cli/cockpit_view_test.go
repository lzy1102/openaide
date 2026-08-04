package main

import (
	"strings"
	"testing"
	"time"

	"openaide/backend/internal/lang"
)

// 模拟流式执行中的完整 View() 渲染，验证驾驶舱布局完整性
func TestCockpitViewStreaming(t *testing.T) {
	m := testModel()
	m.mode = modeStreaming
	m.stream.startTime = time.Now().Add(-45 * time.Second)
	m.stream.totalTokens = 2500
	m.stream.totalTools = 8
	m.stream.streamRound = 3
	m.stream.streamTotal = 10
	m.stream.cacheHit = 70
	m.stream.cacheMiss = 30
	m.stream.toolNames = []string{"read_file", "search_files", "execute_command"}
	m.stream.fullResponse = "thinking..."

	view := m.View()

	// HUD: 模式 + 模型 + git
	for _, want := range []string{"test-model", "master", "Working"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing HUD %q", want)
		}
	}
	// 仪表盘: token/tools/round/elapsed/cache
	for _, want := range []string{"2.5k", "8", "3/10", "45s", "70%"} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing gauge %q", want)
		}
	}
	// 侧翼: 工具历史
	if !strings.Contains(view, "read_file") {
		t.Errorf("View missing tool history in side panel")
	}
}

// 窄终端降级: 侧翼面板(工具历史标题)不渲染,但状态行工具名仍显示
func TestCockpitViewNarrow(t *testing.T) {
	m := testModel()
	m.width = 80
	m.mode = modeStreaming
	m.stream.startTime = time.Now()
	m.stream.toolNames = []string{"read_file"}
	view := m.View()
	if strings.Contains(view, lang.T("repl.tools_running")) {
		t.Errorf("narrow terminal should not show tool history panel title")
	}
}
