package hertzserver

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	controlplane "github.com/zeefan1555/symphony-go/internal/control"
	controlhttp "github.com/zeefan1555/symphony-go/internal/control/hertzgen/handler/control/http"
	controlmodel "github.com/zeefan1555/symphony-go/internal/control/hertzgen/model/control/model"
	"github.com/zeefan1555/symphony-go/internal/control/hertzgen/router"
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
	restore := controlhttp.SetControlService(controlAdapter{control: s.control})
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

func (a controlAdapter) GetScaffold(ctx context.Context) (controlhttp.ScaffoldStatus, error) {
	status, err := a.control.GetScaffold(ctx)
	if err != nil {
		return controlhttp.ScaffoldStatus{}, err
	}
	return controlhttp.ScaffoldStatus{Status: status.Status}, nil
}

func (a controlAdapter) GetState(ctx context.Context) (*controlmodel.RuntimeState, error) {
	state, err := a.control.RuntimeState(ctx)
	if err != nil {
		return nil, err
	}
	return runtimeStateModel(state), nil
}

func runtimeStateModel(state controlplane.RuntimeState) *controlmodel.RuntimeState {
	running := make([]*controlmodel.IssueRun, 0, len(state.Running))
	for _, entry := range state.Running {
		modelEntry := &controlmodel.IssueRun{
			IssueID:         entry.IssueID,
			IssueIdentifier: entry.IssueIdentifier,
			State:           entry.State,
			TurnCount:       int32(entry.TurnCount),
			Tokens: &controlmodel.TokenUsage{
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
		running = append(running, modelEntry)
	}

	retrying := make([]*controlmodel.RetryRun, 0, len(state.Retrying))
	for _, entry := range state.Retrying {
		modelEntry := &controlmodel.RetryRun{
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
		retrying = append(retrying, modelEntry)
	}

	modelState := &controlmodel.RuntimeState{
		GeneratedAt: formatControlTime(state.GeneratedAt),
		Counts: &controlmodel.RuntimeCounts{
			Running:  int32(state.Counts.Running),
			Retrying: int32(state.Counts.Retrying),
		},
		Running:  running,
		Retrying: retrying,
		CodexTotals: &controlmodel.CodexTotals{
			InputTokens:    int32(state.CodexTotals.InputTokens),
			OutputTokens:   int32(state.CodexTotals.OutputTokens),
			TotalTokens:    int32(state.CodexTotals.TotalTokens),
			SecondsRunning: state.CodexTotals.SecondsRunning,
		},
		Polling: &controlmodel.PollingStatus{
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

func formatControlTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func stringPtr(value string) *string {
	return &value
}
