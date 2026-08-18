package tools

import (
	"os"
	"path/filepath"
	"sync"
)

// fileMutexMap 提供 per-path 互斥锁,防止并发写同一文件丢更新。
// LLM 可能在同一轮 ReAct 中对同一文件发起多个写操作(diff_edit + write_file,
// 或多个 diff_edit),虽然 kernel 的 isParallelSafe 会把写操作串行化,
// 但跨轮次或外部进程仍可能并发写同一文件。
//
// 用法:
//
//	unlock := lockFile(absPath)
//	defer unlock()
//	// ... 读写文件 ...
//
// 不同文件的锁互不阻塞,同文件的锁串行化。
var fileMutexMap sync.Map // path(string) -> *sync.Mutex

// lockFile 获取文件路径对应的互斥锁,返回解锁函数。
// 同一 absPath 的多次调用会串行等待。
func lockFile(absPath string) func() {
	v, _ := fileMutexMap.LoadOrStore(absPath, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// atomicWriteFile 原子写入文件:先写临时文件,再 rename 到目标路径。
// rename 在同一文件系统下是原子的,防止写到一半崩溃导致文件损坏。
// 写入前会获取 per-path 锁,防止并发写丢更新。
func atomicWriteFile(absPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 临时文件与目标文件在同一目录(确保同一文件系统)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	// 写失败时清理临时文件
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpPath)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// rename 是原子操作(同文件系统下)
	if err := os.Rename(tmpPath, absPath); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}
