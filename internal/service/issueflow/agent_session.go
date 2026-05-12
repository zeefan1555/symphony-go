package issueflow

import (
	"context"
	"fmt"

	"symphony-go/internal/runtime/telemetry"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

func runWorkerAttempt(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt int) (Result, error) {
	phase := nextWorkerPhase(issue)
	switch phase {
	case PhaseImplementer, PhaseReviewer:
	default:
		return Result{}, fmt.Errorf("unknown agent phase %q", phase)
	}
	setRunningStage(rt, issue, attempt, phase, StagePreparingWorkspace, "preparing workspace", "", 1)
	hookCtx := workspace.WithHookIssue(ctx, issue)
	stepCtx, endStep := telemetry.StartStep(ctx, rt.Telemetry, string(phase), "workspace_prepared", issueFields(issue))
	workspacePath, _, err := rt.Workspace.Ensure(hookCtx, issue)
	if err != nil {
		endStep("error", err)
		removeRunning(rt, issue.ID)
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	endStep("success", nil)
	defer func() {
		_, endStep := telemetry.StartStep(stepCtx, rt.Telemetry, string(phase), "after_run_hook", issueFields(issue))
		if err := rt.Workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			endStep("error", err)
			logIssue(stepCtx, rt, issue, "after_run_hook_failed", err.Error(), nil)
			return
		}
		endStep("success", nil)
	}()
	setRunningStage(rt, issue, attempt, phase, StageRunningWorkspaceHooks, "running before_run hook", workspacePath, 1)
	_, endStep = telemetry.StartStep(ctx, rt.Telemetry, string(phase), "before_run_hook", issueFields(issue))
	if err := rt.Workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		endStep("error", err)
		removeRunning(rt, issue.ID)
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	endStep("success", nil)
	var renderAttempt *int
	if attempt > 0 {
		value := attempt
		renderAttempt = &value
	}
	setRunningStage(rt, issue, attempt, phase, StageRenderingPrompt, "rendering workflow prompt", workspacePath, 1)
	_, endStep = telemetry.StartStep(ctx, rt.Telemetry, string(phase), "prompt_rendered", issueFields(issue))
	promptText, err := workflow.Render(rt.Workflow.PromptTemplate, issue, renderAttempt)
	if err != nil {
		endStep("error", err)
		removeRunning(rt, issue.ID)
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	endStep("success", nil)
	setRunningStage(rt, issue, attempt, phase, nextWorkerStage(issue, 1), stageMessage(nextWorkerStage(issue, 1)), workspacePath, 1)
	lastAgentMessage := ""
	attemptResult := Result{Outcome: OutcomeRetryContinuation}
	currentIssue := issue
	request := codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts: []codex.TurnPrompt{{
			Text:         promptText,
			Continuation: false,
			Attempt:      renderAttempt,
			Issue:        &issue,
		}},
		AfterTurn: func(ctx context.Context, result codex.Result, turnCount int) (codex.TurnPrompt, bool, error) {
			telemetry.RecordStep(ctx, rt.Telemetry, string(phase), "codex_turn_completed", "success", map[string]any{
				"issue_id":         currentIssue.ID,
				"issue_identifier": currentIssue.Identifier,
				"session_id":       result.SessionID,
				"turn_id":          result.TurnID,
				"turn_count":       turnCount,
			}, nil)
			logIssue(ctx, rt, currentIssue, "turn_completed", "Codex turn completed", map[string]any{"session_id": result.SessionID})
			refreshed, err := rt.Tracker.FetchIssue(ctx, currentIssue.ID)
			if err != nil {
				attemptResult = Result{Outcome: OutcomeRetryFailure}
				return codex.TurnPrompt{}, false, err
			}
			currentIssue = refreshed
			if outcome, done, err := decideAfterTurn(ctx, rt, currentIssue); done {
				attemptResult = outcome
				return codex.TurnPrompt{}, false, err
			}
			if reviewStatePasses(currentIssue, lastAgentMessage) {
				if !stateWriteExtensionEnabled(rt) {
					attemptResult = Result{Outcome: OutcomeWaitHuman}
					return codex.TurnPrompt{}, false, nil
				}
				currentIssue, err = applyReviewPass(ctx, rt, currentIssue)
				if err != nil {
					attemptResult = Result{Outcome: OutcomeRetryFailure}
					return codex.TurnPrompt{}, false, err
				}
			}
			if mergingStatePasses(currentIssue, lastAgentMessage) {
				if !stateWriteExtensionEnabled(rt) {
					attemptResult = Result{Outcome: OutcomeWaitHuman}
					return codex.TurnPrompt{}, false, nil
				}
				currentIssue, err = applyMergePass(ctx, rt, currentIssue)
				if err != nil {
					attemptResult = Result{Outcome: OutcomeRetryFailure}
					return codex.TurnPrompt{}, false, err
				}
				attemptResult = Result{Outcome: OutcomeDone}
				return codex.TurnPrompt{}, false, nil
			}
			if turnCount >= maxTurns(rt) {
				attemptResult = Result{Outcome: OutcomeRetryContinuation}
				return codex.TurnPrompt{}, false, nil
			}
			nextTurn := turnCount + 1
			nextPhase := nextWorkerPhase(currentIssue)
			nextStage := nextWorkerStage(currentIssue, nextTurn)
			setRunningStage(rt, currentIssue, attempt, nextPhase, nextStage, stageMessage(nextStage), workspacePath, nextTurn)
			promptIssue := currentIssue
			return codex.TurnPrompt{
				Text:         nextWorkerPrompt(currentIssue),
				Continuation: true,
				Attempt:      renderAttempt,
				Issue:        &promptIssue,
			}, true, nil
		},
	}
	_, err = rt.Runner.RunSession(ctx, request, func(event codex.Event) {
		if text := CompletedAgentMessageText(event); text != "" {
			lastAgentMessage = text
		}
		updateRunningFromEvent(rt, issue.ID, event)
		logIssue(ctx, rt, issue, "codex_event", event.Name, event.Payload)
	})
	removeRunning(rt, issue.ID)
	if err != nil {
		return returnRetryOrStop(err)
	}
	return attemptResult, nil
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
