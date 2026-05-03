package scaffold

import (
	"context"

	"github.com/zeefan1555/symphony-go/internal/generated/hertz/scaffold/model"
	generated "github.com/zeefan1555/symphony-go/internal/generated/hertz/scaffold/workspace"
	"github.com/zeefan1555/symphony-go/internal/types"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/workspace"
)

type Adapter struct {
	manager *coreworkspace.Manager
}

func NewAdapter(manager *coreworkspace.Manager) *Adapter {
	return &Adapter{manager: manager}
}

func (a *Adapter) ResolveWorkspacePath(ctx context.Context, request *generated.WorkspacePrepareRequest) (*generated.WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := a.pathForIssue(request)
	if err != nil {
		return nil, err
	}
	contained := a.manager.ValidateWorkspacePath(path) == nil
	return workspacePreparation(path, contained), nil
}

func (a *Adapter) ValidateWorkspacePath(ctx context.Context, request *generated.WorkspacePathValidationRequest) (*generated.WorkspacePathValidation, error) {
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

func (a *Adapter) PrepareWorkspace(ctx context.Context, request *generated.WorkspacePrepareRequest) (*generated.WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	issue := types.Issue{}
	if request != nil {
		issue.Identifier = request.IssueIdentifier
	}
	path, _, err := a.manager.Ensure(ctx, issue)
	if err != nil {
		return nil, err
	}
	return workspacePreparation(path, a.manager.ValidateWorkspacePath(path) == nil), nil
}

func (a *Adapter) CleanupWorkspace(ctx context.Context, request *generated.WorkspaceCleanupRequest) (*generated.WorkspaceCleanupResult, error) {
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

func (a *Adapter) pathForIssue(request *generated.WorkspacePrepareRequest) (string, error) {
	issue := types.Issue{}
	if request != nil {
		issue.Identifier = request.IssueIdentifier
	}
	return a.manager.PathForIssue(issue)
}

func workspacePreparation(path string, contained bool) *generated.WorkspacePreparation {
	return &generated.WorkspacePreparation{
		Boundary:        workspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: contained,
	}
}

func workspaceBoundary() *model.CapabilityBoundary {
	return &model.CapabilityBoundary{
		Name:               "workspace.lifecycle",
		Purpose:            "Resolve, validate, prepare, and clean up issue workspaces through the handwritten workspace manager.",
		HandwrittenAdapter: "internal/workspace/scaffold",
	}
}
