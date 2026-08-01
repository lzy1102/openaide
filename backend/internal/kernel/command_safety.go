package kernel

import "strings"

// IsDangerousCommand 判断命令是否危险（token 级分析，防黑名单绕过）。
// 这是 execute_command 的最终防线,同时被工具层和审批层使用。
// 相比纯前缀/子串匹配,它能识别:
//   - "rm -rf ./*"                 —— 不匹配 "rm -rf /" 或 "rm -rf *"
//   - "rm -rf --no-preserve-root /" —— 中间隔着参数
//   - "find / -name x -exec rm {} \;" —— 不含 rm -rf 前缀
//   - "echo hi && rm -rf /"        —— 不以 rm 开头
func IsDangerousCommand(cmd string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if lower == "" {
		return false
	}
	// 1. 子串模式:覆盖基础危险写法(含命令替换 $(...) 内部)
	for _, p := range dangerousSubstrings {
		if strings.Contains(lower, p) {
			return true
		}
	}
	// 2. token 级分析:按 shell 分隔符分段,逐段看命令名和参数
	for _, seg := range splitShellSegments(lower) {
		if isDangerousSegment(seg) {
			return true
		}
	}
	return false
}

// dangerousSubstrings 是整条命令任意位置命中即拦截的基础危险模式。
// 用子串而非前缀,确保 "echo hi && rm -rf /"、"$(rm -rf /)" 这类变体也能命中。
var dangerousSubstrings = []string{
	"rm -rf /", "rm -rf ~",
	"rmdir /",
	"sudo rm -rf", "sudo rm -r", "sudo rm -f", "sudo mkfs", "sudo dd",
	"mkfs.ext4", "mkfs.ext3", "mkfs.ext2", "mkfs.xfs", "mkfs.btrfs", "mkfs.fat",
	"dd if=/dev/zero", "of=/dev/sd", "of=/dev/hd",
	"shutdown", "reboot", "halt", "poweroff", "init 0",
	"chmod -R 777", "chmod 777 /", "chown -R",
	"drop table", "drop database", "delete from",
	":(){", ":() {", // fork bomb
	"| sh", "| bash", "| zsh", // 管道执行远程脚本
}

// splitShellSegments 按 shell 分隔符(; && || | 换行)切分命令,
// 忽略引号内和 $(...) 内的分隔符。
func splitShellSegments(cmd string) []string {
	var segs []string
	var cur strings.Builder
	var inQuote byte // 0, '"', '\''
	depth := 0       // $(...) 嵌套深度
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case inQuote != 0:
			cur.WriteByte(c)
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
			cur.WriteByte(c)
		case c == '$' && i+1 < len(cmd) && cmd[i+1] == '(':
			depth++
			cur.WriteString("$(")
			i++
		case c == ')' && depth > 0:
			depth--
			cur.WriteByte(c)
		case depth > 0:
			cur.WriteByte(c)
		case c == ';' || c == '|' || c == '\n' || c == '&':
			if t := strings.TrimSpace(cur.String()); t != "" {
				segs = append(segs, t)
			}
			cur.Reset()
			// 跳过 && / || 的第二个字符
			if i+1 < len(cmd) && cmd[i+1] == c {
				i++
			}
		default:
			cur.WriteByte(c)
		}
	}
	if t := strings.TrimSpace(cur.String()); t != "" {
		segs = append(segs, t)
	}
	return segs
}

// isDangerousSegment 分析单个命令段(一个命令 + 参数)。
func isDangerousSegment(seg string) bool {
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return false
	}
	// 去掉 sudo 前缀,看真实命令
	prog := fields[0]
	rest := fields[1:]
	if prog == "sudo" && len(fields) > 1 {
		prog = fields[1]
		rest = fields[2:]
	}
	joined := strings.Join(fields, " ")

	switch prog {
	case "rm", "unlink":
		return rmDangerous(fields)
	case "rmdir":
		// rmdir / 直接删根
		return hasTarget(rest, "/")
	case "mkfs":
		return true
	case "dd":
		return containsAny(joined, "of=/dev/sd", "of=/dev/hd", "if=/dev/zero", "of=/dev/nvme")
	case "shutdown", "reboot", "halt", "poweroff", "init", "telinit":
		return true
	case "chmod":
		return containsAny(joined, "777", "666") && containsAny(joined, "/", "*", ".")
	case "chown", "chgrp":
		// 递归修改系统目录属主
		if containsAny(joined, "-r", "-R") {
			for _, f := range rest {
				if strings.HasPrefix(f, "/") && !strings.HasPrefix(f, "/tmp") && !strings.HasPrefix(f, "/var") {
					return true
				}
			}
		}
	case "find":
		// find ... -exec rm 或 -delete
		return (strings.Contains(joined, "-exec") && strings.Contains(joined, "rm")) ||
			strings.Contains(joined, "-delete")
	case "sh", "bash", "zsh", "python", "python3", "perl", "ruby", "node", "curl", "wget":
		// 解释器/下载器执行远程内容
		if containsAny(joined, "| sh", "| bash", "| zsh", "-c", "-e") {
			return true
		}
	}
	return false
}

// rmDangerous 解析 rm 参数,识别危险删除目标。
// 只有显式的危险目标(根、家目录、危险通配符、非 /tmp 绝对路径)才拦截,
// 避免误伤常见的 rm -rf ./build、rm -rf *.txt 等安全操作。
func rmDangerous(fields []string) bool {
	var recursive, force bool
	for i := 1; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "-") {
			if strings.Contains(f, "r") || strings.Contains(f, "R") {
				recursive = true
			}
			if strings.Contains(f, "f") {
				force = true
			}
			if strings.Contains(f, "no-preserve-root") {
				return true
			}
			continue
		}
		// 非递归、非强制的单个 rm 文件危险度低,放行
		if !recursive && !force {
			continue
		}
		switch {
		case f == "/" || f == "~" || f == "." || f == ".." || f == "*":
			return true
		case strings.HasPrefix(f, "./") && (f == "./" || f == "./*" || strings.HasSuffix(f, "**")):
			return true
		case strings.HasPrefix(f, "~/") && strings.Contains(f, "*"):
			return true
		case strings.Contains(f, "*") && !strings.HasSuffix(f, ".txt") && !strings.HasSuffix(f, ".log") &&
			!strings.HasSuffix(f, ".tmp") && !strings.HasSuffix(f, ".bak") && !strings.HasSuffix(f, ".zip"):
			// 危险通配符:*.go 等指定后缀可放行,裸 * / ** 拦截
			if f == "*" || strings.HasSuffix(f, "**") || strings.HasPrefix(f, "/") {
				return true
			}
		case strings.HasPrefix(f, "/"):
			// 绝对路径:除 /tmp 下的清理操作,其余视为危险
			if f == "/tmp" || strings.HasPrefix(f, "/tmp/") {
				continue
			}
			return true
		}
	}
	return false
}

func hasTarget(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
