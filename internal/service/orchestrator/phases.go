package orchestrator

import (
	"context"

	issuemodel "symphony-go/internal/service/issue"
)

type agentPhase string

const (
	phaseImplementer agentPhase = "implementer"
	phaseReviewer    agentPhase = "reviewer"

	mergingContinuationPromptText = "Continue in the same reviewer session and execute the merge protocol for this issue. Re-check the current workspace and issue state, perform the required merge steps, run the smallest relevant verification, move the issue to Done only after the merge is complete, and report concrete results or blockers."
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
	case "AI Review", "Merging":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseReviewer)
	}
	return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
}
