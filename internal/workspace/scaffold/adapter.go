package scaffold

import (
	"context"

	"github.com/zeefan1555/symphony-go/biz/model/common"
	generated "github.com/zeefan1555/symphony-go/biz/model/workspace"
	issuemodel "github.com/zeefan1555/symphony-go/internal/service/issue"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/workspace"
)

type Adapter struct {
	manager *coreworkspace.Manager
}

func NewAdapter(manager *coreworkspace.Manager) *Adapter {
	return &Adapter{manager: manager}
}

func (a *Adapter) ResolveWorkspacePath(ctx context.Context, request *generated.ResolveWorkspacePathReq) (*generated.WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := a.pathForIssue(request.GetIssueIdentifier())
	if err != nil {
		return nil, err
	}
	contained := a.manager.ValidateWorkspacePath(path) == nil
	return workspacePreparation(path, contained), nil
}

func (a *Adapter) ValidateWorkspacePath(ctx context.Context, request *generated.ValidateWorkspacePathReq) (*generated.WorkspacePathValidation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := ""
	if request != nil {
		path = request.WorkspacePath
	}
	return &generated.WorkspacePathValidation{
		Boundary:        workspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: a.manager.ValidateWorkspacePath(path) == nil,
	}, nil
}

func (a *Adapter) PrepareWorkspace(ctx context.Context, request *generated.PrepareWorkspaceReq) (*generated.WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issue := issuemodel.Issue{}
	if request != nil {
		issue.Identifier = request.IssueIdentifier
	}
	path, _, err := a.manager.Ensure(ctx, issue)
	if err != nil {
		return nil, err
	}
	return workspacePreparation(path, a.manager.ValidateWorkspacePath(path) == nil), nil
}

func (a *Adapter) CleanupWorkspace(ctx context.Context, request *generated.CleanupWorkspaceReq) (*generated.WorkspaceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := ""
	if request != nil {
		path = request.WorkspacePath
	}
	if err := a.manager.Remove(ctx, path); err != nil {
		return nil, err
	}
	return &generated.WorkspaceCleanupResult{
		Boundary:        workspaceBoundary(),
		WorkspacePath:   path,
		Removed:         true,
		ContainedInRoot: true,
	}, nil
}

func (a *Adapter) pathForIssue(issueIdentifier string) (string, error) {
	issue := issuemodel.Issue{Identifier: issueIdentifier}
	return a.manager.PathForIssue(issue)
}

func workspacePreparation(path string, contained bool) *generated.WorkspacePreparation {
	return &generated.WorkspacePreparation{
		Boundary:        workspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: contained,
	}
}

func workspaceBoundary() *common.CapabilityBoundary {
	return &common.CapabilityBoundary{
		Name:               "workspace.lifecycle",
		Purpose:            "Resolve, validate, prepare, and clean up issue workspaces through the handwritten workspace manager.",
		HandwrittenAdapter: "internal/workspace/scaffold",
	}
}
