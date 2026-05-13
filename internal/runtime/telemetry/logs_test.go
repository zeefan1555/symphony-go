package telemetry

import (
	"context"
	"testing"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"

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
			"phase":       "implementation",
			"raw_payload": "do not export",
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
	if attrs["issue_identifier"] != "ZEE-1" || attrs["phase"] != "implementation" {
		t.Fatalf("attrs = %#v, want issue_identifier and phase", attrs)
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
					"text":  "Final answer summary that should be exported as a bounded log body.",
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
						map[string]any{"path": "/tmp/work/ZEE-1/SMOKE.md", "diff": "+done\n"},
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
	if commandAttrs["command"] != "git status --short" || commandAttrs["cwd"] != "ZEE-1" {
		t.Fatalf("command attrs = %#v", commandAttrs)
	}
	commandInts := logRecordIntAttrs(logger.records[2])
	if commandInts["exit_code"] != 0 || commandInts["duration_ms"] != 25 {
		t.Fatalf("command int attrs = %#v, want exit_code and duration_ms", commandInts)
	}
	fileAttrs := logRecordAttrs(logger.records[3])
	if fileAttrs["files"] != "SMOKE.md" {
		t.Fatalf("file attrs = %#v, want SMOKE.md", fileAttrs)
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
		"review_pass",
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

	if len(logger.records) != 7 {
		t.Fatalf("records = %d, want lifecycle logs", len(logger.records))
	}
	for _, record := range logger.records {
		attrs := logRecordAttrs(record)
		if attrs["issue_identifier"] != "ZEE-1" {
			t.Fatalf("attrs = %#v, want issue_identifier", attrs)
		}
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
