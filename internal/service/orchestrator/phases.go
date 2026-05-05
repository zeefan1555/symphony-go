package orchestrator

import (
	"context"

	issuemodel "symphony-go/internal/service/issue"
)

type agentPhase string

const (
	phaseImplementer agentPhase = "implementation"
	phaseReview      agentPhase = "review"
	phaseMerge       agentPhase = "merge"

	reviewContinuationPromptText  = "Continue in the same issue session and perform the AI Review stage for this issue. Re-check the current workspace, issue state, workpad, diff, commit range, and validation evidence. If the review passes, report a clear pass conclusion so the orchestrator can continue to merging; if it fails, move the issue to Rework with concrete findings."
	reworkContinuationPromptText  = "Continue in the same issue session and handle the Rework stage. Re-check the review findings and current workspace state, fix the issues, run the smallest relevant verification, update the workpad, and move the issue back to AI Review only after the rework is complete."
	mergingContinuationPromptText = "Continue in the same issue session and execute the merge protocol for this issue. Re-check the current workspace and issue state, perform the required merge steps, run the smallest relevant verification, move the issue to Done only after the merge is complete, and report concrete results or blockers."
)

func (o *Orchestrator) runAgentWith(ctx context.Context, rt runtimeSnapshot, issue issuemodel.Issue, attempt int) error {
	if isTerminal(issue.State, rt.workflow.Config.Tracker.TerminalStates) {
		return nil
	}
	switch issue.State {
	case "Todo":
		if err := rt.tracker.UpdateIssueState(ctx, issue.ID, "In Progress"); err != nil {
			return err
		}
		issue.State = "In Progress"
		o.logIssue(issue, "state_changed", "Todo -> In Progress", nil)
	}
	switch issue.State {
	case "Human Review", "In Review":
		o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
		return errNoRetryNeeded
	case "AI Review":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseReview)
	case "Merging":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseMerge)
	}
	return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
}
