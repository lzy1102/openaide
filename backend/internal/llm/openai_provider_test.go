package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openaide/backend/internal/kernel"
)

func TestOpenAIProvider_ChatStream_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" world\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Timeout:      10,
	})

	ch, err := provider.ChatStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var content strings.Builder
	var done bool
	for chunk := range ch {
		content.WriteString(chunk.Content)
		if chunk.Done {
			done = true
		}
	}

	if content.String() != "Hello world" {
		t.Errorf("expected 'Hello world', got '%s'", content.String())
	}
	if !done {
		t.Error("expected done signal")
	}
}

func TestOpenAIProvider_ChatStream_ToolCallAccumulation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test\"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Using search tool"},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			fmt.Fprint(w, chunk+"\n\n")
		}
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Timeout:      10,
	})

	ch, err := provider.ChatStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var gotContent strings.Builder
	var gotToolCalls []kernel.ToolCall
	for chunk := range ch {
		gotContent.WriteString(chunk.Content)
		if len(chunk.ToolCalls) > 0 {
			gotToolCalls = chunk.ToolCalls
		}
	}

	if len(gotToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(gotToolCalls))
	}
	if gotToolCalls[0].Function.Name != "search" {
		t.Errorf("expected 'search', got '%s'", gotToolCalls[0].Function.Name)
	}
	if gotToolCalls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("expected '{\"q\":\"test\"}', got '%s'", gotToolCalls[0].Function.Arguments)
	}
}

func TestOpenAIProvider_ChatStream_MultipleToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		chunks := []string{
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"read_file","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"test\"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"/tmp/foo.txt\""}}]},"finish_reason":null}]}`,
			`data: {"id":"1","object":"chat.completion.chunk","created":123,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: [DONE]`,
		}
		for _, chunk := range chunks {
			fmt.Fprint(w, chunk+"\n\n")
		}
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Timeout:      10,
	})

	ch, err := provider.ChatStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var gotToolCalls []kernel.ToolCall
	for chunk := range ch {
		if len(chunk.ToolCalls) > 0 {
			gotToolCalls = chunk.ToolCalls
		}
	}

	if len(gotToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(gotToolCalls))
	}

	if gotToolCalls[0].Function.Name != "search" {
		t.Errorf("expected tool[0] name 'search', got '%s'", gotToolCalls[0].Function.Name)
	}
	if gotToolCalls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("expected tool[0] args '{\"q\":\"test\"}', got '%s'", gotToolCalls[0].Function.Arguments)
	}

	if gotToolCalls[1].Function.Name != "read_file" {
		t.Errorf("expected tool[1] name 'read_file', got '%s'", gotToolCalls[1].Function.Name)
	}
	if gotToolCalls[1].Function.Arguments != `"/tmp/foo.txt"` {
		t.Errorf("expected tool[1] args '/tmp/foo.txt', got '%s'", gotToolCalls[1].Function.Arguments)
	}
}

func TestOpenAIProvider_ChatStream_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"error\":\"invalid_api_key\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      srv.URL,
		APIKey:       "bad-key",
		DefaultModel: "gpt-4o",
		Timeout:      10,
	})

	ch, err := provider.ChatStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var content string
	for chunk := range ch {
		content += chunk.Content
	}
	if content != "" {
		t.Errorf("expected empty content from error-only response, got '%s'", content)
	}
}

func TestOpenAIProvider_ChatStream_Cancel(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      "http://127.0.0.1:1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Timeout:      5,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := provider.ChatStream(ctx, nil, nil, nil)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestOpenAIProvider_ChatStream_ServerClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"created\":123,\"model\":\"gpt-4o\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"chunk1\"},\"finish_reason\":null}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		BaseURL:      srv.URL,
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Timeout:      10,
	})

	ctx := context.Background()
	ch, err := provider.ChatStream(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ChatStream failed: %v", err)
	}

	var chunks []string
	for chunk := range ch {
		chunks = append(chunks, chunk.Content)
	}

	if len(chunks) != 1 || chunks[0] != "chunk1" {
		t.Errorf("expected [chunk1], got %v", chunks)
	}
}

func TestNewOpenAIProvider(t *testing.T) {
	provider := NewOpenAIProvider(&ProviderConfig{
		Name:         "test",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "test-key",
		DefaultModel: "gpt-4o",
		Enabled:      true,
	})

	if provider.GetModelID() != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", provider.GetModelID())
	}
}
