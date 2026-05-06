package issueflow

import issuemodel "symphony-go/internal/service/issue"

const (
	ContinuationPromptText = "Continue working on the same issue. Re-check the current workspace state, finish any remaining acceptance criteria from the issue, run the smallest relevant verification, and report concrete progress or blockers. Do not repeat completed work."

	AIReviewContinuationPromptText = "Continue in the same issue session and execute the AI Review protocol for this issue. Re-check the issue, workpad, diff, commits, review feedback, and validation evidence. If the work is correct, report Review: PASS so the orchestrator can continue to Merging in this same session; if it is not correct, record actionable findings, move the issue to Rework, and report concrete blockers."
	MergingContinuationPromptText  = "Continue in the same issue session and execute the Merging protocol for this issue. Use the PR skill fast path: confirm the PR skill was opened, confirm pr_merge_flow.sh is executable, prepare the PR title/body, run the script, then update the workpad once with merge evidence. Do not move Linear to Done from the agent; final reply must start with Merge: PASS and include PR URL, merge commit, and root status so the orchestrator can mark Done."
)

type AgentPhase string

const (
	PhaseImplementer AgentPhase = "implementer"
	PhaseReviewer    AgentPhase = "reviewer"
)

type RunStage string

const (
	StageQueued                   RunStage = "queued"
	StagePreparingWorkspace       RunStage = "preparing_workspace"
	StageRunningWorkspaceHooks    RunStage = "running_workspace_hooks"
	StageRenderingPrompt          RunStage = "rendering_prompt"
	StageRunningAgent             RunStage = "running_agent"
	StageContinuingAIReview       RunStage = "continuing_ai_review"
	StageContinuingMerging        RunStage = "continuing_merging"
	StageContinuingImplementation RunStage = "continuing_implementation"
)

type Outcome string

const (
	OutcomeDone              Outcome = "done"
	OutcomeWaitHuman         Outcome = "wait_human"
	OutcomeRetryFailure      Outcome = "retry_failure"
	OutcomeRetryContinuation Outcome = "retry_continuation"
	OutcomeStopped           Outcome = "stopped"
)

type Result struct {
	Outcome Outcome
}

type TurnResult struct {
	Issue            issuemodel.Issue
	LastAgentMessage string
}
