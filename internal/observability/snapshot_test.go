package observability

import (
	"testing"
	"time"
)

func TestSnapshotInitializesEmptySlices(t *testing.T) {
	snapshot := NewSnapshot()
	if snapshot.Running == nil {
		t.Fatal("running must be an empty slice, not nil")
	}
	if snapshot.Retrying == nil {
		t.Fatal("retrying must be an empty slice, not nil")
	}
	if snapshot.GeneratedAt.IsZero() {
		t.Fatal("generated_at must be initialized")
	}
	if snapshot.Counts.Running != 0 || snapshot.Counts.Retrying != 0 {
		t.Fatalf("counts = %#v", snapshot.Counts)
	}
}

func TestSnapshotIncludesLiveRuntimeSeconds(t *testing.T) {
	startedAt := time.Now().Add(-2 * time.Second)
	snapshot := NewSnapshot()
	snapshot.CodexTotals.SecondsRunning = 10
	snapshot.Running = []RunningEntry{{StartedAt: startedAt}}

	total := snapshot.TotalRuntimeSeconds(time.Now())
	if total < 11.5 {
		t.Fatalf("runtime seconds = %f, want active runtime included", total)
	}
}
