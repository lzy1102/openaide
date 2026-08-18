package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"openaide/backend/core"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(64 * 1024)
	conn.SetReadDeadline(time.Now().Add(60 * time.Minute))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Minute))
		return nil
	})

	// 心跳
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req ChatRequest
		if err := json.Unmarshal(msgBytes, &req); err != nil {
			sendWSError(conn, "invalid json: "+err.Error())
			continue
		}

		// 流式处理
		ctx := r.Context()
		stream, err := s.orchestrator.ProcessQueryStream(ctx, "", req.UserID, req.ProjectID, req.Message, kernel.QueryOptions{
			ModelID:     req.Model,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			ToolFilter:  req.Tools,
		})
		if err != nil {
			sendWSError(conn, err.Error())
			continue
		}

		done := ctx.Done()
	loop:
		for {
			select {
			case <-done:
				break loop
			case chunk, ok := <-stream:
				if !ok {
					break loop
				}
				if chunk.Error != nil {
					sendWS(conn, map[string]interface{}{
						"type":  "error",
						"error": chunk.Error.Error(),
					})
					break loop
				}

				event := map[string]interface{}{
					"type": "chunk",
				}

				if chunk.Done {
					event["type"] = "done"
					if chunk.Usage != nil {
						event["tokens"] = chunk.Usage.TotalTokens
					}
					sendWS(conn, event)
					break loop
				}

				if chunk.Content != "" {
					event["content"] = chunk.Content
				}
				if chunk.ReasoningContent != "" {
					event["thinking"] = chunk.ReasoningContent
				}
				if len(chunk.ToolCalls) > 0 {
					event["tool_calls"] = chunk.ToolCalls
				}

				sendWS(conn, event)
			}
		}
	}
}

func sendWS(conn *websocket.Conn, data interface{}) {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := conn.WriteJSON(data); err != nil {
		slog.Warn("WebSocket write failed", "error", err)
	}
}

func sendWSError(conn *websocket.Conn, msg string) {
	sendWS(conn, map[string]interface{}{
		"type":  "error",
		"error": msg,
	})
}
