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
