package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeefan1555/symphony-go/internal/types"
)

func TestEnsureRunsAfterCreateOnceWithUTF8Env(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{
		AfterCreate: `printf '%s\n' "$LANG|$LC_ALL|中文" > hook.txt`,
		TimeoutMS:   5000,
	})
	issue := types.Issue{Identifier: "ZEE-中文"}
	workspacePath, created, err := manager.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first ensure should create workspace")
	}
	raw, err := os.ReadFile(filepath.Join(workspacePath, "hook.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "UTF-8") || !strings.Contains(string(raw), "中文") {
		t.Fatalf("hook output = %q", string(raw))
	}
	_, created, err = manager.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second ensure should reuse workspace")
	}
}

func TestHookObserverReceivesLifecycleEventsWithIssueContext(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{
		AfterCreate: `printf 'hook-output'`,
		TimeoutMS:   5000,
	})
	var events []HookEvent
	manager.SetHookObserver(func(event HookEvent) {
		events = append(events, event)
	})
	issue := types.Issue{ID: "issue-id", Identifier: "ZEE-HOOK"}
	_, created, err := manager.Ensure(WithHookIssue(context.Background(), issue), issue)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first ensure should create workspace")
	}
	if len(events) != 2 {
		t.Fatalf("hook events = %#v, want started and completed", events)
	}
	started := events[0]
	if started.Name != "after_create" || started.Stage != "started" || started.Script != "printf 'hook-output'" {
		t.Fatalf("started event = %#v", started)
	}
	completed := events[1]
	if completed.Name != "after_create" || completed.Stage != "completed" {
		t.Fatalf("completed event = %#v", completed)
	}
	if completed.IssueID != issue.ID || completed.IssueIdentifier != issue.Identifier {
		t.Fatalf("completed issue context = %#v", completed)
	}
	if completed.Output != "hook-output" {
		t.Fatalf("completed output = %q", completed.Output)
	}
	if completed.Duration <= 0 {
		t.Fatalf("completed duration = %s, want positive", completed.Duration)
	}
}

func TestPathForIssueStaysInsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{})
	path, err := manager.PathForIssue(types.Issue{Identifier: "../ZEE/unsafe"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != root {
		t.Fatalf("path escaped root: %q", path)
	}
	if filepath.Base(path) != ".._ZEE_unsafe" {
		t.Fatalf("sanitized base = %q", filepath.Base(path))
	}
	if err := manager.ValidateWorkspacePath(path); err != nil {
		t.Fatal(err)
	}
	workspacePath, created, err := manager.Ensure(context.Background(), types.Issue{Identifier: "../ZEE/unsafe"})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("ensure should create sanitized workspace")
	}
	if workspacePath != path {
		t.Fatalf("workspace path = %q, want %q", workspacePath, path)
	}
}

func TestPathForIssueSanitizesDotDotIdentifierInsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{})
	path, err := manager.PathForIssue(types.Issue{Identifier: ".."})
	if err != nil {
		t.Fatal(err)
	}
	if path == filepath.Dir(root) {
		t.Fatalf("path escaped to root parent: %q", path)
	}
	if filepath.Dir(path) != root {
		t.Fatalf("path is not a direct child of root: %q", path)
	}
	if err := manager.ValidateWorkspacePath(path); err != nil {
		t.Fatal(err)
	}
}

func TestBeforeRunAndAfterRunHookSemantics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{
		BeforeRun: `printf before >> order.txt`,
		AfterRun:  `printf after >> order.txt; exit 9`,
		TimeoutMS: 5000,
	})
	issue := types.Issue{Identifier: "ZEE-HOOKS"}
	workspacePath, _, err := manager.Ensure(context.Background(), issue)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.BeforeRun(context.Background(), workspacePath); err != nil {
		t.Fatal(err)
	}
	err = manager.AfterRun(context.Background(), workspacePath)
	if err == nil {
		t.Fatal("after_run should return an error for logging")
	}
	raw, err := os.ReadFile(filepath.Join(workspacePath, "order.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "beforeafter" {
		t.Fatalf("hook order = %q", string(raw))
	}
}

func TestValidateWorkspacePathRejectsEscapes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "worktrees")
	manager := New(root, types.HooksConfig{})
	err := manager.ValidateWorkspacePath(filepath.Join(filepath.Dir(root), "outside"))
	if err == nil {
		t.Fatal("expected escaped workspace path to be rejected")
	}
}

func TestEnsureReplacesSymlinkWorkspaceEscapingRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "worktrees")
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ZEE")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	manager := New(root, types.HooksConfig{})
	workspacePath, created, err := manager.Ensure(context.Background(), types.Issue{Identifier: "ZEE"})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("ensure should replace symlink with a real workspace")
	}
	if workspacePath != path {
		t.Fatalf("workspace path = %q, want %q", workspacePath, path)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		t.Fatalf("workspace should be real directory, mode=%s", info.Mode())
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside target should remain intact: %v", err)
	}
}

func TestValidateAndBeforeRunRejectSymlinkWorkspaceEscapingRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "worktrees")
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ZEE")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	manager := New(root, types.HooksConfig{
		BeforeRun: `printf ran > hook.txt`,
		TimeoutMS: 5000,
	})
	if err := manager.ValidateWorkspacePath(path); err == nil {
		t.Fatal("expected symlink workspace escaping root to be rejected")
	}
	if err := manager.BeforeRun(context.Background(), path); err == nil {
		t.Fatal("expected before_run on symlink workspace escaping root to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "hook.txt")); !os.IsNotExist(err) {
		t.Fatalf("before_run should not execute in outside target, err=%v", err)
	}
}

func TestRemoveSymlinkWorkspaceRemovesOnlyLink(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "worktrees")
	outside := filepath.Join(tmp, "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "ZEE")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	manager := New(root, types.HooksConfig{
		BeforeRemove: `printf ran > hook.txt`,
		TimeoutMS:    5000,
	})
	if err := manager.Remove(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("symlink should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "keep.txt")); err != nil {
		t.Fatalf("outside target should remain intact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "hook.txt")); !os.IsNotExist(err) {
		t.Fatalf("before_remove should not execute in outside target, err=%v", err)
	}
}
