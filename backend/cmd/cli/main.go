package main

import (
	"context"
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
	configPath  string
	prompt      string
	continueSess bool
	yes         bool
	model       string
	verbose     bool
}

func parseFlags(args []string) cliFlags {
	f := cliFlags{
		configPath: os.Getenv("HOME") + "/.openaide/config.yaml",
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
		case a == "--model" && i+1 < len(args):
			i++
			f.model = args[i]
		case a == "--config" && i+1 < len(args):
			i++
			f.configPath = args[i]
		case a == "-h" || a == "--help" || a == "help":
			printHelp()
			os.Exit(0)
		case a == "-v" || a == "--version" || a == "version":
			fmt.Println("OpenAIDE CLI dev")
			os.Exit(0)
		case a == "update" || a == "upgrade":
			cmdUpdate(args[i+1:])
			os.Exit(0)
		case !strings.HasPrefix(a, "-"):
			f.prompt = strings.Join(args[i:], " ")
			return f
		}
	}
	return f
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

	if flags.prompt != "" {
		if flags.yes {
			app.SetAutoApprove(true)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()
		resp, err := app.Orchestrator.ProcessQuery(ctx, "cli-user", "default", flags.prompt, kernel.QueryOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(resp.Content)
		return
	}

	if flags.yes {
		app.SetAutoApprove(true)
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

func printHelp() {
	fmt.Println("OpenAIDE CLI — AI Agent 终端")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  openaide                     Start interactive chat")
	fmt.Println("  openaide <prompt>            One-shot mode (non-interactive)")
	fmt.Println("  openaide -c                  Continue last session")
	fmt.Println("  openaide -y                  Auto-approve all actions")
	fmt.Println("  openaide --model <name>      Override model (e.g. gpt-4o, claude-3-opus)")
	fmt.Println("  openaide --config <path>     Custom config file")
	fmt.Println("  openaide --verbose           Debug logging")
	fmt.Println("  openaide update              Update to latest version")
	fmt.Println("  openaide version             Show version info")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  openaide fix this bug                   One-shot fix")
	fmt.Println("  openaide -c -y                          Continue, auto-approve")
	fmt.Println("  openaide --model claude-3-opus review this")
	fmt.Println()
	fmt.Println("In-Chat Keybindings:")
	fmt.Println("  Ctrl+C / Ctrl+D   Quit (or stop streaming)")
	fmt.Println("  Ctrl+S            Open session list")
	fmt.Println("  F1 / Ctrl+H        Show help")
	fmt.Println("  \u2191 / \u2193             Input history")
	fmt.Println("  PgUp / PgDown     Scroll chat")
	fmt.Println()
	fmt.Println("In-Chat Commands:")
	fmt.Println("  /help             Show help overlay")
	fmt.Println("  /clear            Clear chat messages")
	fmt.Println("  /new              Create new session")
	fmt.Println("  /sessions         Open session list")
}

func cmdUpdate(args []string) {
	fmt.Println("\u25b6 OpenAIDE Update")
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
	fmt.Println("\n\u2713 Update complete!")
}
