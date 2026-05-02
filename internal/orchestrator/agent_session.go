package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/types"
	"github.com/zeefan1555/symphony-go/internal/workflow"
	"github.com/zeefan1555/symphony-go/internal/workspace"
)

func (o *Orchestrator) runPhaseAgent(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int, phase agentPhase) error {
	switch phase {
	case phaseImplementer, phaseReviewer:
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
	var nextIssue *types.Issue
	maxTurnsReached := false
	noRetryNeeded := false
	request := codex.SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issue,
		Prompts:       []codex.TurnPrompt{{Text: prompt, Attempt: renderAttempt}},
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
		if refreshed.State == "Human Review" || refreshed.State == "In Review" || refreshed.State == "AI Review" {
			nextIssue = &refreshed
			return codex.TurnPrompt{}, false, nil
		}
		if turn >= maxTurns {
			maxTurnsReached = true
			return codex.TurnPrompt{}, false, nil
		}
		if refreshed.State == "Merging" {
			if phase != phaseReviewer {
				nextIssue = &refreshed
				return codex.TurnPrompt{}, false, nil
			}
			issue = refreshed
			o.setRunning(observability.RunningEntry{
				IssueID:         issue.ID,
				IssueIdentifier: issue.Identifier,
				State:           issue.State,
				WorkspacePath:   workspacePath,
				TurnCount:       turn + 1,
				StartedAt:       time.Now(),
			})
			next := issue
			return codex.TurnPrompt{Text: mergingContinuationPromptText, Continuation: true, Issue: &next}, true, nil
		}
		issue = refreshed
		o.setRunning(observability.RunningEntry{
			IssueID:         issue.ID,
			IssueIdentifier: issue.Identifier,
			State:           issue.State,
			WorkspacePath:   workspacePath,
			TurnCount:       turn + 1,
			StartedAt:       time.Now(),
		})
		return codex.TurnPrompt{Text: continuationPromptText, Continuation: true}, true, nil
	}
	_, err = rt.runner.RunSession(ctx, request, func(event codex.Event) {
		o.updateRunningFromEvent(issue.ID, event)
		o.logIssue(issue, "codex_event", event.Name, event.Payload)
	})
	o.removeRunning(issue.ID)
	if err != nil {
		return err
	}
	if nextIssue != nil {
		if nextIssue.State == "Human Review" || nextIssue.State == "In Review" {
			o.logIssue(*nextIssue, "waiting_for_review", "issue is waiting for human review", nil)
			return errNoRetryNeeded
		}
		if nextIssue.State == "AI Review" || nextIssue.State == "Rework" {
			return errNoRetryNeeded
		}
		return o.runAgentWith(ctx, rt, *nextIssue, attempt)
	}
	if maxTurnsReached {
		return fmt.Errorf("reached max turns for %s while issue stayed active", issue.Identifier)
	}
	if noRetryNeeded {
		return errNoRetryNeeded
	}
	return nil
}
