package kernel

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Process handles a user query synchronously by delegating to ProcessStream
// and collecting all stream chunks into a single Response.
// This eliminates ~200 lines of duplicated ReAct loop logic.
func (k *AgentKernel) Process(ctx context.Context, query *Query) (*Response, error) {
	start := time.Now()

	stream, err := k.ProcessStream(ctx, query)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	toolCalls := 0
	totalTokens := 0

	for chunk := range stream {
		switch chunk.Type {
		case ChunkTypeContent:
			content.WriteString(chunk.Content)
		case ChunkTypeToolCall:
			toolCalls++
			content.Reset() // tool round, reset for final answer
		case ChunkTypeDone:
			if chunk.Usage != nil {
				totalTokens = chunk.Usage.TotalTokens
			}
			if chunk.Content != "" {
				content.WriteString(chunk.Content)
			}
		case ChunkTypeError:
			if chunk.Error != nil {
				return nil, chunk.Error
			}
		}
	}

	model := ""
	if k.llmProvider != nil {
		model = k.llmProvider.GetModelID()
	}
	return &Response{
		Content:    content.String(),
		ToolCalls:  toolCalls,
		TokensUsed: totalTokens,
		Duration:   time.Since(start),
		Model:      model,
	}, nil
}

func (k *AgentKernel) doReflection(ctx context.Context, sessionID, query, response string, toolCalls, toolErrors int) {
	if k.reflection == nil {
		return
	}

	record := ExecutionRecord{
		Query:     query,
		Response:  response,
		Success:   toolErrors == 0,
		ToolCalls: make([]ToolCall, 0),
		Messages:  k.loadSessionMessages(ctx, sessionID),
		TaskType:  k.detectTaskType(ctx, query),
	}

	result, err := k.reflection.Reflect(ctx, sessionID, record)
	if err != nil {
		slog.Warn("Reflection failed", "error", err)
		return
	}

	// Infer user verdict from reflection — LLM analyzed the full conversation flow
	if result.Learned != "" && k.sessionStore != nil {
		verdict, cleanLearned := extractVerdictFromLearned(result.Learned)
		if verdict != "" {
			result.Learned = cleanLearned
			if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
				if session.Metadata == nil { session.Metadata = make(map[string]interface{}) }
				session.Metadata["user_verdict"] = verdict
				k.sessionStore.Update(ctx, session)
			}
		}
	}

	// Skill feedback: record quality for the activated skill
	if k.skillActor != nil {
		k.skillActor.RecordLastUsage(result.Quality)
	}

	// Self-Rewarding: update evaluation criteria from reflection suggestions

	// 存储反思结果到会话 + 知识使用反馈
	if result != nil && k.sessionStore != nil {
		session, err := k.sessionStore.Get(ctx, sessionID)
		if err == nil && session != nil {
			if session.Metadata == nil {
				session.Metadata = make(map[string]interface{})
			}
			session.Metadata["reflection"] = result
			if err := k.sessionStore.Update(ctx, session); err != nil {
		slog.Warn("session update failed", "error", err)
	}

			// 反馈：本次使用的知识质量如何（仅当 LLM 判定值得存）
			if k.cachedAnalysis.HasPostProcess("knowledge") && k.knowledgeCollector != nil {
				if docIDsRaw, ok := session.Metadata["knowledge_doc_ids"]; ok {
					if docIDs, ok := docIDsRaw.([]string); ok && len(docIDs) > 0 {
				if result.Learned != "" {
					k.knowledgeCollector.AddKnowledge(ctx, "lesson: "+query[:min(80, len(query))], result.Learned, "reflection", []string{"lesson", "reflection", "self-improvement"})
				}
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, float64(result.Quality)/10.0)
					}
				}
			}
		}
	}

	// 持久化学习洞察

	// 检测对话模式 + 技能提取（仅当 LLM 判定值得蒸馏）
	if k.cachedAnalysis.HasPostProcess("distill") && k.patternDetector != nil && k.sessionStore != nil {
		session, err := k.sessionStore.Get(ctx, sessionID)
		if err == nil && session != nil {
			patterns, pErr := k.patternDetector.Detect(ctx, sessionID, session.Messages)
			if pErr == nil && len(patterns) > 0 {
				if session.Metadata == nil {
					session.Metadata = make(map[string]interface{})
				}
				session.Metadata["patterns"] = patterns
				if err := k.sessionStore.Update(ctx, session); err != nil {
					slog.Warn("session update failed", "error", err)
				}

				// Auto-extract skills from patterns (sync within doReflection goroutine)
				if k.skillActor != nil {
					k.extractSkillsFromPatterns(context.WithoutCancel(ctx), patterns)
				}
			}
		}
	}

	// 自动知识抽取：质量门控通过后存入知识库（仅当 LLM 判定值得存）
	if k.cachedAnalysis.HasPostProcess("knowledge") {
		k.autoSaveKnowledge(ctx, sessionID, query, response, toolCalls, toolErrors)
	}
}

// autoSaveKnowledge 自动知识抽取 — 质量门控通过后存入知识库
func (k *AgentKernel) autoSaveKnowledge(ctx context.Context, sessionID, query, response string, toolCalls, toolErrors int) {
	if k.knowledgeCollector == nil {
		return
	}

	toolSuccesses := toolCalls - toolErrors
	toolFailures := toolErrors

	var reflectResult *ReflectionResult
	var userVerdict string // "good", "bad", or empty
	if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
		if ref, ok := session.Metadata["reflection"]; ok {
			if r, ok := ref.(*ReflectionResult); ok {
				reflectResult = r
				if r.Quality < 5 {
					toolFailures = 1 // 低质量标记
					toolSuccesses = 0
				}
			}
		}
		// User feedback — the only ground truth signal
		if v, ok := session.Metadata["user_verdict"].(string); ok {
			userVerdict = v
		}
	}

	// Apply user verdict directly to knowledge weights
	if userVerdict != "" && k.knowledgeCollector != nil && reflectResult != nil {
		if session, err := k.sessionStore.Get(ctx, sessionID); err == nil && session != nil {
			if docIDsRaw, ok := session.Metadata["knowledge_doc_ids"]; ok {
				if docIDs, ok := docIDsRaw.([]string); ok && len(docIDs) > 0 {
					if userVerdict == "good" {
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, 0.9) // boost
					} else if userVerdict == "bad" {
						k.knowledgeCollector.RecordKnowledgeUsage(ctx, docIDs, 0.2) // decay
					}
				}
			}
		}
	}

	// 质量门控判断
	if k.qualityGate != nil {
		if !k.qualityGate.Pass(query, response, toolSuccesses, toolFailures, reflectResult) {
			return
		}
	}

	// 存入知识库 — 使用精炼管道（去重 + LLM 精炼 + 结构化存储）
	if refiner, ok := k.knowledgeCollector.(interface {
		Refine(ctx context.Context, query, response, sessionID string) string
	}); ok {
		refiner.Refine(ctx, query, response, sessionID)
	} else if _, err := k.knowledgeCollector.AddKnowledge(ctx, query, response, "auto-extract", []string{"session:" + sessionID}); err != nil {
		slog.Debug("Auto knowledge save failed", "error", err)
	}
}

// extractSkillsFromPatterns analyzes detected patterns and auto-creates skills
// for recurring successful patterns.
func (k *AgentKernel) extractSkillsFromPatterns(ctx context.Context, patterns []Pattern) {
	if k.skillActor == nil || !k.distillEnabled {
		return
	}
	sd, _ := k.patternDetector.(*SemanticPatternDetector)

	for _, p := range patterns {
		p := p // capture loop variable for goroutine
		theme := strings.ToLower(strings.TrimSpace(p.Description))
		skillID := "auto-" + strings.ReplaceAll(strings.ReplaceAll(theme, " ", "-"), ":", "")
		if len(skillID) > 60 { skillID = skillID[:60] }
		skillName := capitalize(strings.TrimSpace(p.Description))
		if len(skillName) > 50 { skillName = skillName[:50] }

		keywords := tokenize(theme)
		if len(keywords) == 0 { keywords = strings.Fields(p.Type) }

		if k.llmProvider == nil {
			simplePrompt := fmt.Sprintf("Auto-detected from %d executions sharing theme: %s", p.Frequency, p.Description)
			k.skillActor.AddSkill(skillID, skillName, p.Description, simplePrompt, keywords)
			slog.Info("Auto-extracted skill", "id", skillID, "name", skillName, "frequency", p.Frequency)
			continue
		}

		// Get examples for this specific pattern's cluster
		examples := []clusterExample{}
		if sd != nil {
			examples = sd.GetExamplesByTheme(p.Description)
		}
		if len(examples) < 2 {
			slog.Debug("Cluster too small for distillation", "theme", p.Description, "examples", len(examples))
			continue
		}

		// Capture all values before the goroutine to avoid closure bugs
		sid, sname, desc, kw := skillID, skillName, p.Description, keywords
		go func() {
			distillCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			distilled := evaluateAndDistill(distillCtx, k.llmProvider, p, examples)
			if distilled == "" {
				slog.Debug("Cluster rejected by LLM", "theme", desc, "freq", p.Frequency)
				return
			}
			if sd != nil {
				sd.MarkDistilled(desc)
			}
			// Extract tool + file patterns from cluster
			skillCtx := extractSkillContext(examples)
			if len(skillCtx.Files) > 0 {
				distilled += "\n\n## Generated Scripts\nThese files are commonly created:\n"
				for _, f := range skillCtx.Files {
					distilled += fmt.Sprintf("- `%s`\n", f)
				}
			}
			k.skillActor.AddDistilledSkill(sid, sname, desc, distilled, kw, skillCtx.Tools)
			if k.knowledgeCollector != nil {
				k.knowledgeCollector.AddKnowledge(distillCtx,
					"pattern: "+sname, distilled, "distillation",
					append(kw, "auto-distilled", "pattern"))
			}
			slog.Info("Skill distilled", "id", sid, "name", sname, "freq", p.Frequency)
		}()
	}
}

func capitalize(s string) string {
	if s == "" { return s }
	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
	return string(runes)
}


// extractVerdictFromLearned parses the reflection's learned field for a verdict prefix.
// LLM is instructed to prefix with [good], [bad], or [neutral].
func extractVerdictFromLearned(learned string) (verdict string, clean string) {
	for _, v := range []string{"[good]", "[bad]", "[neutral]"} {
		if strings.HasPrefix(learned, v) {
			return v[1 : len(v)-1], strings.TrimSpace(learned[len(v):])
		}
	}
	return "", learned
}

// skillContext holds extracted patterns from a query cluster.
type skillContext struct {
	Tools []string // commonly used tools
	Files []string // commonly created/modified script files
}

// extractSkillContext scans cluster examples for tools and generated scripts.
func extractSkillContext(examples []clusterExample) skillContext {
	if len(examples) < 2 {
		return skillContext{}
	}
	var ctx skillContext

	// Count tools and files across examples
	toolCount := map[string]int{}
	fileCount := map[string]int{}
	for _, ex := range examples {
		lower := strings.ToLower(ex.response)
		for _, tool := range []string{"read_file", "search_files", "list_directory", "execute_command",
			"write_file", "diff_edit", "git_log", "git_diff", "git_status", "git_blame",
			"web_search", "web_fetch", "search_knowledge", "manage_memory"} {
			if strings.Contains(lower, tool) {
				toolCount[tool]++
			}
		}
		// Scan for reusable generated scripts — files that appear inside code blocks
		// or are explicitly marked as scripts/templates. Excludes regular coding files.
		inCodeBlock := false
		for _, line := range strings.Split(ex.response, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inCodeBlock = !inCodeBlock
				continue
			}
			if !inCodeBlock { continue } // only look inside code blocks
			lower := strings.ToLower(line)
			for _, ext := range []string{".sh", ".py", ".rb", ".js"} {
				if strings.Contains(lower, ext) {
					for _, word := range strings.Fields(line) {
						if strings.HasSuffix(strings.Trim(word, "`'\".,;:!?()[]{}"), ext) {
							fileCount[strings.Trim(word, "`'\".,;:!?()[]{}")]++
						}
					}
				}
			}
		}
	}

	threshold := len(examples) / 2
	if threshold < 2 { threshold = 2 }
	for tool, count := range toolCount {
		if count >= threshold {
			ctx.Tools = append(ctx.Tools, tool)
		}
	}
	for file, count := range fileCount {
		if count >= threshold {
			ctx.Files = append(ctx.Files, file)
		}
	}
	return ctx
}
