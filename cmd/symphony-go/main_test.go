package main

import "testing"

func TestDefaultRunOptionsEnableTUI(t *testing.T) {
	opts := defaultRunOptions()
	opts.ApplyDefaults()

	if !opts.TUI {
		t.Fatal("TUI should be enabled by default for continuous run")
	}
	if opts.MergeTarget != "main" {
		t.Fatalf("merge target = %q, want main", opts.MergeTarget)
	}
	if opts.WorkflowPath != "./WORKFLOW.md" {
		t.Fatalf("workflow path = %q, want ./WORKFLOW.md", opts.WorkflowPath)
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
