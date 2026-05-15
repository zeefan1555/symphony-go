package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"symphony-go/internal/app"
	"symphony-go/internal/service/workflow"
)

func TestDefaultRunOptionsEnableTUI(t *testing.T) {
	opts := defaultRunOptions()
	opts.ApplyDefaults()

	if !opts.TUI {
		t.Fatal("TUI should be enabled by default for continuous run")
	}
	if opts.MergeTarget != "main" {
		t.Fatalf("merge target = %q, want main", opts.MergeTarget)
	}
	if opts.WorkflowPath != "workflow.md" {
		t.Fatalf("workflow path = %q, want workflow.md", opts.WorkflowPath)
	}
	if opts.mergeExplicit {
		t.Fatal("default merge target should not be marked explicit")
	}
}

func TestParseRunOptionsMarksExplicitMergeTarget(t *testing.T) {
	opts, err := parseRunOptions([]string{"--merge-target", "release"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}
	if opts.MergeTarget != "release" {
		t.Fatalf("merge target = %q, want release", opts.MergeTarget)
	}
	if !opts.mergeExplicit {
		t.Fatal("merge target flag should be marked explicit")
	}
}

func TestParseRunOptionsAcceptsPositionalWorkflowPath(t *testing.T) {
	opts, err := parseRunOptions([]string{"custom.WORKFLOW.md"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}
	if opts.WorkflowPath != "custom.WORKFLOW.md" {
		t.Fatalf("workflow path = %q, want custom.WORKFLOW.md", opts.WorkflowPath)
	}
	if opts.workflowExplicit {
		t.Fatal("positional workflow path should not mark workflow flag explicit")
	}
}

func TestParseRunOptionsWorkflowFlagOverridesPositionalPath(t *testing.T) {
	opts, err := parseRunOptions([]string{"--workflow", "flag.WORKFLOW.md", "pos.WORKFLOW.md"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}
	if opts.WorkflowPath != "flag.WORKFLOW.md" {
		t.Fatalf("workflow path = %q, want flag.WORKFLOW.md", opts.WorkflowPath)
	}
	if !opts.workflowExplicit {
		t.Fatal("workflow flag should be marked explicit")
	}
}

func TestParseRunOptionsRejectsMultiplePositionalWorkflowPaths(t *testing.T) {
	_, err := parseRunOptions([]string{"one.WORKFLOW.md", "two.WORKFLOW.md"})
	if err == nil {
		t.Fatal("parseRunOptions() error = nil, want multiple workflow path error")
	}
}

func TestParseRunOptionsMarksExplicitHTTPPort(t *testing.T) {
	opts, err := parseRunOptions([]string{"--port", "0"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}
	if opts.ServerPort != 0 {
		t.Fatalf("server port = %d, want 0", opts.ServerPort)
	}
	if !opts.serverPortExplicit {
		t.Fatal("server port flag should be marked explicit")
	}
	appOpts := opts.AppOptions()
	if appOpts.Server.Port != 0 || !appOpts.Server.PortExplicit {
		t.Fatalf("app server options = %#v, want explicit port 0", appOpts.Server)
	}
}

func TestParseRunOptionsRejectsNegativeHTTPPort(t *testing.T) {
	_, err := parseRunOptions([]string{"--port", "-1"})
	if err == nil {
		t.Fatal("parseRunOptions() error = nil, want negative port error")
	}
}

func TestMergeTargetOverrideOnlyWhenExplicit(t *testing.T) {
	implicit := defaultRunOptions()
	if got := mergeTargetOverride(implicit); got != "" {
		t.Fatalf("implicit override = %q, want empty workflow-controlled target", got)
	}

	explicit, err := parseRunOptions([]string{"--merge-target", "release"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}
	if got := mergeTargetOverride(explicit); got != "release" {
		t.Fatalf("explicit override = %q, want release", got)
	}
}

func TestOnceDisablesTUIWhenNotExplicit(t *testing.T) {
	opts, err := parseRunOptions([]string{"--once"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}

	if opts.TUI {
		t.Fatal("once mode should disable TUI by default")
	}
}

func TestOnceAllowsExplicitTUI(t *testing.T) {
	opts, err := parseRunOptions([]string{"--once", "--tui"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}

	if !opts.TUI {
		t.Fatal("explicit --tui should enable TUI in once mode")
	}
}

func TestExplicitNoTUIDisablesContinuousTUI(t *testing.T) {
	opts, err := parseRunOptions([]string{"--no-tui"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}

	if opts.TUI {
		t.Fatal("explicit --no-tui should disable TUI")
	}
}

func TestTUIFalseDisablesContinuousTUI(t *testing.T) {
	opts, err := parseRunOptions([]string{"--tui=false"})
	if err != nil {
		t.Fatalf("parseRunOptions() error = %v", err)
	}

	if opts.TUI {
		t.Fatal("explicit --tui=false should disable TUI")
	}
}

func TestRunMainLifecycleExitCodes(t *testing.T) {
	startupErr := errors.New("workflow load failed")
	tests := []struct {
		name      string
		args      []string
		run       func(app.Options) error
		wantCode  int
		wantCalls int
	}{
		{
			name:     "missing command exits usage",
			args:     []string{"symphony-go"},
			wantCode: 2,
		},
		{
			name:     "parse failure exits usage",
			args:     []string{"symphony-go", "run", "--port", "-1"},
			wantCode: 2,
		},
		{
			name: "normal success exits zero",
			args: []string{"symphony-go", "run", "--once", "--workflow", "custom.WORKFLOW.md"},
			run: func(opts app.Options) error {
				if opts.WorkflowPath != "custom.WORKFLOW.md" || !opts.Once || opts.TUI {
					t.Fatalf("app options = %#v, want once custom workflow without TUI", opts)
				}
				return nil
			},
			wantCode:  0,
			wantCalls: 1,
		},
		{
			name: "startup failure exits nonzero",
			args: []string{"symphony-go", "run", "--workflow", "missing.WORKFLOW.md"},
			run: func(app.Options) error {
				return startupErr
			},
			wantCode:  1,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			run := tt.run
			if run == nil {
				run = func(app.Options) error {
					t.Fatal("app runner should not be called")
					return nil
				}
			}
			code := runMain(tt.args, func(opts app.Options) error {
				calls++
				return run(opts)
			})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d", code, tt.wantCode)
			}
			if calls != tt.wantCalls {
				t.Fatalf("app runner calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

func TestParseInitOptionsDefaultsToRepoLocalWorkflow(t *testing.T) {
	opts, err := parseInitOptions(nil)
	if err != nil {
		t.Fatalf("parseInitOptions() error = %v", err)
	}
	if opts.Output != "workflow.md" {
		t.Fatalf("output = %q, want workflow.md", opts.Output)
	}
	if opts.Dir != "." {
		t.Fatalf("dir = %q, want .", opts.Dir)
	}
}

func TestRunMainInitWritesRepoLocalWorkflow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runMain([]string{"symphony-go", "init", "--dir", dir, "--project-slug", "demo-project", "--merge-target", "release"}, nil)
	if code != 0 {
		t.Fatalf("init exit code = %d, want 0", code)
	}

	content, err := os.ReadFile(filepath.Join(dir, "workflow.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{
		`project_slug: "demo-project"`,
		`mode: static_cwd`,
		`cwd: .`,
		`target: "release"`,
		"本仓存在 `AGENTS.md`",
		"go test ./...",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated workflow missing %q:\n%s", want, text)
		}
	}
}

func TestGeneratedInitialWorkflowLoads(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_test")
	dir := t.TempDir()
	path := filepath.Join(dir, "workflow.md")
	content := generateInitialWorkflow(initOptions{
		ProjectSlug: "demo-project",
		MergeTarget: "main",
	}, localRepoInfo{
		ProjectName:   "demo",
		DefaultBranch: "main",
	})
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := workflow.Load(path)
	if err != nil {
		t.Fatalf("workflow.Load() error = %v", err)
	}
	if loaded.Config.Workspace.Mode != "static_cwd" {
		t.Fatalf("workspace mode = %q, want static_cwd", loaded.Config.Workspace.Mode)
	}
	if loaded.Config.Workspace.CWD != dir {
		t.Fatalf("workspace cwd = %q, want %q", loaded.Config.Workspace.CWD, dir)
	}
	if loaded.Config.Tracker.ProjectSlug != "demo-project" {
		t.Fatalf("project slug = %q, want demo-project", loaded.Config.Tracker.ProjectSlug)
	}
}

func TestRunMainInitRefusesExistingWorkflowWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "workflow.md"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code := runMain([]string{"symphony-go", "init", "--dir", dir}, nil)
	if code != 1 {
		t.Fatalf("init exit code = %d, want 1", code)
	}
}
