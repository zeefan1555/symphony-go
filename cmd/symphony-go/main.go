package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	"github.com/zeefan1555/symphony-go/internal/linear"
	"github.com/zeefan1555/symphony-go/internal/logging"
	"github.com/zeefan1555/symphony-go/internal/orchestrator"
	"github.com/zeefan1555/symphony-go/internal/tui"
	"github.com/zeefan1555/symphony-go/internal/types"
	"github.com/zeefan1555/symphony-go/internal/workflow"
	"github.com/zeefan1555/symphony-go/internal/workspace"
)

type runOptions struct {
	WorkflowPath string
	Once         bool
	Issue        string
	MergeTarget  string
	TUI          bool
	tuiExplicit  bool
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
	runFlags.Var(tuiFlag{opts: &opts, enabled: true}, "tui", "render terminal TUI")
	runFlags.Var(tuiFlag{opts: &opts, enabled: false}, "no-tui", "disable terminal TUI")
	if err := runFlags.Parse(args); err != nil {
		return runOptions{}, err
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
		fmt.Fprintln(os.Stderr, "usage: symphony-go run --workflow ./WORKFLOW.md [--once] [--issue ZEE-8] [--tui|--no-tui]")
		os.Exit(2)
	}
	opts, err := parseRunOptions(os.Args[2:])
	if err != nil {
		os.Exit(2)
	}

	reloader, err := workflow.NewReloader(opts.WorkflowPath)
	if err != nil {
		fatal(err)
	}
	loaded := reloader.Current()
	tracker, err := linear.New(loaded.Config.Tracker)
	if err != nil {
		fatal(err)
	}
	absWorkflow, err := filepath.Abs(opts.WorkflowPath)
	if err != nil {
		fatal(err)
	}
	repoRoot := orchestrator.RepoRootFromWorkflow(absWorkflow)
	logPath := logging.LogPath(filepath.Dir(absWorkflow))
	logOptions := []logging.Option{
		logging.WithHumanFile(logging.HumanLogPath(logPath), false),
		logging.WithHumanFileMinLevel(slog.LevelDebug),
	}
	if !opts.TUI {
		logOptions = append(logOptions, logging.WithConsole(os.Stderr, true))
	}
	log, err := logging.New(logPath, logOptions...)
	if err != nil {
		fatal(err)
	}
	defer log.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	manager := workspace.New(loaded.Config.Workspace.Root, loaded.Config.Hooks)
	runner := codex.New(loaded.Config.Codex)
	service := orchestrator.New(orchestrator.Options{
		Workflow:  loaded,
		Tracker:   tracker,
		Workspace: manager,
		Runner:    runner,
		TrackerFactory: func(cfg types.TrackerConfig) (orchestrator.Tracker, error) {
			return linear.New(cfg)
		},
		WorkspaceFactory: func(cfg types.WorkspaceConfig, hooks types.HooksConfig) *workspace.Manager {
			return workspace.New(cfg.Root, hooks)
		},
		RunnerFactory: func(cfg types.CodexConfig) orchestrator.AgentRunner {
			return codex.New(cfg)
		},
		Logger:      log,
		Reloader:    reloader,
		Once:        opts.Once,
		IssueFilter: opts.Issue,
		RepoRoot:    repoRoot,
		MergeTarget: orchestrator.NormalizeMergeTarget(opts.MergeTarget),
	})
	service.StartupCleanup(ctx)
	if opts.TUI {
		go renderTUI(ctx, service, tui.Options{
			MaxAgents:   loaded.Config.Agent.MaxConcurrentAgents,
			ProjectSlug: loaded.Config.Tracker.ProjectSlug,
			Color:       true,
		})
	}
	if err := service.Run(ctx); err != nil && err != context.Canceled {
		fatal(err)
	}
}

func renderTUI(ctx context.Context, service *orchestrator.Orchestrator, opts tui.Options) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame := tui.Render(service.Snapshot(), opts)
			fmt.Print(tui.ClearAndRender(frame))
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "symphony-go:", err)
	os.Exit(1)
}
