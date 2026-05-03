package control

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/zeefan1555/symphony-go/internal/observability"
)

var (
	ErrSnapshotProviderRequired = errors.New("control snapshot provider required")
	ErrInvalidIssueIdentifier   = errors.New("issue identifier is required")
	ErrIssueNotFound            = errors.New("issue not found")
	ErrRefreshTriggerRequired   = errors.New("control refresh trigger required")
)

const (
	IssueStatusRunning  = "running"
	IssueStatusRetrying = "retrying"

	RefreshStatusQueued         = "queued"
	RefreshStatusAlreadyPending = "already_pending"
)

type SnapshotProvider interface {
	Snapshot() observability.Snapshot
}

type RefreshTrigger interface {
	RequestRefresh(context.Context) (bool, error)
}

type ControlService interface {
	GetScaffold(context.Context) (ScaffoldStatus, error)
	RuntimeState(context.Context) (RuntimeState, error)
	IssueDetail(context.Context, string) (IssueDetail, error)
	Refresh(context.Context) (RefreshResult, error)
}

type Service struct {
	provider SnapshotProvider
}

func NewService(provider SnapshotProvider) *Service {
	return &Service{provider: provider}
}

func (s *Service) GetScaffold(ctx context.Context) (ScaffoldStatus, error) {
	if err := ctx.Err(); err != nil {
		return ScaffoldStatus{}, err
	}
	if s == nil || s.provider == nil {
		return ScaffoldStatus{Status: "unconfigured"}, nil
	}
	return ScaffoldStatus{Status: "ok"}, nil
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

type ScaffoldStatus struct {
	Status string `json:"status"`
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
