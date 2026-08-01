package main

import (
	"github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/kernel"
)

// streamMsg 包装一个 kernel.StreamChunk
type streamMsg struct {
	chunk kernel.StreamChunk
}

// waitForChunk 订阅流式 channel，收到块后转成 tea.Msg
func waitForChunk(ch <-chan kernel.StreamChunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return streamMsg{chunk: kernel.StreamChunk{Type: kernel.ChunkTypeDone, Done: true}}
		}
		return streamMsg{chunk: c}
	}
}

// waitForProgress 计划执行时同时监听进度与结果
func waitForProgress(progressCh chan progressMsg, resultCh chan planExecMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case p, ok := <-progressCh:
			if !ok {
				return nil
			}
			return p
		case r, ok := <-resultCh:
			if !ok {
				return nil
			}
			return r
		}
	}
}
