package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
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
	ErrInvalidServerPort         = "invalid_server_port"
	ErrInvalidWorkerConfig       = "invalid_worker_config"
	ErrInvalidReviewPolicy       = "invalid_review_policy"
	WarnWorkflowMergeTarget      = "workflow_merge_target_deprecated"
)

type AppConfig struct {
	Git GitConfig `mapstructure:"git"`
}

type GitConfig struct {
	MergeTarget string `mapstructure:"merge_target"`
}

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

func Resolve(raw Config, workflowPath string) (Config, error) {
	cfg := raw
	appCfg, appLoaded, err := LoadAppConfig(workflowPath)
	if err != nil {
		return Config{}, err
	}
	applyAppConfig(&cfg, appCfg, appLoaded)
	applyDefaults(&cfg)
	resolveEnv(&cfg)
	normalizeStates(&cfg)
	normalizeWorker(&cfg)
	normalizeMerge(&cfg)
	if err := normalizeWorkspaceRoot(&cfg, workflowPath); err != nil {
		return Config{}, err
	}
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadAppConfig(workflowPath string) (AppConfig, bool, error) {
	configPath := filepath.Join(workflowDir(workflowPath), "conf", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			return AppConfig{}, false, nil
		}
		return AppConfig{}, false, err
	}
	reader := viper.New()
	reader.SetConfigFile(configPath)
	reader.SetConfigType("yaml")
	if err := reader.ReadInConfig(); err != nil {
		return AppConfig{}, false, fmt.Errorf("read app config: %w", err)
	}
	var cfg AppConfig
	if err := reader.Unmarshal(&cfg); err != nil {
		return AppConfig{}, false, fmt.Errorf("parse app config: %w", err)
	}
	return cfg, true, nil
}

func workflowDir(workflowPath string) string {
	if workflowPath == "" {
		return "."
	}
	return filepath.Dir(workflowPath)
}

func applyAppConfig(cfg *Config, appCfg AppConfig, appLoaded bool) {
	if !appLoaded {
		return
	}
	target := strings.TrimSpace(appCfg.Git.MergeTarget)
	if target == "" {
		return
	}
	workflowTarget := strings.TrimSpace(cfg.Merge.Target)
	if workflowTarget != "" && workflowTarget != target {
		cfg.Warnings = append(cfg.Warnings, ConfigWarning{
			Code:    WarnWorkflowMergeTarget,
			Message: fmt.Sprintf("workflow merge.target %q is deprecated and ignored because conf/config.yaml git.merge_target is set", workflowTarget),
		})
	}
	cfg.Merge.Target = target
}

func applyDefaults(cfg *Config) {
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
	if cfg.Worker.SSHHosts == nil {
		cfg.Worker.SSHHosts = []string{}
	}
	if cfg.Hooks.TimeoutMS == 0 {
		cfg.Hooks.TimeoutMS = 60000
	}
	if strings.TrimSpace(cfg.Merge.Target) == "" {
		cfg.Merge.Target = "main"
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

func resolveEnv(cfg *Config) {
	cfg.Tracker.APIKey = resolveDollar(cfg.Tracker.APIKey)
	cfg.Workspace.Root = resolveDollar(cfg.Workspace.Root)
}

func resolveDollar(value string) string {
	if strings.HasPrefix(value, "$") && len(value) > 1 && !strings.ContainsAny(value[1:], "/\\ ") {
		return os.Getenv(value[1:])
	}
	return value
}

func normalizeStates(cfg *Config) {
	normalized := map[string]int{}
	for state, limit := range cfg.Agent.MaxConcurrentAgentsByState {
		if limit > 0 {
			normalized[strings.ToLower(state)] = limit
		}
	}
	cfg.Agent.MaxConcurrentAgentsByState = normalized
}

func normalizeWorker(cfg *Config) {
	hosts := make([]string, 0, len(cfg.Worker.SSHHosts))
	for _, host := range cfg.Worker.SSHHosts {
		hosts = append(hosts, strings.TrimSpace(host))
	}
	cfg.Worker.SSHHosts = hosts
}

func normalizeMerge(cfg *Config) {
	cfg.Merge.Target = strings.TrimSpace(cfg.Merge.Target)
	if cfg.Merge.Target == "" {
		cfg.Merge.Target = "main"
	}
}

func normalizeWorkspaceRoot(cfg *Config, workflowPath string) error {
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

func validate(cfg Config) error {
	if cfg.Tracker.Kind != "linear" {
		return &Error{Code: ErrUnsupportedTrackerKind, Message: fmt.Sprintf("tracker.kind %q is not supported", cfg.Tracker.Kind)}
	}
	if cfg.Tracker.APIKey == "" {
		return &Error{Code: ErrMissingTrackerAPIKey, Message: "tracker.api_key is required; use explicit $VAR indirection for environment secrets"}
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
	if cfg.Server.PortSet && cfg.Server.Port < 0 {
		return &Error{Code: ErrInvalidServerPort, Message: "server.port must be zero or positive"}
	}
	if err := validateWorker(cfg.Worker); err != nil {
		return err
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
	return nil
}

func validateWorker(worker WorkerConfig) error {
	for _, host := range worker.SSHHosts {
		if host == "" {
			return &Error{Code: ErrInvalidWorkerConfig, Message: "worker.ssh_hosts must not contain blank hosts"}
		}
	}
	if worker.MaxConcurrentAgentsPerHost < 0 || (worker.MaxConcurrentAgentsPerHostIsSet && worker.MaxConcurrentAgentsPerHost == 0) {
		return &Error{Code: ErrInvalidWorkerConfig, Message: "worker.max_concurrent_agents_per_host must be positive when set"}
	}
	return nil
}

func validateReviewPolicy(policy ReviewPolicyConfig) error {
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
