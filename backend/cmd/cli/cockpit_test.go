package main

import (
	"strings"
	"testing"
	"time"

	"openaide/backend/internal/infra"
)

func testModel() tuiModel {
	m := initialTUI(&infra.Application{}, false)
	m.width = 120
	m.height = 40
	m.modelName = "test-model"
	m.sessionID = "sess_abc123"
	m.gitBranch = "master"
	m.gitDirty = true
	return m
}

func TestHudView(t *testing.T) {
	m := testModel()
	hud := m.hudView()
	if !strings.Contains(hud, "test-model") {
		t.Errorf("HUD missing model: %q", hud)
	}
	if !strings.Contains(hud, "master") {
		t.Errorf("HUD missing git branch: %q", hud)
	}
	if !strings.Contains(hud, "✚") {
		t.Errorf("HUD missing dirty marker: %q", hud)
	}
}

func TestGaugeViewIdle(t *testing.T) {
	m := testModel()
	m.idle.sessionCount = 3
	m.idle.toolCount = 42
	m.idle.factCount = 7
	m.idle.learnCount = 5
	got := m.gaugeView()
	for _, want := range []string{"3", "42", "7", "5"} {
		if !strings.Contains(got, want) {
			t.Errorf("idle gauge missing %q: %q", want, got)
		}
	}
}

func TestGaugeViewIdleZero(t *testing.T) {
	m := testModel()
	got := m.gaugeView()
	for _, want := range []string{"0", "0"} {
		if !strings.Contains(got, want) {
			t.Errorf("idle gauge with zero stats should render, missing %q: %q", want, got)
		}
	}
}

func TestSidePanelIdleRecentSessions(t *testing.T) {
	m := testModel()
	m.mode = modeIdle
	m.idle.recent = []idleRecentSession{
		{id: "sess_11111111", msgCount: 12, updatedAt: time.Now().Add(-5 * time.Minute)},
		{id: "sess_22222222", msgCount: 3, updatedAt: time.Now().Add(-2 * time.Hour)},
	}
	got := m.sidePanel()
	for _, want := range []string{"sess_111", "sess_222", "12 msgs", "3 msgs", "5m", "2h"} {
		if !strings.Contains(got, want) {
			t.Errorf("idle side panel missing %q: %q", want, got)
		}
	}
}

func TestSidePanelIdleEmpty(t *testing.T) {
	m := testModel()
	m.mode = modeIdle
	if got := m.sidePanel(); got != "" {
		t.Errorf("idle side panel without sessions should be empty, got %q", got)
	}
}

func TestGaugeViewBusy(t *testing.T) {
	m := testModel()
	m.mode = modeStreaming
	m.stream.startTime = time.Now().Add(-45 * time.Second)
	m.stream.totalTokens = 1200
	m.stream.totalTools = 8
	m.stream.streamRound = 3
	m.stream.streamTotal = 10
	m.stream.cacheHit = 70
	m.stream.cacheMiss = 30
	got := m.gaugeView()
	for _, want := range []string{"1.2k", "8", "3/10", "45s", "70%"} {
		if !strings.Contains(got, want) {
			t.Errorf("busy gauge missing %q: %q", want, got)
		}
	}
}

func TestSidePanelNarrowTerminal(t *testing.T) {
	m := testModel()
	m.width = 80
	m.mode = modePlanExec
	m.plan.tasks = []taskState{{title: "t1", status: taskDone}}
	if got := m.sidePanel(); got != "" {
		t.Errorf("narrow terminal should hide side panel, got %q", got)
	}
}

func TestSidePanelWidePlan(t *testing.T) {
	m := testModel()
	m.mode = modePlanExec
	m.plan.tasks = []taskState{{title: "t1", status: taskDone}, {title: "t2", status: taskRunning, role: "coder"}}
	got := m.sidePanel()
	for _, want := range []string{"t1", "t2", "coder"} {
		if !strings.Contains(got, want) {
			t.Errorf("side panel missing %q: %q", want, got)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		in  int
		out string
	}{
		{0, "0"}, {999, "999"}, {1000, "1.0k"}, {2500, "2.5k"},
	}
	for _, c := range cases {
		if got := formatTokens(c.in); got != c.out {
			t.Errorf("formatTokens(%d) = %q, want %q", c.in, got, c.out)
		}
	}
}
