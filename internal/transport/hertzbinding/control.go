package hertzbinding

import (
	"context"
	"errors"
	"time"

	codexsessionmodel "symphony-go/gen/hertz/model/codexsession"
	commonmodel "symphony-go/gen/hertz/model/common"
	controlmodel "symphony-go/gen/hertz/model/control"
	observabilitymodel "symphony-go/gen/hertz/model/observability"
	orchestratormodel "symphony-go/gen/hertz/model/orchestrator"
	runtimemodel "symphony-go/gen/hertz/model/runtime"
	trackermodel "symphony-go/gen/hertz/model/tracker"
	workflowmodel "symphony-go/gen/hertz/model/workflow"
	workspacemodel "symphony-go/gen/hertz/model/workspace"
	controlplane "symphony-go/internal/service/control"
)

type Control = controlplane.ControlService

type ControlBinding struct {
	control Control
}

func NewControlBinding(control Control) ControlService {
	if control == nil {
		control = controlplane.NewService(nil)
	}
	return ControlBinding{control: control}
}

func (a ControlBinding) GetState(ctx context.Context) (*commonmodel.RuntimeState, error) {
	state, err := a.control.RuntimeState(ctx)
	if err != nil {
		return nil, err
	}
	return runtimeStateModel(state), nil
}

func (a ControlBinding) GetIssue(ctx context.Context, issueIdentifier string) (*commonmodel.IssueDetail, error) {
	detail, err := a.control.IssueDetail(ctx, issueIdentifier)
	if err != nil {
		return nil, controlHTTPError(err)
	}
	return issueDetailModel(detail), nil
}

func (a ControlBinding) GetObservabilitySnapshot(ctx context.Context) (*observabilitymodel.GetObservabilitySnapshotResp, error) {
	snapshot, err := a.control.ObservabilitySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &observabilitymodel.GetObservabilitySnapshotResp{
		Boundary: capabilityBoundaryModel(snapshot.Boundary),
		State:    runtimeStateModel(snapshot.State),
	}, nil
}

func (a ControlBinding) ProjectIssueRun(ctx context.Context, issueIdentifier string) (*orchestratormodel.ProjectIssueRunResp, error) {
	projection, err := a.control.ProjectIssueRun(ctx, issueIdentifier)
	if err != nil {
		return nil, controlHTTPError(err)
	}
	return issueRunProjectionModel(projection), nil
}

func (a ControlBinding) GetRuntimeSettings(ctx context.Context) (*runtimemodel.GetRuntimeSettingsResp, error) {
	result, err := a.control.RuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	return &runtimemodel.GetRuntimeSettingsResp{
		Boundary: capabilityBoundaryModel(result.Boundary),
		Settings: runtimeSettingsModel(result.Settings),
	}, nil
}

func (a ControlBinding) ListTrackerIssues(ctx context.Context, stateNames []string) (*trackermodel.ListTrackerIssuesResp, error) {
	result, err := a.control.ListTrackerIssues(ctx, stateNames)
	if err != nil {
		return nil, controlTrackerHTTPError(err)
	}
	return &trackermodel.ListTrackerIssuesResp{
		Boundary: capabilityBoundaryModel(result.Boundary),
		Issues:   trackerIssueModels(result.Issues),
	}, nil
}

func (a ControlBinding) GetTrackerIssue(ctx context.Context, issueIdentifier string) (*trackermodel.GetTrackerIssueResp, error) {
	result, err := a.control.GetTrackerIssue(ctx, issueIdentifier)
	if err != nil {
		return nil, controlTrackerHTTPError(err)
	}
	return &trackermodel.GetTrackerIssueResp{
		Boundary: capabilityBoundaryModel(result.Boundary),
		Issue:    trackerIssueModel(result.Issue),
	}, nil
}

func (a ControlBinding) UpdateTrackerIssueState(ctx context.Context, issueID, stateName string) (*trackermodel.UpdateTrackerIssueStateResp, error) {
	result, err := a.control.UpdateTrackerIssueState(ctx, controlplane.TrackerIssueStateInput{
		IssueID:   issueID,
		StateName: stateName,
	})
	if err != nil {
		return nil, controlTrackerHTTPError(err)
	}
	return &trackermodel.UpdateTrackerIssueStateResp{
		Boundary:  capabilityBoundaryModel(result.Boundary),
		IssueID:   result.IssueID,
		StateName: result.StateName,
		Updated:   result.Updated,
	}, nil
}

func (a ControlBinding) ResolveWorkspacePath(ctx context.Context, issueIdentifier string) (*workspacemodel.ResolveWorkspacePathResp, error) {
	preparation, err := a.control.ResolveWorkspacePath(ctx, issueIdentifier)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.ResolveWorkspacePathResp{Preparation: workspacePreparationModel(preparation)}, nil
}

func (a ControlBinding) ValidateWorkspacePath(ctx context.Context, workspacePath string) (*workspacemodel.ValidateWorkspacePathResp, error) {
	validation, err := a.control.ValidateWorkspacePath(ctx, workspacePath)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.ValidateWorkspacePathResp{Validation: workspacePathValidationModel(validation)}, nil
}

func (a ControlBinding) PrepareWorkspace(ctx context.Context, issueIdentifier string) (*workspacemodel.PrepareWorkspaceResp, error) {
	preparation, err := a.control.PrepareWorkspace(ctx, issueIdentifier)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.PrepareWorkspaceResp{Preparation: workspacePreparationModel(preparation)}, nil
}

func (a ControlBinding) CleanupWorkspace(ctx context.Context, workspacePath string) (*workspacemodel.CleanupWorkspaceResp, error) {
	result, err := a.control.CleanupWorkspace(ctx, workspacePath)
	if err != nil {
		return nil, controlWorkspaceHTTPError(err)
	}
	return &workspacemodel.CleanupWorkspaceResp{Result: workspaceCleanupResultModel(result)}, nil
}

func (a ControlBinding) LoadWorkflow(ctx context.Context, workflowPath string) (*workflowmodel.LoadWorkflowResp, error) {
	summary, err := a.control.LoadWorkflow(ctx, workflowPath)
	if err != nil {
		return nil, controlWorkflowHTTPError(err)
	}
	return &workflowmodel.LoadWorkflowResp{Summary: workflowSummaryModel(summary)}, nil
}

func (a ControlBinding) RenderWorkflowPrompt(ctx context.Context, request WorkflowRenderRequest) (*workflowmodel.RenderWorkflowPromptResp, error) {
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

func (a ControlBinding) RunTurn(ctx context.Context, request CodexTurnRequest) (*codexsessionmodel.RunTurnResp, error) {
	summary, err := a.control.RunCodexTurn(ctx, controlplane.CodexTurnInput{
		IssueIdentifier: request.IssueIdentifier,
		PromptName:      request.PromptName,
		WorkspacePath:   request.WorkspacePath,
		PromptText:      request.PromptText,
	})
	if err != nil {
		return nil, controlCodexHTTPError(err)
	}
	return &codexsessionmodel.RunTurnResp{Summary: codexTurnSummaryModel(summary)}, nil
}

func (a ControlBinding) Refresh(ctx context.Context) (*controlmodel.RefreshResp, error) {
	result, err := a.control.Refresh(ctx)
	if err != nil {
		return nil, controlRefreshHTTPError(err)
	}
	return &controlmodel.RefreshResp{Accepted: result.Accepted, Status: result.Status}, nil
}

func controlHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidIssueIdentifier):
		return NewError(400, "invalid_issue_identifier", "issue identifier is required")
	case errors.Is(err, controlplane.ErrIssueNotFound):
		return NewError(404, "issue_not_found", "issue not found")
	default:
		return err
	}
}

func controlWorkspaceHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidIssueIdentifier):
		return NewError(400, "invalid_issue_identifier", "issue identifier is required")
	case errors.Is(err, controlplane.ErrInvalidWorkspacePath):
		return NewError(400, "invalid_workspace_path", "workspace path is invalid")
	case errors.Is(err, controlplane.ErrWorkspaceManagerRequired):
		return NewError(503, "workspace_unavailable", "workspace manager is unavailable")
	default:
		return err
	}
}

func controlWorkflowHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidWorkflowPath):
		return NewError(400, "invalid_workflow_path", "workflow path is required")
	default:
		return err
	}
}

func controlCodexHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidCodexTurnRequest):
		return NewError(400, "invalid_codex_turn_request", "issue identifier, workspace path, and prompt text are required")
	case errors.Is(err, controlplane.ErrCodexRunnerRequired):
		return NewError(503, "codex_runner_unavailable", "codex runner is unavailable")
	default:
		return err
	}
}

func controlTrackerHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrInvalidIssueIdentifier):
		return NewError(400, "invalid_issue_identifier", "issue identifier is required")
	case errors.Is(err, controlplane.ErrInvalidIssueState):
		return NewError(400, "invalid_issue_state", "issue state is required")
	case errors.Is(err, controlplane.ErrIssueTrackerRequired):
		return NewError(503, "tracker_unavailable", "issue tracker is unavailable")
	default:
		return err
	}
}

func controlRefreshHTTPError(err error) error {
	switch {
	case errors.Is(err, controlplane.ErrRefreshTriggerRequired):
		return NewError(503, "refresh_unavailable", "refresh trigger is unavailable")
	default:
		return NewError(500, "refresh_failed", err.Error())
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

func codexTurnSummaryModel(summary controlplane.CodexTurnSummary) *codexsessionmodel.CodexTurnSummary {
	return &codexsessionmodel.CodexTurnSummary{
		Boundary:  capabilityBoundaryModel(summary.Boundary),
		SessionID: summary.SessionID,
		TurnCount: summary.TurnCount,
	}
}

func runtimeSettingsModel(settings controlplane.RuntimeSettings) *runtimemodel.RuntimeSettings {
	return &runtimemodel.RuntimeSettings{
		TrackerKind:           settings.TrackerKind,
		TrackerProjectSlug:    settings.TrackerProjectSlug,
		TrackerActiveStates:   append([]string(nil), settings.TrackerActiveStates...),
		TrackerTerminalStates: append([]string(nil), settings.TrackerTerminalStates...),
		ServerPort:            int32(settings.ServerPort),
		ServerPortSet:         settings.ServerPortSet,
		PollingIntervalMs:     int32(settings.PollingIntervalMS),
		WorkspaceRoot:         settings.WorkspaceRoot,
		MergeTarget:           settings.MergeTarget,
		MaxConcurrentAgents:   int32(settings.MaxConcurrentAgents),
		MaxTurns:              int32(settings.MaxTurns),
		MaxRetryBackoffMs:     int32(settings.MaxRetryBackoffMS),
		CodexThreadSandbox:    settings.CodexThreadSandbox,
		CodexTurnTimeoutMs:    int32(settings.CodexTurnTimeoutMS),
		CodexReadTimeoutMs:    int32(settings.CodexReadTimeoutMS),
		CodexStallTimeoutMs:   int32(settings.CodexStallTimeoutMS),
	}
}

func trackerIssueModels(issues []controlplane.TrackerIssue) []*trackermodel.TrackerIssue {
	models := make([]*trackermodel.TrackerIssue, 0, len(issues))
	for _, issue := range issues {
		models = append(models, trackerIssueModel(issue))
	}
	return models
}

func trackerIssueModel(issue controlplane.TrackerIssue) *trackermodel.TrackerIssue {
	blockers := make([]*trackermodel.TrackerBlockerRef, 0, len(issue.BlockedBy))
	for _, blocker := range issue.BlockedBy {
		blockers = append(blockers, &trackermodel.TrackerBlockerRef{
			IssueID:         blocker.IssueID,
			IssueIdentifier: blocker.IssueIdentifier,
			State:           blocker.State,
		})
	}

	model := &trackermodel.TrackerIssue{
		IssueID:         issue.IssueID,
		IssueIdentifier: issue.IssueIdentifier,
		Title:           issue.Title,
		Description:     issue.Description,
		State:           issue.State,
		BranchName:      issue.BranchName,
		Url:             issue.URL,
		Labels:          append([]string(nil), issue.Labels...),
		BlockedBy:       blockers,
	}
	if issue.Priority != nil {
		priority := int32(*issue.Priority)
		model.Priority = &priority
	}
	if issue.CreatedAt != "" {
		model.CreatedAt = stringPtr(issue.CreatedAt)
	}
	if issue.UpdatedAt != "" {
		model.UpdatedAt = stringPtr(issue.UpdatedAt)
	}
	return model
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
