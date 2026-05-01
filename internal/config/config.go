package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeefan1555/symphony-go/internal/types"
)

const (
	ErrUnsupportedTrackerKind    = "unsupported_tracker_kind"
	ErrMissingTrackerAPIKey      = "missing_tracker_api_key"
	ErrMissingTrackerProjectSlug = "missing_tracker_project_slug"
	ErrMissingCodexCommand       = "missing_codex_command"
	ErrInvalidHookTimeout        = "invalid_hook_timeout"
	ErrInvalidMaxTurns           = "invalid_max_turns"
	ErrInvalidMaxRetryBackoff    = "invalid_max_retry_backoff"
	ErrInvalidPollingInterval    = "invalid_polling_interval"
	ErrInvalidReviewPolicy       = "invalid_review_policy"
	ErrInvalidMergePolicy        = "invalid_merge_policy"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

func Code(err error) string {
	var cfgErr *Error
	if errors.As(err, &cfgErr) {
		return cfgErr.Code
	}
	return ""
}

func Resolve(raw types.Config, workflowPath string) (types.Config, error) {
	cfg := raw
	applyDefaults(&cfg)
	resolveEnv(&cfg)
	normalizeStates(&cfg)
	if err := normalizeWorkspaceRoot(&cfg, workflowPath); err != nil {
		return types.Config{}, err
	}
	if err := validate(cfg); err != nil {
		return types.Config{}, err
	}
	return cfg, nil
}

func applyDefaults(cfg *types.Config) {
	if cfg.Tracker.Endpoint == "" {
		cfg.Tracker.Endpoint = "https://api.linear.app/graphql"
	}
	if len(cfg.Tracker.ActiveStates) == 0 {
		cfg.Tracker.ActiveStates = []string{"Todo", "In Progress"}
	}
	if len(cfg.Tracker.TerminalStates) == 0 {
		cfg.Tracker.TerminalStates = []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"}
	}
	if cfg.Polling.IntervalMS == 0 {
		cfg.Polling.IntervalMS = 30000
	}
	if cfg.Workspace.Root == "" {
		cfg.Workspace.Root = filepath.Join(os.TempDir(), "symphony_workspaces")
	}
	if cfg.Hooks.TimeoutMS == 0 {
		cfg.Hooks.TimeoutMS = 60000
	}
	if cfg.Agent.MaxConcurrentAgents == 0 {
		cfg.Agent.MaxConcurrentAgents = 10
	}
	if cfg.Agent.MaxTurns == 0 {
		cfg.Agent.MaxTurns = 20
	}
	if cfg.Agent.MaxRetryBackoffMS == 0 {
		cfg.Agent.MaxRetryBackoffMS = 300000
	}
	applyStateSkillDefaults(&cfg.Agent)
	if cfg.Codex.Command == "" {
		cfg.Codex.Command = "codex app-server"
	}
	if cfg.Codex.TurnTimeoutMS == 0 {
		cfg.Codex.TurnTimeoutMS = 3600000
	}
	if cfg.Codex.ReadTimeoutMS == 0 {
		cfg.Codex.ReadTimeoutMS = 5000
	}
	if !cfg.Codex.StallTimeoutMSSet && cfg.Codex.StallTimeoutMS == 0 {
		cfg.Codex.StallTimeoutMS = 300000
	}
	if cfg.Codex.ApprovalPolicy == nil {
		cfg.Codex.ApprovalPolicy = "never"
	}
	if cfg.Codex.ThreadSandbox == "" {
		cfg.Codex.ThreadSandbox = "workspace-write"
	}
}

func resolveEnv(cfg *types.Config) {
	cfg.Tracker.APIKey = resolveDollar(cfg.Tracker.APIKey)
	if cfg.Tracker.APIKey == "" && cfg.Tracker.Kind == "linear" {
		cfg.Tracker.APIKey = os.Getenv("LINEAR_API_KEY")
	}
	cfg.Workspace.Root = resolveDollar(cfg.Workspace.Root)
}

func resolveDollar(value string) string {
	if strings.HasPrefix(value, "$") && len(value) > 1 && !strings.ContainsAny(value[1:], "/\\ ") {
		return os.Getenv(value[1:])
	}
	return value
}

func normalizeStates(cfg *types.Config) {
	normalized := map[string]int{}
	for state, limit := range cfg.Agent.MaxConcurrentAgentsByState {
		if limit > 0 {
			normalized[strings.ToLower(state)] = limit
		}
	}
	cfg.Agent.MaxConcurrentAgentsByState = normalized
}

func normalizeWorkspaceRoot(cfg *types.Config, workflowPath string) error {
	root := expandHome(cfg.Workspace.Root)
	if !filepath.IsAbs(root) {
		base := "."
		if workflowPath != "" {
			base = filepath.Dir(workflowPath)
		}
		root = filepath.Join(base, root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	cfg.Workspace.Root = filepath.Clean(abs)
	return nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func validate(cfg types.Config) error {
	if cfg.Tracker.Kind != "linear" {
		return &Error{Code: ErrUnsupportedTrackerKind, Message: fmt.Sprintf("tracker.kind %q is not supported", cfg.Tracker.Kind)}
	}
	if cfg.Tracker.APIKey == "" {
		return &Error{Code: ErrMissingTrackerAPIKey, Message: "tracker.api_key or LINEAR_API_KEY is required"}
	}
	if cfg.Tracker.ProjectSlug == "" {
		return &Error{Code: ErrMissingTrackerProjectSlug, Message: "tracker.project_slug is required for linear"}
	}
	if strings.TrimSpace(cfg.Codex.Command) == "" {
		return &Error{Code: ErrMissingCodexCommand, Message: "codex.command must be non-empty"}
	}
	if cfg.Polling.IntervalMS <= 0 {
		return &Error{Code: ErrInvalidPollingInterval, Message: "polling.interval_ms must be positive"}
	}
	if cfg.Hooks.TimeoutMS <= 0 {
		return &Error{Code: ErrInvalidHookTimeout, Message: "hooks.timeout_ms must be positive"}
	}
	if cfg.Agent.MaxTurns <= 0 {
		return &Error{Code: ErrInvalidMaxTurns, Message: "agent.max_turns must be positive"}
	}
	if cfg.Agent.MaxRetryBackoffMS <= 0 {
		return &Error{Code: ErrInvalidMaxRetryBackoff, Message: "agent.max_retry_backoff_ms must be positive"}
	}
	if err := validateReviewPolicy(cfg.Agent.ReviewPolicy); err != nil {
		return err
	}
	if err := validateStateSkills(cfg.Agent); err != nil {
		return err
	}
	return nil
}

func applyStateSkillDefaults(agent *types.AgentConfig) {
	if agent.StateSkills == nil {
		agent.StateSkills = map[string]string{}
	}
	if strings.TrimSpace(agent.StateSkills["Merging"]) != "" {
		return
	}
	if legacy := strings.TrimSpace(agent.MergePolicy.Skill); legacy != "" {
		agent.StateSkills["Merging"] = legacy
		return
	}
	agent.StateSkills["Merging"] = ".codex/skills/local-merge/SKILL.md"
}

func validateReviewPolicy(policy types.ReviewPolicyConfig) error {
	mode := strings.ToLower(strings.TrimSpace(policy.Mode))
	if mode != "" && mode != "human" && mode != "ai" && mode != "auto" {
		return &Error{Code: ErrInvalidReviewPolicy, Message: "agent.review_policy.mode must be one of human, ai, auto"}
	}
	onFail := strings.ToLower(strings.TrimSpace(policy.OnAIFail))
	if onFail != "" && onFail != "rework" && onFail != "hold" {
		return &Error{Code: ErrInvalidReviewPolicy, Message: "agent.review_policy.on_ai_fail must be one of rework, hold"}
	}
	return nil
}

func validateStateSkills(agent types.AgentConfig) error {
	for state, skillPath := range agent.StateSkills {
		if err := validateSkillPath("agent.state_skills."+state, skillPath, false); err != nil {
			return err
		}
	}
	if err := validateSkillPath("agent.merge_policy.skill", agent.MergePolicy.Skill, true); err != nil {
		return err
	}
	return nil
}

func validateSkillPath(field, value string, allowEmpty bool) error {
	raw := value
	path := strings.TrimSpace(value)
	if path == "" {
		if allowEmpty {
			return nil
		}
		return &Error{Code: ErrInvalidMergePolicy, Message: field + " must be a repo-root relative SKILL.md path"}
	}
	if strings.ContainsAny(raw, "\x00\r\n") || filepath.IsAbs(path) {
		return &Error{Code: ErrInvalidMergePolicy, Message: field + " must be a repo-root relative SKILL.md path"}
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return &Error{Code: ErrInvalidMergePolicy, Message: field + " must stay under the repo root"}
	}
	if !strings.ContainsAny(path, `/\`) || filepath.Base(clean) != "SKILL.md" {
		return &Error{Code: ErrInvalidMergePolicy, Message: field + " must be a repo-root relative SKILL.md path"}
	}
	return nil
}
