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

type StreamEvent struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Tool    string `json:"tool"`
	Params  string `json:"params"`
	Result  string `json:"result"`
	Model   string `json:"model"`
	Thinking string `json:"thinking"`
	Done    bool   `json:"done"`
}

func FetchModels(apiURL string) ([]Model, error) {
	data, err := makeRequest("GET", apiURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	var models []Model
	if err := json.Unmarshal(data, &models); err != nil {
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
	if err := json.Unmarshal(data, &dialogues); err != nil {
		return nil, err
	}
	return dialogues, nil
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
	if err := json.Unmarshal(data, &result); err != nil {
		return Dialogue{}, err
	}
	return result, nil
}

func SendMessage(apiURL, dialogueID string, content, model string) (string, error) {
	reqBody := map[string]interface{}{
		"user_id":  "cli-user",
		"content":  content,
		"model_id": model,
	}
	endpoint := apiURL + "/dialogues/" + dialogueID + "/messages"
	data, err := makeRequest("POST", endpoint, reqBody)
	if err != nil {
		return "", err
	}
	var resp ChatResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return string(data), nil
	}
	return resp.Message.Content, nil
}

type StreamCallbacks struct {
	OnThinking func(content string)
	OnToolCall func(tool string, params string)
	OnToolDone func(tool string, result string)
	OnContent  func(chunk string)
	OnDone     func(model string)
}

func SendMessageStream(ctx context.Context, apiURL, dialogueID string, content, model string, timeoutSec int, cb *StreamCallbacks) (string, error) {
	reqBody := map[string]interface{}{
		"user_id":  "cli-user",
		"content":  content,
		"model_id": model,
	}

	endpoint := apiURL + "/dialogues/" + dialogueID + "/stream"

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
					cb.OnDone(model)
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
					cb.OnDone(m)
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

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error: %d - %s", resp.StatusCode, string(data))
	}

	return data, nil
}
