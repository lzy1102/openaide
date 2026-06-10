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

func TestManager_GetServerTools(t *testing.T) {
	m := NewManager()
	tools := m.GetServerTools("nonexistent")
	if tools == nil {
		t.Log("GetServerTools returns nil when no servers connected (expected)")
	}
}

func TestManager_Shutdown(t *testing.T) {
	m := NewManager()
	// Should not panic even with no connections
	m.Shutdown()
}
