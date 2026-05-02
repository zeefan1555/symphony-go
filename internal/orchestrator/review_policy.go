package orchestrator

import (
	"strings"

	"github.com/zeefan1555/symphony-go/internal/types"
)

const (
	reviewPolicyHuman = "human"
	reviewPolicyAI    = "ai"
	reviewPolicyAuto  = "auto"
)

type reviewPolicy struct {
	mode                string
	allowManualAIReview bool
}

func effectiveReviewPolicy(agent types.AgentConfig) reviewPolicy {
	cfg := agent.ReviewPolicy
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		return legacyReviewPolicy(agent.AIReview)
	}
	if mode != reviewPolicyAI && mode != reviewPolicyAuto {
		mode = reviewPolicyHuman
	}
	return reviewPolicy{
		mode:                mode,
		allowManualAIReview: cfg.AllowManualAIReview,
	}
}

func legacyReviewPolicy(review types.AIReviewConfig) reviewPolicy {
	policy := reviewPolicy{
		mode: reviewPolicyHuman,
	}
	if review.Enabled {
		policy.mode = reviewPolicyAI
		if review.AutoMerge {
			policy.mode = reviewPolicyAuto
		}
	}
	return policy
}

func (p reviewPolicy) allowsAIReviewState() bool {
	return p.mode == reviewPolicyAI || p.mode == reviewPolicyAuto || p.allowManualAIReview
}
