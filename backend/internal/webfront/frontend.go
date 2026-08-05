// Package webfront provides the embedded web frontend shared by all
// entrypoints (CLI, server). The frontend assets live in ./frontend and
// are copied there by the Makefile before building.
package webfront

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
)

//go:embed frontend/*
var frontendFS embed.FS

// FrontendHandler returns an HTTP handler that serves the embedded frontend.
// Returns nil if the frontend was not embedded (development builds).
func FrontendHandler() http.Handler {
	subFS, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		slog.Debug("embedded frontend not available — dev build without frontend copy")
		return nil
	}

	// Verify index.html exists
	if _, err := fs.Stat(subFS, "index.html"); err != nil {
		slog.Debug("embedded frontend missing index.html")
		return nil
	}

	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: routes without a file extension (client-side routing
		// handles /chat, /settings, etc.) are rewritten to "/", letting
		// FileServer serve index.html directly. Rewriting to "/index.html"
		// instead would make FileServer 301-redirect back to "./".
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") && !strings.Contains(r.URL.Path, ".") {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
