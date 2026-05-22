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
	"openaide/backend/internal/lang"
)

type cliFlags struct {
	contextFiles []string
	prompt       string
	continueSess bool
	yes          bool
	model        string
	verbose      bool
	outputFormat string
}

func defaultConfigPath() string {
	return os.Getenv("HOME") + "/.openaide/config.yaml"
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
		outputFormat: "text",
	}
	var positional []string

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
			fmt.Fprintf(os.Stderr, "%s\n", lang.T("warn.read_file", path, err))
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

	cfg, err := config.Load(defaultConfigPath())
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
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.start_failed", err))
		os.Exit(1)
	}
	if flags.yes {
		app.SetAutoApprove(true)
	}

	// One-shot: prompt from positional args
	if flags.prompt != "" || len(flags.contextFiles) > 0 {
		prompt := buildPrompt(flags.contextFiles, flags.prompt)
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		ch, err := app.Orchestrator.ProcessQueryStream(ctx, "cli-user", "default", prompt, kernel.QueryOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.process", err))
			os.Exit(1)
		}
		var full strings.Builder
		for chunk := range ch {
			if chunk.Type == kernel.ChunkTypeError {
				fmt.Fprintf(os.Stderr, "\n%s\n", lang.T("err.process", chunk.Content))
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

	m := initModel(app, flags.continueSess)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	m.program = p
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.process", err))
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(lang.T("cli.usage"))
	fmt.Println()
	fmt.Println(lang.T("cli.usage_detail"))
	fmt.Println(lang.T("cli.oneshot"))
	fmt.Println(lang.T("cli.file_oneshot"))
	fmt.Println(lang.T("cli.c"))
	fmt.Println(lang.T("cli.y"))
	fmt.Println(lang.T("cli.model"))
	fmt.Println(lang.T("cli.output"))
	fmt.Println(lang.T("cli.verbose"))
	fmt.Println(lang.T("cli.sessions"))
	fmt.Println(lang.T("cli.update"))
	fmt.Println()
	fmt.Println(lang.T("cli.examples"))
	fmt.Println(lang.T("cli.ex_oneshot"))
	fmt.Println(lang.T("cli.ex_file"))
	fmt.Println(lang.T("cli.ex_continue"))
	fmt.Println(lang.T("cli.ex_model"))
	fmt.Println()
	fmt.Println(lang.T("cli.keybindings"))
	fmt.Println(lang.T("cli.kb_quit"))
	fmt.Println(lang.T("cli.kb_sessions"))
	fmt.Println(lang.T("cli.kb_help"))
	fmt.Println(lang.T("cli.kb_history"))
	fmt.Println(lang.T("cli.kb_scroll"))
	fmt.Println()
	fmt.Println(lang.T("cli.commands"))
	fmt.Println(lang.T("cli.cmd_help"))
	fmt.Println(lang.T("cli.cmd_clear"))
	fmt.Println(lang.T("cli.cmd_model"))
}

func cmdSessions(args []string) {
	cfg, err := config.Load(defaultConfigPath())
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.start_failed", err))
		os.Exit(1)
	}
	ctx := context.Background()

	sessions, err := app.Orchestrator.ListSessions(ctx, "default", "cli-user", 100, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", lang.T("err.process", err))
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Println(lang.T("sess.none"))
		return
	}
	fmt.Println(lang.T("sess.info"))
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
