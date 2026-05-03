package control_test

import (
	"context"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/control"
	"github.com/zeefan1555/symphony-go/internal/observability"
)

type fakeSnapshotProvider struct {
	snapshot observability.Snapshot
}

func (p fakeSnapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func TestServiceReadsRuntimeStateFromSnapshotProvider(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	startedAt := generatedAt.Add(-2 * time.Minute)
	dueAt := generatedAt.Add(30 * time.Second)
	lastPollAt := generatedAt.Add(-10 * time.Second)
	nextPollAt := generatedAt.Add(20 * time.Second)
	provider := fakeSnapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: generatedAt,
		Counts:      observability.Counts{Running: 1, Retrying: 1},
		Running: []observability.RunningEntry{{
			IssueID:         "issue-id",
			IssueIdentifier: "ZEE-46",
			State:           "In Progress",
			WorkspacePath:   "/tmp/ZEE-46",
			SessionID:       "session-id",
			TurnCount:       2,
			StartedAt:       startedAt,
			Tokens:          observability.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			RuntimeSeconds:  120,
		}},
		Retrying: []observability.RetryEntry{{
			IssueID:         "retry-id",
			IssueIdentifier: "ZEE-47",
			Attempt:         3,
			DueAt:           dueAt,
			Error:           "rate limited",
			WorkspacePath:   "/tmp/ZEE-47",
		}},
		CodexTotals: observability.CodexTotals{InputTokens: 100, OutputTokens: 50, TotalTokens: 150, SecondsRunning: 500},
		Polling: observability.PollingStatus{
			Checking:     true,
			LastPollAt:   lastPollAt,
			NextPollAt:   nextPollAt,
			NextPollInMS: 20000,
			IntervalMS:   30000,
		},
		LastError: "last error",
	}}

	service := control.NewService(provider)
	state, err := service.RuntimeState(context.Background())
	if err != nil {
		t.Fatalf("RuntimeState returned error: %v", err)
	}

	if state.GeneratedAt != generatedAt {
		t.Fatalf("GeneratedAt = %v, want %v", state.GeneratedAt, generatedAt)
	}
	if state.Counts.Running != 1 || state.Counts.Retrying != 1 {
		t.Fatalf("Counts = %#v, want running=1 retrying=1", state.Counts)
	}
	if len(state.Running) != 1 || state.Running[0].IssueIdentifier != "ZEE-46" {
		t.Fatalf("Running = %#v, want ZEE-46 entry", state.Running)
	}
	if state.Running[0].Tokens.TotalTokens != 15 {
		t.Fatalf("Running tokens = %#v, want total 15", state.Running[0].Tokens)
	}
	if len(state.Retrying) != 1 || state.Retrying[0].Attempt != 3 {
		t.Fatalf("Retrying = %#v, want attempt 3", state.Retrying)
	}
	if state.CodexTotals.TotalTokens != 150 || state.CodexTotals.SecondsRunning != 500 {
		t.Fatalf("CodexTotals = %#v, want tokens=150 seconds=500", state.CodexTotals)
	}
	if !state.Polling.Checking || state.Polling.NextPollInMS != 20000 {
		t.Fatalf("Polling = %#v, want checking next poll in 20000ms", state.Polling)
	}
	if state.LastError != "last error" {
		t.Fatalf("LastError = %q, want last error", state.LastError)
	}
}
