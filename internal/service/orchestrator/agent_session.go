package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"symphony-go/internal/runtime/observability"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workflow"
	"symphony-go/internal/service/workspace"
)

func (o *Orchestrator) runPhaseAgent(ctx context.Context, rt runtimeSnapshot, issue issuemodel.Issue, attempt int, phase agentPhase) error {
	switch phase {
	case phaseImplementer, phaseReview, phaseMerge:
	default:
		return fmt.Errorf("unknown agent phase %q", phase)
	}
	o.setRunning(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		TurnCount:       1,
		StartedAt:       time.Now(),
		LastEvent:       "preparing workspace",
		LastMessage:     "preparing workspace",
	})
	hookCtx := workspace.WithHookIssue(ctx, issue)
	workspacePath, _, err := rt.workspace.Ensure(hookCtx, issue)
	if err != nil {
		o.removeRunning(issue.ID)
		return err
	}
	defer func() {
		if err := rt.workspace.AfterRun(workspace.WithHookIssue(context.Background(), issue), workspacePath); err != nil {
			o.logIssue(issue, "after_run_hook_failed", err.Error(), nil)
		}
	}()
	if err := rt.workspace.BeforeRun(hookCtx, workspacePath); err != nil {
		o.removeRunning(issue.ID)
		return err
	}
	maxTurns := rt.workflow.Config.Agent.MaxTurns
	var renderAttempt *int
	if attempt > 0 {
		value := attempt
		renderAttempt = &value
	}
	prompt, err := workflow.Render(rt.workflow.PromptTemplate, issue, renderAttempt)
	if err != nil {
		return err
	}
	o.setRunning(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		WorkspacePath:   workspacePath,
		TurnCount:       1,
		StartedAt:       time.Now(),
	})
	maxTurnsReached := false
	noRetryNeeded := false
	reviewFinal := ""
	currentPhase := phase
	currentPhaseTurns := 1
	request := codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts:       []codex.TurnPrompt{{Text: prompt, Attempt: renderAttempt}},
	}
	continueWith := func(nextIssue issuemodel.Issue, nextPhase agentPhase, promptText string) (codex.TurnPrompt, bool) {
		if nextPhase == currentPhase {
			if currentPhaseTurns >= maxTurns {
				maxTurnsReached = true
				return codex.TurnPrompt{}, false
			}
			currentPhaseTurns++
		} else {
			currentPhase = nextPhase
			currentPhaseTurns = 1
		}
		issue = nextIssue
		o.setRunning(observability.RunningEntry{
			IssueID:         issue.ID,
			IssueIdentifier: issue.Identifier,
			State:           issue.State,
			WorkspacePath:   workspacePath,
			TurnCount:       turnCountForRunning(currentPhaseTurns),
			StartedAt:       time.Now(),
		})
		next := issue
		return codex.TurnPrompt{Text: promptText, Continuation: true, Issue: &next}, true
	}
	request.AfterTurn = func(ctx context.Context, result codex.Result, turn int) (codex.TurnPrompt, bool, error) {
		o.logIssue(issue, "turn_completed", "Codex turn completed", map[string]any{"session_id": result.SessionID})
		refreshed, err := rt.tracker.FetchIssue(ctx, issue.ID)
		if err != nil {
			return codex.TurnPrompt{}, false, err
		}
		if !isActive(refreshed.State, rt.workflow.Config.Tracker.ActiveStates) || isTerminal(refreshed.State, rt.workflow.Config.Tracker.TerminalStates) {
			noRetryNeeded = true
			return codex.TurnPrompt{}, false, nil
		}
		switch refreshed.State {
		case "Human Review", "In Review":
			o.logIssue(refreshed, "waiting_for_review", "issue is waiting for human review", nil)
			noRetryNeeded = true
			return codex.TurnPrompt{}, false, nil
		case "AI Review":
			if currentPhase == phaseReview && reviewFinalPasses(reviewFinal) {
				if err := rt.tracker.UpdateIssueState(ctx, refreshed.ID, "Merging"); err != nil {
					return codex.TurnPrompt{}, false, err
				}
				refreshed.State = "Merging"
				reviewFinal = ""
				o.logIssue(refreshed, "state_changed", "AI Review -> Merging", nil)
				next, ok := continueWith(refreshed, phaseMerge, mergingContinuationPromptText)
				return next, ok, nil
			}
			next, ok := continueWith(refreshed, phaseReview, reviewContinuationPromptText)
			return next, ok, nil
		case "Merging":
			next, ok := continueWith(refreshed, phaseMerge, mergingContinuationPromptText)
			return next, ok, nil
		case "Rework":
			next, ok := continueWith(refreshed, phaseImplementer, reworkContinuationPromptText)
			return next, ok, nil
		}
		next, ok := continueWith(refreshed, phaseImplementer, continuationPromptText)
		return next, ok, nil
	}
	_, err = rt.runner.RunSession(ctx, request, func(event codex.Event) {
		if currentPhase == phaseReview {
			if text := completedAgentMessageText(event); text != "" {
				reviewFinal = text
			}
		}
		o.updateRunningFromEvent(issue.ID, event)
		o.logIssue(issue, "codex_event", event.Name, event.Payload)
	})
	o.removeRunning(issue.ID)
	if err != nil {
		return err
	}
	if maxTurnsReached {
		return fmt.Errorf("reached max turns for %s while issue stayed active", issue.Identifier)
	}
	if noRetryNeeded {
		return errNoRetryNeeded
	}
	return nil
}

func turnCountForRunning(phaseTurns int) int {
	if phaseTurns < 1 {
		return 1
	}
	return phaseTurns
}

func reviewFinalPasses(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range []string{
		"review: pass",
		"conclusion: pass",
		"结论: pass",
		"结论：pass",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func completedAgentMessageText(event codex.Event) string {
	if event.Name != "item/completed" {
		return ""
	}
	params, _ := event.Payload["params"].(map[string]any)
	item, _ := params["item"].(map[string]any)
	itemType, _ := item["type"].(string)
	if itemType != "agentMessage" {
		return ""
	}
	text, _ := item["text"].(string)
	return text
}
