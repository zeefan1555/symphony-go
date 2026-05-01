package orchestrator

import (
	"testing"
	"time"

	"github.com/zeefan1555/symphony-go/internal/types"
)

func TestRetryDelayUsesContinuationAndCappedFailureBackoff(t *testing.T) {
	opts := Options{Workflow: &types.Workflow{Config: types.Config{
		Agent: types.AgentConfig{MaxRetryBackoffMS: 25_000},
	}}}

	if got := retryDelay(opts, retryContinuation, 1); got != time.Second {
		t.Fatalf("continuation retry delay = %s, want 1s", got)
	}
	if got := retryDelay(opts, retryFailure, 1); got != 10*time.Second {
		t.Fatalf("first failure retry delay = %s, want 10s", got)
	}
	if got := retryDelay(opts, retryFailure, 2); got != 20*time.Second {
		t.Fatalf("second failure retry delay = %s, want 20s", got)
	}
	if got := retryDelay(opts, retryFailure, 3); got != 25*time.Second {
		t.Fatalf("capped failure retry delay = %s, want 25s", got)
	}
}
