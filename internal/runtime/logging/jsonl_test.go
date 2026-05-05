package logging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesChineseJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	logger, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Issue:           "ZEE-8",
		IssueID:         "issue-id",
		IssueIdentifier: "ZEE-8",
		SessionID:       "session-1",
		Event:           "中文事件",
		Message:         "zeefan 中文 smoke test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("missing jsonl row")
	}
	var event Event
	if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Event != "中文事件" || event.Message != "zeefan 中文 smoke test" {
		t.Fatalf("event = %#v", event)
	}
	if event.Level != "info" {
		t.Fatalf("level = %q, want info", event.Level)
	}
	if event.IssueID != "issue-id" || event.IssueIdentifier != "ZEE-8" || event.SessionID != "session-1" {
		t.Fatalf("structured fields = %#v", event)
	}
}

func TestLoggerWritesColoredConsoleSink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	var console bytes.Buffer
	logger, err := New(path, WithConsole(&console, true))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:            "2026-05-01T20:00:00Z",
		IssueIdentifier: "ZEE-8",
		Event:           "ai_review_failed",
		Message:         "AI Review 未通过",
		Fields: map[string]any{
			"commit": "abc123",
			"params": map[string]any{
				"large": "payload",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	out := console.String()
	for _, want := range []string{"20:00:00", "\033[33mWARN\033[0m", "event=ai_review_failed", "issue=ZEE-8", "commit=abc123", "msg=\"AI Review 未通过\""} {
		if !strings.Contains(out, want) {
			t.Fatalf("console output %q missing %q", out, want)
		}
	}
	if strings.Contains(out, "payload") {
		t.Fatalf("console output included raw params payload: %q", out)
	}
}

func TestLoggerWritesPersistentHumanLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "run.jsonl")
	humanPath := HumanLogPath(path)
	logger, err := New(path, WithHumanFile(humanPath, false), WithHumanFileMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:            "2026-05-01T20:00:00Z",
		IssueIdentifier: "ZEE-8",
		Event:           "state_changed",
		Message:         "Todo -> In Progress",
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:01Z",
		Event:   "codex_event",
		Message: "item/agentMessage/delta",
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, `event=state_changed`) || !strings.Contains(out, `msg="Todo -> In Progress"`) {
		t.Fatalf("human log = %q", out)
	}
	if strings.Contains(out, "codex_event") {
		t.Fatalf("human log included default debug event: %q", out)
	}
}

func TestHumanLogWritesIssueSectionHeaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "run.jsonl")
	humanPath := HumanLogPath(path)
	logger, err := New(path, WithHumanFile(humanPath, false), WithHumanFileMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{
		{
			Time:            "2026-05-01T20:00:00Z",
			IssueIdentifier: "ZEE-8",
			Event:           "state_changed",
			Message:         "Todo -> In Progress",
		},
		{
			Time:            "2026-05-01T20:00:01Z",
			IssueIdentifier: "ZEE-8",
			Event:           "workpad_updated",
			Message:         "Linear workpad updated",
		},
		{
			Time:            "2026-05-01T20:00:02Z",
			IssueIdentifier: "ZEE-9",
			Event:           "state_changed",
			Message:         "Todo -> In Progress",
		},
	} {
		if err := logger.Write(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 5 {
		t.Fatalf("human log lines = %d, want 5: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "event=issue_section") || !strings.Contains(lines[0], "issue=ZEE-8") {
		t.Fatalf("first line should start ZEE-8 section: %q", out)
	}
	if strings.Count(out, "event=issue_section issue=ZEE-8") != 1 ||
		strings.Count(out, "event=issue_section issue=ZEE-9") != 1 {
		t.Fatalf("human log should include one section header per issue: %q", out)
	}
	jsonl, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonl), "issue_section") {
		t.Fatalf("json log should not include display-only section headers: %s", jsonl)
	}
}

func TestHumanLogSummarizesCodexEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "run.jsonl")
	humanPath := HumanLogPath(path)
	logger, err := New(path, WithHumanFile(humanPath, false), WithHumanFileMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{
		{
			Time:            "2026-05-01T20:00:00Z",
			IssueIdentifier: "ZEE-8",
			Event:           "codex_event",
			Message:         "turn/plan/updated",
			Fields: map[string]any{
				"params": map[string]any{
					"threadId": "thread-1",
					"turnId":   "turn-1",
					"plan": []any{
						map[string]any{"status": "completed", "step": "定位目标文件"},
						map[string]any{"status": "inProgress", "step": "运行验证命令"},
						map[string]any{"status": "pending", "step": "提交本地 commit"},
					},
				},
			},
		},
		{
			Time:            "2026-05-01T20:00:01Z",
			IssueIdentifier: "ZEE-8",
			Event:           "codex_event",
			Message:         "item/completed",
			Fields: map[string]any{
				"params": map[string]any{
					"threadId": "thread-1",
					"turnId":   "turn-1",
					"item": map[string]any{
						"type":             "commandExecution",
						"command":          "/bin/zsh -lc 'git status --short'",
						"cwd":              "/tmp/ZEE-8",
						"durationMs":       12,
						"exitCode":         0,
						"aggregatedOutput": " M SMOKE.md\n",
					},
				},
			},
		},
		{
			Time:            "2026-05-01T20:00:02Z",
			IssueIdentifier: "ZEE-8",
			Event:           "codex_event",
			Message:         "item/completed",
			Fields: map[string]any{
				"params": map[string]any{
					"threadId": "thread-1",
					"turnId":   "turn-1",
					"item": map[string]any{
						"type":  "agentMessage",
						"phase": "commentary",
						"text":  "已确认 Worktree 干净，准备修改 SMOKE.md。",
					},
				},
			},
		},
		{
			Time:            "2026-05-01T20:00:03Z",
			IssueIdentifier: "ZEE-8",
			Event:           "codex_event",
			Message:         "item/completed",
			Fields: map[string]any{
				"params": map[string]any{
					"threadId": "thread-1",
					"turnId":   "turn-1",
					"item": map[string]any{
						"type": "fileChange",
						"changes": []any{
							map[string]any{
								"path": "/tmp/ZEE-8/SMOKE.md",
								"diff": "@@ -1 +1,2 @@\n old\n+new\n",
							},
						},
					},
				},
			},
		},
	}
	for _, event := range events {
		if err := logger.Write(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	for _, want := range []string{
		"event=codex_plan", "progress=1/3", "current=运行验证命令",
		"event=codex_command", `command="git status --short"`, "cwd=ZEE-8", `output="M SMOKE.md"`,
		"event=codex_message", "phase=commentary", "已确认",
		"event=codex_file_change", "files=SMOKE.md", "summary=+1/-0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("human log %q missing %q", out, want)
		}
	}
	if strings.Contains(out, "codex_event") {
		t.Fatalf("human log should show summarized codex events, got %q", out)
	}
}

func TestHumanLogPath(t *testing.T) {
	got := HumanLogPath(filepath.Join("logs", "run-20260501-211219.jsonl"))
	want := filepath.Join("logs", "run-20260501-211219.human.log")
	if got != want {
		t.Fatalf("HumanLogPath() = %q, want %q", got, want)
	}
}

func TestConsoleSkipsDebugByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	var console bytes.Buffer
	logger, err := New(path, WithConsole(&console, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:00Z",
		Event:   "codex_event",
		Message: "item/agentMessage/delta",
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(console.String()); got != "" {
		t.Fatalf("console output = %q, want empty", got)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"level":"debug"`) {
		t.Fatalf("json log missing debug level: %s", raw)
	}
}

func TestHumanLogSkipsSuccessfulWorkspaceCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "run.jsonl")
	humanPath := HumanLogPath(path)
	logger, err := New(path, WithHumanFile(humanPath, false), WithHumanFileMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:00Z",
		Event:   "workspace_cleaned",
		Message: "workspace removed",
		Fields:  map[string]any{"workspace_path": "/tmp/ZEE-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("human log = %q, want empty", raw)
	}
	jsonl, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonl), `"event":"workspace_cleaned"`) {
		t.Fatalf("json log missing workspace_cleaned: %s", jsonl)
	}
}

func TestHumanLogSkipsSuccessfulStartupCleanupHooks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "run.jsonl")
	humanPath := HumanLogPath(path)
	logger, err := New(path, WithHumanFile(humanPath, false), WithHumanFileMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []Event{
		{
			Time:    "2026-05-01T20:00:00Z",
			Event:   "workspace_hook_started",
			Message: "before_remove hook started",
			Fields:  map[string]any{"hook": "before_remove", "stage": "started", "source": "startup_cleanup"},
		},
		{
			Time:    "2026-05-01T20:00:01Z",
			Event:   "workspace_hook_completed",
			Message: "before_remove hook completed",
			Fields:  map[string]any{"hook": "before_remove", "stage": "completed", "source": "startup_cleanup"},
		},
	} {
		if err := logger.Write(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(humanPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("human log = %q, want empty", raw)
	}
	jsonl, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(jsonl), `"event":"workspace_hook_started"`) ||
		!strings.Contains(string(jsonl), `"source":"startup_cleanup"`) {
		t.Fatalf("json log missing startup cleanup hook evidence: %s", jsonl)
	}
}

func TestConsoleSkipsContextReadCommandsByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	var console bytes.Buffer
	logger, err := New(path, WithConsole(&console, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:00Z",
		Event:   "codex_event",
		Message: "item/completed",
		Fields: map[string]any{
			"params": map[string]any{
				"item": map[string]any{
					"type":             "commandExecution",
					"command":          "/bin/zsh -lc 'sed -n 1,20p MEMORY.md'",
					"cwd":              "/Users/bytedance/.codex/memories",
					"durationMs":       1,
					"exitCode":         0,
					"aggregatedOutput": "memory notes\n",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(console.String()); got != "" {
		t.Fatalf("console output = %q, want empty", got)
	}
}

func TestConsoleCanShowDebugWhenEnabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "run.jsonl")
	var console bytes.Buffer
	logger, err := New(path, WithConsole(&console, false), WithConsoleMinLevel(slog.LevelDebug))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:00Z",
		Event:   "codex_event",
		Message: "item/completed",
		Fields: map[string]any{
			"params": map[string]any{
				"item": map[string]any{
					"type":  "agentMessage",
					"phase": "commentary",
					"text":  "AI 内部进展说明",
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Write(Event{
		Time:    "2026-05-01T20:00:01Z",
		Event:   "codex_event",
		Message: "item/agentMessage/delta",
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	out := console.String()
	if !strings.Contains(out, "DEBUG") || !strings.Contains(out, "event=codex_message") || !strings.Contains(out, "AI 内部进展说明") {
		t.Fatalf("console output = %q", out)
	}
	if strings.Contains(out, "agentMessage/delta") {
		t.Fatalf("console output included raw delta: %q", out)
	}
}

func TestFormatConsoleKeepsArrowsReadable(t *testing.T) {
	got := FormatConsole(Event{
		Time:    "2026-05-01T20:00:00Z",
		Level:   "info",
		Event:   "state_changed",
		Message: "Todo -> In Progress",
	}, false)
	if !strings.Contains(got, `msg="Todo -> In Progress"`) {
		t.Fatalf("FormatConsole() = %q", got)
	}
	if strings.Contains(got, `\u003e`) {
		t.Fatalf("FormatConsole() escaped arrow: %q", got)
	}
}

func TestInferLevel(t *testing.T) {
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{name: "codex", event: Event{Event: "codex_event"}, want: "debug"},
		{name: "state", event: Event{Event: "state_changed"}, want: "info"},
		{name: "wait", event: Event{Event: "waiting_for_review"}, want: "warn"},
		{name: "error", event: Event{Event: "issue_error"}, want: "error"},
		{name: "blocked", event: Event{Event: "merge_blocked"}, want: "warn"},
		{name: "cleanup", event: Event{Event: "workspace_cleaned"}, want: "debug"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LevelName(InferLevel(tt.event)); got != tt.want {
				t.Fatalf("InferLevel(%#v) = %q, want %q", tt.event, got, tt.want)
			}
		})
	}
}
