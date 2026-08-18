package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeTestPNG writes a tiny valid PNG to dir and returns its path.
func makeTestPNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, image.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHandleReadImage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pngPath := makeTestPNG(t, dir)

	t.Run("success", func(t *testing.T) {
		r, _ := handleReadImage(ctx, `{"path":"`+pngPath+`"}`)
		if r.Error != "" {
			t.Fatalf("unexpected error: %v", r.Error)
		}
		content, ok := r.Content.(string)
		if !ok || !strings.HasPrefix(content, "data:image/png;base64,") {
			t.Fatalf("content = %q, want data:image/png;base64, prefix", r.Content)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(content, "data:image/png;base64,"))
		if err != nil {
			t.Fatalf("base64 decode failed: %v", err)
		}
		if len(decoded) == 0 {
			t.Error("decoded image is empty")
		}
	})

	t.Run("missing_path", func(t *testing.T) {
		r, _ := handleReadImage(ctx, `{}`)
		if !strings.Contains(r.Error, "path is required") {
			t.Errorf("error = %q, want 'path is required'", r.Error)
		}
	})

	t.Run("bad_json", func(t *testing.T) {
		r, _ := handleReadImage(ctx, `{oops`)
		if r.Error == "" {
			t.Error("expected error for invalid JSON")
		}
	})

	t.Run("unsupported_format", func(t *testing.T) {
		f := filepath.Join(dir, "notes.txt")
		os.WriteFile(f, []byte("hello"), 0o644)
		r, _ := handleReadImage(ctx, `{"path":"`+f+`"}`)
		if !strings.Contains(r.Error, "unsupported format") {
			t.Errorf("error = %q, want 'unsupported format'", r.Error)
		}
	})

	t.Run("nonexistent_file", func(t *testing.T) {
		r, _ := handleReadImage(ctx, `{"path":"`+filepath.Join(dir, "nope.png")+`"}`)
		if r.Error == "" {
			t.Error("expected error for nonexistent file")
		}
	})
}
