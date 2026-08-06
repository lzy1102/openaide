package tools

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestHTTPServer returns a local HTTP server serving body with status 200.
func newTestHTTPServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(body))
	}))
	t.Cleanup(ts.Close)
	return ts
}
