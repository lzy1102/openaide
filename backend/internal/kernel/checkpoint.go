package kernel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Checkpoint 会话检查点 - 保存 ReAct 循环的中间状态
type Checkpoint struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Round     int       `json:"round"`
	Messages  []Message `json:"messages"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Checkpointer 检查点接口
type Checkpointer interface {
	Save(ctx context.Context, sessionID string, cp *Checkpoint) error
	LoadLatest(ctx context.Context, sessionID string) (*Checkpoint, error)
	List(ctx context.Context, sessionID string) ([]*Checkpoint, error)
	Delete(ctx context.Context, sessionID string, checkpointID string) error
}

// FileCheckpointer 基于文件的检查点实现
type FileCheckpointer struct {
	mu     sync.Mutex
	dir    string
	suffix string // 可选后缀标识，如 "pre_tool"、"post_tool"
}

// FileCheckpointerConfig 文件检查点配置
type FileCheckpointerConfig struct {
	Dir string // 检查点目录，默认 ./data/checkpoints
}

// NewFileCheckpointer 创建文件检查点
func NewFileCheckpointer(cfg FileCheckpointerConfig) (*FileCheckpointer, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = filepath.Join("data", "checkpoints")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create checkpoint dir: %w", err)
	}
	return &FileCheckpointer{dir: dir}, nil
}

func (fc *FileCheckpointer) checkpointPath(sessionID, id string) string {
	return filepath.Join(fc.dir, fmt.Sprintf("%s_%s.json", sessionID, id))
}

func (fc *FileCheckpointer) sessionDir() string {
	return fc.dir
}

func (fc *FileCheckpointer) Save(ctx context.Context, sessionID string, cp *Checkpoint) error {
	slog.Debug("Checkpoint save", "session", sessionID[:min(8, len(sessionID))], "id", cp.ID, "msgs", len(cp.Messages))
	fc.mu.Lock()

	if cp.ID == "" {
		cp.ID = fmt.Sprintf("cp_%d", time.Now().UnixNano())
	}
	cp.CreatedAt = time.Now()

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		fc.mu.Unlock()
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	path := fc.checkpointPath(sessionID, cp.ID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		fc.mu.Unlock()
		return fmt.Errorf("write checkpoint: %w", err)
	}

	// 先解锁再清理旧检查点（List 也要获取同一把锁，sync.Mutex 不可重入）
	fc.mu.Unlock()

	// 每个会话最多保留 5 个检查点，删除旧的
	prefix := sessionID + "_"
	entries, err := os.ReadDir(fc.dir)
	if err == nil {
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
	}
	return nil
}

func (fc *FileCheckpointer) LoadLatest(ctx context.Context, sessionID string) (*Checkpoint, error) {
	slog.Debug("Checkpoint load", "session", sessionID[:min(8, len(sessionID))])
	checkpoints, err := fc.List(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(checkpoints) == 0 {
		return nil, nil
	}
	return checkpoints[len(checkpoints)-1], nil
}

func (fc *FileCheckpointer) List(ctx context.Context, sessionID string) ([]*Checkpoint, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	entries, err := os.ReadDir(fc.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read checkpoint dir: %w", err)
	}

	prefix := sessionID + "_"
	var checkpoints []*Checkpoint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fc.dir, entry.Name()))
		if err != nil {
			continue
		}
		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue
		}
		checkpoints = append(checkpoints, &cp)
	}
	return checkpoints, nil
}

func (fc *FileCheckpointer) Delete(ctx context.Context, sessionID string, checkpointID string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	path := fc.checkpointPath(sessionID, checkpointID)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("delete checkpoint: %w", err)
	}
	return nil
}
