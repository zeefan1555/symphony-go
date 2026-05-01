package orchestrator

import (
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/types"
)

func TestSortCandidatesUsesPriorityCreatedAtIdentifier(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	p0 := 0
	p1 := 1
	p2 := 2
	p5 := 5
	issues := []types.Issue{
		{ID: "d", Identifier: "ZEE-4", Title: "D", State: "Todo", Priority: &p0, CreatedAt: &old},
		{ID: "c", Identifier: "ZEE-3", Title: "C", State: "Todo", Priority: &p2, CreatedAt: &old},
		{ID: "b", Identifier: "ZEE-2", Title: "B", State: "Todo", Priority: &p1, CreatedAt: &newer},
		{ID: "a", Identifier: "ZEE-1", Title: "A", State: "Todo", Priority: &p1, CreatedAt: &old},
		{ID: "e", Identifier: "ZEE-5", Title: "E", State: "Todo", Priority: &p5, CreatedAt: &newer},
	}

	sortCandidates(issues)

	if issues[0].Identifier != "ZEE-1" ||
		issues[1].Identifier != "ZEE-2" ||
		issues[2].Identifier != "ZEE-3" ||
		issues[3].Identifier != "ZEE-4" ||
		issues[4].Identifier != "ZEE-5" {
		t.Fatalf("sorted = %#v", issues)
	}
}

func TestTodoBlockedByNonTerminalIsNotEligible(t *testing.T) {
	issue := types.Issue{
		ID:         "i1",
		Identifier: "ZEE-1",
		Title:      "Blocked",
		State:      "Todo",
		BlockedBy:  []types.BlockerRef{{Identifier: "ZEE-0", State: "In Progress"}},
	}

	ok, reason := candidateEligible(issue, eligibilityState{
		activeStates:   []string{"Todo", "In Progress"},
		terminalStates: []string{"Done", "Canceled"},
		maxConcurrent:  1,
	})

	if ok {
		t.Fatalf("expected blocked issue to be ineligible")
	}
	if reason != "blocked_by_non_terminal" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestLowercaseTodoBlockedByNonTerminalIsNotEligible(t *testing.T) {
	issue := types.Issue{
		ID:         "i1",
		Identifier: "ZEE-1",
		Title:      "Blocked",
		State:      "todo",
		BlockedBy:  []types.BlockerRef{{Identifier: "ZEE-0", State: "In Progress"}},
	}

	ok, reason := candidateEligible(issue, eligibilityState{
		activeStates:   []string{"Todo", "In Progress"},
		terminalStates: []string{"Done", "Canceled"},
		maxConcurrent:  1,
	})

	if ok {
		t.Fatalf("expected blocked issue to be ineligible")
	}
	if reason != "blocked_by_non_terminal" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestReviewWaitStateIsNotDispatchEligible(t *testing.T) {
	issue := types.Issue{
		ID:         "i1",
		Identifier: "ZEE-1",
		Title:      "Review",
		State:      "Human Review",
	}

	ok, reason := candidateEligible(issue, eligibilityState{
		activeStates:   []string{"In Progress", "Human Review", "Merging"},
		terminalStates: []string{"Done", "Canceled"},
		maxConcurrent:  1,
	})

	if ok {
		t.Fatal("expected review wait state to be ineligible for dispatch")
	}
	if reason != "waiting_for_review" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestAvailableSlotsHonorsGlobalAndPerStateLimits(t *testing.T) {
	state := eligibilityState{
		maxConcurrent: 3,
		perState:      map[string]int{"todo": 1},
		running: map[string]runningIssue{
			"i1": {state: "Todo"},
			"i2": {state: "In Progress"},
		},
	}

	if slots := availableSlotsForState("Todo", state); slots != 0 {
		t.Fatalf("todo slots = %d", slots)
	}
	if slots := availableSlotsForState("In Progress", state); slots != 1 {
		t.Fatalf("in-progress slots = %d", slots)
	}
}
