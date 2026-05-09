package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/issueflow"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

var (
	ErrSnapshotProviderRequired = errors.New("control snapshot provider required")
	ErrInvalidIssueIdentifier   = errors.New("issue identifier is required")
	ErrIssueNotFound            = errors.New("issue not found")
	ErrRefreshTriggerRequired   = errors.New("control refresh trigger required")
	ErrWorkspaceManagerRequired = errors.New("workspace manager required")
	ErrInvalidWorkspacePath     = errors.New("invalid workspace path")
	ErrInvalidWorkflowPath      = errors.New("workflow path is required")
	ErrCodexRunnerRequired      = errors.New("codex runner required")
	ErrInvalidCodexTurnRequest  = errors.New("invalid codex turn request")
	ErrIssueTrackerRequired     = errors.New("issue tracker required")
	ErrInvalidIssueState        = errors.New("issue state is required")
)

const (
	IssueStatusRunning    = "running"
	IssueStatusRetrying   = "retrying"
	IssueStatusNotRunning = "not_running"

	RefreshStatusQueued         = "queued"
	RefreshStatusAlreadyPending = "already_pending"
)

type SnapshotProvider interface {
	Snapshot() observability.Snapshot
}

type RefreshTrigger interface {
	RequestRefresh(context.Context) (bool, error)
}

type RuntimeProvider interface {
	RuntimeConfig() runtimeconfig.Config
	RuntimeWorkspace() *workspace.Manager
	RuntimeRunner() interface {
		RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
	}
	RuntimeTracker() interface {
		FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error)
		FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error)
		FetchIssue(context.Context, string) (issuemodel.Issue, error)
		UpdateIssueState(context.Context, string, string) error
	}
}

type CodexSessionRunner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}

type IssueTracker interface {
	FetchActiveIssues(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]issuemodel.Issue, error)
	FetchIssue(context.Context, string) (issuemodel.Issue, error)
	UpdateIssueState(context.Context, string, string) error
}

type ControlService interface {
	RuntimeState(context.Context) (RuntimeState, error)
	IssueDetail(context.Context, string) (IssueDetail, error)
	ObservabilitySnapshot(context.Context) (ObservabilitySnapshot, error)
	IssueFlow(context.Context) (IssueFlowResult, error)
	ProjectIssueRun(context.Context, string) (IssueRunProjection, error)
	RuntimeSettings(context.Context) (RuntimeSettingsResult, error)
	ListTrackerIssues(context.Context, []string) (TrackerIssueList, error)
	GetTrackerIssue(context.Context, string) (TrackerIssueResult, error)
	UpdateTrackerIssueState(context.Context, TrackerIssueStateInput) (TrackerIssueStateResult, error)
	ResolveWorkspacePath(context.Context, string) (WorkspacePreparation, error)
	ValidateWorkspacePath(context.Context, string) (WorkspacePathValidation, error)
	PrepareWorkspace(context.Context, string) (WorkspacePreparation, error)
	CleanupWorkspace(context.Context, string) (WorkspaceCleanupResult, error)
	LoadWorkflow(context.Context, string) (WorkflowSummary, error)
	RenderWorkflowPrompt(context.Context, WorkflowRenderInput) (WorkflowRenderResult, error)
	RunCodexTurn(context.Context, CodexTurnInput) (CodexTurnSummary, error)
	Refresh(context.Context) (RefreshResult, error)
}

type Service struct {
	provider  SnapshotProvider
	workspace *workspace.Manager
	runner    CodexSessionRunner
	tracker   IssueTracker
	config    runtimeconfig.Config
}

type ServiceOptions struct {
	Provider  SnapshotProvider
	Workspace *workspace.Manager
	Runner    CodexSessionRunner
	Tracker   IssueTracker
	Config    runtimeconfig.Config
}

func NewService(provider SnapshotProvider) *Service {
	return NewServiceWithOptions(ServiceOptions{Provider: provider})
}

func NewServiceWithWorkspace(provider SnapshotProvider, manager *workspace.Manager) *Service {
	return NewServiceWithOptions(ServiceOptions{Provider: provider, Workspace: manager})
}

func NewServiceWithCodexRunner(provider SnapshotProvider, runner CodexSessionRunner) *Service {
	return NewServiceWithOptions(ServiceOptions{Provider: provider, Runner: runner})
}

func NewServiceWithWorkspaceAndCodexRunner(provider SnapshotProvider, manager *workspace.Manager, runner CodexSessionRunner) *Service {
	return NewServiceWithOptions(ServiceOptions{Provider: provider, Workspace: manager, Runner: runner})
}

func NewServiceWithOptions(opts ServiceOptions) *Service {
	return &Service{
		provider:  opts.Provider,
		workspace: opts.Workspace,
		runner:    opts.Runner,
		tracker:   opts.Tracker,
		config:    opts.Config,
	}
}

func (s *Service) runtimeProvider() RuntimeProvider {
	if s == nil || s.provider == nil {
		return nil
	}
	provider, _ := s.provider.(RuntimeProvider)
	return provider
}

func (s *Service) currentConfig() runtimeconfig.Config {
	if provider := s.runtimeProvider(); provider != nil {
		return provider.RuntimeConfig()
	}
	if s == nil {
		return runtimeconfig.Config{}
	}
	return s.config
}

func (s *Service) currentWorkspace() *workspace.Manager {
	if provider := s.runtimeProvider(); provider != nil {
		return provider.RuntimeWorkspace()
	}
	if s == nil {
		return nil
	}
	return s.workspace
}

func (s *Service) currentRunner() CodexSessionRunner {
	if provider := s.runtimeProvider(); provider != nil {
		return provider.RuntimeRunner()
	}
	if s == nil {
		return nil
	}
	return s.runner
}

func (s *Service) currentTracker() IssueTracker {
	if provider := s.runtimeProvider(); provider != nil {
		return provider.RuntimeTracker()
	}
	if s == nil {
		return nil
	}
	return s.tracker
}

func (s *Service) RuntimeState(ctx context.Context) (RuntimeState, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeState{}, err
	}
	if s == nil || s.provider == nil {
		return RuntimeState{}, ErrSnapshotProviderRequired
	}
	return ProjectSnapshot(s.provider.Snapshot()), nil
}

func (s *Service) IssueDetail(ctx context.Context, issueIdentifier string) (IssueDetail, error) {
	if err := ctx.Err(); err != nil {
		return IssueDetail{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return IssueDetail{}, ErrInvalidIssueIdentifier
	}
	if s == nil || s.provider == nil {
		return IssueDetail{}, ErrSnapshotProviderRequired
	}
	return FindIssueDetail(ProjectSnapshot(s.provider.Snapshot()), issueIdentifier)
}

func (s *Service) ObservabilitySnapshot(ctx context.Context) (ObservabilitySnapshot, error) {
	state, err := s.RuntimeState(ctx)
	if err != nil {
		return ObservabilitySnapshot{}, err
	}
	return ObservabilitySnapshot{
		Boundary: ObservabilityBoundary(),
		State:    state,
	}, nil
}

func (s *Service) IssueFlow(ctx context.Context) (IssueFlowResult, error) {
	if err := ctx.Err(); err != nil {
		return IssueFlowResult{}, err
	}
	return projectIssueFlow(issueflow.DefinitionForTrunk()), nil
}

func (s *Service) ProjectIssueRun(ctx context.Context, issueIdentifier string) (IssueRunProjection, error) {
	if err := ctx.Err(); err != nil {
		return IssueRunProjection{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return IssueRunProjection{}, ErrInvalidIssueIdentifier
	}
	if s == nil || s.provider == nil {
		return IssueRunProjection{}, ErrSnapshotProviderRequired
	}
	return ProjectIssueRunState(ProjectSnapshot(s.provider.Snapshot()), issueIdentifier), nil
}

func (s *Service) RuntimeSettings(ctx context.Context) (RuntimeSettingsResult, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeSettingsResult{}, err
	}
	if s == nil {
		return RuntimeSettingsResult{}, nil
	}
	cfg := s.currentConfig()
	return RuntimeSettingsResult{
		Boundary: RuntimeBoundary(),
		Settings: RuntimeSettings{
			TrackerKind:           cfg.Tracker.Kind,
			TrackerProjectSlug:    cfg.Tracker.ProjectSlug,
			TrackerActiveStates:   append([]string(nil), cfg.Tracker.ActiveStates...),
			TrackerTerminalStates: append([]string(nil), cfg.Tracker.TerminalStates...),
			ServerPort:            cfg.Server.Port,
			ServerPortSet:         cfg.Server.PortSet,
			PollingIntervalMS:     cfg.Polling.IntervalMS,
			WorkspaceRoot:         cfg.Workspace.Root,
			MergeTarget:           cfg.Merge.Target,
			MaxConcurrentAgents:   cfg.Agent.MaxConcurrentAgents,
			MaxTurns:              cfg.Agent.MaxTurns,
			MaxRetryBackoffMS:     cfg.Agent.MaxRetryBackoffMS,
			CodexThreadSandbox:    cfg.Codex.ThreadSandbox,
			CodexTurnTimeoutMS:    cfg.Codex.TurnTimeoutMS,
			CodexReadTimeoutMS:    cfg.Codex.ReadTimeoutMS,
			CodexStallTimeoutMS:   cfg.Codex.StallTimeoutMS,
		},
	}, nil
}

func (s *Service) ListTrackerIssues(ctx context.Context, stateNames []string) (TrackerIssueList, error) {
	if err := ctx.Err(); err != nil {
		return TrackerIssueList{}, err
	}
	tracker := s.currentTracker()
	if tracker == nil {
		return TrackerIssueList{}, ErrIssueTrackerRequired
	}
	states := cleanStates(stateNames)
	var (
		issues []issuemodel.Issue
		err    error
	)
	if len(states) == 0 {
		issues, err = tracker.FetchActiveIssues(ctx, s.currentConfig().Tracker.ActiveStates)
	} else {
		issues, err = tracker.FetchIssuesByStates(ctx, states)
	}
	if err != nil {
		return TrackerIssueList{}, err
	}
	return TrackerIssueList{
		Boundary: TrackerBoundary(),
		Issues:   trackerIssueModels(issues),
	}, nil
}

func (s *Service) GetTrackerIssue(ctx context.Context, issueIdentifier string) (TrackerIssueResult, error) {
	if err := ctx.Err(); err != nil {
		return TrackerIssueResult{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return TrackerIssueResult{}, ErrInvalidIssueIdentifier
	}
	tracker := s.currentTracker()
	if tracker == nil {
		return TrackerIssueResult{}, ErrIssueTrackerRequired
	}
	issue, err := tracker.FetchIssue(ctx, issueIdentifier)
	if err != nil {
		return TrackerIssueResult{}, err
	}
	return TrackerIssueResult{
		Boundary: TrackerBoundary(),
		Issue:    trackerIssueModel(issue),
	}, nil
}

func (s *Service) UpdateTrackerIssueState(ctx context.Context, input TrackerIssueStateInput) (TrackerIssueStateResult, error) {
	if err := ctx.Err(); err != nil {
		return TrackerIssueStateResult{}, err
	}
	issueID := strings.TrimSpace(input.IssueID)
	if issueID == "" {
		return TrackerIssueStateResult{}, ErrInvalidIssueIdentifier
	}
	stateName := strings.TrimSpace(input.StateName)
	if stateName == "" {
		return TrackerIssueStateResult{}, ErrInvalidIssueState
	}
	tracker := s.currentTracker()
	if tracker == nil {
		return TrackerIssueStateResult{}, ErrIssueTrackerRequired
	}
	if err := tracker.UpdateIssueState(ctx, issueID, stateName); err != nil {
		return TrackerIssueStateResult{}, err
	}
	return TrackerIssueStateResult{
		Boundary:  TrackerBoundary(),
		IssueID:   issueID,
		StateName: stateName,
		Updated:   true,
	}, nil
}

func (s *Service) ResolveWorkspacePath(ctx context.Context, issueIdentifier string) (WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePreparation{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return WorkspacePreparation{}, ErrInvalidIssueIdentifier
	}
	manager := s.currentWorkspace()
	if manager == nil {
		return WorkspacePreparation{}, ErrWorkspaceManagerRequired
	}
	path, err := manager.PathForIssue(issuemodel.Issue{Identifier: issueIdentifier})
	if err != nil {
		return WorkspacePreparation{}, err
	}
	return workspacePreparation(path, manager.ValidateWorkspacePath(path) == nil), nil
}

func (s *Service) ValidateWorkspacePath(ctx context.Context, path string) (WorkspacePathValidation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePathValidation{}, err
	}
	manager := s.currentWorkspace()
	if manager == nil {
		return WorkspacePathValidation{}, ErrWorkspaceManagerRequired
	}
	return WorkspacePathValidation{
		Boundary:        WorkspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: manager.ValidateWorkspacePath(path) == nil,
	}, nil
}

func (s *Service) PrepareWorkspace(ctx context.Context, issueIdentifier string) (WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePreparation{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return WorkspacePreparation{}, ErrInvalidIssueIdentifier
	}
	manager := s.currentWorkspace()
	if manager == nil {
		return WorkspacePreparation{}, ErrWorkspaceManagerRequired
	}
	path, _, err := manager.Ensure(ctx, issuemodel.Issue{Identifier: issueIdentifier})
	if err != nil {
		return WorkspacePreparation{}, err
	}
	return workspacePreparation(path, manager.ValidateWorkspacePath(path) == nil), nil
}

func (s *Service) CleanupWorkspace(ctx context.Context, path string) (WorkspaceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceCleanupResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return WorkspaceCleanupResult{}, ErrInvalidWorkspacePath
	}
	manager := s.currentWorkspace()
	if manager == nil {
		return WorkspaceCleanupResult{}, ErrWorkspaceManagerRequired
	}
	if err := manager.ValidateWorkspacePath(path); err != nil {
		return WorkspaceCleanupResult{}, fmt.Errorf("%w: %v", ErrInvalidWorkspacePath, err)
	}
	if err := manager.Remove(ctx, path); err != nil {
		return WorkspaceCleanupResult{}, err
	}
	return WorkspaceCleanupResult{
		Boundary:        WorkspaceBoundary(),
		WorkspacePath:   path,
		Removed:         true,
		ContainedInRoot: true,
	}, nil
}

func (s *Service) LoadWorkflow(ctx context.Context, path string) (WorkflowSummary, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowSummary{}, err
	}
	if strings.TrimSpace(path) == "" {
		return WorkflowSummary{}, ErrInvalidWorkflowPath
	}
	loaded, err := workflow.Load(path)
	if err != nil {
		return WorkflowSummary{}, err
	}
	return WorkflowSummary{
		Boundary:     WorkflowBoundary(),
		WorkflowPath: path,
		StateNames:   append([]string(nil), loaded.Config.Tracker.ActiveStates...),
	}, nil
}

func (s *Service) RenderWorkflowPrompt(ctx context.Context, input WorkflowRenderInput) (WorkflowRenderResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowRenderResult{}, err
	}
	if strings.TrimSpace(input.WorkflowPath) == "" {
		return WorkflowRenderResult{}, ErrInvalidWorkflowPath
	}
	loaded, err := workflow.Load(input.WorkflowPath)
	if err != nil {
		return WorkflowRenderResult{}, err
	}
	var attempt *int
	if input.HasAttempt {
		value := input.Attempt
		attempt = &value
	}
	prompt, err := workflow.Render(loaded.PromptTemplate, issuemodel.Issue{
		Identifier:  input.IssueIdentifier,
		Title:       input.IssueTitle,
		Description: input.IssueDescription,
	}, attempt)
	if err != nil {
		return WorkflowRenderResult{}, err
	}
	return WorkflowRenderResult{
		Boundary: WorkflowBoundary(),
		Prompt:   prompt,
	}, nil
}

func (s *Service) RunCodexTurn(ctx context.Context, input CodexTurnInput) (CodexTurnSummary, error) {
	if err := ctx.Err(); err != nil {
		return CodexTurnSummary{}, err
	}
	if strings.TrimSpace(input.IssueIdentifier) == "" || strings.TrimSpace(input.WorkspacePath) == "" || strings.TrimSpace(input.PromptText) == "" {
		return CodexTurnSummary{}, ErrInvalidCodexTurnRequest
	}
	runner := s.currentRunner()
	if runner == nil {
		return CodexTurnSummary{}, ErrCodexRunnerRequired
	}
	result, err := runner.RunSession(ctx, codex.SessionRequest{
		WorkspacePath: input.WorkspacePath,
		Issue:         issuemodel.Issue{Identifier: input.IssueIdentifier},
		Prompts: []codex.TurnPrompt{{
			Text: input.PromptText,
		}},
	}, nil)
	if err != nil {
		return CodexTurnSummary{}, err
	}
	return CodexTurnSummary{
		Boundary:  CodexSessionBoundary(),
		SessionID: result.SessionID,
		TurnCount: int32(len(result.Turns)),
	}, nil
}

func (s *Service) Refresh(ctx context.Context) (RefreshResult, error) {
	if err := ctx.Err(); err != nil {
		return RefreshResult{}, err
	}
	if s == nil || s.provider == nil {
		return RefreshResult{}, ErrSnapshotProviderRequired
	}
	trigger, ok := s.provider.(RefreshTrigger)
	if !ok {
		return RefreshResult{}, ErrRefreshTriggerRequired
	}
	queued, err := trigger.RequestRefresh(ctx)
	if err != nil {
		return RefreshResult{}, err
	}
	if !queued {
		return RefreshResult{Accepted: true, Status: RefreshStatusAlreadyPending}, nil
	}
	return RefreshResult{Accepted: true, Status: RefreshStatusQueued}, nil
}

func ProjectSnapshot(snapshot observability.Snapshot) RuntimeState {
	running := make([]IssueRun, 0, len(snapshot.Running))
	for _, entry := range snapshot.Running {
		running = append(running, IssueRun{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.IssueIdentifier,
			State:           entry.State,
			AgentPhase:      entry.AgentPhase,
			Stage:           entry.Stage,
			WorkspacePath:   entry.WorkspacePath,
			Attempt:         entry.Attempt,
			SessionID:       entry.SessionID,
			ThreadID:        entry.ThreadID,
			TurnID:          entry.TurnID,
			PID:             entry.PID,
			TurnCount:       entry.TurnCount,
			LastEvent:       entry.LastEvent,
			LastMessage:     entry.LastMessage,
			StartedAt:       entry.StartedAt,
			LastEventAt:     entry.LastEventAt,
			Tokens: TokenUsage{
				InputTokens:  entry.Tokens.InputTokens,
				OutputTokens: entry.Tokens.OutputTokens,
				TotalTokens:  entry.Tokens.TotalTokens,
			},
			RuntimeSeconds: entry.RuntimeSeconds,
		})
	}

	retrying := make([]Retry, 0, len(snapshot.Retrying))
	for _, entry := range snapshot.Retrying {
		retrying = append(retrying, Retry{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.IssueIdentifier,
			Attempt:         entry.Attempt,
			DueAt:           entry.DueAt,
			Error:           entry.Error,
			WorkspacePath:   entry.WorkspacePath,
		})
	}

	return RuntimeState{
		GeneratedAt: snapshot.GeneratedAt,
		Counts: Counts{
			Running:  snapshot.Counts.Running,
			Retrying: snapshot.Counts.Retrying,
		},
		Running:  running,
		Retrying: retrying,
		CodexTotals: CodexTotals{
			InputTokens:    snapshot.CodexTotals.InputTokens,
			OutputTokens:   snapshot.CodexTotals.OutputTokens,
			TotalTokens:    snapshot.CodexTotals.TotalTokens,
			SecondsRunning: snapshot.CodexTotals.SecondsRunning,
		},
		Polling: Polling{
			Checking:     snapshot.Polling.Checking,
			NextPollAt:   snapshot.Polling.NextPollAt,
			NextPollInMS: snapshot.Polling.NextPollInMS,
			IntervalMS:   snapshot.Polling.IntervalMS,
			LastPollAt:   snapshot.Polling.LastPollAt,
		},
		LastError:  snapshot.LastError,
		RateLimits: snapshot.RateLimits,
	}
}

func FindIssueDetail(state RuntimeState, issueIdentifier string) (IssueDetail, error) {
	if strings.TrimSpace(issueIdentifier) == "" {
		return IssueDetail{}, ErrInvalidIssueIdentifier
	}
	for _, entry := range state.Running {
		if entry.IssueIdentifier == issueIdentifier {
			running := entry
			return IssueDetail{
				IssueID:         entry.IssueID,
				IssueIdentifier: entry.IssueIdentifier,
				Status:          IssueStatusRunning,
				Running:         &running,
			}, nil
		}
	}
	for _, entry := range state.Retrying {
		if entry.IssueIdentifier == issueIdentifier {
			retry := entry
			return IssueDetail{
				IssueID:         entry.IssueID,
				IssueIdentifier: entry.IssueIdentifier,
				Status:          IssueStatusRetrying,
				Retry:           &retry,
			}, nil
		}
	}
	return IssueDetail{}, ErrIssueNotFound
}

func ProjectIssueRunState(state RuntimeState, issueIdentifier string) IssueRunProjection {
	runtimeState := IssueStatusNotRunning
	for _, entry := range state.Running {
		if entry.IssueIdentifier == issueIdentifier {
			runtimeState = IssueStatusRunning
			break
		}
	}
	if runtimeState == IssueStatusNotRunning {
		for _, entry := range state.Retrying {
			if entry.IssueIdentifier == issueIdentifier {
				runtimeState = IssueStatusRetrying
				break
			}
		}
	}
	return IssueRunProjection{
		Boundary:        OrchestratorBoundary("issue_run_projection", "Project issue-run control state from the handwritten orchestrator runtime."),
		IssueIdentifier: issueIdentifier,
		RuntimeState:    runtimeState,
	}
}

func projectIssueFlow(def issueflow.Definition) IssueFlowResult {
	steps := make([]IssueFlowStep, 0, len(def.Steps))
	for _, step := range def.Steps {
		steps = append(steps, IssueFlowStep{
			Name:          step.Name,
			Actor:         step.Actor,
			Purpose:       step.Purpose,
			CoreInterface: step.CoreInterface,
		})
	}
	transitions := make([]IssueFlowTransition, 0, len(def.Transitions))
	for _, transition := range def.Transitions {
		transitions = append(transitions, IssueFlowTransition{
			From:            transition.From,
			To:              transition.To,
			Actor:           transition.Actor,
			CoreInterface:   transition.CoreInterface,
			SuccessSignal:   transition.SuccessSignal,
			FailureHandling: transition.FailureHandling,
		})
	}
	return IssueFlowResult{
		Boundary:      OrchestratorBoundary("issue_flow", "Expose the human-readable trunk issue lifecycle and its core interfaces."),
		Name:          def.Name,
		Purpose:       def.Purpose,
		EntryPoint:    def.EntryPoint,
		Steps:         steps,
		Transitions:   transitions,
		FailurePolicy: append([]string(nil), def.FailurePolicy...),
	}
}

func OrchestratorBoundary(name, purpose string) CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "orchestrator." + name,
		Purpose:            purpose,
		HandwrittenAdapter: "internal/service/control",
	}
}

func ObservabilityBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "observability.snapshot",
		Purpose:            "Expose runtime observability as a stable product control-plane projection.",
		HandwrittenAdapter: "internal/service/control",
	}
}

func RuntimeBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "runtime.settings",
		Purpose:            "Expose non-secret runtime settings resolved by the handwritten runtime configuration layer.",
		HandwrittenAdapter: "internal/service/control",
	}
}

func TrackerBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "tracker.issue",
		Purpose:            "Read and update issue tracker state through the handwritten tracker integration boundary.",
		HandwrittenAdapter: "internal/service/control",
	}
}

func cleanStates(states []string) []string {
	cleaned := make([]string, 0, len(states))
	for _, state := range states {
		if trimmed := strings.TrimSpace(state); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}

func trackerIssueModels(issues []issuemodel.Issue) []TrackerIssue {
	models := make([]TrackerIssue, 0, len(issues))
	for _, issue := range issues {
		models = append(models, trackerIssueModel(issue))
	}
	return models
}

func trackerIssueModel(issue issuemodel.Issue) TrackerIssue {
	blockers := make([]TrackerBlockerRef, 0, len(issue.BlockedBy))
	for _, blocker := range issue.BlockedBy {
		blockers = append(blockers, TrackerBlockerRef{
			IssueID:         blocker.ID,
			IssueIdentifier: blocker.Identifier,
			State:           blocker.State,
		})
	}

	model := TrackerIssue{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		Title:           issue.Title,
		Description:     issue.Description,
		State:           issue.State,
		BranchName:      issue.BranchName,
		URL:             issue.URL,
		Labels:          append([]string(nil), issue.Labels...),
		BlockedBy:       blockers,
	}
	if issue.Priority != nil {
		model.Priority = issue.Priority
	}
	if issue.CreatedAt != nil {
		model.CreatedAt = formatOptionalTime(*issue.CreatedAt)
	}
	if issue.UpdatedAt != nil {
		model.UpdatedAt = formatOptionalTime(*issue.UpdatedAt)
	}
	return model
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

type CapabilityBoundary struct {
	Name               string `json:"name"`
	Purpose            string `json:"purpose"`
	HandwrittenAdapter string `json:"handwritten_adapter"`
}

type ObservabilitySnapshot struct {
	Boundary CapabilityBoundary `json:"boundary"`
	State    RuntimeState       `json:"state"`
}

type IssueFlowResult struct {
	Boundary      CapabilityBoundary    `json:"boundary"`
	Name          string                `json:"name"`
	Purpose       string                `json:"purpose"`
	EntryPoint    string                `json:"entry_point"`
	Steps         []IssueFlowStep       `json:"steps"`
	Transitions   []IssueFlowTransition `json:"transitions"`
	FailurePolicy []string              `json:"failure_policy"`
}

type IssueFlowStep struct {
	Name          string `json:"name"`
	Actor         string `json:"actor"`
	Purpose       string `json:"purpose"`
	CoreInterface string `json:"core_interface"`
}

type IssueFlowTransition struct {
	From            string `json:"from"`
	To              string `json:"to"`
	Actor           string `json:"actor"`
	CoreInterface   string `json:"core_interface"`
	SuccessSignal   string `json:"success_signal"`
	FailureHandling string `json:"failure_handling"`
}

type IssueRunProjection struct {
	Boundary        CapabilityBoundary `json:"boundary"`
	IssueIdentifier string             `json:"issue_identifier"`
	RuntimeState    string             `json:"runtime_state"`
}

type RuntimeSettingsResult struct {
	Boundary CapabilityBoundary `json:"boundary"`
	Settings RuntimeSettings    `json:"settings"`
}

type RuntimeSettings struct {
	TrackerKind           string   `json:"tracker_kind"`
	TrackerProjectSlug    string   `json:"tracker_project_slug"`
	TrackerActiveStates   []string `json:"tracker_active_states"`
	TrackerTerminalStates []string `json:"tracker_terminal_states"`
	ServerPort            int      `json:"server_port"`
	ServerPortSet         bool     `json:"server_port_set"`
	PollingIntervalMS     int      `json:"polling_interval_ms"`
	WorkspaceRoot         string   `json:"workspace_root"`
	MergeTarget           string   `json:"merge_target"`
	MaxConcurrentAgents   int      `json:"max_concurrent_agents"`
	MaxTurns              int      `json:"max_turns"`
	MaxRetryBackoffMS     int      `json:"max_retry_backoff_ms"`
	CodexThreadSandbox    string   `json:"codex_thread_sandbox"`
	CodexTurnTimeoutMS    int      `json:"codex_turn_timeout_ms"`
	CodexReadTimeoutMS    int      `json:"codex_read_timeout_ms"`
	CodexStallTimeoutMS   int      `json:"codex_stall_timeout_ms"`
}

type TrackerIssueList struct {
	Boundary CapabilityBoundary `json:"boundary"`
	Issues   []TrackerIssue     `json:"issues"`
}

type TrackerIssueResult struct {
	Boundary CapabilityBoundary `json:"boundary"`
	Issue    TrackerIssue       `json:"issue"`
}

type TrackerIssueStateInput struct {
	IssueID   string `json:"issue_id"`
	StateName string `json:"state_name"`
}

type TrackerIssueStateResult struct {
	Boundary  CapabilityBoundary `json:"boundary"`
	IssueID   string             `json:"issue_id"`
	StateName string             `json:"state_name"`
	Updated   bool               `json:"updated"`
}

type TrackerIssue struct {
	IssueID         string              `json:"issue_id"`
	IssueIdentifier string              `json:"issue_identifier"`
	Title           string              `json:"title"`
	Description     string              `json:"description"`
	Priority        *int                `json:"priority,omitempty"`
	State           string              `json:"state"`
	BranchName      string              `json:"branch_name"`
	URL             string              `json:"url"`
	Labels          []string            `json:"labels"`
	BlockedBy       []TrackerBlockerRef `json:"blocked_by"`
	CreatedAt       string              `json:"created_at,omitempty"`
	UpdatedAt       string              `json:"updated_at,omitempty"`
}

type TrackerBlockerRef struct {
	IssueID         string `json:"issue_id"`
	IssueIdentifier string `json:"issue_identifier"`
	State           string `json:"state"`
}

func WorkspaceBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "workspace.lifecycle",
		Purpose:            "Resolve, validate, prepare, and clean up issue workspaces through the handwritten workspace manager.",
		HandwrittenAdapter: "internal/service/control",
	}
}

func workspacePreparation(path string, contained bool) WorkspacePreparation {
	return WorkspacePreparation{
		Boundary:        WorkspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: contained,
	}
}

type WorkspacePreparation struct {
	Boundary        CapabilityBoundary `json:"boundary"`
	WorkspacePath   string             `json:"workspace_path"`
	ContainedInRoot bool               `json:"contained_in_root"`
}

type WorkspacePathValidation struct {
	Boundary        CapabilityBoundary `json:"boundary"`
	WorkspacePath   string             `json:"workspace_path"`
	ContainedInRoot bool               `json:"contained_in_root"`
}

type WorkspaceCleanupResult struct {
	Boundary        CapabilityBoundary `json:"boundary"`
	WorkspacePath   string             `json:"workspace_path"`
	Removed         bool               `json:"removed"`
	ContainedInRoot bool               `json:"contained_in_root"`
}

func WorkflowBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "workflow.load_render",
		Purpose:            "Load workflow configuration and render prompts through the handwritten workflow package.",
		HandwrittenAdapter: "internal/service/control",
	}
}

type WorkflowRenderInput struct {
	WorkflowPath     string `json:"workflow_path"`
	IssueIdentifier  string `json:"issue_identifier"`
	IssueTitle       string `json:"issue_title"`
	IssueDescription string `json:"issue_description"`
	HasAttempt       bool   `json:"has_attempt"`
	Attempt          int    `json:"attempt"`
}

type WorkflowSummary struct {
	Boundary     CapabilityBoundary `json:"boundary"`
	WorkflowPath string             `json:"workflow_path"`
	StateNames   []string           `json:"state_names"`
}

type WorkflowRenderResult struct {
	Boundary CapabilityBoundary `json:"boundary"`
	Prompt   string             `json:"prompt"`
}

func CodexSessionBoundary() CapabilityBoundary {
	return CapabilityBoundary{
		Name:               "codex_session.turn",
		Purpose:            "Run a single Codex turn through the handwritten Codex runner without exposing app-server protocol details.",
		HandwrittenAdapter: "internal/service/control",
	}
}

type CodexTurnInput struct {
	IssueIdentifier string `json:"issue_identifier"`
	PromptName      string `json:"prompt_name"`
	WorkspacePath   string `json:"workspace_path"`
	PromptText      string `json:"prompt_text"`
}

type CodexTurnSummary struct {
	Boundary  CapabilityBoundary `json:"boundary"`
	SessionID string             `json:"session_id"`
	TurnCount int32              `json:"turn_count"`
}

type RuntimeState struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Counts      Counts      `json:"counts"`
	Running     []IssueRun  `json:"running"`
	Retrying    []Retry     `json:"retrying"`
	CodexTotals CodexTotals `json:"codex_totals"`
	Polling     Polling     `json:"polling"`
	LastError   string      `json:"last_error,omitempty"`
	RateLimits  any         `json:"rate_limits,omitempty"`
}

type IssueDetail struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	Status          string    `json:"status"`
	Running         *IssueRun `json:"running,omitempty"`
	Retry           *Retry    `json:"retry,omitempty"`
}

type RefreshResult struct {
	Accepted bool   `json:"accepted"`
	Status   string `json:"status"`
}

type Counts struct {
	Running  int `json:"running"`
	Retrying int `json:"retrying"`
}

type IssueRun struct {
	IssueID         string     `json:"issue_id"`
	IssueIdentifier string     `json:"issue_identifier"`
	State           string     `json:"state"`
	AgentPhase      string     `json:"agent_phase,omitempty"`
	Stage           string     `json:"stage,omitempty"`
	WorkspacePath   string     `json:"workspace_path,omitempty"`
	Attempt         int        `json:"attempt,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
	ThreadID        string     `json:"thread_id,omitempty"`
	TurnID          string     `json:"turn_id,omitempty"`
	PID             int        `json:"pid,omitempty"`
	TurnCount       int        `json:"turn_count"`
	LastEvent       string     `json:"last_event,omitempty"`
	LastMessage     string     `json:"last_message,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	LastEventAt     time.Time  `json:"last_event_at,omitempty"`
	Tokens          TokenUsage `json:"tokens"`
	RuntimeSeconds  float64    `json:"runtime_seconds"`
}

type Retry struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	Attempt         int       `json:"attempt"`
	DueAt           time.Time `json:"due_at"`
	Error           string    `json:"error,omitempty"`
	WorkspacePath   string    `json:"workspace_path,omitempty"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type CodexTotals struct {
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	SecondsRunning float64 `json:"seconds_running"`
}

type Polling struct {
	Checking     bool      `json:"checking"`
	NextPollAt   time.Time `json:"next_poll_at,omitempty"`
	NextPollInMS int64     `json:"next_poll_in_ms"`
	IntervalMS   int       `json:"interval_ms"`
	LastPollAt   time.Time `json:"last_poll_at,omitempty"`
}
