package kernel

import (
	"strings"
	"testing"
)

func TestStuckDetector_NotStuckInitially(t *testing.T) {
	d := NewStuckDetector()
	stuck, _ := d.IsStuck(0)
	if stuck {
		t.Error("new detector should not be stuck")
	}
}

func TestStuckDetector_SuccessResetsFails(t *testing.T) {
	d := NewStuckDetector()
	// 2 failures (below threshold)
	d.RecordResult("read_file", `{"path":"a"}`, "not found", 0)
	d.RecordResult("read_file", `{"path":"b"}`, "not found", 1)
	stuck, _ := d.IsStuck(2)
	if stuck {
		t.Error("2 failures should not trigger stuck")
	}
	// success resets the counter
	d.RecordResult("read_file", `{"path":"c"}`, "", 2)
	stuck, _ = d.IsStuck(3)
	if stuck {
		t.Error("success should reset fail counter")
	}
}

func TestStuckDetector_ThreeConsecutiveFails(t *testing.T) {
	d := NewStuckDetector()
	d.RecordResult("write_file", `{"path":"a"}`, "permission denied", 0)
	d.RecordResult("write_file", `{"path":"a"}`, "permission denied", 1)
	d.RecordResult("write_file", `{"path":"a"}`, "permission denied", 2)
	stuck, reason := d.IsStuck(3)
	if !stuck {
		t.Error("3 consecutive failures should trigger stuck")
	}
	if !strings.Contains(reason, "consecutive") {
		t.Errorf("reason should mention consecutive, got: %s", reason)
	}
	if d.PivotCount() != 1 {
		t.Errorf("expected pivot count 1, got %d", d.PivotCount())
	}
}

func TestStuckDetector_SameArgsRepeated(t *testing.T) {
	d := NewStuckDetector()
	args := `{"path":"config.json","content":"..."}`
	// 3 identical calls (even with success — still indicates loop)
	d.RecordResult("write_file", args, "", 0)
	d.RecordResult("write_file", args, "", 1)
	d.RecordResult("write_file", args, "", 2)
	stuck, reason := d.IsStuck(3)
	if !stuck {
		t.Error("3 identical calls should trigger stuck")
	}
	if !strings.Contains(reason, "identical args") {
		t.Errorf("reason should mention identical args, got: %s", reason)
	}
}

func TestStuckDetector_DifferentArgsNotStuck(t *testing.T) {
	d := NewStuckDetector()
	d.RecordResult("read_file", `{"path":"a.go"}`, "", 0)
	d.RecordResult("read_file", `{"path":"b.go"}`, "", 1)
	d.RecordResult("read_file", `{"path":"c.go"}`, "", 2)
	stuck, _ := d.IsStuck(3)
	if stuck {
		t.Error("different args should not trigger stuck")
	}
}

func TestStuckDetector_SameErrorRepeated(t *testing.T) {
	d := NewStuckDetector()
	// 3 calls with same error but different args
	d.RecordResult("search_files", `{"pattern":"foo","path":"a"}`, "no matches", 0)
	d.RecordResult("search_files", `{"pattern":"bar","path":"b"}`, "no matches", 1)
	d.RecordResult("search_files", `{"pattern":"baz","path":"c"}`, "no matches", 2)
	stuck, reason := d.IsStuck(3)
	// Note: consecutiveFails=3 also triggers, so this should be stuck
	if !stuck {
		t.Error("3 same errors should trigger stuck")
	}
	// Reason could be either "consecutive" or "same error"
	if !strings.Contains(reason, "consecutive") && !strings.Contains(reason, "same error") {
		t.Errorf("reason should mention consecutive or same error, got: %s", reason)
	}
}

func TestStuckDetector_CooldownAfterPivot(t *testing.T) {
	d := NewStuckDetector()
	// Trigger first pivot at round 3
	d.RecordResult("write_file", `{"path":"a"}`, "err", 0)
	d.RecordResult("write_file", `{"path":"a"}`, "err", 1)
	d.RecordResult("write_file", `{"path":"a"}`, "err", 2)
	stuck, _ := d.IsStuck(3)
	if !stuck {
		t.Fatal("expected first pivot at round 3")
	}
	// Immediately after (within cooldown), should not pivot even if conditions met
	d.RecordResult("write_file", `{"path":"a"}`, "err", 4)
	d.RecordResult("write_file", `{"path":"a"}`, "err", 5)
	stuck, _ = d.IsStuck(6)
	if stuck {
		t.Error("should not pivot during cooldown (round 6, last pivot at 3, need 3+ gap)")
	}
	// After cooldown (round 3 + 3 = 6, so round 7+)
	d.RecordResult("write_file", `{"path":"a"}`, "err", 7)
	stuck, _ = d.IsStuck(8)
	if !stuck {
		t.Error("should pivot again after cooldown expires")
	}
}

func TestStuckDetector_MaxPivotsCap(t *testing.T) {
	d := NewStuckDetector()
	// Trigger pivot 1 at round 3
	for i := 0; i < 3; i++ {
		d.RecordResult("write_file", `{"path":"a"}`, "err", i)
	}
	d.IsStuck(3)
	// Trigger pivot 2 at round 8 (after cooldown)
	for i := 4; i < 7; i++ {
		d.RecordResult("write_file", `{"path":"a"}`, "err", i)
	}
	d.IsStuck(8)
	// Trigger pivot 3 at round 13
	for i := 9; i < 12; i++ {
		d.RecordResult("write_file", `{"path":"a"}`, "err", i)
	}
	d.IsStuck(13)
	if d.PivotCount() != 3 {
		t.Fatalf("expected 3 pivots, got %d", d.PivotCount())
	}
	// 4th pivot attempt should be blocked by maxPivotsPerSession
	for i := 14; i < 17; i++ {
		d.RecordResult("write_file", `{"path":"a"}`, "err", i)
	}
	stuck, _ := d.IsStuck(18)
	if stuck {
		t.Error("should not pivot beyond maxPivotsPerSession")
	}
}

func TestStuckDetector_PivotMessage(t *testing.T) {
	d := NewStuckDetector()
	msg := d.PivotMessage("3 consecutive tool failures")
	if !strings.Contains(msg, "[System Pivot]") {
		t.Error("pivot message should have [System Pivot] prefix")
	}
	if !strings.Contains(msg, "3 consecutive tool failures") {
		t.Error("pivot message should contain the reason")
	}
	if !strings.Contains(msg, "STOP") {
		t.Error("pivot message should instruct to stop")
	}
	if !strings.Contains(msg, "different") {
		t.Error("pivot message should suggest different strategy")
	}
}

func TestStuckDetector_HistoryTrimmed(t *testing.T) {
	d := NewStuckDetector()
	// Record more than maxHistory (12) calls
	for i := 0; i < 20; i++ {
		d.RecordResult("read_file", `{"path":"f"}`, "", i)
	}
	// Should not panic and should detect stuck (last 3+ are identical)
	stuck, _ := d.IsStuck(20)
	if !stuck {
		t.Error("identical calls should trigger stuck even with trimmed history")
	}
}
