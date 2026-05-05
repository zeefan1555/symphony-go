package scaffold

import (
	"context"

	"github.com/zeefan1555/symphony-go/biz/model/common"
	"github.com/zeefan1555/symphony-go/biz/model/orchestrator"
	"github.com/zeefan1555/symphony-go/internal/runtime/observability"
)

type SnapshotProvider interface {
	Snapshot() observability.Snapshot
}

type Adapter struct {
	provider SnapshotProvider
}

func NewAdapter(provider SnapshotProvider) *Adapter {
	return &Adapter{provider: provider}
}

func (a *Adapter) ProjectIssueRun(ctx context.Context, request *orchestrator.ProjectIssueRunReq) (*orchestrator.IssueRunProjection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issueIdentifier := ""
	if request != nil {
		issueIdentifier = request.IssueIdentifier
	}
	return &orchestrator.IssueRunProjection{
		Boundary: &common.CapabilityBoundary{
			Name:               "orchestrator.issue_run_projection",
			Purpose:            "Project issue-run control state from the handwritten orchestrator runtime.",
			HandwrittenAdapter: "internal/orchestrator/scaffold",
		},
		IssueIdentifier: issueIdentifier,
		RuntimeState:    a.runtimeState(issueIdentifier),
	}, nil
}

func (a *Adapter) runtimeState(issueIdentifier string) string {
	if a == nil || a.provider == nil {
		return "not_running"
	}
	snapshot := a.provider.Snapshot()
	for _, entry := range snapshot.Running {
		if entry.IssueIdentifier == issueIdentifier {
			return "running"
		}
	}
	for _, entry := range snapshot.Retrying {
		if entry.IssueIdentifier == issueIdentifier {
			return "retrying"
		}
	}
	return "not_running"
}
