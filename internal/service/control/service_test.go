package control_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/service/control"
	"github.com/zeefan1555/symphony-go/internal/types"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/workspace"
)

type fakeSnapshotProvider struct {
	snapshot observability.Snapshot
}

type fakeRefreshProvider struct {
	snapshot observability.Snapshot
	results  []bool
	calls    int
}

func TestServiceReadsIssueDetailFromSnapshotProvider(t *testing.T) {
	generatedAt := time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC)
	dueAt := generatedAt.Add(30 * time.Second)
	provider := fakeSnapshotProvider{snapshot: observability.Snapshot{
		Running: []observability.RunningEntry{{
			IssueID:         "running-id",
			IssueIdentifier: "ZEE-48",
			State:           "In Progress",
			WorkspacePath:   "/tmp/ZEE-48",
			SessionID:       "session-id",
			TurnCount:       2,
			StartedAt:       generatedAt,
			Tokens:          observability.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
			RuntimeSeconds:  120,
		}},
		Retrying: []observability.RetryEntry{{
			IssueID:         "retry-id",
			IssueIdentifier: "ZEE-49",
			Attempt:         3,
			DueAt:           dueAt,
			Error:           "rate limited",
			WorkspacePath:   "/tmp/ZEE-49",
		}},
	}}

	service := control.NewService(provider)
	running, err := service.IssueDetail(context.Background(), "ZEE-48")
	if err != nil {
		t.Fatalf("IssueDetail running returned error: %v", err)
	}
	if running.Status != control.IssueStatusRunning || running.IssueID != "running-id" || running.Running == nil {
		t.Fatalf("running detail = %#v, want running projection", running)
	}
	if running.Running.SessionID != "session-id" || running.Running.Tokens.TotalTokens != 15 {
		t.Fatalf("running fields = %#v, want session and token projection", running.Running)
	}

	retrying, err := service.IssueDetail(context.Background(), "ZEE-49")
	if err != nil {
		t.Fatalf("IssueDetail retrying returned error: %v", err)
	}
	if retrying.Status != control.IssueStatusRetrying || retrying.IssueID != "retry-id" || retrying.Retry == nil {
		t.Fatalf("retrying detail = %#v, want retry projection", retrying)
	}
	if retrying.Retry.Attempt != 3 || retrying.Retry.Error != "rate limited" {
		t.Fatalf("retry fields = %#v, want attempt and error projection", retrying.Retry)
	}

	_, err = service.IssueDetail(context.Background(), "ZEE-404")
	if !errors.Is(err, control.ErrIssueNotFound) {
		t.Fatalf("IssueDetail unknown error = %v, want ErrIssueNotFound", err)
	}
	_, err = service.IssueDetail(context.Background(), "")
	if !errors.Is(err, control.ErrInvalidIssueIdentifier) {
		t.Fatalf("IssueDetail empty identifier error = %v, want ErrInvalidIssueIdentifier", err)
	}
	_, err = service.IssueDetail(context.Background(), "  ")
	if !errors.Is(err, control.ErrInvalidIssueIdentifier) {
		t.Fatalf("IssueDetail blank identifier error = %v, want ErrInvalidIssueIdentifier", err)
	}
}

func (p fakeSnapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func (p *fakeRefreshProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}

func (p *fakeRefreshProvider) RequestRefresh(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.calls++
	if len(p.results) == 0 {
		return false, nil
	}
	result := p.results[0]
	p.results = p.results[1:]
	return result, nil
}

func TestServiceRefreshRequestsProviderPoll(t *testing.T) {
	provider := &fakeRefreshProvider{results: []bool{true, false}}
	service := control.NewService(provider)

	first, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh first returned error: %v", err)
	}
	if !first.Accepted || first.Status != control.RefreshStatusQueued {
		t.Fatalf("first refresh = %#v, want queued accepted result", first)
	}

	second, err := service.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh second returned error: %v", err)
	}
	if !second.Accepted || second.Status != control.RefreshStatusAlreadyPending {
		t.Fatalf("second refresh = %#v, want already pending accepted result", second)
	}
	if provider.calls != 2 {
		t.Fatalf("refresh calls = %d, want 2", provider.calls)
	}
}

func TestServiceRefreshRequiresTrigger(t *testing.T) {
	service := control.NewService(fakeSnapshotProvider{snapshot: observability.NewSnapshot()})

	_, err := service.Refresh(context.Background())
	if !errors.Is(err, control.ErrRefreshTriggerRequired) {
		t.Fatalf("Refresh error = %v, want ErrRefreshTriggerRequired", err)
	}
}

func TestServiceProjectsIssueRunState(t *testing.T) {
	provider := fakeSnapshotProvider{snapshot: observability.Snapshot{
		Running: []observability.RunningEntry{{
			IssueIdentifier: "ZEE-56",
		}},
		Retrying: []observability.RetryEntry{{
			IssueIdentifier: "ZEE-57",
		}},
	}}
	service := control.NewService(provider)

	running, err := service.ProjectIssueRun(context.Background(), "ZEE-56")
	if err != nil {
		t.Fatalf("ProjectIssueRun running returned error: %v", err)
	}
	if running.RuntimeState != control.IssueStatusRunning || running.IssueIdentifier != "ZEE-56" {
		t.Fatalf("running projection = %#v, want running ZEE-56", running)
	}
	if running.Boundary.Name != "orchestrator.issue_run_projection" {
		t.Fatalf("boundary = %#v, want orchestrator issue-run boundary", running.Boundary)
	}

	retrying, err := service.ProjectIssueRun(context.Background(), "ZEE-57")
	if err != nil {
		t.Fatalf("ProjectIssueRun retrying returned error: %v", err)
	}
	if retrying.RuntimeState != control.IssueStatusRetrying {
		t.Fatalf("retrying projection = %#v, want retrying", retrying)
	}

	missing, err := service.ProjectIssueRun(context.Background(), "ZEE-404")
	if err != nil {
		t.Fatalf("ProjectIssueRun missing returned error: %v", err)
	}
	if missing.RuntimeState != control.IssueStatusNotRunning {
		t.Fatalf("missing projection = %#v, want not_running", missing)
	}

	_, err = service.ProjectIssueRun(context.Background(), "")
	if !errors.Is(err, control.ErrInvalidIssueIdentifier) {
		t.Fatalf("ProjectIssueRun empty error = %v, want ErrInvalidIssueIdentifier", err)
	}
}

func TestServiceProjectsWorkspaceLifecycle(t *testing.T) {
	root := t.TempDir()
	manager := coreworkspace.New(root, types.HooksConfig{})
	service := control.NewServiceWithWorkspace(fakeSnapshotProvider{snapshot: observability.NewSnapshot()}, manager)

	resolved, err := service.ResolveWorkspacePath(context.Background(), "../ZEE/unsafe")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath returned error: %v", err)
	}
	wantPath := filepath.Join(root, coreworkspace.SafeIdentifier("../ZEE/unsafe"))
	if resolved.WorkspacePath != wantPath || !resolved.ContainedInRoot {
		t.Fatalf("resolved = %#v, want contained path %q", resolved, wantPath)
	}
	if resolved.Boundary.Name != "workspace.lifecycle" {
		t.Fatalf("boundary = %#v, want workspace lifecycle", resolved.Boundary)
	}
	if _, err := os.Stat(resolved.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("resolve should not create workspace, stat err=%v", err)
	}

	prepared, err := service.PrepareWorkspace(context.Background(), "../ZEE/unsafe")
	if err != nil {
		t.Fatalf("PrepareWorkspace returned error: %v", err)
	}
	if prepared.WorkspacePath != wantPath || !prepared.ContainedInRoot {
		t.Fatalf("prepared = %#v, want contained path %q", prepared, wantPath)
	}
	if info, err := os.Stat(prepared.WorkspacePath); err != nil || !info.IsDir() {
		t.Fatalf("prepare should create directory, info=%v err=%v", info, err)
	}

	validated, err := service.ValidateWorkspacePath(context.Background(), prepared.WorkspacePath)
	if err != nil {
		t.Fatalf("ValidateWorkspacePath returned error: %v", err)
	}
	if validated.WorkspacePath != prepared.WorkspacePath || !validated.ContainedInRoot {
		t.Fatalf("validated = %#v, want contained prepared workspace", validated)
	}

	outsidePath := filepath.Join(filepath.Dir(root), "outside")
	invalid, err := service.ValidateWorkspacePath(context.Background(), outsidePath)
	if err != nil {
		t.Fatalf("ValidateWorkspacePath outside returned error: %v", err)
	}
	if invalid.WorkspacePath != outsidePath || invalid.ContainedInRoot {
		t.Fatalf("invalid validation = %#v, want outside root", invalid)
	}

	cleanup, err := service.CleanupWorkspace(context.Background(), prepared.WorkspacePath)
	if err != nil {
		t.Fatalf("CleanupWorkspace returned error: %v", err)
	}
	if cleanup.WorkspacePath != prepared.WorkspacePath || !cleanup.Removed || !cleanup.ContainedInRoot {
		t.Fatalf("cleanup = %#v, want removed prepared workspace", cleanup)
	}
	if _, err := os.Stat(prepared.WorkspacePath); !os.IsNotExist(err) {
		t.Fatalf("cleanup should remove workspace, stat err=%v", err)
	}

	_, err = service.CleanupWorkspace(context.Background(), outsidePath)
	if !errors.Is(err, control.ErrInvalidWorkspacePath) {
		t.Fatalf("CleanupWorkspace outside error = %v, want ErrInvalidWorkspacePath", err)
	}
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
