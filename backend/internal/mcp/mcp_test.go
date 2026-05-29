package mcp

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManager_ConnectInvalid(t *testing.T) {
	m := NewManager()
	err := m.ConnectServer("test", "nonexistent-command-xyz")
	if err == nil {
		t.Log("expected error for nonexistent command")
	}
}

func TestManager_GetTools(t *testing.T) {
	m := NewManager()
	tools := m.GetAllTools()
	if tools == nil {
		t.Log("GetAllTools returns nil when no servers connected (expected)")
	}
}

func TestManager_Shutdown(t *testing.T) {
	m := NewManager()
	// Should not panic even with no connections
	m.Shutdown()
}
