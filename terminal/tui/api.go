package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type APIResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type StreamEvent struct {
	Type     string `json:"type"`
	Content  string `json:"content"`
	Tool     string `json:"tool"`
	Params   string `json:"params"`
	Result   string `json:"result"`
	Model    string `json:"model"`
	Thinking string `json:"thinking"`
	Done     bool   `json:"done"`
}

func FetchModels(apiURL string) ([]Model, error) {
	data, err := makeRequest("GET", apiURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	var models []Model
	if err := unwrapResponse(data, &models); err != nil {
		return nil, err
	}
	return models, nil
}

func FetchDialogues(apiURL string, userID string) ([]Dialogue, error) {
	data, err := makeRequest("GET", apiURL+"/dialogues/user/"+userID, nil)
	if err != nil {
		return nil, err
	}
	var dialogues []Dialogue
	if err := unwrapResponse(data, &dialogues); err != nil {
		var pageResp struct {
			Items []Dialogue `json:"items"`
			Total int64      `json:"total"`
		}
		if err2 := unwrapResponse(data, &pageResp); err2 != nil {
			return nil, err
		}
		dialogues = pageResp.Items
	}
	return dialogues, nil
}

func FetchMessages(apiURL, dialogueID string) ([]Message, error) {
	data, err := makeRequest("GET", apiURL+"/dialogues/"+dialogueID+"/messages", nil)
	if err != nil {
		return nil, err
	}
	var messages []Message
	if err := unwrapResponse(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func CreateDialogue(apiURL string) (Dialogue, error) {
	reqBody := map[string]interface{}{
		"user_id": "cli-user",
		"title":   "CLI Chat",
	}
	data, err := makeRequest("POST", apiURL+"/dialogues", reqBody)
	if err != nil {
		return Dialogue{}, err
	}
	var result Dialogue
	if err := unwrapResponse(data, &result); err != nil {
		return Dialogue{}, err
	}
	return result, nil
}

func CompactContext(apiURL, dialogueID string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
	}
	data, err := makeRequest("POST", apiURL+"/context/compress", reqBody)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func CompactContextStructured(apiURL, dialogueID string, customInstructions string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
	}
	if customInstructions != "" {
		reqBody["custom_instructions"] = customInstructions
	}
	data, err := makeRequest("POST", apiURL+"/compact/structured", reqBody)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if apiResp.Data != nil {
		var result map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func SetToolMode(apiURL, mode string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"mode": mode,
	}
	data, err := makeRequest("POST", apiURL+"/tool-mode/set", reqBody)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	json.Unmarshal(data, &result)
	return result, nil
}

func GetCurrentToolMode(apiURL string) string {
	data, err := makeRequest("GET", apiURL+"/tool-mode/current", nil)
	if err != nil {
		return "build"
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return "build"
	}
	if apiResp.Data != nil {
		var result map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &result); err == nil {
			if mode, ok := result["mode"].(string); ok {
				return mode
			}
		}
	}
	return "build"
}

func ClearMessages(apiURL, dialogueID string) error {
	_, err := makeRequest("DELETE", apiURL+"/dialogues/"+dialogueID+"/messages", nil)
	return err
}

func SendMessage(apiURL, dialogueID string, content, model string) (string, error) {
	reqBody := map[string]interface{}{
		"user_id":     "cli-user",
		"content":     content,
		"model_id":    model,
		"dialogue_id": dialogueID,
	}
	endpoint := apiURL + "/chat/tools"
	data, err := makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return "", err
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := unwrapResponse(data, &resp); err != nil {
		var rawResp map[string]interface{}
		if json.Unmarshal(data, &rawResp) == nil {
			if c, ok := rawResp["content"].(string); ok {
				return c, nil
			}
		}
		return string(data), nil
	}
	return resp.Content, nil
}

type StreamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type StreamCallbacks struct {
	OnThinking      func(content string)
	OnToolCall      func(tool string, params string)
	OnToolDone      func(tool string, result string)
	OnContent       func(chunk string)
	OnDone          func(model string, usage *StreamUsage)
	OnCompact       func(reason string, beforeMsgs, afterMsgs, savedTokens int)
	OnGuardianReview func(tool string, verdict string, riskLevel string, reason string)
}

func SendMessageStream(ctx context.Context, apiURL, dialogueID string, content, model string, timeoutSec int, cb *StreamCallbacks) (string, error) {
	reqBody := map[string]interface{}{
		"user_id":    "cli-user",
		"content":    content,
		"model_id":   model,
		"dialogue_id": dialogueID,
	}

	endpoint := apiURL + "/chat/route"

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(jsonData)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	addAuthHeader(req)

	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var fullResponse strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") || strings.HasPrefix(line, "data: ") {
			var data string
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			} else {
				data = strings.TrimPrefix(line, "data:")
			}
			if data == "[DONE]" {
				if cb != nil && cb.OnDone != nil {
					cb.OnDone(model, nil)
				}
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			eventType, _ := chunk["type"].(string)

			switch eventType {
			case "thinking":
				if cb != nil && cb.OnThinking != nil {
					if thinking, ok := chunk["content"].(string); ok {
						cb.OnThinking(thinking)
					}
				}
			case "tool_call":
				if cb != nil && cb.OnToolCall != nil {
					tool, _ := chunk["tool"].(string)
					params, _ := chunk["params"].(string)
					cb.OnToolCall(tool, params)
				}
			case "tool_done":
				if cb != nil && cb.OnToolDone != nil {
					tool, _ := chunk["tool"].(string)
					result, _ := chunk["result"].(string)
					cb.OnToolDone(tool, result)
				}
			case "context_compact":
				if cb != nil && cb.OnCompact != nil {
					reason, _ := chunk["reason"].(string)
					beforeMsgs := 0
					afterMsgs := 0
					savedTokens := 0
					if v, ok := chunk["before_msgs"].(float64); ok {
						beforeMsgs = int(v)
					}
					if v, ok := chunk["after_msgs"].(float64); ok {
						afterMsgs = int(v)
					}
					if v, ok := chunk["saved_tokens"].(float64); ok {
						savedTokens = int(v)
					}
					cb.OnCompact(reason, beforeMsgs, afterMsgs, savedTokens)
				}
			case "guardian_review":
				if cb != nil && cb.OnGuardianReview != nil {
					tool, _ := chunk["tool"].(string)
					verdict, _ := chunk["verdict"].(string)
					riskLevel, _ := chunk["risk_level"].(string)
					reason, _ := chunk["reason"].(string)
					cb.OnGuardianReview(tool, verdict, riskLevel, reason)
				}
			case "content":
				if content, ok := chunk["content"].(string); ok {
					if cb != nil && cb.OnContent != nil {
						cb.OnContent(content)
					}
					fullResponse.WriteString(content)
				}
			case "done":
				if cb != nil && cb.OnDone != nil {
					m, _ := chunk["model"].(string)
					var usage *StreamUsage
					if usageRaw, ok := chunk["usage"]; ok {
						usageBytes, _ := json.Marshal(usageRaw)
						var u StreamUsage
						if json.Unmarshal(usageBytes, &u) == nil {
							usage = &u
						}
					}
					cb.OnDone(m, usage)
				}
				return fullResponse.String(), nil
			case "error":
				errMsg, _ := chunk["content"].(string)
				if errMsg != "" {
					return fullResponse.String(), fmt.Errorf("%s", errMsg)
				}
			default:
				if content, ok := chunk["content"].(string); ok {
					if eventType == "" && content != "" {
						if cb != nil && cb.OnContent != nil {
							cb.OnContent(content)
						}
						fullResponse.WriteString(content)
					}
				}
			}
		}
	}

	return fullResponse.String(), nil
}

func unwrapResponse(data []byte, target interface{}) error {
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return json.Unmarshal(data, target)
	}
	if apiResp.Code != 0 {
		return fmt.Errorf("API error: %s", apiResp.Message)
	}
	if apiResp.Data != nil {
		return json.Unmarshal(apiResp.Data, target)
	}
	return nil
}

func ForkSession(apiURL, dialogueID, userID, name string, branchPoint int) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"dialogue_id":  dialogueID,
		"user_id":      userID,
		"name":         name,
		"branch_point": branchPoint,
	}
	data, err := makeRequest("POST", apiURL+"/session-branches/fork", reqBody)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func ListBranches(apiURL, dialogueID string) ([]map[string]interface{}, error) {
	data, err := makeRequest("GET", apiURL+"/session-branches/list/"+dialogueID, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func GetPersistentMemories(apiURL, userID string) ([]map[string]interface{}, error) {
	data, err := makeRequest("GET", apiURL+"/persistent-memories?user_id="+userID, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func RememberPersistent(apiURL, userID, category, key, value string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"user_id":  userID,
		"category": category,
		"key":      key,
		"value":    value,
		"source":   "cli",
	}
	data, err := makeRequest("POST", apiURL+"/persistent-memories", reqBody)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

func GetCostSummary(apiURL, userID string, days int) (map[string]interface{}, error) {
	data, err := makeRequest("GET", fmt.Sprintf("%s/cost/summary?user_id=%s&days=%d", apiURL, userID, days), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

func EvaluateExecPolicy(apiURL, command string) (map[string]interface{}, error) {
	reqBody := map[string]interface{}{"command": command}
	data, err := makeRequest("POST", apiURL+"/exec-policy/evaluate", reqBody)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(data, &resp)
	return resp.Data, nil
}

type GuardianReviewResult struct {
	Verdict   string `json:"verdict"`
	RiskLevel string `json:"risk_level"`
	Reason    string `json:"reason"`
}

func RequestGuardianReview(apiURL, toolName, arguments, contextStr string) (*GuardianReviewResult, error) {
	reqBody := map[string]interface{}{
		"tool":     toolName,
		"args":     arguments,
		"context":  contextStr,
	}
	data, err := makeRequest("POST", apiURL+"/guardian/review", reqBody)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if apiResp.Data != nil {
		var result GuardianReviewResult
		if err := json.Unmarshal(apiResp.Data, &result); err != nil {
			return nil, err
		}
		return &result, nil
	}
	return nil, fmt.Errorf("no data in guardian response")
}

func makeRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var req *http.Request
	var err error

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		req, err = http.NewRequest(method, endpoint, strings.NewReader(string(jsonData)))
	} else {
		req, err = http.NewRequest(method, endpoint, nil)
	}
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	addAuthHeader(req)

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication required: set api.token in config or enable server.local_mode")
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(data))
	}

	return data, nil
}

func addAuthHeader(req *http.Request) {
	if token := GetAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
