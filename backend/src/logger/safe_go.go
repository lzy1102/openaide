package logger

import (
	"log/slog"
	"runtime/debug"
)

// SafeGo 安全启动 goroutine，捕获 panic 并记录日志
func SafeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered",
					"error", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}

// SafeGoWithCallback 安全启动 goroutine，捕获 panic 并调用回调
func SafeGoWithCallback(fn func(), onPanic func(recovered interface{})) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("goroutine panic recovered",
					"error", r,
					"stack", string(debug.Stack()),
				)
				if onPanic != nil {
					onPanic(r)
				}
			}
		}()
		fn()
	}()
}
