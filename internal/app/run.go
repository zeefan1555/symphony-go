package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/zeefan1555/symphony-go/internal/codex"
	"github.com/zeefan1555/symphony-go/internal/control/hertzserver"
	"github.com/zeefan1555/symphony-go/internal/issuetracker/linear"
	"github.com/zeefan1555/symphony-go/internal/logging"
	"github.com/zeefan1555/symphony-go/internal/observability"
	"github.com/zeefan1555/symphony-go/internal/orchestrator"
	controlplane "github.com/zeefan1555/symphony-go/internal/service/control"
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
	Server        ServerOptions
	TUI           bool
	Stdout        io.Writer
	Stderr        io.Writer
}

type ServerOptions struct {
	Enabled      bool
	Host         string
	Port         int
	PortExplicit bool
}

type Runtime struct {
	Options        Options
	Loaded         *types.Workflow
	Service        runtimeService
	ControlServer  controlServer
	ControlAddress string
	Logger         io.Closer
}

const defaultServerHost = "127.0.0.1"
const controlShutdownTimeout = time.Second

var (
	ErrControlServerListen = errors.New("control HTTP server listen failed")
	ErrControlServerRun    = errors.New("control HTTP server failed")
)

type runtimeService interface {
	StartupCleanup(context.Context)
	Run(context.Context) error
	Snapshot() observability.Snapshot
}

type controlServer interface {
	Serve(net.Listener) error
	Shutdown(context.Context) error
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
	opts.Server = resolveServerOptions(loaded.Config.Server, opts.Server)
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
	for _, warning := range loaded.Config.Warnings {
		_ = log.Write(logging.Event{
			Level:   "warn",
			Event:   "config_warning",
			Message: warning.Message,
			Fields: map[string]any{
				"code": warning.Code,
			},
		})
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
		ControlServer: hertzserver.New(controlplane.NewServiceWithWorkspaceAndCodexRunner(service, manager, runner)),
		Logger:        log,
	}, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if r.Service == nil {
		return fmt.Errorf("app runtime service is nil")
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var controlErr <-chan error
	var stopControl func()
	if r.Options.Server.Enabled {
		var err error
		controlErr, stopControl, err = r.startControlServer()
		if err != nil {
			return err
		}
		defer stopControl()
	}

	r.Service.StartupCleanup(ctx)
	if r.Options.TUI {
		go renderTUI(runCtx, r.Service, tui.Options{
			MaxAgents:   r.Loaded.Config.Agent.MaxConcurrentAgents,
			ProjectSlug: r.Loaded.Config.Tracker.ProjectSlug,
			Color:       true,
		}, r.Options.Stdout)
	}

	serviceErr := make(chan error, 1)
	go func() {
		serviceErr <- r.Service.Run(runCtx)
	}()
	select {
	case err := <-serviceErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	case err := <-controlErr:
		cancel()
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("%w: %v", ErrControlServerRun, err)
		}
	case <-ctx.Done():
		cancel()
		err := <-serviceErr
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

func (r *Runtime) startControlServer() (<-chan error, func(), error) {
	if r.ControlServer == nil {
		return nil, nil, fmt.Errorf("app control server is nil")
	}
	host := r.Options.Server.Host
	if host == "" {
		host = defaultServerHost
	}
	address := net.JoinHostPort(host, strconv.Itoa(r.Options.Server.Port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %v", ErrControlServerListen, address, err)
	}
	r.ControlAddress = listener.Addr().String()

	errCh := make(chan error, 1)
	go func() {
		errCh <- r.ControlServer.Serve(listener)
	}()
	stop := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), controlShutdownTimeout)
		defer cancel()
		_ = r.ControlServer.Shutdown(shutdownCtx)
	}
	return errCh, stop, nil
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

func resolveServerOptions(workflow types.ServerConfig, opts ServerOptions) ServerOptions {
	if opts.PortExplicit {
		opts.Enabled = true
	} else if workflow.PortSet {
		opts.Enabled = true
		opts.Port = workflow.Port
	}
	if opts.Host == "" {
		opts.Host = defaultServerHost
	}
	return opts
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
