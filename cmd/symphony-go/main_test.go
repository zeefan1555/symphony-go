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
