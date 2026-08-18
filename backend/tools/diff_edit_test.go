package tools

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseSearchReplaceBlocks_Single(t *testing.T) {
	content := `<<<<<<< SEARCH
old code
=======
new code
>>>>>>> REPLACE`
	blocks := parseSearchReplaceBlocks(content)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].Search != "old code" {
		t.Errorf("expected 'old code', got %q", blocks[0].Search)
	}
	if blocks[0].Replace != "new code" {
		t.Errorf("expected 'new code', got %q", blocks[0].Replace)
	}
}

func TestParseSearchReplaceBlocks_Multiple(t *testing.T) {
	content := `<<<<<<< SEARCH
foo
=======
bar
>>>>>>> REPLACE
some text
<<<<<<< SEARCH
baz
=======
qux
>>>>>>> REPLACE`
	blocks := parseSearchReplaceBlocks(content)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
}

func TestParseSearchReplaceBlocks_Empty(t *testing.T) {
	blocks := parseSearchReplaceBlocks("no blocks here")
	if len(blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(blocks))
	}
}

func TestApplySearchReplacePatch(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/test.txt"
	os.WriteFile(f, []byte("line1\nold line\nline3\n"), 0644)

	patch := `<<<<<<< SEARCH
old line
=======
new line
>>>>>>> REPLACE`
	result, err := applySearchReplacePatch(f, patch)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "1 SEARCH/REPLACE") {
		t.Error("expected success message")
	}

	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "new line") {
		t.Error("file should contain new line")
	}
	if strings.Contains(string(data), "old line") {
		t.Error("file should not contain old line")
	}
}

func TestApplySearchReplacePatch_NoMatch(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/test.txt"
	os.WriteFile(f, []byte("hello world\n"), 0644)

	patch := `<<<<<<< SEARCH
nonexistent
=======
replacement
>>>>>>> REPLACE`
	_, err := applySearchReplacePatch(f, patch)
	if err == nil {
		t.Error("expected error for no matching search")
	}
}

func TestHandleApplyPatch(t *testing.T) {
	dir := t.TempDir()
	f := dir + "/test.txt"
	os.WriteFile(f, []byte("line1\nold line\nline3\n"), 0644)

	ctx := context.Background()
	patch := `<<<<<<< SEARCH
old line
=======
new line
>>>>>>> REPLACE`

	t.Run("success", func(t *testing.T) {
		arg, _ := json.Marshal(map[string]string{"path": f, "content": patch})
		r, _ := handleApplyPatch(ctx, string(arg))
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		if !strings.Contains(r.Content.(string), "1 SEARCH/REPLACE") {
			t.Errorf("content = %q, want success message", r.Content)
		}
		data, _ := os.ReadFile(f)
		if !strings.Contains(string(data), "new line") {
			t.Error("file should contain new line")
		}
	})

	t.Run("missing_args", func(t *testing.T) {
		r, _ := handleApplyPatch(ctx, `{}`)
		if !strings.Contains(r.Error, "path and content are required") {
			t.Errorf("error = %q, want 'path and content are required'", r.Error)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		r, _ := handleApplyPatch(ctx, `{oops`)
		if r.Error == "" {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		r, _ := handleApplyPatch(ctx, `{"path":"`+dir+`/nope.txt","content":"`+patch+`"}`)
		if r.Error == "" {
			t.Error("expected error for nonexistent file")
		}
	})
}
