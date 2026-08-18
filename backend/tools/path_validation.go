package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validatePath resolves a raw path to an absolute, cleaned path.
// Rejects empty paths. Does not check existence (safe for both read and write).
func validatePath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", raw, err)
	}
	return filepath.Clean(abs), nil
}

// resolveSafePath resolves symlinks and rejects path traversal.
// It ensures the real path (after symlink resolution) stays within the
// same directory tree as the original cleaned path.
// If the file doesn't exist (write target), validates the parent directory.
func resolveSafePath(cleanedPath string) (string, error) {
	realPath, err := filepath.EvalSymlinks(cleanedPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — validate the parent directory instead.
			parent := filepath.Dir(cleanedPath)
			realParent, err2 := filepath.EvalSymlinks(parent)
			if err2 != nil {
				// Parent doesn't exist either; let the caller handle it.
				return cleanedPath, nil
			}
			if !isSubPath(filepath.Dir(cleanedPath), realParent) {
				return "", fmt.Errorf("symlink traversal: parent %s resolves to %s",
					parent, realParent)
			}
			return cleanedPath, nil
		}
		return "", fmt.Errorf("cannot resolve path: %w", err)
	}
	// Reject if symlink escapes the parent directory.
	if !isSubPath(filepath.Dir(cleanedPath), realPath) {
		return "", fmt.Errorf("symlink traversal: %s resolves outside its parent directory to %s",
			cleanedPath, realPath)
	}
	return realPath, nil
}

// isSubPath checks whether child is located under parent.
// Both paths should be absolute and cleaned.
func isSubPath(parent, child string) bool {
	parent = filepath.Clean(parent)
	child = filepath.Clean(child)
	if parent == child {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}
