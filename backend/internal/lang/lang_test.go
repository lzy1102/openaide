package lang

import (
	"strings"
	"testing"
)

func TestSetLang_GetLang(t *testing.T) {
	old := GetLang()
	defer SetLang(old)

	SetLang(ZH)
	if got := GetLang(); got != ZH {
		t.Errorf("GetLang() after SetLang(ZH) = %v, want %v", got, ZH)
	}

	SetLang(EN)
	if got := GetLang(); got != EN {
		t.Errorf("GetLang() after SetLang(EN) = %v, want %v", got, EN)
	}
}

func TestSetLang_InvalidFallsBack(t *testing.T) {
	old := GetLang()
	defer SetLang(old)

	SetLang(Lang("fr"))
	if got := GetLang(); got != EN {
		t.Errorf("GetLang() after SetLang('fr') = %v, want EN", got)
	}
}

func TestT_ExistingKey(t *testing.T) {
	got := T("cli.usage")
	if got == "" {
		t.Error("T('cli.usage') returned empty string")
	}
	if got == "cli.usage" {
		t.Error("T('cli.usage') returned the key itself, expected translation")
	}
}

func TestT_MissingKey(t *testing.T) {
	got := T("nonexistent.key")
	if got != "nonexistent.key" {
		t.Errorf("T('nonexistent.key') = %q, want 'nonexistent.key'", got)
	}
}

func TestT_WithArgs(t *testing.T) {
	SetLang(EN)
	got := T("err.start_failed", "connection refused")
	if !strings.Contains(got, "connection refused") {
		t.Errorf("T with args should contain the arg, got %q", got)
	}
}

func TestT_WithMultipleArgs(t *testing.T) {
	SetLang(EN)
	got := T("warn.read_file", "foo.txt", "no such file")
	if !strings.Contains(got, "foo.txt") || !strings.Contains(got, "no such file") {
		t.Errorf("T with multiple args should contain both, got %q", got)
	}
}

func TestT_NoArgsWhenTemplateExpectsNone(t *testing.T) {
	got := T("cli.usage")
	if got == "" {
		t.Error("T('cli.usage') returned empty")
	}
}

func TestT_LanguageSwitch(t *testing.T) {
	old := GetLang()
	defer SetLang(old)

	SetLang(ZH)
	zhVal := T("cli.usage")

	SetLang(EN)
	enVal := T("cli.usage")

	if zhVal == enVal {
		t.Log("zh and en translations may differ, but they can be the same")
	}

	if zhVal == "" || enVal == "" {
		t.Error("both translations should be non-empty")
	}
}

func TestT_AllKeysPresent(t *testing.T) {
	// Every key defined in ZH should also exist in EN
	for key := range messages[ZH] {
		if _, ok := messages[EN][key]; !ok {
			t.Errorf("key %q exists in ZH but missing in EN", key)
		}
	}
	for key := range messages[EN] {
		if _, ok := messages[ZH][key]; !ok {
			t.Errorf("key %q exists in EN but missing in ZH", key)
		}
	}
}

func TestT_NewKeys(t *testing.T) {
	// Verify newly added keys
	SetLang(EN)

	cases := []string{
		"cli.version",
		"sess.list_format",
		"update.title",
		"update.script_not_found",
		"update.failed",
		"update.complete",
		"git.auto_commit_msg",
	}
	for _, key := range cases {
		got := T(key)
		if got == "" || got == key {
			t.Errorf("T(%q) = %q, expected non-empty translation", key, got)
		}
	}

	SetLang(ZH)
	for _, key := range cases {
		got := T(key)
		if got == "" || got == key {
			t.Errorf("T(%q) in ZH = %q, expected non-empty translation", key, got)
		}
	}
}
