package hertzbinding

import (
	"context"
	"errors"
	"sync"

	codexsessionmodel "symphony-go/gen/hertz/model/codexsession"
	commonmodel "symphony-go/gen/hertz/model/common"
	controlmodel "symphony-go/gen/hertz/model/control"
	observabilitymodel "symphony-go/gen/hertz/model/observability"
	orchestratormodel "symphony-go/gen/hertz/model/orchestrator"
	runtimemodel "symphony-go/gen/hertz/model/runtime"
	trackermodel "symphony-go/gen/hertz/model/tracker"
	workflowmodel "symphony-go/gen/hertz/model/workflow"
	workspacemodel "symphony-go/gen/hertz/model/workspace"
)

type ControlService interface {
	GetState(context.Context) (*commonmodel.RuntimeState, error)
	GetIssue(context.Context, string) (*commonmodel.IssueDetail, error)
	GetObservabilitySnapshot(context.Context) (*observabilitymodel.GetObservabilitySnapshotResp, error)
	ProjectIssueRun(context.Context, string) (*orchestratormodel.ProjectIssueRunResp, error)
	GetRuntimeSettings(context.Context) (*runtimemodel.GetRuntimeSettingsResp, error)
	ListTrackerIssues(context.Context, []string) (*trackermodel.ListTrackerIssuesResp, error)
	GetTrackerIssue(context.Context, string) (*trackermodel.GetTrackerIssueResp, error)
	UpdateTrackerIssueState(context.Context, string, string) (*trackermodel.UpdateTrackerIssueStateResp, error)
	ResolveWorkspacePath(context.Context, string) (*workspacemodel.ResolveWorkspacePathResp, error)
	ValidateWorkspacePath(context.Context, string) (*workspacemodel.ValidateWorkspacePathResp, error)
	PrepareWorkspace(context.Context, string) (*workspacemodel.PrepareWorkspaceResp, error)
	CleanupWorkspace(context.Context, string) (*workspacemodel.CleanupWorkspaceResp, error)
	LoadWorkflow(context.Context, string) (*workflowmodel.LoadWorkflowResp, error)
	RenderWorkflowPrompt(context.Context, WorkflowRenderRequest) (*workflowmodel.RenderWorkflowPromptResp, error)
	RunTurn(context.Context, CodexTurnRequest) (*codexsessionmodel.RunTurnResp, error)
	Refresh(context.Context) (*controlmodel.RefreshResp, error)
}

type WorkflowRenderRequest struct {
	WorkflowPath     string
	IssueIdentifier  string
	IssueTitle       string
	IssueDescription string
	HasAttempt       bool
	Attempt          int32
}

type CodexTurnRequest struct {
	IssueIdentifier string
	PromptName      string
	WorkspacePath   string
	PromptText      string
}

type unavailableControlService struct{}

func (unavailableControlService) GetState(context.Context) (*commonmodel.RuntimeState, error) {
	return emptyRuntimeState(), nil
}

func (unavailableControlService) GetIssue(context.Context, string) (*commonmodel.IssueDetail, error) {
	return nil, NewError(404, "issue_not_found", "issue not found")
}

func (unavailableControlService) GetObservabilitySnapshot(context.Context) (*observabilitymodel.GetObservabilitySnapshotResp, error) {
	return nil, NewError(503, "observability_unavailable", "observability snapshot is unavailable")
}

func (unavailableControlService) ProjectIssueRun(context.Context, string) (*orchestratormodel.ProjectIssueRunResp, error) {
	return nil, NewError(404, "issue_run_not_found", "issue run not found")
}

func (unavailableControlService) GetRuntimeSettings(context.Context) (*runtimemodel.GetRuntimeSettingsResp, error) {
	return nil, NewError(503, "runtime_unavailable", "runtime settings are unavailable")
}

func (unavailableControlService) ListTrackerIssues(context.Context, []string) (*trackermodel.ListTrackerIssuesResp, error) {
	return nil, NewError(503, "tracker_unavailable", "issue tracker is unavailable")
}

func (unavailableControlService) GetTrackerIssue(context.Context, string) (*trackermodel.GetTrackerIssueResp, error) {
	return nil, NewError(503, "tracker_unavailable", "issue tracker is unavailable")
}

func (unavailableControlService) UpdateTrackerIssueState(context.Context, string, string) (*trackermodel.UpdateTrackerIssueStateResp, error) {
	return nil, NewError(503, "tracker_unavailable", "issue tracker is unavailable")
}

func (unavailableControlService) ResolveWorkspacePath(context.Context, string) (*workspacemodel.ResolveWorkspacePathResp, error) {
	return nil, NewError(503, "workspace_unavailable", "workspace manager is unavailable")
}

func (unavailableControlService) ValidateWorkspacePath(context.Context, string) (*workspacemodel.ValidateWorkspacePathResp, error) {
	return nil, NewError(503, "workspace_unavailable", "workspace manager is unavailable")
}

func (unavailableControlService) PrepareWorkspace(context.Context, string) (*workspacemodel.PrepareWorkspaceResp, error) {
	return nil, NewError(503, "workspace_unavailable", "workspace manager is unavailable")
}

func (unavailableControlService) CleanupWorkspace(context.Context, string) (*workspacemodel.CleanupWorkspaceResp, error) {
	return nil, NewError(503, "workspace_unavailable", "workspace manager is unavailable")
}

func (unavailableControlService) LoadWorkflow(context.Context, string) (*workflowmodel.LoadWorkflowResp, error) {
	return nil, NewError(503, "workflow_unavailable", "workflow loader is unavailable")
}

func (unavailableControlService) RenderWorkflowPrompt(context.Context, WorkflowRenderRequest) (*workflowmodel.RenderWorkflowPromptResp, error) {
	return nil, NewError(503, "workflow_unavailable", "workflow renderer is unavailable")
}

func (unavailableControlService) RunTurn(context.Context, CodexTurnRequest) (*codexsessionmodel.RunTurnResp, error) {
	return nil, NewError(503, "codex_runner_unavailable", "codex runner is unavailable")
}

func (unavailableControlService) Refresh(context.Context) (*controlmodel.RefreshResp, error) {
	return nil, NewError(503, "refresh_unavailable", "refresh trigger is unavailable")
}

var controlService = struct {
	sync.RWMutex
	current ControlService
}{
	current: unavailableControlService{},
}

func SetControlService(service ControlService) func() {
	if service == nil {
		service = unavailableControlService{}
	}

	controlService.Lock()
	previous := controlService.current
	controlService.current = service
	controlService.Unlock()

	return func() {
		controlService.Lock()
		controlService.current = previous
		controlService.Unlock()
	}
}

func CurrentService() ControlService {
	controlService.RLock()
	defer controlService.RUnlock()
	return controlService.current
}

func emptyRuntimeState() *commonmodel.RuntimeState {
	return &commonmodel.RuntimeState{
		Counts:      &commonmodel.RuntimeCounts{},
		Running:     []*commonmodel.IssueRun{},
		Retrying:    []*commonmodel.RetryRun{},
		CodexTotals: &commonmodel.CodexTotals{},
		Polling:     &commonmodel.PollingStatus{},
	}
}

type Error struct {
	status  int
	code    string
	message string
}

func NewError(status int, code, message string) *Error {
	return &Error{status: status, code: code, message: message}
}

func (e *Error) Error() string {
	return e.message
}

func (e *Error) StatusCode() int {
	return e.status
}

func (e *Error) ErrorCode() string {
	return e.code
}

func (e *Error) Message() string {
	return e.message
}

func ErrorEnvelope(err error) (*commonmodel.ErrorEnvelope, int) {
	var controlErr *Error
	if errors.As(err, &controlErr) {
		return &commonmodel.ErrorEnvelope{Error: &commonmodel.ErrorDetail{
			Code:    controlErr.ErrorCode(),
			Message: controlErr.Message(),
		}}, controlErr.StatusCode()
	}
	return &commonmodel.ErrorEnvelope{Error: &commonmodel.ErrorDetail{
		Code:    "internal_error",
		Message: err.Error(),
	}}, 500
}
