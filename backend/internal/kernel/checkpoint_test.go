package kernel

import (
	"context"
	"testing"
)

func TestFileCheckpointer_SaveAndList(t *testing.T) {
	fc, err := NewFileCheckpointer(FileCheckpointerConfig{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer fc.Stop()

	ctx := context.Background()
	cp := &Checkpoint{SessionID: "s1", Round: 1, Messages: []Message{{Role: "user", Content: "test"}}}
	if err := fc.Save(ctx, "s1", cp); err != nil {
		t.Fatal(err)
	}

	list, err := fc.List(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Error("expected at least 1 checkpoint")
	}
}

func TestFileCheckpointer_SaveAndLoad(t *testing.T) {
	fc, _ := NewFileCheckpointer(FileCheckpointerConfig{Dir: t.TempDir()})
	defer fc.Stop()

	ctx := context.Background()
	cp := &Checkpoint{SessionID: "s2", Round: 0, Messages: []Message{{Role: "assistant", Content: "response"}}}
	fc.Save(ctx, "s2", cp)

	loaded, err := fc.LoadLatest(ctx, "s2")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected loaded checkpoint, got nil")
	}
	if loaded.SessionID != "s2" {
		t.Errorf("expected session 's2', got '%s'", loaded.SessionID)
	}
}

func TestFileCheckpointer_Delete(t *testing.T) {
	fc, _ := NewFileCheckpointer(FileCheckpointerConfig{Dir: t.TempDir()})
	defer fc.Stop()

	ctx := context.Background()
	cp := &Checkpoint{SessionID: "s3", Round: 1, Messages: []Message{}}
	fc.Save(ctx, "s3", cp)

	list, _ := fc.List(ctx, "s3")
	if len(list) == 0 {
		t.Fatal("expected checkpoint before delete")
	}
	id := list[0].ID

	fc.Delete(ctx, "s3", id)
	list2, _ := fc.List(ctx, "s3")
	if len(list2) != 0 {
		t.Error("expected empty list after delete")
	}
}
