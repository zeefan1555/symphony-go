package hertzserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	commonmodel "github.com/zeefan1555/symphony-go/biz/model/common"
	controlmodel "github.com/zeefan1555/symphony-go/biz/model/control"
	orchestratormodel "github.com/zeefan1555/symphony-go/biz/model/orchestrator"
	workflowmodel "github.com/zeefan1555/symphony-go/biz/model/workflow"
	workspacemodel "github.com/zeefan1555/symphony-go/biz/model/workspace"
	"github.com/zeefan1555/symphony-go/biz/router"
	"github.com/zeefan1555/symphony-go/internal/control/hertzhook"
	controlplane "github.com/zeefan1555/symphony-go/internal/service/control"
)

type Control = controlplane.ControlService

type Server struct {
	control Control

	mu      sync.Mutex
	hertz   *server.Hertz
	restore func()
}

func New(control Control) *Server {
	if control == nil {
		control = controlplane.NewService(nil)
	}
	return &Server{control: control}
}

func (s *Server) Serve(listener net.Listener) error {
	h := server.New(server.WithListener(listener))
	restore := hertzhook.SetControlService(controlAdapter{control: s.control})
	router.GeneratedRegister(h)

	s.mu.Lock()
	s.hertz = h
	s.restore = restore
	s.mu.Unlock()

	return h.Run()
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	h := s.hertz
	restore := s.restore
	s.hertz = nil
	s.restore = nil
	s.mu.Unlock()

	if restore != nil {
		restore()
	}
	if h == nil {
		return nil
	}
	return h.Shutdown(ctx)
}

type controlAdapter struct {
	control Control
}

func (a controlAdapter) GetScaffold(ctx context.Context) (hertzhook.ScaffoldStatus, error) {
	status, err := a.control.GetScaffold(ctx)
	if err != nil {
		return hertzhook.ScaffoldStatus{}, err
	}
	return hertzhook.ScaffoldStatus{Status: status.Status}, nil
}

func (a controlAdapter) GetState(ctx context.Context) (*commonmodel.RuntimeState, error) {
	state, err := a.control.RuntimeState(ctx)
	if err != nil {
		return nil, err
	}
	return runtimeStateModel(state), nil
}

func (a controlAdapter) GetIssue(ctx context.Context, issueIdentifier string) (*commonmodel.IssueDetail, error) {
	detail, err := a.control.IssueDetail(ctx, issueIdentifier)
	if err != nil {
		return nil, controlHTTPError(err)
	}
	return issueDetailModel(detail), nil
}

func (a controlAdapter) ProjectIssueRun(ctx context.Context, issueIdentifier string) (*orchestratormodel.ProjectIssueRunResp, error) {
	projection, err := a.control.ProjectIssueRun(ctx, issueIdentifier)
	if err != nil {
		return nil, controlHTTPError(err)
	}
	return issueRunProjectionModel(projection), nil
}

func (a controlAdapter) ResolveWorkspacePath(ctx context.Context, issueIdentifier string) (*workspacemodel.ResolveWorkspacePathResp, error) {
	preparation, err := a.control.ResolveWorkspacePath(ctx, issueIdentifier)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.ResolveWorkspacePathResp{Preparation: workspacePreparationModel(preparation)}, nil
}

func (a controlAdapter) ValidateWorkspacePath(ctx context.Context, workspacePath string) (*workspacemodel.ValidateWorkspacePathResp, error) {
	validation, err := a.control.ValidateWorkspacePath(ctx, workspacePath)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.ValidateWorkspacePathResp{Validation: workspacePathValidationModel(validation)}, nil
}

func (a controlAdapter) PrepareWorkspace(ctx context.Context, issueIdentifier string) (*workspacemodel.PrepareWorkspaceResp, error) {
	preparation, err := a.control.PrepareWorkspace(ctx, issueIdentifier)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.PrepareWorkspaceResp{Preparation: workspacePreparationModel(preparation)}, nil
}

func (a controlAdapter) CleanupWorkspace(ctx context.Context, workspacePath string) (*workspacemodel.CleanupWorkspaceResp, error) {
	result, err := a.control.CleanupWorkspace(ctx, workspacePath)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.CleanupWorkspaceResp{Result: workspaceCleanupResultModel(result)}, nil
}

func (a controlAdapter) LoadWorkflow(ctx context.Context, workflowPath string) (*workflowmodel.LoadWorkflowResp, error) {
	summary, err := a.control.LoadWorkflow(ctx, workflowPath)
	if err != nil {
		return nil, controlWorkflowHTTPError(err)
	}
	return &workflowmodel.LoadWorkflowResp{Summary: workflowSummaryModel(summary)}, nil
}

func (a controlAdapter) RenderWorkflowPrompt(ctx context.Context, request hertzhook.WorkflowRenderRequest) (*workflowmodel.RenderWorkflowPromptResp, error) {
	result, err := a.control.RenderWorkflowPrompt(ctx, controlplane.WorkflowRenderInput{
		WorkflowPath:     request.WorkflowPath,
		IssueIdentifier:  request.IssueIdentifier,
		IssueTitle:       request.IssueTitle,
		IssueDescription: request.IssueDescription,
		HasAttempt:       request.HasAttempt,
		Attempt:          int(request.Attempt),
	})
	if err != nil {
		return nil, controlWorkflowHTTPError(err)
	}
	return &workflowmodel.RenderWorkflowPromptResp{Result: workflowRenderResultModel(result)}, nil
}

func (a controlAdapter) Refresh(ctx context.Context) (*controlmodel.RefreshResp, error) {
	result, err := a.control.Refresh(ctx)
	if err != nil {
		return nil, controlRefreshHTTPError(err)
	}
	return &controlmodel.RefreshResp{Accepted: result.Accepted, Status: result.Status}, nil
}

func controlHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidIssueIdentifier):
		return hertzhook.NewError(400, "invalid_issue_identifier", "issue identifier is required")
	case errors.Is(err, controlplane.ErrIssueNotFound):
		return hertzhook.NewError(404, "issue_not_found", "issue not found")
	default:
		return err
	}
}

func controlWorkspaceHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidIssueIdentifier):
		return hertzhook.NewError(400, "invalid_issue_identifier", "issue identifier is required")
	case errors.Is(err, controlplane.ErrInvalidWorkspacePath):
		return hertzhook.NewError(400, "invalid_workspace_path", "workspace path is invalid")
	case errors.Is(err, controlplane.ErrWorkspaceManagerRequired):
		return hertzhook.NewError(503, "workspace_unavailable", "workspace manager is unavailable")
	default:
		return err
	}
}

func controlWorkflowHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidWorkflowPath):
		return hertzhook.NewError(400, "invalid_workflow_path", "workflow path is required")
	default:
		return err
	}
}

func controlRefreshHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrRefreshTriggerRequired):
		return hertzhook.NewError(503, "refresh_unavailable", "refresh trigger is unavailable")
	default:
		return hertzhook.NewError(500, "refresh_failed", err.Error())
	}
}

func runtimeStateModel(state controlplane.RuntimeState) *commonmodel.RuntimeState {
	running := make([]*commonmodel.IssueRun, 0, len(state.Running))
	for _, entry := range state.Running {
		running = append(running, issueRunModel(entry))
	}

	retrying := make([]*commonmodel.RetryRun, 0, len(state.Retrying))
	for _, entry := range state.Retrying {
		retrying = append(retrying, retryRunModel(entry))
	}

	modelState := &commonmodel.RuntimeState{
		GeneratedAt: formatControlTime(state.GeneratedAt),
		Counts: &commonmodel.RuntimeCounts{
			Running:  int32(state.Counts.Running),
			Retrying: int32(state.Counts.Retrying),
		},
		Running:  running,
		Retrying: retrying,
		CodexTotals: &commonmodel.CodexTotals{
			InputTokens:    int32(state.CodexTotals.InputTokens),
			OutputTokens:   int32(state.CodexTotals.OutputTokens),
			TotalTokens:    int32(state.CodexTotals.TotalTokens),
			SecondsRunning: state.CodexTotals.SecondsRunning,
		},
		Polling: &commonmodel.PollingStatus{
			Checking:     state.Polling.Checking,
			NextPollInMs: state.Polling.NextPollInMS,
			IntervalMs:   int32(state.Polling.IntervalMS),
		},
	}
	if value := formatControlTime(state.Polling.NextPollAt); value != "" {
		modelState.Polling.NextPollAt = stringPtr(value)
	}
	if value := formatControlTime(state.Polling.LastPollAt); value != "" {
		modelState.Polling.LastPollAt = stringPtr(value)
	}
	if state.LastError != "" {
		modelState.LastError = stringPtr(state.LastError)
	}
	return modelState
}

func issueDetailModel(detail controlplane.IssueDetail) *commonmodel.IssueDetail {
	modelDetail := &commonmodel.IssueDetail{
		IssueID:         detail.IssueID,
		IssueIdentifier: detail.IssueIdentifier,
		Status:          detail.Status,
	}
	if detail.Running != nil {
		modelDetail.Running = issueRunModel(*detail.Running)
	}
	if detail.Retry != nil {
		modelDetail.Retry = retryRunModel(*detail.Retry)
	}
	return modelDetail
}

func issueRunModel(entry controlplane.IssueRun) *commonmodel.IssueRun {
	modelEntry := &commonmodel.IssueRun{
		IssueID:         entry.IssueID,
		IssueIdentifier: entry.IssueIdentifier,
		State:           entry.State,
		TurnCount:       int32(entry.TurnCount),
		Tokens: &commonmodel.TokenUsage{
			InputTokens:  int32(entry.Tokens.InputTokens),
			OutputTokens: int32(entry.Tokens.OutputTokens),
			TotalTokens:  int32(entry.Tokens.TotalTokens),
		},
		RuntimeSeconds: entry.RuntimeSeconds,
	}
	if entry.WorkspacePath != "" {
		modelEntry.WorkspacePath = stringPtr(entry.WorkspacePath)
	}
	if entry.SessionID != "" {
		modelEntry.SessionID = stringPtr(entry.SessionID)
	}
	if entry.PID != 0 {
		pid := int32(entry.PID)
		modelEntry.Pid = &pid
	}
	if entry.LastEvent != "" {
		modelEntry.LastEvent = stringPtr(entry.LastEvent)
	}
	if entry.LastMessage != "" {
		modelEntry.LastMessage = stringPtr(entry.LastMessage)
	}
	if value := formatControlTime(entry.StartedAt); value != "" {
		modelEntry.StartedAt = stringPtr(value)
	}
	if value := formatControlTime(entry.LastEventAt); value != "" {
		modelEntry.LastEventAt = stringPtr(value)
	}
	return modelEntry
}

func retryRunModel(entry controlplane.Retry) *commonmodel.RetryRun {
	modelEntry := &commonmodel.RetryRun{
		IssueID:         entry.IssueID,
		IssueIdentifier: entry.IssueIdentifier,
		Attempt:         int32(entry.Attempt),
		DueAt:           formatControlTime(entry.DueAt),
	}
	if entry.Error != "" {
		modelEntry.Error = stringPtr(entry.Error)
	}
	if entry.WorkspacePath != "" {
		modelEntry.WorkspacePath = stringPtr(entry.WorkspacePath)
	}
	return modelEntry
}

func issueRunProjectionModel(projection controlplane.IssueRunProjection) *orchestratormodel.ProjectIssueRunResp {
	return &orchestratormodel.ProjectIssueRunResp{
		Projection: &orchestratormodel.IssueRunProjection{
			Boundary:        capabilityBoundaryModel(projection.Boundary),
			IssueIdentifier: projection.IssueIdentifier,
			RuntimeState:    projection.RuntimeState,
		},
	}
}

func workspacePreparationModel(preparation controlplane.WorkspacePreparation) *workspacemodel.WorkspacePreparation {
	return &workspacemodel.WorkspacePreparation{
		Boundary:        capabilityBoundaryModel(preparation.Boundary),
		WorkspacePath:   preparation.WorkspacePath,
		ContainedInRoot: preparation.ContainedInRoot,
	}
}

func workspacePathValidationModel(validation controlplane.WorkspacePathValidation) *workspacemodel.WorkspacePathValidation {
	return &workspacemodel.WorkspacePathValidation{
		Boundary:        capabilityBoundaryModel(validation.Boundary),
		WorkspacePath:   validation.WorkspacePath,
		ContainedInRoot: validation.ContainedInRoot,
	}
}

func workspaceCleanupResultModel(result controlplane.WorkspaceCleanupResult) *workspacemodel.WorkspaceCleanupResult {
	return &workspacemodel.WorkspaceCleanupResult{
		Boundary:        capabilityBoundaryModel(result.Boundary),
		WorkspacePath:   result.WorkspacePath,
		Removed:         result.Removed,
		ContainedInRoot: result.ContainedInRoot,
	}
}

func workflowSummaryModel(summary controlplane.WorkflowSummary) *workflowmodel.WorkflowSummary {
	return &workflowmodel.WorkflowSummary{
		Boundary:     capabilityBoundaryModel(summary.Boundary),
		WorkflowPath: summary.WorkflowPath,
		StateNames:   append([]string(nil), summary.StateNames...),
	}
}

func workflowRenderResultModel(result controlplane.WorkflowRenderResult) *workflowmodel.WorkflowRenderResult {
	return &workflowmodel.WorkflowRenderResult{
		Boundary: capabilityBoundaryModel(result.Boundary),
		Prompt:   result.Prompt,
	}
}

func capabilityBoundaryModel(boundary controlplane.CapabilityBoundary) *commonmodel.CapabilityBoundary {
	return &commonmodel.CapabilityBoundary{
		Name:               boundary.Name,
		Purpose:            boundary.Purpose,
		HandwrittenAdapter: boundary.HandwrittenAdapter,
	}
}

func formatControlTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func stringPtr(value string) *string {
	return &value
}
