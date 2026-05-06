package issueflow

const (
	StateBlocked     = "Blocked"
	StateTodo        = "Todo"
	StateInProgress  = "In Progress"
	StateAIReview    = "AI Review"
	StateMerging     = "Merging"
	StateDone        = "Done"
	StateRework      = "Rework"
	StateHumanReview = "Human Review"

	ActorHuman        = "human"
	ActorOrchestrator = "orchestrator"
	ActorAgent        = "agent"
)

type Step struct {
	Name          string
	Actor         string
	Purpose       string
	CoreInterface string
}

type Transition struct {
	From            string
	To              string
	Actor           string
	CoreInterface   string
	SuccessSignal   string
	FailureHandling string
}

type Definition struct {
	Name          string
	Purpose       string
	Steps         []Step
	Transitions   []Transition
	FailurePolicy []string
}

func DefinitionForTrunk() Definition {
	return Definition{
		Name:    "issue-flow-trunk",
		Purpose: "Human-readable trunk for the issue lifecycle: manual unblock, agent implementation, AI review, merge, and terminal cleanup.",
		Steps: []Step{
			{
				Name:          StateBlocked,
				Actor:         ActorHuman,
				Purpose:       "Hold issues until product, dependency, or review blockers are cleared.",
				CoreInterface: "tracker.UpdateIssueState",
			},
			{
				Name:          StateTodo,
				Actor:         ActorHuman,
				Purpose:       "Mark an unblocked issue as ready for orchestration.",
				CoreInterface: "orchestrator.candidateEligible",
			},
			{
				Name:          StateInProgress,
				Actor:         ActorOrchestrator,
				Purpose:       "Prepare workspace, render workflow prompt, and run the implementer agent.",
				CoreInterface: "orchestrator.runPhaseAgent(implementer)",
			},
			{
				Name:          StateAIReview,
				Actor:         ActorAgent,
				Purpose:       "Review the issue diff, evidence, and workpad in the same issue session.",
				CoreInterface: "orchestrator.runPhaseAgent(reviewer)",
			},
			{
				Name:          StateMerging,
				Actor:         ActorAgent,
				Purpose:       "Run the merge protocol after review passes and report merge evidence.",
				CoreInterface: "orchestrator.runPhaseAgent(reviewer)",
			},
			{
				Name:          StateDone,
				Actor:         ActorOrchestrator,
				Purpose:       "Mark terminal completion and clean the issue workspace through workspace manager hooks.",
				CoreInterface: "orchestrator.cleanupTerminalIssueAfterExit",
			},
		},
		Transitions: []Transition{
			{
				From:            StateBlocked,
				To:              StateTodo,
				Actor:           ActorHuman,
				CoreInterface:   "tracker.UpdateIssueState",
				SuccessSignal:   "Linear state is Todo and blockers are terminal or removed.",
				FailureHandling: "No automatic dispatch; candidateEligible reports blockers until a human clears them.",
			},
			{
				From:            StateTodo,
				To:              StateInProgress,
				Actor:           ActorOrchestrator,
				CoreInterface:   "orchestrator.runAgentWith",
				SuccessSignal:   "state_changed Todo -> In Progress is logged before the implementer run.",
				FailureHandling: "Tracker update errors abort the run and enter the failure retry path.",
			},
			{
				From:            StateInProgress,
				To:              StateAIReview,
				Actor:           ActorAgent,
				CoreInterface:   "codex.RunSession",
				SuccessSignal:   "Agent updates the issue to AI Review after implementation evidence is ready.",
				FailureHandling: "Runner, workspace, or prompt errors schedule exponential failure retry.",
			},
			{
				From:            StateAIReview,
				To:              StateMerging,
				Actor:           ActorOrchestrator,
				CoreInterface:   "reviewFinalPasses",
				SuccessSignal:   "Reviewer final message starts with Review: PASS.",
				FailureHandling: "Reviewer findings move the issue to Rework or Human Review; no retry is scheduled for human wait states.",
			},
			{
				From:            StateMerging,
				To:              StateDone,
				Actor:           ActorOrchestrator,
				CoreInterface:   "mergeFinalPasses",
				SuccessSignal:   "Merge final message starts with Merge: PASS.",
				FailureHandling: "Without Merge: PASS the issue stays active until max turns or retry policy stops the run.",
			},
		},
		FailurePolicy: []string{
			"Blocked and Human Review states are deliberate human wait points and do not auto-dispatch.",
			"Workspace, prompt, tracker, and runner errors schedule retryFailure with capped exponential backoff.",
			"Completed turns that leave an issue active schedule retryContinuation after a short delay.",
			"Stalled running entries are canceled and requeued through the same retryFailure path.",
			"Terminal issues trigger workspace cleanup through workspace.Manager.Remove and before_remove hooks.",
		},
	}
}
