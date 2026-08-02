package main

import (
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIsSuspiciousKey(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want bool
	}{
		{"normal single char", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}, false},
		{"paste", tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("gb:0c0c")}, true},
		{"paste single char", tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("a")}, true},
		{"ctrl+l form feed", tea.KeyMsg{Type: tea.KeyCtrlL}, true},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, true},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, true},
		{"alt+enter", tea.KeyMsg{Type: tea.KeyEnter, Alt: true}, true},
		{"space is normal typing", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")}, false},
		{"multi runes", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ab")}, true},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, true},
	}
	for _, c := range cases {
		if got := isSuspiciousKey(c.key); got != c.want {
			t.Errorf("%s: isSuspiciousKey(%q) = %v, want %v", c.name, c.key.String(), got, c.want)
		}
	}
}

func TestSuspiciousKeyMatchesPasteStrings(t *testing.T) {
	// 用户报告的异常输入:gb:0c0c/0c0c/0c0c\
	// 若经 paste 进入,必须被诊断日志捕获
	for _, s := range []string{"gb:0c0c/0c0c/0c0c\\", "gb:0c0c", "0c0c/0c0c/0c0c"} {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune(s)}
		if !isSuspiciousKey(msg) {
			t.Errorf("paste %q should be flagged suspicious", s)
		}
	}
}

func TestSuspiciousKeyNormalTypingNotFlagged(t *testing.T) {
	// 正常打字(逐字符可见字符)不应产生日志噪音
	for _, s := range []string{"h", "e", "l", "o", "中", "文"} {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
		if isSuspiciousKey(msg) {
			t.Errorf("normal typing %q should NOT be flagged", s)
		}
	}
}

func TestKeyMsgStringQuoted(t *testing.T) {
	// strconv.Quote 确保日志中控制字符可见(如 \x0c)
	msg := tea.KeyMsg{Type: tea.KeyCtrlL}
	quoted := strconv.Quote(msg.String())
	if !strings.Contains(quoted, "ctrl+l") {
		t.Errorf("quoted key string %q should contain ctrl+l", quoted)
	}
}
