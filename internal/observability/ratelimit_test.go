package observability

import "testing"

func TestExtractRateLimitsFromExplicitPayload(t *testing.T) {
	payload := map[string]any{
		"method": "codex/event/rate_limits",
		"params": map[string]any{
			"rate_limits": map[string]any{"remaining": 3.0},
		},
	}

	limits, ok := ExtractRateLimits(payload)
	if !ok {
		t.Fatal("expected rate limits")
	}
	got := limits.(map[string]any)
	if got["remaining"] != 3.0 {
		t.Fatalf("rate limits = %#v", limits)
	}
}

func TestExtractRateLimitsFromRateLimitMethod(t *testing.T) {
	payload := map[string]any{
		"method": "thread/rateLimits/updated",
		"params": map[string]any{"remaining": 2.0},
	}

	limits, ok := ExtractRateLimits(payload)
	if !ok {
		t.Fatal("expected rate limits")
	}
	got := limits.(map[string]any)
	if got["remaining"] != 2.0 {
		t.Fatalf("rate limits = %#v", limits)
	}
}
