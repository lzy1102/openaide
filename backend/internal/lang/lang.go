package lang

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
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
		"cli.usage":        "OpenAIDE — AI Agent 终端",
		"cli.usage_detail": "用法:",
		"cli.help":         "帮助",
		"cli.version":      "OpenAIDE CLI dev",
		"cli.oneshot":      "  openaide <prompt>            单次执行",
		"cli.file_oneshot": "  openaide <file> <prompt>     传文件 + prompt",
		"cli.y":            "  openaide -y                  自动批准所有操作",
		"cli.c":            "  openaide -c                  继续上次会话",
		"cli.model":        "  openaide --model <name>      指定模型",
		"cli.output":       "  openaide -o json             结构化输出",
		"cli.verbose":      "  openaide --verbose           调试日志",
		"cli.sessions":     "  openaide sessions            列出会话",
		"cli.setup":        "  openaide setup               交互式配置向导",
		"cli.update":       "  openaide update              更新",
		"cli.examples":     "示例:",
		"cli.ex_oneshot":   "  openaide fix this bug",
		"cli.ex_file":      "  openaide main.go review this function",
		"cli.ex_continue":  "  openaide -c -y",
		"cli.ex_model":     "  openaide --model gpt-4o rewrite this",

		"cli.keybindings": "快捷键:",
		"cli.kb_quit":     "  Ctrl+C / Ctrl+D    退出",
		"cli.kb_sessions": "  Ctrl+S             会话列表",
		"cli.kb_help":     "  F1 / Ctrl+H        帮助",
		"cli.kb_history":  "  ↑ / ↓              输入历史",
		"cli.kb_scroll":   "  PgUp / PgDown      滚动",

		"cli.commands":     "斜杠命令:",
		"cli.cmd_help":     "  /help               显示帮助",
		"cli.cmd_clear":    "  /clear              清屏",
		"cli.cmd_model":    "  /model [name]       显示/切换模型",
		"cli.cmd_analyst":  "  /analyst <task>     分析员角色（只读）",
		"cli.cmd_coder":    "  /coder <task>       程序员角色（读写）",
		"cli.cmd_reviewer": "  /reviewer <task>    审查员角色（只读）",
		"cli.cmd_executor": "  /executor <task>    执行者角色（测试）",
		"cli.cmd_team":     "  /team <task>        团队协作（全角色链）",

		"err.start_failed":    "启动失败: %v",
		"err.process":         "处理失败: %v",
		"err.unknown_cmd":     "未知命令: %s（可用 /help）",
		"warn.read_file":      "警告: 无法读取 %s: %v",
		"warn.no_sessions":    "没有会话。按 n 创建一个。",
		"warn.delete_confirm": "再按一次 d 删除此会话",
		"warn.nav_help":       "↑/↓ 导航 · Enter 选择 · n 新建 · d 删除 · esc/q 返回",
		"warn.model_help":     "↑/↓ 导航 · Enter 选择 · esc/q 返回",

		"sess.none":        "没有会话。",
		"sess.info":        "会话 | 消息 | 预览",
		"sess.list_format": "%-24s  %3d 条消息  %s\n",
		"sess.tui_row":     "%s%s  [%d 条消息]  %s",

		"tui.placeholder":       "输入消息... (Ctrl+H 帮助)",
		"tui.model_switched":    "已切换模型至: %s",
		"tui.provider_switched": "已切换至: %s（模型: %s）",
		"tui.switch_failed":     "切换失败: %v",
		"tui.no_providers":      "没有配置提供商。",

		"model.title":   "选择提供商 / 模型",
		"model.default": "（默认）",

		"mode.chat":      "聊天",
		"mode.thinking":  "思考中…",
		"mode.streaming": "流式输出中…（Ctrl+C 停止）",

		"help.title":          "帮助",
		"help.keybindings":    "快捷键",
		"help.commands":       "命令",
		"help.tips":           "提示",
		"help.tips_text":      "输入消息后按 Enter 开始对话。\n按 ↑ 回忆上一条消息。\n按 / 查看所有命令。",
		"help.cmd_help_desc":  "显示此帮助",
		"help.cmd_clear_desc": "清屏",
		"help.cmd_model_desc": "显示/切换当前模型",

		"help.kb_quit":     "退出（或停止流式）",
		"help.kb_sessions": "打开会话列表",
		"help.kb_help":     "显示此帮助",
		"help.kb_history":  "输入历史",
		"help.kb_scroll":   "滚动聊天",

		"update.title":            "OpenAIDE 更新",
		"update.script_not_found": "错误: 更新脚本未找到",
		"update.failed":           "\n更新失败: %v\n",
		"update.complete":         "\n更新完成!",

		"git.auto_commit_msg":      "openaide 自动提交",
		"repl.goodbye":             "再见。",
		"repl.resume":              "📋 恢复会话",
		"repl.recent":              "最近消息",
		"repl.placeholder":         "输入问题，Enter 发送，Ctrl+J 换行，Ctrl+C 取消/退出",
		"repl.no_sessions_new":     "没有可恢复的会话，开始新会话",
		"repl.thinking":            "正在思考…",
		"repl.working":             "正在处理…",
		"repl.sub_agent":           "子代理执行中…",
		"repl.done":                "完成",
		"repl.you_label":           "你",
		"repl.assistant_label":     "OpenAIDE",
		"repl.banner_hint":         "/help 查看命令 · @file 引用文件 · Ctrl+C 中断",
		"repl.no_api_key":          "未配置 API Key",
		"repl.openaide_loaded":     "OPENAIDE.md loaded",
		"repl.status_idle":         "● idle",
		"repl.status_approval":     "● awaiting approval",
		"repl.help_line":           "Enter 发送 · Ctrl+J 换行 · Tab 补全 · Ctrl+R 历史 · Alt+Enter 编辑器 · Esc 撤销 · Ctrl+C 退出",
		"repl.help_busy":           "Ctrl+C 取消 · PgUp/PgDn 滚动",
		"repl.approval_title":      "需要授权",
		"repl.approval_keys":       "[y] 允许  [a] 全部允许  [n] 拒绝  [esc] 取消",
		"repl.analyzing":           "分析任务…",
		"repl.deep_analysis":       "复杂任务，启动深度分析…",
		"repl.deep_failed":         "深度分析失败，使用默认计划",
		"repl.plan_failed":         "计划生成失败，使用默认计划",
		"repl.selected":            "已选择: %s",
		"repl.exec_plan":           "执行此计划? (%d 个子任务)",
		"repl.allow_tool":          "选择操作:",
		"repl.approve_yes":         "✓ 允许 (本次)",
		"repl.approve_always":      "✓ 全部允许 (不再询问)",
		"repl.approve_no":          "✗ 拒绝",
		"repl.rounds_exhausted":    "轮次用尽 (%d/%d)，继续分析?",
		"repl.executing":           "执行中",
		"repl.executing_status":    "⏳ 执行中… (%v)",
		"repl.no_models":           "没有可用的模型",
		"repl.canceled":            "已取消",
		"repl.no_sessions":         "没有会话",
		"repl.select_session":      "选择会话 (↑↓ 移动, Enter 确认, Esc 取消)",
		"repl.select_model":        "选择模型 (↑↓ 移动, Enter 确认, Esc 取消)",
		"repl.select_approach":     "选择方案 (↑↓ 移动, Enter 确认)",
		"repl.select_mode":         "选择模式",
		"repl.switched_sess":       "✓ 切换到会话 %s (%d 条消息)",
		"repl.invalid_sess":        "无效的会话编号",
		"repl.usage_sess":          "用法: /session <编号>",
		"repl.help_title":          "通用 AI 助手 — 编程 · 写作 · 研究 · 日常",
		"repl.help_intro":          "Ctrl+C 中断 | Ctrl+R 搜索历史 | Tab 补全 | ↑↓ 历史",
		"repl.help_clear":          "清屏",
		"repl.help_mode":           "切换模式",
		"repl.help_model":          "查看/切换模型",
		"repl.help_lang":           "切换语言",
		"repl.help_log":            "最近日志",
		"repl.help_sessions":       "会话列表",
		"repl.help_handoff":        "保存会话",
		"repl.help_exit":           "退出",
		"repl.help_analyst":        "分析任务",
		"repl.help_coder":          "编码任务",
		"repl.help_reviewer":       "审查任务",
		"repl.help_executor":       "执行/验证",
		"repl.help_team":           "完整团队链",
		"repl.mode_coding":         "模式: 编程助手",
		"repl.mode_writing":        "模式: 写作助手",
		"repl.mode_research":       "模式: 研究分析",
		"repl.mode_general":        "模式: 通用助手",
		"repl.mode_available":      "可用模式: code, write, research, general",
		"repl.mode_code_label":     "code  编程开发",
		"repl.mode_write_label":    "write  写作创作",
		"repl.mode_research_label": "research  研究分析",
		"repl.mode_general_label":  "general  通用助手",
		"repl.lang_zh":             "中文",
		"repl.risk_effort":         "  (风险:%s 工作量:%s)",

		"repl.select_subtasks":  "选择子任务 (%d 个，空格切换，回车确认)",
		"repl.select_action":    "操作: 切换到此会话 | 删除此会话 | 取消",
		"repl.action_switch":    "切换到此会话",
		"repl.action_delete":    "删除此会话",
		"repl.action_cancel":    "取消",
		"repl.export_saved":     "会话已导出: %s",
		"repl.execute_progress": "执行中 [%d/%d]",

		"prompt.file_content": "内容来源 %s:\n---\n%s\n---",
	},
	EN: {
		"cli.usage":        "OpenAIDE — AI Agent Terminal",
		"cli.usage_detail": "Usage:",
		"cli.help":         "help",
		"cli.version":      "OpenAIDE CLI dev",
		"cli.oneshot":      "  openaide <prompt>            One-shot mode",
		"cli.file_oneshot": "  openaide <file> <prompt>     File + prompt",
		"cli.y":            "  openaide -y                  Auto-approve all actions",
		"cli.c":            "  openaide -c                  Continue last session",
		"cli.model":        "  openaide --model <name>      Override model",
		"cli.output":       "  openaide -o json             Structured output",
		"cli.verbose":      "  openaide --verbose           Debug logging",
		"cli.sessions":     "  openaide sessions            List sessions",
		"cli.setup":        "  openaide setup               Interactive setup wizard",
		"cli.update":       "  openaide update              Update",
		"cli.examples":     "Examples:",
		"cli.ex_oneshot":   "  openaide fix this bug",
		"cli.ex_file":      "  openaide main.go review this function",
		"cli.ex_continue":  "  openaide -c -y",
		"cli.ex_model":     "  openaide --model gpt-4o rewrite this",

		"cli.keybindings": "Keybindings:",
		"cli.kb_quit":     "  Ctrl+C / Ctrl+D    Quit",
		"cli.kb_sessions": "  Ctrl+S             Session list",
		"cli.kb_help":     "  F1 / Ctrl+H        Help",
		"cli.kb_history":  "  ↑ / ↓              History",
		"cli.kb_scroll":   "  PgUp / PgDown      Scroll",

		"cli.commands":     "Commands:",
		"cli.cmd_help":     "  /help               Show help",
		"cli.cmd_clear":    "  /clear              Clear chat",
		"cli.cmd_model":    "  /model [name]       Show/switch model",
		"cli.cmd_analyst":  "  /analyst <task>     Analyst role (read-only)",
		"cli.cmd_coder":    "  /coder <task>       Coder role (read-write)",
		"cli.cmd_reviewer": "  /reviewer <task>    Reviewer role (read-only)",
		"cli.cmd_executor": "  /executor <task>    Executor role (tests)",
		"cli.cmd_team":     "  /team <task>        Team collaboration (full chain)",

		"err.start_failed":    "Failed to start: %v",
		"err.process":         "Error: %v",
		"err.unknown_cmd":     "Unknown command: %s (try /help)",
		"warn.read_file":      "Warning: cannot read %s: %v",
		"warn.no_sessions":    "No sessions. Press n to create one.",
		"warn.delete_confirm": "Press d again to delete this session",
		"warn.nav_help":       "↑/↓ navigate · Enter select · n new · d delete · esc/q back",
		"warn.model_help":     "↑/↓ navigate · Enter select · esc/q back",

		"sess.none":        "No sessions found.",
		"sess.info":        "Session | Messages | Preview",
		"sess.list_format": "%-24s  %3d msgs  %s\n",
		"sess.tui_row":     "%s%s  [%d msgs]  %s",

		"tui.placeholder":       "Type a message... (Ctrl+H for help)",
		"tui.model_switched":    "Switched model to: %s",
		"tui.provider_switched": "Switched to: %s (model: %s)",
		"tui.switch_failed":     "Failed to switch: %v",
		"tui.no_providers":      "No providers configured.",

		"model.title":   "Select Provider / Model",
		"model.default": "(default)",

		"mode.chat":      "Chat",
		"mode.thinking":  "Thinking…",
		"mode.streaming": "Streaming… (Ctrl+C to stop)",

		"help.title":          "Help",
		"help.keybindings":    "Keybindings",
		"help.commands":       "Commands",
		"help.tips":           "Tips",
		"help.tips_text":      "Type a message and press Enter to chat.\nPress ↑ to recall previous messages.\nPress / to browse commands.",
		"help.cmd_help_desc":  "Show this help",
		"help.cmd_clear_desc": "Clear chat messages",
		"help.cmd_model_desc": "Show/switch current model",

		"help.kb_quit":     "Quit (or stop streaming)",
		"help.kb_sessions": "Open session list",
		"help.kb_help":     "Show this help",
		"help.kb_history":  "Input history",
		"help.kb_scroll":   "Scroll chat",

		"update.title":            "OpenAIDE Update",
		"update.script_not_found": "Error: update script not found",
		"update.failed":           "\nUpdate failed: %v\n",
		"update.complete":         "\nUpdate complete!",

		"git.auto_commit_msg":      "openaide auto-commit",
		"repl.goodbye":             "Goodbye.",
		"repl.resume":              "📋 Resume session",
		"repl.recent":              "Recent",
		"repl.placeholder":         "Type a question, Enter to send, Ctrl+J newline, Ctrl+C cancel/quit",
		"repl.no_sessions_new":     "No previous sessions — starting a new one",
		"repl.thinking":            "Thinking…",
		"repl.working":             "Working…",
		"repl.sub_agent":           "Sub-agent working…",
		"repl.done":                "Done",
		"repl.you_label":           "You",
		"repl.assistant_label":     "OpenAIDE",
		"repl.banner_hint":         "/help for commands · @file reference · ctrl+c interrupt",
		"repl.no_api_key":          "No API key configured",
		"repl.openaide_loaded":     "OPENAIDE.md loaded",
		"repl.status_idle":         "● idle",
		"repl.status_approval":     "● awaiting approval",
		"repl.help_line":           "enter send · ctrl+j newline · tab complete · ctrl+r history · alt+enter editor · esc undo · ctrl+c quit",
		"repl.help_busy":           "ctrl+c cancel · pgup/pgdn scroll",
		"repl.approval_title":      "Permission Required",
		"repl.approval_keys":       "[y] allow  [a] allow all  [n] deny  [esc] cancel",
		"repl.analyzing":           "Analyzing…",
		"repl.deep_analysis":       "Complex task, deep analysis…",
		"repl.deep_failed":         "Deep analysis failed, using default plan",
		"repl.plan_failed":         "Plan generation failed, using default",
		"repl.selected":            "Selected: %s",
		"repl.exec_plan":           "Execute plan? (%d subtasks)",
		"repl.allow_tool":          "Choose action:",
		"repl.approve_yes":         "✓ Allow (once)",
		"repl.approve_always":      "✓ Allow all (don't ask again)",
		"repl.approve_no":          "✗ Deny",
		"repl.rounds_exhausted":    "Rounds exhausted (%d/%d), continue?",
		"repl.executing":           "Executing",
		"repl.executing_status":    "⏳ Executing… (%v)",
		"repl.no_models":           "No models available",
		"repl.canceled":            "Canceled",
		"repl.no_sessions":         "No sessions",
		"repl.select_session":      "Select session (↑↓ move, Enter select, Esc cancel)",
		"repl.select_model":        "Select model (↑↓ move, Enter select, Esc cancel)",
		"repl.select_approach":     "Select approach (↑↓ move, Enter select)",
		"repl.select_mode":         "Select mode",
		"repl.switched_sess":       "✓ Switched to session %s (%d msgs)",
		"repl.invalid_sess":        "Invalid session number",
		"repl.usage_sess":          "Usage: /session <number>",
		"repl.help_title":          "General AI assistant — coding, writing, research",
		"repl.help_intro":          "Ctrl+C interrupt | Ctrl+R search | Tab complete | ↑↓ history",
		"repl.help_clear":          "clear screen",
		"repl.help_mode":           "switch mode",
		"repl.help_model":          "view/switch model",
		"repl.help_lang":           "switch language",
		"repl.help_log":            "recent logs",
		"repl.help_sessions":       "session list",
		"repl.help_handoff":        "save session",
		"repl.help_exit":           "exit",
		"repl.help_analyst":        "analyze",
		"repl.help_coder":          "code",
		"repl.help_reviewer":       "review",
		"repl.help_executor":       "execute/verify",
		"repl.help_team":           "full team pipeline",
		"repl.mode_coding":         "Mode: coding",
		"repl.mode_writing":        "Mode: writing",
		"repl.mode_research":       "Mode: research",
		"repl.mode_general":        "Mode: general",
		"repl.mode_available":      "Available: code, write, research, general",
		"repl.mode_code_label":     "code  dev/coding",
		"repl.mode_write_label":    "write  writing",
		"repl.mode_research_label": "research  analysis",
		"repl.mode_general_label":  "general  assistant",
		"repl.lang_zh":             "中文 (Chinese)",
		"repl.risk_effort":         "  (risk:%s effort:%s)",

		"repl.select_subtasks":  "Select subtasks (%d total, space=toggle, enter=confirm)",
		"repl.select_action":    "Action: Switch to session | Delete session | Cancel",
		"repl.action_switch":    "Switch to session",
		"repl.action_delete":    "Delete session",
		"repl.action_cancel":    "Cancel",
		"repl.export_saved":     "Session exported: %s",
		"repl.execute_progress": "Executing [%d/%d]",

		"prompt.file_content": "Content of %s:\n---\n%s\n---",
	},
}

var current atomic.Value // Lang — lock-free, read-heavy

func init() { current.Store(EN); SetLang(Detect()) }

func SetLang(l Lang) {
	if _, ok := messages[l]; ok {
		current.Store(l)
	} else {
		current.Store(EN)
	}
}

func GetLang() Lang { return current.Load().(Lang) }

func T(key string, args ...any) string {
	cur := current.Load().(Lang)

	msg, ok := messages[cur]
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
