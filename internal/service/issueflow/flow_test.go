package issueflow

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	runtimeconfig "symphony-go/internal/runtime/config"
	"symphony-go/internal/service/codex"
	issuemodel "symphony-go/internal/service/issue"
	"symphony-go/internal/service/workspace"
)

var errFakeTrackerUpdate = errors.New("tracker update failed")
var errFakeTrackerFetch = errors.New("tracker fetch failed")
var errFakeRunner = errors.New("runner failed")

func TestDefinitionForTrunkShowsHumanReadableMainline(t *testing.T) {
	def := DefinitionForTrunk()

	if def.Name != "issue-flow-trunk" {
		t.Fatalf("Name = %q, want issue-flow-trunk", def.Name)
	}
	if def.EntryPoint != "issueflow.RunIssueTrunk" {
		t.Fatalf("EntryPoint = %q, want issueflow.RunIssueTrunk", def.EntryPoint)
	}
	wantSteps := []string{StateBlocked, StateTodo, StateInProgress, StateAIReview, StateMerging, StateDone}
	if len(def.Steps) != len(wantSteps) {
		t.Fatalf("steps = %#v, want %d trunk steps", def.Steps, len(wantSteps))
	}
	for i, want := range wantSteps {
		if def.Steps[i].Name != want {
			t.Fatalf("step[%d] = %q, want %q", i, def.Steps[i].Name, want)
		}
		if def.Steps[i].Purpose == "" || def.Steps[i].CoreInterface == "" {
			t.Fatalf("step[%d] missing purpose/core interface: %#v", i, def.Steps[i])
		}
	}
	if len(def.Transitions) != len(wantSteps)-1 {
		t.Fatalf("transitions = %#v, want trunk transitions", def.Transitions)
	}
	if def.Transitions[0].From != StateBlocked || def.Transitions[0].To != StateTodo || def.Transitions[0].Actor != ActorHuman {
		t.Fatalf("first transition = %#v, want human Blocked -> Todo", def.Transitions[0])
	}
	if def.Transitions[len(def.Transitions)-1].To != StateDone {
		t.Fatalf("last transition = %#v, want terminal Done", def.Transitions[len(def.Transitions)-1])
	}
	if len(def.FailurePolicy) == 0 {
		t.Fatal("failure policy must explain retry and human wait handling")
	}
}

func TestRunIssueTrunkPromotesTodoAndRunsImplementer(t *testing.T) {
	tracker := &fakeTracker{issue: issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "todo", State: StateTodo}}
	runner := &fakeRunner{}
	observer := &fakeObserver{}
	rt := testRuntime(t, tracker, runner, observer)

	result, err := RunIssueTrunk(context.Background(), rt, tracker.issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeRetryContinuation {
		t.Fatalf("outcome = %q, want retry continuation", result.Outcome)
	}
	if tracker.updateState != StateInProgress {
		t.Fatalf("UpdateIssueState = %q, want In Progress", tracker.updateState)
	}
	if runner.request.Issue.State != StateInProgress {
		t.Fatalf("runner issue state = %q, want In Progress", runner.request.Issue.State)
	}
	if !observer.sawStage(PhaseImplementer, StageRunningAgent) {
		t.Fatalf("stages = %#v, want implementer running_agent", observer.stages)
	}
}

func TestRunIssueTrunkWaitsForHumanReview(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "review", State: StateHumanReview}
	tracker := &fakeTracker{issue: issue}
	runner := &fakeRunner{}
	observer := &fakeObserver{}
	rt := testRuntime(t, tracker, runner, observer)

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeWaitHuman {
		t.Fatalf("outcome = %q, want human wait", result.Outcome)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want no agent run", runner.calls)
	}
	if !observer.sawLog("waiting_for_review") {
		t.Fatalf("logs = %#v, want waiting_for_review", observer.logs)
	}
}

func TestRunIssueTrunkWaitsForBlockedAndInReview(t *testing.T) {
	for _, state := range []string{StateBlocked, "In Review"} {
		t.Run(state, func(t *testing.T) {
			issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "wait", State: state}
			tracker := &fakeTracker{issue: issue}
			runner := &fakeRunner{}
			observer := &fakeObserver{}
			rt := testRuntime(t, tracker, runner, observer)

			result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
			if err != nil {
				t.Fatalf("RunIssueTrunk returned error: %v", err)
			}
			if result.Outcome != OutcomeWaitHuman {
				t.Fatalf("outcome = %q, want human wait", result.Outcome)
			}
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want no agent run", runner.calls)
			}
		})
	}
}

func TestRunIssueTrunkAIReviewPassContinuesToMerging(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "review", State: StateAIReview}
	tracker := &fakeTracker{issue: issue}
	runner := &fakeRunner{agentMessage: "Review: PASS\nlooks good"}
	rt := testRuntime(t, tracker, runner, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeRetryContinuation {
		t.Fatalf("outcome = %q, want retry continuation", result.Outcome)
	}
	if tracker.updateState != StateMerging {
		t.Fatalf("UpdateIssueState = %q, want Merging", tracker.updateState)
	}
	if len(runner.prompts) != 2 || runner.prompts[1].Text != MergingContinuationPromptText || !runner.prompts[1].Continuation {
		t.Fatalf("prompts = %#v, want Merging continuation", runner.prompts)
	}
}

func TestRunIssueTrunkMergePassMarksDone(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "merge", State: StateMerging}
	tracker := &fakeTracker{issue: issue}
	runner := &fakeRunner{agentMessage: "Merge: PASS\nmerged"}
	rt := testRuntime(t, tracker, runner, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeDone {
		t.Fatalf("outcome = %q, want done", result.Outcome)
	}
	if tracker.updateState != StateDone {
		t.Fatalf("UpdateIssueState = %q, want Done", tracker.updateState)
	}
}

func TestRunIssueTrunkInProgressRunsImplementer(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "in progress", State: StateInProgress}
	tracker := &fakeTracker{issue: issue}
	runner := &fakeRunner{}
	observer := &fakeObserver{}
	rt := testRuntime(t, tracker, runner, observer)

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeRetryContinuation {
		t.Fatalf("outcome = %q, want retry continuation", result.Outcome)
	}
	if tracker.updateState != "" {
		t.Fatalf("UpdateIssueState = %q, want no state update", tracker.updateState)
	}
	if !observer.sawStage(PhaseImplementer, StageRunningAgent) {
		t.Fatalf("stages = %#v, want implementer running_agent", observer.stages)
	}
}

func TestRunIssueTrunkTrackerUpdateFailureReturnsFailureRetry(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "todo", State: StateTodo}
	tracker := &fakeTracker{issue: issue, updateErr: errFakeTrackerUpdate}
	rt := testRuntime(t, tracker, &fakeRunner{}, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err == nil {
		t.Fatal("RunIssueTrunk returned nil error, want tracker update failure")
	}
	if result.Outcome != OutcomeRetryFailure {
		t.Fatalf("outcome = %q, want retry failure", result.Outcome)
	}
}

func TestRunIssueTrunkWorkspaceFailureReturnsFailureRetry(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Title: "missing identifier", State: StateInProgress}
	tracker := &fakeTracker{issue: issue}
	rt := testRuntime(t, tracker, &fakeRunner{}, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err == nil {
		t.Fatal("RunIssueTrunk returned nil error, want workspace failure")
	}
	if result.Outcome != OutcomeRetryFailure {
		t.Fatalf("outcome = %q, want retry failure", result.Outcome)
	}
}

func TestRunIssueTrunkPromptFailureReturnsFailureRetry(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "prompt", State: StateInProgress}
	tracker := &fakeTracker{issue: issue}
	rt := testRuntime(t, tracker, &fakeRunner{}, &fakeObserver{})
	rt.Workflow.PromptTemplate = "work on {{ issue.unknown }}"

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err == nil {
		t.Fatal("RunIssueTrunk returned nil error, want prompt render failure")
	}
	if result.Outcome != OutcomeRetryFailure {
		t.Fatalf("outcome = %q, want retry failure", result.Outcome)
	}
}

func TestRunIssueTrunkRunnerFailureReturnsFailureRetry(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "runner", State: StateInProgress}
	tracker := &fakeTracker{issue: issue}
	rt := testRuntime(t, tracker, &fakeRunner{err: errFakeRunner}, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if !errors.Is(err, errFakeRunner) {
		t.Fatalf("RunIssueTrunk error = %v, want fake runner error", err)
	}
	if result.Outcome != OutcomeRetryFailure {
		t.Fatalf("outcome = %q, want retry failure", result.Outcome)
	}
}

func TestRunIssueTrunkTrackerFetchFailureReturnsFailureRetry(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "fetch", State: StateInProgress}
	tracker := &fakeTracker{issue: issue, fetchErr: errFakeTrackerFetch}
	rt := testRuntime(t, tracker, &fakeRunner{}, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if !errors.Is(err, errFakeTrackerFetch) {
		t.Fatalf("RunIssueTrunk error = %v, want fake tracker fetch error", err)
	}
	if result.Outcome != OutcomeRetryFailure {
		t.Fatalf("outcome = %q, want retry failure", result.Outcome)
	}
}

func TestRunIssueTrunkContextCanceledReturnsStopped(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "canceled", State: StateInProgress}
	rt := testRuntime(t, &fakeTracker{issue: issue}, &fakeRunner{}, &fakeObserver{})

	result, err := RunIssueTrunk(ctx, rt, issue, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunIssueTrunk error = %v, want context canceled", err)
	}
	if result.Outcome != OutcomeStopped {
		t.Fatalf("outcome = %q, want stopped", result.Outcome)
	}
}

func TestRunIssueTrunkNonActiveStateReturnsStopped(t *testing.T) {
	issue := issuemodel.Issue{ID: "issue-1", Identifier: "ZEE-1", Title: "paused", State: "Paused"}
	rt := testRuntime(t, &fakeTracker{issue: issue}, &fakeRunner{}, &fakeObserver{})

	result, err := RunIssueTrunk(context.Background(), rt, issue, 0)
	if err != nil {
		t.Fatalf("RunIssueTrunk returned error: %v", err)
	}
	if result.Outcome != OutcomeStopped {
		t.Fatalf("outcome = %q, want stopped", result.Outcome)
	}
}

type fakeTracker struct {
	issue       issuemodel.Issue
	updateID    string
	updateState string
	updateErr   error
	fetchErr    error
}

func (f *fakeTracker) FetchIssue(ctx context.Context, id string) (issuemodel.Issue, error) {
	if f.fetchErr != nil {
		return issuemodel.Issue{}, f.fetchErr
	}
	return f.issue, nil
}

func (f *fakeTracker) UpdateIssueState(ctx context.Context, issueID, stateName string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.updateID = issueID
	f.updateState = stateName
	f.issue.State = stateName
	return nil
}

type fakeRunner struct {
	calls        int
	request      codex.SessionRequest
	prompts      []codex.TurnPrompt
	agentMessage string
	err          error
}

func (f *fakeRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	f.calls++
	f.request = request
	f.prompts = append(f.prompts, request.Prompts...)
	if f.err != nil {
		return codex.SessionResult{}, f.err
	}
	result := codex.Result{SessionID: "session-1", ThreadID: "thread-1", TurnID: "turn-1"}
	if f.agentMessage != "" && onEvent != nil {
		onEvent(codex.Event{Name: "item/completed", Payload: map[string]any{
			"params": map[string]any{
				"item": map[string]any{
					"type": "agentMessage",
					"text": f.agentMessage,
				},
			},
		}})
	}
	if request.AfterTurn != nil {
		next, ok, err := request.AfterTurn(ctx, result, 1)
		if err != nil {
			return codex.SessionResult{}, err
		}
		if ok {
			f.prompts = append(f.prompts, next)
		}
	}
	return codex.SessionResult{SessionID: result.SessionID, ThreadID: result.ThreadID, Turns: []codex.Result{result}}, nil
}

type fakeObserver struct {
	stages []string
	logs   []string
}

func (f *fakeObserver) SetRunningStage(issue issuemodel.Issue, attempt int, phase AgentPhase, stage RunStage, message, workspacePath string, turnCount int) {
	f.stages = append(f.stages, string(phase)+"/"+string(stage))
}

func (f *fakeObserver) RemoveRunning(issueID string) {}

func (f *fakeObserver) LogIssue(issue issuemodel.Issue, event, message string, fields map[string]any) {
	f.logs = append(f.logs, event)
}

func (f *fakeObserver) UpdateRunningFromEvent(issueID string, event codex.Event) {}

func (f *fakeObserver) sawStage(phase AgentPhase, stage RunStage) bool {
	want := string(phase) + "/" + string(stage)
	for _, got := range f.stages {
		if got == want {
			return true
		}
	}
	return false
}

func (f *fakeObserver) sawLog(event string) bool {
	for _, got := range f.logs {
		if got == event {
			return true
		}
	}
	return false
}

func testRuntime(t *testing.T, tracker *fakeTracker, runner *fakeRunner, observer *fakeObserver) Runtime {
	t.Helper()
	return Runtime{
		Workflow: &runtimeconfig.Workflow{
			Config: runtimeconfig.Config{
				Tracker: runtimeconfig.TrackerConfig{
					ActiveStates:   []string{StateTodo, StateInProgress, StateAIReview, StateMerging, StateHumanReview},
					TerminalStates: []string{StateDone},
				},
				Agent: runtimeconfig.AgentConfig{MaxTurns: 2},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), runtimeconfig.HooksConfig{}),
		Runner:    runner,
		Observer:  observer,
	}
}
