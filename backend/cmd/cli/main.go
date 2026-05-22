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
	configPath    string
	contextFiles  []string
	prompt        string
	continueSess  bool
	resumeID      string
	yes           bool
	model         string
	verbose       bool
	outputFormat  string
}

func isExistingFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func detectFiles(args []string) (files []string, promptParts []string) {
	for _, a := range args {
		if isExistingFile(a) {
			files = append(files, a)
		} else {
			promptParts = append(promptParts, a)
		}
	}
	return
}

func parseFlags(args []string) cliFlags {
	f := cliFlags{
		configPath:   os.Getenv("HOME") + "/.openaide/config.yaml",
		outputFormat: "text",
	}
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-c" || a == "--continue":
			f.continueSess = true
		case a == "-p" || a == "--prompt":
			if i+1 < len(args) {
				i++
				f.prompt = args[i]
			}
		case a == "--resume" && i+1 < len(args):
			i++
			f.resumeID = args[i]
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
			positional = append(positional, a)
		}
	}

	if len(positional) > 0 {
		files, promptParts := detectFiles(positional)
		f.contextFiles = files
		f.prompt = strings.Join(promptParts, " ")
	}

	return f
}

func buildPrompt(files []string, prompt string) string {
	var parts []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", path, err)
			continue
		}
		parts = append(parts, fmt.Sprintf("Content of %s:\n---\n%s\n---", path, string(data)))
	}
	if len(parts) > 0 && prompt != "" {
		return strings.Join(parts, "\n\n") + "\n\n" + prompt
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n\n")
	}
	return prompt
}

func doAutoCommit(prompt string) {
	if _, err := os.Stat(".git"); os.IsNotExist(err) {
		return
	}
	msg := prompt
	if msg == "" {
		msg = "openaide auto-commit"
	}
	if len(msg) > 72 {
		msg = msg[:72]
	}

	exec.Command("git", "add", "-A").Run()

	if exec.Command("git", "diff", "--cached", "--quiet").Run() != nil {
		cmd := exec.Command("git", "commit", "-m", msg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}
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

	// One-shot: prompt from positional args or -p flag (pipe)
	if flags.prompt != "" || len(flags.contextFiles) > 0 {
		prompt := buildPrompt(flags.contextFiles, flags.prompt)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		ch, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", prompt, kernel.QueryOptions{})
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
			out := map[string]string{"content": full.String()}
			data, _ := json.Marshal(out)
			fmt.Println(string(data))
		}

		doAutoCommit(flags.prompt)
		return
	}

	// Resume specific session
	if flags.resumeID != "" {
		ctx := context.Background()
		sess, err := app.Orchestrator.GetSession(ctx, flags.resumeID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: session %s not found: %v\n", flags.resumeID, err)
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

func printHelp() {
	fmt.Println("OpenAIDE CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  openaide                          Interactive chat (TUI)")
	fmt.Println("  openaide <prompt>                 One-shot mode")
	fmt.Println("  openaide <file.go> <prompt>       File + prompt")
	fmt.Println("  openaide -p <prompt>              Pipe mode")
	fmt.Println("  echo \"fix this\" | openaide -p     Pipe from stdin")
	fmt.Println("  openaide -c                       Continue last session")
	fmt.Println("  openaide --resume <id>            Resume specific session")
	fmt.Println("  openaide -y                       Auto-approve all actions")
	fmt.Println("  openaide --model <name>           Override model")
	fmt.Println("  openaide --config <path>          Custom config path")
	fmt.Println("  openaide --verbose                Debug logging")
	fmt.Println("  openaide --output json            JSON output")
	fmt.Println("  openaide sessions                 List sessions")
	fmt.Println("  openaide update                   Update")
	fmt.Println("  openaide version                  Version")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  openaide fix this bug")
	fmt.Println("  openaide main.go review this")
	fmt.Println("  openaide -c -y")
	fmt.Println("  openaide --model claude-3-opus explain")
	fmt.Println("  echo 'add error handling' | openaide -p")
	fmt.Println()
	fmt.Println("In-chat commands:")
	fmt.Println("  /help    /clear   /model    /cost")
	fmt.Println("  /diff    /add     /drop     /compact")
	fmt.Println("  /git     /web     /undo     /architect")
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

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "..."
}
