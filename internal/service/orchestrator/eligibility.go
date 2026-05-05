package orchestrator

import (
	"sort"
	"strings"
	"time"

	issuemodel "symphony-go/internal/service/issue"
)

type runningIssue struct {
	state string
}

type eligibilityState struct {
	activeStates   []string
	terminalStates []string
	claimed        map[string]bool
	running        map[string]runningIssue
	maxConcurrent  int
	perState       map[string]int
	aiReview       bool
}

func candidateEligible(issue issuemodel.Issue, state eligibilityState) (bool, string) {
	if issue.ID == "" || issue.Identifier == "" || issue.Title == "" || issue.State == "" {
		return false, "missing_required_issue_field"
	}
	if !stateNameIn(issue.State, state.activeStates) || stateNameIn(issue.State, state.terminalStates) {
		return false, "not_active"
	}
	if strings.EqualFold(issue.State, "Human Review") || strings.EqualFold(issue.State, "In Review") {
		return false, "waiting_for_review"
	}
	if strings.EqualFold(issue.State, "AI Review") && !state.aiReview {
		return false, "waiting_for_ai_review"
	}
	if state.claimed != nil && state.claimed[issue.ID] {
		return false, "claimed"
	}
	if state.running != nil {
		if _, ok := state.running[issue.ID]; ok {
			return false, "running"
		}
	}
	if strings.EqualFold(issue.State, "Todo") {
		for _, blocker := range issue.BlockedBy {
			if !stateNameIn(blocker.State, state.terminalStates) {
				return false, "blocked_by_non_terminal"
			}
		}
	}
	if availableSlotsForState(issue.State, state) <= 0 {
		return false, "no_available_orchestrator_slots"
	}
	return true, ""
}

func availableSlotsForState(stateName string, state eligibilityState) int {
	globalAvailable := state.maxConcurrent - len(state.running)
	if globalAvailable < 0 {
		globalAvailable = 0
	}

	limit := state.maxConcurrent
	if state.perState != nil {
		if value, ok := state.perState[strings.ToLower(stateName)]; ok {
			limit = value
		}
	}

	runningInState := 0
	for _, running := range state.running {
		if strings.EqualFold(running.state, stateName) {
			runningInState++
		}
	}

	stateAvailable := limit - runningInState
	if stateAvailable < 0 {
		stateAvailable = 0
	}
	if stateAvailable < globalAvailable {
		return stateAvailable
	}
	return globalAvailable
}

func sortCandidates(issues []issuemodel.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		lp, rp := prioritySortValue(left.Priority), prioritySortValue(right.Priority)
		if lp != rp {
			return lp < rp
		}
		lt, rt := timeSortValue(left.CreatedAt), timeSortValue(right.CreatedAt)
		if !lt.Equal(rt) {
			return lt.Before(rt)
		}
		return left.Identifier < right.Identifier
	})
}

func prioritySortValue(priority *int) int {
	if priority == nil || *priority < 1 || *priority > 4 {
		return 999
	}
	return *priority
}

func timeSortValue(value *time.Time) time.Time {
	if value == nil {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return *value
}

func stateNameIn(state string, list []string) bool {
	for _, item := range list {
		if strings.EqualFold(item, state) {
			return true
		}
	}
	return false
}
