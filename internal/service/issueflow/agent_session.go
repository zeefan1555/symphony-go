package issueflow

import (
	"context"
	"errors"
	"fmt"

	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

func runAgentTurn(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt int, phase AgentPhase, stage RunStage, promptText string, continuation bool, turn int) (TurnResult, error) {
	switch phase {
	case PhaseImplementer, PhaseReviewer:
	default:
		return TurnResult{}, fmt.Errorf("unknown agent phase %q", phase)
	}
	setRunningStage(rt, issue, attempt, phase, StagePreparingWorkspace, "preparing workspace", "", 1)
	hookCtx := workspace.WithHookIssue(ctx, issue)
	workspacePath, _, err := rt.Workspace.Ensure(hookCtx, issue)
	if err != nil {
		removeRunning(rt, issue.ID)
		return TurnResult{}, err
	}
	defer func() {
		if err := rt.Workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			logIssue(rt, issue, "after_run_hook_failed", err.Error(), nil)
		}
	}()
	setRunningStage(rt, issue, attempt, phase, StageRunningWorkspaceHooks, "running before_run hook", workspacePath, 1)
	if err := rt.Workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		removeRunning(rt, issue.ID)
		return TurnResult{}, err
	}
	var renderAttempt *int
	if attempt > 0 {
		value := attempt
		renderAttempt = &value
	}
	if promptText == "" {
		setRunningStage(rt, issue, attempt, phase, StageRenderingPrompt, "rendering workflow prompt", workspacePath, turn)
		promptText, err = workflow.Render(rt.Workflow.PromptTemplate, issue, renderAttempt)
		if err != nil {
			return TurnResult{}, err
		}
	}
	setRunningStage(rt, issue, attempt, phase, stage, stageMessage(stage), workspacePath, turn)
	lastAgentMessage := ""
	request := codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts: []codex.TurnPrompt{{
			Text:         promptText,
			Continuation: continuation,
			Attempt:      renderAttempt,
			Issue:        &issue,
		}},
	}
	sessionResult, err := rt.Runner.RunSession(ctx, request, func(event codex.Event) {
		if text := CompletedAgentMessageText(event); text != "" {
			lastAgentMessage = text
		}
		updateRunningFromEvent(rt, issue.ID, event)
		logIssue(rt, issue, "codex_event", event.Name, event.Payload)
	})
	removeRunning(rt, issue.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return TurnResult{}, err
		}
		return TurnResult{}, err
	}
	if len(sessionResult.Turns) == 0 {
		return TurnResult{Issue: issue, LastAgentMessage: lastAgentMessage}, nil
	}
	last := sessionResult.Turns[len(sessionResult.Turns)-1]
	logIssue(rt, issue, "turn_completed", "Codex turn completed", map[string]any{"session_id": last.SessionID})
	refreshed, err := rt.Tracker.FetchIssue(ctx, issue.ID)
	if err != nil {
		return TurnResult{}, err
	}
	return TurnResult{Issue: refreshed, LastAgentMessage: lastAgentMessage}, nil
}

func stageMessage(stage RunStage) string {
	switch stage {
	case StageContinuingImplementation:
		return "continuing implementation"
	case StageContinuingAIReview:
		return "continuing in AI Review"
	case StageContinuingMerging:
		return "continuing in Merging"
	default:
		return "running Codex turn"
	}
}
