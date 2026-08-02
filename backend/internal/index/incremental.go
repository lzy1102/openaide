package index

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileFingerprint 文件指纹
type FileFingerprint struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mtime"`
	ContentHash string    `json:"hash"`
}

// ChangeSet 变更集合
type ChangeSet struct {
	Added    []FileFingerprint `json:"added"`
	Modified []FileFingerprint `json:"modified"`
	Deleted  []string          `json:"deleted"`
}

// IncrementalIndexer 增量索引器
type IncrementalIndexer struct {
	*Indexer
	fingerprints     map[string]*FileFingerprint
	fingerprintsMu   sync.RWMutex
	fingerprintsPath string
}

// NewIncrementalIndexer 创建增量索引器
func NewIncrementalIndexer(indexDir string) (*IncrementalIndexer, error) {
	base, err := NewIndexer(indexDir)
	if err != nil {
		return nil, err
	}

	ii := &IncrementalIndexer{
		Indexer:          base,
		fingerprints:     make(map[string]*FileFingerprint),
		fingerprintsPath: filepath.Join(indexDir, "fingerprints.json"),
	}

	ii.loadFingerprints()
	return ii, nil
}

// DetectChanges 检测变更
func (ii *IncrementalIndexer) DetectChanges(root string, extensions []string) (*ChangeSet, error) {
	changes := &ChangeSet{
		Added:    make([]FileFingerprint, 0),
		Modified: make([]FileFingerprint, 0),
		Deleted:  make([]string, 0),
	}

	// 扫描当前文件
	currentFiles := make(map[string]*FileFingerprint)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// 检查扩展名
		ext := filepath.Ext(path)
		if len(extensions) > 0 {
			found := false
			for _, e := range extensions {
				if ext == e {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		fp, err := ii.computeFingerprint(path, info)
		if err != nil {
			return nil
		}
		currentFiles[path] = fp
		return nil
	})

	if err != nil {
		return nil, err
	}

	ii.fingerprintsMu.RLock()
	lastPrints := make(map[string]*FileFingerprint, len(ii.fingerprints))
	for k, v := range ii.fingerprints {
		lastPrints[k] = v
	}
	ii.fingerprintsMu.RUnlock()

	// 检测新增和修改
	for path, fp := range currentFiles {
		last, exists := lastPrints[path]
		if !exists {
			changes.Added = append(changes.Added, *fp)
		} else if fp.ContentHash != last.ContentHash {
			changes.Modified = append(changes.Modified, *fp)
		}
		delete(lastPrints, path)
	}

	// 剩余的是删除的
	for path := range lastPrints {
		changes.Deleted = append(changes.Deleted, path)
	}

	return changes, nil
}

// UpdateIndex 增量更新索引
func (ii *IncrementalIndexer) UpdateIndex(root string, extensions []string) error {
	changes, err := ii.DetectChanges(root, extensions)
	if err != nil {
		return err
	}

	totalChanges := len(changes.Added) + len(changes.Modified) + len(changes.Deleted)
	if totalChanges == 0 {
		return nil
	}

	// 处理新增
	for _, fp := range changes.Added {
		ii.IndexFile(fp.Path)
		ii.updateFingerprint(&fp)
	}

	// 处理修改
	for _, fp := range changes.Modified {
		ii.IndexFile(fp.Path)
		ii.updateFingerprint(&fp)
	}

	// 处理删除
	for _, path := range changes.Deleted {
		ii.removeFile(path)
		ii.removeFingerprint(path)
	}

	// 保存指纹
	return ii.saveFingerprints()
}

// FullIndex 全量索引
func (ii *IncrementalIndexer) FullIndex(root string, extensions []string) error {
	// 清除旧索引
	ii.index = &ProjectIndex{
		Files:    make(map[string]*FileIndex),
		Symbols:  make(map[string][]*Symbol),
		Packages: make(map[string][]string),
	}

	ii.fingerprintsMu.Lock()
	ii.fingerprints = make(map[string]*FileFingerprint)
	ii.fingerprintsMu.Unlock()

	// 执行全量索引
	err := ii.IndexDirectory(root, extensions)
	if err != nil {
		return err
	}

	// 重新计算所有指纹
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if len(extensions) > 0 {
			found := false
			for _, e := range extensions {
				if ext == e {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		}

		fp, err := ii.computeFingerprint(path, info)
		if err != nil {
			return nil
		}
		ii.updateFingerprint(fp)
		return nil
	})
}

// ============ 内部方法 ============

func (ii *IncrementalIndexer) computeFingerprint(path string, info os.FileInfo) (*FileFingerprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return &FileFingerprint{
		Path:        path,
		Size:        info.Size(),
		ModTime:     info.ModTime(),
		ContentHash: fmt.Sprintf("%x", sha256.Sum256(data)),
	}, nil
}

func (ii *IncrementalIndexer) updateFingerprint(fp *FileFingerprint) {
	ii.fingerprintsMu.Lock()
	ii.fingerprints[fp.Path] = fp
	ii.fingerprintsMu.Unlock()
}

func (ii *IncrementalIndexer) removeFingerprint(path string) {
	ii.fingerprintsMu.Lock()
	delete(ii.fingerprints, path)
	ii.fingerprintsMu.Unlock()
}

func (ii *IncrementalIndexer) removeFile(path string) {
	ii.mu.Lock()
	delete(ii.index.Files, path)
	ii.mu.Unlock()
}

func (ii *IncrementalIndexer) saveFingerprints() error {
	ii.fingerprintsMu.RLock()
	data, err := json.MarshalIndent(ii.fingerprints, "", "  ")
	ii.fingerprintsMu.RUnlock()

	if err != nil {
		return err
	}

	return os.WriteFile(ii.fingerprintsPath, data, 0644)
}

func (ii *IncrementalIndexer) loadFingerprints() error {
	data, err := os.ReadFile(ii.fingerprintsPath)
	if err != nil {
		return nil
	}

	return json.Unmarshal(data, &ii.fingerprints)
}
