# Agent Driven Review Phases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Symphony Go align with `SPEC.md` by letting agents own workflow state transitions, adding a real reviewer agent phase, and keeping reviewer-approved merge work in the same reviewer session instead of spawning a separate merge agent.

**Architecture:** Keep the orchestrator as the scheduler, runner, tracker reader, retry owner, and cleanup owner. Move phase-specific behavior into focused files: implementer sessions for `Todo` / `In Progress` / `Rework`, reviewer sessions for `AI Review`, and merge continuation inside the reviewer session when the reviewer moves the issue to `Merging`. Split the long `orchestrator.go` so the main loop remains readable and phase logic is isolated.

**Tech Stack:** Go, existing `internal/codex` app-server runner, existing `internal/workflow` prompt renderer, existing Linear tracker abstraction, repository scripts `./test.sh` and `./build.sh`.

---

## Design Commitments

- `SPEC.md:36-42` says Symphony is a scheduler/runner/tracker reader and ticket writes are typically performed by the coding agent. Therefore the framework must not treat "new commit exists" as "ready for review".
- `SPEC.md:637-644` and `SPEC.md:959-997` say a worker may run multiple turns in the same live coding-agent thread. Therefore implementer work should continue in the same session until the agent changes tracker state or `max_turns` is reached.
- `AI Review` must run a real reviewer agent. The deterministic checks currently in `aiReviewAfterCommit` should become a preflight helper or be removed from the state transition path.
- Normal approved path should be `implementer agent -> AI Review state -> reviewer agent -> Merging state -> merge in same reviewer session -> Done`.
- If the service starts while an issue is already in `Merging`, there is no live reviewer session to reuse. In that resume case, the orchestrator may start a merge-capable reviewer session for `Merging`.

## File Structure

- Modify `internal/orchestrator/orchestrator.go`
  - Keep: public types, `New`, `Snapshot`, `Run`, `pollDispatched`, reconciliation entrypoints, top-level orchestration.
  - Remove from this file: phase-specific `runAgentWith`, review policy helpers, git helpers, deterministic review helpers.
- Create `internal/orchestrator/agent_session.go`
  - Own generic Codex session startup, workspace ensure / before-run / after-run, continuation loop, event logging, state refresh after each turn.
  - Export no symbols; expose methods on `*Orchestrator`.
- Create `internal/orchestrator/phases.go`
  - Own phase routing for `Todo`, `In Progress`, `Rework`, `AI Review`, `Merging`, `Human Review`, terminal states.
  - Own phase prompt construction for implementer and reviewer.
- Create `internal/orchestrator/review_policy.go`
  - Move `reviewPolicy`, `effectiveReviewPolicy`, legacy compatibility, and any deterministic preflight helpers.
  - Rename `aiReviewAfterCommit` to `reviewPreflight` if it remains useful; do not let it move tracker state by itself.
- Create `internal/orchestrator/git_helpers.go`
  - Move `gitOutput`, `nonEmptyLines`, `sameStringSet`, and related small helpers.
- Modify `internal/orchestrator/orchestrator_test.go`
  - Replace tests that expect auto transition on commit.
  - Add tests for agent-owned transition, reviewer agent execution, and same-session review-to-merge continuation.
- Modify `WORKFLOW.md`
  - Replace "orchestrator moves to AI Review/Merging based on HEAD" with "agent moves to AI Review when acceptance and validation are complete".
  - Describe reviewer agent responsibilities and same-session merge continuation after approval.

## Task 1: Lock New State Ownership Semantics With Failing Tests

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: Add a fake runner that lets the test agent mutate tracker state**

Add this helper near the existing runner test doubles at the bottom of `internal/orchestrator/orchestrator_test.go`:

```go
type stateChangingRunner struct {
	tracker *recordingTracker
	state   string
	commit  bool
}

func (r stateChangingRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	if r.commit {
		if err := commitFile(ctx, request.WorkspacePath, "README.md", "agent owned transition\n", "agent commit"); err != nil {
			return codex.SessionResult{}, err
		}
	}
	if r.state != "" {
		if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, r.state); err != nil {
			return codex.SessionResult{}, err
		}
	}
	return codex.SessionResult{SessionID: "agent-state-session"}, nil
}
```

- [ ] **Step 2: Replace the auto-merge expectation test with an agent-owned transition test**

Remove `TestRunAgentReviewPolicyAutoMergesWhenAIReviewPasses` and add this test in its place:

```go
func TestRunAgentDoesNotAutoPromoteAfterCommitWhenAgentMovesToAIReview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := types.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-AGENT-REVIEW",
		Title:      "agent owned review transition",
		State:      "In Progress",
	}
	tracker := &recordingTracker{issue: issue}
	o := New(Options{
		Workflow: &types.Workflow{
			Config: types.Config{
				Agent: types.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: types.ReviewPolicyConfig{
						Mode:     "auto",
						OnAIFail: "rework",
					},
				},
			},
			PromptTemplate: "work on {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    stateChangingRunner{tracker: tracker, state: "AI Review", commit: true},
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if got, want := tracker.states, []string{"AI Review"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 3: Run the targeted test and verify it fails before implementation**

Run:

```bash
./test.sh ./internal/orchestrator -run TestRunAgentDoesNotAutoPromoteAfterCommitWhenAgentMovesToAIReview
```

Expected before implementation: FAIL because current `handoffAfterTurn` adds framework-owned `AI Review` and `Merging` transitions after the commit.

- [ ] **Step 4: Commit only the failing test**

Run:

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "test(orchestrator): lock agent-owned review transition"
```

Expected: one commit containing only the new/changed test helper and test.

## Task 2: Remove Commit-Based Auto Promotion

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: Replace `handoffAfterTurn` call in `runAgentWith` with tracker refresh only**

In `internal/orchestrator/orchestrator.go`, replace the body of `request.AfterTurn` with this behavior:

```go
request.AfterTurn = func(ctx context.Context, result codex.Result, turn int) (codex.TurnPrompt, bool, error) {
	o.logIssue(issue, "turn_completed", "Codex turn completed", map[string]any{"session_id": result.SessionID})
	refreshed, err := rt.tracker.FetchIssue(ctx, issue.ID)
	if err != nil {
		return codex.TurnPrompt{}, false, err
	}
	if !isActive(refreshed.State, rt.workflow.Config.Tracker.ActiveStates) || isTerminal(refreshed.State, rt.workflow.Config.Tracker.TerminalStates) {
		return codex.TurnPrompt{}, false, nil
	}
	if refreshed.State == "Human Review" || refreshed.State == "In Review" || refreshed.State == "AI Review" || refreshed.State == "Merging" {
		nextIssue = &refreshed
		return codex.TurnPrompt{}, false, nil
	}
	if turn >= maxTurns {
		maxTurnsReached = true
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
	return codex.TurnPrompt{Text: continuationPromptText, Continuation: true}, true, nil
}
```

- [ ] **Step 2: Delete framework-owned auto promotion from `handoffAfterTurn`**

Remove `handoffAfterTurn` entirely after the tests no longer reference it. The replacement logic in Step 1 makes `HEAD` changes informational rather than state-driving.

- [ ] **Step 3: Adjust next-state recursion after the session**

In `runAgentWith`, keep the existing "do not retry for `AI Review` / `Rework`" behavior for now:

```go
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
```

Task 4 will change `AI Review` from no-retry to reviewer execution.

- [ ] **Step 4: Run the targeted test**

Run:

```bash
./test.sh ./internal/orchestrator -run TestRunAgentDoesNotAutoPromoteAfterCommitWhenAgentMovesToAIReview
```

Expected: PASS.

- [ ] **Step 5: Run existing continuation tests**

Run:

```bash
./test.sh ./internal/orchestrator -run 'TestRunAgentContinues|TestRunAgentDoesNotMoveToReviewWithoutCommit'
```

Expected: PASS. If `TestRunAgentDoesNotMoveToReviewWithoutCommit` asserts legacy behavior, update it so it asserts no tracker state update occurs without an agent-initiated tracker update.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "fix(orchestrator): stop commit-based state promotion"
```

Expected: one implementation commit; no workflow documentation changes yet.

## Task 3: Add Real Reviewer Agent Behavior

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`
- Modify: `internal/orchestrator/orchestrator.go`

- [ ] **Step 1: Add a failing reviewer test**

Add this test near the other review-policy tests:

```go
func TestAIReviewStateRunsReviewerAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := types.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEWER",
		Title:      "reviewer agent",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := stateChangingRunner{tracker: tracker, state: "Rework", commit: false}
	o := New(Options{
		Workflow: &types.Workflow{
			Config: types.Config{
				Agent: types.AgentConfig{
					MaxTurns: 1,
					ReviewPolicy: types.ReviewPolicyConfig{
						Mode:     "auto",
						OnAIFail: "rework",
					},
				},
			},
			PromptTemplate: "review {{ issue.identifier }} in state {{ issue.state }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if got, want := tracker.states, []string{"Rework"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: Run the reviewer test and verify current behavior fails**

Run:

```bash
./test.sh ./internal/orchestrator -run TestAIReviewStateRunsReviewerAgent
```

Expected before implementation: FAIL because current `reviewIssueState` performs deterministic review logic without invoking `Runner`.

- [ ] **Step 3: Change `AI Review` routing to run a reviewer agent**

In `runAgentWith`, replace this branch:

```go
case "AI Review":
	return o.reviewIssueState(ctx, rt, issue)
```

with:

```go
case "AI Review":
	return o.runReviewerAgent(ctx, rt, issue, attempt)
```

Add `runReviewerAgent` in `internal/orchestrator/orchestrator.go` for this task:

```go
func (o *Orchestrator) runReviewerAgent(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int) error {
	return o.runPhaseAgent(ctx, rt, issue, attempt, phaseReviewer)
}
```

Add a small phase type near the existing constants:

```go
type agentPhase string

const (
	phaseImplementer agentPhase = "implementer"
	phaseReviewer    agentPhase = "reviewer"
)
```

- [ ] **Step 4: Extract current `runAgentWith` body into `runPhaseAgent`**

Rename the current `runAgentWith` implementation body to:

```go
func (o *Orchestrator) runPhaseAgent(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int, phase agentPhase) error {
	// move the existing session startup and continuation logic here
}
```

Then make `runAgentWith` route implementer states:

```go
func (o *Orchestrator) runAgentWith(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int) error {
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
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	case "In Progress", "Rework":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	case "AI Review":
		return o.runReviewerAgent(ctx, rt, issue, attempt)
	case "Human Review", "In Review":
		o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
		return errNoRetryNeeded
	case "Merging":
		return o.runReviewerAgent(ctx, rt, issue, attempt)
	default:
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	}
}
```

- [ ] **Step 5: Run the reviewer test**

Run:

```bash
./test.sh ./internal/orchestrator -run TestAIReviewStateRunsReviewerAgent
```

Expected: PASS.

- [ ] **Step 6: Commit**

Run:

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "feat(orchestrator): run reviewer agent for AI Review"
```

Expected: commit contains reviewer routing but not large file splitting yet.

## Task 4: Reuse Reviewer Session For Merging After Approval

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`
- Modify: `internal/orchestrator/orchestrator.go`

- [ ] **Step 1: Add a runner that exercises same-session phase continuation**

Add this helper near the other test runners:

```go
type reviewThenMergeRunner struct {
	tracker  *recordingTracker
	requests []codex.SessionRequest
}

func (r *reviewThenMergeRunner) RunSession(ctx context.Context, request codex.SessionRequest, _ func(codex.Event)) (codex.SessionResult, error) {
	r.requests = append(r.requests, request)
	if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, "Merging"); err != nil {
		return codex.SessionResult{}, err
	}
	if request.AfterTurn == nil {
		return codex.SessionResult{SessionID: "review-turn-1"}, nil
	}
	nextPrompt, ok, err := request.AfterTurn(ctx, codex.Result{SessionID: "review-thread-turn-1", ThreadID: "review-thread", TurnID: "turn-1"}, 1)
	if err != nil {
		return codex.SessionResult{}, err
	}
	if !ok {
		return codex.SessionResult{SessionID: "review-thread-turn-1"}, nil
	}
	if !nextPrompt.Continuation {
		return codex.SessionResult{}, fmt.Errorf("merge continuation prompt was not marked as continuation")
	}
	if err := r.tracker.UpdateIssueState(ctx, request.Issue.ID, "Done"); err != nil {
		return codex.SessionResult{}, err
	}
	_, ok, err = request.AfterTurn(ctx, codex.Result{SessionID: "review-thread-turn-2", ThreadID: "review-thread", TurnID: "turn-2"}, 2)
	if err != nil {
		return codex.SessionResult{}, err
	}
	if ok {
		return codex.SessionResult{}, fmt.Errorf("expected session to stop after Done")
	}
	return codex.SessionResult{
		SessionID: "review-thread-turn-2",
		ThreadID:  "review-thread",
		Turns: []codex.Result{
			{SessionID: "review-thread-turn-1", ThreadID: "review-thread", TurnID: "turn-1"},
			{SessionID: "review-thread-turn-2", ThreadID: "review-thread", TurnID: "turn-2"},
		},
	}, nil
}
```

- [ ] **Step 2: Add a failing same-session test**

Add this test:

```go
func TestReviewerAgentContinuesIntoMergingInSameSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	issue := types.Issue{
		ID:         "issue-id",
		Identifier: "ZEE-REVIEW-MERGE",
		Title:      "review then merge",
		State:      "AI Review",
	}
	tracker := &recordingTracker{issue: issue}
	runner := &reviewThenMergeRunner{tracker: tracker}
	o := New(Options{
		Workflow: &types.Workflow{
			Config: types.Config{
				Agent: types.AgentConfig{MaxTurns: 3},
				Tracker: types.TrackerConfig{
					ActiveStates:   []string{"In Progress", "AI Review", "Merging", "Rework"},
					TerminalStates: []string{"Done"},
				},
			},
			PromptTemplate: "state {{ issue.state }} for {{ issue.identifier }}",
		},
		Tracker:   tracker,
		Workspace: workspace.New(filepath.Join(t.TempDir(), ".worktrees"), gitSeedHook()),
		Runner:    runner,
	})

	if err := o.runAgent(ctx, issue, 0); err != nil {
		t.Fatalf("runAgent returned error: %v", err)
	}

	if len(runner.requests) != 1 {
		t.Fatalf("runner sessions = %d, want 1", len(runner.requests))
	}
	if got, want := tracker.states, []string{"Merging", "Done"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state updates = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 3: Run the same-session test and verify current behavior fails**

Run:

```bash
./test.sh ./internal/orchestrator -run TestReviewerAgentContinuesIntoMergingInSameSession
```

Expected before implementation: FAIL because the current session loop stops on `Merging` and then starts a separate run.

- [ ] **Step 4: Make `Merging` a continuation state for reviewer phase**

Inside `runPhaseAgent`'s `AfterTurn`, replace the hard stop for `Merging` with phase-aware behavior:

```go
if refreshed.State == "Human Review" || refreshed.State == "In Review" || refreshed.State == "AI Review" {
	nextIssue = &refreshed
	return codex.TurnPrompt{}, false, nil
}
if refreshed.State == "Merging" && phase != phaseReviewer {
	nextIssue = &refreshed
	return codex.TurnPrompt{}, false, nil
}
if refreshed.State == "Merging" && phase == phaseReviewer {
	issue = refreshed
	o.setRunning(observability.RunningEntry{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		State:           issue.State,
		WorkspacePath:   workspacePath,
		TurnCount:       turn + 1,
		StartedAt:       time.Now(),
	})
	return codex.TurnPrompt{Text: mergingContinuationPromptText, Continuation: true}, true, nil
}
```

Add this constant near `continuationPromptText`:

```go
const mergingContinuationPromptText = "The reviewer has approved the issue and moved it to Merging. Continue in this same reviewer session as the merge operator: re-check the issue worktree branch, confirm the repo root main checkout is safe, run the workflow-defined merge protocol, run required validation, push main if required by the workflow, update the single workpad with merge evidence, and move the issue to Done only after merge, validation, and push succeed."
```

- [ ] **Step 5: Avoid recursive new reviewer session after same-session merge**

In the `nextIssue` handling after `RunSession`, keep recursion for direct `Merging` resume only. The same-session path reaches terminal `Done` and should leave `nextIssue == nil`.

Use this shape:

```go
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
```

- [ ] **Step 6: Run same-session test**

Run:

```bash
./test.sh ./internal/orchestrator -run TestReviewerAgentContinuesIntoMergingInSameSession
```

Expected: PASS and exactly one runner session.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go
git commit -m "feat(orchestrator): let reviewer session continue into merging"
```

Expected: behavior change is isolated to phase continuation.

## Task 5: Split Orchestrator Into Focused Files

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Create: `internal/orchestrator/agent_session.go`
- Create: `internal/orchestrator/phases.go`
- Create: `internal/orchestrator/review_policy.go`
- Create: `internal/orchestrator/git_helpers.go`

- [ ] **Step 1: Move generic agent session code**

Create `internal/orchestrator/agent_session.go`:

```go
package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/types"
	"github.com/zeefan1555/symphony-go/internal/workflow"
	"github.com/zeefan1555/symphony-go/internal/workspace"
)

func (o *Orchestrator) runPhaseAgent(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int, phase agentPhase) error {
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
	prompt = phasePrompt(phase, prompt)
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
			return codex.TurnPrompt{}, false, nil
		}
		if refreshed.State == "Human Review" || refreshed.State == "In Review" || refreshed.State == "AI Review" {
			nextIssue = &refreshed
			return codex.TurnPrompt{}, false, nil
		}
		if refreshed.State == "Merging" && phase != phaseReviewer {
			nextIssue = &refreshed
			return codex.TurnPrompt{}, false, nil
		}
		if refreshed.State == "Merging" && phase == phaseReviewer {
			issue = refreshed
			o.setRunning(observability.RunningEntry{
				IssueID:         issue.ID,
				IssueIdentifier: issue.Identifier,
				State:           issue.State,
				WorkspacePath:   workspacePath,
				TurnCount:       turn + 1,
				StartedAt:       time.Now(),
			})
			return codex.TurnPrompt{Text: mergingContinuationPromptText, Continuation: true}, true, nil
		}
		if turn >= maxTurns {
			maxTurnsReached = true
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
	return nil
}

func phasePrompt(phase agentPhase, rendered string) string {
	switch phase {
	case phaseReviewer:
		return reviewerPromptPrefix + "\n\n" + rendered
	default:
		return rendered
	}
}

func normalizeStateName(value string) string {
	return strings.TrimSpace(value)
}
```

- [ ] **Step 2: Move phase routing**

Create `internal/orchestrator/phases.go`:

```go
package orchestrator

import (
	"context"

	"github.com/zeefan1555/symphony-go/internal/types"
)

type agentPhase string

const (
	phaseImplementer agentPhase = "implementer"
	phaseReviewer    agentPhase = "reviewer"
)

const reviewerPromptPrefix = "You are the AI Review agent for this issue. Review the issue, workpad, diff, commits, and validation evidence. If the work is correct, update the workpad with concise review evidence, move the issue to Merging, and continue with the merge protocol in this same session. If the work is not correct, write actionable findings to the workpad and move the issue to Rework. If an external permission, secret, or tool is missing, move the issue to Human Review with a concise blocker brief."

const mergingContinuationPromptText = "The reviewer has approved the issue and moved it to Merging. Continue in this same reviewer session as the merge operator: re-check the issue worktree branch, confirm the repo root main checkout is safe, run the workflow-defined merge protocol, run required validation, push main if required by the workflow, update the single workpad with merge evidence, and move the issue to Done only after merge, validation, and push succeed."

func (o *Orchestrator) runAgentWith(ctx context.Context, rt runtimeSnapshot, issue types.Issue, attempt int) error {
	issue.State = normalizeStateName(issue.State)
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
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	case "In Progress", "Rework":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	case "AI Review", "Merging":
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseReviewer)
	case "Human Review", "In Review":
		o.logIssue(issue, "waiting_for_review", "issue is waiting for human review", nil)
		return errNoRetryNeeded
	default:
		return o.runPhaseAgent(ctx, rt, issue, attempt, phaseImplementer)
	}
}
```

- [ ] **Step 3: Move review policy and preflight helpers**

Create `internal/orchestrator/review_policy.go`:

```go
package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/zeefan1555/symphony-go/internal/types"
)

const (
	reviewPolicyHuman = "human"
	reviewPolicyAI    = "ai"
	reviewPolicyAuto  = "auto"
	aiFailRework      = "rework"
	aiFailHold        = "hold"
)

type reviewPolicy struct {
	mode                 string
	allowManualAIReview  bool
	onAIFail             string
	expectedChangedFiles []string
}

func effectiveReviewPolicy(agent types.AgentConfig) reviewPolicy {
	cfg := agent.ReviewPolicy
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		return legacyReviewPolicy(agent.AIReview)
	}
	if mode != reviewPolicyAI && mode != reviewPolicyAuto {
		mode = reviewPolicyHuman
	}
	onFail := strings.ToLower(strings.TrimSpace(cfg.OnAIFail))
	if onFail == "" {
		onFail = aiFailRework
	}
	if onFail != aiFailRework {
		onFail = aiFailHold
	}
	expected := cfg.ExpectedChangedFiles
	if len(expected) == 0 {
		expected = agent.AIReview.ExpectedChangedFiles
	}
	return reviewPolicy{
		mode:                 mode,
		allowManualAIReview:  cfg.AllowManualAIReview,
		onAIFail:             onFail,
		expectedChangedFiles: expected,
	}
}

func legacyReviewPolicy(review types.AIReviewConfig) reviewPolicy {
	policy := reviewPolicy{
		mode:     reviewPolicyHuman,
		onAIFail: aiFailRework,
	}
	if review.Enabled {
		policy.mode = reviewPolicyAI
		if review.AutoMerge {
			policy.mode = reviewPolicyAuto
		}
		if !review.ReworkOnFailure {
			policy.onAIFail = aiFailHold
		}
		policy.expectedChangedFiles = review.ExpectedChangedFiles
	}
	return policy
}

type reviewPreflightOutcome struct {
	Passed       bool
	Summary      string
	Reasons      []string
	ChangedFiles []string
}

func reviewPreflight(ctx context.Context, policy reviewPolicy, workspacePath, baseHead, head, changedFiles, status string) reviewPreflightOutcome {
	outcome := reviewPreflightOutcome{
		Passed:       true,
		Summary:      "review preflight passed",
		ChangedFiles: nonEmptyLines(changedFiles),
	}
	if strings.TrimSpace(status) != "" {
		outcome.Passed = false
		outcome.Reasons = append(outcome.Reasons, "worktree still has uncommitted changes")
	}
	if baseHead != "" && head != "" {
		if _, err := gitOutput(ctx, workspacePath, "diff", "--check", baseHead+".."+head); err != nil {
			outcome.Passed = false
			outcome.Reasons = append(outcome.Reasons, "git diff --check failed: "+err.Error())
		}
	}
	expected := policy.expectedChangedFiles
	if len(expected) > 0 && !sameStringSet(outcome.ChangedFiles, expected) {
		outcome.Passed = false
		outcome.Reasons = append(outcome.Reasons, fmt.Sprintf("changed files mismatch: got %v want %v", outcome.ChangedFiles, expected))
	}
	if !outcome.Passed {
		outcome.Summary = "review preflight failed"
	}
	return outcome
}
```

This keeps legacy helpers available for follow-up work without using them as the AI Review decision.

- [ ] **Step 4: Move git helpers**

Create `internal/orchestrator/git_helpers.go` and move the existing helper implementations without changing behavior:

```go
package orchestrator

import (
	"context"
	"os/exec"
	"strings"
)

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func nonEmptyLines(value string) []string {
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := map[string]int{}
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}
```

- [ ] **Step 5: Shrink imports in `orchestrator.go`**

After moving code, `internal/orchestrator/orchestrator.go` should no longer import `os/exec`, and it should only keep imports used by the main orchestration loop. Run:

```bash
gofmt -w internal/orchestrator/orchestrator.go internal/orchestrator/agent_session.go internal/orchestrator/phases.go internal/orchestrator/review_policy.go internal/orchestrator/git_helpers.go
```

Expected: files format cleanly.

- [ ] **Step 6: Run orchestrator tests**

Run:

```bash
./test.sh ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 7: Commit**

Run:

```bash
git add internal/orchestrator/orchestrator.go internal/orchestrator/agent_session.go internal/orchestrator/phases.go internal/orchestrator/review_policy.go internal/orchestrator/git_helpers.go internal/orchestrator/orchestrator_test.go
git commit -m "refactor(orchestrator): split phase and session flow"
```

Expected: refactor commit with passing tests and no behavior drift beyond Tasks 2-4.

## Task 6: Update Workflow Contract Text

**Files:**
- Modify: `WORKFLOW.md`

- [ ] **Step 1: Replace review routing language**

In `WORKFLOW.md`, replace the current Review 路由原则 section with:

```markdown
## Review 路由原则

- `agent.review_policy.mode` 描述 review 目标路径，但不让 orchestrator 根据 commit 自动推进业务状态。
- 当前默认使用 `mode: auto`，目标路径是 `In Progress -> AI Review -> Merging -> Done`。
- `In Progress` / `Rework` 的 implementer agent 可以多 turn、多 commit；只有当它确认 acceptance、validation、workpad 和 commit 都完成后，才移动 issue 到 `AI Review`。
- `AI Review` 由真实 reviewer agent 执行。Reviewer 必须读取 issue、workpad、diff、commit range 和验证证据。
- Reviewer 通过时，先把 issue 移动到 `Merging`，然后在同一个 reviewer session 中继续执行 merge 协议；不额外启动独立 merger agent。
- Reviewer 不通过时，必须把具体发现写入同一个 workpad，并移动 issue 到 `Rework`。
- `Human Review` 只作为真实外部 blocker 的人工 hold 状态；默认流程不得依赖人工把 issue 从 `Human Review` 推到 `Merging`。
```

- [ ] **Step 2: Replace completion handoff language**

Replace the existing lines that say the orchestrator moves based on `HEAD` with:

```markdown
10. 不要把“已有 commit”当成自动 handoff 信号。
    - Implementer 可以按任务需要产生多个 commit。
    - 只有完成 acceptance、validation、workpad 更新和最终 commit 后，implementer agent 才移动 issue 到 `AI Review`。
    - Orchestrator 只负责刷新 tracker state、续跑 active issue、记录事件和处理 cleanup/retry，不根据 `HEAD` 变化自行进入 `AI Review` 或 `Merging`。
```

- [ ] **Step 3: Replace AI Review step language**

Replace Step 3 with:

```markdown
## Step 3：AI Review、Rework 与 main merge 处理

1. 当 issue 处于 `AI Review`，orchestrator 启动 reviewer agent，而不是执行内置伪 review。
2. Reviewer agent 必须审查 issue、workpad、diff、commit range 和验证证据。
3. 如果 reviewer 判断需要修改，更新 workpad review findings 并移动 issue 到 `Rework`。
4. 如果 reviewer 判断可以合入，移动 issue 到 `Merging`，并在同一个 reviewer session 中继续执行 `Main merge 协议`。
5. Merge、验证和 push 全部成功后，更新 workpad 证据并移动 issue 到 `Done`。
6. `Human Review` 只处理真实外部 blocker；人工解锁后应回到 `Todo`、`In Progress` 或 `AI Review` 继续自动流程。
```

- [ ] **Step 4: Run documentation validation**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 5: Commit**

Run:

```bash
git add WORKFLOW.md
git commit -m "docs(workflow): define agent-owned review phases"
```

Expected: docs-only commit.

## Task 7: Full Local Verification

**Files:**
- No new files.
- Validate: whole repository behavior.

- [ ] **Step 1: Run orchestrator package tests**

Run:

```bash
./test.sh ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 2: Run Codex runner tests**

Run:

```bash
./test.sh ./internal/codex
```

Expected: PASS. This protects turn/session continuation and Merging sandbox roots.

- [ ] **Step 3: Run full test script**

Run:

```bash
./test.sh
```

Expected: PASS. If unrelated package failures appear, capture exact package and failure text before deciding whether they belong to this change.

- [ ] **Step 4: Run build**

Run:

```bash
./build.sh
```

Expected: PASS. If the local Go cache causes permission errors, run:

```bash
GOCACHE=/tmp/symphony-go-gocache ./build.sh
```

Expected with fallback: PASS.

- [ ] **Step 5: Run whitespace validation**

Run:

```bash
git diff --check
```

Expected: no output and exit code 0.

- [ ] **Step 6: Commit verification notes if a tracked plan or docs file was updated**

If this plan file is being updated during implementation, commit it with:

```bash
git add docs/superpowers/plans/2026-05-02-agent-driven-review-phases.md
git commit -m "docs(plan): record agent-driven review phase plan"
```

Expected: commit only if the plan file changed during execution.

## Task 8: Real Workflow Smoke

**Files:**
- Modify only experiment records if the smoke harness is used:
  - `.codex/skills/spec-gap-harness/experiments/run_log.tsv`
  - `.codex/skills/spec-gap-harness/experiments/gaps.tsv`
  - `.codex/skills/spec-gap-harness/experiments/coverage.tsv`
  - `.codex/skills/spec-gap-harness/experiments/rounds.md`

- [ ] **Step 1: Create a SPEC gap smoke issue**

Use the existing `spec-gap-harness` pattern to create one issue that requires at least two implementer commits before review. The issue body must include:

```markdown
Validation:
- `rg -n "phase handoff smoke" SPEC_SMOKE.md`
- `git diff --check`
- `git status --short`

Acceptance:
- Implementer must create at least two local commits before moving to AI Review.
- AI Review must be performed by a reviewer agent.
- Reviewer approval must continue into Merging in the same reviewer session.
- Final state must be Done after merge validation and push.
```

- [ ] **Step 2: Run the listener**

Run:

```bash
make build
./bin/symphony-go run --workflow ./WORKFLOW.md --no-tui --issue <ISSUE-ID>
```

Expected:
- Issue moves from `Todo` to `In Progress`.
- Implementer creates two commits and moves issue to `AI Review`.
- Human log shows a reviewer session starts for `AI Review`.
- Human log shows the same reviewer session continues after `Merging`.
- Issue reaches `Done`.

- [ ] **Step 3: Verify no commit-based auto handoff happened**

Inspect the human log:

```bash
rg -n "state_changed|turn_started|session_started|AI Review|Merging|Done" .symphony/logs/<run>.human.log
```

Expected:
- No framework log line says `In Progress -> AI Review` immediately after the first commit.
- The workpad or Linear history shows the implementer moved to `AI Review` after final validation.
- Reviewer session evidence appears before `Merging`.

- [ ] **Step 4: Record smoke result**

Append the result to the spec-gap harness ledgers:

```bash
git add .codex/skills/spec-gap-harness/experiments/run_log.tsv .codex/skills/spec-gap-harness/experiments/gaps.tsv .codex/skills/spec-gap-harness/experiments/coverage.tsv .codex/skills/spec-gap-harness/experiments/rounds.md
git commit -m "docs(spec-gap): record agent-driven review smoke"
```

Expected: experiment files record issue ID, commits, logs, verdict, and any next gap.

## Self-Review

**Spec coverage:** Tasks 1-4 cover `SPEC.md` session continuation and agent-owned ticket writes. Task 5 covers orchestrator readability and file boundaries. Task 6 updates workflow contract language. Tasks 7-8 cover local and real workflow verification.

**Placeholder scan:** The plan avoids placeholder markers and gives exact file paths, commands, test names, and code snippets for each code-changing step.

**Type consistency:** The plan uses existing `codex.SessionRequest`, `codex.TurnPrompt`, `types.Issue`, `types.Workflow`, `types.AgentConfig`, `recordingTracker`, and existing test helpers. New names are consistently `agentPhase`, `phaseImplementer`, `phaseReviewer`, `runPhaseAgent`, `phasePrompt`, `reviewPreflight`, and `mergingContinuationPromptText`.
