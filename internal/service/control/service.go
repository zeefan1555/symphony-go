package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zeefan1555/symphony-go/internal/runtime/observability"
	corecodex "github.com/zeefan1555/symphony-go/internal/service/codex"
	issuemodel "github.com/zeefan1555/symphony-go/internal/service/issue"
	coreworkflow "github.com/zeefan1555/symphony-go/internal/service/workflow"
	coreworkspace "github.com/zeefan1555/symphony-go/internal/service/workspace"
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
	RunSession(context.Context, corecodex.SessionRequest, func(corecodex.Event)) (corecodex.SessionResult, error)
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
	workspace *coreworkspace.Manager
	runner    CodexSessionRunner
}

func NewService(provider SnapshotProvider) *Service {
	return &Service{provider: provider}
}

func NewServiceWithWorkspace(provider SnapshotProvider, manager *coreworkspace.Manager) *Service {
	return &Service{provider: provider, workspace: manager}
}

func NewServiceWithCodexRunner(provider SnapshotProvider, runner CodexSessionRunner) *Service {
	return &Service{provider: provider, runner: runner}
}

func NewServiceWithWorkspaceAndCodexRunner(provider SnapshotProvider, manager *coreworkspace.Manager, runner CodexSessionRunner) *Service {
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
	loaded, err := coreworkflow.Load(path)
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
	loaded, err := coreworkflow.Load(input.WorkflowPath)
	if err != nil {
		return WorkflowRenderResult{}, err
	}
	var attempt *int
	if input.HasAttempt {
		value := input.Attempt
		attempt = &value
	}
	prompt, err := coreworkflow.Render(loaded.PromptTemplate, issuemodel.Issue{
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
	result, err := s.runner.RunSession(ctx, corecodex.SessionRequest{
		WorkspacePath: input.WorkspacePath,
		Issue:         issuemodel.Issue{Identifier: input.IssueIdentifier},
		Prompts: []corecodex.TurnPrompt{{
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
			HandwrittenAdapter: "internal/service/orchestrator/scaffold",
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
		HandwrittenAdapter: "internal/service/workspace/scaffold",
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
		HandwrittenAdapter: "internal/service/workflow/scaffold",
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
		HandwrittenAdapter: "internal/service/codex/scaffold",
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
