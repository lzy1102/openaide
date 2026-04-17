package services

// ChatMessage 聊天消息 (API 层)
type ChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// LLMMessage LLM 消息 (内部使用)
type LLMMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
