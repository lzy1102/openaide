package eval

// BuiltinTasks returns the standard benchmark task suite.
func BuiltinTasks() []Task {
	return []Task{
		// ── Easy tasks ──
		{
			ID: "explain-concurrency", Name: "Explain Go Concurrency",
			Category: "think", Difficulty: "easy",
			Query:        "Explain how goroutines and channels work in Go. Give a short example.",
			EvalCriteria: "Response explains goroutines (lightweight threads) and channels (communication mechanism) with a valid Go code example. Must mention both concepts.",
			MustNotContain: []string{"I don't know", "I'm not sure"},
		},
		{
			ID: "hello-response", Name: "Greeting Response",
			Category: "general", Difficulty: "easy",
			Query:        "Hello! What can you help me with?",
			EvalCriteria: "Friendly greeting response mentioning helpful capabilities. Identifies itself as OpenAIDE or an AI coding assistant.",
			MustNotContain: []string{"I can't", "not able"},
		},
		{
			ID: "simple-calc", Name: "Simple Calculation",
			Category: "coding", Difficulty: "easy",
			Query:        "What is the time complexity of binary search? Answer in one sentence.",
			EvalCriteria: "States that binary search has O(log n) time complexity. Answer is concise (one sentence).",
		},

		// ── Medium tasks ──
		{
			ID: "analyze-structure", Name: "Analyze Project Structure",
			Category: "think", Difficulty: "medium",
			Query:        "What are the main packages in this project? List their responsibilities briefly.",
			EvalCriteria: "Identifies key packages (kernel, tools, api, orchestration, llm, memory, knowledge) and describes each one's role. Shows understanding of the layered architecture. May use tools to explore the codebase.",
			MustNotContain: []string{"I don't know", "I haven't read"},
		},
		{
			ID: "code-review-pattern", Name: "Review Code Pattern",
			Category: "review", Difficulty: "medium",
			Query:        "Review this Go error handling pattern: if err != nil { return err }. Is it correct? What could go wrong?",
			EvalCriteria: "Explains that the pattern is syntactically correct but loses context — the error should be wrapped with fmt.Errorf to preserve call stack/context. Mentions error wrapping best practices in Go.",
		},
		{
			ID: "fix-hypothetical-bug", Name: "Fix Hypothetical Bug",
			Category: "coding", Difficulty: "medium",
			Query:        "A function returns nil instead of an error when a file is not found. What's the fix? Explain in 2-3 sentences.",
			EvalCriteria: "Identifies the bug (returning nil when error should be returned) and provides the correct fix (return the error, likely os.IsNotExist or similar). Answer is concise.",
			MustNotContain: []string{"I don't know"},
		},
		{
			ID: "explain-actor-model", Name: "Explain Actor Model",
			Category: "think", Difficulty: "medium",
			Query:        "Explain the CSP actor pattern used in OpenAIDE's kernel. What problem does it solve?",
			EvalCriteria: "Explains CSP (Communicating Sequential Processes) actor pattern: each module owns its data in a goroutine, communicates via channels. Mentions benefits: zero locks, no data races, simpler concurrency. Shows understanding of the pattern's value in concurrent systems.",
			MustNotContain: []string{"I'm not familiar"},
		},

		// ── Hard tasks ──
		{
			ID: "architecture-critique", Name: "Architecture Critique",
			Category: "review", Difficulty: "hard",
			Query:        "Analyze the layered architecture of OpenAIDE. What are the strengths and one potential weakness? Be specific.",
			EvalCriteria: "Describes the layered architecture of OpenAIDE with specific layers. Lists at least two strengths. References actual packages or design patterns used in the project. The response is self-contained and complete (not cut off mid-sentence).",
			MustNotContain: []string{"I don't know", "I can't"},
		},
		{
			ID: "refactoring-plan", Name: "Refactoring Plan",
			Category: "think", Difficulty: "hard",
			Query:        "If you were to split the kernel package into smaller sub-packages, what would you extract and why?",
			EvalCriteria: "Proposes concrete sub-package extractions (e.g. prompt/, react/, session/, reflection/) with reasoning for each. Mentions interfaces, dependency management. Shows understanding of the kernel's current structure.",
		},
		{
			ID: "multi-step-logic", Name: "Multi-step Logic",
			Category: "coding", Difficulty: "hard",
			Query:        "Design a rate limiter using the token bucket algorithm. Describe the key components and how they work together. Keep it under 200 words.",
			EvalCriteria: "Describes token bucket components: bucket with max tokens, refill rate, tokens consumed per request. Explains the flow: request arrives → check tokens available → consume if available → reject if empty → tokens refill over time. Concise (under ~200 words).",
		},
	}
}

// FullCapabilityTasks returns a comprehensive suite covering all agent capabilities.
// FullCapabilityTasks tests all agent functions
func FullCapabilityTasks() []Task {
	return []Task{
		// ── Reading & Understanding ──
		{ID: "read-explain", Name: "Read & Explain", Category: "think", Difficulty: "medium",
			Query:        "Read backend/internal/eval/eval.go and explain what Runner.RunTasks does. Be specific.",
			EvalCriteria: "Explains that RunTasks iterates over tasks, calls runOne for each, collects results into a Run struct. Mentions key details: result aggregation, timing, pass/fail tracking, and the Scorecard function. Shows understanding of the code structure by referencing specific fields or methods.",
			MinToolCalls: 1},

		// ── Searching ──
		{ID: "search-codebase", Name: "Search Codebase", Category: "think", Difficulty: "medium",
			Query:        "Search for all files that define a function with 'Reflection' in its name. List file paths.",
			EvalCriteria: "Uses search_files or grep to find Go files containing functions with 'Reflection' in the name. Lists specific file paths (should include llm_reflection.go at minimum). Results are concrete and verifiable.",
			MinToolCalls: 1},

		// ── File Writing ──
		{ID: "write-verify", Name: "Write & Verify", Category: "coding", Difficulty: "easy",
			Query:        "Create /tmp/openaide_test_write.txt with content 'acceptance test passed'. Then read it back to verify.",
			EvalCriteria: "Successfully creates a file at /tmp/openaide_test_write.txt with the specified content. Verbatim confirmation that the file was written OR reads it back to verify. The file content is confirmed to match.",
			MinToolCalls: 1},

		// ── Diff Editing ──
		{ID: "diff-edit-precise", Name: "Precise Diff Edit", Category: "coding", Difficulty: "medium",
			Query:        "First write a file /tmp/openaide_diff_target.txt with content 'line1\nline2\nline3'. Then use diff_edit to change 'line2' to 'line2-edited'. Finally read the file to confirm the change.",
			EvalCriteria: "Successfully creates /tmp/openaide_diff_target.txt, uses diff_edit to precisely change 'line2' to 'line2-edited', and verifies by reading the file. The final content must show 'line2-edited' on the second line with 'line1' and 'line3' unchanged.",
			MinToolCalls: 2},

		// ── Knowledge RAG ──
		{ID: "knowledge-search", Name: "Knowledge Search", Category: "think", Difficulty: "easy",
			Query:        "Use search_knowledge to find any stored facts about error handling or context propagation in this project.",
			EvalCriteria: "Uses the search_knowledge tool. Reports findings (may be empty if no facts stored yet, which is acceptable). Confirms the search was performed.",
			MinToolCalls: 1},

		// ── Memory Management ──
		{ID: "memory-store", Name: "Memory Store", Category: "general", Difficulty: "easy",
			Query:        "Store this fact using manage_memory(action='remember'): 'acceptance test fact: error handling uses fmt.Errorf with %w wrapping throughout the kernel'",
			EvalCriteria: "Uses the manage_memory tool with action='remember'. Confirms the fact was stored. The response acknowledges the memory operation completed.",
			MinToolCalls: 1},

		// ── Architecture Understanding ──
		{ID: "arch-synthesis", Name: "Architecture Synthesis", Category: "think", Difficulty: "hard",
			Query:        "Based on reading key files (CLAUDE.md, kernel/*.go), describe: (1) the layered architecture, (2) the learning pipeline, (3) the CSP actor pattern. Keep under 300 words.",
			EvalCriteria: "Reads key project files. Describes all three topics: (1) layered architecture from entry points to infra to kernel to tools, (2) learning pipeline with reflection → knowledge → pattern detection → distillation, (3) CSP actor pattern with goroutines and channels. Specific and accurate, not generic. Under ~300 words.",
			MinToolCalls: 2},
	}
}

// QuickTasks returns a fast subset for smoke testing.
func QuickTasks() []Task {
	tasks := BuiltinTasks()
	var quick []Task
	for _, t := range tasks {
		if t.Difficulty == "easy" {
			quick = append(quick, t)
		}
	}
	return quick
}

// CategoryTasks returns tasks filtered by category.
func CategoryTasks(category string) []Task {
	var filtered []Task
	for _, t := range BuiltinTasks() {
		if t.Category == category {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
