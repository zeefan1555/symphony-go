package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
)

func TestReloaderKeepsLastGoodWorkflowAfterInvalidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	writeWorkflow(t, path, "1000", "hello {{ issue.identifier }}")

	reloader, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	first := reloader.Current()
	if first.Config.Polling.IntervalMS != 1000 {
		t.Fatalf("first interval = %d", first.Config.Polling.IntervalMS)
	}

	if err := os.WriteFile(path, []byte("---\ntracker: [not-a-map\n---\nbad"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidate, changed, err := reloader.ReloadIfChanged()
	if err == nil {
		t.Fatal("expected invalid reload error")
	}
	if candidate != nil {
		t.Fatalf("invalid reload candidate = %#v, want nil", candidate)
	}
	if changed {
		t.Fatal("invalid reload must not replace current workflow")
	}
	current := reloader.Current()
	if current.Config.Polling.IntervalMS != 1000 {
		t.Fatalf("last good interval was not preserved: %d", current.Config.Polling.IntervalMS)
	}
}

func TestReloaderAppliesValidEdit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	writeWorkflow(t, path, "1000", "first")

	reloader, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	writeWorkflow(t, path, "2000", "second {{ issue.identifier }}")

	candidate, changed, err := reloader.ReloadIfChanged()
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected reload")
	}
	if candidate.Config.Polling.IntervalMS != 2000 {
		t.Fatalf("candidate interval = %d", candidate.Config.Polling.IntervalMS)
	}
	if current := reloader.Current(); current.Config.Polling.IntervalMS != 1000 {
		t.Fatalf("current advanced before commit: %d", current.Config.Polling.IntervalMS)
	}
	reloader.CommitCandidate()
	current := reloader.Current()
	if current.Config.Polling.IntervalMS != 2000 {
		t.Fatalf("interval = %d", current.Config.Polling.IntervalMS)
	}
	if current.PromptTemplate != "second {{ issue.identifier }}" {
		t.Fatalf("prompt = %q", current.PromptTemplate)
	}
}

func TestNewReloaderRetriesWhenFileChangesDuringInitialLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "WORKFLOW.md")
	oldTime := time.Unix(100, 0)
	newTime := time.Unix(200, 0)
	stats := []os.FileInfo{
		fakeFileInfo{modTime: oldTime, size: 10},
		fakeFileInfo{modTime: newTime, size: 20},
		fakeFileInfo{modTime: newTime, size: 20},
		fakeFileInfo{modTime: newTime, size: 20},
	}
	loads := []*runtimeconfig.Workflow{
		{Config: runtimeconfig.Config{Polling: runtimeconfig.PollingConfig{IntervalMS: 1000}}, PromptTemplate: "old"},
		{Config: runtimeconfig.Config{Polling: runtimeconfig.PollingConfig{IntervalMS: 2000}}, PromptTemplate: "new"},
	}
	statCalls := 0
	loadCalls := 0

	reloader, err := newReloader(path, func(string) (os.FileInfo, error) {
		if statCalls >= len(stats) {
			return nil, errors.New("unexpected stat")
		}
		info := stats[statCalls]
		statCalls++
		return info, nil
	}, func(string) (*runtimeconfig.Workflow, error) {
		if loadCalls >= len(loads) {
			return nil, errors.New("unexpected load")
		}
		loaded := loads[loadCalls]
		loadCalls++
		return loaded, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if loadCalls != 2 {
		t.Fatalf("load calls = %d, want 2", loadCalls)
	}
	current := reloader.Current()
	if current.Config.Polling.IntervalMS != 2000 {
		t.Fatalf("interval = %d, want 2000", current.Config.Polling.IntervalMS)
	}
	if current.PromptTemplate != "new" {
		t.Fatalf("prompt = %q, want new", current.PromptTemplate)
	}
	if !reloader.modTime.Equal(newTime) || reloader.size != 20 {
		t.Fatalf("recorded stat = (%s, %d), want (%s, 20)", reloader.modTime, reloader.size, newTime)
	}
}

func TestReloaderCurrentClonesTurnSandboxPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "WORKFLOW.md")
	t.Setenv("LINEAR_API_KEY", "lin_test")
	content := `---
tracker:
  kind: linear
  project_slug: demo
codex:
  turn_sandbox_policy:
    mode: workspace-write
    writable_roots:
      - /tmp/one
    network:
      allow: true
---
prompt
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	reloader, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	current := reloader.Current()
	current.Config.Codex.TurnSandboxPolicy["mode"] = "danger-full-access"
	current.Config.Codex.TurnSandboxPolicy["writable_roots"].([]any)[0] = "/tmp/two"
	current.Config.Codex.TurnSandboxPolicy["network"].(map[string]any)["allow"] = false

	next := reloader.Current()
	if next.Config.Codex.TurnSandboxPolicy["mode"] != "workspace-write" {
		t.Fatalf("mode was mutated: %#v", next.Config.Codex.TurnSandboxPolicy["mode"])
	}
	if next.Config.Codex.TurnSandboxPolicy["writable_roots"].([]any)[0] != "/tmp/one" {
		t.Fatalf("writable_roots was mutated: %#v", next.Config.Codex.TurnSandboxPolicy["writable_roots"])
	}
	if next.Config.Codex.TurnSandboxPolicy["network"].(map[string]any)["allow"] != true {
		t.Fatalf("nested policy was mutated: %#v", next.Config.Codex.TurnSandboxPolicy["network"])
	}
}

type fakeFileInfo struct {
	modTime time.Time
	size    int64
}

func (f fakeFileInfo) Name() string       { return "WORKFLOW.md" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func writeWorkflow(t *testing.T, path string, interval string, prompt string) {
	t.Helper()
	content := `---
tracker:
  kind: linear
  project_slug: demo
polling:
  interval_ms: ` + interval + `
---
` + prompt + `
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
