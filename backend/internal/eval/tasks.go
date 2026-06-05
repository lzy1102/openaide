package eval

// BuiltinTasks returns the standard benchmark task suite.
func BuiltinTasks() []Task {
	return []Task{
		// ── Easy tasks ──
		{
			ID: "explain-concurrency", Name: "Explain Go Concurrency",
			Category: "teaching", Difficulty: "easy",
			Query:       "Explain how goroutines and channels work in Go. Give a short example.",
			MustContain: []string{"goroutine", "channel"},
			MustNotContain: []string{"I don't know", "I'm not sure"},
		},
		{
			ID: "hello-response", Name: "Greeting Response",
			Category: "general", Difficulty: "easy",
			Query:       "Hello! What can you help me with?",
			MustContain: []string{"help", "code", "OpenAIDE"},
			MustNotContain: []string{"I can't", "not able"},
		},
		{
			ID: "simple-calc", Name: "Simple Calculation",
			Category: "coding", Difficulty: "easy",
			Query:       "What is the time complexity of binary search? Answer in one sentence.",
			MustContain: []string{"O(log n)"},
		},

		// ── Medium tasks ──
		{
			ID: "analyze-structure", Name: "Analyze Project Structure",
			Category: "research", Difficulty: "medium",
			Query:       "What are the main packages in this project? List their responsibilities briefly.",
			MustContain: []string{"kernel", "tools", "api"},
			MustNotContain: []string{"I don't know", "I haven't read"},
		},
		{
			ID: "code-review-pattern", Name: "Review Code Pattern",
			Category: "review", Difficulty: "medium",
			Query:       "Review this Go error handling pattern: if err != nil { return err }. Is it correct? What could go wrong?",
			MustContain: []string{"error", "context", "return"},
		},
		{
			ID: "fix-hypothetical-bug", Name: "Fix Hypothetical Bug",
			Category: "coding", Difficulty: "medium",
			Query:       "A function returns nil instead of an error when a file is not found. What's the fix? Explain in 2-3 sentences.",
			MustContain: []string{"error", "return"},
			MustNotContain: []string{"I don't know"},
		},
		{
			ID: "explain-actor-model", Name: "Explain Actor Model",
			Category: "teaching", Difficulty: "medium",
			Query:       "Explain the CSP actor pattern used in OpenAIDE's kernel. What problem does it solve?",
			MustContain: []string{"goroutine", "channel", "lock"},
			MustNotContain: []string{"I'm not familiar"},
		},

		// ── Hard tasks ──
		{
			ID: "architecture-critique", Name: "Architecture Critique",
			Category: "review", Difficulty: "hard",
			Query:       "Analyze the layered architecture of OpenAIDE. What are the strengths and one potential weakness? Be specific.",
			MustContain: []string{"kernel", "orchestration", "infra"},
			MustNotContain: []string{"I don't know", "I can't"},
		},
		{
			ID: "refactoring-plan", Name: "Refactoring Plan",
			Category: "research", Difficulty: "hard",
			Query:       "If you were to split the kernel package into smaller sub-packages, what would you extract and why?",
			MustContain: []string{"package", "extract", "interface", "actor", "session"},
		},
		{
			ID: "multi-step-logic", Name: "Multi-step Logic",
			Category: "coding", Difficulty: "hard",
			Query:       "Design a rate limiter using the token bucket algorithm. Describe the key components and how they work together. Keep it under 200 words.",
			MustContain: []string{"token", "bucket", "rate"},
		},
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
