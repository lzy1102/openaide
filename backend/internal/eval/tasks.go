package eval

// BuiltinTasks returns the standard benchmark task suite (30+ tasks).
func BuiltinTasks() []Task {
	return []Task{
		// ══════════════════════════════════════════════════════════════
		// EASY (8 tasks)
		// ══════════════════════════════════════════════════════════════

		// ── General ──
		{
			ID: "hello-response", Name: "Greeting Response",
			Category: "general", Difficulty: "easy",
			Query:        "Hello! What can you help me with?",
			EvalCriteria: "Friendly greeting response mentioning helpful capabilities. Identifies itself as OpenAIDE or an AI coding assistant.",
			MustNotContain: []string{"I can't", "not able"},
		},
		{
			ID: "simple-calc", Name: "Simple Calculation",
			Category: "general", Difficulty: "easy",
			Query:        "What is the time complexity of binary search? Answer in one sentence.",
			EvalCriteria: "States that binary search has O(log n) time complexity. Answer is concise (one sentence).",
		},

		// ── Think ──
		{
			ID: "explain-concurrency", Name: "Explain Go Concurrency",
			Category: "think", Difficulty: "easy",
			Query:        "Explain how goroutines and channels work in Go. Give a short example.",
			EvalCriteria: "Response explains goroutines (lightweight threads) and channels (communication mechanism) with a valid Go code example. Must mention both concepts.",
			MustNotContain: []string{"I don't know", "I'm not sure"},
		},

		// ── Coding ──
		{
			ID: "write-verify", Name: "Write & Verify",
			Category: "coding", Difficulty: "easy",
			Query:        "Create /tmp/openaide_test_write.txt with content 'acceptance test passed'. Then read it back to verify.",
			EvalCriteria: "Successfully creates a file at /tmp/openaide_test_write.txt with the specified content. Verbatim confirmation that the file was written OR reads it back to verify. The file content is confirmed to match.",
			MinToolCalls: 1,
		},
		{
			ID: "memory-store", Name: "Memory Store",
			Category: "coding", Difficulty: "easy",
			Query:        "Store this fact using manage_memory(action='remember'): 'acceptance test fact: error handling uses fmt.Errorf with %w wrapping throughout the kernel'",
			EvalCriteria: "Uses the manage_memory tool with action='remember'. Confirms the fact was stored. The response acknowledges the memory operation completed.",
			MinToolCalls: 1,
		},

		// ── Review ──
		{
			ID: "review-nil-check", Name: "Review Nil Check",
			Category: "review", Difficulty: "easy",
			Query:        "Is this Go code safe? `func getVal(m map[string]int, key string) int { return m[key] }`",
			EvalCriteria: "Explains that reading from a nil map in Go returns the zero value (0 for int), so it won't panic. But writing to a nil map WILL panic. Should mention that if the map could be nil, a nil check is needed before write operations.",
		},

		// ── OpenAIDE-specific Easy ──
		{
			ID: "oa-read-file", Name: "Read OpenAIDE Config",
			Category: "think", Difficulty: "easy",
			Query:        "Read the file CLAUDE.md in the project root and tell me: what language is this project written in, and what is its main purpose?",
			EvalCriteria: "States the project is written in Go. Identifies the main purpose as an AI agent kernel/platform. References specific details from CLAUDE.md (architecture, features, etc.).",
			MinToolCalls: 1,
		},
		{
			ID: "oa-list-tools", Name: "List Agent Tools",
			Category: "think", Difficulty: "easy",
			Query:        "List 5 built-in tools available in OpenAIDE's tools/ directory. Just list the tool names.",
			EvalCriteria: "Lists at least 5 tool names that exist in the codebase (e.g., read_file, write_file, execute_command, search_files, diff_edit, git_status, web_search, etc.). Tools must be real, not made up.",
			MinToolCalls: 1,
		},

		// ══════════════════════════════════════════════════════════════
		// MEDIUM (14 tasks)
		// ══════════════════════════════════════════════════════════════

		// ── General Medium ──
		{
			ID: "design-pattern", Name: "Design Pattern Question",
			Category: "general", Difficulty: "medium",
			Query:        "When would you use the Strategy pattern vs the Builder pattern? Give one concrete example for each.",
			EvalCriteria: "Strategy: used when you need to swap algorithms at runtime (e.g., different sorting strategies). Builder: used when constructing complex objects step by step (e.g., building an HTTP request). Examples are concrete and distinct.",
		},
		{
			ID: "explain-docker", Name: "Explain Docker",
			Category: "general", Difficulty: "medium",
			Query:        "What is the difference between a Docker image and a Docker container? Explain in 3 sentences.",
			EvalCriteria: "Image is a read-only template/blueprint. Container is a running instance of an image. Images are stored in registries, containers are ephemeral runtime instances. All three concepts are correctly distinguished.",
		},

		// ── Think Medium ──
		{
			ID: "analyze-structure", Name: "Analyze Project Structure",
			Category: "think", Difficulty: "medium",
			Query:        "What are the main packages in this project? List their responsibilities briefly.",
			EvalCriteria: "Identifies key packages (kernel, tools, api, orchestration, llm, memory) and describes each one's role. Shows understanding of the layered architecture. May use tools to explore the codebase.",
			MustNotContain: []string{"I don't know", "I haven't read"},
		},
		{
			ID: "explain-actor-model", Name: "Explain Actor Model",
			Category: "think", Difficulty: "medium",
			Query:        "Explain the CSP actor pattern used in OpenAIDE's kernel. What problem does it solve?",
			EvalCriteria: "Explains CSP (Communicating Sequential Processes) actor pattern: each module owns its data in a goroutine, communicates via channels. Mentions benefits: zero locks, no data races, simpler concurrency. Shows understanding of the pattern's value in concurrent systems.",
			MustNotContain: []string{"I'm not familiar"},
		},
		{
			ID: "read-explain", Name: "Read & Explain Code",
			Category: "think", Difficulty: "medium",
			Query:        "Read backend/internal/eval/eval.go and explain what Runner.RunTasks does. Be specific.",
			EvalCriteria: "Explains that RunTasks iterates over tasks, calls runOne for each, collects results into a Run struct. Mentions key details: result aggregation, timing, pass/fail tracking. Shows understanding of the code structure.",
			MinToolCalls: 1,
		},

		// ── Coding Medium ──
		{
			ID: "code-review-pattern", Name: "Review Code Pattern",
			Category: "coding", Difficulty: "medium",
			Query:        "Review this Go error handling pattern: `if err != nil { return err }`. Is it correct? What could go wrong?",
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
			ID: "diff-edit-precise", Name: "Precise Diff Edit",
			Category: "coding", Difficulty: "medium",
			Query:        "First write a file /tmp/openaide_diff_target.txt with content 'line1\\nline2\\nline3'. Then use diff_edit to change 'line2' to 'line2-edited'. Finally read the file to confirm the change.",
			EvalCriteria: "Successfully creates the file, uses diff_edit to change 'line2' to 'line2-edited', and verifies by reading the file. The final content must show 'line2-edited' on the second line with 'line1' and 'line3' unchanged.",
			MinToolCalls: 2,
		},
		{
			ID: "search-codebase", Name: "Search Codebase",
			Category: "coding", Difficulty: "medium",
			Query:        "Search for all files that define a function with 'Reflection' in its name. List file paths.",
			EvalCriteria: "Uses search_files or grep to find Go files containing functions with 'Reflection' in the name. Lists specific file paths (should include llm_reflection.go at minimum). Results are concrete and verifiable.",
			MinToolCalls: 1,
		},

		// ── Review Medium ──
		{
			ID: "review-race-condition", Name: "Review Race Condition",
			Category: "review", Difficulty: "medium",
			Query:        "Is this code thread-safe? `var counter int; func inc() { counter++ }` If multiple goroutines call inc(), what happens?",
			EvalCriteria: "Identifies the race condition: counter++ is not atomic (read-modify-write). Multiple goroutines can interleave, causing lost updates. Solutions: sync.Mutex, sync/atomic, or channel-based synchronization.",
		},
		{
			ID: "review-sql-injection", Name: "Review SQL Injection",
			Category: "review", Difficulty: "medium",
			Query:        "Is this Go code vulnerable to SQL injection? `db.Query(\"SELECT * FROM users WHERE name = '\" + name + \"'\")`",
			EvalCriteria: "Identifies SQL injection vulnerability: string concatenation allows attacker to inject SQL. Solution: use parameterized queries (db.Query(\"SELECT * FROM users WHERE name = ?\", name)). Explains why parameterized queries prevent injection.",
		},

		// ── OpenAIDE-specific Medium ──
		{
			ID: "oa-prompt-layers", Name: "Explain Prompt Layers",
			Category: "think", Difficulty: "medium",
			Query:        "Read kernel/kernel_prompt.go and explain the layered prompt system (L0-L5). What does each layer contain and when is it injected?",
			EvalCriteria: "Explains: L0 = core rules (always present), L1 = project context, L2 = skill injection, L3 = mode signal (per-query), L5 = reflection (from last task). Describes when each layer is injected. References specific code or comments.",
			MinToolCalls: 1,
		},
		{
			ID: "oa-tool-execution", Name: "Explain Tool Execution",
			Category: "think", Difficulty: "medium",
			Query:        "How does OpenAIDE execute tool calls in parallel? Read kernel/kernel_react.go and explain the partitionToolCalls and executeToolBatch functions.",
			EvalCriteria: "Explains that partitionToolCalls separates tools into parallel-safe (read-only) and serial (write) batches. executeToolBatch runs parallel-safe tools concurrently using goroutines. Mentions the parallelSafeTools map. Shows understanding of the concurrency model.",
			MinToolCalls: 1,
		},

		// ══════════════════════════════════════════════════════════════
		// HARD (10 tasks)
		// ══════════════════════════════════════════════════════════════

		// ── Think Hard ──
		{
			ID: "architecture-critique", Name: "Architecture Critique",
			Category: "think", Difficulty: "hard",
			Query:        "Analyze the layered architecture of OpenAIDE. What are the strengths and one potential weakness? Be specific.",
			EvalCriteria: "Describes the layered architecture with specific layers. Lists at least two strengths. References actual packages or design patterns. The response is self-contained and complete.",
			MustNotContain: []string{"I don't know", "I can't"},
		},
		{
			ID: "refactoring-plan", Name: "Refactoring Plan",
			Category: "think", Difficulty: "hard",
			Query:        "If you were to split the kernel package into smaller sub-packages, what would you extract and why?",
			EvalCriteria: "Proposes concrete sub-package extractions (e.g. prompt/, react/, session/, reflection/) with reasoning for each. Mentions interfaces, dependency management. Shows understanding of the kernel's current structure.",
		},
		{
			ID: "trace-data-flow", Name: "Trace Data Flow",
			Category: "think", Difficulty: "hard",
			Query:        "Trace the flow of a user query from entry (CLI or API) through to the final response. What are the key transformation points? Read the relevant files.",
			EvalCriteria: "Traces: entry point (cmd/cli or api) → buildMessages → ProcessStream → ReAct loop (LLM call → tool execution → observe) → finalizeResponse. Mentions key files: kernel_stream.go, kernel_react.go, kernel_prompt.go. Shows understanding of the full pipeline.",
			MinToolCalls: 2,
		},

		// ── Coding Hard ──
		{
			ID: "multi-step-logic", Name: "Multi-step Logic",
			Category: "coding", Difficulty: "hard",
			Query:        "Design a rate limiter using the token bucket algorithm. Describe the key components and how they work together. Keep it under 200 words.",
			EvalCriteria: "Describes token bucket components: bucket with max tokens, refill rate, tokens consumed per request. Explains the flow: request arrives → check tokens available → consume if available → reject if empty → tokens refill over time. Concise (under ~200 words).",
		},
		{
			ID: "concurrent-safe-map", Name: "Concurrent-safe Map",
			Category: "coding", Difficulty: "hard",
			Query:        "Implement a thread-safe map in Go using channels (CSP pattern). The map should support Get, Set, and Delete operations. Show the struct and key methods.",
			EvalCriteria: "Implements a channel-based map (actor pattern): a struct with a request channel, a goroutine that owns the map data, and methods that send requests through the channel. Shows understanding of CSP: no locks, goroutine owns state, external access via channels.",
		},
		{
			ID: "optimize-algorithm", Name: "Optimize Algorithm",
			Category: "coding", Difficulty: "hard",
			Query:        "Given a slice of integers, find the longest subarray where the sum equals a target. O(n) solution required. Show the Go code.",
			EvalCriteria: "Uses prefix sum + hash map for O(n) solution. Maintains running sum, stores first occurrence of each prefix sum. When (current_sum - target) exists in map, compute subarray length. Correct handling of edge cases (empty result, negative numbers).",
		},

		// ── Review Hard ──
		{
			ID: "review-security", Name: "Security Review",
			Category: "review", Difficulty: "hard",
			Query:        "Review this code for security issues: `func exec(cmd string) { exec.Command(\"sh\", \"-c\", cmd).Run() }` What are the risks and mitigations?",
			EvalCriteria: "Identifies command injection vulnerability: arbitrary shell execution. Risks: arbitrary code execution, data exfiltration, system compromise. Mitigations: input validation/allowlisting, use exec.Command with fixed args (not shell), sandboxing, least privilege. Mentions at least 2 specific risks and 2 mitigations.",
		},
		{
			ID: "review-concurrency", Name: "Concurrency Review",
			Category: "review", Difficulty: "hard",
			Query:        "Review this Go code for concurrency issues: `var mu sync.Mutex; func f() { mu.Lock(); go g(); mu.Unlock() }` What's wrong?",
			EvalCriteria: "Identifies that mu.Unlock() is called before the goroutine g() finishes. The mutex is released prematurely, allowing concurrent access. The goroutine g() might need the lock but it's already released. Fix: use defer mu.Unlock() or wait for goroutine completion.",
		},

		// ── OpenAIDE-specific Hard ──
		{
			ID: "oa-auto-verify", Name: "Explain Auto-Verification",
			Category: "think", Difficulty: "hard",
			Query:        "Read kernel/kernel_stream.go and explain the auto-verification mechanism. How does it detect the test command, run tests, and inject failures back into the ReAct loop?",
			EvalCriteria: "Explains: detectTestCommand checks for go.mod/package.json/makefile to determine test command (go test/npm test/make test). After coding, kernel runs the test command. If tests fail, error output is injected as a user message back into the ReAct loop. Max 3 retry rounds. Shows understanding of the full flow.",
			MinToolCalls: 2,
		},
		{
			ID: "oa-compare-approaches", Name: "Compare Approaches",
			Category: "think", Difficulty: "hard",
			Query:        "OpenAIDE uses CSP Actors for concurrency. Compare this approach with traditional mutex-based locking. When would CSP be better? When would mutexes be better?",
			EvalCriteria: "CSP better for: complex state machines, message passing patterns, avoiding deadlocks. Mutex better for: simple shared state, fine-grained locking, performance-critical paths. References OpenAIDE's actual use of CSP (Actor pattern in kernel/actor/). Balanced analysis with concrete examples.",
		},
	}
}

// FullCapabilityTasks returns a comprehensive suite covering all agent capabilities.
func FullCapabilityTasks() []Task {
	return []Task{
		// ── Reading & Understanding ──
		{ID: "read-explain", Name: "Read & Explain", Category: "think", Difficulty: "medium",
			Query:        "Read backend/internal/eval/eval.go and explain what Runner.RunTasks does. Be specific.",
			EvalCriteria: "Explains that RunTasks iterates over tasks, calls runOne for each, collects results into a Run struct. Mentions key details: result aggregation, timing, pass/fail tracking, and the Scorecard function.",
			MinToolCalls: 1},

		// ── Searching ──
		{ID: "search-codebase", Name: "Search Codebase", Category: "think", Difficulty: "medium",
			Query:        "Search for all files that define a function with 'Reflection' in its name. List file paths.",
			EvalCriteria: "Uses search_files or grep to find Go files containing functions with 'Reflection' in the name. Lists specific file paths (should include llm_reflection.go at minimum).",
			MinToolCalls: 1},

		// ── File Writing ──
		{ID: "write-verify", Name: "Write & Verify", Category: "coding", Difficulty: "easy",
			Query:        "Create /tmp/openaide_test_write.txt with content 'acceptance test passed'. Then read it back to verify.",
			EvalCriteria: "Successfully creates a file at /tmp/openaide_test_write.txt with the specified content. Verifies by reading it back.",
			MinToolCalls: 1},

		// ── Diff Editing ──
		{ID: "diff-edit-precise", Name: "Precise Diff Edit", Category: "coding", Difficulty: "medium",
			Query:        "First write a file /tmp/openaide_diff_target.txt with content 'line1\\nline2\\nline3'. Then use diff_edit to change 'line2' to 'line2-edited'. Finally read the file to confirm the change.",
			EvalCriteria: "Successfully creates the file, uses diff_edit to change 'line2' to 'line2-edited', and verifies by reading the file.",
			MinToolCalls: 2},

		// ── Memory Management ──
		{ID: "memory-store", Name: "Memory Store", Category: "coding", Difficulty: "easy",
			Query:        "Store this fact using manage_memory(action='remember'): 'acceptance test fact: error handling uses fmt.Errorf with %w wrapping throughout the kernel'",
			EvalCriteria: "Uses the manage_memory tool with action='remember'. Confirms the fact was stored.",
			MinToolCalls: 1},

		// ── Architecture Understanding ──
		{ID: "arch-synthesis", Name: "Architecture Synthesis", Category: "think", Difficulty: "hard",
			Query:        "Based on reading key files (CLAUDE.md, kernel/*.go), describe: (1) the layered architecture, (2) the CSP actor pattern, (3) the prompt system. Keep under 300 words.",
			EvalCriteria: "Describes all three topics with specific details from the codebase. References actual packages and design patterns. Under ~300 words.",
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
