package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeefan1555/symphony-go/internal/observability"
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
	if got := runtime.Loaded.Config.Workspace.Root; !filepath.IsAbs(got) {
		t.Fatalf("workspace root = %q, want absolute path", got)
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

func writeWorkflow(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	content := `---
tracker:
  kind: linear
  api_key: test-key
  project_slug: symphony_test
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
	runErr  error
	cleaned bool
}

func (f *fakeService) StartupCleanup(context.Context) {
	f.cleaned = true
}

func (f *fakeService) Run(context.Context) error {
	return f.runErr
}

func (f *fakeService) Snapshot() observability.Snapshot {
	return observability.Snapshot{}
}

type fakeCloser struct {
	closed bool
}

func (f *fakeCloser) Close() error {
	f.closed = true
	return nil
}
