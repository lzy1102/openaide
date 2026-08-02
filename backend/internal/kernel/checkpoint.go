package kernel

import (
	"context"
	"time"
)

// Checkpoint 会话检查点
type Checkpoint struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"session_id"`
	Round     int                    `json:"round"`
	Messages  []Message              `json:"messages"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// Checkpointer 检查点接口
type Checkpointer interface {
	Save(ctx context.Context, sessionID string, cp *Checkpoint) error
	LoadLatest(ctx context.Context, sessionID string) (*Checkpoint, error)
	List(ctx context.Context, sessionID string) ([]*Checkpoint, error)
	Delete(ctx context.Context, sessionID string, checkpointID string) error
}
