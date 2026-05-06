package issueflow

import "testing"

func TestDefinitionForTrunkShowsHumanReadableMainline(t *testing.T) {
	def := DefinitionForTrunk()

	if def.Name != "issue-flow-trunk" {
		t.Fatalf("Name = %q, want issue-flow-trunk", def.Name)
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
