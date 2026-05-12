package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"symphony-go/internal/runtime/observability"
	controlplane "symphony-go/internal/service/control"
	"symphony-go/internal/transport/hertzserver"
)

func TestNewRuntimeAssemblesRunDependencies(t *testing.T) {
	workflowPath := writeWorkflow(t)

	runtime, err := NewRuntime(Options{WorkflowPath: workflowPath, TUI: false})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close()

	if runtime.Service == nil {
		t.Fatal("runtime Service is nil")
	}
	if runtime.ControlServer == nil {
		t.Fatal("runtime ControlServer is nil")
	}
	if runtime.Loaded == nil {
		t.Fatal("runtime Loaded workflow is nil")
	}
	if runtime.runner == nil || len(runtime.runner.DynamicToolSpecs()) != 1 {
		t.Fatal("runtime Codex runner is missing Linear GraphQL dynamic tool")
	}
	if runtime.Telemetry == nil || runtime.Telemetry.Enabled() {
		t.Fatal("runtime telemetry should be present and disabled without OTLP endpoint")
	}
	if got := runtime.Loaded.Config.Workspace.Root; !filepath.IsAbs(got) {
		t.Fatalf("workspace root = %q, want absolute path", got)
	}
	if runtime.Options.Server.Enabled {
		t.Fatal("control HTTP server should be disabled without CLI or workflow port")
	}
}

func TestNewRuntimeUsesWorkflowServerPort(t *testing.T) {
	workflowPath := writeWorkflow(t, "server:\n  port: 0\n")

	runtime, err := NewRuntime(Options{WorkflowPath: workflowPath, TUI: false})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close()

	if !runtime.Options.Server.Enabled {
		t.Fatal("workflow server.port should enable control HTTP server")
	}
	if runtime.Options.Server.Host != defaultServerHost {
		t.Fatalf("server host = %q, want %q", runtime.Options.Server.Host, defaultServerHost)
	}
	if runtime.Options.Server.Port != 0 {
		t.Fatalf("server port = %d, want 0", runtime.Options.Server.Port)
	}
}

func TestNewRuntimeCLIPortOverridesWorkflowServerPort(t *testing.T) {
	workflowPath := writeWorkflow(t, "server:\n  port: 10001\n")

	runtime, err := NewRuntime(Options{
		WorkflowPath: workflowPath,
		TUI:          false,
		Server:       ServerOptions{Port: 0, PortExplicit: true},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close()

	if !runtime.Options.Server.Enabled {
		t.Fatal("explicit CLI port should enable control HTTP server")
	}
	if runtime.Options.Server.Port != 0 {
		t.Fatalf("server port = %d, want CLI override 0", runtime.Options.Server.Port)
	}
}

func TestNewRuntimePropagatesWorkflowLoadError(t *testing.T) {
	_, err := NewRuntime(Options{WorkflowPath: filepath.Join(t.TempDir(), "missing.md")})
	if err == nil {
		t.Fatal("NewRuntime() error = nil, want missing workflow error")
	}
}

func TestRuntimeRunPropagatesServiceError(t *testing.T) {
	want := errors.New("run failed")
	service := &fakeService{runErr: want}
	runtime := Runtime{Service: service}

	err := runtime.Run(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want %v", err, want)
	}
	if !service.cleaned {
		t.Fatal("StartupCleanup was not called")
	}
}

func TestRuntimeRunStartsHTTPControlServer(t *testing.T) {
	service := &fakeService{
		blockUntilCanceled: true,
		started:            make(chan struct{}),
		snapshot: observability.Snapshot{
			GeneratedAt: time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
			Counts:      observability.Counts{Running: 1},
			Running: []observability.RunningEntry{{
				IssueID:         "issue-id",
				IssueIdentifier: "ZEE-55",
				State:           "In Progress",
				StartedAt:       time.Date(2026, 5, 3, 11, 0, 0, 0, time.UTC),
			}},
		},
	}
	runtime := Runtime{
		Options:       Options{Server: ServerOptions{Enabled: true, Host: defaultServerHost, Port: 0}},
		Service:       service,
		ControlServer: hertzserver.New(controlplane.NewService(service)),
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runtime.Run(ctx)
	}()

	select {
	case <-service.started:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start")
	}
	if runtime.ControlAddress == "" {
		t.Fatal("control address was not recorded")
	}
	resp, err := http.Post("http://"+runtime.ControlAddress+"/api/v1/control/get-state", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST get-state: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		State struct {
			Counts struct {
				Running int `json:"running"`
			} `json:"counts"`
			Running []struct {
				IssueIdentifier string `json:"issue_identifier"`
			} `json:"running"`
		} `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
	if body.State.Counts.Running != 1 || len(body.State.Running) != 1 || body.State.Running[0].IssueIdentifier != "ZEE-55" {
		t.Fatalf("state response = %#v, want same runtime snapshot", body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not shut down")
	}
	if !service.cleaned {
		t.Fatal("StartupCleanup was not called")
	}
}

func TestRuntimeRunReportsHTTPBindFailure(t *testing.T) {
	listener, err := net.Listen("tcp", net.JoinHostPort(defaultServerHost, "0"))
	if err != nil {
		t.Fatalf("listen occupied port: %v", err)
	}
	defer listener.Close()

	service := &fakeService{}
	runtime := Runtime{
		Options:       Options{Server: ServerOptions{Enabled: true, Host: defaultServerHost, Port: listener.Addr().(*net.TCPAddr).Port}},
		Service:       service,
		ControlServer: hertzserver.New(controlplane.NewService(service)),
	}

	err = runtime.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want bind failure")
	}
	if !errors.Is(err, ErrControlServerListen) {
		t.Fatalf("Run() error = %v, want ErrControlServerListen", err)
	}
	if service.cleaned {
		t.Fatal("service should not start when HTTP bind fails")
	}
}

func TestRuntimeCloseClosesLogger(t *testing.T) {
	closer := &fakeCloser{}
	runtime := Runtime{Logger: closer}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !closer.closed {
		t.Fatal("logger was not closed")
	}
}

func TestRuntimeCloseShutsDownTelemetry(t *testing.T) {
	telemetry := &fakeTelemetry{}
	runtime := Runtime{Telemetry: telemetry}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !telemetry.closed {
		t.Fatal("telemetry was not shut down")
	}
}

func writeWorkflow(t *testing.T, extraFrontMatter ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	extra := ""
	for _, value := range extraFrontMatter {
		extra += value
	}
	content := `---
tracker:
  kind: linear
  api_key: test-key
  project_slug: symphony_test
` + extra + `
workspace:
  root: ./worktrees
codex:
  command: codex app-server
polling:
  interval_ms: 1000
---
Handle {{ issue.identifier }}.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return path
}

type fakeService struct {
	runErr             error
	cleaned            bool
	blockUntilCanceled bool
	started            chan struct{}
	snapshot           observability.Snapshot
}

func (f *fakeService) StartupCleanup(context.Context) {
	f.cleaned = true
}

func (f *fakeService) Run(ctx context.Context) error {
	if f.started != nil {
		close(f.started)
	}
	if f.blockUntilCanceled {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.runErr
}

func (f *fakeService) Snapshot() observability.Snapshot {
	return f.snapshot
}

type fakeCloser struct {
	closed bool
}

func (f *fakeCloser) Close() error {
	f.closed = true
	return nil
}

type fakeTelemetry struct {
	closed bool
}

func (f *fakeTelemetry) Enabled() bool {
	return false
}

func (f *fakeTelemetry) Tracer() trace.Tracer {
	return nooptrace.NewTracerProvider().Tracer("test")
}

func (f *fakeTelemetry) Meter() metric.Meter {
	return noopmetric.NewMeterProvider().Meter("test")
}

func (f *fakeTelemetry) Shutdown(context.Context) error {
	f.closed = true
	return nil
}
