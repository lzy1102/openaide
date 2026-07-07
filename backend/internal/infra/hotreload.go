package infra

import (
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"openaide/backend/internal/config"
	"openaide/backend/internal/kernel"
	"github.com/fsnotify/fsnotify"
)

// ConfigReloader watches config file and hot-reloads mutable settings
type ConfigReloader struct {
	path      string
	app       *Application
	watcher   *fsnotify.Watcher
	callbacks []func(*config.Config)
	debounce  time.Duration
	running   atomic.Bool
}

// NewConfigReloader creates a config file watcher
func NewConfigReloader(path string, app *Application) *ConfigReloader {
	return &ConfigReloader{
		path:     path,
		app:      app,
		debounce: 500 * time.Millisecond,
	}
}

// OnReload registers callbacks invoked after config reload
func (r *ConfigReloader) OnReload(fn func(*config.Config)) {
	r.callbacks = append(r.callbacks, fn)
}

// Start begins watching the config file
func (r *ConfigReloader) Start() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	r.watcher = w

	if err := w.Add(r.path); err != nil {
		return err
	}

	r.running.Store(true)
	go r.loop()
	slog.Info("Config hot-reload enabled", "path", r.path)
	return nil
}

// Stop stops watching
func (r *ConfigReloader) Stop() {
	r.running.Store(false)
	if r.watcher != nil {
		r.watcher.Close()
	}
}

func (r *ConfigReloader) loop() {
	// Debounce: batch rapid writes (e.g. editor save with temp file rename)
	var timer *time.Timer
	for r.running.Load() {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok { return }
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if timer != nil { timer.Stop() }
				timer = time.AfterFunc(r.debounce, r.doReload)
			}
		case err, ok := <-r.watcher.Errors:
			if !ok { return }
			slog.Warn("Config watcher error", "error", err)
		}
	}
}

func (r *ConfigReloader) doReload() {
	cfg, err := config.Load(r.path)
	if err != nil {
		slog.Warn("Config reload skipped (parse error)", "error", err)
		return
	}

	// Apply mutable settings without restart
	if r.app.Config != nil {
		// Log level
		if cfg.Log.Level != "" && cfg.Log.Level != r.app.Config.Log.Level {
			InitLogger(cfg.Log.Level, cfg.Log.Format)
			slog.Info("Log level changed", "level", cfg.Log.Level)
		}

		// Language
		if cfg.Lang != "" && cfg.Lang != r.app.Config.Lang {
			reloadLang(cfg.Lang)
		}

		// Kernel settings (hot-reloadable via concrete type)
		if ak, ok := r.app.Kernel.(*kernel.AgentKernel); ok {
			ak.ApplyConfig(
				cfg.Kernel.MaxRounds,
				cfg.Kernel.MaxTokens,
				cfg.Kernel.MinRounds,
				cfg.Kernel.MaxRoundsCap,
			)
		}

		// LLM gateway: update model IDs and routing
		if r.app.LLMGateway != nil {
			models := make(map[string]string)
			for _, p := range cfg.LLM.Providers {
				models[p.Name] = p.DefaultModel
			}
			r.app.LLMGateway.ReloadConfig(models, cfg.LLM.ModelRouting.Reasoning, cfg.LLM.ModelRouting.Execution)
		}

		// Tool settings (browser, searxng)
		if cfg.Search.SearXNGURL != r.app.Config.Search.SearXNGURL {
			// tools.SetSearXNGURL is set at startup, update if available
			slog.Info("Search config changed (restart recommended)", "searxng_url", cfg.Search.SearXNGURL)
		}
	}

	// Replace config reference
	r.app.Config = cfg

	// Notify callbacks
	for _, cb := range r.callbacks {
		cb(cfg)
	}

	slog.Info("Config reloaded successfully")
}

// reloadLang switches language at runtime
func reloadLang(lang string) {
	// Import cycle avoidance: set via env-like mechanism
	if lang == "zh" {
		os.Setenv("OPENAIDE_LANG", "zh")
	} else {
		os.Setenv("OPENAIDE_LANG", "en")
	}
}
