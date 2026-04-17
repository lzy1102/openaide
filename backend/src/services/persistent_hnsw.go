package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"openaide/backend/src/config"
)

// PersistentHNSW 持久化 HNSW 索引
type PersistentHNSW struct {
	*HNSWIndex
	dataDir      string
	autoSave     bool
	saveInterval time.Duration
	mu           sync.RWMutex
	stopCh       chan struct{}
}

// NewPersistentHNSW 创建持久化 HNSW 索引
func NewPersistentHNSW(dimension int, dataDir string) (*PersistentHNSW, error) {
	if dataDir == "" {
		// 使用统一的数据目录 ~/.openaide/data/vectors
		dataDir = config.DefaultPaths.VectorDir
	}

	// 确保目录存在
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	ph := &PersistentHNSW{
		HNSWIndex:    NewHNSWIndex(dimension),
		dataDir:      dataDir,
		autoSave:     true,
		saveInterval: 5 * time.Minute,
		stopCh:       make(chan struct{}),
	}

	// 尝试加载已有索引
	if err := ph.Load(); err != nil {
		// 文件不存在是正常的，忽略错误
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load index: %w", err)
		}
	}

	// 启动自动保存
	if ph.autoSave {
		go ph.autoSaveLoop()
	}

	return ph, nil
}

// GetDataPath 获取数据文件路径
func (ph *PersistentHNSW) GetDataPath() string {
	return filepath.Join(ph.dataDir, fmt.Sprintf("hnsw_%d.json", ph.Dimension))
}

// Save 保存索引到文件
func (ph *PersistentHNSW) Save() error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	dataPath := ph.GetDataPath()
	tempPath := dataPath + ".tmp"

	// 序列化数据
	data, err := json.MarshalIndent(ph.HNSWIndex, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// 写入临时文件
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// 原子重命名
	if err := os.Rename(tempPath, dataPath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// Load 从文件加载索引
func (ph *PersistentHNSW) Load() error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	dataPath := ph.GetDataPath()

	// 检查文件是否存在
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		return err
	}

	// 读取文件
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 反序列化
	var loaded HNSWIndex
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to unmarshal index: %w", err)
	}

	// 验证维度
	if loaded.Dimension != ph.Dimension {
		return fmt.Errorf("dimension mismatch: expected %d, got %d", ph.Dimension, loaded.Dimension)
	}

	// 替换当前索引
	ph.HNSWIndex = &loaded

	return nil
}

// Insert 插入向量并自动保存
func (ph *PersistentHNSW) Insert(id string, vector []float32, metadata map[string]interface{}) error {
	if err := ph.HNSWIndex.Insert(id, vector, metadata); err != nil {
		return err
	}

	if ph.autoSave {
		ph.Save()
	}

	return nil
}

// Delete 删除向量并自动保存
func (ph *PersistentHNSW) Delete(id string) error {
	if err := ph.HNSWIndex.Delete(id); err != nil {
		return err
	}

	if ph.autoSave {
		ph.Save()
	}

	return nil
}

// autoSaveLoop 自动保存循环
func (ph *PersistentHNSW) autoSaveLoop() {
	ticker := time.NewTicker(ph.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ph.stopCh:
			return
		case <-ticker.C:
			if err := ph.Save(); err != nil {
				fmt.Printf("[PersistentHNSW] Auto-save failed: %v\n", err)
			}
		}
	}
}

// Close 关闭并保存
func (ph *PersistentHNSW) Close() error {
	ph.autoSave = false
	close(ph.stopCh)
	return ph.Save()
}

// GetStats 获取统计信息
func (ph *PersistentHNSW) GetStats() map[string]interface{} {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	dataPath := ph.GetDataPath()
	fileSize := int64(0)
	if info, err := os.Stat(dataPath); err == nil {
		fileSize = info.Size()
	}

	return map[string]interface{}{
		"dimension":   ph.Dimension,
		"count":       ph.Count,
		"data_dir":    ph.dataDir,
		"file_size":   fileSize,
		"auto_save":   ph.autoSave,
		"entry_point": ph.EntryPoint,
		"max_level":   ph.CurrentMaxLevel,
	}
}

// Backup 创建备份
func (ph *PersistentHNSW) Backup(backupDir string) error {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("hnsw_%d_%s.json", ph.Dimension, timestamp))

	data, err := json.MarshalIndent(ph.HNSWIndex, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(backupPath, data, 0644)
}

// Restore 从备份恢复
func (ph *PersistentHNSW) Restore(backupPath string) error {
	ph.mu.Lock()
	defer ph.mu.Unlock()

	data, err := os.ReadFile(backupPath)
	if err != nil {
		return err
	}

	var loaded HNSWIndex
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}

	if loaded.Dimension != ph.Dimension {
		return fmt.Errorf("dimension mismatch")
	}

	ph.HNSWIndex = &loaded
	return ph.Save()
}
