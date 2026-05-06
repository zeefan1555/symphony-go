package issueflow

import (
	"strings"

	"symphony-go/internal/service/codex"
)

func reviewFinalPasses(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range []string{
		"review: pass",
		"conclusion: pass",
		"结论: pass",
		"结论：pass",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func mergeFinalPasses(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(normalized, "merge: pass")
}

func CompletedAgentMessageText(event codex.Event) string {
	if event.Name != "item/completed" {
		return ""
	}
	params, _ := event.Payload["params"].(map[string]any)
	item, _ := params["item"].(map[string]any)
	itemType, _ := item["type"].(string)
	if itemType != "agentMessage" {
		return ""
	}
	text, _ := item["text"].(string)
	return text
}
