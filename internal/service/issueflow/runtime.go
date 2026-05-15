package issueflow

import (
	"context"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/runtime/logging"
	"symphony-go/internal/runtime/sessionstore"
	"symphony-go/internal/runtime/telemetry"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workspace"
)

type Tracker interface {
	FetchIssue(context.Context, string) (issuemodel.Issue, error)
	UpdateIssueState(context.Context, string, string) error
}

type Runner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}

type SessionStore interface {
	Get(context.Context, string) (sessionstore.Record, error)
	Upsert(context.Context, sessionstore.Record) error
	Delete(context.Context, string) error
}

type Observer interface {
	SetRunningStage(issue issuemodel.Issue, attempt int, phase AgentPhase, stage RunStage, message, workspacePath string, turnCount int)
	RemoveRunning(issueID string)
	LogIssue(ctx context.Context, issue issuemodel.Issue, event, message string, fields map[string]any)
	UpdateRunningFromEvent(issueID string, event codex.Event)
}

type Runtime struct {
	Workflow  *runtimeconfig.Workflow
	Tracker   Tracker
	Workspace *workspace.Manager
	Runner    Runner
	Sessions  SessionStore
	Observer  Observer
	Telemetry telemetry.Facade
}

func setRunningStage(rt Runtime, issue issuemodel.Issue, attempt int, phase AgentPhase, stage RunStage, message, workspacePath string, turnCount int) {
	if rt.Observer != nil {
		rt.Observer.SetRunningStage(issue, attempt, phase, stage, message, workspacePath, turnCount)
	}
}

func removeRunning(rt Runtime, issueID string) {
	if rt.Observer != nil {
		rt.Observer.RemoveRunning(issueID)
	}
}

func logIssue(ctx context.Context, rt Runtime, issue issuemodel.Issue, event, message string, fields map[string]any) {
	if rt.Observer != nil {
		fields = mergeLogFields(fields, logging.SourceFields(1))
		rt.Observer.LogIssue(ctx, issue, event, message, fields)
	}
}

func mergeLogFields(fields map[string]any, extra map[string]any) map[string]any {
	for key, value := range extra {
		if value == nil || value == "" {
			continue
		}
		if fields == nil {
			fields = map[string]any{}
		}
		if _, ok := fields[key]; !ok {
			fields[key] = value
		}
	}
	return fields
}

func updateRunningFromEvent(rt Runtime, issueID string, event codex.Event) {
	if rt.Observer != nil {
		rt.Observer.UpdateRunningFromEvent(issueID, event)
	}
}
