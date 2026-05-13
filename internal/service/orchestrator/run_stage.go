package orchestrator

import (
	"context"
	"time"

	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/runtime/telemetry"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/issueflow"
)

func (o *Orchestrator) SetRunningStage(issue issuemodel.Issue, attempt int, phase issueflow.AgentPhase, stage issueflow.RunStage, message, workspacePath string, turnCount int) {
	o.setRunningStage(issue, attempt, phase, stage, message, workspacePath, turnCount)
}

func (o *Orchestrator) setRunningStage(issue issuemodel.Issue, attempt int, phase issueflow.AgentPhase, stage issueflow.RunStage, message, workspacePath string, turnCount int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.setRunningStageLocked(issue, attempt, phase, stage, message, workspacePath, turnCount)
}

func (o *Orchestrator) setRunningStageLocked(issue issuemodel.Issue, attempt int, phase issueflow.AgentPhase, stage issueflow.RunStage, message, workspacePath string, turnCount int) {
	now := time.Now()
	if turnCount <= 0 {
		turnCount = 1
	}
	entry := observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		AgentPhase:      string(phase),
		Stage:           string(stage),
		WorkspacePath:   workspacePath,
		Attempt:         attempt,
		TurnCount:       turnCount,
		StartedAt:       now,
		LastEvent:       string(stage),
		LastMessage:     message,
		LastEventAt:     now,
	}
	if entry.LastMessage == "" {
		entry.LastMessage = string(stage)
	}

	for i, existing := range o.snapshot.Running {
		if existing.IssueID != issue.ID {
			continue
		}
		if !existing.StartedAt.IsZero() {
			entry.StartedAt = existing.StartedAt
		}
		if entry.AgentPhase == "" {
			entry.AgentPhase = existing.AgentPhase
		}
		if entry.Stage == "" {
			entry.Stage = existing.Stage
			entry.LastEvent = existing.LastEvent
		}
		if entry.WorkspacePath == "" {
			entry.WorkspacePath = existing.WorkspacePath
		}
		entry.SessionID = existing.SessionID
		entry.ThreadID = existing.ThreadID
		entry.TurnID = existing.TurnID
		entry.PID = existing.PID
		entry.Tokens = existing.Tokens
		entry.RuntimeSeconds = existing.RuntimeSeconds
		o.snapshot.Running[i] = entry
		return
	}
	o.snapshot.Running = append(o.snapshot.Running, entry)
	telemetry.RecordIssueActive(context.Background(), o.opts.Telemetry, 1, runningMetricFields(entry))
}
