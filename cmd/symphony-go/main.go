package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"symphony-go/internal/app"
)

type runOptions struct {
	WorkflowPath       string
	Once               bool
	Issue              string
	MergeTarget        string
	mergeExplicit      bool
	ServerPort         int
	serverPortExplicit bool
	TUI                bool
	tuiExplicit        bool
}

func defaultRunOptions() runOptions {
	return runOptions{
		WorkflowPath: "./WORKFLOW.md",
		MergeTarget:  "main",
		TUI:          true,
	}
}

func parseRunOptions(args []string) (runOptions, error) {
	opts := defaultRunOptions()
	runFlags := flag.NewFlagSet("run", flag.ContinueOnError)
	runFlags.StringVar(&opts.WorkflowPath, "workflow", opts.WorkflowPath, "path to WORKFLOW.md")
	runFlags.BoolVar(&opts.Once, "once", opts.Once, "poll once and exit")
	runFlags.StringVar(&opts.Issue, "issue", opts.Issue, "optional issue identifier or id filter")
	runFlags.StringVar(&opts.MergeTarget, "merge-target", opts.MergeTarget, "local branch receiving Merging-state work")
	runFlags.IntVar(&opts.ServerPort, "port", opts.ServerPort, "start local HTTP control plane on this port")
	runFlags.Var(tuiFlag{opts: &opts, enabled: true}, "tui", "render terminal TUI")
	runFlags.Var(tuiFlag{opts: &opts, enabled: false}, "no-tui", "disable terminal TUI")
	if err := runFlags.Parse(args); err != nil {
		return runOptions{}, err
	}
	runFlags.Visit(func(f *flag.Flag) {
		if f.Name == "merge-target" {
			opts.mergeExplicit = true
		}
		if f.Name == "port" {
			opts.serverPortExplicit = true
		}
	})
	if opts.serverPortExplicit && opts.ServerPort < 0 {
		return runOptions{}, fmt.Errorf("port must be zero or positive")
	}
	opts.ApplyDefaults()
	return opts, nil
}

func (o *runOptions) ApplyDefaults() {
	if o.Once && !o.tuiExplicit {
		o.TUI = false
	}
}

type tuiFlag struct {
	opts    *runOptions
	enabled bool
}

func (f tuiFlag) String() string {
	if f.opts == nil || !f.opts.TUI {
		return "false"
	}
	return "true"
}

func (f tuiFlag) Set(value string) error {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	if parsed {
		f.opts.TUI = f.enabled
	} else {
		f.opts.TUI = !f.enabled
	}
	f.opts.tuiExplicit = true
	return nil
}

func (f tuiFlag) IsBoolFlag() bool {
	return true
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "run" {
		fmt.Fprintln(os.Stderr, "usage: symphony-go run --workflow ./WORKFLOW.md [--once] [--issue ZEE-8] [--port 0] [--tui|--no-tui]")
		os.Exit(2)
	}
	opts, err := parseRunOptions(os.Args[2:])
	if err != nil {
		os.Exit(2)
	}

	if err := app.RunWithSignals(opts.AppOptions()); err != nil {
		fatal(err)
	}
}

func mergeTargetOverride(opts runOptions) string {
	return app.MergeTargetOverride(opts.MergeTarget, opts.mergeExplicit)
}

func (o runOptions) AppOptions() app.Options {
	return app.Options{
		WorkflowPath:  o.WorkflowPath,
		Once:          o.Once,
		Issue:         o.Issue,
		MergeTarget:   o.MergeTarget,
		MergeExplicit: o.mergeExplicit,
		Server: app.ServerOptions{
			Port:         o.ServerPort,
			PortExplicit: o.serverPortExplicit,
		},
		TUI:    o.TUI,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "symphony-go:", err)
	os.Exit(1)
}
