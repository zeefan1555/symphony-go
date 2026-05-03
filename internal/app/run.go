package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	controlplane "github.com/zeefan1555/symphony-go/internal/control"
	"github.com/zeefan1555/symphony-go/internal/control/hertzserver"
	"github.com/zeefan1555/symphony-go/internal/linear"
	"github.com/zeefan1555/symphony-go/internal/logging"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/orchestrator"
	"github.com/zeefan1555/symphony-go/internal/tui"
	"github.com/zeefan1555/symphony-go/internal/types"
	"github.com/zeefan1555/symphony-go/internal/workflow"
	"github.com/zeefan1555/symphony-go/internal/workspace"
)

type Options struct {
	WorkflowPath  string
	Once          bool
	Issue         string
	MergeTarget   string
	MergeExplicit bool
	TUI           bool
	Stdout        io.Writer
	Stderr        io.Writer
}

type Runtime struct {
	Options       Options
	Loaded        *types.Workflow
	Service       runtimeService
	ControlServer *hertzserver.Server
	Logger        io.Closer
}

type runtimeService interface {
	StartupCleanup(context.Context)
	Run(context.Context) error
	Snapshot() observability.Snapshot
}

func RunWithSignals(opts Options) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, opts)
}

func Run(ctx context.Context, opts Options) error {
	runtime, err := NewRuntime(opts)
	if err != nil {
		return err
	}
	defer runtime.Close()
	return runtime.Run(ctx)
}

func NewRuntime(opts Options) (*Runtime, error) {
	opts.applyDefaults()

	reloader, err := workflow.NewReloader(opts.WorkflowPath)
	if err != nil {
		return nil, err
	}
	loaded := reloader.Current()
	tracker, err := linear.New(loaded.Config.Tracker)
	if err != nil {
		return nil, err
	}
	absWorkflow, err := filepath.Abs(opts.WorkflowPath)
	if err != nil {
		return nil, err
	}
	repoRoot := orchestrator.RepoRootFromWorkflow(absWorkflow)
	logPath := logging.LogPath(filepath.Dir(absWorkflow))
	logOptions := []logging.Option{
		logging.WithHumanFile(logging.HumanLogPath(logPath), false),
		logging.WithHumanFileMinLevel(slog.LevelDebug),
	}
	if !opts.TUI {
		logOptions = append(logOptions, logging.WithConsole(opts.Stderr, true))
	}
	log, err := logging.New(logPath, logOptions...)
	if err != nil {
		return nil, err
	}

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
		MergeTarget: MergeTargetOverride(opts.MergeTarget, opts.MergeExplicit),
	})

	return &Runtime{
		Options:       opts,
		Loaded:        loaded,
		Service:       service,
		ControlServer: hertzserver.New(controlplane.NewService(service)),
		Logger:        log,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if r.Service == nil {
		return fmt.Errorf("app runtime service is nil")
	}
	r.Service.StartupCleanup(ctx)
	if r.Options.TUI {
		go renderTUI(ctx, r.Service, tui.Options{
			MaxAgents:   r.Loaded.Config.Agent.MaxConcurrentAgents,
			ProjectSlug: r.Loaded.Config.Tracker.ProjectSlug,
			Color:       true,
		}, r.Options.Stdout)
	}
	if err := r.Service.Run(ctx); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

func (r *Runtime) Close() error {
	if r == nil || r.Logger == nil {
		return nil
	}
	return r.Logger.Close()
}

func MergeTargetOverride(target string, explicit bool) string {
	if explicit {
		return orchestrator.NormalizeMergeTarget(target)
	}
	return ""
}

func (o *Options) applyDefaults() {
	if o.WorkflowPath == "" {
		o.WorkflowPath = "./WORKFLOW.md"
	}
	if o.MergeTarget == "" {
		o.MergeTarget = "main"
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

func renderTUI(ctx context.Context, service runtimeService, opts tui.Options, out io.Writer) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			frame := tui.Render(service.Snapshot(), opts)
			fmt.Fprint(out, tui.ClearAndRender(frame))
		}
	}
}
