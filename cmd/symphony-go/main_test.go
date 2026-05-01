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
