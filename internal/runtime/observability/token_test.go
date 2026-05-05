package observability

import "testing"

func TestExtractTokenUsageFromThreadTokenUsageUpdated(t *testing.T) {
	payload := map[string]any{
		"method": "thread/tokenUsage/updated",
		"params": map[string]any{
			"input_tokens":  120.0,
			"output_tokens": 34.0,
			"total_tokens":  154.0,
		},
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 34 || usage.TotalTokens != 154 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractTokenUsageFromTotalTokenUsageWrapper(t *testing.T) {
	payload := map[string]any{
		"method": "codex/event/token_count",
		"params": map[string]any{
			"total_token_usage": map[string]any{
				"input_tokens":  10.0,
				"output_tokens": 5.0,
			},
		},
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 || usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractTokenUsageFromCodexAppServerTokenUsageTotal(t *testing.T) {
	payload := map[string]any{
		"method": "thread/tokenUsage/updated",
		"params": map[string]any{
			"tokenUsage": map[string]any{
				"total": map[string]any{
					"inputTokens":  342584.0,
					"outputTokens": 3750.0,
					"totalTokens":  346334.0,
				},
			},
		},
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.InputTokens != 342584 || usage.OutputTokens != 3750 || usage.TotalTokens != 346334 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractTokenUsageFromTopLevelPayload(t *testing.T) {
	payload := map[string]any{
		"inputTokens":  7.0,
		"outputTokens": 8.0,
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.InputTokens != 7 || usage.OutputTokens != 8 || usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestHumanizeCodexEvent(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{name: "turn completed", payload: map[string]any{"method": "turn/completed"}, want: "turn completed"},
		{name: "turn failed", payload: map[string]any{"method": "turn/failed"}, want: "turn failed"},
		{name: "turn cancelled", payload: map[string]any{"method": "turn/cancelled"}, want: "turn cancelled"},
		{name: "task started", payload: map[string]any{"method": "codex/event/task_started"}, want: "task started"},
		{name: "codex token count", payload: map[string]any{"method": "codex/event/token_count"}, want: "token usage updated"},
		{name: "thread token usage", payload: map[string]any{"method": "thread/tokenUsage/updated"}, want: "token usage updated"},
		{name: "unknown method", payload: map[string]any{"method": "codex/event/custom_event"}, want: "codex/event/custom event"},
		{name: "empty method", payload: map[string]any{}, want: "event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HumanizeCodexEvent(tt.payload); got != tt.want {
				t.Fatalf("HumanizeCodexEvent() = %q, want %q", got, tt.want)
			}
		})
	}
}
