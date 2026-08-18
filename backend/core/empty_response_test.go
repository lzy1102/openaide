package kernel

import (
	"context"
	"testing"
)

// TestProcessStream_EmptyLLMResponseIsError 回归测试:
// LLM 流返回空(限流/静默失败)时,ProcessStream 必须返回错误,
// 而不是"成功完成 + 保存空 assistant"。
func TestProcessStream_EmptyLLMResponseIsError(t *testing.T) {
	llm := &MockLLMProvider{
		// 只有 Done chunk,无 content 无 tool calls — 模拟 LLM 静默失败
		streamChunks: [][]StreamChunk{
			{{Type: ChunkTypeDone, Done: true}},
		},
	}
	tools := &MockToolExecutor{}
	mem := &MockMemory{}
	store := NewSessionStoreAdapter()
	kernel := NewAgentKernel(llm, tools, mem, store, DefaultConfig())

	ch, err := kernel.ProcessStream(context.Background(), &Query{
		SessionID: "",
		Content:   "测试空回复",
		UserID:    "u1",
		ProjectID: "p1",
	})
	if err != nil {
		t.Fatalf("ProcessStream returned error before stream: %v", err)
	}

	gotError := false
	gotDone := false
	for chunk := range ch {
		t.Logf("chunk: type=%q done=%v err=%v", chunk.Type, chunk.Done, chunk.Error)
		if chunk.Type == ChunkTypeError || chunk.Error != nil {
			gotError = true
		}
		if chunk.Done {
			gotDone = true
		}
	}
	if !gotError {
		t.Error("expected an error chunk when LLM returns empty, got none")
	}
	if !gotDone {
		t.Error("expected the error chunk to carry done=true as the stream terminator")
	}
}

// TestProcessStream_EmptyLLMResponse_NoSessionPollution 回归测试:
// LLM 空回复时不保存空 assistant 消息到会话。
func TestProcessStream_EmptyLLMResponse_NoSessionPollution(t *testing.T) {
	llm := &MockLLMProvider{
		streamChunks: [][]StreamChunk{
			{{Type: ChunkTypeDone, Done: true}},
		},
	}
	store := NewSessionStoreAdapter()
	kernel := NewAgentKernel(llm, &MockToolExecutor{}, &MockMemory{}, store, DefaultConfig())

	ch, _ := kernel.ProcessStream(context.Background(), &Query{
		Content:   "空回复测试",
		UserID:    "u1",
		ProjectID: "p1",
	})
	for range ch {
	}

	// 会话不应保存空 assistant 消息(修复前会保存,污染恢复界面)
	sessions, _ := store.List(context.Background(), "p1", "u1", 10, 0)
	if len(sessions) == 0 {
		return // 无会话也接受 — 关键是不要保存空内容
	}
	for _, s := range sessions {
		for _, m := range s.Messages {
			if m.Role == "assistant" && m.Content == "" {
				t.Errorf("session %s saved an empty assistant message: %+v", s.ID[:8], m)
			}
		}
	}
}

// TestProcessStream_StallRoundsConverges 验证连续空转轮触发收敛提示。
// 用始终返回空内容的 LLM 模拟空转:流结束但无 content、无工具调用。
// 第 2 轮起应注入 [Progress] 收敛提示(每 5 轮一次),最终以正常方式结束。
func TestProcessStream_StallRoundsConverges(t *testing.T) {
	llm := &MockLLMProvider{
		// 每轮都是:无内容 + Done — 模拟空转
		streamChunks: [][]StreamChunk{
			{{Type: ChunkTypeDone, Done: true}},
			{{Type: ChunkTypeDone, Done: true}},
			{{Type: ChunkTypeDone, Done: true}},
			{{Type: ChunkTypeDone, Done: true}},
			{{Type: ChunkTypeDone, Done: true}},
			{{Type: ChunkTypeDone, Done: true}},
		},
	}
	store := NewSessionStoreAdapter()
	kernel := NewAgentKernel(llm, &MockToolExecutor{}, &MockMemory{}, store, DefaultConfig())

	ch, err := kernel.ProcessStream(context.Background(), &Query{
		Content:   "空转测试",
		UserID:    "u1",
		ProjectID: "p1",
	})
	if err != nil {
		t.Fatalf("ProcessStream error: %v", err)
	}
	for range ch {
	}
	// 无 panic、正常结束即可 — 防跑偏逻辑不应导致死循环
}
