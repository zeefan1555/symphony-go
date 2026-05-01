package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/observability"
)

func TestRenderSnapshotShowsCoreSections(t *testing.T) {
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	snapshot := observability.NewSnapshot()
	snapshot.GeneratedAt = now
	snapshot.Counts = observability.Counts{Running: 1, Retrying: 1}
	snapshot.CodexTotals = observability.CodexTotals{
		InputTokens:    100,
		OutputTokens:   50,
		TotalTokens:    150,
		SecondsRunning: 60,
	}
	snapshot.RateLimits = map[string]any{"remaining": 3}
	snapshot.Polling = observability.PollingStatus{NextPollInMS: 5000}
	snapshot.Running = []observability.RunningEntry{{
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-123",
		State:           "In Progress",
		SessionID:       "session-abcdef123456",
		TurnCount:       7,
		LastMessage:     "working on tests",
		StartedAt:       now.Add(-2 * time.Minute),
		Tokens:          observability.TokenUsage{TotalTokens: 150},
	}}
	snapshot.Retrying = []observability.RetryEntry{{
		IssueIdentifier: "ZEE-124",
		Attempt:         2,
		DueAt:           now.Add(30 * time.Second),
		Error:           "rate limited",
	}}

	frame := Render(snapshot, Options{ProjectSlug: "demo-project", MaxAgents: 5})

	for _, want := range []string{
		"SYMPHONY STATUS",
		"│ Project: https://linear.app/project/demo-project/issues",
		"│ Next refresh: 5s",
		"│ Agents: 1/5",
		"│ Throughput: 0 tps",
		"│ Runtime: 3m 0s",
		"│ Tokens: in 100 | out 50 | total 150",
		"│ Rate Limits: not limited | remaining 3",
		"Running",
		"● ZEE-123  In Progress",
		"2m 0s / 7",
		"sess...123456",
		"working on tests",
		"Backoff queue",
		"↻ ZEE-124 attempt=2 in 30.000s error=rate limited",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("Render() missing %q in:\n%s", want, frame)
		}
	}

	clearFrame := ClearAndRender(frame)
	if !strings.HasPrefix(clearFrame, "\033[H\033[2J") {
		t.Fatalf("ClearAndRender() missing ANSI prefix: %q", clearFrame)
	}
	if !strings.HasSuffix(clearFrame, "\n") {
		t.Fatalf("ClearAndRender() missing trailing newline: %q", clearFrame)
	}
}

func TestRenderSnapshotSummarizesCodexRateLimits(t *testing.T) {
	snapshot := observability.NewSnapshot()
	snapshot.RateLimits = map[string]any{
		"rateLimitReachedType": nil,
		"primary": map[string]any{
			"usedPercent":        1.0,
			"windowDurationMins": 300.0,
		},
		"secondary": map[string]any{
			"usedPercent":        60.0,
			"windowDurationMins": 10080.0,
		},
	}

	frame := Render(snapshot, Options{})

	want := "│ Rate Limits: not limited | primary 1% used / 300m window | secondary 60% used / 10080m window"
	if !strings.Contains(frame, want) {
		t.Fatalf("Render() missing summarized rate limits %q in:\n%s", want, frame)
	}
	if strings.Contains(frame, "{\"") {
		t.Fatalf("Render() should not expose raw rate limit JSON:\n%s", frame)
	}
}

func TestRenderSnapshotCanUseSemanticColor(t *testing.T) {
	snapshot := observability.NewSnapshot()
	snapshot.Counts = observability.Counts{Running: 1, Retrying: 1}
	snapshot.Running = []observability.RunningEntry{{IssueIdentifier: "ZEE-1"}}
	snapshot.Retrying = []observability.RetryEntry{{IssueIdentifier: "ZEE-2", Attempt: 1}}

	frame := Render(snapshot, Options{Color: true})

	for _, want := range []string{
		"\033[1m\033[36mSYMPHONY STATUS\033[0m",
		"\033[1m\033[32mRunning\033[0m",
		"\033[32m●\033[0m",
		"\033[1m\033[33mBackoff queue\033[0m",
		"\033[33m↻\033[0m",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("Render() missing color sequence %q in:\n%s", want, frame)
		}
	}
}

func TestRenderSnapshotShowsNoActiveAgents(t *testing.T) {
	snapshot := observability.NewSnapshot()
	snapshot.GeneratedAt = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	frame := Render(snapshot, Options{})

	for _, want := range []string{
		"No active agents",
		"No queued retries",
		"╰─",
	} {
		if !strings.Contains(frame, want) {
			t.Fatalf("Render() missing %q in:\n%s", want, frame)
		}
	}
}
