package issueflow

import (
	"context"
	"errors"

	issuemodel "symphony-go/internal/service/issue"
)

func RunIssueTrunk(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt int) (Result, error) {
	if result, ok, err := stopIfContextDone(ctx); ok {
		return result, err
	}
	if result, ok := returnIfTerminal(rt, issue); ok {
		return result, nil
	}
	if result, ok := waitIfBlocked(rt, issue); ok {
		return result, nil
	}
	var err error
	issue, err = promoteTodoToInProgress(ctx, rt, issue)
	if err != nil {
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	if result, ok := waitIfHumanReview(rt, issue); ok {
		return result, nil
	}

	switch issue.State {
	case StateAIReview:
		return runAIReviewUntilMerge(ctx, rt, issue, attempt, 1)
	case StateMerging:
		return runMergingUntilDone(ctx, rt, issue, attempt, 1)
	default:
		if !isActive(issue.State, rt.Workflow.Config.Tracker.ActiveStates) {
			return Result{Outcome: OutcomeStopped}, nil
		}
		return runImplementationUntilReview(ctx, rt, issue, attempt, 1, "")
	}
}

func stopIfContextDone(ctx context.Context) (Result, bool, error) {
	if err := ctx.Err(); err != nil {
		return Result{Outcome: OutcomeStopped}, true, err
	}
	return Result{}, false, nil
}

func returnIfTerminal(rt Runtime, issue issuemodel.Issue) (Result, bool) {
	if isTerminal(issue.State, rt.Workflow.Config.Tracker.TerminalStates) {
		return Result{Outcome: OutcomeDone}, true
	}
	return Result{}, false
}

func waitIfBlocked(rt Runtime, issue issuemodel.Issue) (Result, bool) {
	if issue.State != StateBlocked {
		return Result{}, false
	}
	logIssue(rt, issue, "blocked", "issue is blocked by non-terminal dependencies", nil)
	return Result{Outcome: OutcomeWaitHuman}, true
}

func promoteTodoToInProgress(ctx context.Context, rt Runtime, issue issuemodel.Issue) (issuemodel.Issue, error) {
	if issue.State != StateTodo {
		return issue, nil
	}
	if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateInProgress); err != nil {
		return issue, err
	}
	issue.State = StateInProgress
	logIssue(rt, issue, "state_changed", "Todo -> In Progress", nil)
	return issue, nil
}

func waitIfHumanReview(rt Runtime, issue issuemodel.Issue) (Result, bool) {
	if issue.State != StateHumanReview && issue.State != "In Review" {
		return Result{}, false
	}
	logIssue(rt, issue, "waiting_for_review", "issue is waiting for human review", nil)
	return Result{Outcome: OutcomeWaitHuman}, true
}

func runImplementationUntilReview(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt, turn int, prompt string) (Result, error) {
	for turn <= maxTurns(rt) {
		result, err := runAgentTurn(ctx, rt, issue, attempt, PhaseImplementer, implementationStage(turn), prompt, turn > 1, turn)
		if err != nil {
			return returnRetryOrStop(err)
		}
		issue = result.Issue
		if outcome, done, err := decideAfterTurn(ctx, rt, issue); done {
			return outcome, err
		}
		switch issue.State {
		case StateAIReview:
			return runAIReviewUntilMerge(ctx, rt, issue, attempt, turn+1)
		case StateMerging:
			return runMergingUntilDone(ctx, rt, issue, attempt, turn+1)
		}
		turn++
		prompt = ContinuationPromptText
	}
	return Result{Outcome: OutcomeRetryContinuation}, nil
}

func runAIReviewUntilMerge(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt, turn int) (Result, error) {
	prompt := AIReviewContinuationPromptText
	continuation := true
	if turn <= 1 {
		prompt = ""
		continuation = false
	}
	for turn <= maxTurns(rt) {
		result, err := runAgentTurn(ctx, rt, issue, attempt, PhaseReviewer, aiReviewStage(turn), prompt, continuation, turn)
		if err != nil {
			return returnRetryOrStop(err)
		}
		issue = result.Issue
		if outcome, done, err := decideAfterTurn(ctx, rt, issue); done {
			return outcome, err
		}
		if issue.State == StateAIReview && reviewFinalPasses(result.LastAgentMessage) {
			if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateMerging); err != nil {
				return Result{Outcome: OutcomeRetryFailure}, err
			}
			issue.State = StateMerging
			logIssue(rt, issue, "state_changed", "AI Review -> Merging", nil)
			return runMergingUntilDone(ctx, rt, issue, attempt, turn+1)
		}
		if issue.State == StateMerging {
			return runMergingUntilDone(ctx, rt, issue, attempt, turn+1)
		}
		if issue.State != StateAIReview {
			return runImplementationUntilReview(ctx, rt, issue, attempt, turn+1, ContinuationPromptText)
		}
		turn++
		prompt = AIReviewContinuationPromptText
		continuation = true
	}
	return Result{Outcome: OutcomeRetryContinuation}, nil
}

func runMergingUntilDone(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt, turn int) (Result, error) {
	prompt := MergingContinuationPromptText
	continuation := true
	if turn <= 1 {
		prompt = ""
		continuation = false
	}
	for turn <= maxTurns(rt) {
		result, err := runAgentTurn(ctx, rt, issue, attempt, PhaseReviewer, mergingStage(turn), prompt, continuation, turn)
		if err != nil {
			return returnRetryOrStop(err)
		}
		issue = result.Issue
		if outcome, done, err := decideAfterTurn(ctx, rt, issue); done {
			return outcome, err
		}
		if issue.State == StateMerging && mergeFinalPasses(result.LastAgentMessage) {
			if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateDone); err != nil {
				return Result{Outcome: OutcomeRetryFailure}, err
			}
			issue.State = StateDone
			logIssue(rt, issue, "state_changed", "Merging -> Done", nil)
			return Result{Outcome: OutcomeDone}, nil
		}
		if issue.State == StateAIReview {
			return runAIReviewUntilMerge(ctx, rt, issue, attempt, turn+1)
		}
		if issue.State != StateMerging {
			return runImplementationUntilReview(ctx, rt, issue, attempt, turn+1, ContinuationPromptText)
		}
		turn++
		prompt = MergingContinuationPromptText
		continuation = true
	}
	return Result{Outcome: OutcomeRetryContinuation}, nil
}

func decideAfterTurn(ctx context.Context, rt Runtime, issue issuemodel.Issue) (Result, bool, error) {
	if result, ok, err := stopIfContextDone(ctx); ok {
		return result, true, err
	}
	if result, ok := returnIfTerminal(rt, issue); ok {
		return result, true, nil
	}
	if result, ok := waitIfHumanReview(rt, issue); ok {
		return result, true, nil
	}
	if !isActive(issue.State, rt.Workflow.Config.Tracker.ActiveStates) {
		return Result{Outcome: OutcomeStopped}, true, nil
	}
	return Result{}, false, nil
}

func returnRetryOrStop(err error) (Result, error) {
	if err == nil {
		return Result{Outcome: OutcomeRetryContinuation}, nil
	}
	if errors.Is(err, context.Canceled) {
		return Result{Outcome: OutcomeStopped}, err
	}
	return Result{Outcome: OutcomeRetryFailure}, err
}

func maxTurns(rt Runtime) int {
	if rt.Workflow == nil || rt.Workflow.Config.Agent.MaxTurns <= 0 {
		return 1
	}
	return rt.Workflow.Config.Agent.MaxTurns
}

func implementationStage(turn int) RunStage {
	if turn <= 1 {
		return StageRunningAgent
	}
	return StageContinuingImplementation
}

func aiReviewStage(turn int) RunStage {
	if turn <= 1 {
		return StageRunningAgent
	}
	return StageContinuingAIReview
}

func mergingStage(turn int) RunStage {
	if turn <= 1 {
		return StageRunningAgent
	}
	return StageContinuingMerging
}

func isActive(state string, active []string) bool {
	if len(active) == 0 {
		return true
	}
	for _, item := range active {
		if item == state {
			return true
		}
	}
	return false
}

func isTerminal(state string, terminal []string) bool {
	for _, item := range terminal {
		if item == state {
			return true
		}
	}
	return false
}
