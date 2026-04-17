package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 卡片 JSON 构建辅助函数

// buildThinkingCard 构建初始"思考中"卡片
func buildThinkingCard() string {
	return buildCard("thinking", "🤔 思考中...", "正在为你生成回复...", "")
}

// buildStreamCard 构建流式追加卡片（patch_mode=1）
func buildStreamCard(content string) string {
	return buildCard("stream", "✍️ 回复中...", "", content)
}

// buildFinalCard 构建最终完成卡片（patch_mode=2，绿色标题）
func buildFinalCard(content string, duration string) string {
	return buildCard("final", "✅ 回复完成", duration, content)
}

// buildErrorCard 构建错误卡片（红色标题）
func buildErrorCard(errMsg string) string {
	return buildCard("error", "❌ 回复失败", "", errMsg)
}

// buildCard 构建飞书卡片 JSON
func buildCard(cardType, headerTitle, headerSubTitle, bodyContent string) string {
	var headerTemplate, color string

	switch cardType {
	case "thinking":
		color = "blue"
		headerTemplate = buildHeader(headerTitle, headerSubTitle, color)
		return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":[{"tag":"div","text":{"tag":"lark_md","content":"正在思考，请稍候..."}}]}`, headerTemplate)
	case "stream":
		color = "blue"
		headerTemplate = buildHeader(headerTitle, "", color)
		return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":[{"tag":"div","text":{"tag":"lark_md","content":"%s"}}]}`, headerTemplate, escapeJSON(bodyContent))
	case "final":
		color = "green"
		headerTemplate = buildHeader(headerTitle, headerSubTitle, color)
		return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":[{"tag":"div","text":{"tag":"lark_md","content":"%s"}}]}`, headerTemplate, escapeJSON(bodyContent))
	case "error":
		color = "red"
		headerTemplate = buildHeader(headerTitle, "", color)
		return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":[{"tag":"div","text":{"tag":"lark_md","content":"%s"}}]}`, headerTemplate, escapeJSON(bodyContent))
	default:
		return `{"config":{"wide_screen_mode":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"empty"}}]}`
	}
}

// buildHeader 构建卡片头部
func buildHeader(title, subtitle, color string) string {
	h := map[string]interface{}{
		"title": map[string]string{
			"tag":     "plain_text",
			"content": title,
		},
		"template": color,
	}
	if subtitle != "" {
		h["subtitle"] = map[string]string{
			"tag":     "plain_text",
			"content": subtitle,
		}
	}
	data, _ := json.Marshal(h)
	return string(data)
}

// escapeJSON 转义 JSON 特殊字符
func escapeJSON(s string) string {
	data, _ := json.Marshal(s)
	// json.Marshal 会加上前后引号，去掉
	str := string(data)
	if len(str) >= 2 {
		return str[1 : len(str)-1]
	}
	return str
}

// escapeMarkdown 转义飞书 Markdown 特殊字符
func escapeMarkdown(s string) string {
	// 飞书 Markdown 中需要转义的特殊字符
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// buildFeedbackCard 构建带反馈按钮的最终卡片
func buildFeedbackCard(content, duration, dialogueID string) string {
	headerTemplate := buildHeader("✅ 回复完成", duration, "green")

	elements := fmt.Sprintf(`[
		{"tag":"div","text":{"tag":"lark_md","content":"%s"}},
		{"tag":"hr"},
		{"tag":"action","actions":[
			{"tag":"button","text":{"tag":"plain_text","content":"👍 有帮助"},"type":"primary","value":{"action":"feedback_positive","dialogue_id":"%s","rating":"5"}},
			{"tag":"button","text":{"tag":"plain_text","content":"👎 需改进"},"type":"default","value":{"action":"feedback_negative","dialogue_id":"%s","rating":"2"}}
		]}
	]`, escapeJSON(content), escapeJSON(dialogueID), escapeJSON(dialogueID))

	return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":%s}`, headerTemplate, elements)
}

// buildFeedbackAckCard 构建反馈确认卡片
func buildFeedbackAckCard(action string) string {
	emoji := "👍"
	text := "感谢你的反馈！我们会继续改进。"
	if action == "feedback_negative" {
		emoji = "👎"
		text = "感谢你的反馈！我们会努力改进。"
	}

	headerTemplate := buildHeader(fmt.Sprintf("%s 感谢反馈", emoji), "", "green")
	elements := fmt.Sprintf(`[{"tag":"div","text":{"tag":"lark_md","content":"%s"}}]`, escapeJSON(text))

	return fmt.Sprintf(`{"config":{"wide_screen_mode":true},"header":%s,"elements":%s}`, headerTemplate, elements)
}
