package issueflow

import (
	"context"
	"errors"
	"strings"

	"symphony-go/internal/runtime/telemetry"
	issuemodel "symphony-go/internal/service/issue"
)

func RunIssueTrunk(ctx context.Context, rt Runtime, issue issuemodel.Issue, attempt int) (result Result, err error) {
	ctx, endIssueRun := telemetry.StartIssueRun(ctx, rt.Telemetry, issueRunFields(issue, attempt))
	defer func() {
		endIssueRun(string(result.Outcome), err)
	}()
	if result, ok, err := stopIfContextDone(ctx); ok {
		return result, err
	}
	if result, ok := returnIfTerminal(rt, issue); ok {
		return result, nil
	}
	if result, ok := waitIfBlocked(ctx, rt, issue); ok {
		return result, nil
	}
	issue, err = promoteTodoToInProgress(ctx, rt, issue)
	if err != nil {
		return Result{Outcome: OutcomeRetryFailure}, err
	}
	if result, ok := waitIfHumanReview(ctx, rt, issue); ok {
		return result, nil
	}
	if !isActive(issue.State, rt.Workflow.Config.Tracker.ActiveStates) {
		return Result{Outcome: OutcomeStopped}, nil
	}
	return runWorkerAttempt(ctx, rt, issue, attempt)
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

func waitIfBlocked(ctx context.Context, rt Runtime, issue issuemodel.Issue) (Result, bool) {
	if !strings.EqualFold(strings.TrimSpace(issue.State), StateBlocked) {
		return Result{}, false
	}
	logIssue(ctx, rt, issue, "blocked", "issue is blocked by non-terminal dependencies", nil)
	return Result{Outcome: OutcomeWaitHuman}, true
}

func promoteTodoToInProgress(ctx context.Context, rt Runtime, issue issuemodel.Issue) (issuemodel.Issue, error) {
	if !strings.EqualFold(strings.TrimSpace(issue.State), StateTodo) {
		return issue, nil
	}
	_, endTransition := telemetry.StartTransition(ctx, rt.Telemetry, StateTodo, StateInProgress, issueFields(issue))
	if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateInProgress); err != nil {
		endTransition("error", err)
		return issue, err
	}
	issue.State = StateInProgress
	endTransition("success", nil)
	logIssue(ctx, rt, issue, "state_changed", "Todo -> In Progress", nil)
	return issue, nil
}

func waitIfHumanReview(ctx context.Context, rt Runtime, issue issuemodel.Issue) (Result, bool) {
	state := strings.TrimSpace(issue.State)
	if !strings.EqualFold(state, StateHumanReview) && !strings.EqualFold(state, "In Review") {
		return Result{}, false
	}
	logIssue(ctx, rt, issue, "waiting_for_review", "issue is waiting for human review", nil)
	return Result{Outcome: OutcomeWaitHuman}, true
}

func decideAfterTurn(ctx context.Context, rt Runtime, issue issuemodel.Issue) (Result, bool, error) {
	if result, ok, err := stopIfContextDone(ctx); ok {
		return result, true, err
	}
	if result, ok := returnIfTerminal(rt, issue); ok {
		return result, true, nil
	}
	if result, ok := waitIfHumanReview(ctx, rt, issue); ok {
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

func pushingStage(turn int) RunStage {
	if turn <= 1 {
		return StageRunningAgent
	}
	return StageContinuingPushing
}

func isActive(state string, active []string) bool {
	if len(active) == 0 {
		return true
	}
	return stateNameIn(state, active)
}

func isTerminal(state string, terminal []string) bool {
	return stateNameIn(state, terminal)
}

func stateNameIn(state string, states []string) bool {
	state = strings.TrimSpace(state)
	if state == "" {
		return false
	}
	for _, item := range states {
		if strings.EqualFold(strings.TrimSpace(item), state) {
			return true
		}
	}
	return false
}

func stateWriteExtensionEnabled(rt Runtime) bool {
	return rt.Workflow != nil && strings.EqualFold(strings.TrimSpace(rt.Workflow.Config.Agent.ReviewPolicy.Mode), "auto")
}

func reviewStatePasses(issue issuemodel.Issue, lastAgentMessage string) bool {
	return stateNameIn(issue.State, []string{StateAIReview}) && reviewFinalPasses(lastAgentMessage)
}

func mergingStatePasses(issue issuemodel.Issue, lastAgentMessage string) bool {
	return stateNameIn(issue.State, []string{StateMerging}) && mergeFinalPasses(lastAgentMessage)
}

func pushingStatePasses(issue issuemodel.Issue, lastAgentMessage string) bool {
	return stateNameIn(issue.State, []string{StatePushing}) && pushFinalPasses(lastAgentMessage)
}

func nextWorkerPhase(issue issuemodel.Issue) AgentPhase {
	switch {
	case stateNameIn(issue.State, []string{StateAIReview, StatePushing, StateMerging}):
		return PhaseReviewer
	default:
		return PhaseImplementer
	}
}

func nextWorkerPrompt(issue issuemodel.Issue) string {
	switch {
	case stateNameIn(issue.State, []string{StateAIReview}):
		return AIReviewContinuationPromptText
	case stateNameIn(issue.State, []string{StatePushing}):
		return PushingContinuationPromptText
	case stateNameIn(issue.State, []string{StateMerging}):
		return MergingContinuationPromptText
	default:
		return ContinuationPromptText
	}
}

func nextWorkerStage(issue issuemodel.Issue, turn int) RunStage {
	switch {
	case stateNameIn(issue.State, []string{StateAIReview}):
		return aiReviewStage(turn)
	case stateNameIn(issue.State, []string{StatePushing}):
		return pushingStage(turn)
	case stateNameIn(issue.State, []string{StateMerging}):
		return mergingStage(turn)
	default:
		return implementationStage(turn)
	}
}

func applyReviewPass(ctx context.Context, rt Runtime, issue issuemodel.Issue) (issuemodel.Issue, error) {
	_, endTransition := telemetry.StartTransition(ctx, rt.Telemetry, StateAIReview, StatePushing, issueFields(issue))
	if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StatePushing); err != nil {
		endTransition("error", err)
		return issue, err
	}
	issue.State = StatePushing
	endTransition("success", nil)
	logIssue(ctx, rt, issue, "state_changed", "AI Review -> Pushing", nil)
	return issue, nil
}

func applyPushPass(ctx context.Context, rt Runtime, issue issuemodel.Issue) (issuemodel.Issue, error) {
	_, endTransition := telemetry.StartTransition(ctx, rt.Telemetry, StatePushing, StateDone, issueFields(issue))
	if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateDone); err != nil {
		endTransition("error", err)
		return issue, err
	}
	issue.State = StateDone
	endTransition("success", nil)
	logIssue(ctx, rt, issue, "state_changed", "Pushing -> Done", nil)
	return issue, nil
}

func applyMergePass(ctx context.Context, rt Runtime, issue issuemodel.Issue) (issuemodel.Issue, error) {
	_, endTransition := telemetry.StartTransition(ctx, rt.Telemetry, StateMerging, StateDone, issueFields(issue))
	if err := rt.Tracker.UpdateIssueState(ctx, issue.ID, StateDone); err != nil {
		endTransition("error", err)
		return issue, err
	}
	issue.State = StateDone
	endTransition("success", nil)
	logIssue(ctx, rt, issue, "state_changed", "Merging -> Done", nil)
	return issue, nil
}

func issueRunFields(issue issuemodel.Issue, attempt int) map[string]any {
	fields := issueFields(issue)
	fields["attempt"] = attempt
	fields["state"] = issue.State
	return fields
}

func issueFields(issue issuemodel.Issue) map[string]any {
	return map[string]any{
		"issue_id":         issue.ID,
		"issue_identifier": issue.Identifier,
	}
}
