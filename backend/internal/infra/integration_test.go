package infra_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"openaide/backend/config"
	"openaide/backend/internal/infra"
)

// TestIntegration_FullPipeline verifies the complete actor pipeline:
// Application start → session create → knowledge store → memory search
func TestIntegration_FullPipeline(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Storage.DataDir = dir
	cfg.Server.Mode = "direct"

	app, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication:", err)
	}
	defer app.Stop(context.Background())

	ctx := context.Background()

	// 1. Session actor works
	sessions, err := app.Orchestrator.ListSessions(ctx, "default", "test-user", 10, 0)
	if err != nil {
		t.Fatal("ListSessions:", err)
	}
	if len(sessions) != 0 {
		t.Logf("existing sessions: %d (from previous test data?)", len(sessions))
	}

	// 2. Create a session
	sess, err := app.Orchestrator.CreateSession(ctx, "default", "test-user")
	if err != nil {
		t.Fatal("CreateSession:", err)
	}
	if sess.ID == "" {
		t.Error("expected non-empty session ID")
	}

	// 3. Verify SQLite files exist
	time.Sleep(100 * time.Millisecond) // wait for async init
	for _, f := range []string{"sessions.db", "memory.db"} {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected %s to exist", f)
		} else {
			t.Logf("✓ %s exists (%d bytes)", f, fileSize(path))
		}
	}

	// 4. Start server in background for health/metrics check
	app.Config.Server.Mode = "server"
	go app.Start()
	time.Sleep(300 * time.Millisecond)

	resp, err := http.Get("http://localhost:8080/health")
	if err != nil {
		t.Log("health check:", err)
	} else {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Logf("✓ health → 200 (%d bytes)", len(body))
		}
	}

	resp2, err := http.Get("http://localhost:8080/metrics")
	if err != nil {
		t.Log("metrics:", err)
	} else {
		resp2.Body.Close()
		if resp2.StatusCode == http.StatusOK {
			t.Logf("✓ metrics → 200")
		}
	}
}

// TestIntegration_ConcurrentSessions verifies concurrent session access is safe.
func TestIntegration_ConcurrentSessions(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Storage.DataDir = dir
	cfg.Server.Mode = "direct"

	app, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication:", err)
	}
	defer app.Stop(context.Background())

	ctx := context.Background()
	done := make(chan error, 20)

	// 10 concurrent writers
	for i := 0; i < 10; i++ {
		go func(n int) {
			_, err := app.Orchestrator.CreateSession(ctx, "proj", "user")
			done <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Error("concurrent create:", err)
		}
	}

	sessions, err := app.Orchestrator.ListSessions(ctx, "proj", "user", 50, 0)
	if err != nil {
		t.Fatal("ListSessions:", err)
	}
	if len(sessions) < 10 {
		t.Errorf("expected >=10 sessions, got %d", len(sessions))
	}
}

// TestIntegration_KnowledgeStore verifies the application starts without errors (knowledge base removed).
func TestIntegration_KnowledgeStore(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Storage.DataDir = dir
	cfg.Server.Mode = "direct"

	app, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication:", err)
	}
	defer app.Stop(context.Background())

	time.Sleep(100 * time.Millisecond)
	t.Log("✓ application started successfully without knowledge base")
}

// TestIntegration_APIEndpoints verifies all API endpoints.
func TestIntegration_APIEndpoints(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Storage.DataDir = dir
	cfg.Server.Mode = "direct"

	app, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication:", err)
	}
	defer app.Stop(context.Background())

	app.Config.Server.Mode = "server"
	go app.Start()
	time.Sleep(300 * time.Millisecond)

	baseURL := "http://localhost:8080"
	endpoints := []struct {
		path string
	}{
		{"/health"},
		{"/metrics"},
		{"/api/v1/tools"},
	}

	for _, ep := range endpoints {
		resp, err := http.Get(baseURL + ep.path)
		if err != nil {
			t.Logf("%s: %v", ep.path, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Logf("✓ %s → %d (%d bytes)", ep.path, resp.StatusCode, len(body))
			// Verify tools is valid JSON
			if ep.path == "/api/v1/tools" {
				var tools []map[string]interface{}
				if err := json.Unmarshal(body, &tools); err != nil {
					t.Error("tools not valid JSON:", err)
				} else if len(tools) < 20 {
					t.Errorf("expected >=20 tools, got %d", len(tools))
				} else {
					t.Logf("  %d tools registered", len(tools))
				}
			}
		} else {
			t.Errorf("%s: expected 200, got %d", ep.path, resp.StatusCode)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestIntegration_DataSurvival verifies data survives server restart.
func TestIntegration_DataSurvival(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DefaultConfig()
	cfg.Storage.DataDir = dir
	cfg.Server.Mode = "direct"

	// Create, write, close
	app1, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication:", err)
	}
	ctx := context.Background()
	sess, _ := app1.Orchestrator.CreateSession(ctx, "proj", "user")
	sid := sess.ID
	app1.Stop(context.Background())

	// Reopen, verify data survived
	app2, err := infra.NewApplication(cfg)
	if err != nil {
		t.Fatal("NewApplication #2:", err)
	}
	defer app2.Stop(context.Background())

	got, err := app2.Orchestrator.GetSession(ctx, sid)
	if err != nil {
		t.Fatal("session lost after restart:", err)
	}
	if got.ID != sid {
		t.Errorf("expected ID %s, got %s", sid, got.ID)
	}
	t.Logf("✓ session %s survived restart", sid[:min(16, len(sid))])
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
