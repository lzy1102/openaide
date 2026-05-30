package infra

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PluginWatcher monitors the plugins directory and hot-reloads new plugins.
// Git clone a plugin into the directory — it activates without restart.
type PluginWatcher struct {
	dir      string
	watcher  *fsnotify.Watcher
	reloader func() []string
	debounce time.Duration
	running  atomic.Bool
}

// NewPluginWatcher creates a plugin directory watcher.
// reloader is called when a new directory appears in the plugins dir.
func NewPluginWatcher(pluginsDir string, reloader func() []string) *PluginWatcher {
	return &PluginWatcher{
		dir:      pluginsDir,
		reloader: reloader,
		debounce: 500 * time.Millisecond,
	}
}

// Start begins watching the plugins directory.
func (w *PluginWatcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	if err := watcher.Add(w.dir); err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll(w.dir, 0755)
			if err := watcher.Add(w.dir); err != nil {
				watcher.Close()
				return err
			}
		} else {
			watcher.Close()
			return err
		}
	}

	w.running.Store(true)
	go w.loop()

	slog.Info("Plugin hot-reload enabled", "dir", w.dir)
	return nil
}

// Stop stops watching.
func (w *PluginWatcher) Stop() {
	if w.running.CompareAndSwap(true, false) {
		w.watcher.Close()
	}
}

func (w *PluginWatcher) loop() {
	var timer *time.Timer
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Only react to new directories (Claude plugins are dirs with .claude-plugin/ inside)
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if timer != nil {
						timer.Stop()
					}
					timer = time.AfterFunc(w.debounce, func() {
						newIDs := w.reloader()
						if len(newIDs) > 0 {
							for _, id := range newIDs {
								slog.Info("Plugin hot-loaded", "id", id, "path", filepath.Join(w.dir, id))
							}
						}
					})
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("Plugin watcher error", "error", err)
		}
	}
}
