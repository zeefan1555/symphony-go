package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	runtimeconfig "symphony-go/internal/runtime/config"
	issuemodel "symphony-go/internal/service/issue"
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
	runner := New(runtimeconfig.CodexConfig{
		Command:           fake,
		ApprovalPolicy:    "never",
		ThreadSandbox:     "workspace-write",
		TurnSandboxPolicy: map[string]any{"type": "workspaceWrite"},
		TurnTimeoutMS:     5000,
		ReadTimeoutMS:     5000,
	})
	t.Setenv("TRACE_FILE", trace)
	result, err := runner.Run(context.Background(), workspacePath, "zeefan 中文 smoke test", issuemodel.Issue{Identifier: "ZEE-8", Title: "中文标题"}, nil)
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

func TestMergingTurnSandboxDoesNotIncludeMainCheckoutRoot(t *testing.T) {
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

	runner := New(runtimeconfig.CodexConfig{
		TurnSandboxPolicy: map[string]any{"type": "workspaceWrite"},
	})
	implementationRoots := toStringSlice(runner.turnSandboxPolicy(worktreePath, issuemodel.Issue{State: "In Progress"})["writableRoots"])
	if containsString(implementationRoots, canonicalRepoRoot) {
		t.Fatalf("implementation roots unexpectedly include main checkout root %q: %#v", canonicalRepoRoot, implementationRoots)
	}

	mergingRoots := toStringSlice(runner.turnSandboxPolicy(worktreePath, issuemodel.Issue{State: "Merging"})["writableRoots"])
	for _, want := range []string{worktreePath, filepath.Join(canonicalRepoRoot, ".git")} {
		if !containsString(mergingRoots, want) {
			t.Fatalf("merging roots missing %q: %#v", want, mergingRoots)
		}
	}
	if containsString(mergingRoots, canonicalRepoRoot) {
		t.Fatalf("merging roots unexpectedly include main checkout root %q: %#v", canonicalRepoRoot, mergingRoots)
	}
}

func TestContinuationPromptIssueControlsTurnSandbox(t *testing.T) {
	repoRoot := t.TempDir()
	git(t, repoRoot, "init")
	git(t, repoRoot, "config", "user.email", "test@example.com")
	git(t, repoRoot, "config", "user.name", "Test User")
	git(t, repoRoot, "commit", "--allow-empty", "-m", "initial")
	canonicalRepoRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	worktreePath := filepath.Join(t.TempDir(), "ZEE-CONTINUE")
	git(t, repoRoot, "worktree", "add", "-b", "symphony-go/ZEE-CONTINUE", worktreePath)

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
	runner := New(runtimeconfig.CodexConfig{
		Command:           fake,
		ApprovalPolicy:    "never",
		ThreadSandbox:     "workspace-write",
		TurnSandboxPolicy: map[string]any{"type": "workspaceWrite"},
		TurnTimeoutMS:     5000,
		ReadTimeoutMS:     5000,
	})
	mergingIssue := issuemodel.Issue{Identifier: "ZEE-CONTINUE", Title: "merge continuation", State: "Merging"}
	_, err = runner.RunSession(context.Background(), SessionRequest{
		WorkspacePath: worktreePath,
		Issue:         issuemodel.Issue{Identifier: "ZEE-CONTINUE", Title: "review continuation", State: "AI Review"},
		Prompts: []TurnPrompt{
			{Text: "review prompt"},
			{Text: "merge prompt", Continuation: true, Issue: &mergingIssue},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	turnRoots := turnWritableRoots(t, trace)
	if got, want := len(turnRoots), 2; got != want {
		t.Fatalf("turn/start count = %d, want %d", got, want)
	}
	if containsString(turnRoots[0], canonicalRepoRoot) {
		t.Fatalf("review turn roots unexpectedly include main checkout root %q: %#v", canonicalRepoRoot, turnRoots[0])
	}
	if containsString(turnRoots[1], canonicalRepoRoot) {
		t.Fatalf("merge continuation roots unexpectedly include main checkout root %q: %#v", canonicalRepoRoot, turnRoots[1])
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
	runner := New(runtimeconfig.CodexConfig{
		Command:        fake,
		ApprovalPolicy: "never",
		ThreadSandbox:  "workspace-write",
		TurnTimeoutMS:  5000,
		ReadTimeoutMS:  5000,
	})
	result, err := runner.RunSession(context.Background(), SessionRequest{
		WorkspacePath: workspacePath,
		Issue:         issuemodel.Issue{Identifier: "ZEE-1", Title: "continue"},
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

func TestRunnerAdvertisesAndHandlesLinearGraphQLDynamicTool(t *testing.T) {
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
printf '%s\n' '{"id":2,"result":{"thread":{"id":"thread-tools"}}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"id":3,"result":{"turn":{"id":"turn-tools"}}}'
printf '%s\n' '{"id":101,"method":"item/tool/call","params":{"tool":"linear_graphql","callId":"call-tools","threadId":"thread-tools","turnId":"turn-tools","arguments":{"query":"query Viewer { viewer { id } }","variables":{"includeTeams":false}}}}'
IFS= read -r line
printf '%s\n' "$line" >> "$trace"
printf '%s\n' '{"method":"turn/completed"}'
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRACE_FILE", trace)
	client := &fakeGraphQLRawClient{
		response: map[string]any{"data": map[string]any{"viewer": map[string]any{"id": "usr_tools"}}},
	}
	runner := New(runtimeconfig.CodexConfig{
		Command:        fake,
		ApprovalPolicy: "never",
		ThreadSandbox:  "workspace-write",
		TurnTimeoutMS:  5000,
		ReadTimeoutMS:  5000,
	}, WithDynamicToolExecutor(NewDynamicToolExecutor(client)))

	_, err := runner.Run(context.Background(), workspacePath, "use the tool", issuemodel.Issue{Identifier: "ZEE-TOOLS", Title: "tools"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.query != "query Viewer { viewer { id } }" {
		t.Fatalf("query = %q", client.query)
	}
	if client.variables["includeTeams"] != false {
		t.Fatalf("variables = %#v", client.variables)
	}

	raw, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var sawToolSpec bool
	var sawToolResponse bool
	for _, line := range lines {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid trace line %q: %v", line, err)
		}
		if payload["method"] == "thread/start" {
			params, _ := payload["params"].(map[string]any)
			tools, _ := params["dynamicTools"].([]any)
			if len(tools) == 1 {
				tool, _ := tools[0].(map[string]any)
				sawToolSpec = tool["name"] == "linear_graphql"
			}
		}
		if id, ok := numericID(payload["id"]); ok && id == 101 {
			result, _ := payload["result"].(map[string]any)
			if result["success"] == true && strings.Contains(fmt.Sprint(result["output"]), "usr_tools") {
				sawToolResponse = true
			}
		}
	}
	if !sawToolSpec {
		t.Fatalf("thread/start did not advertise linear_graphql:\n%s", raw)
	}
	if !sawToolResponse {
		t.Fatalf("tool response missing successful Linear payload:\n%s", raw)
	}
}

func TestRunnerFailsUserInputAndApprovalRequestsWithoutWaiting(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params string
	}{
		{name: "turn input required", method: "turn/input_required", params: `"params":{"reason":"blocked"}`},
		{name: "turn approval required", method: "turn/approval_required", params: `"params":{"reason":"approval"}`},
		{name: "tool request user input", method: "item/tool/requestUserInput", params: `"params":{"questions":[{"id":"q1"}]}`},
		{name: "command approval", method: "item/commandExecution/requestApproval", params: `"params":{"command":"gh pr view"}`},
		{name: "file approval", method: "item/fileChange/requestApproval", params: `"params":{"fileChangeCount":2}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workspacePath := t.TempDir()
			fake := filepath.Join(t.TempDir(), "fake-codex")
			script := fmt.Sprintf(`#!/bin/sh
IFS= read -r line
printf '%%s\n' '{"id":1,"result":{}}'
IFS= read -r line
IFS= read -r line
printf '%%s\n' '{"id":2,"result":{"thread":{"id":"thread-input"}}}'
IFS= read -r line
printf '%%s\n' '{"id":3,"result":{"turn":{"id":"turn-input"}}}'
printf '%%s\n' '{"id":99,"method":"%s",%s}'
sleep 5
`, tc.method, tc.params)
			if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			runner := New(runtimeconfig.CodexConfig{
				Command:        fake,
				ApprovalPolicy: "never",
				ThreadSandbox:  "workspace-write",
				TurnTimeoutMS:  10000,
				ReadTimeoutMS:  5000,
			})
			start := time.Now()
			_, err := runner.Run(context.Background(), workspacePath, "requires input", issuemodel.Issue{Identifier: "ZEE-INPUT", Title: "input"}, nil)
			if err == nil {
				t.Fatal("expected input/approval request to fail")
			}
			if elapsed := time.Since(start); elapsed > 2*time.Second {
				t.Fatalf("runner waited too long: %s", elapsed)
			}
			if !strings.Contains(err.Error(), tc.method) {
				t.Fatalf("error = %v, want method %q", err, tc.method)
			}
		})
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

func turnWritableRoots(t *testing.T, tracePath string) [][]string {
	t.Helper()
	raw, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	var roots [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid trace line %q: %v", line, err)
		}
		if payload["method"] != "turn/start" {
			continue
		}
		params, _ := payload["params"].(map[string]any)
		sandboxPolicy, _ := params["sandboxPolicy"].(map[string]any)
		roots = append(roots, toStringSlice(sandboxPolicy["writableRoots"]))
	}
	return roots
}
