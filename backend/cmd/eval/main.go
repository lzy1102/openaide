package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"openaide/backend/config"
	"openaide/backend/internal/eval"
	"openaide/backend/internal/infra"
)

func main() {
	var (
		configPath = flag.String("config", os.Getenv("HOME")+"/.openaide/config.yaml", "config file path")
		outputPath = flag.String("output", "eval_results.json", "output file for results")
		category   = flag.String("category", "", "filter by category (coding/review/think/general)")
		quick      = flag.Bool("quick", false, "only run easy tasks (smoke test)")
		fullCap    = flag.Bool("full", false, "run full capability acceptance test")
	)
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Make sure ~/.openaide/config.yaml exists with valid API keys.\n")
		os.Exit(1)
	}

	// Create application
	fmt.Println("Initializing application...")
	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create application: %v\n", err)
		os.Exit(1)
	}
	go app.Start()
	defer app.Stop(context.Background())

	// Select tasks
	var tasks []eval.Task
	if *fullCap {
		tasks = eval.FullCapabilityTasks()
		fmt.Println("Running FULL CAPABILITY acceptance test")
	} else if *quick {
		tasks = eval.QuickTasks()
	} else if *category != "" {
		tasks = eval.CategoryTasks(*category)
	} else {
		tasks = eval.BuiltinTasks()
	}

	fmt.Printf("Running %d evaluation tasks...\n\n", len(tasks))

	// Run evaluation with LLM as judge
	runner := eval.NewRunnerWithJudge(app.Kernel, eval.NewLLMJudge(app.LLMGateway))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	run := runner.RunTasks(ctx, tasks)

	// Print scorecard
	fmt.Println(run.Scorecard())
	fmt.Println()

	// Sort by slowest for analysis
	eval.SortResultsByDuration(run.Results)
	fmt.Println("Slowest tasks:")
	for i := len(run.Results) - 1; i >= 0 && i >= len(run.Results)-3; i-- {
		r := run.Results[i]
		status := "✓"
		if !r.Passed {
			status = "✗"
		}
		fmt.Printf("  %s %s (%s) — %v, %d tools\n", status, r.Task.Name, r.Task.Difficulty,
			r.Duration.Round(time.Millisecond), r.ToolCalls)
	}

	// Save results
	data, _ := json.MarshalIndent(run, "", "  ")
	os.WriteFile(*outputPath, data, 0644)
	fmt.Printf("\nResults saved to %s\n", *outputPath)
}
