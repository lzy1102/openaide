package kernel

import (
	"strings"
	"testing"
)

func TestTruncateToolResult_ShortContent(t *testing.T) {
	content := "short content"
	got := truncateToolResult(content)
	if got != content {
		t.Errorf("short content should not be truncated, got %q", got)
	}
}

func TestTruncateToolResult_ExactLimit(t *testing.T) {
	content := strings.Repeat("x", maxToolResultChars)
	got := truncateToolResult(content)
	if got != content {
		t.Errorf("content at exact limit should not be truncated")
	}
}

func TestTruncateToolResult_OverLimit(t *testing.T) {
	content := strings.Repeat("x", maxToolResultChars+10000)
	got := truncateToolResult(content)
	if len(got) >= len(content) {
		t.Errorf("over-limit content should be truncated, got len=%d, original=%d", len(got), len(content))
	}
	if !strings.Contains(got, "chars truncated") {
		t.Errorf("truncated content should contain truncation marker, got: %s", got[:min(100, len(got))])
	}
	// 头部应保留内容开头
	if !strings.HasPrefix(got, "xxx") {
		t.Errorf("truncated content should preserve head, got prefix: %q", got[:min(20, len(got))])
	}
	// 尾部应保留内容结尾
	if !strings.HasSuffix(got, "xxx") {
		t.Errorf("truncated content should preserve tail, got suffix: %q", got[max(0, len(got)-20):])
	}
}

func TestTruncateToolResult_UTF8Safe(t *testing.T) {
	// 混合中英文,确保截断不破坏 UTF-8 字符
	content := strings.Repeat("中", maxToolResultChars) + strings.Repeat("x", 5000)
	got := truncateToolResult(content)
	if len(got) >= len(content) {
		t.Errorf("should be truncated")
	}
	// 验证截断后的字符串是合法 UTF-8
	if !isValidUTF8(got) {
		t.Errorf("truncated content has broken UTF-8")
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD {
			return false
		}
	}
	return true
}

func TestSnipOldToolOutputsDynamic_LowPressure(t *testing.T) {
	msgs := []Message{
		{Role: "tool", Content: strings.Repeat("a", 2000)},
		{Role: "tool", Content: strings.Repeat("b", 2000)},
		{Role: "tool", Content: strings.Repeat("c", 2000)},
		{Role: "tool", Content: strings.Repeat("d", 2000)},
		{Role: "tool", Content: strings.Repeat("e", 2000)},
		{Role: "tool", Content: strings.Repeat("f", 2000)},
	}
	snipOldToolOutputsDynamic(msgs, pressureLow)
	// 低压力:keepFull=4,最近 4 条完整保留
	if msgs[5].Content != strings.Repeat("f", 2000) {
		t.Error("most recent 4 should be kept full (low pressure)")
	}
	// 第 5 条(索引 1)应该被裁剪
	if !strings.Contains(msgs[1].Content, "snipped") {
		t.Error("5th message should be snipped (low pressure)")
	}
}

func TestSnipOldToolOutputsDynamic_HighPressure(t *testing.T) {
	msgs := []Message{
		{Role: "tool", Content: strings.Repeat("a", 2000)},
		{Role: "tool", Content: strings.Repeat("b", 2000)},
		{Role: "tool", Content: strings.Repeat("c", 2000)},
	}
	snipOldToolOutputsDynamic(msgs, pressureHigh)
	// 高压力:keepFull=1,只有最近 1 条完整保留
	if msgs[2].Content != strings.Repeat("c", 2000) {
		t.Error("most recent 1 should be kept full (high pressure)")
	}
	// 第 2 条(索引 1)应该被裁剪
	if !strings.Contains(msgs[1].Content, "snipped") {
		t.Error("2nd message should be snipped (high pressure)")
	}
}

func TestEstimateContextPressure(t *testing.T) {
	k := &AgentKernel{maxTokens: 100000}

	if k.estimateContextPressure(50000) != pressureLow {
		t.Error("50% should be low pressure")
	}
	if k.estimateContextPressure(75000) != pressureMedium {
		t.Error("75% should be medium pressure")
	}
	if k.estimateContextPressure(90000) != pressureHigh {
		t.Error("90% should be high pressure")
	}

	// maxTokens=0 不应 panic
	k2 := &AgentKernel{maxTokens: 0}
	if k2.estimateContextPressure(999999) != pressureLow {
		t.Error("maxTokens=0 should default to low pressure")
	}
}
