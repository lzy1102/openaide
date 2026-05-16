package main

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

// streamIntoProgram 启动流式处理并通过 p.Send() 实时发送每个 chunk
// 这实现了真正的逐字流式输出
func streamIntoProgram(p *tea.Program, app *infra.Application, query string) {
	go func() {
		ctx := context.Background()
		stream, err := app.Orchestrator.ProcessQueryStream(ctx, "tui-user", "default", query, kernel.QueryOptions{})
		if err != nil {
			p.Send(streamChunkMsg{err: err, done: true})
			return
		}

		totalTools := 0
		totalTokens := 0

		for chunk := range stream {
			if chunk.Error != nil {
				p.Send(streamChunkMsg{err: chunk.Error, done: true})
				return
			}

			if chunk.Done {
				if chunk.Usage != nil {
					totalTokens = chunk.Usage.TotalTokens
				}
				p.Send(streamChunkMsg{done: true, tokens: totalTokens, toolCnt: totalTools})
				return
			}

			if len(chunk.ToolCalls) > 0 {
				totalTools += len(chunk.ToolCalls)
			}

			p.Send(streamChunkMsg{
				content:  chunk.Content,
				thinking: chunk.ReasoningContent,
			})

			// 小延迟让渲染自然
			time.Sleep(5 * time.Millisecond)
		}
	}()
}
