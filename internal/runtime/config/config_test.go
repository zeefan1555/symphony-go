package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveAppliesSpecDefaultsAndRelativeWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")

	resolved, err := Resolve(Config{
		Tracker: TrackerConfig{
			Kind:        "linear",
			APIKey:      "$LINEAR_API_KEY",
			ProjectSlug: "demo",
		},
	}, workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Tracker.Endpoint != "https://api.linear.app/graphql" {
		t.Fatalf("endpoint = %q", resolved.Tracker.Endpoint)
	}
	if resolved.Tracker.APIKey != "lin_test" {
		t.Fatalf("api key was not resolved through explicit $LINEAR_API_KEY")
	}
	if !reflect.DeepEqual(resolved.Tracker.ActiveStates, []string{"Todo", "In Progress"}) {
		t.Fatalf("active states = %#v", resolved.Tracker.ActiveStates)
	}
	if !reflect.DeepEqual(resolved.Tracker.TerminalStates, []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"}) {
		t.Fatalf("terminal states = %#v", resolved.Tracker.TerminalStates)
	}
	if resolved.Polling.IntervalMS != 30000 {
		t.Fatalf("poll interval = %d", resolved.Polling.IntervalMS)
	}
	if resolved.Workspace.Root == "" || !filepath.IsAbs(resolved.Workspace.Root) {
		t.Fatalf("workspace root must be absolute, got %q", resolved.Workspace.Root)
	}
	if want := filepath.Clean(filepath.Join(os.TempDir(), "symphony_workspaces")); resolved.Workspace.Root != want {
		t.Fatalf("workspace root = %q, want %q", resolved.Workspace.Root, want)
	}
	if resolved.Hooks.TimeoutMS != 60000 {
		t.Fatalf("hook timeout = %d", resolved.Hooks.TimeoutMS)
	}
	if resolved.Merge.Target != "main" {
		t.Fatalf("merge target = %q, want main", resolved.Merge.Target)
	}
	if resolved.Agent.MaxConcurrentAgents != 10 {
		t.Fatalf("max agents = %d", resolved.Agent.MaxConcurrentAgents)
	}
	if resolved.Agent.MaxTurns != 20 {
		t.Fatalf("max turns = %d", resolved.Agent.MaxTurns)
	}
	if resolved.Agent.MaxRetryBackoffMS != 300000 {
		t.Fatalf("max retry backoff = %d", resolved.Agent.MaxRetryBackoffMS)
	}
	if resolved.Codex.Command != "codex app-server" {
		t.Fatalf("codex command = %q", resolved.Codex.Command)
	}
	if resolved.Codex.TurnTimeoutMS != 3600000 {
		t.Fatalf("turn timeout = %d", resolved.Codex.TurnTimeoutMS)
	}
	if resolved.Codex.ReadTimeoutMS != 5000 {
		t.Fatalf("read timeout = %d", resolved.Codex.ReadTimeoutMS)
	}
	if resolved.Codex.StallTimeoutMS != 300000 {
		t.Fatalf("stall timeout = %d", resolved.Codex.StallTimeoutMS)
	}
	if resolved.Codex.ApprovalPolicy != "never" {
		t.Fatalf("approval policy = %#v", resolved.Codex.ApprovalPolicy)
	}
	if resolved.Codex.ThreadSandbox != "workspace-write" {
		t.Fatalf("thread sandbox = %q", resolved.Codex.ThreadSandbox)
	}
	if resolved.Server.PortSet {
		t.Fatal("server.port should be disabled by default")
	}
}

func TestResolvePreservesExplicitMergeTarget(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(Config{
		Tracker: validTracker(),
		Merge:   MergeConfig{Target: "release"},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Merge.Target != "release" {
		t.Fatalf("merge target = %q, want release", resolved.Merge.Target)
	}
}

func TestResolveUsesAppConfigMergeTarget(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	writeAppConfig(t, dir, "release")

	resolved, err := Resolve(Config{
		Tracker: validTracker(),
	}, filepath.Join(dir, "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Merge.Target != "release" {
		t.Fatalf("merge target = %q, want release", resolved.Merge.Target)
	}
	if len(resolved.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", resolved.Warnings)
	}
}

func TestResolveAppConfigMergeTargetOverridesWorkflowWithWarning(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	writeAppConfig(t, dir, "release")

	resolved, err := Resolve(Config{
		Tracker: validTracker(),
		Merge:   MergeConfig{Target: "main"},
	}, filepath.Join(dir, "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Merge.Target != "release" {
		t.Fatalf("merge target = %q, want release", resolved.Merge.Target)
	}
	if len(resolved.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one warning", resolved.Warnings)
	}
	if resolved.Warnings[0].Code != WarnWorkflowMergeTarget {
		t.Fatalf("warning code = %q", resolved.Warnings[0].Code)
	}
	if !strings.Contains(resolved.Warnings[0].Message, "workflow merge.target") {
		t.Fatalf("warning message = %q", resolved.Warnings[0].Message)
	}
}

func TestResolveDoesNotUseEnvironmentAsAppConfigOverride(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	t.Setenv("SYMPHONY_GO_GIT_MERGE_TARGET", "env-release")
	dir := t.TempDir()
	writeAppConfig(t, dir, "config-release")

	resolved, err := Resolve(Config{
		Tracker: validTracker(),
	}, filepath.Join(dir, "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Merge.Target != "config-release" {
		t.Fatalf("merge target = %q, want config-release", resolved.Merge.Target)
	}
}

func TestResolvePreservesExplicitServerPortZero(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(Config{
		Tracker: validTracker(),
		Server:  ServerConfig{Port: 0, PortSet: true},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.Server.PortSet {
		t.Fatal("server.port presence should be preserved")
	}
	if resolved.Server.Port != 0 {
		t.Fatalf("server port = %d, want 0", resolved.Server.Port)
	}
}

func TestResolveUsesExplicitEnvIndirectionOnly(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_TOKEN_FOR_TEST", "lin_from_env")

	resolved, err := Resolve(Config{
		Tracker: TrackerConfig{
			Kind:        "linear",
			APIKey:      "$LINEAR_TOKEN_FOR_TEST",
			ProjectSlug: "demo",
		},
	}, workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Tracker.APIKey != "lin_from_env" {
		t.Fatalf("api key = %q", resolved.Tracker.APIKey)
	}

	t.Setenv("LINEAR_API_KEY", "lin_global_should_not_be_used")
	_, err = Resolve(Config{
		Tracker: TrackerConfig{
			Kind:        "linear",
			ProjectSlug: "demo",
		},
	}, workflowPath)
	if err == nil {
		t.Fatal("expected missing tracker API key without explicit $VAR")
	}
	if Code(err) != ErrMissingTrackerAPIKey {
		t.Fatalf("code = %q, want %s", Code(err), ErrMissingTrackerAPIKey)
	}
}

func TestResolvePreservesExplicitCodexStallTimeout(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	for _, tc := range []struct {
		name string
		raw  int
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := Resolve(Config{
				Tracker: validTracker(),
				Codex: CodexConfig{
					StallTimeoutMS:    tc.raw,
					StallTimeoutMSSet: true,
				},
			}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Codex.StallTimeoutMS != tc.raw {
				t.Fatalf("stall timeout = %d, want %d", resolved.Codex.StallTimeoutMS, tc.raw)
			}
		})
	}
}

func TestResolveDefaultsAbsentCodexStallTimeout(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(Config{
		Tracker: validTracker(),
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Codex.StallTimeoutMS != 300000 {
		t.Fatalf("stall timeout = %d", resolved.Codex.StallTimeoutMS)
	}
}

func TestResolvePreservesProgrammaticPositiveCodexStallTimeout(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(Config{
		Tracker: validTracker(),
		Codex:   CodexConfig{StallTimeoutMS: 100},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Codex.StallTimeoutMS != 100 {
		t.Fatalf("stall timeout = %d", resolved.Codex.StallTimeoutMS)
	}
}

func TestResolveRejectsInvalidDispatchConfig(t *testing.T) {
	_, err := Resolve(Config{
		Tracker: TrackerConfig{Kind: "github"},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err == nil {
		t.Fatal("expected unsupported tracker error")
	}
	if Code(err) != ErrUnsupportedTrackerKind {
		t.Fatalf("code = %q", Code(err))
	}
}

func TestResolveRejectsInvalidReviewPolicy(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	for _, tc := range []struct {
		name   string
		policy ReviewPolicyConfig
	}{
		{
			name:   "mode",
			policy: ReviewPolicyConfig{Mode: "robot"},
		},
		{
			name:   "on_ai_fail",
			policy: ReviewPolicyConfig{Mode: "ai", OnAIFail: "ignore"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(Config{
				Tracker: validTracker(),
				Agent:   AgentConfig{ReviewPolicy: tc.policy},
			}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
			if err == nil {
				t.Fatal("expected invalid review policy error")
			}
			if Code(err) != ErrInvalidReviewPolicy {
				t.Fatalf("code = %q", Code(err))
			}
		})
	}
}

func TestResolveNormalizesPerStateConcurrency(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(Config{
		Tracker: validTracker(),
		Agent: AgentConfig{
			MaxConcurrentAgentsByState: map[string]int{
				"Todo":        2,
				"In Progress": 1,
				"Broken":      0,
			},
		},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Agent.MaxConcurrentAgentsByState["todo"] != 2 {
		t.Fatalf("todo limit = %#v", resolved.Agent.MaxConcurrentAgentsByState)
	}
	if _, ok := resolved.Agent.MaxConcurrentAgentsByState["broken"]; ok {
		t.Fatalf("invalid non-positive entries must be ignored: %#v", resolved.Agent.MaxConcurrentAgentsByState)
	}
}

func writeAppConfig(t *testing.T, dir string, mergeTarget string) {
	t.Helper()
	confDir := filepath.Join(dir, "conf")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "git:\n  merge_target: " + mergeTarget + "\n"
	if err := os.WriteFile(filepath.Join(confDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validTracker() TrackerConfig {
	return TrackerConfig{Kind: "linear", APIKey: "lin_test", ProjectSlug: "demo"}
}
