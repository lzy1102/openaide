package webfront

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFrontendHandlerServesIndex(t *testing.T) {
	h := FrontendHandler()
	if h == nil {
		t.Fatal("FrontendHandler() returned nil — frontend not embedded")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "<!DOCTYPE html") {
		t.Errorf("GET / body does not look like index.html")
	}
}

func TestFrontendHandlerSpaFallback(t *testing.T) {
	h := FrontendHandler()
	if h == nil {
		t.Fatal("FrontendHandler() returned nil — frontend not embedded")
	}

	// SPA routes without a file extension must fall back to index.html
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /settings = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("GET /settings Content-Type = %q, want text/html", ct)
	}
}

func TestFrontendHandlerServesAssets(t *testing.T) {
	h := FrontendHandler()
	if h == nil {
		t.Fatal("FrontendHandler() returned nil — frontend not embedded")
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/src/utils/i18n.js", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /src/utils/i18n.js = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "internationalization") && !strings.Contains(string(body), "i18n") && !strings.Contains(string(body), "translations") {
		t.Errorf("GET asset body does not look like i18n.js")
	}
}
