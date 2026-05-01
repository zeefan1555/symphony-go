package types

import (
	"time"

	"gopkg.in/yaml.v3"
)

type BlockerRef struct {
	ID         string
	Identifier string
	State      string
}

type Issue struct {
	ID          string
	Identifier  string
	Title       string
	Description string
	Priority    *int
	State       string
	BranchName  string
	URL         string
	Labels      []string
	BlockedBy   []BlockerRef
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

type Workflow struct {
	Config         Config
	PromptTemplate string
}

type Config struct {
	Tracker   TrackerConfig   `yaml:"tracker"`
	Polling   PollingConfig   `yaml:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace"`
	Hooks     HooksConfig     `yaml:"hooks"`
	Agent     AgentConfig     `yaml:"agent"`
	Codex     CodexConfig     `yaml:"codex"`
}

type TrackerConfig struct {
	Kind           string   `yaml:"kind"`
	Endpoint       string   `yaml:"endpoint"`
	APIKey         string   `yaml:"api_key"`
	ProjectSlug    string   `yaml:"project_slug"`
	ActiveStates   []string `yaml:"active_states"`
	TerminalStates []string `yaml:"terminal_states"`
}

type PollingConfig struct {
	IntervalMS int `yaml:"interval_ms"`
}

type WorkspaceConfig struct {
	Root string `yaml:"root"`
}

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
	TimeoutMS    int    `yaml:"timeout_ms"`
}

type AgentConfig struct {
	MaxConcurrentAgents        int                `yaml:"max_concurrent_agents"`
	MaxTurns                   int                `yaml:"max_turns"`
	MaxRetryBackoffMS          int                `yaml:"max_retry_backoff_ms"`
	MaxConcurrentAgentsByState map[string]int     `yaml:"max_concurrent_agents_by_state"`
	ReviewPolicy               ReviewPolicyConfig `yaml:"review_policy"`
	MergePolicy                MergePolicyConfig  `yaml:"merge_policy"`
	AIReview                   AIReviewConfig     `yaml:"ai_review"`
}

type ReviewPolicyConfig struct {
	Mode                 string   `yaml:"mode"`
	AllowManualAIReview  bool     `yaml:"allow_manual_ai_review"`
	OnAIFail             string   `yaml:"on_ai_fail"`
	ExpectedChangedFiles []string `yaml:"expected_changed_files"`
}

type MergePolicyConfig struct {
	Mode  string `yaml:"mode"`
	Skill string `yaml:"skill"`
}

// AIReviewConfig is kept for legacy workflow files. Prefer ReviewPolicyConfig.
type AIReviewConfig struct {
	Enabled              bool     `yaml:"enabled"`
	AutoMerge            bool     `yaml:"auto_merge"`
	ReworkOnFailure      bool     `yaml:"rework_on_failure"`
	ExpectedChangedFiles []string `yaml:"expected_changed_files"`
}

type CodexConfig struct {
	Command           string         `yaml:"command"`
	ApprovalPolicy    any            `yaml:"approval_policy"`
	ThreadSandbox     string         `yaml:"thread_sandbox"`
	TurnSandboxPolicy map[string]any `yaml:"turn_sandbox_policy"`
	TurnTimeoutMS     int            `yaml:"turn_timeout_ms"`
	ReadTimeoutMS     int            `yaml:"read_timeout_ms"`
	StallTimeoutMS    int            `yaml:"stall_timeout_ms"`
	StallTimeoutMSSet bool           `yaml:"-"`
}

func (c *CodexConfig) UnmarshalYAML(value *yaml.Node) error {
	type raw CodexConfig
	var decoded raw
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*c = CodexConfig(decoded)
	c.StallTimeoutMSSet = yamlMappingHasNonNullKey(value, "stall_timeout_ms")
	return nil
}

func yamlMappingHasNonNullKey(value *yaml.Node, key string) bool {
	if value.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(value.Content); i += 2 {
		if value.Content[i].Value == key {
			return value.Content[i+1].Tag != "!!null"
		}
	}
	return false
}
