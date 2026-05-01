package observability

import "strings"

func ExtractRateLimits(payload map[string]any) (any, bool) {
	if payload == nil {
		return nil, false
	}
	if value, ok := rateLimitsFromMap(payload); ok {
		return value, true
	}
	params, _ := payload["params"].(map[string]any)
	if value, ok := rateLimitsFromMap(params); ok {
		return value, true
	}
	method, _ := payload["method"].(string)
	if strings.Contains(strings.ToLower(method), "rate") && strings.Contains(strings.ToLower(method), "limit") && len(params) > 0 {
		return params, true
	}
	return nil, false
}

func rateLimitsFromMap(value map[string]any) (any, bool) {
	if value == nil {
		return nil, false
	}
	for _, key := range []string{"rate_limits", "rateLimits", "rateLimit", "rate_limit"} {
		if limits, ok := value[key]; ok {
			return limits, true
		}
	}
	return nil, false
}
