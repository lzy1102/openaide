package main

import "openaide/backend/internal/infra"

// runREPL 已迁移到 bubbletea TUI（runTUI）。保留入口以兼容调用方。
func runREPL(app *infra.Application, continueSess, autoYes bool) {
	runTUI(app, continueSess, autoYes)
}
