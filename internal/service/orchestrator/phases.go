package orchestrator

import (
	"context"

	issuemodel "symphony-go/internal/service/issue"
)

type agentPhase string

const (
	phaseImplementer agentPhase = "implementer"
	phaseReviewer    agentPhase = "reviewer"

	aiReviewContinuationPromptText = "Continue in the same issue session and execute the AI Review protocol for this issue. Re-check the issue, workpad, diff, commits, review feedback, and validation evidence. If the work is correct, report Review: PASS so the orchestrator can continue to Merging in this same session; if it is not correct, record actionable findings, move the issue to Rework, and report concrete blockers."
	mergingContinuationPromptText  = "Continue in the same issue session and execute the Merging protocol for this issue. Use the PR skill fast path: confirm the PR skill was opened, confirm pr_merge_flow.sh is executable, prepare the PR title/body, run the script, then update the workpad once with merge evidence. Do not move Linear to Done from the agent; final reply must start with Merge: PASS and include PR URL, merge commit, and root status so the orchestrator can mark Done."
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
