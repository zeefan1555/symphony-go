package issueflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"symphony-go/internal/runtime/sessionstore"
	"symphony-go/internal/runtime/telemetry"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

const slowCodexTurnThreshold = 120 * time.Second

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
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	endStep("success", nil)
	defer func() {
		setRunningStage(rt, issue, attempt, phase, StageRunningWorkspaceHooks, "running after_run hook", workspacePath, 1)
		_, endStep := telemetry.StartStep(stepCtx, rt.Telemetry, string(phase), "after_run_hook", issueFields(issue))
		if err := rt.Workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			endStep("error", err)
			logIssue(stepCtx, rt, issue, "after_run_hook_failed", err.Error(), stepLogFields(phase, "after_run_hook", "error", nil))
			return
		}
		endStep("success", nil)
	}()
	setRunningStage(rt, issue, attempt, phase, StageRunningWorkspaceHooks, "running before_run hook", workspacePath, 1)
	_, endStep = telemetry.StartStep(ctx, rt.Telemetry, string(phase), "before_run_hook", issueFields(issue))
	if err := rt.Workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		endStep("error", err)
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
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	endStep("success", nil)
	setRunningStage(rt, issue, attempt, phase, nextWorkerStage(issue, 1), stageMessage(nextWorkerStage(issue, 1)), workspacePath, 1)
	resumeThreadID := resumeThreadIDForRework(ctx, rt, issue, workspacePath)
	attemptResult, sessionResult, err := runCodexSessionAttempt(ctx, rt, issue, attempt, workspacePath, promptText, renderAttempt, resumeThreadID)
	if err != nil && resumeThreadID != "" && ctx.Err() == nil && len(sessionResult.Turns) == 0 {
		logIssue(ctx, rt, issue, "thread_resume_failed", err.Error(), stepLogFields(nextWorkerPhase(issue), "thread_resume", "error", map[string]any{
			"thread_id":      resumeThreadID,
			"workspace_path": workspacePath,
			"fallback":       "new_thread",
		}))
		deleteSessionRecord(ctx, rt, issue.ID)
		attemptResult, sessionResult, err = runCodexSessionAttempt(ctx, rt, issue, attempt, workspacePath, promptText, renderAttempt, "")
	}
	if err != nil {
		return returnRetryOrStop(err)
	}
	return attemptResult, nil
}

func runCodexSessionAttempt(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt int, workspacePath string, promptText string, renderAttempt *int, resumeThreadID string) (Result, codex.SessionResult, error) {
	lastAgentMessage := ""
	attemptResult := Result{Outcome: OutcomeRetryContinuation}
	currentIssue := issue
	currentTurnCount := 1
	initialPromptText := promptText
	initialContinuation := false
	if resumeThreadID != "" {
		initialPromptText = ContinuationPromptText
		initialContinuation = true
	}
	request := codex.SessionRequest{
		WorkspacePath:  workspacePath,
		ResumeThreadID: resumeThreadID,
		Issue:          issue,
		Prompts: []codex.TurnPrompt{{
			Text:         initialPromptText,
			Continuation: initialContinuation,
			Attempt:      renderAttempt,
			Issue:        &issue,
		}},
		AfterTurn: func(ctx context.Context, result codex.Result, turnCount int) (codex.TurnPrompt, bool, error) {
			turnStartIssue := currentIssue
			turnPhase := nextWorkerPhase(turnStartIssue)
			turnStage := nextWorkerStage(turnStartIssue, turnCount)
			durationMS := result.Duration.Milliseconds()
			telemetry.RecordStepInterval(ctx, rt.Telemetry, string(turnPhase), "codex_turn_completed", "success", result.StartedAt, result.CompletedAt, map[string]any{
				"issue_id":         currentIssue.ID,
				"issue_identifier": currentIssue.Identifier,
				"session_id":       result.SessionID,
				"turn_id":          result.TurnID,
				"turn_count":       turnCount,
				"continuation":     result.Continuation,
				"stage":            string(turnStage),
				"state":            turnStartIssue.State,
				"duration_ms":      durationMS,
			}, nil)
			logIssue(ctx, rt, currentIssue, "codex_turn_completed", "Codex turn completed", stepLogFields(turnPhase, "codex_turn_completed", "success", map[string]any{
				"session_id":   result.SessionID,
				"turn_id":      result.TurnID,
				"turn_count":   turnCount,
				"continuation": result.Continuation,
				"stage":        string(turnStage),
				"state":        turnStartIssue.State,
				"duration_ms":  durationMS,
			}))
			activityFields := turnActivityFields(result, turnCount, turnPhase, turnStage, turnStartIssue.State)
			activityFields["issue_id"] = currentIssue.ID
			activityFields["issue_identifier"] = currentIssue.Identifier
			telemetry.RecordStep(ctx, rt.Telemetry, string(turnPhase), "codex_turn_activity_summary", "success", activityFields, nil)
			logIssue(ctx, rt, currentIssue, "codex_turn_activity_summary", turnActivityMessage(result, turnCount), stepLogFields(turnPhase, "codex_turn_activity_summary", "success", activityFields))
			if result.Duration >= slowCodexTurnThreshold {
				slowFields := cloneMap(activityFields)
				slowFields["threshold_ms"] = slowCodexTurnThreshold.Milliseconds()
				telemetry.RecordStep(ctx, rt.Telemetry, string(turnPhase), "codex_slow_turn", "success", slowFields, nil)
				logIssue(ctx, rt, currentIssue, "codex_slow_turn", slowTurnMessage(result), stepLogFields(turnPhase, "codex_slow_turn", "success", slowFields))
			}
			refreshed, err := rt.Tracker.FetchIssue(ctx, currentIssue.ID)
			if err != nil {
				attemptResult = Result{Outcome: OutcomeRetryFailure}
				return codex.TurnPrompt{}, false, err
			}
			currentIssue = refreshed
			upsertSessionRecord(ctx, rt, currentIssue, workspacePath, result)
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
				nextTurn := turnCount + 1
				nextPhase := nextWorkerPhase(currentIssue)
				nextStage := nextWorkerStage(currentIssue, nextTurn)
				setRunningStage(rt, currentIssue, attempt, nextPhase, nextStage, stageMessage(nextStage), workspacePath, nextTurn)
				currentTurnCount = nextTurn
				promptIssue := currentIssue
				return codex.TurnPrompt{
					Text:         nextWorkerPrompt(currentIssue),
					Continuation: true,
					Attempt:      renderAttempt,
					Issue:        &promptIssue,
				}, true, nil
			}
			if stateNameIn(turnStartIssue.State, []string{StatePushing}) && pushingStatePasses(currentIssue, lastAgentMessage) {
				if !stateWriteExtensionEnabled(rt) {
					attemptResult = Result{Outcome: OutcomeWaitHuman}
					return codex.TurnPrompt{}, false, nil
				}
				currentIssue, err = applyPushPass(ctx, rt, currentIssue)
				if err != nil {
					attemptResult = Result{Outcome: OutcomeRetryFailure}
					return codex.TurnPrompt{}, false, err
				}
				attemptResult = Result{Outcome: OutcomeDone}
				return codex.TurnPrompt{}, false, nil
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
			currentTurnCount = nextTurn
			promptIssue := currentIssue
			return codex.TurnPrompt{
				Text:         nextWorkerPrompt(currentIssue),
				Continuation: true,
				Attempt:      renderAttempt,
				Issue:        &promptIssue,
			}, true, nil
		},
	}
	sessionResult, err := rt.Runner.RunSession(ctx, request, func(event codex.Event) {
		if text := CompletedAgentMessageText(event); text != "" {
			lastAgentMessage = text
		}
		updateRunningFromEvent(rt, issue.ID, event)
		eventPhase := nextWorkerPhase(currentIssue)
		eventStage := nextWorkerStage(currentIssue, currentTurnCount)
		logIssue(ctx, rt, issue, "codex_event", event.Name, stepLogFields(eventPhase, "codex_event", "", mergeEventFields(event.Payload, eventStage, currentIssue.State)))
	})
	if err != nil {
		return attemptResult, sessionResult, err
	}
	return attemptResult, sessionResult, nil
}

func resumeThreadIDForRework(ctx context.Context, rt Runtime, issue issuemodel.Issue, workspacePath string) string {
	if rt.Sessions == nil || !stateNameIn(issue.State, []string{StateRework}) {
		return ""
	}
	record, err := rt.Sessions.Get(ctx, issue.ID)
	if err != nil {
		if !errors.Is(err, sessionstore.ErrNotFound) {
			logIssue(ctx, rt, issue, "session_store_lookup_failed", err.Error(), stepLogFields(nextWorkerPhase(issue), "session_store_lookup", "error", nil))
		}
		return ""
	}
	if record.ThreadID == "" {
		logIssue(ctx, rt, issue, "thread_resume_skipped", "stored session record has no thread id", stepLogFields(nextWorkerPhase(issue), "thread_resume", "skipped", map[string]any{
			"reason": "empty_thread_id",
		}))
		return ""
	}
	if record.WorkspacePath != workspacePath {
		logIssue(ctx, rt, issue, "thread_resume_skipped", "stored session workspace does not match current workspace", stepLogFields(nextWorkerPhase(issue), "thread_resume", "skipped", map[string]any{
			"reason":           "workspace_mismatch",
			"stored_workspace": record.WorkspacePath,
			"workspace_path":   workspacePath,
			"thread_id":        record.ThreadID,
		}))
		return ""
	}
	return record.ThreadID
}

func upsertSessionRecord(ctx context.Context, rt Runtime, issue issuemodel.Issue, workspacePath string, result codex.Result) {
	if rt.Sessions == nil || result.ThreadID == "" {
		return
	}
	record := sessionstore.Record{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		ThreadID:        result.ThreadID,
		WorkspacePath:   workspacePath,
		WorkerHost:      result.WorkerHost,
		LastState:       issue.State,
		LastSessionID:   result.SessionID,
		LastTurnID:      result.TurnID,
		UpdatedAt:       time.Now().UTC(),
	}
	if err := rt.Sessions.Upsert(ctx, record); err != nil {
		logIssue(ctx, rt, issue, "session_store_upsert_failed", err.Error(), stepLogFields(nextWorkerPhase(issue), "session_store_upsert", "error", map[string]any{
			"thread_id":      result.ThreadID,
			"workspace_path": workspacePath,
		}))
	}
}

func deleteSessionRecord(ctx context.Context, rt Runtime, issueID string) {
	if rt.Sessions == nil {
		return
	}
	if err := rt.Sessions.Delete(ctx, issueID); err != nil {
		logIssue(ctx, rt, issuemodel.Issue{ID: issueID}, "session_store_delete_failed", err.Error(), stepLogFields(PhaseImplementer, "session_store_delete", "error", nil))
	}
}

func stepLogFields(phase AgentPhase, step, outcome string, fields map[string]any) map[string]any {
	correlated := make(map[string]any, len(fields)+3)
	for key, value := range fields {
		correlated[key] = value
	}
	correlated["phase"] = string(phase)
	correlated["step"] = step
	if outcome != "" {
		correlated["outcome"] = outcome
	}
	return correlated
}

func turnActivityFields(result codex.Result, turnCount int, phase AgentPhase, stage RunStage, state string) map[string]any {
	durationMS := result.Duration.Milliseconds()
	commandDurationMS := result.Stats.CommandDurationMS
	nonCommandDurationMS := durationMS - commandDurationMS
	if nonCommandDurationMS < 0 {
		nonCommandDurationMS = 0
	}
	return map[string]any{
		"session_id":                  result.SessionID,
		"turn_id":                     result.TurnID,
		"turn_count":                  turnCount,
		"continuation":                result.Continuation,
		"phase":                       string(phase),
		"stage":                       string(stage),
		"state":                       state,
		"duration_ms":                 durationMS,
		"command_count":               result.Stats.CommandCount,
		"failed_command_count":        result.Stats.FailedCommandCount,
		"command_duration_ms":         commandDurationMS,
		"slowest_command_duration_ms": result.Stats.SlowestCommandDurationMS,
		"non_command_duration_ms":     nonCommandDurationMS,
		"dominant_command_kind":       result.Stats.DominantCommandKind(),
		"file_change_count":           result.Stats.FileChangeCount,
		"changed_file_count":          result.Stats.ChangedFileCount,
		"final_message_present":       result.Stats.FinalMessagePresent,
	}
}

func turnActivityMessage(result codex.Result, turnCount int) string {
	return fmt.Sprintf("Turn %d finished in %dms; commands=%d failed=%d command_ms=%d files_changed=%d dominant=%s",
		turnCount,
		result.Duration.Milliseconds(),
		result.Stats.CommandCount,
		result.Stats.FailedCommandCount,
		result.Stats.CommandDurationMS,
		result.Stats.ChangedFileCount,
		result.Stats.DominantCommandKind(),
	)
}

func slowTurnMessage(result codex.Result) string {
	nonCommandDurationMS := result.Duration.Milliseconds() - result.Stats.CommandDurationMS
	if nonCommandDurationMS < 0 {
		nonCommandDurationMS = 0
	}
	return fmt.Sprintf("Slow turn detected: duration=%dms command_ms=%d dominant=%s; likely non-command time=%dms",
		result.Duration.Milliseconds(),
		result.Stats.CommandDurationMS,
		result.Stats.DominantCommandKind(),
		nonCommandDurationMS,
	)
}

func mergeEventFields(fields map[string]any, stage RunStage, state string) map[string]any {
	merged := cloneMap(fields)
	merged["stage"] = string(stage)
	merged["state"] = state
	return merged
}

func cloneMap(fields map[string]any) map[string]any {
	clone := make(map[string]any, len(fields)+2)
	for key, value := range fields {
		clone[key] = value
	}
	return clone
}

func stageMessage(stage RunStage) string {
	switch stage {
	case StageContinuingImplementation:
		return "continuing implementation"
	case StageContinuingAIReview:
		return "continuing in AI Review"
	case StageContinuingPushing:
		return "continuing in Pushing"
	case StageContinuingMerging:
		return "continuing in Merging"
	default:
		return "running Codex turn"
	}
}
