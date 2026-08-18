package kernel

import (
	"fmt"
	"strings"
	"time"
)

// StuckDetector 检测 agent 是否陷入循环无进展:
//   - 同一工具+参数连续调用 3+ 次(重复尝试)
//   - 3+ 次连续工具失败
//   - 同一错误消息在最近历史中出现 3+ 次
//
// 检测到 stuck 时,内核注入一条 "pivot" 系统消息,
// 强制 agent 反思当前策略并换一种方式尝试。
// 每次 pivot 后有 3 轮冷却期,避免 pivot 消息自身形成循环。
type StuckDetector struct {
	recentCalls      []callRecord // 环形缓冲:最近的工具调用记录
	consecutiveFails int          // 连续失败计数(成功则归零)
	maxHistory       int          // 最多保留多少条历史
	repeatThreshold  int          // 重复多少次判定为 stuck
	lastPivotRound   int          // 上次触发 pivot 的轮次(用于冷却)
	pivotCount       int          // 总 pivot 次数(用于日志和限制)
}

type callRecord struct {
	tool  string
	args  string
	err   string
	round int
	time  time.Time
}

// maxPivotsPerSession 限制单次查询最多 pivot 次数。
// 超过后不再注入 pivot 消息,避免无限循环 ——
// 此时 agent 真的可能需要人类介入。
const maxPivotsPerSession = 3

// NewStuckDetector 创建一个带默认参数的检测器。
func NewStuckDetector() *StuckDetector {
	return &StuckDetector{
		maxHistory:      12,
		repeatThreshold: 3,
	}
}

// RecordResult 记录一次工具执行结果。
// 在每个工具批次执行完后调用(可批量调多次,每个工具一次)。
func (d *StuckDetector) RecordResult(tool, args, errMsg string, round int) {
	rec := callRecord{
		tool:  tool,
		args:  args,
		err:   errMsg,
		round: round,
		time:  time.Now(),
	}
	d.recentCalls = append(d.recentCalls, rec)
	if len(d.recentCalls) > d.maxHistory {
		d.recentCalls = d.recentCalls[len(d.recentCalls)-d.maxHistory:]
	}

	if errMsg != "" {
		d.consecutiveFails++
	} else {
		d.consecutiveFails = 0
	}
}

// IsStuck 检查 agent 是否陷入停滞。
// 返回 (true, reason) 表示需要 pivot,(false, "") 表示正常。
func (d *StuckDetector) IsStuck(round int) (bool, string) {
	// 冷却期:上次 pivot 后 3 轮内不再触发
	// (pivot at round N → rounds N+1, N+2, N+3 are cooldown, N+4+ allowed)
	if d.lastPivotRound > 0 && round-d.lastPivotRound <= 3 {
		return false, ""
	}
	// 总次数限制:超过 maxPivotsPerSession 后不再 pivot
	if d.pivotCount >= maxPivotsPerSession {
		return false, ""
	}

	// 检查 1:连续失败 >= repeatThreshold
	if d.consecutiveFails >= d.repeatThreshold {
		d.markPivot(round)
		return true, fmt.Sprintf("%d consecutive tool failures", d.consecutiveFails)
	}

	if len(d.recentCalls) < d.repeatThreshold {
		return false, ""
	}

	// 检查 2:最近的工具+参数连续重复 repeatThreshold 次
	// (从最后一条往前数,只要连续相同就算)
	last := d.recentCalls[len(d.recentCalls)-1]
	repeats := 0
	for i := len(d.recentCalls) - 1; i >= 0; i-- {
		c := d.recentCalls[i]
		if c.tool == last.tool && c.args == last.args {
			repeats++
		} else {
			break
		}
	}
	if repeats >= d.repeatThreshold {
		d.markPivot(round)
		return true, fmt.Sprintf("repeated %s with identical args %d times", last.tool, repeats)
	}

	// 检查 3:最近 repeatThreshold 条记录的错误消息完全相同
	if last.err != "" {
		errRepeats := 0
		for i := len(d.recentCalls) - 1; i >= 0; i-- {
			if d.recentCalls[i].err == last.err {
				errRepeats++
			} else {
				break
			}
		}
		if errRepeats >= d.repeatThreshold {
			d.markPivot(round)
			return true, fmt.Sprintf("same error repeated %d times: %s", errRepeats, truncStr(last.err, 60))
		}
	}

	return false, ""
}

// markPivot 记录 pivot 触发,用于冷却和限制。
func (d *StuckDetector) markPivot(round int) {
	d.lastPivotRound = round
	d.pivotCount++
}

// PivotMessage 生成 pivot 系统消息,强制 agent 换策略。
func (d *StuckDetector) PivotMessage(reason string) string {
	var sb strings.Builder
	sb.WriteString("[System Pivot] ")
	sb.WriteString(fmt.Sprintf("You appear stuck: %s. ", reason))
	sb.WriteString("STOP repeating the same action. Reflect on why this approach isn't working, then try a DIFFERENT strategy.\n")
	sb.WriteString("Options to consider:\n")
	sb.WriteString("- Re-read the file to verify current state (your cached view may be stale)\n")
	sb.WriteString("- Use a different tool (e.g. diff_edit instead of write_file, or search_files to find the right location first)\n")
	sb.WriteString("- Break the problem into smaller steps and tackle one at a time\n")
	sb.WriteString("- Check for typos, wrong paths, or mismatched whitespace in your inputs\n")
	sb.WriteString("- Ask the user for clarification if the task is genuinely ambiguous\n")
	return sb.String()
}

// PivotCount 返回累计 pivot 次数(用于日志/指标)。
func (d *StuckDetector) PivotCount() int { return d.pivotCount }

// PivotLimitReached 报告是否已达到本查询的 pivot 上限。
// 达到后 IsStuck 不再触发,调用方应注入恢复重定向。
func (d *StuckDetector) PivotLimitReached() bool { return d.pivotCount >= maxPivotsPerSession }
