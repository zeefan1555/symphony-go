package observability

import "strings"

func ExtractTokenUsage(payload map[string]any) (TokenUsage, bool) {
	if payload == nil {
		return TokenUsage{}, false
	}
	params, ok := payload["params"].(map[string]any)
	if !ok {
		return usageFromMap(payload)
	}
	if usage, ok := usageFromMap(params); ok {
		return usage, true
	}
	if tokenUsage, ok := params["tokenUsage"].(map[string]any); ok {
		if total, ok := tokenUsage["total"].(map[string]any); ok {
			return usageFromMap(total)
		}
		if usage, ok := usageFromMap(tokenUsage); ok {
			return usage, true
		}
	}
	nested, ok := params["total_token_usage"].(map[string]any)
	if !ok {
		return usageFromMap(payload)
	}
	return usageFromMap(nested)
}

func usageFromMap(value map[string]any) (TokenUsage, bool) {
	input, inputOK := intField(value, "input_tokens", "inputTokens", "input")
	output, outputOK := intField(value, "output_tokens", "outputTokens", "output")
	total, totalOK := intField(value, "total_tokens", "totalTokens", "total")
	if !totalOK && (inputOK || outputOK) {
		total = input + output
		totalOK = true
	}
	if !inputOK && !outputOK && !totalOK {
		return TokenUsage{}, false
	}
	return TokenUsage{InputTokens: input, OutputTokens: output, TotalTokens: total}, true
}

func intField(value map[string]any, names ...string) (int, bool) {
	for _, name := range names {
		switch raw := value[name].(type) {
		case int:
			return raw, true
		case int64:
			return int(raw), true
		case float64:
			return int(raw), true
		}
	}
	return 0, false
}

func HumanizeCodexEvent(payload map[string]any) string {
	method, _ := payload["method"].(string)
	switch method {
	case "turn/completed":
		return "turn completed"
	case "turn/failed":
		return "turn failed"
	case "turn/cancelled":
		return "turn cancelled"
	case "codex/event/task_started":
		return "task started"
	case "codex/event/token_count", "thread/tokenUsage/updated":
		return "token usage updated"
	default:
		if method == "" {
			return "event"
		}
		return strings.ReplaceAll(method, "_", " ")
	}
}
