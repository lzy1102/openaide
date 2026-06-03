package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openaide/backend/internal/kernel"
	"openaide/backend/internal/kernel/actor"
)

// FileCheckpointer saves ReAct loop snapshots to disk for crash recovery.
// Uses a CSP actor to serialize file I/O — no locks needed.
//
// Each session keeps at most 5 checkpoints; older ones are pruned on save.
// Checkpoints are JSON files named {sessionID}_{checkpointID}.json.
type FileCheckpointer struct {
	actor *actor.Actor
	dir   string
}

// FileCheckpointerConfig 文件检查点配置
type FileCheckpointerConfig struct {
	Dir string
}

// NewFileCheckpointer 创建文件检查点
func NewFileCheckpointer(cfg FileCheckpointerConfig) (*FileCheckpointer, error) {
	if cfg.Dir == "" {
		cfg.Dir = filepath.Join("data", "checkpoints")
	}
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}
	return &FileCheckpointer{actor: actor.NewActor(8), dir: cfg.Dir}, nil
}

func (fc *FileCheckpointer) Save(ctx context.Context, sessionID string, cp *kernel.Checkpoint) error {
	ch := make(chan error, 1)
	fc.actor.Send(func() {
		if cp.ID == "" {
			cp.ID = fmt.Sprintf("cp_%d", time.Now().UnixNano())
		}
		cp.CreatedAt = time.Now()
		data, err := json.MarshalIndent(cp, "", "  ")
		if err != nil {
			ch <- fmt.Errorf("marshal checkpoint: %w", err)
			return
		}
		path := filepath.Join(fc.dir, fmt.Sprintf("%s_%s.json", sessionID, cp.ID))
		if err := os.WriteFile(path, data, 0644); err != nil {
			ch <- fmt.Errorf("write checkpoint: %w", err)
			return
		}
		ch <- nil
		// Cleanup old checkpoints (keep last 5 per session)
		prefix := sessionID + "_"
		entries, _ := os.ReadDir(fc.dir)
		var sessionEntries []os.DirEntry
		for _, e := range entries {
			if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
				sessionEntries = append(sessionEntries, e)
			}
		}
		if len(sessionEntries) > 5 {
			for _, old := range sessionEntries[:len(sessionEntries)-5] {
				os.Remove(filepath.Join(fc.dir, old.Name()))
			}
		}
	})
	return <-ch
}

func (fc *FileCheckpointer) LoadLatest(ctx context.Context, sessionID string) (*kernel.Checkpoint, error) {
	checkpoints, err := fc.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(checkpoints) == 0 {
		return nil, nil
	}
	return checkpoints[len(checkpoints)-1], nil
}

func (fc *FileCheckpointer) List(ctx context.Context, sessionID string) ([]*kernel.Checkpoint, error) {
	ch := make(chan struct {
		cps []*kernel.Checkpoint
		err error
	}, 1)
	fc.actor.Send(func() {
		entries, err := os.ReadDir(fc.dir)
		if err != nil {
			if os.IsNotExist(err) {
				ch <- struct {
					cps []*kernel.Checkpoint
					err error
				}{nil, nil}
				return
			}
			ch <- struct {
				cps []*kernel.Checkpoint
				err error
			}{nil, fmt.Errorf("read checkpoint dir: %w", err)}
			return
		}
		prefix := sessionID + "_"
		var checkpoints []*kernel.Checkpoint
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(fc.dir, entry.Name()))
			if err != nil {
				continue
			}
			var cp kernel.Checkpoint
			if json.Unmarshal(data, &cp) == nil {
				checkpoints = append(checkpoints, &cp)
			}
		}
		ch <- struct {
			cps []*kernel.Checkpoint
			err error
		}{checkpoints, nil}
	})
	result := <-ch
	return result.cps, result.err
}

func (fc *FileCheckpointer) Delete(ctx context.Context, sessionID string, checkpointID string) error {
	ch := make(chan error, 1)
	fc.actor.Send(func() {
		path := filepath.Join(fc.dir, fmt.Sprintf("%s_%s.json", sessionID, checkpointID))
		err := os.Remove(path)
		if os.IsNotExist(err) {
			err = nil
		}
		ch <- err
	})
	return <-ch
}

// Stop shuts down the actor
func (fc *FileCheckpointer) Stop() { fc.actor.Stop() }
