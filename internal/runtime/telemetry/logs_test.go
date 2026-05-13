package telemetry

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"symphony-go/internal/runtime/logging"
)

func TestRecordLogExportsAllowedEvents(t *testing.T) {
	logger := &recordingLogger{}
	provider, _ := newTestProvider()
	provider = testFacade{
		tracer: provider.Tracer(),
		meter:  provider.Meter(),
		logger: logger,
	}

	RecordLog(context.Background(), provider, logging.Event{
		Event:           "state_changed",
		Message:         "Todo -> In Progress",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"phase":           "implementation",
			"raw_payload":     "do not export",
			"source_file":     "internal/service/issueflow/flow.go",
			"source_function": "internal/service/issueflow.RunIssueTrunk",
			"source_line":     72,
		},
	})

	if len(logger.records) != 1 {
		t.Fatalf("records = %d, want 1", len(logger.records))
	}
	record := logger.records[0]
	if record.EventName() != "state_changed" {
		t.Fatalf("event name = %q, want state_changed", record.EventName())
	}
	if record.Body().AsString() != "Todo -> In Progress" {
		t.Fatalf("body = %q, want transition message", record.Body().AsString())
	}
	attrs := logRecordAttrs(record)
	if attrs["issue_identifier"] != "ZEE-1" || attrs["phase"] != "implementation" || attrs["source_file"] != "internal/service/issueflow/flow.go" || attrs["source_function"] != "internal/service/issueflow.RunIssueTrunk" {
		t.Fatalf("attrs = %#v, want issue_identifier, phase, and source fields", attrs)
	}
	ints := logRecordIntAttrs(record)
	if ints["source_line"] != 72 {
		t.Fatalf("int attrs = %#v, want source_line", ints)
	}
	if _, ok := attrs["raw_payload"]; ok {
		t.Fatalf("raw_payload should not be exported: %#v", attrs)
	}
}

func TestRecordLogSkipsRawCodexPayload(t *testing.T) {
	logger := &recordingLogger{}
	provider, _ := newTestProvider()
	provider = testFacade{
		tracer: provider.Tracer(),
		meter:  provider.Meter(),
		logger: logger,
	}

	RecordLog(context.Background(), provider, logging.Event{
		Event:   "codex_event",
		Message: "turn.delta",
		Fields: map[string]any{
			"payload": "large/sensitive raw event",
		},
	})

	if len(logger.records) != 0 {
		t.Fatalf("records = %d, want 0", len(logger.records))
	}
}

func TestRecordLogExportsCuratedCodexEvents(t *testing.T) {
	logger := &recordingLogger{}
	provider, _ := newTestProvider()
	provider = testFacade{
		tracer: provider.Tracer(),
		meter:  provider.Meter(),
		logger: logger,
	}

	RecordLog(context.Background(), provider, logging.Event{
		Event:           "codex_event",
		Message:         "turn_started",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		SessionID:       "session-1",
		Fields: map[string]any{
			"session_id":   "session-1",
			"turn_id":      "turn-1",
			"turn_count":   1,
			"continuation": false,
			"payload":      "raw payload must not export",
		},
	})
	RecordLog(context.Background(), provider, logging.Event{
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"params": map[string]any{
				"item": map[string]any{
					"type":  "agentMessage",
					"phase": "final_answer",
					"text":  "Final answer summary references service/drop_reward.go:264 and handler.go:1145.",
				},
			},
			"token_delta": "do not export",
		},
	})
	RecordLog(context.Background(), provider, logging.Event{
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"source_file":     "internal/service/issueflow/agent_session.go",
			"source_function": "internal/service/issueflow.runWorkerAttempt.func3",
			"source_line":     180,
			"params": map[string]any{
				"item": map[string]any{
					"type":             "commandExecution",
					"command":          "git status --short",
					"cwd":              "/tmp/work/ZEE-1",
					"exitCode":         0,
					"durationMs":       25,
					"aggregatedOutput": "this output should stay out of OTel logs",
				},
			},
		},
	})
	RecordLog(context.Background(), provider, logging.Event{
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"params": map[string]any{
				"item": map[string]any{
					"type": "fileChange",
					"changes": []any{
						map[string]any{"path": "/tmp/work/ZEE-1/SMOKE.md", "diff": "@@ -0,0 +1 @@\n+done\n"},
					},
				},
			},
		},
	})
	RecordLog(context.Background(), provider, logging.Event{
		Event:   "codex_event",
		Message: "turn_completed",
		Fields:  map[string]any{"duration_ms": 25},
	})

	if len(logger.records) != 4 {
		t.Fatalf("records = %d, want four curated codex records", len(logger.records))
	}
	names := []string{}
	for _, record := range logger.records {
		names = append(names, record.EventName())
		attrs := logRecordAttrs(record)
		if _, ok := attrs["payload"]; ok {
			t.Fatalf("payload should not be exported: %#v", attrs)
		}
		if _, ok := attrs["token_delta"]; ok {
			t.Fatalf("token_delta should not be exported: %#v", attrs)
		}
		if _, ok := attrs["output"]; ok {
			t.Fatalf("output should not be exported: %#v", attrs)
		}
	}
	wantNames := []string{"codex_turn_started", "codex_final", "codex_command", "codex_file_change"}
	if !equalStrings(names, wantNames) {
		t.Fatalf("event names = %#v, want %#v", names, wantNames)
	}
	commandAttrs := logRecordAttrs(logger.records[2])
	if commandAttrs["command"] != "git status --short" || commandAttrs["command_kind"] != "git" || commandAttrs["command_status"] != "succeeded" || commandAttrs["cwd"] != "ZEE-1" {
		t.Fatalf("command attrs = %#v", commandAttrs)
	}
	if commandAttrs["source_file"] != "internal/service/issueflow/agent_session.go" || commandAttrs["source_function"] != "internal/service/issueflow.runWorkerAttempt.func3" {
		t.Fatalf("command source attrs = %#v", commandAttrs)
	}
	if logger.records[2].Body().AsString() != "Command succeeded: git status --short (25ms)" {
		t.Fatalf("command body = %q", logger.records[2].Body().AsString())
	}
	commandInts := logRecordIntAttrs(logger.records[2])
	if commandInts["exit_code"] != 0 || commandInts["duration_ms"] != 25 || commandInts["source_line"] != 180 {
		t.Fatalf("command int attrs = %#v, want exit_code, duration_ms, and source_line", commandInts)
	}
	fileAttrs := logRecordAttrs(logger.records[3])
	if fileAttrs["file"] != "SMOKE.md" || fileAttrs["files"] != "SMOKE.md" || fileAttrs["file_locations"] != "SMOKE.md:1" || fileAttrs["evidence_locations"] != "SMOKE.md:1" {
		t.Fatalf("file attrs = %#v, want SMOKE.md location", fileAttrs)
	}
	if logger.records[3].Body().AsString() != "Changed SMOKE.md:1 (+1/-0)" {
		t.Fatalf("file body = %q", logger.records[3].Body().AsString())
	}
	fileInts := logRecordIntAttrs(logger.records[3])
	if fileInts["file_count"] != 1 || fileInts["line_start"] != 1 || fileInts["line_end"] != 1 || fileInts["changed_lines"] != 1 || fileInts["additions"] != 1 || fileInts["deletions"] != 0 {
		t.Fatalf("file int attrs = %#v, want file count and line metadata", fileInts)
	}
	finalAttrs := logRecordAttrs(logger.records[1])
	if finalAttrs["evidence_file"] != "service/drop_reward.go" || finalAttrs["evidence_locations"] != "service/drop_reward.go:264,handler.go:1145" {
		t.Fatalf("final attrs = %#v, want evidence locations", finalAttrs)
	}
	finalInts := logRecordIntAttrs(logger.records[1])
	if finalInts["evidence_line"] != 264 {
		t.Fatalf("final int attrs = %#v, want evidence_line", finalInts)
	}
}

func TestRecordLogExportsLifecycleEvents(t *testing.T) {
	logger := &recordingLogger{}
	provider, _ := newTestProvider()
	provider = testFacade{
		tracer: provider.Tracer(),
		meter:  provider.Meter(),
		logger: logger,
	}

	for _, event := range []string{
		"dispatch_started",
		"state_changed",
		"codex_turn_completed",
		"codex_turn_activity_summary",
		"codex_slow_turn",
		"review_pass",
		"merge_pass",
		"push_pass",
		"blocked",
		"issue_error",
	} {
		RecordLog(context.Background(), provider, logging.Event{
			Event:           event,
			Message:         event,
			IssueID:         "issue-1",
			IssueIdentifier: "ZEE-1",
			Fields: map[string]any{
				"from_state": "AI Review",
				"to_state":   "Pushing",
				"outcome":    "success",
			},
		})
	}

	if len(logger.records) != 10 {
		t.Fatalf("records = %d, want lifecycle logs", len(logger.records))
	}
	for _, record := range logger.records {
		attrs := logRecordAttrs(record)
		if attrs["issue_identifier"] != "ZEE-1" {
			t.Fatalf("attrs = %#v, want issue_identifier", attrs)
		}
	}
}

func TestRecordLogCreatesCuratedCodexTraceSpans(t *testing.T) {
	provider, recorder := newTestProvider()
	ctx, endRun := StartIssueRun(context.Background(), provider, map[string]any{
		"issue_id":         "issue-1",
		"issue_identifier": "ZEE-1",
	})

	RecordLog(ctx, provider, logging.Event{
		Time:            "2026-05-13T12:00:00Z",
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"phase": "implementer",
			"stage": "running_agent",
			"params": map[string]any{
				"item": map[string]any{
					"type":       "commandExecution",
					"command":    "git diff --check",
					"cwd":        "/tmp/work/ZEE-1",
					"durationMs": 25,
					"exitCode":   0,
				},
			},
		},
	})
	RecordLog(ctx, provider, logging.Event{
		Time:            "2026-05-13T12:00:01Z",
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"phase": "implementer",
			"stage": "running_agent",
			"params": map[string]any{
				"item": map[string]any{
					"type": "fileChange",
					"changes": []any{
						map[string]any{"path": "/tmp/work/ZEE-1/SMOKE.md", "diff": "@@ -3 +3,2 @@\n-old\n+new\n+line\n"},
					},
				},
			},
		},
	})
	RecordLog(ctx, provider, logging.Event{
		Time:            "2026-05-13T12:00:02Z",
		Event:           "codex_event",
		Message:         "item/completed",
		IssueID:         "issue-1",
		IssueIdentifier: "ZEE-1",
		Fields: map[string]any{
			"phase": "implementer",
			"params": map[string]any{
				"item": map[string]any{
					"type":  "agentMessage",
					"phase": "final_answer",
					"text":  "Summary: done.",
				},
			},
		},
	})
	endRun("done", nil)

	spans := spansByName(recorder.Ended())
	command := spans["step implementer/codex_command"]
	if command == nil {
		t.Fatalf("missing command span: %#v", spanNames(recorder.Ended()))
	}
	if got := command.EndTime().Sub(command.StartTime()); got != 25*time.Millisecond {
		t.Fatalf("command span duration = %s, want 25ms", got)
	}
	commandAttrs := spanAttrs(command)
	if commandAttrs["command"] != "git diff --check" || commandAttrs["command_status"] != "succeeded" || commandAttrs["outcome"] != "succeeded" {
		t.Fatalf("command attrs = %#v", commandAttrs)
	}
	if command.Status().Code != codes.Ok {
		t.Fatalf("command status = %v, want OK", command.Status())
	}
	file := spans["step implementer/codex_file_change"]
	if file == nil {
		t.Fatalf("missing file span: %#v", spanNames(recorder.Ended()))
	}
	fileAttrs := spanAttrs(file)
	if fileAttrs["file_locations"] != "SMOKE.md:3-4" || fileAttrs["outcome"] != "success" {
		t.Fatalf("file attrs = %#v", fileAttrs)
	}
	final := spans["step implementer/codex_final"]
	if final == nil {
		t.Fatalf("missing final span: %#v", spanNames(recorder.Ended()))
	}
	finalAttrs := spanAttrs(final)
	if finalAttrs["message"] != "Summary: done." {
		t.Fatalf("final attrs = %#v, want bounded final message", finalAttrs)
	}
	issueRun := spans["issue_run"]
	if issueRun == nil {
		t.Fatalf("missing issue_run span: %#v", spanNames(recorder.Ended()))
	}
	if command.Parent().SpanID() != issueRun.SpanContext().SpanID() || file.Parent().SpanID() != issueRun.SpanContext().SpanID() || final.Parent().SpanID() != issueRun.SpanContext().SpanID() {
		t.Fatalf("curated codex spans should be children of issue_run")
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func spansByName(spans []sdktrace.ReadOnlySpan) map[string]sdktrace.ReadOnlySpan {
	byName := map[string]sdktrace.ReadOnlySpan{}
	for _, span := range spans {
		byName[span.Name()] = span
	}
	return byName
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name())
	}
	return names
}

type recordingLogger struct {
	embedded.Logger
	records []otellog.Record
}

func (l *recordingLogger) Emit(_ context.Context, record otellog.Record) {
	l.records = append(l.records, record)
}

func (l *recordingLogger) Enabled(context.Context, otellog.EnabledParameters) bool {
	return true
}

func logRecordAttrs(record otellog.Record) map[string]string {
	attrs := map[string]string{}
	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		attrs[attr.Key] = attr.Value.AsString()
		return true
	})
	return attrs
}

func logRecordIntAttrs(record otellog.Record) map[string]int64 {
	attrs := map[string]int64{}
	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		if attr.Value.Kind() == otellog.KindInt64 {
			attrs[attr.Key] = attr.Value.AsInt64()
		}
		return true
	})
	return attrs
}
