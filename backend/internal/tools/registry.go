package tools

import (
	"context"
	"fmt"
	"sync/atomic"

	"openaide/backend/internal/kernel"
)

type registryData struct {
	definitions map[string]kernel.ToolDefinition
	handlers    map[string]kernel.ToolHandler
}

// Registry is a lock-free tool registry using atomic.Value.
// Writes are rare (init only), reads are on every query.
type Registry struct {
	data             atomic.Value    // *registryData
	pendingQuestions chan string     // questions from ask_user tool (buffered, non-blocking)
}

// GetPendingQuestions drains and returns all pending user questions.
func (r *Registry) GetPendingQuestions() []string {
	if r.pendingQuestions == nil { return nil }
	var qs []string
	for {
		select {
		case q := <-r.pendingQuestions: qs = append(qs, q)
		default: return qs
		}
	}
}

// NewRegistry creates a lock-free tool registry.
func NewRegistry() *Registry {
	r := &Registry{pendingQuestions: make(chan string, 32)}
	r.data.Store(&registryData{
		definitions: make(map[string]kernel.ToolDefinition),
		handlers:    make(map[string]kernel.ToolHandler),
	})
	return r
}

func (r *Registry) load() *registryData { return r.data.Load().(*registryData) }

// Register adds a tool. Thread-safe but typically called during init.
func (r *Registry) Register(tool kernel.ToolDefinition, handler kernel.ToolHandler) error {
	name := tool.Function.Name
	if name == "" {
		return fmt.Errorf("tool name is empty")
	}

	old := r.load()
	if _, exists := old.definitions[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	// Copy-on-write
	newDefs := make(map[string]kernel.ToolDefinition, len(old.definitions)+1)
	newHandlers := make(map[string]kernel.ToolHandler, len(old.handlers)+1)
	for k, v := range old.definitions { newDefs[k] = v }
	for k, v := range old.handlers { newHandlers[k] = v }
	newDefs[name] = tool
	newHandlers[name] = handler
	r.data.Store(&registryData{definitions: newDefs, handlers: newHandlers})
	return nil
}

// Unregister removes a tool.
func (r *Registry) Unregister(name string) error {
	old := r.load()
	if _, exists := old.definitions[name]; !exists {
		return fmt.Errorf("tool not found: %s", name)
	}
	newDefs := make(map[string]kernel.ToolDefinition, len(old.definitions)-1)
	newHandlers := make(map[string]kernel.ToolHandler, len(old.handlers)-1)
	for k, v := range old.definitions {
		if k != name { newDefs[k] = v }
	}
	for k, v := range old.handlers {
		if k != name { newHandlers[k] = v }
	}
	r.data.Store(&registryData{definitions: newDefs, handlers: newHandlers})
	return nil
}

// GetDefinitions returns all tool definitions. Lock-free.
func (r *Registry) GetDefinitions() []kernel.ToolDefinition {
	d := r.load()
	defs := make([]kernel.ToolDefinition, 0, len(d.definitions))
	for _, v := range d.definitions { defs = append(defs, v) }
	return defs
}

// GetDefinitionsByNames returns selected tool definitions. Lock-free.
func (r *Registry) GetDefinitionsByNames(names []string) []kernel.ToolDefinition {
	d := r.load()
	defs := make([]kernel.ToolDefinition, 0, len(names))
	for _, name := range names {
		if def, ok := d.definitions[name]; ok { defs = append(defs, def) }
	}
	return defs
}

// Execute runs a tool call. Lock-free map lookup.
func (r *Registry) Execute(ctx context.Context, call kernel.ToolCall, sessionID string) (*kernel.ToolResult, error) {
	d := r.load()
	handler, ok := d.handlers[call.Function.Name]
	if !ok {
		return &kernel.ToolResult{Error: fmt.Sprintf("unknown tool: %s", call.Function.Name)}, nil
	}
	return handler(ctx, call.Function.Arguments)
}

// HasTool checks if a tool exists. Lock-free.
func (r *Registry) HasTool(name string) bool {
	_, ok := r.load().handlers[name]
	return ok
}

// Count returns the number of registered tools. Lock-free.
func (r *Registry) Count() int { return len(r.load().definitions) }
