package lsp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLSPGoConnect(t *testing.T) {
	root := "/mnt/d/project/android/openaide/backend"
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) {
		t.Skip("go.mod not found")
	}
	c, err := Start(root, "go")
	if err != nil {
		t.Skip("gopls not available:", err)
	}
	defer c.Close()

	// Hover over a known location (package declaration)
	hover, err := c.Hover(filepath.Join(root, "cmd/cli/main.go"), 0, 8)
	if err != nil {
		t.Error("Hover:", err)
	} else {
		t.Logf("LSP Hover: %s", hover.Contents.Value)
	}
}
