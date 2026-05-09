package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MemoryType string

const (
	MemoryTypeUser     MemoryType = "user"
	MemoryTypeFeedback MemoryType = "feedback"
	MemoryTypeProject  MemoryType = "project"
	MemoryTypeReference MemoryType = "reference"
)

type MemoryEntry struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        MemoryType `json:"type"`
	Content     string     `json:"content"`
	Why         string     `json:"why,omitempty"`
	HowToApply  string     `json:"how_to_apply,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	IsPrivate   bool       `json:"is_private"`
}

func memoryDir() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, ".openaide", "memory")
}

func claudeMDPath() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, "OPENAIDE.md")
}

func rulesDir() string {
	wd, _ := os.Getwd()
	return filepath.Join(wd, ".openaide", "rules")
}

func LoadProjectMemory() string {
	var parts []string

	if content, err := os.ReadFile(claudeMDPath()); err == nil {
		trimmed := strings.TrimSpace(string(content))
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	rulesDir := rulesDir()
	if entries, err := os.ReadDir(rulesDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(rulesDir, entry.Name()))
			if err != nil {
				continue
			}
			trimmed := strings.TrimSpace(string(content))
			if trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n---\n\n")
}

func SaveProjectMemory(content string) error {
	path := claudeMDPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func InitProjectMemory() error {
	path := claudeMDPath()
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	template := `# Project Context

## Tech Stack
<!-- Describe your tech stack here -->

## Code Style
<!-- Add your code style preferences -->

## Common Commands
<!-- Add frequently used commands -->
<!-- Example: Run tests: go test ./... -->

## Important Rules
<!-- Add rules the assistant should always follow -->

## Directory Structure
<!-- Describe key directories and their purposes -->
`
	return SaveProjectMemory(template)
}

func LoadMemoryEntries() []MemoryEntry {
	memDir := memoryDir()
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return nil
	}

	var result []MemoryEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(memDir, entry.Name()))
		if err != nil {
			continue
		}
		me := parseMemoryFile(string(content))
		me.Name = strings.TrimSuffix(entry.Name(), ".md")
		result = append(result, me)
	}
	return result
}

func SaveMemoryEntry(entry MemoryEntry) error {
	memDir := memoryDir()
	if err := os.MkdirAll(memDir, 0755); err != nil {
		return err
	}

	filename := string(entry.Type) + "_" + sanitizeName(entry.Name) + ".md"
	path := filepath.Join(memDir, filename)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("---\nname: %s\ndescription: %s\ntype: %s\nprivate: %v\ncreated: %s\n---\n",
		entry.Name, entry.Description, string(entry.Type), entry.IsPrivate, entry.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(entry.Content)
	if entry.Why != "" {
		sb.WriteString(fmt.Sprintf("\n\n**Why:** %s", entry.Why))
	}
	if entry.HowToApply != "" {
		sb.WriteString(fmt.Sprintf("\n\n**How to apply:** %s", entry.HowToApply))
	}

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func DeleteMemoryEntry(name string) error {
	memDir := memoryDir()
	entries, err := os.ReadDir(memDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), name) || strings.TrimSuffix(entry.Name(), ".md") == name {
			return os.Remove(filepath.Join(memDir, entry.Name()))
		}
	}
	return fmt.Errorf("memory entry not found: %s", name)
}

func parseMemoryFile(content string) MemoryEntry {
	me := MemoryEntry{
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if !strings.HasPrefix(content, "---") {
		me.Content = strings.TrimSpace(content)
		return me
	}

	end := strings.Index(content[3:], "---")
	if end < 0 {
		me.Content = strings.TrimSpace(content)
		return me
	}

	frontmatter := content[3 : end+3]
	me.Content = strings.TrimSpace(content[end+6:])

	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "name":
			me.Name = val
		case "description":
			me.Description = val
		case "type":
			me.Type = MemoryType(val)
		case "private":
			me.IsPrivate = val == "true"
		case "created":
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				me.CreatedAt = t
			}
		}
	}

	if idx := strings.Index(me.Content, "**Why:**"); idx >= 0 {
		whyStart := idx + len("**Why:**")
		howIdx := strings.Index(me.Content[whyStart:], "**How to apply:**")
		if howIdx >= 0 {
			me.Why = strings.TrimSpace(me.Content[whyStart : whyStart+howIdx])
			applyStart := whyStart + howIdx + len("**How to apply:**")
			me.HowToApply = strings.TrimSpace(me.Content[applyStart:])
			me.Content = strings.TrimSpace(me.Content[:idx])
		} else {
			me.Why = strings.TrimSpace(me.Content[whyStart:])
			me.Content = strings.TrimSpace(me.Content[:idx])
		}
	}

	return me
}

func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}

func FetchRelevantMemories(apiURL, query string, limit int) ([]map[string]interface{}, error) {
	reqBody := map[string]interface{}{
		"query":  query,
		"limit":  limit,
		"userId": "cli-user",
	}
	data, err := makeRequest("POST", apiURL+"/memories/search", reqBody)
	if err != nil {
		return nil, err
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, err
	}
	if apiResp.Data != nil {
		var result []map[string]interface{}
		if err := json.Unmarshal(apiResp.Data, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, nil
}

func TriggerMemoryExtraction(apiURL, dialogueID string) error {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
		"user_id":     "cli-user",
	}
	_, err := makeRequest("POST", apiURL+"/memories/extract", reqBody)
	return err
}

func SaveFeedback(apiURL, dialogueID, messageID, feedbackType, comment string) error {
	reqBody := map[string]interface{}{
		"dialogue_id": dialogueID,
		"message_id":  messageID,
		"user_id":     "cli-user",
		"type":        feedbackType,
		"comment":     comment,
	}
	_, err := makeRequest("POST", apiURL+"/feedback", reqBody)
	return err
}
