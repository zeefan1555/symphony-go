namespace go api

include "common.thrift"
include "control.thrift"
include "orchestrator.thrift"
include "workspace.thrift"

service SymphonyAPI {
    control.GetScaffoldResp GetScaffold(1: control.GetScaffoldReq req) (api.post="/api/v1/control/get-scaffold")
    control.GetStateResp GetState(1: control.GetStateReq req) (api.post="/api/v1/control/get-state")
    control.RefreshResp Refresh(1: control.RefreshReq req) (api.post="/api/v1/control/refresh")
    control.GetIssueResp GetIssue(1: control.GetIssueReq req) (api.post="/api/v1/control/get-issue")
    orchestrator.ProjectIssueRunResp ProjectIssueRun(1: orchestrator.ProjectIssueRunReq req) (api.post="/api/v1/orchestrator/project-issue-run")
    workspace.ResolveWorkspacePathResp ResolveWorkspacePath(1: workspace.ResolveWorkspacePathReq req) (api.post="/api/v1/workspace/resolve")
    workspace.ValidateWorkspacePathResp ValidateWorkspacePath(1: workspace.ValidateWorkspacePathReq req) (api.post="/api/v1/workspace/validate")
    workspace.PrepareWorkspaceResp PrepareWorkspace(1: workspace.PrepareWorkspaceReq req) (api.post="/api/v1/workspace/prepare")
    workspace.CleanupWorkspaceResp CleanupWorkspace(1: workspace.CleanupWorkspaceReq req) (api.post="/api/v1/workspace/cleanup")
}
