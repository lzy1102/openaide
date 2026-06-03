package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSanitizeParam(t *testing.T) {
	tests := []struct{ in, want string }{
		{"hello", "hello"},
		{"../etc/passwd", "_etc_passwd"},
		{"/etc/passwd", "_etc_passwd"},
		{"./config", "._config"},
		{"a/b/c", "a_b_c"},
	}
	for _, tt := range tests {
		got := sanitizeParam(tt.in)
		if got != tt.want {
			t.Errorf("sanitizeParam(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWithCORS(t *testing.T) {
	s := &Server{}
	handler := s.withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS * header")
	}
}

func TestWriteJSON(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.writeJSON(w, 200, map[string]string{"status": "ok"})

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
}

func TestWriteError(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.writeError(w, 400, http.ErrBodyNotAllowed)

	if w.Code != 400 {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	if w.Code != 200 || !stringsContains(w.Body.String(), "healthy") {
		t.Errorf("health check failed: code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestWithCORS_Options(t *testing.T) {
	s := &Server{}
	handler := s.withCORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("OPTIONS should return 200, got %d", w.Code)
	}
}

func stringsContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
