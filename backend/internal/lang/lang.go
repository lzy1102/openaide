package lang

import (
	"fmt"
	"os"
	"strings"
)

type Lang string

const (
	ZH Lang = "zh"
	EN Lang = "en"
)

func Detect() Lang {
	env := os.Getenv("LANG")
	if env == "" {
		env = os.Getenv("LC_MESSAGES")
	}
	if env == "" {
		env = os.Getenv("LC_ALL")
	}
	env = strings.ToLower(env)
	if strings.HasPrefix(env, "zh") {
		return ZH
	}
	return EN
}

	var messages = map[Lang]map[string]string{
	ZH: {
		"cli.usage":         "OpenAIDE — AI Agent 终端",
		"cli.usage_detail":  "用法:",
		"cli.help":          "帮助",
		"cli.version":       "OpenAIDE CLI dev",
		"cli.oneshot":       "  openaide <prompt>            单次执行",
		"cli.file_oneshot":  "  openaide <file> <prompt>     传文件 + prompt",
		"cli.y":             "  openaide -y                  自动批准所有操作",
		"cli.c":             "  openaide -c                  继续上次会话",
		"cli.model":         "  openaide --model <name>      指定模型",
		"cli.output":        "  openaide -o json             结构化输出",
		"cli.verbose":       "  openaide --verbose           调试日志",
		"cli.sessions":      "  openaide sessions            列出会话",
		"cli.update":        "  openaide update              更新",
		"cli.examples":      "示例:",
		"cli.ex_oneshot":    "  openaide fix this bug",
		"cli.ex_file":       "  openaide main.go review this function",
		"cli.ex_continue":   "  openaide -c -y",
		"cli.ex_model":      "  openaide --model gpt-4o rewrite this",

		"cli.keybindings":   "快捷键:",
		"cli.kb_quit":       "  Ctrl+C / Ctrl+D    退出",
		"cli.kb_sessions":   "  Ctrl+S             会话列表",
		"cli.kb_help":       "  F1 / Ctrl+H        帮助",
		"cli.kb_history":    "  ↑ / ↓              输入历史",
		"cli.kb_scroll":     "  PgUp / PgDown      滚动",

		"cli.commands":      "斜杠命令:",
		"cli.cmd_help":      "  /help               显示帮助",
		"cli.cmd_clear":     "  /clear              清屏",
		"cli.cmd_model":     "  /model [name]       显示/切换模型",

		"err.start_failed":  "启动失败: %v",
		"err.process":       "处理失败: %v",
		"err.unknown_cmd":   "未知命令: %s（可用 /help）",
		"warn.read_file":    "警告: 无法读取 %s: %v",
		"warn.no_sessions":  "没有会话。按 n 创建一个。",
		"warn.delete_confirm": "再按一次 d 删除此会话",
		"warn.nav_help":     "↑/↓ 导航 · Enter 选择 · n 新建 · d 删除 · esc/q 返回",
		"warn.model_help":   "↑/↓ 导航 · Enter 选择 · esc/q 返回",

		"sess.none":         "没有会话。",
		"sess.info":         "会话 | 消息 | 预览",
		"sess.list_format":  "%-24s  %3d 条消息  %s\n",

		"tui.placeholder":   "输入消息... (Ctrl+H 帮助)",
		"tui.model_switched":"已切换模型至: %s",
		"tui.provider_switched": "已切换至: %s（模型: %s）",
		"tui.switch_failed": "切换失败: %v",
		"tui.no_providers":  "没有配置提供商。",

		"mode.chat":         "聊天",
		"mode.thinking":     "思考中…",
		"mode.streaming":    "流式输出中…（Ctrl+C 停止）",

		"help.title":        "帮助",
		"help.keybindings":  "快捷键",
		"help.commands":     "命令",
		"help.tips":         "提示",
		"help.tips_text":    "输入消息后按 Enter 开始对话。\n按 ↑ 回忆上一条消息。\n按 / 查看所有命令。",
		"help.cmd_help_desc":   "显示此帮助",
		"help.cmd_clear_desc":  "清屏",
		"help.cmd_model_desc":  "显示/切换当前模型",

		"update.title":           "OpenAIDE 更新",
		"update.script_not_found":"错误: 更新脚本未找到",
		"update.failed":          "\n更新失败: %v\n",
		"update.complete":        "\n更新完成!",

		"git.auto_commit_msg":    "openaide 自动提交",
	},
	EN: {
		"cli.usage":         "OpenAIDE — AI Agent Terminal",
		"cli.usage_detail":  "Usage:",
		"cli.help":          "help",
		"cli.version":       "OpenAIDE CLI dev",
		"cli.oneshot":       "  openaide <prompt>            One-shot mode",
		"cli.file_oneshot":  "  openaide <file> <prompt>     File + prompt",
		"cli.y":             "  openaide -y                  Auto-approve all actions",
		"cli.c":             "  openaide -c                  Continue last session",
		"cli.model":         "  openaide --model <name>      Override model",
		"cli.output":        "  openaide -o json             Structured output",
		"cli.verbose":       "  openaide --verbose           Debug logging",
		"cli.sessions":      "  openaide sessions            List sessions",
		"cli.update":        "  openaide update              Update",
		"cli.examples":      "Examples:",
		"cli.ex_oneshot":    "  openaide fix this bug",
		"cli.ex_file":       "  openaide main.go review this function",
		"cli.ex_continue":   "  openaide -c -y",
		"cli.ex_model":      "  openaide --model gpt-4o rewrite this",

		"cli.keybindings":   "Keybindings:",
		"cli.kb_quit":       "  Ctrl+C / Ctrl+D    Quit",
		"cli.kb_sessions":   "  Ctrl+S             Session list",
		"cli.kb_help":       "  F1 / Ctrl+H        Help",
		"cli.kb_history":    "  ↑ / ↓              History",
		"cli.kb_scroll":     "  PgUp / PgDown      Scroll",

		"cli.commands":      "Commands:",
		"cli.cmd_help":      "  /help               Show help",
		"cli.cmd_clear":     "  /clear              Clear chat",
		"cli.cmd_model":     "  /model [name]       Show/switch model",

		"err.start_failed":  "Failed to start: %v",
		"err.process":       "Error: %v",
		"err.unknown_cmd":   "Unknown command: %s (try /help)",
		"warn.read_file":    "Warning: cannot read %s: %v",
		"warn.no_sessions":  "No sessions. Press n to create one.",
		"warn.delete_confirm": "Press d again to delete this session",
		"warn.nav_help":     "↑/↓ navigate · Enter select · n new · d delete · esc/q back",
		"warn.model_help":   "↑/↓ navigate · Enter select · esc/q back",

		"sess.none":         "No sessions found.",
		"sess.info":         "Session | Messages | Preview",
		"sess.list_format":  "%-24s  %3d msgs  %s\n",

		"tui.placeholder":   "Type a message... (Ctrl+H for help)",
		"tui.model_switched":"Switched model to: %s",
		"tui.provider_switched": "Switched to: %s (model: %s)",
		"tui.switch_failed": "Failed to switch: %v",
		"tui.no_providers":  "No providers configured.",

		"mode.chat":         "Chat",
		"mode.thinking":     "Thinking…",
		"mode.streaming":    "Streaming… (Ctrl+C to stop)",

		"help.title":        "Help",
		"help.keybindings":  "Keybindings",
		"help.commands":     "Commands",
		"help.tips":         "Tips",
		"help.tips_text":    "Type a message and press Enter to chat.\nPress ↑ to recall previous messages.\nPress / to browse commands.",
		"help.cmd_help_desc":   "Show this help",
		"help.cmd_clear_desc":  "Clear chat messages",
		"help.cmd_model_desc":  "Show/switch current model",

		"update.title":           "OpenAIDE Update",
		"update.script_not_found":"Error: update script not found",
		"update.failed":          "\nUpdate failed: %v\n",
		"update.complete":        "\nUpdate complete!",

		"git.auto_commit_msg":    "openaide auto-commit",
	},
}

var current Lang

func init() {
	SetLang(Detect())
}

func SetLang(l Lang) {
	current = l
	if _, ok := messages[current]; !ok {
		current = EN
	}
}

func GetLang() Lang {
	return current
}

func T(key string, args ...any) string {
	msg, ok := messages[current]
	if !ok {
		msg = messages[EN]
	}
	template, ok := msg[key]
	if !ok {
		template, ok = messages[EN][key]
		if !ok {
			return key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(template, args...)
	}
	return template
}
