package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
	"openaide/backend/internal/kernel"
)

type cliFlags struct {
	configPath   string
	contextFiles []string
	prompt       string
	continueSess bool
	yes          bool
	model        string
	verbose      bool
	noStream     bool
	noTUI        bool
	outputFormat string
}

func parseFlags(args []string) cliFlags {
	f := cliFlags{
		configPath:   os.Getenv("HOME") + "/.openaide/config.yaml",
		outputFormat: "text",
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-c" || a == "--continue":
			f.continueSess = true
		case a == "-y" || a == "--yes":
			f.yes = true
		case a == "--verbose":
			f.verbose = true
		case a == "--no-stream":
			f.noStream = true
		case a == "--no-tui":
			f.noTUI = true
		case a == "-f" || a == "--file":
			if i+1 < len(args) {
				i++
				f.contextFiles = append(f.contextFiles, args[i])
			}
		case a == "--model" && i+1 < len(args):
			i++
			f.model = args[i]
		case a == "--config" && i+1 < len(args):
			i++
			f.configPath = args[i]
		case a == "--output" && i+1 < len(args):
			i++
			switch args[i] {
			case "json":
				f.outputFormat = "json"
			default:
				f.outputFormat = "text"
			}
		case a == "-h" || a == "--help" || a == "help":
			printHelp()
			os.Exit(0)
		case a == "-v" || a == "--version" || a == "version":
			fmt.Println("OpenAIDE CLI dev")
			os.Exit(0)
		case a == "update" || a == "upgrade":
			cmdUpdate(args[i+1:])
			os.Exit(0)
		case a == "sessions":
			cmdSessions(args[i+1:])
			os.Exit(0)
		case !strings.HasPrefix(a, "-"):
			f.prompt = strings.Join(args[i:], " ")
			return f
		}
	}
	return f
}

func buildPrompt(files []string, prompt string) string {
	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read file %s: %v\n", path, err)
			continue
		}
		parts = append(parts, fmt.Sprintf("Content of %s:\n---\n%s\n---", path, string(data)))
	}
	if prompt == "" && len(parts) > 0 {
		return strings.Join(parts, "\n\n")
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n\n") + "\n\n" + prompt
	}
	return prompt
}

func main() {
	flags := parseFlags(os.Args[1:])

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	if flags.verbose {
		cfg.Log.Level = "debug"
	}
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	if flags.model != "" {
		for i := range cfg.LLM.Providers {
			if cfg.LLM.Providers[i].DefaultModel != "" {
				cfg.LLM.Providers[i].DefaultModel = flags.model
				break
			}
		}
	}

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}
	if flags.yes {
		app.SetAutoApprove(true)
	}

	if flags.prompt != "" || len(flags.contextFiles) > 0 {
		prompt := buildPrompt(flags.contextFiles, flags.prompt)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		opts := kernel.QueryOptions{}
		if flags.noStream {
			resp, err := app.Orchestrator.ProcessQuery(ctx, "cli-user", "default", prompt, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			outputResult(flags.outputFormat, resp.Content)
		} else {
			ch, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", prompt, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			var full strings.Builder
			for chunk := range ch {
				if chunk.Type == kernel.ChunkTypeError {
					fmt.Fprintf(os.Stderr, "\nError: %s\n", chunk.Content)
					os.Exit(1)
				}
				if chunk.Type == kernel.ChunkTypeContent {
					fmt.Print(chunk.Content)
					full.WriteString(chunk.Content)
				}
			}
			fmt.Println()
			if flags.outputFormat == "json" {
				outputResult("json", full.String())
			}
		}
		return
	}

	m := initModel(app, flags.continueSess)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	m.program = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func outputResult(format, content string) {
	if format == "json" {
		out := map[string]string{"content": content}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))
	} else {
		fmt.Println(content)
	}
}

func printHelp() {
	fmt.Println("OpenAIDE CLI — AI Agent terminal")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  openaide                     Interactive chat (TUI)")
	fmt.Println("  openaide <prompt>            One-shot mode")
	fmt.Println("  openaide -f <file> <prompt>  One-shot with file context")
	fmt.Println("  openaide -c                  Continue last session")
	fmt.Println("  openaide -y                  Auto-approve all actions")
	fmt.Println("  openaide --model <name>      Override model")
	fmt.Println("  openaide --config <path>     Custom config path")
	fmt.Println("  openaide --verbose           Debug logging")
	fmt.Println("  openaide --no-stream         Non-streaming output")
	fmt.Println("  openaide --no-tui            Non-interactive text mode")
	fmt.Println("  openaide --output json       JSON output")
	fmt.Println("  openaide sessions [cmd]      Manage sessions")
	fmt.Println("  openaide update              Update")
	fmt.Println("  openaide version             Version")
	fmt.Println()
	fmt.Println("Session commands:")
	fmt.Println("  openaide sessions            List all sessions")
	fmt.Println("  openaide sessions delete <id> Delete session")
	fmt.Println("  openaide sessions resume <id> Resume specific session")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  openaide fix this bug")
	fmt.Println("  openaide -f main.go review this function")
	fmt.Println("  openaide -f main.go -f utils.go refactor both")
	fmt.Println("  openaide -c -y")
	fmt.Println("  openaide --model claude-3-opus explain this")
	fmt.Println("  openaide --output json find the bug")
	fmt.Println("  openaide sessions")
	fmt.Println("  openaide sessions delete abc123")
}

func cmdSessions(args []string) {
	cfgPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	cfg, err := config.Load(cfgPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()

	if len(args) == 0 || args[0] == "list" {
		sessions, err := app.Orchestrator.ListSessions(ctx, "default", "cli-user", 100, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return
		}
		for _, s := range sessions {
			msgCount := len(s.Messages)
			preview := ""
			for _, m := range s.Messages {
				if m.Role == "user" && m.Content != "" {
					preview = truncate(m.Content, 60)
					break
				}
			}
			fmt.Printf("%-24s  %3d msgs  %s\n", s.ID, msgCount, preview)
		}
		return
	}

	switch args[0] {
	case "delete":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: openaide sessions delete <session-id>")
			os.Exit(1)
		}
		for _, id := range args[1:] {
			if err := app.Orchestrator.DeleteSession(ctx, id); err != nil {
				fmt.Fprintf(os.Stderr, "Error deleting %s: %v\n", id, err)
			} else {
				fmt.Printf("Deleted: %s\n", id)
			}
		}
	case "resume":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: openaide sessions resume <session-id>")
			os.Exit(1)
		}
		sess, err := app.Orchestrator.GetSession(ctx, args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		m := initModel(app, false)
		m.currentSess = sess
		m.loadChatHistory()
		p := tea.NewProgram(m,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
		)
		m.program = p
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown session command: %s\n", args[0])
		os.Exit(1)
	}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "..."
}

func cmdUpdate(args []string) {
	fmt.Println("OpenAIDE Update")
	installDir := os.Getenv("HOME") + "/.openaide"
	script := filepath.Join(installDir, "scripts", "update.sh")
	if _, err := os.Stat(script); os.IsNotExist(err) {
		script = filepath.Join(installDir, "install.sh")
		if _, err := os.Stat(script); os.IsNotExist(err) {
			fmt.Println("Error: update script not found")
			os.Exit(1)
		}
	}

	cmdArgs := []string{script}
	for _, arg := range args {
		if arg == "--local" || arg == "-l" {
			cmdArgs = append(cmdArgs, "--local")
		}
	}

	cmd := exec.Command("bash", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("\nUpdate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("\nUpdate complete!")
}
