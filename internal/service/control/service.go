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

type CodexSessionRunner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}

type ControlService interface {
	RuntimeState(context.Context) (RuntimeState, error)
	IssueDetail(context.Context, string) (IssueDetail, error)
	ProjectIssueRun(context.Context, string) (IssueRunProjection, error)
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
}

func NewService(provider SnapshotProvider) *Service {
	return &Service{provider: provider}
}

func NewServiceWithWorkspace(provider SnapshotProvider, manager *workspace.Manager) *Service {
	return &Service{provider: provider, workspace: manager}
}

func NewServiceWithCodexRunner(provider SnapshotProvider, runner CodexSessionRunner) *Service {
	return &Service{provider: provider, runner: runner}
}

func NewServiceWithWorkspaceAndCodexRunner(provider SnapshotProvider, manager *workspace.Manager, runner CodexSessionRunner) *Service {
	return &Service{provider: provider, workspace: manager, runner: runner}
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

func (s *Service) ResolveWorkspacePath(ctx context.Context, issueIdentifier string) (WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePreparation{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return WorkspacePreparation{}, ErrInvalidIssueIdentifier
	}
	if s == nil || s.workspace == nil {
		return WorkspacePreparation{}, ErrWorkspaceManagerRequired
	}
	path, err := s.workspace.PathForIssue(issuemodel.Issue{Identifier: issueIdentifier})
	if err != nil {
		return WorkspacePreparation{}, err
	}
	return workspacePreparation(path, s.workspace.ValidateWorkspacePath(path) == nil), nil
}

func (s *Service) ValidateWorkspacePath(ctx context.Context, path string) (WorkspacePathValidation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePathValidation{}, err
	}
	if s == nil || s.workspace == nil {
		return WorkspacePathValidation{}, ErrWorkspaceManagerRequired
	}
	return WorkspacePathValidation{
		Boundary:        WorkspaceBoundary(),
		WorkspacePath:   path,
		ContainedInRoot: s.workspace.ValidateWorkspacePath(path) == nil,
	}, nil
}

func (s *Service) PrepareWorkspace(ctx context.Context, issueIdentifier string) (WorkspacePreparation, error) {
	if err := ctx.Err(); err != nil {
		return WorkspacePreparation{}, err
	}
	if strings.TrimSpace(issueIdentifier) == "" {
		return WorkspacePreparation{}, ErrInvalidIssueIdentifier
	}
	if s == nil || s.workspace == nil {
		return WorkspacePreparation{}, ErrWorkspaceManagerRequired
	}
	path, _, err := s.workspace.Ensure(ctx, issuemodel.Issue{Identifier: issueIdentifier})
	if err != nil {
		return WorkspacePreparation{}, err
	}
	return workspacePreparation(path, s.workspace.ValidateWorkspacePath(path) == nil), nil
}

func (s *Service) CleanupWorkspace(ctx context.Context, path string) (WorkspaceCleanupResult, error) {
	if err := ctx.Err(); err != nil {
		return WorkspaceCleanupResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return WorkspaceCleanupResult{}, ErrInvalidWorkspacePath
	}
	if s == nil || s.workspace == nil {
		return WorkspaceCleanupResult{}, ErrWorkspaceManagerRequired
	}
	if err := s.workspace.ValidateWorkspacePath(path); err != nil {
		return WorkspaceCleanupResult{}, fmt.Errorf("%w: %v", ErrInvalidWorkspacePath, err)
	}
	if err := s.workspace.Remove(ctx, path); err != nil {
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
		IssueFlow:    workflowIssueFlow(loaded.Config),
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
	if s == nil || s.runner == nil {
		return CodexTurnSummary{}, ErrCodexRunnerRequired
	}
	result, err := s.runner.RunSession(ctx, codex.SessionRequest{
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
			WorkspacePath:   entry.WorkspacePath,
			SessionID:       entry.SessionID,
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
		LastError: snapshot.LastError,
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
		Boundary: CapabilityBoundary{
			Name:               "orchestrator.issue_run_projection",
			Purpose:            "Project issue-run control state from the handwritten orchestrator runtime.",
			HandwrittenAdapter: "internal/service/control",
		},
		IssueIdentifier: issueIdentifier,
		RuntimeState:    runtimeState,
	}
}

type CapabilityBoundary struct {
	Name               string `json:"name"`
	Purpose            string `json:"purpose"`
	HandwrittenAdapter string `json:"handwritten_adapter"`
}

type IssueRunProjection struct {
	Boundary        CapabilityBoundary `json:"boundary"`
	IssueIdentifier string             `json:"issue_identifier"`
	RuntimeState    string             `json:"runtime_state"`
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
	IssueFlow    WorkflowIssueFlow  `json:"issue_flow"`
}

type WorkflowIssueFlow struct {
	ActiveStates   []string                  `json:"active_states"`
	TerminalStates []string                  `json:"terminal_states"`
	ReviewPolicy   WorkflowReviewPolicy      `json:"review_policy"`
	PhaseRoutes    []WorkflowPhaseRoute      `json:"phase_routes"`
	Transitions    []WorkflowStateTransition `json:"transitions"`
	DispatchRules  []WorkflowDispatchRule    `json:"dispatch_rules"`
	SingleSession  bool                      `json:"single_agent_session"`
	StageFlows     []WorkflowStageFlow       `json:"stage_flows"`
}

type WorkflowReviewPolicy struct {
	Mode                string `json:"mode"`
	AllowManualAIReview bool   `json:"allow_manual_ai_review"`
	OnAIFail            string `json:"on_ai_fail"`
}

type WorkflowPhaseRoute struct {
	State    string `json:"state"`
	Phase    string `json:"phase"`
	Behavior string `json:"behavior"`
}

type WorkflowStateTransition struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Owner     string `json:"owner"`
	Trigger   string `json:"trigger"`
	Condition string `json:"condition"`
}

type WorkflowDispatchRule struct {
	State    string `json:"state"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

type WorkflowStageFlow struct {
	State          string   `json:"state"`
	Stage          string   `json:"stage"`
	SessionPolicy  string   `json:"session_policy"`
	EntryCondition string   `json:"entry_condition"`
	Action         string   `json:"action"`
	ExitCondition  string   `json:"exit_condition"`
	NextStates     []string `json:"next_states"`
}

type WorkflowRenderResult struct {
	Boundary CapabilityBoundary `json:"boundary"`
	Prompt   string             `json:"prompt"`
}

func workflowIssueFlow(cfg runtimeconfig.Config) WorkflowIssueFlow {
	policy := workflowReviewPolicy(cfg.Agent)
	return WorkflowIssueFlow{
		ActiveStates:   append([]string(nil), cfg.Tracker.ActiveStates...),
		TerminalStates: append([]string(nil), cfg.Tracker.TerminalStates...),
		ReviewPolicy:   policy,
		PhaseRoutes:    workflowPhaseRoutes(cfg.Tracker.ActiveStates, cfg.Tracker.TerminalStates, policy),
		Transitions:    workflowStateTransitions(policy),
		DispatchRules:  workflowDispatchRules(),
		SingleSession:  true,
		StageFlows:     workflowStageFlows(),
	}
}

func workflowReviewPolicy(agent runtimeconfig.AgentConfig) WorkflowReviewPolicy {
	mode := strings.ToLower(strings.TrimSpace(agent.ReviewPolicy.Mode))
	if mode == "" {
		mode = "human"
		if agent.AIReview.Enabled {
			mode = "ai"
			if agent.AIReview.AutoMerge {
				mode = "auto"
			}
		}
	}
	onFail := strings.ToLower(strings.TrimSpace(agent.ReviewPolicy.OnAIFail))
	if onFail == "" && agent.AIReview.ReworkOnFailure {
		onFail = "rework"
	}
	if onFail == "" {
		onFail = "unspecified"
	}
	return WorkflowReviewPolicy{
		Mode:                mode,
		AllowManualAIReview: agent.ReviewPolicy.AllowManualAIReview,
		OnAIFail:            onFail,
	}
}

func workflowPhaseRoutes(activeStates, terminalStates []string, policy WorkflowReviewPolicy) []WorkflowPhaseRoute {
	routes := make([]WorkflowPhaseRoute, 0, len(activeStates)+len(terminalStates))
	reviewEnabled := policy.Mode == "ai" || policy.Mode == "auto" || policy.AllowManualAIReview
	for _, state := range activeStates {
		switch strings.ToLower(state) {
		case "todo":
			routes = append(routes, WorkflowPhaseRoute{
				State:    state,
				Phase:    "implementation",
				Behavior: "orchestrator moves Todo to In Progress before starting the same issue session",
			})
		case "human review", "in review":
			routes = append(routes, WorkflowPhaseRoute{
				State:    state,
				Phase:    "hold",
				Behavior: "orchestrator waits for a human to resolve the external blocker",
			})
		case "ai review":
			behavior := "orchestrator waits because review policy does not allow AI Review dispatch"
			if reviewEnabled {
				behavior = "the same issue session continues into the review stage"
			}
			routes = append(routes, WorkflowPhaseRoute{State: state, Phase: "review", Behavior: behavior})
		case "merging":
			routes = append(routes, WorkflowPhaseRoute{
				State:    state,
				Phase:    "merge",
				Behavior: "the same issue session continues into the PR merge protocol",
			})
		default:
			routes = append(routes, WorkflowPhaseRoute{
				State:    state,
				Phase:    "implementation",
				Behavior: "the same issue session starts or continues implementation",
			})
		}
	}
	for _, state := range terminalStates {
		routes = append(routes, WorkflowPhaseRoute{
			State:    state,
			Phase:    "terminal",
			Behavior: "orchestrator does not run an agent and cleans up the issue workspace",
		})
	}
	return routes
}

func workflowStateTransitions(policy WorkflowReviewPolicy) []WorkflowStateTransition {
	reviewCondition := "review_policy.mode is ai or auto"
	if policy.AllowManualAIReview {
		reviewCondition = "review_policy allows manual AI Review dispatch"
	}
	return []WorkflowStateTransition{
		{
			FromState: "Todo",
			ToState:   "In Progress",
			Owner:     "orchestrator",
			Trigger:   "dispatch start",
			Condition: "issue is active, unblocked, unclaimed, and an orchestrator slot is available",
		},
		{
			FromState: "In Progress",
			ToState:   "AI Review",
			Owner:     "same issue agent",
			Trigger:   "implementation complete",
			Condition: reviewCondition,
		},
		{
			FromState: "In Progress",
			ToState:   "Human Review",
			Owner:     "same issue agent",
			Trigger:   "external blocker",
			Condition: "required auth, permission, secret, or external decision is missing",
		},
		{
			FromState: "AI Review",
			ToState:   "Merging",
			Owner:     "same issue agent or orchestrator",
			Trigger:   "review passes",
			Condition: "review stage final message starts with a pass marker or the agent updates the tracker state",
		},
		{
			FromState: "AI Review",
			ToState:   "Rework",
			Owner:     "same issue agent",
			Trigger:   "review fails",
			Condition: "review findings require code, test, or documentation changes",
		},
		{
			FromState: "AI Review",
			ToState:   "Human Review",
			Owner:     "same issue agent",
			Trigger:   "external blocker",
			Condition: "review cannot continue without human action",
		},
		{
			FromState: "Merging",
			ToState:   "Done",
			Owner:     "same issue agent",
			Trigger:   "PR merge protocol completes",
			Condition: "PR is merged, root checkout is synced, and required evidence is recorded",
		},
		{
			FromState: "Rework",
			ToState:   "AI Review",
			Owner:     "same issue agent",
			Trigger:   "rework complete",
			Condition: "review findings are addressed and validation is recorded",
		},
		{
			FromState: "any active state",
			ToState:   "terminal state",
			Owner:     "agent or issue tracker",
			Trigger:   "issue leaves active workflow",
			Condition: "state is listed in tracker.terminal_states; orchestrator cleans up the workspace",
		},
	}
}

func workflowDispatchRules() []WorkflowDispatchRule {
	return []WorkflowDispatchRule{
		{State: "any active state", Decision: "dispatch", Reason: "required issue fields exist, state is active, state is not terminal, issue is unclaimed, issue is not already running, and slots are available"},
		{State: "Todo", Decision: "skip", Reason: "any blocking issue is not in a terminal state"},
		{State: "Human Review", Decision: "hold", Reason: "state is reserved for real external blockers and waits for human action"},
		{State: "In Review", Decision: "hold", Reason: "legacy/manual review hold state waits for human action"},
		{State: "AI Review", Decision: "dispatch or hold", Reason: "dispatch only when review policy mode is ai/auto or manual AI Review dispatch is allowed"},
		{State: "terminal state", Decision: "cleanup", Reason: "terminal issues are not dispatched; startup and worker-exit cleanup remove their workspaces"},
	}
}

func workflowStageFlows() []WorkflowStageFlow {
	return []WorkflowStageFlow{
		{
			State:          "Todo",
			Stage:          "queue",
			SessionPolicy:  "no Codex session until dispatch",
			EntryCondition: "issue is active and all blockers are terminal",
			Action:         "orchestrator moves the issue to In Progress before starting work",
			ExitCondition:  "state update succeeds",
			NextStates:     []string{"In Progress"},
		},
		{
			State:          "In Progress",
			Stage:          "implementation",
			SessionPolicy:  "same issue agent session",
			EntryCondition: "issue is dispatched or continued after Rework",
			Action:         "implement acceptance criteria, update workpad, commit changes, and move to AI Review when ready",
			ExitCondition:  "acceptance criteria and validation are recorded, or a real external blocker is found",
			NextStates:     []string{"AI Review", "Human Review"},
		},
		{
			State:          "AI Review",
			Stage:          "review",
			SessionPolicy:  "same issue agent session",
			EntryCondition: "implementation or rework has moved the issue to AI Review",
			Action:         "review workpad, diff, commit range, and validation evidence in the existing session context",
			ExitCondition:  "review passes, review findings require rework, or a real external blocker is found",
			NextStates:     []string{"Merging", "Rework", "Human Review"},
		},
		{
			State:          "Rework",
			Stage:          "rework",
			SessionPolicy:  "same issue agent session",
			EntryCondition: "AI Review produced concrete findings",
			Action:         "address findings, rerun relevant validation, update workpad, and return to AI Review",
			ExitCondition:  "findings are resolved and validation evidence is recorded",
			NextStates:     []string{"AI Review", "Human Review"},
		},
		{
			State:          "Merging",
			Stage:          "merge",
			SessionPolicy:  "same issue agent session",
			EntryCondition: "AI Review passed",
			Action:         "run the PR merge protocol, sync the root checkout, record evidence, and move to Done",
			ExitCondition:  "merge protocol completes or a real external blocker is found",
			NextStates:     []string{"Done", "Human Review"},
		},
		{
			State:          "Human Review",
			Stage:          "hold",
			SessionPolicy:  "session stops until human action resolves the blocker",
			EntryCondition: "agent cannot continue without external auth, permission, secret, tool, or business decision",
			Action:         "record a blocker brief and wait",
			ExitCondition:  "human moves the issue back to an active executable state",
			NextStates:     []string{"Todo", "In Progress", "AI Review", "Rework", "Merging"},
		},
		{
			State:          "Done",
			Stage:          "terminal",
			SessionPolicy:  "no Codex session",
			EntryCondition: "issue is in a terminal tracker state",
			Action:         "orchestrator skips agent work and cleans up the workspace",
			ExitCondition:  "workspace cleanup is attempted",
			NextStates:     nil,
		},
	}
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
	WorkspacePath   string     `json:"workspace_path,omitempty"`
	SessionID       string     `json:"session_id,omitempty"`
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
