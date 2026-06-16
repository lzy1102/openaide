package main

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
		// Serve index.html for SPA routes (client-side routing handles /chat, /settings, etc.)
		if r.URL.Path == "/" || (!strings.HasPrefix(r.URL.Path, "/assets/") && !strings.Contains(r.URL.Path, ".")) {
			f, err := subFS.Open("index.html")
			if err == nil {
				f.Close()
				r.URL.Path = "/index.html"
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
