package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openaide/backend/core"
	"openaide/backend/core/actor"
)

// mockProvider implements the llm.Provider interface with scripted responses.
type mockProvider struct {
	modelID      string
	chatResp     *kernel.LLMResponse
	chatErr      error
	streamChunks []kernel.StreamChunk
	streamErr    error
	healthErr    error
	chatCalls    int
}

func (m *mockProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	m.chatCalls++
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return m.chatResp, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	ch := make(chan kernel.StreamChunk, len(m.streamChunks)+1)
	for _, c := range m.streamChunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (m *mockProvider) GetModelID() string { return m.modelID }
func (m *mockProvider) SetModelID(model string) {
	m.modelID = model
}
func (m *mockProvider) HealthCheck(ctx context.Context) error { return m.healthErr }

// ============ router.go ============

func TestRouter_Route_MatchByPriority(t *testing.T) {
	r := NewRouter([]RouteRule{
		{Name: "low", Pattern: ".*", Provider: "p-low", Priority: 1},
		{Name: "high", Pattern: ".*golang.*", Provider: "p-high", Priority: 10},
	})
	provider, model, matched := r.Route("fix golang bug")
	if !matched {
		t.Fatal("expected match")
	}
	if provider != "p-high" {
		t.Errorf("expected p-high (higher priority), got %q", provider)
	}
	if model != "" {
		t.Errorf("expected empty model, got %q", model)
	}
}

func TestRouter_Route_ModelOverride(t *testing.T) {
	r := NewRouter([]RouteRule{
		{Name: "go", Pattern: "golang", Provider: "p1", Model: "gpt-4o", Priority: 5},
	})
	provider, model, matched := r.Route("golang task")
	if !matched || provider != "p1" || model != "gpt-4o" {
		t.Errorf("got (%q, %q, %v)", provider, model, matched)
	}
}

func TestRouter_Route_NoMatch(t *testing.T) {
	r := NewRouter([]RouteRule{{Name: "x", Pattern: "zzz", Provider: "p1", Priority: 1}})
	p, m, ok := r.Route("hello")
	if ok || p != "" || m != "" {
		t.Errorf("expected no match, got (%q, %q, %v)", p, m, ok)
	}
}

func TestRouter_Route_InvalidRegex(t *testing.T) {
	r := NewRouter([]RouteRule{{Name: "bad", Pattern: "[invalid", Provider: "p1", Priority: 1}})
	// Invalid regex → matcher is nil → skipped, no match, no panic
	p, _, ok := r.Route("anything")
	if ok || p != "" {
		t.Errorf("expected no match for invalid regex, got (%q, %v)", p, ok)
	}
}

func TestRouter_Route_Nil(t *testing.T) {
	var r *Router
	p, m, ok := r.Route("query")
	if ok || p != "" || m != "" {
		t.Errorf("nil router should return no match, got (%q, %q, %v)", p, m, ok)
	}
}

func TestDefaultRouter(t *testing.T) {
	r := DefaultRouter()
	if r == nil || len(r.rules) != 0 {
		t.Error("expected empty default router")
	}
	p, _, ok := r.Route("anything")
	if ok || p != "" {
		t.Error("default router should never match")
	}
}

func TestIsComplexQuery(t *testing.T) {
	if isComplexQuery("short") {
		t.Error("short query should not be complex")
	}
	long := strings.Repeat("x", 201)
	if !isComplexQuery(long) {
		t.Error("long query should be complex")
	}
}

// ============ gateway.go: provider registration ============

func TestGateway_RegisterAndAccessors(t *testing.T) {
	g := NewGateway()
	cfg := &ProviderConfig{Name: "test", Enabled: true, DefaultModel: "m1"}
	prov := &mockProvider{modelID: "m1"}
	g.RegisterProvider("test", prov, cfg)

	if g.GetDefaultProvider() != "test" {
		t.Errorf("expected default provider test, got %q", g.GetDefaultProvider())
	}
	names := g.GetProviders()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("expected [test], got %v", names)
	}
	infos := g.GetProviderInfos()
	if len(infos) != 1 || infos[0].Name != "test" || !infos[0].Default {
		t.Errorf("unexpected infos: %+v", infos)
	}
	enabled := g.GetEnabledProviders()
	if len(enabled) != 1 || enabled[0] != "test" {
		t.Errorf("expected [test] enabled, got %v", enabled)
	}
}

func TestGateway_DefaultProvider_FirstEnabledWins(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{modelID: "a1"}, &ProviderConfig{Name: "a", Enabled: true})
	g.RegisterProvider("b", &mockProvider{modelID: "b1"}, &ProviderConfig{Name: "b", Enabled: true})
	if g.GetDefaultProvider() != "a" {
		t.Errorf("expected first enabled provider a, got %q", g.GetDefaultProvider())
	}
}

func TestGateway_SetDefaultProvider(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{modelID: "a1"}, &ProviderConfig{Name: "a"})
	g.RegisterProvider("b", &mockProvider{modelID: "b1"}, &ProviderConfig{Name: "b"})
	if err := g.SetDefaultProvider("b"); err != nil {
		t.Fatalf("SetDefaultProvider failed: %v", err)
	}
	if g.GetDefaultProvider() != "b" {
		t.Errorf("expected b, got %q", g.GetDefaultProvider())
	}
	if err := g.SetDefaultProvider("nonexistent"); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGateway_ReloadConfig(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{modelID: "old"}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a", DefaultModel: "old"})
	g.ReloadConfig(map[string]string{"a": "new-model"}, "reasoning-new", "execution-new")
	if prov.modelID != "new-model" {
		t.Errorf("expected provider model updated to new-model, got %q", prov.modelID)
	}
	if g.ReasoningModel != "reasoning-new" || g.ExecutionModel != "execution-new" {
		t.Errorf("routing models not updated: %q %q", g.ReasoningModel, g.ExecutionModel)
	}
	// Empty updates should be ignored
	g.ReloadConfig(map[string]string{}, "", "")
	if g.ReasoningModel != "reasoning-new" {
		t.Error("empty reasoning update should be ignored")
	}
}

func TestGateway_SetModelID_GetModelID(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{modelID: "m1"}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a", DefaultModel: "m1", Enabled: true})
	if g.GetModelID() != "m1" {
		t.Errorf("expected m1, got %q", g.GetModelID())
	}
	g.SetModelID("m2")
	if prov.modelID != "m2" {
		t.Errorf("expected provider model m2, got %q", prov.modelID)
	}
	if g.GetModelID() != "m2" {
		t.Errorf("expected GetModelID m2, got %q", g.GetModelID())
	}
}

func TestGateway_GetModelID_NoProvider(t *testing.T) {
	g := NewGateway()
	if g.GetModelID() != "" {
		t.Error("expected empty model id with no providers")
	}
}

// ============ gateway.go: routing ============

func TestGateway_findProviderForModel(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{modelID: "model-a"}, &ProviderConfig{Name: "a"})
	g.RegisterProvider("b", &mockProvider{modelID: "model-b"}, &ProviderConfig{Name: "b"})
	if got := g.findProviderForModel("model-b"); got != "b" {
		t.Errorf("expected b, got %q", got)
	}
	if got := g.findProviderForModel("nonexistent"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestGateway_routeProvider_RouterHit(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{modelID: "m1"}, &ProviderConfig{Name: "a", Enabled: true})
	g.RegisterProvider("b", &mockProvider{modelID: "m2"}, &ProviderConfig{Name: "b"})
	r := NewRouter([]RouteRule{{Name: "r", Pattern: "golang", Provider: "b", Priority: 1}})
	g.SetRouter(r)
	if got := g.routeProvider("fix golang bug"); got != "b" {
		t.Errorf("expected b via router, got %q", got)
	}
	if got := g.routeProvider("something else"); got != "a" {
		t.Errorf("expected default a, got %q", got)
	}
}

func TestGateway_routeProvider_RouterUnknownProvider(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{modelID: "m1"}, &ProviderConfig{Name: "a", Enabled: true})
	r := NewRouter([]RouteRule{{Name: "r", Pattern: "golang", Provider: "ghost", Priority: 1}})
	g.SetRouter(r)
	// Router says ghost but ghost isn't registered → falls back to default
	if got := g.routeProvider("golang task"); got != "a" {
		t.Errorf("expected fallback to a, got %q", got)
	}
}

// ============ gateway.go: Chat with routing ============

func TestGateway_Chat_NoProvider(t *testing.T) {
	g := NewGateway()
	_, err := g.Chat(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Fatal("expected error with no providers")
	}
	if !strings.Contains(err.Error(), "no provider configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGateway_Chat_ExecutionRoute(t *testing.T) {
	g := NewGateway()
	execProv := &mockProvider{modelID: "flash", chatResp: &kernel.LLMResponse{Content: "exec"}}
	reasonProv := &mockProvider{modelID: "pro", chatResp: &kernel.LLMResponse{Content: "reason"}}
	g.RegisterProvider("exec", execProv, &ProviderConfig{Name: "exec", Enabled: true})
	g.RegisterProvider("reason", reasonProv, &ProviderConfig{Name: "reason", Enabled: true})
	g.ExecutionModel = "flash"
	g.ReasoningModel = "pro"

	resp, err := g.Chat(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, map[string]interface{}{"route": "execution"})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "exec" {
		t.Errorf("expected exec response, got %q", resp.Content)
	}
	if execProv.chatCalls != 1 {
		t.Errorf("expected exec provider called once, got %d", execProv.chatCalls)
	}
	if reasonProv.chatCalls != 0 {
		t.Errorf("expected reason provider not called, got %d", reasonProv.chatCalls)
	}
}

func TestGateway_Chat_ReasoningRoute_FallbackModel(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{modelID: "m1", chatResp: &kernel.LLMResponse{Content: "ok"}}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a", Enabled: true})
	// ReasoningModel "pro" has no matching provider → options["model"] set instead
	g.ReasoningModel = "pro"

	_, err := g.Chat(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, map[string]interface{}{"route": "reasoning"})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	// Provider model was temporarily set to "pro" then restored
	if prov.modelID != "m1" {
		t.Errorf("expected model restored to m1, got %q", prov.modelID)
	}
}

func TestGateway_ChatWithProvider_NotFound(t *testing.T) {
	g := NewGateway()
	_, err := g.ChatWithProvider(context.Background(), "ghost", nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "provider not found") {
		t.Errorf("expected provider not found, got %v", err)
	}
}

func TestGateway_ChatWithProvider_RetryOnError(t *testing.T) {
	g := NewGateway()
	failures := 2
	prov := &mockProvider{
		chatErr:  fmt.Errorf("transient error"),
		chatResp: &kernel.LLMResponse{Content: "retried"},
	}
	// Custom provider that fails twice then succeeds
	orig := prov.chatErr
	_ = orig
	prov.chatErr = nil
	// Use a wrapper to simulate transient failures
	failing := &failingProvider{inner: prov, failCount: failures}
	g.RegisterProvider("a", failing, &ProviderConfig{Name: "a"})

	resp, err := g.ChatWithProvider(context.Background(), "a", []kernel.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("ChatWithProvider failed: %v", err)
	}
	if resp.Content != "retried" {
		t.Errorf("expected retried response, got %q", resp.Content)
	}
	if failing.calls != 3 {
		t.Errorf("expected 3 calls (2 fail + 1 success), got %d", failing.calls)
	}
}

// failingProvider fails the first N calls then delegates.
type failingProvider struct {
	inner     *mockProvider
	failCount int
	calls     int
}

func (f *failingProvider) Chat(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (*kernel.LLMResponse, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, fmt.Errorf("transient failure %d", f.calls)
	}
	return f.inner.Chat(ctx, messages, tools, options)
}
func (f *failingProvider) ChatStream(ctx context.Context, messages []kernel.Message, tools []kernel.ToolDefinition, options map[string]interface{}) (<-chan kernel.StreamChunk, error) {
	f.calls++
	if f.calls <= f.failCount {
		return nil, fmt.Errorf("transient stream failure %d", f.calls)
	}
	return f.inner.ChatStream(ctx, messages, tools, options)
}
func (f *failingProvider) GetModelID() string { return f.inner.GetModelID() }
func (f *failingProvider) SetModelID(m string) {
	f.inner.SetModelID(m)
}
func (f *failingProvider) HealthCheck(ctx context.Context) error { return f.inner.HealthCheck(ctx) }

func TestGateway_ChatWithProvider_AllRetriesFail(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{chatErr: fmt.Errorf("permanent")}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a"})
	_, err := g.ChatWithProvider(context.Background(), "a", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if prov.chatCalls != 3 {
		t.Errorf("expected 3 retry attempts, got %d", prov.chatCalls)
	}
}

// ============ gateway.go: cache integration ============

func TestGateway_Chat_UsesCache(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{modelID: "m1", chatResp: &kernel.LLMResponse{Content: "cached-answer"}}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a", Enabled: true})
	cache := NewPromptCache("")
	defer cache.Shutdown()
	g.SetPromptCache(cache)

	msgs := []kernel.Message{{Role: "user", Content: "same query"}}
	resp1, err := g.Chat(context.Background(), msgs, nil, nil)
	if err != nil {
		t.Fatalf("first Chat failed: %v", err)
	}
	resp2, err := g.Chat(context.Background(), msgs, nil, nil)
	if err != nil {
		t.Fatalf("second Chat failed: %v", err)
	}
	if resp2.Content != "cached-answer" {
		t.Errorf("unexpected content: %q", resp2.Content)
	}
	if prov.chatCalls != 1 {
		t.Errorf("expected 1 provider call (second from cache), got %d", prov.chatCalls)
	}
	if resp1.Content != resp2.Content {
		t.Error("expected identical content from cache")
	}
}

func TestGateway_ChatStream_NoProvider(t *testing.T) {
	g := NewGateway()
	_, err := g.ChatStream(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err == nil {
		t.Error("expected error with no providers")
	}
}

func TestGateway_ChatStreamWithProvider_NotFound(t *testing.T) {
	g := NewGateway()
	_, err := g.ChatStreamWithProvider(context.Background(), "ghost", nil, nil, nil)
	if err == nil {
		t.Error("expected provider not found")
	}
}

func TestGateway_ChatStreamWithProvider_Success(t *testing.T) {
	g := NewGateway()
	prov := &mockProvider{
		streamChunks: []kernel.StreamChunk{{Content: "streamed"}, {Done: true}},
	}
	g.RegisterProvider("a", prov, &ProviderConfig{Name: "a"})
	ch, err := g.ChatStreamWithProvider(context.Background(), "a", nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStreamWithProvider failed: %v", err)
	}
	var content strings.Builder
	done := false
	for c := range ch {
		content.WriteString(c.Content)
		if c.Done {
			done = true
		}
	}
	if content.String() != "streamed" || !done {
		t.Errorf("got content=%q done=%v", content.String(), done)
	}
}

func TestGateway_ChatStreamWithProvider_Retry(t *testing.T) {
	g := NewGateway()
	inner := &mockProvider{streamChunks: []kernel.StreamChunk{{Content: "ok"}, {Done: true}}}
	failing := &failingProvider{inner: inner, failCount: 1}
	g.RegisterProvider("a", failing, &ProviderConfig{Name: "a"})
	ch, err := g.ChatStreamWithProvider(context.Background(), "a", nil, nil, nil)
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	for range ch {
	}
	if failing.calls != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", failing.calls)
	}
}

// ============ gateway.go: FallbackChat / HealthCheck ============

func TestGateway_FallbackChat_NoProviders(t *testing.T) {
	g := NewGateway()
	_, err := g.FallbackChat(context.Background(), nil, nil, nil)
	if err == nil {
		t.Error("expected error with no enabled providers")
	}
}

func TestGateway_FallbackChat_FailsOver(t *testing.T) {
	g := NewGateway()
	bad := &mockProvider{chatErr: fmt.Errorf("bad provider")}
	good := &mockProvider{chatResp: &kernel.LLMResponse{Content: "from-good"}}
	g.RegisterProvider("bad", bad, &ProviderConfig{Name: "bad", Enabled: true})
	g.RegisterProvider("good", good, &ProviderConfig{Name: "good", Enabled: true})

	resp, err := g.FallbackChat(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("FallbackChat failed: %v", err)
	}
	if resp.Content != "from-good" {
		t.Errorf("expected from-good, got %q", resp.Content)
	}
}

func TestGateway_FallbackChat_AllFail(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("a", &mockProvider{chatErr: fmt.Errorf("fail-a")}, &ProviderConfig{Name: "a", Enabled: true})
	g.RegisterProvider("b", &mockProvider{chatErr: fmt.Errorf("fail-b")}, &ProviderConfig{Name: "b", Enabled: true})
	_, err := g.FallbackChat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "all providers failed") {
		t.Errorf("expected all providers failed, got %v", err)
	}
}

func TestGateway_HealthCheck(t *testing.T) {
	g := NewGateway()
	g.RegisterProvider("ok", &mockProvider{healthErr: nil}, &ProviderConfig{Name: "ok"})
	g.RegisterProvider("bad", &mockProvider{healthErr: fmt.Errorf("down")}, &ProviderConfig{Name: "bad"})
	results := g.HealthCheck(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results["ok"] != nil {
		t.Errorf("expected ok provider healthy, got %v", results["ok"])
	}
	if results["bad"] == nil {
		t.Error("expected bad provider to report error")
	}
}

func TestGateway_Shutdown(t *testing.T) {
	g := NewGateway()
	g.SetPromptCache(NewPromptCache(""))
	g.Shutdown() // must not panic
}

// ============ gateway.go: humanizeError ============

func TestHumanizeError(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"http error 429: too many requests", "余额不足"},
		{"http error 401: unauthorized", "API Key 无效"},
		{"http error 404: not found", "模型或接口不存在"},
		{"request timeout after 30s", "网络超时"},
		{"context deadline exceeded", "网络超时"},
		{"plain error", ""},
	}
	for _, tt := range tests {
		err := humanizeError(fmt.Errorf("%s", tt.msg))
		if tt.want == "" {
			if err == nil || !strings.Contains(err.Error(), tt.msg) {
				t.Errorf("expected original error preserved, got %v", err)
			}
			continue
		}
		if !strings.Contains(err.Error(), tt.want) {
			t.Errorf("humanizeError(%q) should contain %q, got: %v", tt.msg, tt.want, err)
		}
	}
}

func TestHumanizeError_Nil(t *testing.T) {
	if humanizeError(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestContainsAny(t *testing.T) {
	if !containsAny("Rate limit exceeded", "429", "rate limit") {
		t.Error("expected match for rate limit")
	}
	if containsAny("hello world", "zzz") {
		t.Error("expected no match")
	}
	if !containsAny("UNAUTHORIZED REQUEST", "unauthorized") {
		t.Error("expected case-insensitive match")
	}
}

// ============ cache.go: cleanup / eviction ============

func TestPromptCache_Cleanup(t *testing.T) {
	c := NewPromptCache("")
	defer c.Shutdown()
	msgs := []kernel.Message{{Role: "user", Content: "q"}}
	c.Set(msgs, nil, "m1", &kernel.LLMResponse{Content: "a"})
	// Manually age the entry past the cutoff
	var entry *cacheEntry
	c.entries.Range(func(k string, e *cacheEntry) bool {
		entry = e
		return false
	})
	if entry == nil {
		t.Fatal("expected entry")
	}
	entry.CreatedAt = time.Now().Add(-3 * time.Hour)
	c.cleanup(2 * time.Hour)
	if c.entries.Len() != 0 {
		t.Errorf("expected cache emptied, got %d entries", c.entries.Len())
	}
}

func TestPromptCache_Eviction(t *testing.T) {
	c := &PromptCache{
		entries: actor.NewSafeMap[string, *cacheEntry](64),
		maxSize: 10,
		stopCh:  make(chan struct{}),
	}
	defer close(c.stopCh)

	// Seed 3 stale entries (older than the TTL cutoff) so eviction has targets.
	for i := 0; i < 3; i++ {
		key := fmt.Sprintf("stale-%d", i)
		c.entries.Store(key, &cacheEntry{
			Response:  &kernel.LLMResponse{Content: "stale"},
			CreatedAt: time.Now().Add(-2 * time.Hour),
		})
	}
	// Fill the cache up to maxSize with fresh entries.
	for i := 0; i < 7; i++ {
		msgs := []kernel.Message{{Role: "user", Content: fmt.Sprintf("query-%d", i)}}
		c.Set(msgs, nil, "m", &kernel.LLMResponse{Content: fmt.Sprintf("r%d", i)})
	}
	if c.entries.Len() != 10 {
		t.Fatalf("expected 10 entries before overflow, got %d", c.entries.Len())
	}
	// One more insert pushes past maxSize → eviction removes stale entries.
	msgs := []kernel.Message{{Role: "user", Content: "overflow"}}
	c.Set(msgs, nil, "m", &kernel.LLMResponse{Content: "overflow"})
	if c.entries.Len() > 10 {
		t.Errorf("expected cache capped at 10, got %d", c.entries.Len())
	}
	// At least one stale entry must be gone.
	staleLeft := 0
	c.entries.Range(func(k string, e *cacheEntry) bool {
		if e.CreatedAt.Before(time.Now().Add(-1 * time.Hour)) {
			staleLeft++
		}
		return true
	})
	if staleLeft >= 3 {
		t.Errorf("expected at least 1 stale entry evicted, %d remain", staleLeft)
	}
}

// ============ openai_provider.go: Chat (non-stream) ============

func TestOpenAIProvider_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"1","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"answer","tool_calls":[{"id":"c1","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "test-key", DefaultModel: "gpt-4o", Timeout: 10})
	resp, err := p.Chat(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "answer" {
		t.Errorf("expected answer, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "search" {
		t.Errorf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Errorf("expected usage 15 tokens, got %+v", resp.Usage)
	}
}

func TestOpenAIProvider_Chat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, "rate limited")
	}))
	defer srv.Close()

	p := NewOpenAIProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	_, err := p.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 error, got %v", err)
	}
}

func TestOpenAIProvider_Chat_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"1","model":"m","choices":[]}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	if _, err := p.Chat(context.Background(), nil, nil, nil); err == nil {
		t.Error("expected empty response error")
	}
}

func TestOpenAIProvider_HealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"1","model":"m","choices":[{"message":{"content":"hi"}}]}`)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestOpenAIProvider_SetModelID(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m1", Timeout: 10})
	if p.GetModelID() != "m1" {
		t.Errorf("expected m1, got %q", p.GetModelID())
	}
	p.SetModelID("m2")
	if p.GetModelID() != "m2" {
		t.Errorf("expected m2, got %q", p.GetModelID())
	}
}

func TestOpenAIProvider_IsDeepSeek(t *testing.T) {
	p1 := NewOpenAIProvider(&ProviderConfig{Name: "deepseek", BaseURL: "https://api.deepseek.com", APIKey: "k", DefaultModel: "m", Timeout: 10})
	if !p1.isDeepSeek() {
		t.Error("expected deepseek detection by name")
	}
	p2 := NewOpenAIProvider(&ProviderConfig{Name: "openai", BaseURL: "https://api.openai.com", APIKey: "k", DefaultModel: "m", Timeout: 10})
	if p2.isDeepSeek() {
		t.Error("expected no deepseek detection for openai")
	}
}

// ============ openai_provider.go: convert/build helpers ============

func TestConvertContent_Plain(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	got := p.convertContent(kernel.Message{Content: "plain text"})
	if got != "plain text" {
		t.Errorf("expected plain text passthrough, got %v", got)
	}
}

func TestConvertContent_Multimodal(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	raw := "look at data:image/png;base64,AAAA and data:image/jpeg;base64,BBBB ok"
	got := p.convertContent(kernel.Message{Content: raw})
	parts, ok := got.([]map[string]interface{})
	if !ok {
		t.Fatalf("expected content array, got %T", got)
	}
	// 1 text + 2 images
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}
	imgCount := 0
	for _, part := range parts {
		if part["type"] == "image_url" {
			imgCount++
		}
	}
	if imgCount != 2 {
		t.Errorf("expected 2 images, got %d", imgCount)
	}
}

func TestConvertContent_Multimodal_PureImage(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	got := p.convertContent(kernel.Message{Content: "data:image/png;base64,AAAA"})
	parts := got.([]map[string]interface{})
	if len(parts) != 1 || parts[0]["type"] != "image_url" {
		t.Errorf("expected single image part, got %+v", parts)
	}
}

func TestConvertTools(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	tools := []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "read_file", Description: "read", Parameters: map[string]interface{}{"type": "object"}}},
		{Type: "function", Function: kernel.FunctionDef{Name: "strict_tool", Strict: true}},
	}
	got := p.convertTools(tools)
	if len(got) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(got))
	}
	fn0 := got[0]["function"].(map[string]interface{})
	if fn0["name"] != "read_file" || fn0["description"] != "read" {
		t.Errorf("unexpected tool 0: %+v", fn0)
	}
	fn1 := got[1]["function"].(map[string]interface{})
	if fn1["strict"] != true {
		t.Errorf("expected strict flag, got %+v", fn1)
	}
}

func TestConvertToolCalls(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	calls := []kernel.ToolCall{{ID: "c1", Type: "function", Function: kernel.FunctionCall{Name: "search", Arguments: "{}"}}}
	got := p.convertToolCalls(calls)
	if len(got) != 1 {
		t.Fatalf("expected 1 call, got %d", len(got))
	}
	fn := got[0]["function"].(map[string]interface{})
	if fn["name"] != "search" || fn["arguments"] != "{}" {
		t.Errorf("unexpected call: %+v", got[0])
	}
}

func TestConvertMessages_ToolAndReasoning(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10})
	msgs := []kernel.Message{
		{Role: "assistant", Content: "thinking", ReasoningContent: "deep", ToolCalls: []kernel.ToolCall{{ID: "c1", Function: kernel.FunctionCall{Name: "f"}}}},
		{Role: "tool", Content: "result", ToolCallID: "c1"},
		{Role: "user", Content: "next", Name: "alice"},
	}
	got := p.convertMessages(msgs)
	if len(got) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(got))
	}
	if got[0]["reasoning_content"] != "deep" {
		t.Errorf("expected reasoning_content, got %+v", got[0])
	}
	if got[1]["tool_call_id"] != "c1" {
		t.Errorf("expected tool_call_id, got %+v", got[1])
	}
	if got[2]["name"] != "alice" {
		t.Errorf("expected name, got %+v", got[2])
	}
}

func TestBuildRequestBody_JSONModeAndOptions(t *testing.T) {
	p := NewOpenAIProvider(&ProviderConfig{
		BaseURL: "http://x", APIKey: "k", DefaultModel: "m", Timeout: 10,
		JSONMode: true,
	})
	body := p.buildRequestBody([]kernel.Message{{Role: "user", Content: "hi"}}, nil, map[string]interface{}{
		"temperature": 0.5, "max_tokens": 100, "route": "execution", "no_thinking": true,
	})
	if body["model"] != "m" {
		t.Errorf("expected model m, got %v", body["model"])
	}
	if body["response_format"] == nil {
		t.Error("expected response_format for JSON mode")
	}
	// Internal keys filtered out
	if _, ok := body["route"]; ok {
		t.Error("route should be filtered")
	}
	if _, ok := body["no_thinking"]; ok {
		t.Error("no_thinking should be filtered")
	}
	// External options passed through
	if body["temperature"] != 0.5 || body["max_tokens"] != 100 {
		t.Errorf("expected external options, got %+v", body)
	}
}

func TestBuildRequestBody_DeepSeekThinking(t *testing.T) {
	thinking := true
	p := NewOpenAIProvider(&ProviderConfig{
		BaseURL: "https://api.deepseek.com", Name: "deepseek", APIKey: "k",
		DefaultModel: "m", Timeout: 10, Thinking: &thinking, ReasoningEffort: "high",
	})
	body := p.buildRequestBody(nil, nil, nil)
	if body["thinking"] == nil {
		t.Error("expected thinking enabled for deepseek")
	}
	if body["reasoning_effort"] != "high" {
		t.Errorf("expected reasoning_effort high, got %v", body["reasoning_effort"])
	}
	// no_thinking disables
	body2 := p.buildRequestBody(nil, nil, map[string]interface{}{"no_thinking": true})
	if body2["thinking"] != nil {
		t.Error("expected thinking disabled with no_thinking")
	}
}

// ============ anthropic_provider.go ============

func TestNewAnthropicProvider(t *testing.T) {
	p := NewAnthropicProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "claude", Timeout: 10})
	if p.GetModelID() != "claude" {
		t.Errorf("expected claude, got %q", p.GetModelID())
	}
	p.SetModelID("claude-2")
	if p.GetModelID() != "claude-2" {
		t.Errorf("expected claude-2, got %q", p.GetModelID())
	}
}

func TestAnthropicProvider_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("expected x-api-key test-key, got %q", got)
		}
		fmt.Fprint(w, `{"id":"m1","model":"claude","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"tu1","name":"search","input":{"q":"x"}}],"usage":{"input_tokens":10,"output_tokens":5}}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "test-key", DefaultModel: "claude", Timeout: 10})
	resp, err := p.Chat(context.Background(), []kernel.Message{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected hello, got %q", resp.Content)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Function.Name != "search" {
		t.Errorf("unexpected tool calls: %+v", resp.ToolCalls)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 10 {
		t.Errorf("expected usage, got %+v", resp.Usage)
	}
}

func TestAnthropicProvider_Chat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer srv.Close()

	p := NewAnthropicProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	_, err := p.Chat(context.Background(), nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "anthropic http 400") {
		t.Errorf("expected anthropic http error, got %v", err)
	}
}

func TestAnthropicProvider_ChatStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	p := NewAnthropicProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	ch, err := p.ChatStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}
	var content strings.Builder
	done := false
	for c := range ch {
		content.WriteString(c.Content)
		if c.Done {
			done = true
		}
	}
	if content.String() != "Hello" || !done {
		t.Errorf("got content=%q done=%v", content.String(), done)
	}
}

func TestAnthropicProvider_HealthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":"1","model":"m","content":[{"type":"text","text":"hi"}],"usage":{}}`)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(&ProviderConfig{BaseURL: srv.URL, APIKey: "k", DefaultModel: "m", Timeout: 10})
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
}

func TestAnthropicProvider_BuildBody_SystemAndTools(t *testing.T) {
	p := NewAnthropicProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "claude", Timeout: 10})
	body := p.buildAnthropicBody([]kernel.Message{
		{Role: "system", Content: "be careful"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "ok", ToolCalls: []kernel.ToolCall{{ID: "c1", Function: kernel.FunctionCall{Name: "search", Arguments: `{"q":"x"}`}}}},
		{Role: "tool", Content: "result", ToolCallID: "c1"},
	}, []kernel.ToolDefinition{
		{Type: "function", Function: kernel.FunctionDef{Name: "search", Description: "find", Parameters: map[string]interface{}{"type": "object"}}},
	}, map[string]interface{}{"temperature": 0.7, "max_tokens": 100})

	sysBlocks, ok := body["system"].([]map[string]interface{})
	if !ok || len(sysBlocks) != 1 {
		t.Fatalf("expected system as block array, got %v", body["system"])
	}
	if sysBlocks[0]["text"] != "be careful\n" {
		t.Errorf("expected system text, got %v", sysBlocks[0]["text"])
	}
	if _, ok := sysBlocks[0]["cache_control"]; !ok {
		t.Error("expected cache_control on system block")
	}
	if body["max_tokens"] != 100 {
		t.Errorf("expected max_tokens 100, got %v", body["max_tokens"])
	}
	if body["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", body["temperature"])
	}
	msgs := body["messages"].([]map[string]interface{})
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// tool message → user with tool_result
	if msgs[2]["role"] != "user" {
		t.Errorf("expected tool message converted to user, got %+v", msgs[2])
	}
	tools := body["tools"].([]map[string]interface{})
	if len(tools) != 1 || tools[0]["name"] != "search" {
		t.Errorf("unexpected tools: %+v", tools)
	}
}

func TestSplitMultimodalAnthropic(t *testing.T) {
	raw := "text data:image/png;base64,AAAA more"
	parts := splitMultimodalAnthropic(raw)
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	img := parts[1]
	if img["type"] != "image" {
		t.Errorf("expected image part, got %+v", img)
	}
	source := img["source"].(map[string]interface{})
	if source["media_type"] != "image/png" || source["data"] != "AAAA" {
		t.Errorf("unexpected source: %+v", source)
	}
}

func TestSplitMultimodalAnthropic_PureText(t *testing.T) {
	parts := splitMultimodalAnthropic("no image here")
	if len(parts) != 1 || parts[0]["text"] != "no image here" {
		t.Errorf("expected pure text fallback, got %+v", parts)
	}
}

func TestRegexpCompile(t *testing.T) {
	re := regexpCompile(`\d+`)
	if !re.MatchString("abc123") {
		t.Error("expected regex to match digits")
	}
}

// ============ anthropic_provider.go: JSON marshal of tool input ============

func TestAnthropicProvider_BuildBody_EmptyToolInput(t *testing.T) {
	p := NewAnthropicProvider(&ProviderConfig{BaseURL: "http://x", APIKey: "k", DefaultModel: "claude", Timeout: 10})
	body := p.buildAnthropicBody([]kernel.Message{
		{Role: "assistant", Content: "", ToolCalls: []kernel.ToolCall{{ID: "c1", Function: kernel.FunctionCall{Name: "noop", Arguments: ""}}}},
	}, nil, nil)
	msgs := body["messages"].([]map[string]interface{})
	content := msgs[0]["content"].([]map[string]interface{})
	if len(content) != 1 || content[0]["type"] != "tool_use" {
		t.Errorf("expected tool_use block, got %+v", content)
	}
	// Empty args unmarshal to nil input without panic
	var input map[string]interface{}
	_ = input
}
