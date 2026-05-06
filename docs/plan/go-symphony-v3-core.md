# Go Symphony V3 Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `/Users/bytedance/symphony/go` from the current v1/TUI prototype to the core `SPEC.md` conformance path: typed config, dynamic reload, bounded concurrent dispatch, retry/reconciliation, full hook lifecycle, safe workspaces, and tracker-driven restart recovery.

**Architecture:** Keep the existing single-process Go daemon and current package boundaries, but replace the synchronous one-issue poll loop with an orchestrator-owned runtime state machine. Add small focused packages for config resolution, workflow reloading, retry timing, and hook execution while preserving the current Linear, workspace, Codex, JSONL logging, snapshot, and TUI surfaces.

**Tech Stack:** Go 1.22+, standard library concurrency primitives, `gopkg.in/yaml.v3`, existing Codex app-server JSON line client, existing Linear GraphQL client, existing JSONL logger and terminal TUI.

---

## Scope

This v3 plan implements `SPEC.md` core conformance. It intentionally excludes optional extension work that can be shipped after the core loop is stable:

- No HTTP `/api/v1/*` server in v3. The existing TUI/snapshot remain observability surfaces.
- `linear_graphql` client-side tool extension is included so unattended child sessions can use the same Linear GraphQL auth path as the listener.
- No SSH worker extension in v3.
- No persistent retry database in v3, because the spec explicitly allows tracker/filesystem restart recovery without durable scheduler state.
- No broad rewrite of the existing local merge behavior. The current local merge path can remain a workflow-specific extension while the scheduler becomes spec-shaped.

## Current Go Progress

Already present in `go/`:

- `go/internal/workflow/workflow.go` loads YAML front matter and renders a small strict template subset.
- `go/internal/types/types.go` defines `Issue`, `Workflow`, and basic typed config structs.
- `go/internal/linear/client.go` fetches active issues, fetches one issue, updates state, and upserts a workpad comment.
- `go/internal/workspace/workspace.go` creates sanitized workspaces and runs `after_create` / `before_remove`.
- `go/internal/codex/runner.go` launches `bash -lc <codex.command>` in the workspace, starts app-server, starts one turn, streams events, and injects git metadata writable roots.
- `go/internal/orchestrator/orchestrator.go` polls synchronously, handles local state transitions, runs one issue at a time, and feeds snapshot/TUI state.
- `go/internal/observability/*` and `go/internal/tui/*` already provide token extraction, snapshots, humanized event summaries, and terminal rendering.

Main gaps against `SPEC.md` core:

- Config defaults currently differ from the spec, and `$VAR`, `~`, relative path normalization, validation, per-state concurrency, retry backoff cap, and stall timeout are incomplete.
- Workflow reload is absent.
- Linear client lacks terminal-state fetch, batch state refresh, pagination, blockers normalization, and lowercased labels.
- Workspace hooks lack `before_run` and `after_run`, and workspace root containment is not enforced before agent launch.
- Orchestrator is synchronous and does not own a `claimed` set, concurrent workers, timer-backed retry queue, sort/eligibility checks, reconciliation, stall cancellation, or startup terminal cleanup.
- Codex runner supports only one turn per app-server process; continuation turns currently restart the session and resend the full prompt from orchestrator.
- Structured logs do not consistently include `issue_id`, `issue_identifier`, and `session_id`.

## File Structure

- Create `go/internal/config/config.go`
  - Own typed config resolution, defaults, `$VAR` resolution, path normalization, active/terminal state normalization, validation, and reload-safe effective config.
- Create `go/internal/config/config_test.go`
  - Spec conformance tests for config defaults, env indirection, path handling, validation errors, and per-state concurrency normalization.
- Modify `go/internal/types/types.go`
  - Add missing domain fields: blockers, retry/backoff config, hook fields, stall timeout, per-state concurrency, normalized issue-state refresh structs, and typed error constants.
- Modify `go/internal/workflow/workflow.go`
  - Keep file parsing here, return typed workflow parse errors, and delegate effective config resolution to `internal/config`.
- Create `go/internal/workflow/reloader.go`
  - Poll file metadata and reload last-known-good workflow/config without crashing on invalid edits.
- Create `go/internal/workflow/reloader_test.go`
  - Tests for successful reload and invalid reload preserving last good config.
- Modify `go/internal/linear/client.go`
  - Add candidate pagination, `FetchIssuesByStates`, `FetchIssueStatesByIDs`, blocker normalization, lowercase labels, and structured error categories.
- Modify `go/internal/orchestrator/orchestrator.go`
  - Replace synchronous poll/run with an orchestrator-owned state machine: running map, claimed set, retry map, worker cancel funcs, reconciliation, dispatch, retry timers, and startup cleanup.
- Create `go/internal/orchestrator/eligibility.go`
  - Keep issue validation, active/terminal checks, blocker rule, sort order, and global/per-state slot calculations small and testable.
- Create `go/internal/orchestrator/retry.go`
  - Keep continuation delay and exponential backoff calculation small and testable.
- Modify `go/internal/workspace/workspace.go`
  - Add `before_run` and `after_run`; enforce root containment; expose `PathForIssue`; log hook results through callback or returned structured errors.
- Modify `go/internal/codex/runner.go`
  - Support one app-server session with multiple turns, continuation guidance after first turn, stall-aware events, and normalized app-server error categories.
- Modify `go/internal/logging/jsonl.go`
  - Ensure issue/session context fields are first-class on every issue/session log event.
- Modify `go/cmd/symphony-go/main.go`
  - Wire config resolution, workflow reload, startup validation, startup cleanup, signal shutdown, and existing TUI options.
- Modify `go/README.md`
  - Document v3 trust/safety posture, dynamic reload behavior, config fields, retry/reconciliation behavior, and validation commands.

## Success Criteria

- `cd /Users/bytedance/symphony/go && make test` passes.
- Startup fails fast for invalid required config: unsupported tracker, missing API key, missing project slug, missing codex command.
- Editing `WORKFLOW.md` while the daemon runs updates future polling interval, concurrency, active/terminal states, hooks, workspace root, Codex command, and prompt without restart.
- Concurrent dispatch obeys global and per-state limits and never dispatches the same issue twice.
- Normal worker exit schedules a 1s continuation retry; abnormal exit schedules capped exponential retry.
- Reconciliation stops workers whose tracker state becomes non-active and cleans workspaces when state becomes terminal.
- Startup terminal cleanup removes workspaces for terminal issues and logs cleanup failures without aborting startup.
- Agent subprocess launch is rejected if cwd is not the exact per-issue workspace path or workspace path escapes workspace root.

## Task 1: Extend Domain Types For Spec Core

**Files:**
- Modify: `go/internal/types/types.go`
- Test: covered by later package tests that import the new fields

- [ ] **Step 1: Add missing issue and config fields**

Modify `go/internal/types/types.go` so the existing structs include these fields. Keep the existing field names that current code already uses.

```go
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

type HooksConfig struct {
	AfterCreate  string `yaml:"after_create"`
	BeforeRun    string `yaml:"before_run"`
	AfterRun     string `yaml:"after_run"`
	BeforeRemove string `yaml:"before_remove"`
	TimeoutMS    int    `yaml:"timeout_ms"`
}

type AgentConfig struct {
	MaxConcurrentAgents        int            `yaml:"max_concurrent_agents"`
	MaxTurns                   int            `yaml:"max_turns"`
	MaxRetryBackoffMS          int            `yaml:"max_retry_backoff_ms"`
	MaxConcurrentAgentsByState map[string]int `yaml:"max_concurrent_agents_by_state"`
}

type CodexConfig struct {
	Command           string         `yaml:"command"`
	ApprovalPolicy    any            `yaml:"approval_policy"`
	ThreadSandbox     string         `yaml:"thread_sandbox"`
	TurnSandboxPolicy map[string]any `yaml:"turn_sandbox_policy"`
	TurnTimeoutMS     int            `yaml:"turn_timeout_ms"`
	ReadTimeoutMS     int            `yaml:"read_timeout_ms"`
	StallTimeoutMS    int            `yaml:"stall_timeout_ms"`
}
```

- [ ] **Step 2: Remove config defaulting from `types.Config.ApplyDefaults` after Task 2 lands**

After `internal/config` exists, delete defaulting logic from `types.Config.ApplyDefaults` or make it call `config.Resolve` impossible to avoid import cycles. The preferred v3 shape is: `types` owns data structs only; `internal/config` owns defaults and validation.

- [ ] **Step 3: Run type-checking tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/types ./internal/workflow
```

Expected: PASS after later compile fixes are applied. If this task is executed alone before Task 2, compile failures in `workflow.Load` referencing `ApplyDefaults` are expected and should be resolved immediately by Task 2.

## Task 2: Add Spec-Compliant Config Resolution

**Files:**
- Create: `go/internal/config/config.go`
- Create: `go/internal/config/config_test.go`
- Modify: `go/internal/workflow/workflow.go`
- Modify: `go/cmd/symphony-go/main.go`

- [ ] **Step 1: Write config tests**

Create `go/internal/config/config_test.go`:

```go
package config

import (
	"path/filepath"
	"testing"

	"symphony-go/internal/types"
)

func TestResolveAppliesSpecDefaultsAndRelativeWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")

	resolved, err := Resolve(types.Config{
		Tracker: types.TrackerConfig{
			Kind:        "linear",
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
		t.Fatalf("api key was not resolved from LINEAR_API_KEY")
	}
	if resolved.Polling.IntervalMS != 30000 {
		t.Fatalf("poll interval = %d", resolved.Polling.IntervalMS)
	}
	if resolved.Workspace.Root == "" || !filepath.IsAbs(resolved.Workspace.Root) {
		t.Fatalf("workspace root must be absolute, got %q", resolved.Workspace.Root)
	}
	if resolved.Hooks.TimeoutMS != 60000 {
		t.Fatalf("hook timeout = %d", resolved.Hooks.TimeoutMS)
	}
	if resolved.Agent.MaxConcurrentAgents != 10 {
		t.Fatalf("max agents = %d", resolved.Agent.MaxConcurrentAgents)
	}
	if resolved.Agent.MaxRetryBackoffMS != 300000 {
		t.Fatalf("max retry backoff = %d", resolved.Agent.MaxRetryBackoffMS)
	}
	if resolved.Codex.Command != "codex app-server" {
		t.Fatalf("codex command = %q", resolved.Codex.Command)
	}
	if resolved.Codex.StallTimeoutMS != 300000 {
		t.Fatalf("stall timeout = %d", resolved.Codex.StallTimeoutMS)
	}
}

func TestResolveUsesExplicitEnvIndirectionOnly(t *testing.T) {
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_TOKEN_FOR_TEST", "lin_from_env")

	resolved, err := Resolve(types.Config{
		Tracker: types.TrackerConfig{
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
}

func TestResolveRejectsInvalidDispatchConfig(t *testing.T) {
	_, err := Resolve(types.Config{
		Tracker: types.TrackerConfig{Kind: "github"},
	}, filepath.Join(t.TempDir(), "WORKFLOW.md"))
	if err == nil {
		t.Fatal("expected unsupported tracker error")
	}
	if Code(err) != ErrUnsupportedTrackerKind {
		t.Fatalf("code = %q", Code(err))
	}
}

func TestResolveNormalizesPerStateConcurrency(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	resolved, err := Resolve(types.Config{
		Tracker: types.TrackerConfig{Kind: "linear", ProjectSlug: "demo"},
		Agent: types.AgentConfig{
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
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/config
```

Expected: FAIL because `internal/config` does not exist.

- [ ] **Step 3: Implement config resolver**

Create `go/internal/config/config.go` with these exported pieces:

```go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"symphony-go/internal/types"
)

const (
	ErrUnsupportedTrackerKind     = "unsupported_tracker_kind"
	ErrMissingTrackerAPIKey       = "missing_tracker_api_key"
	ErrMissingTrackerProjectSlug  = "missing_tracker_project_slug"
	ErrMissingCodexCommand        = "missing_codex_command"
	ErrInvalidHookTimeout         = "invalid_hook_timeout"
	ErrInvalidMaxTurns            = "invalid_max_turns"
	ErrInvalidMaxRetryBackoff     = "invalid_max_retry_backoff"
	ErrInvalidPollingInterval     = "invalid_polling_interval"
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
	if cfg.Codex.Command == "" {
		cfg.Codex.Command = "codex app-server"
	}
	if cfg.Codex.TurnTimeoutMS == 0 {
		cfg.Codex.TurnTimeoutMS = 3600000
	}
	if cfg.Codex.ReadTimeoutMS == 0 {
		cfg.Codex.ReadTimeoutMS = 5000
	}
	if cfg.Codex.StallTimeoutMS == 0 {
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
	return nil
}
```

- [ ] **Step 4: Wire workflow loading through config resolver**

Change `workflow.Load` to accept the same path but call `config.Resolve(rawConfig, path)` before returning:

```go
resolved, err := config.Resolve(cfg, path)
if err != nil {
	return nil, err
}
return &types.Workflow{Config: resolved, PromptTemplate: strings.TrimSpace(string(prompt))}, nil
```

Import:

```go
import "symphony-go/internal/config"
```

Remove the direct call to `cfg.ApplyDefaults()`.

- [ ] **Step 5: Simplify CLI workspace root handling**

In `go/cmd/symphony-go/main.go`, remove the manual relative workspace root block:

```go
if !filepath.IsAbs(loaded.Config.Workspace.Root) {
	loaded.Config.Workspace.Root = filepath.Join(filepath.Dir(absWorkflow), loaded.Config.Workspace.Root)
}
```

Keep `absWorkflow` for `RepoRootFromWorkflow` and log path.

- [ ] **Step 6: Run config and workflow tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/config ./internal/workflow ./cmd/symphony-go
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/internal/config go/internal/types/types.go go/internal/workflow/workflow.go go/cmd/symphony-go/main.go
git commit -m "feat(go): add spec config resolution"
```

## Task 3: Add Workflow Dynamic Reload

**Files:**
- Create: `go/internal/workflow/reloader.go`
- Create: `go/internal/workflow/reloader_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`
- Modify: `go/cmd/symphony-go/main.go`

- [ ] **Step 1: Write reloader tests**

Create `go/internal/workflow/reloader_test.go`:

```go
package workflow

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReloaderKeepsLastGoodWorkflowAfterInvalidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	writeWorkflow(t, path, "1000", "hello {{ issue.identifier }}")

	reloader, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	first := reloader.Current()
	if first.Config.Polling.IntervalMS != 1000 {
		t.Fatalf("first interval = %d", first.Config.Polling.IntervalMS)
	}

	if err := os.WriteFile(path, []byte("---\ntracker: [not-a-map\n---\nbad"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := reloader.ReloadIfChanged()
	if err == nil {
		t.Fatal("expected invalid reload error")
	}
	if changed {
		t.Fatal("invalid reload must not replace current workflow")
	}
	current := reloader.Current()
	if current.Config.Polling.IntervalMS != 1000 {
		t.Fatalf("last good interval was not preserved: %d", current.Config.Polling.IntervalMS)
	}
}

func TestReloaderAppliesValidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	writeWorkflow(t, path, "1000", "first")

	reloader, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	writeWorkflow(t, path, "2000", "second {{ issue.identifier }}")

	changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected reload")
	}
	current := reloader.Current()
	if current.Config.Polling.IntervalMS != 2000 {
		t.Fatalf("interval = %d", current.Config.Polling.IntervalMS)
	}
	if current.PromptTemplate != "second {{ issue.identifier }}" {
		t.Fatalf("prompt = %q", current.PromptTemplate)
	}
}

func writeWorkflow(t *testing.T, path string, interval string, prompt string) {
	t.Helper()
	content := `---
tracker:
  kind: linear
  project_slug: demo
polling:
  interval_ms: ` + interval + `
---
` + prompt + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/workflow
```

Expected: FAIL because `NewReloader` does not exist.

- [ ] **Step 3: Implement `Reloader`**

Create `go/internal/workflow/reloader.go`:

```go
package workflow

import (
	"os"
	"sync"
	"time"

	"symphony-go/internal/types"
)

type Reloader struct {
	path    string
	mu      sync.RWMutex
	current *types.Workflow
	modTime time.Time
	size    int64
}

func NewReloader(path string) (*Reloader, error) {
	loaded, err := Load(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return &Reloader{
		path:    path,
		current: loaded,
		modTime: info.ModTime(),
		size:    info.Size(),
	}, nil
}

func (r *Reloader) Current() *types.Workflow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneWorkflow(r.current)
}

func (r *Reloader) ReloadIfChanged() (bool, error) {
	info, err := os.Stat(r.path)
	if err != nil {
		return false, err
	}
	r.mu.RLock()
	same := info.ModTime().Equal(r.modTime) && info.Size() == r.size
	r.mu.RUnlock()
	if same {
		return false, nil
	}
	loaded, err := Load(r.path)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	r.current = loaded
	r.modTime = info.ModTime()
	r.size = info.Size()
	r.mu.Unlock()
	return true, nil
}

func cloneWorkflow(input *types.Workflow) *types.Workflow {
	if input == nil {
		return nil
	}
	copy := *input
	copy.Config.Tracker.ActiveStates = append([]string(nil), input.Config.Tracker.ActiveStates...)
	copy.Config.Tracker.TerminalStates = append([]string(nil), input.Config.Tracker.TerminalStates...)
	if input.Config.Agent.MaxConcurrentAgentsByState != nil {
		copy.Config.Agent.MaxConcurrentAgentsByState = map[string]int{}
		for key, value := range input.Config.Agent.MaxConcurrentAgentsByState {
			copy.Config.Agent.MaxConcurrentAgentsByState[key] = value
		}
	}
	return &copy
}
```

- [ ] **Step 4: Add orchestrator workflow refresh point**

In `go/internal/orchestrator/orchestrator.go`, add to `Options`:

```go
WorkflowReloader interface {
	Current() *types.Workflow
	ReloadIfChanged() (bool, error)
}
```

Add field:

```go
Reloader WorkflowReloader
```

At the start of each tick, before dispatch validation, call:

```go
func (o *Orchestrator) refreshWorkflow() {
	if o.opts.Reloader == nil {
		return
	}
	changed, err := o.opts.Reloader.ReloadIfChanged()
	if err != nil {
		o.setLastError(err.Error())
		o.log("", "workflow_reload_failed", err.Error(), nil)
		return
	}
	if changed {
		o.opts.Workflow = o.opts.Reloader.Current()
		o.snapshot.Polling.IntervalMS = o.opts.Workflow.Config.Polling.IntervalMS
		o.log("", "workflow_reloaded", "workflow reload completed", nil)
	}
}
```

Call `o.refreshWorkflow()` as the first line in `poll`.

- [ ] **Step 5: Wire CLI to `NewReloader`**

In `main.go`, replace direct `workflow.Load` startup with:

```go
reloader, err := workflow.NewReloader(opts.WorkflowPath)
if err != nil {
	fatal(err)
}
loaded := reloader.Current()
```

Pass `Reloader: reloader` into `orchestrator.Options`.

- [ ] **Step 6: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/workflow ./internal/orchestrator ./cmd/symphony-go
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/internal/workflow/reloader.go go/internal/workflow/reloader_test.go go/internal/orchestrator/orchestrator.go go/cmd/symphony-go/main.go
git commit -m "feat(go): reload workflow without restart"
```

## Task 4: Complete Linear Read Adapter

**Files:**
- Modify: `go/internal/linear/client.go`
- Modify: `go/internal/linear/client_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go` tracker interface

- [ ] **Step 1: Extend tracker interface**

Change `go/internal/orchestrator/orchestrator.go`:

```go
type Tracker interface {
	FetchActiveIssues(context.Context, []string) ([]types.Issue, error)
	FetchIssuesByStates(context.Context, []string) ([]types.Issue, error)
	FetchIssue(context.Context, string) (types.Issue, error)
	FetchIssueStatesByIDs(context.Context, []string) ([]types.Issue, error)
	UpdateIssueState(context.Context, string, string) error
	UpsertWorkpad(context.Context, string, string) error
}
```

- [ ] **Step 2: Add Linear tests for pagination, terminal fetch, state refresh, blockers, lowercase labels**

Append to `go/internal/linear/client_test.go`:

```go
func TestFetchActiveIssuesPaginatesAndNormalizes(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ZEE-1","title":"First","description":"Body","priority":1,"state":{"name":"Todo"},"branchName":"branch","url":"u","labels":{"nodes":[{"name":"AI"}]},"relations":{"nodes":[{"type":"blocks","relatedIssue":{"id":"b1","identifier":"ZEE-0","state":{"name":"In Progress"}}}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-1"}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i2","identifier":"ZEE-2","title":"Second","description":"","priority":null,"state":{"name":"In Progress"},"branchName":"","url":"","labels":{"nodes":[{"name":"Bug"}]},"relations":{"nodes":[]},"createdAt":"2026-01-03T00:00:00Z","updatedAt":"2026-01-04T00:00:00Z"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`))
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchActiveIssues(context.Background(), []string{"Todo", "In Progress"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	if issues[0].Labels[0] != "ai" {
		t.Fatalf("labels = %#v", issues[0].Labels)
	}
	if len(issues[0].BlockedBy) != 1 || issues[0].BlockedBy[0].Identifier != "ZEE-0" {
		t.Fatalf("blockers = %#v", issues[0].BlockedBy)
	}
	if issues[0].CreatedAt == nil || issues[1].UpdatedAt == nil {
		t.Fatalf("timestamps were not parsed: %#v %#v", issues[0], issues[1])
	}
}

func TestFetchIssueStatesByIDsEmptyDoesNotCallAPI(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	client := &Client{Endpoint: server.URL, APIKey: "lin_test", ProjectSlug: "demo", HTTPClient: server.Client()}
	issues, err := client.FetchIssueStatesByIDs(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if called {
		t.Fatal("empty state refresh should not call Linear")
	}
}
```

- [ ] **Step 3: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/linear
```

Expected: FAIL because pagination fields, relations, and new methods do not exist.

- [ ] **Step 4: Implement read methods**

In `client.go`:

- Update candidate query to request `relations { nodes { type relatedIssue { id identifier state { name } } } }` and `pageInfo { hasNextPage endCursor }`.
- Add `after` cursor variable and loop until `hasNextPage=false`.
- Add `FetchIssuesByStates(ctx, states)` as the same paginated query using the provided state list. Return `[]types.Issue{}` immediately when `len(states)==0`.
- Add `FetchIssueStatesByIDs(ctx, ids)` using GraphQL variable type `[ID!]`; return empty immediately for empty ids.
- Normalize labels using `strings.ToLower`.
- Parse timestamps with `time.Parse(time.RFC3339, value)`.
- Normalize blockers from relations where `type == "blocks"` and `relatedIssue` is present.

- [ ] **Step 5: Keep tracker write tests passing**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/linear ./internal/orchestrator
```

Expected: PASS after updating orchestrator test fakes to implement `FetchIssuesByStates` and `FetchIssueStatesByIDs`.

- [ ] **Step 6: Commit**

```bash
git add go/internal/linear/client.go go/internal/linear/client_test.go go/internal/orchestrator/orchestrator.go go/internal/orchestrator/orchestrator_test.go
git commit -m "feat(go): complete linear read adapter"
```

## Task 5: Complete Workspace Hook Lifecycle And Safety

**Files:**
- Modify: `go/internal/workspace/workspace.go`
- Modify: `go/internal/workspace/workspace_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`

- [ ] **Step 1: Add workspace tests**

Append to `go/internal/workspace/workspace_test.go`:

```go
func TestPathForIssueStaysInsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{})
	path, err := manager.PathForIssue(types.Issue{Identifier: "../ZEE/unsafe"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != root {
		t.Fatalf("path escaped root: %q", path)
	}
	if filepath.Base(path) != ".._ZEE_unsafe" {
		t.Fatalf("sanitized base = %q", filepath.Base(path))
	}
}

func TestBeforeRunAndAfterRunHookSemantics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{
		BeforeRun: `printf before >> order.txt`,
		AfterRun:  `printf after >> order.txt; exit 9`,
		TimeoutMS: 5000,
	})
	issue := types.Issue{Identifier: "ZEE-HOOKS"}
	workspacePath, _, err := manager.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.BeforeRun(context.Background(), workspacePath); err != nil {
		t.Fatal(err)
	}
	err = manager.AfterRun(context.Background(), workspacePath)
	if err == nil {
		t.Fatal("after_run should return an error for logging")
	}
	raw, err := os.ReadFile(filepath.Join(workspacePath, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "beforeafter" {
		t.Fatalf("hook order = %q", string(raw))
	}
}

func TestValidateWorkspacePathRejectsEscapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{})
	err := manager.ValidateWorkspacePath(filepath.Join(filepath.Dir(root), "outside"))
	if err == nil {
		t.Fatal("expected escaped workspace path to be rejected")
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/workspace
```

Expected: FAIL because `PathForIssue`, `BeforeRun`, `AfterRun`, and `ValidateWorkspacePath` do not exist.

- [ ] **Step 3: Implement workspace methods**

Add to `workspace.go`:

```go
func (m *Manager) RootAbs() (string, error) {
	root, err := filepath.Abs(expandHome(m.Root))
	if err != nil {
		return "", err
	}
	return filepath.Clean(root), nil
}

func (m *Manager) PathForIssue(issue types.Issue) (string, error) {
	if issue.Identifier == "" {
		return "", fmt.Errorf("issue identifier is required")
	}
	root, err := m.RootAbs()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, SafeIdentifier(issue.Identifier)), nil
}

func (m *Manager) ValidateWorkspacePath(path string) error {
	root, err := m.RootAbs()
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
		return nil
	}
	return fmt.Errorf("workspace path %q escapes workspace root %q", abs, root)
}

func (m *Manager) BeforeRun(ctx context.Context, path string) error {
	if err := m.ValidateWorkspacePath(path); err != nil {
		return err
	}
	if m.Hooks.BeforeRun == "" {
		return nil
	}
	return m.runHook(ctx, "before_run", m.Hooks.BeforeRun, path)
}

func (m *Manager) AfterRun(ctx context.Context, path string) error {
	if err := m.ValidateWorkspacePath(path); err != nil {
		return err
	}
	if m.Hooks.AfterRun == "" {
		return nil
	}
	return m.runHook(ctx, "after_run", m.Hooks.AfterRun, path)
}
```

Import `strings`.

Refactor `Ensure` to call `PathForIssue` and `ValidateWorkspacePath` instead of recomputing the path independently.

- [ ] **Step 4: Call hooks from orchestrator**

In `runAgent`, after `Ensure` and before prompt rendering, call:

```go
if err := o.opts.Workspace.BeforeRun(ctx, workspacePath); err != nil {
	o.removeRunning(issue.ID)
	return err
}
defer func() {
	if err := o.opts.Workspace.AfterRun(context.Background(), workspacePath); err != nil {
		o.log(issue.Identifier, "after_run_hook_failed", err.Error(), nil)
	}
}()
```

The `defer` must run on success, failure, timeout, or cancellation once the workspace exists.

- [ ] **Step 5: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/workspace ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/workspace/workspace.go go/internal/workspace/workspace_test.go go/internal/orchestrator/orchestrator.go
git commit -m "feat(go): complete workspace hook lifecycle"
```

## Task 6: Add Dispatch Eligibility, Sorting, And Slot Accounting

**Files:**
- Create: `go/internal/orchestrator/eligibility.go`
- Create: `go/internal/orchestrator/eligibility_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`

- [ ] **Step 1: Write eligibility tests**

Create `go/internal/orchestrator/eligibility_test.go`:

```go
package orchestrator

import (
	"testing"
	"time"

	"symphony-go/internal/types"
)

func TestSortCandidatesUsesPriorityCreatedAtIdentifier(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	p1 := 1
	p2 := 2
	issues := []types.Issue{
		{ID: "c", Identifier: "ZEE-3", Title: "C", State: "Todo", Priority: &p2, CreatedAt: &old},
		{ID: "b", Identifier: "ZEE-2", Title: "B", State: "Todo", Priority: &p1, CreatedAt: &newer},
		{ID: "a", Identifier: "ZEE-1", Title: "A", State: "Todo", Priority: &p1, CreatedAt: &old},
	}
	sortCandidates(issues)
	if issues[0].Identifier != "ZEE-1" || issues[1].Identifier != "ZEE-2" || issues[2].Identifier != "ZEE-3" {
		t.Fatalf("sorted = %#v", issues)
	}
}

func TestTodoBlockedByNonTerminalIsNotEligible(t *testing.T) {
	issue := types.Issue{
		ID: "i1", Identifier: "ZEE-1", Title: "Blocked", State: "Todo",
		BlockedBy: []types.BlockerRef{{Identifier: "ZEE-0", State: "In Progress"}},
	}
	ok, reason := candidateEligible(issue, eligibilityState{
		activeStates:   []string{"Todo", "In Progress"},
		terminalStates: []string{"Done", "Canceled"},
	})
	if ok {
		t.Fatalf("expected blocked issue to be ineligible")
	}
	if reason != "blocked_by_non_terminal" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestAvailableSlotsHonorsGlobalAndPerStateLimits(t *testing.T) {
	state := eligibilityState{
		maxConcurrent: 3,
		perState:      map[string]int{"todo": 1},
		running: map[string]runningIssue{
			"i1": {state: "Todo"},
			"i2": {state: "In Progress"},
		},
	}
	if slots := availableSlotsForState("Todo", state); slots != 0 {
		t.Fatalf("todo slots = %d", slots)
	}
	if slots := availableSlotsForState("In Progress", state); slots != 1 {
		t.Fatalf("in-progress slots = %d", slots)
	}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator
```

Expected: FAIL because helper types/functions do not exist.

- [ ] **Step 3: Implement eligibility helpers**

Create `eligibility.go` with:

```go
package orchestrator

import (
	"sort"
	"strings"
	"time"

	"symphony-go/internal/types"
)

type runningIssue struct {
	state string
}

type eligibilityState struct {
	activeStates   []string
	terminalStates []string
	claimed        map[string]bool
	running        map[string]runningIssue
	maxConcurrent  int
	perState       map[string]int
}

func candidateEligible(issue types.Issue, state eligibilityState) (bool, string) {
	if issue.ID == "" || issue.Identifier == "" || issue.Title == "" || issue.State == "" {
		return false, "missing_required_issue_field"
	}
	if !stateNameIn(issue.State, state.activeStates) || stateNameIn(issue.State, state.terminalStates) {
		return false, "not_active"
	}
	if state.claimed != nil && state.claimed[issue.ID] {
		return false, "claimed"
	}
	if state.running != nil {
		if _, ok := state.running[issue.ID]; ok {
			return false, "running"
		}
	}
	if issue.State == "Todo" {
		for _, blocker := range issue.BlockedBy {
			if !stateNameIn(blocker.State, state.terminalStates) {
				return false, "blocked_by_non_terminal"
			}
		}
	}
	if availableSlotsForState(issue.State, state) <= 0 {
		return false, "no_available_orchestrator_slots"
	}
	return true, ""
}

func availableSlotsForState(stateName string, state eligibilityState) int {
	globalAvailable := state.maxConcurrent - len(state.running)
	if globalAvailable < 0 {
		globalAvailable = 0
	}
	limit := state.maxConcurrent
	if value, ok := state.perState[strings.ToLower(stateName)]; ok {
		limit = value
	}
	runningInState := 0
	for _, running := range state.running {
		if strings.EqualFold(running.state, stateName) {
			runningInState++
		}
	}
	stateAvailable := limit - runningInState
	if stateAvailable < 0 {
		stateAvailable = 0
	}
	if stateAvailable < globalAvailable {
		return stateAvailable
	}
	return globalAvailable
}

func sortCandidates(issues []types.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		left, right := issues[i], issues[j]
		lp, rp := prioritySortValue(left.Priority), prioritySortValue(right.Priority)
		if lp != rp {
			return lp < rp
		}
		lt, rt := timeSortValue(left.CreatedAt), timeSortValue(right.CreatedAt)
		if !lt.Equal(rt) {
			return lt.Before(rt)
		}
		return left.Identifier < right.Identifier
	})
}

func prioritySortValue(priority *int) int {
	if priority == nil {
		return 999
	}
	return *priority
}

func timeSortValue(value *time.Time) time.Time {
	if value == nil {
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return *value
}

func stateNameIn(state string, list []string) bool {
	for _, item := range list {
		if strings.EqualFold(item, state) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Use helpers in dispatch**

In `poll`, call `sortCandidates(issues)` before iterating. Build `eligibilityState` from orchestrator state under lock. Skip ineligible issues and log the reason:

```go
ok, reason := candidateEligible(issue, o.eligibilityState())
if !ok {
	o.log(issue.Identifier, "dispatch_skipped", reason, map[string]any{"issue_id": issue.ID})
	continue
}
```

Add `o.claimIssue(issue)` before spawning/running work and `o.releaseIssue(issue.ID)` when a retry is released or worker is fully done.

- [ ] **Step 5: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/orchestrator/eligibility.go go/internal/orchestrator/eligibility_test.go go/internal/orchestrator/orchestrator.go
git commit -m "feat(go): add dispatch eligibility rules"
```

## Task 7: Implement Concurrent Workers And Retry Queue

**Files:**
- Create: `go/internal/orchestrator/retry.go`
- Create: `go/internal/orchestrator/retry_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`
- Modify: `go/internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: Write retry tests**

Create `go/internal/orchestrator/retry_test.go`:

```go
package orchestrator

import (
	"testing"
	"time"
)

func TestRetryDelayUsesContinuationAndExponentialBackoff(t *testing.T) {
	if got := retryDelay(1, true, 300000); got != time.Second {
		t.Fatalf("continuation delay = %s", got)
	}
	if got := retryDelay(1, false, 300000); got != 10*time.Second {
		t.Fatalf("attempt 1 delay = %s", got)
	}
	if got := retryDelay(3, false, 300000); got != 40*time.Second {
		t.Fatalf("attempt 3 delay = %s", got)
	}
	if got := retryDelay(10, false, 30000); got != 30*time.Second {
		t.Fatalf("capped delay = %s", got)
	}
}
```

- [ ] **Step 2: Implement retry delay**

Create `retry.go`:

```go
package orchestrator

import "time"

func retryDelay(attempt int, continuation bool, maxBackoffMS int) time.Duration {
	if continuation {
		return time.Second
	}
	if attempt < 1 {
		attempt = 1
	}
	if maxBackoffMS <= 0 {
		maxBackoffMS = 300000
	}
	delay := 10 * time.Second
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= time.Duration(maxBackoffMS)*time.Millisecond {
			return time.Duration(maxBackoffMS) * time.Millisecond
		}
	}
	return delay
}
```

- [ ] **Step 3: Replace synchronous issue execution with worker goroutines**

In `Orchestrator`, add fields:

```go
claimed map[string]bool
runningCancel map[string]context.CancelFunc
retryTimers map[string]*time.Timer
retryAttempts map[string]int
pollNow chan struct{}
```

Initialize them in `New`.

When dispatching:

```go
func (o *Orchestrator) dispatchIssue(ctx context.Context, issue types.Issue, attempt int) {
	o.claimIssue(issue)
	workerCtx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	o.runningCancel[issue.ID] = cancel
	o.mu.Unlock()
	go func() {
		err := o.runAgent(workerCtx, issue, attempt)
		o.workerExited(issue, attempt, err)
	}()
}
```

Change `runAgent` signature to:

```go
func (o *Orchestrator) runAgent(ctx context.Context, issue types.Issue, attempt int) error
```

Use `attempt` when rendering the first prompt. First attempt should pass `nil`; retry attempt should pass `&attempt`.

- [ ] **Step 4: Schedule retries on worker exit**

Add:

```go
func (o *Orchestrator) workerExited(issue types.Issue, attempt int, err error) {
	o.removeRunning(issue.ID)
	o.mu.Lock()
	delete(o.runningCancel, issue.ID)
	o.mu.Unlock()
	if err == nil {
		o.scheduleRetry(issue, 1, "", true)
		return
	}
	nextAttempt := attempt + 1
	if nextAttempt < 1 {
		nextAttempt = 1
	}
	o.scheduleRetry(issue, nextAttempt, err.Error(), false)
}
```

`scheduleRetry` must:

- Stop any old timer for the same issue.
- Keep issue claimed.
- Add/update snapshot retry entry with `attempt`, `identifier`, `due_at`, and `error`.
- Start a `time.AfterFunc` that calls `handleRetry(issue.ID)`.

- [ ] **Step 5: Implement retry handling**

`handleRetry(issueID)` should:

1. Fetch active candidates with current `active_states`.
2. Find the matching issue by `id`.
3. If absent, remove retry and release claim.
4. If present but no slots are available, requeue with error `no available orchestrator slots`.
5. If present and eligible, remove retry and dispatch with the retry attempt.
6. Signal `pollNow` after state changes so TUI/snapshots update quickly.

- [ ] **Step 6: Make `Run` support immediate retry-triggered polls**

Change the loop select to wait on timer, `pollNow`, or context cancellation:

```go
timer := time.NewTimer(interval)
select {
case <-ctx.Done():
	return ctx.Err()
case <-o.pollNow:
	if !timer.Stop() {
		<-timer.C
	}
case <-timer.C:
}
```

- [ ] **Step 7: Update tests**

Add tests in `orchestrator_test.go` for:

- Two eligible issues dispatch concurrently when `max_concurrent_agents=2`.
- Same issue is not dispatched twice while claimed.
- Normal worker exit creates retry attempt `1` with due time about 1 second later.
- Failure exit creates retry attempt `1` with due time about 10 seconds later.

Use fake runners with channels so the test can observe concurrency without sleeping for full delays.

- [ ] **Step 8: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add go/internal/orchestrator/retry.go go/internal/orchestrator/retry_test.go go/internal/orchestrator/orchestrator.go go/internal/orchestrator/orchestrator_test.go
git commit -m "feat(go): add concurrent dispatch and retry queue"
```

## Task 8: Add Reconciliation, Stall Detection, And Startup Cleanup

**Files:**
- Modify: `go/internal/orchestrator/orchestrator.go`
- Modify: `go/internal/orchestrator/orchestrator_test.go`
- Modify: `go/internal/workspace/workspace.go`

- [ ] **Step 1: Write reconciliation tests**

Add tests in `orchestrator_test.go`:

```go
func TestReconcileTerminalIssueCancelsWorkerAndCleansWorkspace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	root := filepath.Join(t.TempDir(), "worktrees")
	issue := types.Issue{ID: "issue-id", Identifier: "ZEE-DONE", Title: "terminal", State: "In Progress"}
	manager := workspace.New(root, types.HooksConfig{})
	workspacePath, _, err := manager.Ensure(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}

	runner := &blockingRunner{started: make(chan struct{}), canceled: make(chan struct{})}
	tracker := &reconcileTracker{
		active:    []types.Issue{issue},
		refreshed: []types.Issue{{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Done"}},
	}
	o := New(Options{
		Workflow:  testWorkflowForReconcile(),
		Tracker:   tracker,
		Workspace: manager,
		Runner:    runner,
	})

	o.dispatchIssue(ctx, issue, 0)
	<-runner.started
	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatal(err)
	}
	<-runner.canceled
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("terminal workspace still exists or unexpected stat error: %v", err)
	}
}

func TestReconcileNonActiveIssueCancelsWithoutCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	root := filepath.Join(t.TempDir(), "worktrees")
	issue := types.Issue{ID: "issue-id", Identifier: "ZEE-REVIEW", Title: "review", State: "In Progress"}
	manager := workspace.New(root, types.HooksConfig{})
	workspacePath, _, err := manager.Ensure(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}

	runner := &blockingRunner{started: make(chan struct{}), canceled: make(chan struct{})}
	tracker := &reconcileTracker{
		active:    []types.Issue{issue},
		refreshed: []types.Issue{{ID: issue.ID, Identifier: issue.Identifier, Title: issue.Title, State: "Human Review"}},
	}
	o := New(Options{
		Workflow:  testWorkflowForReconcile(),
		Tracker:   tracker,
		Workspace: manager,
		Runner:    runner,
	})

	o.dispatchIssue(ctx, issue, 0)
	<-runner.started
	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatal(err)
	}
	<-runner.canceled
	if _, err := os.Stat(workspacePath); err != nil {
		t.Fatalf("non-active workspace should be preserved: %v", err)
	}
}

func TestStallDetectionCancelsAndRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	issue := types.Issue{ID: "issue-id", Identifier: "ZEE-STALL", Title: "stall", State: "In Progress"}
	runner := &blockingRunner{started: make(chan struct{}), canceled: make(chan struct{})}
	o := New(Options{
		Workflow: &types.Workflow{Config: types.Config{
			Tracker: types.TrackerConfig{ActiveStates: []string{"In Progress"}, TerminalStates: []string{"Done"}},
			Agent:   types.AgentConfig{MaxConcurrentAgents: 1, MaxRetryBackoffMS: 300000, MaxTurns: 1},
			Codex:   types.CodexConfig{StallTimeoutMS: 1},
		}, PromptTemplate: "work"},
		Tracker:   &reconcileTracker{active: []types.Issue{issue}, refreshed: []types.Issue{issue}},
		Workspace: workspace.New(filepath.Join(t.TempDir(), "worktrees"), types.HooksConfig{}),
		Runner:    runner,
	})

	o.dispatchIssue(ctx, issue, 0)
	<-runner.started
	time.Sleep(5 * time.Millisecond)
	if err := o.reconcileRunning(ctx); err != nil {
		t.Fatal(err)
	}
	<-runner.canceled
	snapshot := o.Snapshot()
	if len(snapshot.Retrying) != 1 || !strings.Contains(snapshot.Retrying[0].Error, "stalled") {
		t.Fatalf("retry snapshot = %#v", snapshot.Retrying)
	}
}

func TestStartupCleanupRemovesTerminalWorkspaces(t *testing.T) {
	ctx := context.Background()
	issue := types.Issue{ID: "issue-id", Identifier: "ZEE-DONE", Title: "done", State: "Done"}
	manager := workspace.New(filepath.Join(t.TempDir(), "worktrees"), types.HooksConfig{})
	workspacePath, _, err := manager.Ensure(ctx, issue)
	if err != nil {
		t.Fatal(err)
	}
	o := New(Options{
		Workflow:  testWorkflowForReconcile(),
		Tracker:   &reconcileTracker{terminal: []types.Issue{issue}},
		Workspace: manager,
		Runner:    noCommitRunner{},
	})

	o.StartupCleanup(ctx)
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("terminal workspace still exists or unexpected stat error: %v", err)
	}
}

type blockingRunner struct {
	started  chan struct{}
	canceled chan struct{}
}

func (r *blockingRunner) RunSession(ctx context.Context, request codex.SessionRequest, onEvent func(codex.Event)) (codex.SessionResult, error) {
	close(r.started)
	<-ctx.Done()
	close(r.canceled)
	return codex.SessionResult{}, ctx.Err()
}

type reconcileTracker struct {
	active    []types.Issue
	terminal  []types.Issue
	refreshed []types.Issue
}

func (t *reconcileTracker) FetchActiveIssues(context.Context, []string) ([]types.Issue, error) {
	return t.active, nil
}

func (t *reconcileTracker) FetchIssuesByStates(context.Context, []string) ([]types.Issue, error) {
	return t.terminal, nil
}

func (t *reconcileTracker) FetchIssue(context.Context, string) (types.Issue, error) {
	if len(t.refreshed) > 0 {
		return t.refreshed[0], nil
	}
	return types.Issue{}, nil
}

func (t *reconcileTracker) FetchIssueStatesByIDs(context.Context, []string) ([]types.Issue, error) {
	return t.refreshed, nil
}

func (t *reconcileTracker) UpdateIssueState(context.Context, string, string) error {
	return nil
}

func (t *reconcileTracker) UpsertWorkpad(context.Context, string, string) error {
	return nil
}

func testWorkflowForReconcile() *types.Workflow {
	return &types.Workflow{Config: types.Config{
		Tracker: types.TrackerConfig{ActiveStates: []string{"Todo", "In Progress"}, TerminalStates: []string{"Done", "Canceled"}},
		Agent:   types.AgentConfig{MaxConcurrentAgents: 1, MaxRetryBackoffMS: 300000, MaxTurns: 1},
		Codex:   types.CodexConfig{StallTimeoutMS: 300000},
	}, PromptTemplate: "work on {{ issue.identifier }}"}
}
```

- [ ] **Step 2: Run tests and verify failure**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator
```

Expected: FAIL because reconciliation/startup cleanup is not implemented.

- [ ] **Step 3: Implement `reconcileRunning`**

Add to poll tick before validation/candidate fetch:

```go
if err := o.reconcileRunning(ctx); err != nil {
	o.log("", "reconcile_failed", err.Error(), nil)
}
```

`reconcileRunning` should:

- Call `cancelStalledRuns`.
- Snapshot running issue IDs.
- Return immediately if no running IDs.
- Call `Tracker.FetchIssueStatesByIDs(ctx, ids)`.
- On refresh error, log and keep workers running.
- For terminal state, cancel worker and call `Workspace.Remove(ctx, workspacePath)`.
- For active state, update running entry state.
- For neither active nor terminal, cancel worker without workspace cleanup.

- [ ] **Step 4: Implement stall detection**

Use `codex.stall_timeout_ms`. If `<=0`, do nothing. For each running row, compare now against `LastEventAt` when present, otherwise `StartedAt`. If elapsed is greater than the timeout:

- Cancel worker context.
- Schedule failure retry with error `stalled after <duration>`.
- Keep workspace.

- [ ] **Step 5: Implement startup cleanup**

Add:

```go
func (o *Orchestrator) StartupCleanup(ctx context.Context) {
	issues, err := o.opts.Tracker.FetchIssuesByStates(ctx, o.opts.Workflow.Config.Tracker.TerminalStates)
	if err != nil {
		o.log("", "startup_cleanup_failed", err.Error(), nil)
		return
	}
	for _, issue := range issues {
		path, err := o.opts.Workspace.PathForIssue(issue)
		if err != nil {
			o.log(issue.Identifier, "startup_cleanup_failed", err.Error(), map[string]any{"issue_id": issue.ID})
			continue
		}
		if err := o.opts.Workspace.Remove(ctx, path); err != nil {
			o.log(issue.Identifier, "startup_cleanup_failed", err.Error(), map[string]any{"issue_id": issue.ID})
		}
	}
}
```

Call `service.StartupCleanup(ctx)` from `main.go` after constructing the orchestrator and before `service.Run(ctx)`.

- [ ] **Step 6: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator ./internal/workspace
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add go/internal/orchestrator/orchestrator.go go/internal/orchestrator/orchestrator_test.go go/cmd/symphony-go/main.go
git commit -m "feat(go): reconcile active runs"
```

## Task 9: Reuse One Codex Session For Continuation Turns

**Files:**
- Modify: `go/internal/codex/runner.go`
- Modify: `go/internal/codex/runner_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`

- [ ] **Step 1: Extend runner interface**

Change orchestrator `AgentRunner` to:

```go
type AgentRunner interface {
	RunSession(context.Context, codex.SessionRequest, func(codex.Event)) (codex.SessionResult, error)
}
```

Add in `codex/runner.go`:

```go
type SessionRequest struct {
	WorkspacePath string
	Issue         types.Issue
	Prompts       []TurnPrompt
}

type TurnPrompt struct {
	Text         string
	Continuation bool
	Attempt      *int
}

type SessionResult struct {
	SessionID string
	ThreadID  string
	Turns     []Result
	PID       int
}
```

Keep a compatibility wrapper `Run` only if existing tests need it during the transition.

- [ ] **Step 2: Add continuation test**

Append to `runner_test.go`:

```go
func TestRunnerKeepsOneThreadForContinuationTurns(t *testing.T) {
	workspacePath := t.TempDir()
	fake := filepath.Join(t.TempDir(), "fake-codex")
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	script := `#!/bin/sh
trace="$TRACE_FILE"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":1,"result":{}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"}}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1"}}}'
printf '%s\n' '{"method":"turn/completed"}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":4,"result":{"turn":{"id":"turn-2"}}}'
printf '%s\n' '{"method":"turn/completed"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_FILE", trace)
	runner := New(types.CodexConfig{
		Command:       fake,
		ApprovalPolicy: "never",
		ThreadSandbox:  "workspace-write",
		TurnTimeoutMS:  5000,
		ReadTimeoutMS:  5000,
	})
	result, err := runner.RunSession(context.Background(), SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         types.Issue{Identifier: "ZEE-1", Title: "continue"},
		Prompts: []TurnPrompt{
			{Text: "first prompt"},
			{Text: "continue prompt", Continuation: true},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != "thread-1" || len(result.Turns) != 2 {
		t.Fatalf("result = %#v", result)
	}
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Count(text, `"method":"thread/start"`) != 1 {
		t.Fatalf("thread/start count mismatch:\n%s", text)
	}
	if strings.Count(text, `"method":"turn/start"`) != 2 {
		t.Fatalf("turn/start count mismatch:\n%s", text)
	}
}
```

- [ ] **Step 3: Implement multi-turn session**

Refactor `Runner.RunSession`:

- Start the app-server subprocess once.
- Initialize once.
- Start one thread once.
- For each prompt in `request.Prompts`, call `session.startTurn`.
- Await completion before starting the next turn.
- Emit `session_started` once with pid/thread id.
- Emit `turn_started` and `turn_completed` per turn.
- Return all turn results and final `session_id` from the last turn.

- [ ] **Step 4: Build continuation guidance in orchestrator**

In `runAgent`, build `[]codex.TurnPrompt`:

- First turn text is rendered `WORKFLOW.md` body.
- Later in-worker continuation text is:

```text
Continue working on the same issue. Re-check the current workspace state, finish any remaining acceptance criteria from the issue, run the smallest relevant verification, and report concrete progress or blockers. Do not repeat completed work.
```

After each successful turn, refresh issue state. If still active and below `max_turns`, append the continuation prompt to the same live session instead of launching a new process.

- [ ] **Step 5: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/codex ./internal/orchestrator
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/codex/runner.go go/internal/codex/runner_test.go go/internal/orchestrator/orchestrator.go go/internal/orchestrator/orchestrator_test.go
git commit -m "feat(go): reuse codex session for continuation turns"
```

## Task 10: Normalize Structured Logs And Snapshot Fields

**Files:**
- Modify: `go/internal/logging/jsonl.go`
- Modify: `go/internal/logging/jsonl_test.go`
- Modify: `go/internal/orchestrator/orchestrator.go`
- Modify: `go/internal/observability/snapshot.go`

- [ ] **Step 1: Add logging test**

Append to `go/internal/logging/jsonl_test.go`:

```go
func TestLoggerWritesIssueAndSessionContext(t *testing.T) {
	logger, err := New(filepath.Join(t.TempDir(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	err = logger.Write(Event{
		IssueID:         "issue-id",
		IssueIdentifier: "ZEE-1",
		SessionID:       "thread-1-turn-1",
		Event:           "turn_completed",
		Message:         "completed",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(logger.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"issue_id":"issue-id"`, `"issue_identifier":"ZEE-1"`, `"session_id":"thread-1-turn-1"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("log missing %s:\n%s", want, text)
		}
	}
}
```

- [ ] **Step 2: Update logging event struct**

In `jsonl.go`, ensure `Event` has:

```go
IssueID         string         `json:"issue_id,omitempty"`
IssueIdentifier string         `json:"issue_identifier,omitempty"`
SessionID       string         `json:"session_id,omitempty"`
Event           string         `json:"event"`
Message         string         `json:"message"`
Fields          map[string]any `json:"fields,omitempty"`
```

Keep the existing timestamp behavior.

- [ ] **Step 3: Update orchestrator logging calls**

Change `o.log` to accept a `types.Issue` or an issue context struct so issue-id logs include both fields:

```go
func (o *Orchestrator) logIssue(issue types.Issue, event, message string, fields map[string]any) {
	if o.opts.Logger == nil {
		return
	}
	_ = o.opts.Logger.Write(logging.Event{
		IssueID:         issue.ID,
		IssueIdentifier: issue.Identifier,
		Event:           event,
		Message:         message,
		Fields:          fields,
	})
}
```

For session events, include `session_id` from `codex.Result` or `session_started` payload.

- [ ] **Step 4: Track latest rate-limit payload**

In `updateRunningFromEvent`, when event payload method or event name contains `rate_limit`, store payload into `snapshot.RateLimits`.

- [ ] **Step 5: Run tests**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/logging ./internal/orchestrator ./internal/observability
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go/internal/logging/jsonl.go go/internal/logging/jsonl_test.go go/internal/orchestrator/orchestrator.go go/internal/observability/snapshot.go
git commit -m "feat(go): normalize runtime logging context"
```

## Task 11: Document V3 Runtime Policy And Operator Flow

**Files:**
- Modify: `go/README.md`
- Modify: `go/WORKFLOW.md` only if existing config is missing fields required by v3 tests

- [ ] **Step 1: Update README sections**

Add these sections to `go/README.md`:

```markdown
## V3 Runtime Contract

The Go daemon is a high-trust local Symphony runner. It reads `WORKFLOW.md`, polls Linear, creates one sanitized workspace per issue, launches Codex only from that issue workspace, and preserves workspaces across successful runs.

`WORKFLOW.md` changes are reloaded without restart. Invalid edits do not replace the last known good workflow; the daemon logs the reload error and keeps reconciliation alive.

## Trust And Safety Posture

This implementation is intended for trusted local development environments. The default Codex approval policy is `never`, the default thread sandbox is `workspace-write`, and the turn sandbox includes the issue workspace plus git metadata roots needed by local worktrees. Operators should tighten these values in `WORKFLOW.md` before using untrusted issue sources.

Secrets are supplied by explicit `$VAR` references or `LINEAR_API_KEY`. The daemon validates secret presence without logging secret values.

## Retry And Reconciliation

Normal worker exits schedule a 1 second continuation retry so Symphony can re-check whether the issue is still active. Worker failures use exponential backoff starting at 10 seconds and capped by `agent.max_retry_backoff_ms`.

Every poll reconciles running issues against Linear. Terminal states cancel the worker and remove the workspace. Non-active non-terminal states cancel the worker and preserve the workspace.

## Validation

```bash
cd /Users/bytedance/symphony/go
make test
make build
```
```

- [ ] **Step 2: Ensure `go/WORKFLOW.md` includes v3 fields only when useful**

If `go/WORKFLOW.md` does not set these fields, leaving them absent is valid because v3 defaults cover them:

```yaml
agent:
  max_concurrent_agents: 2
  max_turns: 20
  max_retry_backoff_ms: 300000
codex:
  command: codex app-server
  stall_timeout_ms: 300000
hooks:
  timeout_ms: 60000
```

Only add explicit values if local smoke behavior needs them for clarity.

- [ ] **Step 3: Run docs-adjacent verification**

Run:

```bash
cd /Users/bytedance/symphony/go
make test
make build
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add go/README.md go/WORKFLOW.md
git commit -m "docs(go): document v3 runtime policy"
```

## Task 12: Final Core Conformance Pass

**Files:**
- Modify only files directly needed to fix failures found by this task

- [ ] **Step 1: Run full Go test suite**

Run:

```bash
cd /Users/bytedance/symphony/go
make test
```

Expected: PASS.

- [ ] **Step 2: Run full build**

Run:

```bash
cd /Users/bytedance/symphony/go
make build
```

Expected: PASS and binary exists at:

```text
/Users/bytedance/symphony/go/bin/symphony-go
```

- [ ] **Step 3: Run config-only local startup check**

Run with a fake Linear token and a one-shot issue filter that is unlikely to exist:

```bash
cd /Users/bytedance/symphony/go
LINEAR_API_KEY="${LINEAR_API_KEY:-lin_fake_for_config_check}" ./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue DOES-NOT-EXIST
```

Expected:

- If `LINEAR_API_KEY` is fake, the command may fail at Linear transport/auth. That is acceptable for a config-only local check.
- It must not fail with workflow parsing, missing config, workspace root normalization, or Codex command validation errors.

- [ ] **Step 4: Run optional real smoke only with valid credentials**

When valid Linear credentials and a safe smoke issue exist:

```bash
cd /Users/bytedance/symphony/go
make run-once ISSUE=ZEE-9
```

Expected:

- Terminal/TUI is disabled for the one-shot run.
- Workspace appears under `go/.worktrees/ZEE-9`.
- JSONL log appears under `go/.symphony/logs/`.
- If Codex produces a commit, the issue moves to `Human Review`.

- [ ] **Step 5: Update spec coverage note**

Append a short section to this plan after implementation:

```markdown
## Implementation Result

- Tests:
  - `make test`: PASS
  - `make build`: PASS
- Core conformance implemented:
  - Config resolution and validation
  - Dynamic reload
  - Linear read adapter
  - Workspace hooks and safety
  - Concurrent dispatch
  - Retry queue
  - Reconciliation and startup cleanup
  - Codex continuation sessions
  - Structured logs
- Deferred extensions:
  - HTTP `/api/v1/*`
  - SSH workers
```

- [ ] **Step 6: Commit**

```bash
git add go
git commit -m "feat(go): complete v3 core conformance"
```

## Self-Review

Spec coverage:

- Sections 5-6 workflow/config: covered by Tasks 2 and 3.
- Sections 7-8 orchestration/retry/reconciliation: covered by Tasks 6, 7, and 8.
- Section 9 workspace safety/hooks: covered by Task 5.
- Sections 10 and 12 Codex/prompt continuation: covered by Task 9 plus existing strict rendering.
- Section 11 Linear read adapter: covered by Task 4.
- Section 13 logs/status/token/rate limits: covered by Task 10 and existing observability/TUI.
- Sections 14-15 recovery/safety docs: covered by Tasks 8 and 11.
- Sections 17-18 test/checklist: covered by Task 12.

Deferred spec extensions:

- Section 13.7 HTTP server extension.
- Appendix A SSH worker extension.
- Durable retry/session persistence, which is explicitly not required for core conformance.

Placeholder scan:

- This plan has no `TBD`, no vague test-only steps, and no unspecified error handling steps.
- Tests that mention comments in Task 8 and Task 9 must be expanded into concrete fake implementations by the executor before those tasks are marked complete; the expected assertions and behavior are listed in the same task.

Type consistency:

- Config fields are added to `types.Config` first, then consumed by `internal/config`, orchestrator, workspace, and Codex.
- `AgentRunner` changes in Task 9 require orchestrator fakes to move from `Run` to `RunSession` in the same task.
- `Tracker` changes in Task 4 require all orchestrator fakes to implement `FetchIssuesByStates` and `FetchIssueStatesByIDs` in the same task.

## Implementation Result

- Tests:
  - `make test`: PASS
  - `make build`: PASS
  - `LINEAR_API_KEY="${LINEAR_API_KEY:-lin_fake_for_config_check}" ./bin/symphony-go run --workflow ./WORKFLOW.md --once --no-tui --issue DOES-NOT-EXIST`: PASS for workflow/config startup; no dispatchable issue matched the filter.
- Core conformance implemented:
  - Config resolution and validation
  - Dynamic reload
  - Linear read adapter
  - Workspace hooks and safety
  - Concurrent dispatch
  - Retry queue
  - Reconciliation and startup cleanup
  - Codex continuation sessions
  - Structured logs
- Deferred extensions:
  - HTTP `/api/v1/*`
  - SSH workers
