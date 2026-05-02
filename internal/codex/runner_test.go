package codex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeefan1555/symphony-go/internal/types"
)

func TestRunnerSendsChinesePromptAndGitWritableRoots(t *testing.T) {
	workspacePath := t.TempDir()
	if err := exec.Command("git", "-C", workspacePath, "init").Run(); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "fake-codex")
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	script := `#!/bin/sh
trace="$TRACE_FILE"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":1,"result":{}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-中文"}}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-中文"}}}'
printf '%s\n' '{"method":"turn/completed"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := New(types.CodexConfig{
		Command:           fake,
		ApprovalPolicy:    "never",
		ThreadSandbox:     "workspace-write",
		TurnSandboxPolicy: map[string]any{"type": "workspaceWrite"},
		TurnTimeoutMS:     5000,
		ReadTimeoutMS:     5000,
	})
	t.Setenv("TRACE_FILE", trace)
	result, err := runner.Run(context.Background(), workspacePath, "zeefan 中文 smoke test", types.Issue{Identifier: "ZEE-8", Title: "中文标题"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "thread-中文-turn-中文" {
		t.Fatalf("session id = %q", result.SessionID)
	}
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"zeefan 中文 smoke test", "中文标题", workspacePath, ".git"} {
		if !strings.Contains(text, want) {
			t.Fatalf("trace missing %q:\n%s", want, text)
		}
	}
}

func TestMergingTurnSandboxIncludesMainCheckoutRoot(t *testing.T) {
	repoRoot := t.TempDir()
	git(t, repoRoot, "init")
	git(t, repoRoot, "config", "user.email", "test@example.com")
	git(t, repoRoot, "config", "user.name", "Test User")
	git(t, repoRoot, "commit", "--allow-empty", "-m", "initial")
	canonicalRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(t.TempDir(), "ZEE-TEST")
	git(t, repoRoot, "worktree", "add", "-b", "symphony-go/ZEE-TEST", worktreePath)

	runner := New(types.CodexConfig{
		TurnSandboxPolicy: map[string]any{"type": "workspaceWrite"},
	})
	implementationRoots := toStringSlice(runner.turnSandboxPolicy(worktreePath, types.Issue{State: "In Progress"})["writableRoots"])
	if containsString(implementationRoots, canonicalRepoRoot) {
		t.Fatalf("implementation roots unexpectedly include main checkout root %q: %#v", canonicalRepoRoot, implementationRoots)
	}

	mergingRoots := toStringSlice(runner.turnSandboxPolicy(worktreePath, types.Issue{State: "Merging"})["writableRoots"])
	for _, want := range []string{worktreePath, filepath.Join(canonicalRepoRoot, ".git"), canonicalRepoRoot} {
		if !containsString(mergingRoots, want) {
			t.Fatalf("merging roots missing %q: %#v", want, mergingRoots)
		}
	}
}

func TestRunnerKeepsOneThreadForContinuationTurns(t *testing.T) {
	workspacePath := t.TempDir()
	fake := filepath.Join(t.TempDir(), "fake-codex")
	trace := filepath.Join(t.TempDir(), "trace.jsonl")
	script := `#!/bin/sh
trace="$TRACE_FILE"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":1,"result":{}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-1"}}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-1"}}}'
printf '%s\n' '{"method":"turn/completed"}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":4,"result":{"turn":{"id":"turn-2"}}}'
printf '%s\n' '{"method":"turn/completed"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_FILE", trace)
	runner := New(types.CodexConfig{
		Command:        fake,
		ApprovalPolicy: "never",
		ThreadSandbox:  "workspace-write",
		TurnTimeoutMS:  5000,
		ReadTimeoutMS:  5000,
	})
	result, err := runner.RunSession(context.Background(), SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         types.Issue{Identifier: "ZEE-1", Title: "continue"},
		Prompts: []TurnPrompt{
			{Text: "first prompt"},
			{Text: "continue prompt", Continuation: true},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != "thread-1" || len(result.Turns) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.SessionID != "thread-1-turn-2" {
		t.Fatalf("session id = %q, want final turn session id", result.SessionID)
	}
	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Count(text, `"method":"initialize"`) != 1 {
		t.Fatalf("initialize count mismatch:\n%s", text)
	}
	if strings.Count(text, `"method":"thread/start"`) != 1 {
		t.Fatalf("thread/start count mismatch:\n%s", text)
	}
	if strings.Count(text, `"method":"turn/start"`) != 2 {
		t.Fatalf("turn/start count mismatch:\n%s", text)
	}
	for _, want := range []string{"first prompt", "continue prompt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("trace missing %q:\n%s", want, text)
		}
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
