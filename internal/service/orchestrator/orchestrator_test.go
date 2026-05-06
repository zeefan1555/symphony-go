package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/runtime/logging"
	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

func TestSnapshotStartsWithEmptyCollections(t *testing.T) {
	o := New(Options{Workflow: &runtimeconfig.Workflow{Config: runtimeconfig.Config{}}})

	snapshot := o.Snapshot()
	if snapshot.Running == nil {
		t.Fatal("snapshot Running must be non-nil")
	}
	if snapshot.Retrying == nil {
		t.Fatal("snapshot Retrying must be non-nil")
	}
}

func TestRequestRefreshSignalsPollWithoutBlocking(t *testing.T) {
	o := New(Options{Workflow: &runtimeconfig.Workflow{Config: runtimeconfig.Config{}}})

	queued, err := o.RequestRefresh(context.Background())
	if err != nil {
		t.Fatalf("RequestRefresh returned error: %v", err)
	}
	if !queued {
		t.Fatal("first RequestRefresh queued = false, want true")
	}

	queued, err = o.RequestRefresh(context.Background())
	if err != nil {
		t.Fatalf("second RequestRefresh returned error: %v", err)
	}
	if queued {
		t.Fatal("second RequestRefresh queued = true, want false while pending")
	}

	select {
	case <-o.pollNow:
	default:
		t.Fatal("pollNow was not signaled")
	}

	queued, err = o.RequestRefresh(context.Background())
	if err != nil {
		t.Fatalf("third RequestRefresh returned error: %v", err)
	}
	if !queued {
		t.Fatal("third RequestRefresh queued = false, want true after signal is consumed")
	}
}

func TestSnapshotTracksCodexEventTokens(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-123",
		Title:      "snapshot test",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	runner := &snapshotRunner{
		eventEmitted: make(chan struct{}),
		release:      make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	done := make(chan error, 1)
	go func() {
		done <- o.runAgent(ctx, issue, 0)
	}()

	select {
	case <-runner.eventEmitted:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for runner event: %v", ctx.Err())
	}

	snapshot := o.Snapshot()
	if len(snapshot.Running) != 1 {
		t.Fatalf("running entries = %d, want 1: %#v", len(snapshot.Running), snapshot.Running)
	}
	entry := snapshot.Running[0]
	if entry.IssueID != issue.ID || entry.IssueIdentifier != issue.Identifier {
		t.Fatalf("running entry issue = %#v", entry)
	}
	if entry.LastEvent != "token_usage" {
		t.Fatalf("last event = %q, want token_usage", entry.LastEvent)
	}
	if entry.LastMessage != "token usage updated" {
		t.Fatalf("last message = %q, want token usage updated", entry.LastMessage)
	}
	if entry.LastEventAt.IsZero() {
		t.Fatal("last event time must be set")
	}
	if entry.Tokens.InputTokens != 10 || entry.Tokens.OutputTokens != 4 || entry.Tokens.TotalTokens != 14 {
		t.Fatalf("entry tokens = %#v", entry.Tokens)
	}
	if snapshot.CodexTotals.InputTokens != 10 || snapshot.CodexTotals.OutputTokens != 4 || snapshot.CodexTotals.TotalTokens != 14 {
		t.Fatalf("codex totals = %#v", snapshot.CodexTotals)
	}
	rateLimits, ok := snapshot.RateLimits.(map[string]any)
	if !ok || rateLimits["remaining"] != 3.0 {
		t.Fatalf("rate limits = %#v", snapshot.RateLimits)
	}
	rateLimits["remaining"] = 0.0
	nextRateLimits, ok := o.Snapshot().RateLimits.(map[string]any)
	if !ok || nextRateLimits["remaining"] != 3.0 {
		t.Fatalf("snapshot returned mutable rate limits: %#v", nextRateLimits)
	}

	close(runner.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runAgent returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for runAgent completion: %v", ctx.Err())
	}

	if running := o.Snapshot().Running; len(running) != 0 {
		t.Fatalf("running entries after completion = %#v, want empty", running)
	}
}

func TestLogIssueWritesStructuredAndLegacyIssueFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	logger, err := logging.New(path)
	if err != nil {
		t.Fatal(err)
	}
	o := New(Options{Logger: logger})

	issue := issuemodel.Issue{ID: "issue-id", Identifier: "ZEE-8"}
	o.logIssue(issue, "dispatch_skipped", "claimed", nil)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var event logging.Event
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if event.IssueID != issue.ID || event.IssueIdentifier != issue.Identifier {
		t.Fatalf("structured issue fields = %#v", event)
	}
	if event.Fields["issue_id"] != issue.ID || event.Fields["issue_identifier"] != issue.Identifier {
		t.Fatalf("legacy issue fields = %#v", event.Fields)
	}
}

func TestPollAddsRetryEntryWhenWorkspacePreparationFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-FAIL",
		Title:      "workspace failure",
		State:      "In Progress",
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Polling: runtimeconfig.PollingConfig{IntervalMS: 5000},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker: &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), runtimeconfig.HooksConfig{
			AfterCreate: "echo workspace failed >&2; exit 7",
		}),
		Runner: &snapshotRunner{
			eventEmitted: make(chan struct{}),
			release:      make(chan struct{}),
		},
	})

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	var snapshot observability.Snapshot
	for {
		snapshot = o.Snapshot()
		if len(snapshot.Retrying) == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for retry entry; snapshot=%#v: %v", snapshot, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	retry := snapshot.Retrying[0]
	if retry.IssueIdentifier != issue.Identifier || retry.Attempt != 1 {
		t.Fatalf("retry entry = %#v", retry)
	}
	if !strings.Contains(retry.Error, "after_create hook failed") {
		t.Fatalf("retry error = %q, want hook failure", retry.Error)
	}
}

func TestPollDispatchesTwoIssuesConcurrently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issues := []issuemodel.Issue{
		{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"},
		{ID: "issue-2", Identifier: "ZEE-2", Title: "two", State: "In Progress"},
	}
	runner := &blockingRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 2},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: issues},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-runner.started:
			seen[id] = true
		case <-ctx.Done():
			t.Fatalf("timed out waiting for concurrent starts; seen=%v: %v", seen, ctx.Err())
		}
	}

	if !seen["issue-1"] || !seen["issue-2"] {
		t.Fatalf("started issues = %v, want both issues", seen)
	}
	close(runner.release)
}

func TestRunOnceWaitsForDispatchedWorkerCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: []issuemodel.Issue{issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
		Once:      true,
	})

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()

	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}

	select {
	case err := <-done:
		t.Fatalf("Run returned before worker completion: %v", err)
	default:
	}

	close(runner.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for Run completion: %v", ctx.Err())
	}
}

func TestPollDoesNotDispatchClaimedIssueTwice(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	runner := &blockingRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 2},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: []issuemodel.Issue{issue, issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for first dispatch: %v", ctx.Err())
	}
	select {
	case id := <-runner.started:
		t.Fatalf("duplicate issue dispatched: %s", id)
	case <-time.After(50 * time.Millisecond):
	}
	close(runner.release)
}

func TestDispatchIssueClaimsIssueDirectly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: []issuemodel.Issue{issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if !o.dispatchIssue(ctx, issue, 0) {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	state := o.eligibilityState()
	if !state.claimed[issue.ID] {
		t.Fatalf("issue %s was not claimed by dispatchIssue", issue.ID)
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	close(runner.release)
}

func TestWorkerNormalExitSchedulesContinuationRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	timerFactory, delays := captureRetryTimers()
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxRetryBackoffMS: 60_000},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    &recordingRunner{},
		NewTimer:  timerFactory,
	})

	o.claimIssue(issue)
	o.dispatchIssue(ctx, issue, 0)

	select {
	case delay := <-delays:
		if delay != time.Second {
			t.Fatalf("normal retry delay = %s, want 1s", delay)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for normal retry scheduling: %v", ctx.Err())
	}
}

func TestWorkerTurnRefreshDoesNotAutoHandoffOrEnterRetryQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	tracker := &snapshotTracker{issue: issue}
	timerFactory, delays := captureRetryTimers()
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress", "Human Review"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxRetryBackoffMS: 60_000},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    commitRunner{},
		NewTimer:  timerFactory,
	})

	done, ok := o.dispatchIssueDone(ctx, issue, 0)
	if !ok {
		t.Fatal("dispatchIssueDone returned false, want dispatch")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker exit: %v", ctx.Err())
	}
	if tracker.updatedState != "" {
		t.Fatalf("updated state = %q, want no automatic handoff", tracker.updatedState)
	}
	if len(o.Snapshot().Retrying) != 0 {
		t.Fatalf("retry queue = %#v, want empty after handoff", o.Snapshot().Retrying)
	}
	state := o.eligibilityState()
	if state.claimed[issue.ID] {
		t.Fatalf("issue %s stayed claimed after handoff", issue.ID)
	}
	select {
	case delay := <-delays:
		t.Fatalf("scheduled retry delay %s after handoff", delay)
	default:
	}
}

func TestWorkerFailureSchedulesFirstBackoffRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	timerFactory, delays := captureRetryTimers()
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxRetryBackoffMS: 60_000},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker: &listTracker{issues: []issuemodel.Issue{issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), runtimeconfig.HooksConfig{
			AfterCreate: "echo workspace failed >&2; exit 7",
		}),
		Runner:   noCommitRunner{},
		NewTimer: timerFactory,
	})

	o.claimIssue(issue)
	o.dispatchIssue(ctx, issue, 0)

	select {
	case delay := <-delays:
		if delay != 10*time.Second {
			t.Fatalf("failure retry delay = %s, want 10s", delay)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for failure retry scheduling: %v", ctx.Err())
	}
}

func TestHandleRetryFetchErrorRequeuesAndKeepsClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	timerFactory, delays := captureRetryTimers()
	tracker := &listTracker{
		issues:         []issuemodel.Issue{issue},
		fetchActiveErr: errors.New("linear unavailable"),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxRetryBackoffMS: 60_000},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:  tracker,
		NewTimer: timerFactory,
	})

	o.claimIssue(issue)
	o.scheduleRetry(issue, 1, retryFailure, errors.New("initial failure"))
	select {
	case <-delays:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for initial retry scheduling: %v", ctx.Err())
	}

	o.handleRetry(issue.ID)

	select {
	case delay := <-delays:
		if delay != 20*time.Second {
			t.Fatalf("fetch error retry delay = %s, want 20s", delay)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for fetch error retry scheduling: %v", ctx.Err())
	}
	state := o.eligibilityState()
	if !state.claimed[issue.ID] {
		t.Fatalf("issue %s was released after retry fetch error", issue.ID)
	}
	snapshot := o.Snapshot()
	if len(snapshot.Retrying) != 1 {
		t.Fatalf("retrying entries = %#v, want one", snapshot.Retrying)
	}
	retry := snapshot.Retrying[0]
	if retry.Attempt != 2 {
		t.Fatalf("retry attempt = %d, want 2", retry.Attempt)
	}
	if !strings.Contains(retry.Error, "retry poll failed: linear unavailable") {
		t.Fatalf("retry error = %q, want retry poll failure", retry.Error)
	}
}

func TestCanceledWorkerDoesNotScheduleRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	timerFactory, delays := captureRetryTimers()
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
		NewTimer:  timerFactory,
	})

	if !o.dispatchIssue(ctx, issue, 0) {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker start")
	}
	cancel()
	waitForCondition(t, 2*time.Second, func() bool {
		return len(o.Snapshot().Running) == 0
	})
	select {
	case delay := <-delays:
		t.Fatalf("scheduled retry after cancellation with delay %s", delay)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHandleRetryDoesNotDispatchAfterRunContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	timerFactory, _ := captureRetryTimers()
	runner := &countingRunner{started: make(chan string, 1)}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: []issuemodel.Issue{issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
		NewTimer:  timerFactory,
	})

	done := make(chan error, 1)
	go func() {
		done <- o.Run(ctx)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Run to exit")
	}

	o.claimIssue(issue)
	o.scheduleRetry(issue, 1, retryContinuation, nil)
	o.handleRetry(issue.ID)
	select {
	case id := <-runner.started:
		t.Fatalf("worker started after service cancellation: %s", id)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestConcurrentRetryTimersHonorGlobalSlots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issues := []issuemodel.Issue{
		{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"},
		{ID: "issue-2", Identifier: "ZEE-2", Title: "two", State: "In Progress"},
	}
	runner := &blockingRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1, MaxConcurrentAgents: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &listTracker{issues: issues},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})
	for _, issue := range issues {
		o.claimIssue(issue)
		o.scheduleRetry(issue, 1, retryContinuation, nil)
	}

	ready := make(chan struct{})
	done := make(chan struct{}, 2)
	for _, id := range []string{"issue-1", "issue-2"} {
		go func(issueID string) {
			<-ready
			o.handleRetry(issueID)
			done <- struct{}{}
		}(id)
	}
	close(ready)
	for range issues {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatalf("timed out waiting for handleRetry: %v", ctx.Err())
		}
	}

	select {
	case startedID := <-runner.started:
		losingID := "issue-1"
		if startedID == losingID {
			losingID = "issue-2"
		}
		snapshot := o.Snapshot()
		if len(snapshot.Retrying) != 1 {
			t.Fatalf("retrying entries = %#v, want losing issue retained", snapshot.Retrying)
		}
		retry := snapshot.Retrying[0]
		if retry.IssueID != losingID {
			t.Fatalf("retry issue = %s, want losing issue %s", retry.IssueID, losingID)
		}
		if retry.Attempt != 2 {
			t.Fatalf("retry attempt = %d, want 2", retry.Attempt)
		}
		if !strings.Contains(retry.Error, "no available orchestrator slots") {
			t.Fatalf("retry error = %q, want no available orchestrator slots", retry.Error)
		}
		state := o.eligibilityState()
		if !state.claimed[losingID] {
			t.Fatalf("losing issue %s was released instead of claimed for retry", losingID)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for first worker: %v", ctx.Err())
	}
	select {
	case id := <-runner.started:
		t.Fatalf("over-dispatched retry worker despite max concurrency 1: %s", id)
	case <-time.After(50 * time.Millisecond):
	}
	close(runner.release)
}

func TestReconcileTerminalStateCancelsWorkerAndCleansWorkspace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-TERM", Title: "terminal", State: "In Progress"}
	runner := &terminalBlockingRunner{
		started:        make(chan string, 1),
		cancelObserved: make(chan struct{}),
		release:        make(chan struct{}),
	}
	workspaceManager := workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook())
	tracker := &reconciliationTracker{
		stateIssues: []issuemodel.Issue{{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Done"}},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspaceManager,
		Runner:    runner,
	})

	done, ok := o.dispatchIssueDone(ctx, issue, 0)
	if !ok {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	workspacePath, err := workspaceManager.PathForIssue(issue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace should exist before reconciliation: %v", err)
	}

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}

	select {
	case <-runner.cancelObserved:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker cancellation: %v", ctx.Err())
	}
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace should remain until worker exits: %v", err)
	}
	close(runner.release)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker exit: %v", ctx.Err())
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace after terminal reconcile err = %v, want removed", err)
	}
	if calls := tracker.calls(); strings.Join(calls, ",") != "FetchIssueStatesByIDs,FetchActiveIssues" {
		t.Fatalf("tracker calls = %v, want state refresh before active fetch", calls)
	}
}

func TestReconcileTerminalCleanupUsesWorkerWorkspaceAfterReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-RELOAD", Title: "reload", State: "In Progress"}
	runner := &terminalBlockingRunner{
		started:        make(chan string, 1),
		cancelObserved: make(chan struct{}),
		release:        make(chan struct{}),
	}
	tracker := &reconciliationTracker{
		stateIssues: []issuemodel.Issue{{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Done"}},
	}
	reloader := &sequenceReloader{
		current: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 50},
				Workspace: runtimeconfig.WorkspaceConfig{Root: newRoot},
				Agent:     runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "new {{ issue.identifier }}",
		},
		errs: []error{nil},
	}
	workspaceManager := workspace.New(oldRoot, gitSeedHook())
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Polling: runtimeconfig.PollingConfig{IntervalMS: 50},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "old {{ issue.identifier }}",
		},
		Reloader:  reloader,
		Tracker:   tracker,
		Workspace: workspaceManager,
		Runner:    runner,
		WorkspaceFactory: func(cfg runtimeconfig.WorkspaceConfig, hooks runtimeconfig.HooksConfig) *workspace.Manager {
			return workspace.New(cfg.Root, hooks)
		},
	})

	done, ok := o.dispatchIssueDone(ctx, issue, 0)
	if !ok {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	workspacePath, err := workspaceManager.PathForIssue(issue)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace should exist before reconciliation: %v", err)
	}
	if failed := o.refreshWorkflow(); failed {
		t.Fatal("refreshWorkflow returned failure")
	}
	if o.opts.Workspace.Root != newRoot {
		t.Fatalf("workspace root = %q, want %q", o.opts.Workspace.Root, newRoot)
	}

	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatalf("reconcileRunning returned error: %v", err)
	}

	select {
	case <-runner.cancelObserved:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker cancellation: %v", ctx.Err())
	}
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace should remain until worker exits: %v", err)
	}
	close(runner.release)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker exit: %v", ctx.Err())
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace after terminal cleanup err = %v, want removed", err)
	}
}

func TestWorkerExitTerminalStateCleansWorkspace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-DONE-EXIT", Title: "terminal after exit", State: "Merging"}
	workspaceManager := workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook())
	workspacePath, err := workspaceManager.PathForIssue(issue)
	if err != nil {
		t.Fatal(err)
	}
	tracker := &recordingTracker{issue: issuemodel.Issue{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Done"}}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"Merging"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspaceManager,
		Runner:    &recordingRunner{},
	})

	done, ok := o.dispatchIssueDone(ctx, issue, 0)
	if !ok {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker exit: %v", ctx.Err())
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("workspace after terminal worker exit err = %v, want removed", err)
	}
	if state := o.eligibilityState(); state.claimed[issue.ID] {
		t.Fatalf("terminal issue remained claimed after cleanup")
	}
}

func TestReconcileNonActiveStateCancelsWorkerWithoutWorkspaceCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-REVIEW", Title: "review", State: "In Progress"}
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	workspaceManager := workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook())
	tracker := &reconciliationTracker{
		stateIssues: []issuemodel.Issue{{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "In Review"}},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspaceManager,
		Runner:    runner,
	})

	if !o.dispatchIssue(ctx, issue, 0) {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	workspacePath, err := workspaceManager.PathForIssue(issue)
	if err != nil {
		t.Fatal(err)
	}

	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatalf("reconcileRunning returned error: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return len(o.Snapshot().Running) == 0
	})
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("workspace after non-active reconcile should remain: %v", err)
	}
}

func TestReconcileStalledWorkerCancelsAndSchedulesRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-STALL", Title: "stall", State: "In Progress"}
	timerFactory, delays := captureRetryTimers()
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1, MaxRetryBackoffMS: 60_000},
				Codex: runtimeconfig.CodexConfig{StallTimeoutMS: 100},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   &reconciliationTracker{},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
		NewTimer:  timerFactory,
	})

	if !o.dispatchIssue(ctx, issue, 0) {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	o.mu.Lock()
	o.snapshot.Running[0].StartedAt = time.Now().Add(-time.Second)
	o.snapshot.Running[0].LastEventAt = time.Now().Add(-time.Second)
	o.mu.Unlock()

	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatalf("reconcileRunning returned error: %v", err)
	}

	select {
	case delay := <-delays:
		if delay != 10*time.Second {
			t.Fatalf("stall retry delay = %s, want 10s", delay)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for stall retry: %v", ctx.Err())
	}
	waitForCondition(t, 2*time.Second, func() bool {
		return len(o.Snapshot().Running) == 0
	})
	snapshot := o.Snapshot()
	if len(snapshot.Retrying) != 1 {
		t.Fatalf("retrying entries = %#v, want one", snapshot.Retrying)
	}
	retry := snapshot.Retrying[0]
	if retry.IssueID != issue.ID || retry.Attempt != 1 {
		t.Fatalf("retry entry = %#v", retry)
	}
	if !strings.Contains(retry.Error, "stalled after") {
		t.Fatalf("retry error = %q, want stalled after", retry.Error)
	}
	state := o.eligibilityState()
	if !state.claimed[issue.ID] {
		t.Fatalf("issue %s should remain claimed while retry is queued", issue.ID)
	}
}

func TestStartupCleanupRemovesTerminalWorkspaces(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "worktrees")
	terminalIssue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-DONE", Title: "done", State: "Done"}
	otherIssue := issuemodel.Issue{ID: "issue-2", Identifier: "ZEE-ACTIVE", Title: "active", State: "In Progress"}
	workspaceManager := workspace.New(root, runtimeconfig.HooksConfig{BeforeRemove: "printf cleanup", TimeoutMS: 5000})
	terminalPath, err := workspaceManager.PathForIssue(terminalIssue)
	if err != nil {
		t.Fatal(err)
	}
	otherPath, err := workspaceManager.PathForIssue(otherIssue)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(terminalPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherPath, 0o755); err != nil {
		t.Fatal(err)
	}
	tracker := &reconciliationTracker{terminalIssues: []issuemodel.Issue{terminalIssue}}
	logPath := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	logger, err := logging.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{TerminalStates: []string{"Done", "Canceled"}},
			},
		},
		Tracker:   tracker,
		Workspace: workspaceManager,
		Logger:    logger,
	})

	o.StartupCleanup(ctx)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(terminalPath); !os.IsNotExist(err) {
		t.Fatalf("terminal workspace err = %v, want removed", err)
	}
	if _, err := os.Stat(otherPath); err != nil {
		t.Fatalf("non-terminal workspace should remain: %v", err)
	}
	if got := tracker.lastFetchStates(); strings.Join(got, ",") != "Done,Canceled" {
		t.Fatalf("startup cleanup states = %v, want terminal states", got)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"event":"workspace_hook_started"`,
		`"hook":"before_remove"`,
		`"source":"startup_cleanup"`,
		`"issue_identifier":"ZEE-DONE"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("startup cleanup log missing %q in:\n%s", want, out)
		}
	}
}

func TestWorkflowReloadWhileWorkerRunningIsRaceFree(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	runner := &blockingRunner{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	reloader := &sequenceReloader{
		current: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker:   runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 50},
				Workspace: runtimeconfig.WorkspaceConfig{Root: newRoot},
				Agent:     runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "new {{ issue.identifier }}",
		},
		errs: []error{nil},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Polling: runtimeconfig.PollingConfig{IntervalMS: 50},
				Agent:   runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "old {{ issue.identifier }}",
		},
		Reloader:  reloader,
		Tracker:   &listTracker{issues: []issuemodel.Issue{issue}},
		Workspace: workspace.New(oldRoot, gitSeedHook()),
		Runner:    runner,
		WorkspaceFactory: func(cfg runtimeconfig.WorkspaceConfig, hooks runtimeconfig.HooksConfig) *workspace.Manager {
			return workspace.New(cfg.Root, hooks)
		},
	})

	if !o.dispatchIssue(ctx, issue, 0) {
		t.Fatal("dispatchIssue returned false, want dispatch")
	}
	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for worker start: %v", ctx.Err())
	}
	if failed := o.refreshWorkflow(); failed {
		t.Fatal("refreshWorkflow returned failure")
	}
	close(runner.release)
}

func TestRunAgentRendersRetryAttemptOnlyForRetryFirstPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "one", State: "In Progress"}
	template := "work on {{ issue.identifier }}{% if attempt %}\nretry {{ attempt }}{% endif %}"
	firstRunner := &promptCaptureRunner{}
	first := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config:         runtimeconfig.Config{Agent: runtimeconfig.AgentConfig{MaxTurns: 1}},
			PromptTemplate: template,
		},
		Tracker:   &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "first"), gitSeedHook()),
		Runner:    firstRunner,
	})
	if err := first.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("first runAgent returned error: %v", err)
	}
	if strings.Contains(firstRunner.prompt, "retry") {
		t.Fatalf("first prompt rendered retry attempt:\n%s", firstRunner.prompt)
	}

	retryRunner := &promptCaptureRunner{}
	retry := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config:         runtimeconfig.Config{Agent: runtimeconfig.AgentConfig{MaxTurns: 1}},
			PromptTemplate: template,
		},
		Tracker:   &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "retry"), gitSeedHook()),
		Runner:    retryRunner,
	})
	if err := retry.runAgent(ctx, issue, 2); err != nil {
		t.Fatalf("retry runAgent returned error: %v", err)
	}
	if !strings.Contains(retryRunner.prompt, "retry 2") {
		t.Fatalf("retry prompt missing attempt:\n%s", retryRunner.prompt)
	}
}

func TestRunAgentContinuesInSameSessionWithoutResendingOriginalPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-CONT", Title: "continue", State: "In Progress"}
	tracker := &sequenceIssueTracker{
		initial: issue,
		refreshed: []issuemodel.Issue{
			{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "In Progress"},
			{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Done"},
		},
	}
	runner := &continuationSessionRunner{}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 2},
			},
			PromptTemplate: "original workflow prompt for {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("RunSession calls = %d, want 1", runner.calls)
	}
	if len(runner.prompts) != 2 {
		t.Fatalf("prompts = %#v, want two turns", runner.prompts)
	}
	if !strings.Contains(runner.prompts[0], "original workflow prompt for ZEE-CONT") {
		t.Fatalf("first prompt = %q, want rendered workflow prompt", runner.prompts[0])
	}
	if runner.prompts[1] != continuationPromptText {
		t.Fatalf("continuation prompt = %q, want fixed continuation guidance", runner.prompts[1])
	}
	if strings.Contains(runner.prompts[1], "original workflow prompt") {
		t.Fatalf("continuation prompt resent original workflow prompt:\n%s", runner.prompts[1])
	}
	if tracker.fetchIssueCalls != 2 {
		t.Fatalf("FetchIssue calls = %d, want refresh after each turn", tracker.fetchIssueCalls)
	}
}

func TestRunAgentDoesNotMoveToHumanReviewAfterLocalCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-COMMIT",
		Title:      "commit smoke",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    commitRunner{},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if tracker.updatedState != "" {
		t.Fatalf("updated state = %q, want no automatic state update", tracker.updatedState)
	}
}

func TestRunAgentReviewPolicyAIDoesNotMoveThroughAIReviewAfterCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-AI-REVIEW",
		Title:      "ai review smoke",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                 "ai",
						OnAIFail:             "rework",
						ExpectedChangedFiles: []string{"README.md"},
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    commitRunner{},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if tracker.updatedState != "" {
		t.Fatalf("updated state = %q, want no automatic state update", tracker.updatedState)
	}
}

func TestRunAgentReviewPolicyHumanDoesNotMoveImplementationToHumanReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-MANUAL-AI",
		Title:      "manual ai review smoke",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                "human",
						AllowManualAIReview: true,
						OnAIFail:            "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    commitRunner{},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if tracker.updatedState != "" {
		t.Fatalf("updated state = %q, want no automatic state update", tracker.updatedState)
	}
}

func TestAIReviewStateDoesNotUseManualPolicyTransition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-MANUAL-AI",
		Title:      "manual ai review smoke",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &recordingRunner{}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                "human",
						AllowManualAIReview: true,
						OnAIFail:            "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.requests))
	}
	if len(tracker.states) != 0 {
		t.Fatalf("state updates = %#v, want none", tracker.states)
	}
}

func TestAIReviewStateRunsReviewerAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEWER",
		Title:      "same issue review smoke",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                "human",
						AllowManualAIReview: true,
						OnAIFail:            "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    stateChangingRunner{tracker: tracker, state: "Rework"},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if got, want := tracker.states, []string{"Rework"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestReviewerAgentContinuesIntoMergingInSameSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEWER-MERGE",
		Title:      "reviewer to merge smoke",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &reviewThenMergeRunner{tracker: tracker}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates: []string{"Todo", "In Progress", "Rework", "AI Review", "Merging"},
				},
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 2,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                "human",
						AllowManualAIReview: true,
						OnAIFail:            "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }} in {{ issue.state }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("reviewer runner calls = %d, want 1", runner.calls)
	}
	if got, want := len(runner.prompts), 2; got != want {
		t.Fatalf("prompts = %d, want %d", got, want)
	}
	if got, want := runner.prompts[0].Text, "work on ZEE-REVIEWER-MERGE in AI Review"; got != want {
		t.Fatalf("first prompt = %q, want %q", got, want)
	}
	if got := runner.prompts[1]; got.Text != mergingContinuationPromptText || !got.Continuation {
		t.Fatalf("merge continuation prompt = %#v, want continuation merge prompt", got)
	}
	if got := runner.prompts[1].Issue; got == nil || got.State != "Merging" {
		t.Fatalf("merge continuation issue = %#v, want Merging", got)
	}
	if got, want := tracker.states, []string{"Merging", "Done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestReviewerPassFinalAutoPromotesToMergingContinuation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEWER-PASS",
		Title:      "reviewer pass smoke",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &reviewPassThenMergeRunner{tracker: tracker}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates: []string{"Todo", "In Progress", "Rework", "AI Review", "Merging"},
				},
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 2,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:     "auto",
						OnAIFail: "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }} in {{ issue.state }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("reviewer runner calls = %d, want 1", runner.calls)
	}
	if got, want := len(runner.prompts), 2; got != want {
		t.Fatalf("prompts = %d, want %d", got, want)
	}
	if got := runner.prompts[1]; got.Text != mergingContinuationPromptText || !got.Continuation {
		t.Fatalf("merge continuation prompt = %#v, want continuation merge prompt", got)
	}
	if got := runner.prompts[1].Issue; got == nil || got.State != "Merging" {
		t.Fatalf("merge continuation issue = %#v, want Merging", got)
	}
	if got, want := tracker.states, []string{"Merging", "Done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestReviewerMergingContinuationRespectsMaxTurns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEWER-MERGE-STUCK",
		Title:      "reviewer merge stuck smoke",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &stuckMergingRunner{tracker: tracker, maxPrompts: 2}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates: []string{"Todo", "In Progress", "Rework", "AI Review", "Merging"},
				},
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 2,
				},
			},
			PromptTemplate: "work on {{ issue.identifier }} in {{ issue.state }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	err := o.runAgent(ctx, issue, 0)
	if err == nil || !strings.Contains(err.Error(), "reached max turns") {
		t.Fatalf("runAgent error = %v, want reached max turns", err)
	}
	if got, want := len(runner.prompts), 2; got != want {
		t.Fatalf("prompts = %d, want %d", got, want)
	}
	if containsStateUpdate(tracker.states, "Done") {
		t.Fatalf("state updates = %#v, want no Done without Merge: PASS", tracker.states)
	}
}

func TestMergingPassFinalAutoPromotesToDone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-MERGE-PASS",
		Title:      "merge pass smoke",
		State:      "Merging",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &mergeMessageRunner{message: "Merge: PASS\n\nPR: https://github.com/zeefan1555/symphony-go/pull/1\nmerge_commit: abc123\nroot_status: ## main...origin/main"}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"Merging"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "merge {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
	if got, want := tracker.states, []string{"Done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestMergingContinuationPromptKeepsDoneWithOrchestrator(t *testing.T) {
	for _, want := range []string{
		"Use the PR skill fast path",
		"update the workpad once with merge evidence",
		"Do not move Linear to Done from the agent",
		"final reply must start with Merge: PASS",
		"orchestrator can mark Done",
	} {
		if !strings.Contains(mergingContinuationPromptText, want) {
			t.Fatalf("merging continuation prompt missing %q:\n%s", want, mergingContinuationPromptText)
		}
	}
	for _, forbidden := range []string{
		"move the issue to Done",
		"move Linear to Done only after",
	} {
		if strings.Contains(mergingContinuationPromptText, forbidden) {
			t.Fatalf("merging continuation prompt still contains forbidden wording %q:\n%s", forbidden, mergingContinuationPromptText)
		}
	}
}

func TestMergingWithoutPassDoesNotAutoPromoteToDone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-MERGE-NO-PASS",
		Title:      "merge no pass smoke",
		State:      "Merging",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &mergeMessageRunner{message: "Merge still running"}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"Merging"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "merge {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	err := o.runAgent(ctx, issue, 0)
	if err == nil || !strings.Contains(err.Error(), "reached max turns") {
		t.Fatalf("runAgent error = %v, want reached max turns", err)
	}
	if containsStateUpdate(tracker.states, "Done") {
		t.Fatalf("state updates = %#v, want no Done without Merge: PASS", tracker.states)
	}
}

func TestRunAgentReviewPolicyAutoStopsWhenAgentMovesToAIReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:          "issue-id",
		Identifier:  "ZEE-AI-MERGE",
		Title:       "ai merge smoke",
		Description: "- 只改 `README.md`\n- 写入 smoke marker\n- 运行 `git diff --check`\n- 创建本地 commit\n- 不创建 PR\n",
		State:       "In Progress",
	}
	tracker := &recordingTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:                 "auto",
						OnAIFail:             "rework",
						ExpectedChangedFiles: []string{"README.md"},
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:     tracker,
		Workspace:   workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:      stateChangingRunner{tracker: tracker, state: "AI Review", commit: true},
		MergeTarget: "main",
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if got, want := tracker.states, []string{"AI Review"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestRunAgentDoesNotAutoPromoteAfterCommitWhenAgentMovesToAIReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-AGENT-REVIEW",
		Title:      "agent owned review transition",
		State:      "In Progress",
	}
	tracker := &recordingTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:     "auto",
						OnAIFail: "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    stateChangingRunner{tracker: tracker, state: "AI Review", commit: true},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if got, want := tracker.states, []string{"AI Review"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestRunAgentContinuesIntoAIReviewInSameIssueSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-SAME-AGENT",
		Title:      "same agent issue",
		State:      "In Progress",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &aiReviewSameSessionRunner{tracker: tracker}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{"In Progress", "AI Review"},
					TerminalStates: []string{"Done"},
				},
				Agent: runtimeconfig.AgentConfig{
					MaxTurns: 2,
					ReviewPolicy: runtimeconfig.ReviewPolicyConfig{
						Mode:     "auto",
						OnAIFail: "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want one session for the issue", runner.calls)
	}
	if got, want := len(runner.prompts), 2; got != want {
		t.Fatalf("prompts = %d, want %d", got, want)
	}
	if got := runner.prompts[1]; got.Text != aiReviewContinuationPromptText || !got.Continuation {
		t.Fatalf("AI Review continuation prompt = %#v, want same-session continuation", got)
	}
	if got := runner.prompts[1].Issue; got == nil || got.State != "AI Review" {
		t.Fatalf("AI Review continuation issue = %#v, want AI Review", got)
	}
	if got, want := tracker.states, []string{"AI Review", "Done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}

func TestMergingStateUsesWorkflowPrompt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-NO-MERGE-SKILL",
		Title:      "no merge skill",
		State:      "Merging",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &recordingRunner{}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config:         runtimeconfig.Config{},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.requests))
	}
	if prompt := runner.requests[0].Prompts[0].Text; !strings.Contains(prompt, "work on ZEE-NO-MERGE-SKILL") {
		t.Fatalf("runner prompt = %q, want rendered workflow prompt", prompt)
	}
	if len(tracker.states) != 0 {
		t.Fatalf("state updates = %#v, want none", tracker.states)
	}
}

func TestEffectiveMergeTargetUsesWorkflowUnlessOverridden(t *testing.T) {
	opts := Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Merge: runtimeconfig.MergeConfig{Target: "release"},
			},
		},
	}
	if got := effectiveMergeTarget(opts); got != "release" {
		t.Fatalf("merge target = %q, want release", got)
	}

	opts.MergeTarget = "main"
	if got := effectiveMergeTarget(opts); got != "main" {
		t.Fatalf("merge target override = %q, want main", got)
	}
}

func TestRunAgentDoesNotMoveToReviewWithoutCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-NOCOMMIT",
		Title:      "no commit smoke",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), gitSeedHook()),
		Runner:    noCommitRunner{},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if tracker.updatedState == "Human Review" {
		t.Fatal("issue should not move to Human Review without a local commit")
	}
}

func TestRunAgentRunsBeforeAndAfterHooksAroundRunner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-HOOK-ORDER",
		Title:      "hook order smoke",
		State:      "In Progress",
	}
	tracker := &snapshotTracker{issue: issue}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker: tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), runtimeconfig.HooksConfig{
			AfterCreate: gitSeedHook().AfterCreate,
			BeforeRun:   `printf before >> order.txt`,
			AfterRun:    `printf after >> order.txt; exit 9`,
			TimeoutMS:   5000,
		}),
		Runner: hookOrderRunner{},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
	workspacePath, err := o.opts.Workspace.PathForIssue(issue)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(workspacePath, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "beforerunnerafter" {
		t.Fatalf("hook order = %q", string(raw))
	}
}

func TestRunAgentLogsWorkspaceHookEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	logPath := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	logger, err := logging.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	issue := issuemodel.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-HOOK-LOG",
		Title:      "hook log smoke",
		State:      "In Progress",
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Agent: runtimeconfig.AgentConfig{MaxTurns: 1},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker: &snapshotTracker{issue: issue},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), runtimeconfig.HooksConfig{
			AfterCreate: gitSeedHook().AfterCreate + "\nprintf after-create",
			BeforeRun:   `printf before > order.txt`,
			AfterRun:    `printf after >> order.txt`,
			TimeoutMS:   5000,
		}),
		Runner: hookOrderRunner{},
		Logger: logger,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		`"event":"workspace_hook_started"`,
		`"event":"workspace_hook_completed"`,
		`"hook":"after_create"`,
		`"hook":"before_run"`,
		`"hook":"after_run"`,
		`printf before`,
		`after-create`,
		`"issue_id":"issue-id"`,
		`"issue_identifier":"ZEE-HOOK-LOG"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("hook log missing %q in:\n%s", want, out)
		}
	}
}

func TestPollKeepsReloadErrorVisibleAfterTrackerSuccess(t *testing.T) {
	ctx := context.Background()
	reloader := &sequenceReloader{
		current: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Polling: runtimeconfig.PollingConfig{IntervalMS: 2000},
			},
			PromptTemplate: "updated",
		},
		errs: []error{errors.New("invalid workflow"), nil},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{ActiveStates: []string{"In Progress"}},
				Polling: runtimeconfig.PollingConfig{IntervalMS: 1000},
			},
			PromptTemplate: "original",
		},
		Reloader: reloader,
		Tracker:  emptyTracker{},
	})

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error: %v", err)
	}
	if got := o.Snapshot().LastError; got != "invalid workflow" {
		t.Fatalf("last error = %q, want reload error", got)
	}

	if err := o.poll(ctx); err != nil {
		t.Fatalf("poll returned error after valid reload: %v", err)
	}
	snapshot := o.Snapshot()
	if snapshot.LastError != "" {
		t.Fatalf("last error after valid reload = %q, want empty", snapshot.LastError)
	}
	if snapshot.Polling.IntervalMS != 2000 {
		t.Fatalf("polling interval = %d, want 2000", snapshot.Polling.IntervalMS)
	}
}

func TestRepoRootFromWorkflowUsesWorkflowDirectory(t *testing.T) {
	workflowPath := filepath.Join(t.TempDir(), "repo", "WORKFLOW.md")

	if got, want := RepoRootFromWorkflow(workflowPath), filepath.Dir(workflowPath); got != want {
		t.Fatalf("repo root = %q, want %q", got, want)
	}
}

func TestRefreshWorkflowRebuildsDependenciesFromFactories(t *testing.T) {
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	reloader := &sequenceReloader{
		current: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker:   runtimeconfig.TrackerConfig{ProjectSlug: "new-project"},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 2000},
				Workspace: runtimeconfig.WorkspaceConfig{Root: newRoot},
				Hooks:     runtimeconfig.HooksConfig{AfterCreate: "new-hook"},
				Codex:     runtimeconfig.CodexConfig{Command: "new-codex"},
			},
			PromptTemplate: "new prompt",
		},
		errs: []error{nil},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker:   runtimeconfig.TrackerConfig{ProjectSlug: "old-project"},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 1000},
				Workspace: runtimeconfig.WorkspaceConfig{Root: oldRoot},
				Hooks:     runtimeconfig.HooksConfig{AfterCreate: "old-hook"},
				Codex:     runtimeconfig.CodexConfig{Command: "old-codex"},
			},
			PromptTemplate: "old prompt",
		},
		Reloader:  reloader,
		Tracker:   configTracker{projectSlug: "old-project"},
		Workspace: workspace.New(oldRoot, runtimeconfig.HooksConfig{AfterCreate: "old-hook"}),
		Runner:    configRunner{command: "old-codex"},
		TrackerFactory: func(cfg runtimeconfig.TrackerConfig) (Tracker, error) {
			return configTracker{projectSlug: cfg.ProjectSlug}, nil
		},
		WorkspaceFactory: func(cfg runtimeconfig.WorkspaceConfig, hooks runtimeconfig.HooksConfig) *workspace.Manager {
			return workspace.New(cfg.Root, hooks)
		},
		RunnerFactory: func(cfg runtimeconfig.CodexConfig) AgentRunner {
			return configRunner{command: cfg.Command}
		},
	})

	if failed := o.refreshWorkflow(); failed {
		t.Fatal("refreshWorkflow returned failure")
	}
	if o.opts.Workflow.PromptTemplate != "new prompt" {
		t.Fatalf("workflow prompt = %q, want new prompt", o.opts.Workflow.PromptTemplate)
	}
	if tracker := o.opts.Tracker.(configTracker); tracker.projectSlug != "new-project" {
		t.Fatalf("tracker project = %q, want new-project", tracker.projectSlug)
	}
	if o.opts.Workspace.Root != newRoot {
		t.Fatalf("workspace root = %q, want %q", o.opts.Workspace.Root, newRoot)
	}
	if o.opts.Workspace.Hooks.AfterCreate != "new-hook" {
		t.Fatalf("workspace hook = %q, want new-hook", o.opts.Workspace.Hooks.AfterCreate)
	}
	if runner := o.opts.Runner.(configRunner); runner.command != "new-codex" {
		t.Fatalf("runner command = %q, want new-codex", runner.command)
	}
	if interval := o.Snapshot().Polling.IntervalMS; interval != 2000 {
		t.Fatalf("snapshot interval = %d, want 2000", interval)
	}
}

func TestRefreshWorkflowFactoryFailureDoesNotHalfApply(t *testing.T) {
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	oldTracker := configTracker{projectSlug: "old-project"}
	oldRunner := configRunner{command: "old-codex"}
	reloader := &sequenceReloader{
		current: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker:   runtimeconfig.TrackerConfig{ProjectSlug: "new-project"},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 2000},
				Workspace: runtimeconfig.WorkspaceConfig{Root: newRoot},
				Codex:     runtimeconfig.CodexConfig{Command: "new-codex"},
			},
			PromptTemplate: "new prompt",
		},
		errs: []error{nil},
	}
	o := New(Options{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker:   runtimeconfig.TrackerConfig{ProjectSlug: "old-project"},
				Polling:   runtimeconfig.PollingConfig{IntervalMS: 1000},
				Workspace: runtimeconfig.WorkspaceConfig{Root: oldRoot},
				Codex:     runtimeconfig.CodexConfig{Command: "old-codex"},
			},
			PromptTemplate: "old prompt",
		},
		Reloader:  reloader,
		Tracker:   oldTracker,
		Workspace: workspace.New(oldRoot, runtimeconfig.HooksConfig{}),
		Runner:    oldRunner,
		TrackerFactory: func(runtimeconfig.TrackerConfig) (Tracker, error) {
			return nil, errors.New("tracker factory failed")
		},
		WorkspaceFactory: func(runtimeconfig.WorkspaceConfig, runtimeconfig.HooksConfig) *workspace.Manager {
			t.Fatal("workspace factory must not run after tracker failure")
			return nil
		},
		RunnerFactory: func(runtimeconfig.CodexConfig) AgentRunner {
			t.Fatal("runner factory must not run after tracker failure")
			return nil
		},
	})

	if failed := o.refreshWorkflow(); !failed {
		t.Fatal("refreshWorkflow returned success, want failure")
	}
	if o.opts.Workflow.PromptTemplate != "old prompt" {
		t.Fatalf("workflow prompt = %q, want old prompt", o.opts.Workflow.PromptTemplate)
	}
	if o.opts.Tracker != oldTracker {
		t.Fatalf("tracker was replaced: %#v", o.opts.Tracker)
	}
	if o.opts.Workspace.Root != oldRoot {
		t.Fatalf("workspace root = %q, want %q", o.opts.Workspace.Root, oldRoot)
	}
	if o.opts.Runner != oldRunner {
		t.Fatalf("runner was replaced: %#v", o.opts.Runner)
	}
	if interval := o.Snapshot().Polling.IntervalMS; interval != 1000 {
		t.Fatalf("snapshot interval = %d, want 1000", interval)
	}
	if got := o.Snapshot().LastError; got != "tracker factory failed" {
		t.Fatalf("last error = %q, want tracker factory failed", got)
	}
}

func TestRefreshWorkflowRetriesFactoryFailureWithoutFileTouch(t *testing.T) {
	oldRoot := filepath.Join(t.TempDir(), "old")
	newRoot := filepath.Join(t.TempDir(), "new")
	workflowPath := filepath.Join(t.TempDir(), "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	writeOrchestratorWorkflow(t, workflowPath, "old-project", oldRoot, "old-codex", "1000", "old prompt")
	reloader, err := workflow.NewReloader(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	writeOrchestratorWorkflow(t, workflowPath, "new-project", newRoot, "new-codex", "2000", "new prompt")

	loaded := reloader.Current()
	if loaded.PromptTemplate != "old prompt" {
		t.Fatalf("initial prompt = %q, want old prompt", loaded.PromptTemplate)
	}
	factoryErrs := []error{errors.New("tracker factory failed"), nil}
	o := New(Options{
		Workflow:  loaded,
		Reloader:  reloader,
		Tracker:   configTracker{projectSlug: "old-project"},
		Workspace: workspace.New(oldRoot, runtimeconfig.HooksConfig{}),
		Runner:    configRunner{command: "old-codex"},
		TrackerFactory: func(cfg runtimeconfig.TrackerConfig) (Tracker, error) {
			err := factoryErrs[0]
			factoryErrs = factoryErrs[1:]
			if err != nil {
				return nil, err
			}
			return configTracker{projectSlug: cfg.ProjectSlug}, nil
		},
		WorkspaceFactory: func(cfg runtimeconfig.WorkspaceConfig, hooks runtimeconfig.HooksConfig) *workspace.Manager {
			return workspace.New(cfg.Root, hooks)
		},
		RunnerFactory: func(cfg runtimeconfig.CodexConfig) AgentRunner {
			return configRunner{command: cfg.Command}
		},
	})

	if failed := o.refreshWorkflow(); !failed {
		t.Fatal("first refresh returned success, want factory failure")
	}
	if o.opts.Workflow.PromptTemplate != "old prompt" {
		t.Fatalf("workflow prompt after failure = %q, want old prompt", o.opts.Workflow.PromptTemplate)
	}

	if failed := o.refreshWorkflow(); failed {
		t.Fatal("second refresh returned failure, want retry success")
	}
	if o.opts.Workflow.PromptTemplate != "new prompt" {
		t.Fatalf("workflow prompt after retry = %q, want new prompt", o.opts.Workflow.PromptTemplate)
	}
	if tracker := o.opts.Tracker.(configTracker); tracker.projectSlug != "new-project" {
		t.Fatalf("tracker project = %q, want new-project", tracker.projectSlug)
	}
	if o.opts.Workspace.Root != newRoot {
		t.Fatalf("workspace root = %q, want %q", o.opts.Workspace.Root, newRoot)
	}
	if runner := o.opts.Runner.(configRunner); runner.command != "new-codex" {
		t.Fatalf("runner command = %q, want new-codex", runner.command)
	}
}

func writeOrchestratorWorkflow(t *testing.T, path, project, root, command, interval, prompt string) {
	t.Helper()
	content := `---
tracker:
  kind: linear
  project_slug: ` + project + `
polling:
  interval_ms: ` + interval + `
workspace:
  root: ` + root + `
codex:
  command: ` + command + `
---
` + prompt + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type snapshotTracker struct {
	issue        issuemodel.Issue
	updatedState string
}

func (t *snapshotTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return []issuemodel.Issue{t.issue}, nil
}

func (t *snapshotTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return []issuemodel.Issue{t.issue}, nil
}

func (t *snapshotTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	issue := t.issue
	issue.State = "Done"
	return issue, nil
}

func (t *snapshotTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	issue := t.issue
	issue.State = "Done"
	return []issuemodel.Issue{issue}, nil
}

func (t *snapshotTracker) UpdateIssueState(_ context.Context, _ string, state string) error {
	t.updatedState = state
	return nil
}

type recordingTracker struct {
	issue  issuemodel.Issue
	states []string
}

func (t *recordingTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return []issuemodel.Issue{t.issue}, nil
}

func (t *recordingTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (t *recordingTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	return t.issue, nil
}

func (t *recordingTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	return []issuemodel.Issue{t.issue}, nil
}

func (t *recordingTracker) UpdateIssueState(_ context.Context, _ string, state string) error {
	t.states = append(t.states, state)
	t.issue.State = state
	return nil
}

type sequenceIssueTracker struct {
	initial         issuemodel.Issue
	refreshed       []issuemodel.Issue
	fetchIssueCalls int
}

func (t *sequenceIssueTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return []issuemodel.Issue{t.initial}, nil
}

func (t *sequenceIssueTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (t *sequenceIssueTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	t.fetchIssueCalls++
	if len(t.refreshed) == 0 {
		return t.initial, nil
	}
	issue := t.refreshed[0]
	t.refreshed = t.refreshed[1:]
	return issue, nil
}

func (t *sequenceIssueTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (t *sequenceIssueTracker) UpdateIssueState(context.Context, string, string) error {
	return nil
}

type emptyTracker struct{}

func (emptyTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (emptyTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (emptyTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	return issuemodel.Issue{}, nil
}

func (emptyTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (emptyTracker) UpdateIssueState(context.Context, string, string) error {
	return nil
}

type configTracker struct {
	projectSlug string
}

func (configTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (configTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (configTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	return issuemodel.Issue{}, nil
}

func (configTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (configTracker) UpdateIssueState(context.Context, string, string) error {
	return nil
}

type configRunner struct {
	command string
}

func (configRunner) RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error) {
	return codex.SessionResult{}, nil
}

type sequenceReloader struct {
	current   *runtimeconfig.Workflow
	errs      []error
	committed int
}

func (r *sequenceReloader) Current() *runtimeconfig.Workflow {
	return r.current
}

func (r *sequenceReloader) ReloadIfChanged() (*runtimeconfig.Workflow, bool, error) {
	if len(r.errs) == 0 {
		return nil, false, nil
	}
	err := r.errs[0]
	r.errs = r.errs[1:]
	if err != nil {
		return nil, false, err
	}
	return r.current, true, nil
}

func (r *sequenceReloader) CommitCandidate() {
	r.committed++
}

type snapshotRunner struct {
	eventEmitted chan struct{}
	release      chan struct{}
}

type listTracker struct {
	mu             sync.Mutex
	issues         []issuemodel.Issue
	fetchActiveErr error
	updatedState   string
}

func (t *listTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fetchActiveErr != nil {
		return nil, t.fetchActiveErr
	}
	return append([]issuemodel.Issue(nil), t.issues...), nil
}

func (t *listTracker) FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error) {
	return t.FetchActiveIssues(context.Background(), nil)
}

func (t *listTracker) FetchIssue(_ context.Context, issueID string) (issuemodel.Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, issue := range t.issues {
		if issue.ID == issueID {
			return issue, nil
		}
	}
	return issuemodel.Issue{}, nil
}

func (t *listTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	return nil, nil
}

func (t *listTracker) UpdateIssueState(_ context.Context, issueID string, state string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.updatedState = state
	for i := range t.issues {
		if t.issues[i].ID == issueID {
			t.issues[i].State = state
		}
	}
	return nil
}

type reconciliationTracker struct {
	mu             sync.Mutex
	activeIssues   []issuemodel.Issue
	stateIssues    []issuemodel.Issue
	terminalIssues []issuemodel.Issue
	callLog        []string
	states         []string
}

func (t *reconciliationTracker) FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callLog = append(t.callLog, "FetchActiveIssues")
	return append([]issuemodel.Issue(nil), t.activeIssues...), nil
}

func (t *reconciliationTracker) FetchIssuesByStates(_ context.Context, states []string) ([]issuemodel.Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callLog = append(t.callLog, "FetchIssuesByStates")
	t.states = append([]string(nil), states...)
	return append([]issuemodel.Issue(nil), t.terminalIssues...), nil
}

func (t *reconciliationTracker) FetchIssue(context.Context, string) (issuemodel.Issue, error) {
	return issuemodel.Issue{}, nil
}

func (t *reconciliationTracker) FetchIssueStatesByIDs(context.Context, []string) ([]issuemodel.Issue, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.callLog = append(t.callLog, "FetchIssueStatesByIDs")
	return append([]issuemodel.Issue(nil), t.stateIssues...), nil
}

func (t *reconciliationTracker) UpdateIssueState(context.Context, string, string) error {
	return nil
}

func (t *reconciliationTracker) calls() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.callLog...)
}

func (t *reconciliationTracker) lastFetchStates() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.states...)
}

type blockingRunner struct {
	started chan string
	release chan struct{}
}

func (r *blockingRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.started <- request.Issue.ID
	select {
	case <-r.release:
		return completeFakeSession(ctx, request, nil)
	case <-ctx.Done():
		return codex.SessionResult{}, ctx.Err()
	}
}

type terminalBlockingRunner struct {
	started        chan string
	cancelObserved chan struct{}
	release        chan struct{}
}

func (r *terminalBlockingRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.started <- request.Issue.ID
	<-ctx.Done()
	close(r.cancelObserved)
	<-r.release
	return codex.SessionResult{}, ctx.Err()
}

type promptCaptureRunner struct {
	prompt string
}

func (r *promptCaptureRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	if len(request.Prompts) > 0 {
		r.prompt = request.Prompts[0].Text
	}
	return completeFakeSession(ctx, request, nil)
}

type continuationSessionRunner struct {
	calls   int
	prompts []string
}

func (r *continuationSessionRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.calls++
	return completeFakeSession(ctx, request, func(prompt codex.TurnPrompt) {
		r.prompts = append(r.prompts, prompt.Text)
	})
}

func completeFakeSession(ctx context.Context, request codex.SessionRequest, observe func(codex.TurnPrompt)) (codex.SessionResult, error) {
	result := codex.SessionResult{ThreadID: "thread-1", PID: 123}
	for turn := 0; turn < len(request.Prompts); turn++ {
		prompt := request.Prompts[turn]
		if observe != nil {
			observe(prompt)
		}
		turnResult := codex.Result{
			SessionID: fmt.Sprintf("thread-1-turn-%d", turn+1),
			ThreadID:  "thread-1",
			TurnID:    fmt.Sprintf("turn-%d", turn+1),
			PID:       123,
		}
		result.Turns = append(result.Turns, turnResult)
		result.SessionID = turnResult.SessionID
		result.PID = turnResult.PID
		if request.AfterTurn == nil {
			continue
		}
		next, ok, err := request.AfterTurn(ctx, turnResult, turn+1)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}
		request.Prompts = append(request.Prompts, next)
	}
	return result, nil
}

func captureRetryTimers() (func(time.Duration, func()) *time.Timer, <-chan time.Duration) {
	delays := make(chan time.Duration, 4)
	return func(delay time.Duration, fn func()) *time.Timer {
		delays <- delay
		return time.NewTimer(time.Hour)
	}, delays
}

func waitForCondition(t *testing.T, timeout time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ok() {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for condition")
		case <-ticker.C:
		}
	}
}

type countingRunner struct {
	started chan string
}

func (r *countingRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.started <- request.Issue.ID
	return completeFakeSession(ctx, request, nil)
}

type sequenceRunner struct {
	runners []AgentRunner
	next    int
}

func (r *sequenceRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	if r.next >= len(r.runners) {
		return codex.SessionResult{}, fmt.Errorf("unexpected runner call %d", r.next+1)
	}
	next := r.runners[r.next]
	r.next++
	return next.RunSession(ctx, request, onEvent)
}

type recordingRunner struct {
	result   codex.SessionResult
	err      error
	requests []codex.SessionRequest
}

func (r *recordingRunner) RunSession(_ context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.requests = append(r.requests, request)
	if r.result.SessionID == "" {
		r.result.SessionID = "recording-session"
	}
	return r.result, r.err
}

type stateChangingRunner struct {
	tracker *recordingTracker
	state   string
	commit  bool
}

func (r stateChangingRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	if r.commit && request.Issue.State == "In Progress" {
		if err := commitFile(ctx, request.WorkspacePath, "README.md", "agent owned transition\n", "agent commit"); err != nil {
			return codex.SessionResult{}, err
		}
	}
	if r.state != "" {
		if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, r.state); err != nil {
			return codex.SessionResult{}, err
		}
	}
	return completeFakeSession(ctx, request, nil)
}

type aiReviewSameSessionRunner struct {
	tracker *recordingTracker
	calls   int
	prompts []codex.TurnPrompt
}

func (r *aiReviewSameSessionRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.calls++
	return completeFakeSession(ctx, request, func(prompt codex.TurnPrompt) {
		r.prompts = append(r.prompts, prompt)
		switch len(r.prompts) {
		case 1:
			_ = r.tracker.UpdateIssueState(ctx, request.Issue.ID, "AI Review")
		case 2:
			_ = r.tracker.UpdateIssueState(ctx, request.Issue.ID, "Done")
		}
	})
}

type observingStateChangingRunner struct {
	tracker *recordingTracker
	state   string
	calls   int
	prompts []string
}

func (r *observingStateChangingRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	r.calls++
	if r.state != "" {
		if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, r.state); err != nil {
			return codex.SessionResult{}, err
		}
	}
	return completeFakeSession(ctx, request, func(prompt codex.TurnPrompt) {
		r.prompts = append(r.prompts, prompt.Text)
	})
}

type reviewThenMergeRunner struct {
	tracker *recordingTracker
	calls   int
	prompts []codex.TurnPrompt
}

func (r *reviewThenMergeRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	r.calls++
	result := codex.SessionResult{ThreadID: "thread-1", PID: 123}
	for turn := 0; turn < len(request.Prompts); turn++ {
		r.prompts = append(r.prompts, request.Prompts[turn])
		switch turn {
		case 0:
			if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, "Merging"); err != nil {
				return result, err
			}
		case 1:
			emitAgentMessage(onEvent, "Merge: PASS\n\nPR: https://github.com/zeefan1555/symphony-go/pull/1\nmerge_commit: abc123\nroot_status: ## main...origin/main")
		}
		turnResult := codex.Result{
			SessionID: fmt.Sprintf("thread-1-turn-%d", turn+1),
			ThreadID:  "thread-1",
			TurnID:    fmt.Sprintf("turn-%d", turn+1),
			PID:       123,
		}
		result.Turns = append(result.Turns, turnResult)
		result.SessionID = turnResult.SessionID
		result.PID = turnResult.PID
		if request.AfterTurn == nil {
			continue
		}
		next, ok, err := request.AfterTurn(ctx, turnResult, turn+1)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}
		request.Prompts = append(request.Prompts, next)
	}
	return result, nil
}

func emitAgentMessage(onEvent func(codex.Event), text string) {
	if onEvent == nil {
		return
	}
	onEvent(codex.Event{
		Name: "item/completed",
		Payload: map[string]any{
			"method": "item/completed",
			"params": map[string]any{
				"item": map[string]any{
					"type": "agentMessage",
					"text": text,
				},
			},
		},
	})
}

func containsStateUpdate(states []string, want string) bool {
	for _, state := range states {
		if state == want {
			return true
		}
	}
	return false
}

type mergeMessageRunner struct {
	message string
}

func (r *mergeMessageRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	return completeFakeSession(ctx, request, func(codex.TurnPrompt) {
		emitAgentMessage(onEvent, r.message)
	})
}

type reviewPassThenMergeRunner struct {
	tracker *recordingTracker
	calls   int
	prompts []codex.TurnPrompt
}

func (r *reviewPassThenMergeRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	r.calls++
	result := codex.SessionResult{ThreadID: "thread-1", PID: 123}
	for turn := 0; turn < len(request.Prompts); turn++ {
		r.prompts = append(r.prompts, request.Prompts[turn])
		if turn == 0 {
			emitAgentMessage(onEvent, "结论: PASS\n\nFindings:\n- 无阻塞发现。")
		}
		if turn == 1 {
			emitAgentMessage(onEvent, "Merge: PASS\n\nPR: https://github.com/zeefan1555/symphony-go/pull/1\nmerge_commit: abc123\nroot_status: ## main...origin/main")
		}
		turnResult := codex.Result{
			SessionID: fmt.Sprintf("thread-1-turn-%d", turn+1),
			ThreadID:  "thread-1",
			TurnID:    fmt.Sprintf("turn-%d", turn+1),
			PID:       123,
		}
		result.Turns = append(result.Turns, turnResult)
		result.SessionID = turnResult.SessionID
		result.PID = turnResult.PID
		if request.AfterTurn == nil {
			continue
		}
		next, ok, err := request.AfterTurn(ctx, turnResult, turn+1)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}
		request.Prompts = append(request.Prompts, next)
	}
	return result, nil
}

type stuckMergingRunner struct {
	tracker    *recordingTracker
	maxPrompts int
	prompts    []codex.TurnPrompt
}

func (r *stuckMergingRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	result := codex.SessionResult{ThreadID: "thread-1", PID: 123}
	for turn := 0; turn < len(request.Prompts); turn++ {
		if len(r.prompts) >= r.maxPrompts {
			return result, fmt.Errorf("runner observed prompt %d, want at most %d", len(r.prompts)+1, r.maxPrompts)
		}
		r.prompts = append(r.prompts, request.Prompts[turn])
		if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, "Merging"); err != nil {
			return result, err
		}
		turnResult := codex.Result{
			SessionID: fmt.Sprintf("thread-1-turn-%d", turn+1),
			ThreadID:  "thread-1",
			TurnID:    fmt.Sprintf("turn-%d", turn+1),
			PID:       123,
		}
		result.Turns = append(result.Turns, turnResult)
		result.SessionID = turnResult.SessionID
		result.PID = turnResult.PID
		next, ok, err := request.AfterTurn(ctx, turnResult, turn+1)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}
		request.Prompts = append(request.Prompts, next)
	}
	return result, nil
}

func commitFile(ctx context.Context, workspacePath, name, contents, message string) error {
	if err := os.WriteFile(filepath.Join(workspacePath, name), []byte(contents), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"add", name}, {"commit", "-m", message}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workspacePath
		if output, err := cmd.CombinedOutput(); err != nil {
			return errFromOutput(args, err, string(output))
		}
	}
	return nil
}

type commitRunner struct{}

func (commitRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	if err := os.WriteFile(filepath.Join(request.WorkspacePath, "README.md"), []byte("hello zeefan\n"), 0o644); err != nil {
		return codex.SessionResult{}, err
	}
	for _, args := range [][]string{{"add", "README.md"}, {"commit", "-m", request.Issue.Identifier + ": smoke"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = request.WorkspacePath
		if output, err := cmd.CombinedOutput(); err != nil {
			return codex.SessionResult{}, errFromOutput(args, err, string(output))
		}
	}
	return completeFakeSession(ctx, request, nil)
}

type noCommitRunner struct{}

func (noCommitRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	return completeFakeSession(ctx, request, nil)
}

type hookOrderRunner struct{}

func (hookOrderRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	raw, err := os.ReadFile(filepath.Join(request.WorkspacePath, "order.txt"))
	if err != nil {
		return codex.SessionResult{}, err
	}
	if string(raw) != "before" {
		return codex.SessionResult{}, fmt.Errorf("runner saw order %q, want before", string(raw))
	}
	f, err := os.OpenFile(filepath.Join(request.WorkspacePath, "order.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return codex.SessionResult{}, err
	}
	defer f.Close()
	if _, err := f.WriteString("runner"); err != nil {
		return codex.SessionResult{}, err
	}
	return completeFakeSession(ctx, request, nil)
}

func errFromOutput(args []string, err error, output string) error {
	return &gitTestError{args: args, err: err, output: output}
}

type gitTestError struct {
	args   []string
	err    error
	output string
}

func (e *gitTestError) Error() string {
	return "git " + strings.Join(e.args, " ") + ": " + e.err.Error() + ": " + e.output
}

func gitSeedHook() runtimeconfig.HooksConfig {
	return runtimeconfig.HooksConfig{AfterCreate: `
git init
git config user.email symphony-go@example.invalid
git config user.name "Symphony Go Test"
printf 'seed\n' > .seed
git add .seed
git commit -m seed
`}
}

func gitWorktreeHook(repoRoot string) runtimeconfig.HooksConfig {
	return runtimeconfig.HooksConfig{AfterCreate: fmt.Sprintf(`
workspace="$(pwd -P)"
cd %s
rm -rf "$workspace"
git worktree add -b "symphony-go/$(basename "$workspace")" "$workspace" HEAD
`, shellQuote(repoRoot))}
}

func initGitRepo(t *testing.T, repoRoot string) string {
	t.Helper()
	commands := [][]string{
		{"init"},
		{"config", "user.email", "symphony-go@example.invalid"},
		{"config", "user.name", "Symphony Go Test"},
	}
	for _, args := range commands {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "README.md"}, {"commit", "-m", "seed"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v: %s", args, err, output)
		}
	}
	branch := gitOutputForTest(t, repoRoot, "branch", "--show-current")
	if branch == "" {
		t.Fatal("git branch --show-current returned empty branch")
	}
	return branch
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (r *snapshotRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	onEvent(codex.Event{
		Name: "rate_limits",
		Payload: map[string]any{
			"method": "thread/rateLimits/updated",
			"params": map[string]any{"remaining": 3.0},
		},
	})
	onEvent(codex.Event{
		Name: "token_usage",
		Payload: map[string]any{
			"method": "thread/tokenUsage/updated",
			"params": map[string]any{
				"input_tokens":  10.0,
				"output_tokens": 4.0,
				"total_tokens":  14.0,
			},
		},
	})
	close(r.eventEmitted)

	select {
	case <-r.release:
		return completeFakeSession(ctx, request, nil)
	case <-ctx.Done():
		return codex.SessionResult{}, ctx.Err()
	}
}
