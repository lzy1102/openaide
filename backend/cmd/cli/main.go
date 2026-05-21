package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"openaide/backend/internal/config"
	"openaide/backend/internal/infra"
)

func main() {
	args := os.Args[1:]
	continueSess := false

	for _, a := range args {
		switch a {
		case "-c", "--continue":
			continueSess = true
		case "update", "upgrade":
			cmdUpdate(args[1:])
			return
		case "version", "-v", "--version":
			fmt.Println("OpenAIDE CLI dev")
			printHelp()
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	configPath := os.Getenv("HOME") + "/.openaide/config.yaml"
	cfg, err := config.Load(configPath)
	if err != nil {
		cfg = config.DefaultConfig()
	}
	cfg.Server.Mode = "direct"
	infra.InitLogger(cfg.Log.Level, cfg.Log.Format)

	app, err := infra.NewApplication(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start: %v\n", err)
		os.Exit(1)
	}

	m := initModel(app, continueSess)
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
	fmt.Println("  openaide          Start new interactive chat (default)")
	fmt.Println("  openaide -c       Continue last session")
	fmt.Println("  openaide update   Update to latest version")
	fmt.Println("  openaide version  Show version info")
	fmt.Println("  openaide help     Show this help")
	fmt.Println()
	fmt.Println("In-Chat Keybindings:")
	fmt.Println("  Ctrl+C / Ctrl+D   Quit (or stop streaming)")
	fmt.Println("  Ctrl+S            Open session list")
	fmt.Println("  F1 / Ctrl+H        Show help")
	fmt.Println("  ↑ / ↓             Input history")
	fmt.Println("  PgUp / PgDown     Scroll chat")
	fmt.Println()
	fmt.Println("In-Chat Commands:")
	fmt.Println("  /help             Show help overlay")
	fmt.Println("  /clear            Clear chat messages")
	fmt.Println("  /new              Create new session")
	fmt.Println("  /sessions         Open session list")
}

func cmdUpdate(args []string) {
	fmt.Println("▶ OpenAIDE Update")
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
	fmt.Println("\n✓ Update complete!")
}
