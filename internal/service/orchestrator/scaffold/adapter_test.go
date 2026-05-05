package scaffold

import (
	"context"
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/biz/model/orchestrator"
	"github.com/zeefan1555/symphony-go/internal/runtime/observability"
)

func TestAdapterExposesStandardOrchestratorDiagnosticMethod(t *testing.T) {
	var _ interface {
		ProjectIssueRun(context.Context, *orchestrator.ProjectIssueRunReq) (*orchestrator.IssueRunProjection, error)
	} = (*Adapter)(nil)
}

func TestProjectIssueRunDelegatesToSnapshotProvider(t *testing.T) {
	provider := snapshotProvider{snapshot: observability.Snapshot{
		GeneratedAt: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		Running: []observability.RunningEntry{{
			IssueIdentifier: "ZEE-56",
			State:           "In Progress",
		}},
	}}
	adapter := NewAdapter(provider)

	projection, err := adapter.ProjectIssueRun(context.Background(), &orchestrator.ProjectIssueRunReq{
		IssueIdentifier: "ZEE-56",
	})
	if err != nil {
		t.Fatalf("ProjectIssueRun() error = %v", err)
	}
	if projection.Boundary == nil {
		t.Fatal("projection boundary is nil")
	}
	if projection.Boundary.Name != "orchestrator.issue_run_projection" {
		t.Fatalf("boundary name = %q", projection.Boundary.Name)
	}
	if projection.Boundary.HandwrittenAdapter != "internal/service/orchestrator/scaffold" {
		t.Fatalf("adapter = %q", projection.Boundary.HandwrittenAdapter)
	}
	if projection.IssueIdentifier != "ZEE-56" {
		t.Fatalf("issue = %q, want ZEE-56", projection.IssueIdentifier)
	}
	if projection.RuntimeState != "running" {
		t.Fatalf("runtime state = %q, want running", projection.RuntimeState)
	}
}

func TestProjectIssueRunReturnsQueuedForMissingIssue(t *testing.T) {
	adapter := NewAdapter(snapshotProvider{snapshot: observability.NewSnapshot()})

	projection, err := adapter.ProjectIssueRun(context.Background(), &orchestrator.ProjectIssueRunReq{
		IssueIdentifier: "ZEE-404",
	})
	if err != nil {
		t.Fatalf("ProjectIssueRun() error = %v", err)
	}
	if projection.RuntimeState != "not_running" {
		t.Fatalf("runtime state = %q, want not_running", projection.RuntimeState)
	}
}

type snapshotProvider struct {
	snapshot observability.Snapshot
}

func (p snapshotProvider) Snapshot() observability.Snapshot {
	return p.snapshot
}
