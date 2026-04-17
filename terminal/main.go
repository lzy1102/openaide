package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"openaide/terminal/tui"
)

// 版本号（编译时注入）
var Version = "dev"

// 标志变量
var (
	flagModel   string
	flagStream  bool
	flagAPI     string
	flagConfig  string
	flagContext int
	flagVerbose bool
)

// 根命令
var rootCmd = &cobra.Command{
	Use:   "openaide",
	Short: "OpenAIDE command line interface",
	Long:  "OpenAIDE - A powerful AI assistant with workflow capabilities, plugin system, and cross-platform automation",
	Example: `  openaide                    # Start interactive chat
  openaide -m gpt-4            # Use specific model
  openaide -s                  # Enable streaming
  openaide dashboard           # Show dashboard
  openaide config edit         # Interactive settings wizard`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _, err := tui.LoadConfig(flagConfig)
		if err != nil && flagVerbose {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
		apiURL := tui.GetAPIURL(cfg, flagAPI)
		model := tui.GetModel(cfg, flagModel, apiURL)
		stream := tui.GetStream(cfg, flagStream)
		tui.RunChat(cfg, apiURL, model, stream)
	},
}

// modelsCmd 模型列表命令
var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "List available models",
	Long:  "List all available AI models with their details",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _, _ := tui.LoadConfig(flagConfig)
		apiURL := tui.GetAPIURL(cfg, flagAPI)
		tui.PrintModelList(apiURL)
	},
}

// configCmd 配置命令
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  "Show, edit, or initialize openaide configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, path, _ := tui.LoadConfig(flagConfig)
		tui.ShowConfig(cfg, path)
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration file",
	Long:  "Create a default configuration file at ~/.openaide/config.yaml",
	Run: func(cmd *cobra.Command, args []string) {
		tui.InitConfig(flagConfig)
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration interactively",
	Long:  "Open an interactive settings wizard to configure OpenAIDE",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, configPath, _ := tui.LoadConfig(flagConfig)
		result := tui.RunSettings(cfg, configPath)
		if result.Saved {
			fmt.Println("Configuration saved.")
		}
	},
}

// dashboardCmd 仪表盘命令
var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Show system dashboard",
	Long:  "Display a dashboard with dialogues, model status, and system info",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _, _ := tui.LoadConfig(flagConfig)
		apiURL := tui.GetAPIURL(cfg, flagAPI)
		tui.RunDashboard(apiURL, Version)
	},
}

func init() {
	// 聊天标志
	rootCmd.Flags().StringVarP(&flagModel, "model", "m", "", "Model ID to use")
	rootCmd.Flags().BoolVarP(&flagStream, "stream", "s", false, "Enable streaming response")
	rootCmd.Flags().IntVarP(&flagContext, "context", "c", 0, "Context limit (number of previous messages)")
	rootCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false, "Verbose output")

	// 全局标志
	rootCmd.PersistentFlags().StringVar(&flagAPI, "api", "", "API base URL (overrides config)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "Config file path")

	// 子命令
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(dashboardCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configEditCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
