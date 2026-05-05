# Go Symphony TUI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 `/Users/bytedance/symphony/go` 增加类似 Elixir `StatusDashboard` 的终端 TUI，让常驻 `make run` 能可视化展示 workflow 状态流转、运行中 agent、retry/backoff、token/runtime 和下一次轮询时间。

**Architecture:** 先实现一个 Go 内存 runtime snapshot，再用终端 renderer 订阅 snapshot 周期刷新。TUI 只读取 orchestrator state，不参与调度决策；这对齐 `SPEC.md` 中 “Status Surface 是 observability/control surface，不是 correctness dependency” 的设计。

**Tech Stack:** Go 标准库优先；ANSI escape sequence 渲染终端；不引入 Bubble Tea/TView 等外部 TUI 框架；继续使用当前 `make build` 规避 macOS `go run` 临时二进制 `missing LC_UUID` 问题。

---

## 参考依据

- Elixir TUI 样板：
  - `elixir/lib/symphony_elixir/status_dashboard.ex`
  - `elixir/lib/symphony_elixir/orchestrator.ex`
  - `elixir/lib/symphony_elixir_web/presenter.ex`
- `SPEC.md` 约束：
  - `13.3 Runtime Snapshot / Monitoring Interface`
  - `13.4 OPTIONAL Human-Readable Status Surface`
  - `13.5 Session Metrics and Token Accounting`
  - `13.6 Humanized Agent Event Summaries`
- 当前 Go v1 现状：
  - `go/internal/orchestrator/orchestrator.go` 现在是同步顺序 poll/run，只有 JSONL 日志，没有内存 snapshot。
  - `go/internal/codex/runner.go` 已能回调 Codex events。
  - `go/Makefile` 已通过 `-linkmode=external` 解决 `go run` 的 dyld 问题。

## 文件结构

- 新增 `go/internal/observability/snapshot.go`
  - 定义 snapshot 数据结构：`Snapshot`、`RunningEntry`、`RetryEntry`、`CodexTotals`、`PollingStatus`。
  - 提供 runtime 秒数计算、running/retry count 和空集合初始化。
- 新增 `go/internal/observability/token.go`
  - 从 Codex event payload 中提取绝对 token totals。
  - 支持 `thread/tokenUsage/updated`、`total_token_usage`、`input_tokens/output_tokens/total_tokens` 等常见形态。
- 新增 `go/internal/tui/dashboard.go`
  - 将 snapshot 渲染为 Elixir 风格的终端面板。
  - 输出 `SYMPHONY STATUS`、Agents、Throughput、Runtime、Tokens、Rate Limits、Project、Next refresh、Running、Backoff queue。
- 修改 `go/internal/orchestrator/orchestrator.go`
  - 给 `Orchestrator` 增加 mutex 保护的 runtime state。
  - poll 开始/结束、issue dispatch、Codex event、turn completion、error/backoff 都更新 snapshot。
  - 暴露 `Snapshot() observability.Snapshot`。
- 修改 `go/cmd/symphony-go/main.go`
  - 新增参数：`--tui` 默认 true，`--no-tui` 禁用。
  - 常驻 `make run` 默认展示 TUI。
  - `--once` 默认不启用持续 TUI，只打印最终 snapshot，避免调试输出刷屏。
- 修改 `go/Makefile`
  - `make run` 保持常驻 TUI。
  - `make run-once` 走 `--once --no-tui`。
- 修改 `go/README.md`
  - 把 TUI 作为主使用路径说明。
  - 记录 `--no-tui`、`make run-once` 和日志位置。

## Task 1: 建立 Runtime Snapshot 数据模型

**Files:**
- Create: `go/internal/observability/snapshot.go`
- Test: `go/internal/observability/snapshot_test.go`

- [ ] **Step 1: 写 snapshot 单元测试**

创建 `go/internal/observability/snapshot_test.go`：

```go
package observability

import (
	"testing"
	"time"
)

func TestSnapshotInitializesEmptySlices(t *testing.T) {
	snapshot := NewSnapshot()
	if snapshot.Running == nil {
		t.Fatal("running must be an empty slice, not nil")
	}
	if snapshot.Retrying == nil {
		t.Fatal("retrying must be an empty slice, not nil")
	}
	if snapshot.Counts.Running != 0 || snapshot.Counts.Retrying != 0 {
		t.Fatalf("counts = %#v", snapshot.Counts)
	}
}

func TestSnapshotIncludesLiveRuntimeSeconds(t *testing.T) {
	startedAt := time.Now().Add(-2 * time.Second)
	snapshot := NewSnapshot()
	snapshot.CodexTotals.SecondsRunning = 10
	snapshot.Running = []RunningEntry{{StartedAt: startedAt}}

	total := snapshot.TotalRuntimeSeconds(time.Now())
	if total < 11.5 {
		t.Fatalf("runtime seconds = %f, want active runtime included", total)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/observability
```

Expected: FAIL，提示 `package .../internal/observability` 或 `NewSnapshot` 未定义。

- [ ] **Step 3: 实现 snapshot 类型**

创建 `go/internal/observability/snapshot.go`：

```go
package observability

import "time"

type Snapshot struct {
	GeneratedAt  time.Time      `json:"generated_at"`
	Counts       Counts         `json:"counts"`
	Running      []RunningEntry `json:"running"`
	Retrying     []RetryEntry   `json:"retrying"`
	CodexTotals  CodexTotals    `json:"codex_totals"`
	RateLimits   any            `json:"rate_limits"`
	Polling      PollingStatus  `json:"polling"`
	LastError    string         `json:"last_error,omitempty"`
}

type Counts struct {
	Running  int `json:"running"`
	Retrying int `json:"retrying"`
}

type RunningEntry struct {
	IssueID         string    `json:"issue_id"`
	IssueIdentifier string    `json:"issue_identifier"`
	State           string    `json:"state"`
	WorkspacePath   string    `json:"workspace_path,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	TurnCount       int       `json:"turn_count"`
	LastEvent       string    `json:"last_event,omitempty"`
	LastMessage     string    `json:"last_message,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	LastEventAt     time.Time `json:"last_event_at,omitempty"`
	Tokens          TokenUsage `json:"tokens"`
	RuntimeSeconds  float64   `json:"runtime_seconds"`
}

type RetryEntry struct {
	IssueID          string    `json:"issue_id"`
	IssueIdentifier  string    `json:"issue_identifier"`
	Attempt          int       `json:"attempt"`
	DueAt            time.Time `json:"due_at"`
	Error            string    `json:"error,omitempty"`
	WorkspacePath    string    `json:"workspace_path,omitempty"`
}

type CodexTotals struct {
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	TotalTokens    int     `json:"total_tokens"`
	SecondsRunning float64 `json:"seconds_running"`
}

type TokenUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type PollingStatus struct {
	Checking      bool      `json:"checking"`
	NextPollAt    time.Time `json:"next_poll_at,omitempty"`
	NextPollInMS  int64     `json:"next_poll_in_ms"`
	IntervalMS    int       `json:"interval_ms"`
	LastPollAt     time.Time `json:"last_poll_at,omitempty"`
}

func NewSnapshot() Snapshot {
	return Snapshot{
		GeneratedAt: time.Now(),
		Running:     []RunningEntry{},
		Retrying:    []RetryEntry{},
	}
}

func (s Snapshot) TotalRuntimeSeconds(now time.Time) float64 {
	total := s.CodexTotals.SecondsRunning
	for _, entry := range s.Running {
		if !entry.StartedAt.IsZero() {
			total += now.Sub(entry.StartedAt).Seconds()
		}
	}
	return total
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/observability
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add go/internal/observability/snapshot.go go/internal/observability/snapshot_test.go
git commit -m "feat(go): add observability snapshot model"
```

## Task 2: 实现 Codex Token 和事件摘要提取

**Files:**
- Create: `go/internal/observability/token.go`
- Test: `go/internal/observability/token_test.go`

- [ ] **Step 1: 写 token 提取测试**

创建 `go/internal/observability/token_test.go`：

```go
package observability

import "testing"

func TestExtractTokenUsageFromThreadTokenUsageUpdated(t *testing.T) {
	payload := map[string]any{
		"method": "thread/tokenUsage/updated",
		"params": map[string]any{
			"input_tokens": 120.0,
			"output_tokens": 34.0,
			"total_tokens": 154.0,
		},
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 34 || usage.TotalTokens != 154 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestExtractTokenUsageFromTotalTokenUsageWrapper(t *testing.T) {
	payload := map[string]any{
		"method": "codex/event/token_count",
		"params": map[string]any{
			"total_token_usage": map[string]any{
				"input_tokens": 10.0,
				"output_tokens": 5.0,
				"total_tokens": 15.0,
			},
		},
	}
	usage, ok := ExtractTokenUsage(payload)
	if !ok {
		t.Fatal("expected token usage")
	}
	if usage.TotalTokens != 15 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestHumanizeCodexEvent(t *testing.T) {
	message := HumanizeCodexEvent(map[string]any{
		"method": "turn/completed",
	})
	if message != "turn completed" {
		t.Fatalf("message = %q", message)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/observability
```

Expected: FAIL，提示 `ExtractTokenUsage` 未定义。

- [ ] **Step 3: 实现 token 提取**

创建 `go/internal/observability/token.go`：

```go
package observability

import "strings"

func ExtractTokenUsage(payload map[string]any) (TokenUsage, bool) {
	if payload == nil {
		return TokenUsage{}, false
	}
	if params, ok := payload["params"].(map[string]any); ok {
		if usage, ok := usageFromMap(params); ok {
			return usage, true
		}
		if nested, ok := params["total_token_usage"].(map[string]any); ok {
			return usageFromMap(nested)
		}
	}
	return usageFromMap(payload)
}

func usageFromMap(value map[string]any) (TokenUsage, bool) {
	input, inputOK := intField(value, "input_tokens", "inputTokens", "input")
	output, outputOK := intField(value, "output_tokens", "outputTokens", "output")
	total, totalOK := intField(value, "total_tokens", "totalTokens", "total")
	if !totalOK && (inputOK || outputOK) {
		total = input + output
		totalOK = true
	}
	if !inputOK && !outputOK && !totalOK {
		return TokenUsage{}, false
	}
	return TokenUsage{InputTokens: input, OutputTokens: output, TotalTokens: total}, true
}

func intField(value map[string]any, names ...string) (int, bool) {
	for _, name := range names {
		switch raw := value[name].(type) {
		case int:
			return raw, true
		case int64:
			return int(raw), true
		case float64:
			return int(raw), true
		}
	}
	return 0, false
}

func HumanizeCodexEvent(payload map[string]any) string {
	method, _ := payload["method"].(string)
	switch method {
	case "turn/completed":
		return "turn completed"
	case "turn/failed":
		return "turn failed"
	case "turn/cancelled":
		return "turn cancelled"
	case "codex/event/task_started":
		return "task started"
	case "codex/event/token_count", "thread/tokenUsage/updated":
		return "token usage updated"
	default:
		if method == "" {
			return "event"
		}
		return strings.ReplaceAll(method, "_", " ")
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/observability
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add go/internal/observability/token.go go/internal/observability/token_test.go
git commit -m "feat(go): extract codex token usage for observability"
```

## Task 3: 让 Orchestrator 维护 Snapshot

**Files:**
- Modify: `go/internal/orchestrator/orchestrator.go`
- Test: `go/internal/orchestrator/orchestrator_test.go`

- [ ] **Step 1: 写 snapshot 行为测试**

创建 `go/internal/orchestrator/orchestrator_test.go`：

```go
package orchestrator

import (
	"context"
	"testing"

	"symphony-go/internal/codex"
	"symphony-go/internal/types"
)

type fakeTracker struct {
	issues []types.Issue
}

func (f *fakeTracker) FetchActiveIssues(context.Context, []string) ([]types.Issue, error) { return f.issues, nil }
func (f *fakeTracker) FetchIssue(context.Context, string) (types.Issue, error) { return f.issues[0], nil }
func (f *fakeTracker) UpdateIssueState(context.Context, string, string) error { return nil }
func (f *fakeTracker) UpsertWorkpad(context.Context, string, string) error { return nil }

type fakeRunner struct{}

func (f fakeRunner) Run(ctx context.Context, workspacePath string, prompt string, issue types.Issue, onEvent func(codex.Event)) (codex.Result, error) {
	onEvent(codex.Event{Name: "thread/tokenUsage/updated", Payload: map[string]any{
		"method": "thread/tokenUsage/updated",
		"params": map[string]any{"input_tokens": 2.0, "output_tokens": 3.0, "total_tokens": 5.0},
	}})
	return codex.Result{SessionID: "thread-1-turn-1", ThreadID: "thread-1", TurnID: "turn-1"}, nil
}

func TestSnapshotStartsWithEmptyCollections(t *testing.T) {
	o := New(Options{Workflow: &types.Workflow{Config: types.Config{}}})
	snapshot := o.Snapshot()
	if snapshot.Running == nil || snapshot.Retrying == nil {
		t.Fatalf("snapshot collections must be non-nil: %#v", snapshot)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator
```

Expected: FAIL，提示 `Snapshot` 未定义。

- [ ] **Step 3: 修改 Orchestrator state**

在 `go/internal/orchestrator/orchestrator.go` 中：

1. 新增 imports：

```go
import (
	"sync"
	"symphony-go/internal/observability"
)
```

2. 修改 `Orchestrator`：

```go
type Orchestrator struct {
	opts Options
	mu sync.RWMutex
	snapshot observability.Snapshot
	endedRuntimeSeconds float64
}
```

3. 修改 `New`：

```go
func New(opts Options) *Orchestrator {
	snapshot := observability.NewSnapshot()
	snapshot.Polling.IntervalMS = opts.Workflow.Config.Polling.IntervalMS
	return &Orchestrator{opts: opts, snapshot: snapshot}
}
```

4. 增加 `Snapshot` 方法：

```go
func (o *Orchestrator) Snapshot() observability.Snapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	snapshot := o.snapshot
	snapshot.GeneratedAt = time.Now()
	snapshot.Counts.Running = len(snapshot.Running)
	snapshot.Counts.Retrying = len(snapshot.Retrying)
	snapshot.CodexTotals.SecondsRunning = snapshot.TotalRuntimeSeconds(time.Now())
	return snapshot
}
```

- [ ] **Step 4: 在 poll/runAgent 中更新 snapshot**

继续修改 `go/internal/orchestrator/orchestrator.go`，添加 helper：

```go
func (o *Orchestrator) markPolling(checking bool, nextPollAt time.Time) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.snapshot.Polling.Checking = checking
	o.snapshot.Polling.LastPollAt = time.Now()
	o.snapshot.Polling.NextPollAt = nextPollAt
	if !nextPollAt.IsZero() {
		o.snapshot.Polling.NextPollInMS = time.Until(nextPollAt).Milliseconds()
	}
}

func (o *Orchestrator) setRunning(entry observability.RunningEntry) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i, existing := range o.snapshot.Running {
		if existing.IssueID == entry.IssueID {
			o.snapshot.Running[i] = entry
			return
		}
	}
	o.snapshot.Running = append(o.snapshot.Running, entry)
}

func (o *Orchestrator) removeRunning(issueID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	filtered := o.snapshot.Running[:0]
	for _, entry := range o.snapshot.Running {
		if entry.IssueID != issueID {
			filtered = append(filtered, entry)
		}
	}
	o.snapshot.Running = filtered
}
```

在 `Run` 的每轮 poll 前后调用：

```go
o.markPolling(true, time.Time{})
err := o.poll(ctx)
o.markPolling(false, time.Now().Add(interval))
```

在 `runAgent` 启动前设置 running entry，在 Codex event 回调里更新 `LastEvent`、`LastMessage`、`LastEventAt`、`Tokens`，turn 结束后 remove running。

- [ ] **Step 5: 运行测试确认通过**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/orchestrator ./internal/observability
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add go/internal/orchestrator/orchestrator.go go/internal/orchestrator/orchestrator_test.go
git commit -m "feat(go): track orchestrator runtime snapshot"
```

## Task 4: 实现 Elixir 风格终端 TUI renderer

**Files:**
- Create: `go/internal/tui/dashboard.go`
- Test: `go/internal/tui/dashboard_test.go`

- [ ] **Step 1: 写 renderer 测试**

创建 `go/internal/tui/dashboard_test.go`：

```go
package tui

import (
	"strings"
	"testing"
	"time"

	"symphony-go/internal/observability"
)

func TestRenderSnapshotShowsCoreSections(t *testing.T) {
	snapshot := observability.NewSnapshot()
	snapshot.Running = []observability.RunningEntry{{
		IssueID: "issue-1",
		IssueIdentifier: "ZEE-8",
		State: "In Progress",
		SessionID: "thread-abc-turn-def",
		TurnCount: 2,
		LastEvent: "turn/completed",
		LastMessage: "turn completed",
		StartedAt: time.Now().Add(-3 * time.Second),
		Tokens: observability.TokenUsage{InputTokens: 10, OutputTokens: 4, TotalTokens: 14},
	}}
	snapshot.Polling.NextPollInMS = 5000
	output := Render(snapshot, Options{MaxAgents: 10, ProjectSlug: "symphony-test-c2a66ab0f2e7"})

	for _, want := range []string{"SYMPHONY STATUS", "Agents:", "Running", "Backoff queue", "ZEE-8", "In Progress", "Next refresh: 5s"} {
		if !strings.Contains(output, want) {
			t.Fatalf("render output missing %q:\n%s", want, output)
		}
	}
}

func TestRenderSnapshotShowsNoActiveAgents(t *testing.T) {
	output := Render(observability.NewSnapshot(), Options{MaxAgents: 10})
	if !strings.Contains(output, "No active agents") {
		t.Fatalf("output = %s", output)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/tui
```

Expected: FAIL，提示 package 或 `Render` 未定义。

- [ ] **Step 3: 实现 TUI renderer**

创建 `go/internal/tui/dashboard.go`：

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	"symphony-go/internal/observability"
)

type Options struct {
	MaxAgents   int
	ProjectSlug string
}

func Render(snapshot observability.Snapshot, opts Options) string {
	runningCount := len(snapshot.Running)
	total := snapshot.CodexTotals
	runtimeSeconds := snapshot.TotalRuntimeSeconds(time.Now())
	lines := []string{
		"╭─ SYMPHONY STATUS",
		fmt.Sprintf("│ Agents: %d/%d", runningCount, opts.MaxAgents),
		"│ Throughput: 0 tps",
		fmt.Sprintf("│ Runtime: %s", formatRuntime(runtimeSeconds)),
		fmt.Sprintf("│ Tokens: in %d | out %d | total %d", total.InputTokens, total.OutputTokens, total.TotalTokens),
		"│ Rate Limits: unavailable",
		fmt.Sprintf("│ Project: %s", projectURL(opts.ProjectSlug)),
		fmt.Sprintf("│ Next refresh: %s", nextRefresh(snapshot.Polling)),
		"├─ Running",
		"│",
		"│   ID       STAGE          SESSION        AGE / TURN   TOKENS     EVENT",
		"│   ─────────────────────────────────────────────────────────────────────────",
	}
	if len(snapshot.Running) == 0 {
		lines = append(lines, "│  No active agents", "│")
	} else {
		for _, entry := range snapshot.Running {
			lines = append(lines, formatRunning(entry))
		}
		lines = append(lines, "│")
	}
	lines = append(lines, "├─ Backoff queue", "│")
	if len(snapshot.Retrying) == 0 {
		lines = append(lines, "│  No queued retries")
	} else {
		for _, entry := range snapshot.Retrying {
			lines = append(lines, formatRetry(entry))
		}
	}
	lines = append(lines, "╰─")
	return strings.Join(lines, "\n")
}

func ClearAndRender(frame string) string {
	return "\033[H\033[2J" + frame + "\n"
}

func formatRunning(entry observability.RunningEntry) string {
	return fmt.Sprintf(
		"│ ● %-8s %-14s %-14s %-12s %-10d %s",
		truncate(entry.IssueIdentifier, 8),
		truncate(entry.State, 14),
		truncate(entry.SessionID, 14),
		fmt.Sprintf("%s / %d", formatRuntime(time.Since(entry.StartedAt).Seconds()), entry.TurnCount),
		entry.Tokens.TotalTokens,
		truncate(firstNonEmpty(entry.LastMessage, entry.LastEvent, "none"), 40),
	)
}

func formatRetry(entry observability.RetryEntry) string {
	return fmt.Sprintf("│  ↻ %-8s attempt=%d due=%s error=%s", entry.IssueIdentifier, entry.Attempt, entry.DueAt.Format(time.RFC3339), entry.Error)
}

func nextRefresh(status observability.PollingStatus) string {
	if status.Checking {
		return "checking now..."
	}
	if status.NextPollInMS <= 0 {
		return "n/a"
	}
	return fmt.Sprintf("%ds", (status.NextPollInMS+999)/1000)
}

func projectURL(slug string) string {
	if slug == "" {
		return "n/a"
	}
	return "https://linear.app/project/" + slug + "/issues"
}

func formatRuntime(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	total := int(seconds)
	minutes := total / 60
	remaining := total % 60
	return fmt.Sprintf("%dm %ds", minutes, remaining)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncate(value string, max int) string {
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max])
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./internal/tui
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add go/internal/tui/dashboard.go go/internal/tui/dashboard_test.go
git commit -m "feat(go): render terminal status dashboard"
```

## Task 5: 将 TUI 接入 CLI 常驻运行

**Files:**
- Modify: `go/cmd/symphony-go/main.go`
- Modify: `go/Makefile`
- Test: `go/cmd/symphony-go/main_test.go`

- [ ] **Step 1: 写 CLI flag 测试**

创建 `go/cmd/symphony-go/main_test.go`：

```go
package main

import "testing"

func TestDefaultRunOptionsEnableTUI(t *testing.T) {
	opts := defaultRunOptions()
	if !opts.TUI {
		t.Fatal("TUI should be enabled by default for continuous run")
	}
}

func TestOnceDisablesTUIWhenNotExplicit(t *testing.T) {
	opts := defaultRunOptions()
	opts.Once = true
	opts.ApplyDefaults()
	if opts.TUI {
		t.Fatal("once mode should disable TUI by default")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./cmd/symphony-go
```

Expected: FAIL，提示 `defaultRunOptions` 未定义。

- [ ] **Step 3: 增加 run options**

在 `go/cmd/symphony-go/main.go` 中增加：

```go
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
		WorkflowPath: "../elixir/WORKFLOW.md",
		MergeTarget:  "feat_zff",
		TUI:          true,
	}
}

func (o *runOptions) ApplyDefaults() {
	if o.Once && !o.tuiExplicit {
		o.TUI = false
	}
}
```

将现有 flag 解析改为使用 `runOptions`，新增：

```go
runFlags.BoolVar(&opts.TUI, "tui", true, "render terminal TUI")
noTUI := runFlags.Bool("no-tui", false, "disable terminal TUI")
```

解析后：

```go
if *noTUI {
	opts.TUI = false
	opts.tuiExplicit = true
}
opts.ApplyDefaults()
```

- [ ] **Step 4: 接入 dashboard renderer**

在 `main.go` 中 import：

```go
"time"
"symphony-go/internal/tui"
```

创建 orchestrator 后，如果 `opts.TUI` 为 true，启动 goroutine：

```go
if opts.TUI {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				frame := tui.Render(service.Snapshot(), tui.Options{
					MaxAgents: loaded.Config.Agent.MaxConcurrentAgents,
					ProjectSlug: loaded.Config.Tracker.ProjectSlug,
				})
				fmt.Print(tui.ClearAndRender(frame))
			}
		}
	}()
}
```

注意：`Orchestrator` 需要已有 `Snapshot()` 方法；如果 Task 3 未完成，不得执行本任务。

- [ ] **Step 5: 修改 Makefile**

修改 `go/Makefile`：

```make
run: build
	$(BINARY) run --workflow $(WORKFLOW) --tui --merge-target $(MERGE_TARGET)

run-once: build
	$(BINARY) run --workflow $(WORKFLOW) --once --no-tui $(if $(ISSUE),--issue $(ISSUE),) --merge-target $(MERGE_TARGET)
```

- [ ] **Step 6: 运行测试确认通过**

Run:

```bash
cd /Users/bytedance/symphony/go
go test ./cmd/symphony-go ./internal/orchestrator ./internal/tui
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add go/cmd/symphony-go/main.go go/cmd/symphony-go/main_test.go go/Makefile
git commit -m "feat(go): wire terminal dashboard into run command"
```

## Task 6: 更新文档和手动验证

**Files:**
- Modify: `go/README.md`
- Modify: `go/docs/plan/go-symphony-tui.md`

- [ ] **Step 1: 更新 README 使用说明**

在 `go/README.md` 中将主路径写成：

```md
## Start the TUI daemon

```bash
cd /Users/bytedance/symphony/go
make run
```

`make run` starts the continuous Linear monitor and renders the terminal status dashboard.

To run without the dashboard:

```bash
bin/symphony-go run --workflow ./WORKFLOW.md --no-tui
```

For one-shot debugging:

```bash
make run-once ISSUE=ZEE-8
```
```

- [ ] **Step 2: 运行完整测试**

Run:

```bash
cd /Users/bytedance/symphony/go
make test
```

Expected: PASS。

- [ ] **Step 3: 本地启动 TUI 验证**

Run:

```bash
cd /Users/bytedance/symphony/go
python3 - <<'PY'
import subprocess
try:
    proc = subprocess.run(["make", "run"], timeout=3, text=True, capture_output=True)
    print(proc.stdout)
    print(proc.stderr)
    raise SystemExit(proc.returncode)
except subprocess.TimeoutExpired as exc:
    out = exc.stdout or ""
    err = exc.stderr or ""
    print(out)
    print(err)
    assert "SYMPHONY STATUS" in out or "SYMPHONY STATUS" in err
    print("TUI started and rendered a status frame")
PY
```

Expected: 输出包含 `SYMPHONY STATUS`，进程被 3 秒 timeout 停止。

- [ ] **Step 4: 手动 smoke 验证**

手动执行：

```bash
cd /Users/bytedance/symphony/go
make run
```

观察 TUI：

- 顶部显示 `Agents: <running>/<max>`。
- `Next refresh` 在倒计时和 `checking now...` 之间切换。
- `Running` 区域展示正在处理的 issue、stage、session、turn、tokens、last event。
- 没有任务时显示 `No active agents`。
- retry/backoff 时出现在 `Backoff queue`。

- [ ] **Step 5: 提交**

```bash
git add go/README.md go/docs/plan/go-symphony-tui.md
git commit -m "docs(go): document terminal dashboard workflow"
```

## 自审结果

- Spec coverage：
  - `SPEC.md 13.3` snapshot 字段由 Task 1/3 覆盖。
  - `SPEC.md 13.4` human-readable terminal surface 由 Task 4/5 覆盖。
  - `SPEC.md 13.5` token/runtime accounting 由 Task 2/3 覆盖。
  - `SPEC.md 13.6` humanized event summaries 由 Task 2/4 覆盖。
- Elixir parity：
  - 对齐 `StatusDashboard` 的顶部指标、Running 表、Backoff queue、refresh 状态。
  - 暂不实现 Phoenix LiveView Web dashboard；这是后续独立计划。
- Scope boundary：
  - 本计划不实现 HTTP `/api/v1/state`。
  - 本计划不改变 workflow 调度正确性，只增加 observability state 和 terminal rendering。
  - 本计划不引入第三方 TUI 框架，降低 v1 复杂度。
