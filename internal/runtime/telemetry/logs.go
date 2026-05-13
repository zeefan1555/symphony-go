package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/codes"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"

	"symphony-go/internal/runtime/logging"
)

var exportedLogEvents = map[string]bool{
	"after_run_hook_failed":    true,
	"codex_command":            true,
	"codex_file_change":        true,
	"codex_final":              true,
	"codex_turn_started":       true,
	"ai_review_failed":         true,
	"blocked":                  true,
	"codex_turn_completed":     true,
	"dispatch_skipped":         true,
	"dispatch_started":         true,
	"issue_error":              true,
	"poll_error":               true,
	"push_pass":                true,
	"reconcile_error":          true,
	"retry_fetch_error":        true,
	"review_pass":              true,
	"state_changed":            true,
	"turn_completed":           true,
	"waiting_for_ai_review":    true,
	"waiting_for_review":       true,
	"worker_stalled":           true,
	"workflow_reload_failed":   true,
	"workspace_cleanup_failed": true,
	"workspace_hook_failed":    true,
}

var logFieldAllowlist = map[string]bool{
	"attempt_kind":      true,
	"additions":         true,
	"changed_lines":     true,
	"command":           true,
	"command_status":    true,
	"continuation":      true,
	"cwd":               true,
	"deletions":         true,
	"duration_ms":       true,
	"error":             true,
	"error_type":        true,
	"exit_code":         true,
	"file":              true,
	"file_count":        true,
	"file_locations":    true,
	"files":             true,
	"from_state":        true,
	"issue_id":          true,
	"issue_identifier":  true,
	"line_end":          true,
	"line_start":        true,
	"outcome":           true,
	"phase":             true,
	"session_id":        true,
	"span_id":           true,
	"stage":             true,
	"state":             true,
	"step":              true,
	"to_state":          true,
	"trace_id":          true,
	"transition_from":   true,
	"transition_to":     true,
	"turn_count":        true,
	"turn_id":           true,
	"workflow_mode":     true,
	"workspace_cleanup": true,
}

func RecordLog(ctx context.Context, provider Facade, event logging.Event) {
	provider = activeFacade(provider)
	event, ok := curatedLogEvent(event)
	if !ok {
		return
	}
	if !provider.Enabled() || !exportedLogEvents[event.Event] {
		return
	}
	timestamp := logTimestamp(event)
	recordCuratedLogSpan(ctx, provider, event, timestamp)
	level := logging.InferLevel(event)
	severity := logSeverity(level)
	logger := provider.Logger()
	if !logger.Enabled(ctx, otellog.EnabledParameters{Severity: severity}) {
		return
	}
	levelName := event.Level
	if levelName == "" {
		levelName = logging.LevelName(level)
	}
	body := event.Message
	if body == "" {
		body = event.Event
	}
	var record otellog.Record
	record.SetTimestamp(timestamp)
	record.SetObservedTimestamp(time.Now())
	record.SetSeverity(severity)
	record.SetSeverityText(levelName)
	record.SetEventName(event.Event)
	record.SetBody(otellog.StringValue(body))
	record.AddAttributes(logAttributes(event)...)
	logger.Emit(ctx, record)
}

func logTimestamp(event logging.Event) time.Time {
	if event.Time != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, event.Time); err == nil {
			return parsed
		}
	}
	return time.Now()
}

func curatedLogEvent(event logging.Event) (logging.Event, bool) {
	display, ok := logging.HumanEvent(event)
	if !ok {
		return logging.Event{}, false
	}
	if event.Event != "codex_event" {
		return display, true
	}
	if display.Event == "codex_turn_completed" {
		return logging.Event{}, false
	}
	display.TraceID = event.TraceID
	display.SpanID = event.SpanID
	display.IssueID = firstNonEmpty(display.IssueID, event.IssueID)
	display.IssueIdentifier = firstNonEmpty(display.IssueIdentifier, event.IssueIdentifier)
	display.SessionID = firstNonEmpty(display.SessionID, event.SessionID)
	display.Fields = mergeCodexLogFields(display.Fields, event.Fields)
	return display, true
}

func recordCuratedLogSpan(ctx context.Context, provider Facade, event logging.Event, completedAt time.Time) {
	switch event.Event {
	case "codex_command":
		recordCommandLogSpan(ctx, provider, event, completedAt)
	case "codex_file_change", "codex_final":
		recordInstantLogSpan(ctx, provider, event, completedAt)
	}
}

func recordCommandLogSpan(ctx context.Context, provider Facade, event logging.Event, completedAt time.Time) {
	startedAt := completedAt
	if durationMS, ok := int64LogField(event.Fields, "duration_ms"); ok && durationMS > 0 {
		startedAt = completedAt.Add(-time.Duration(durationMS) * time.Millisecond)
	}
	_, span := provider.Tracer().Start(ctx, logSpanName(event), trace.WithAttributes(Attrs(logSpanFields(event))...), trace.WithTimestamp(startedAt))
	setLogSpanStatus(span, event)
	span.End(trace.WithTimestamp(completedAt))
}

func recordInstantLogSpan(ctx context.Context, provider Facade, event logging.Event, timestamp time.Time) {
	_, span := provider.Tracer().Start(ctx, logSpanName(event), trace.WithAttributes(Attrs(logSpanFields(event))...), trace.WithTimestamp(timestamp))
	setLogSpanStatus(span, event)
	span.End(trace.WithTimestamp(timestamp))
}

func logSpanName(event logging.Event) string {
	phase := stringLogField(event.Fields, "phase")
	if phase == "" {
		phase = "codex"
	}
	return "step " + phase + "/" + event.Event
}

func logSpanFields(event logging.Event) map[string]any {
	fields := map[string]any{
		"event":            event.Event,
		"issue_id":         event.IssueID,
		"issue_identifier": event.IssueIdentifier,
		"message":          event.Message,
		"outcome":          logSpanOutcome(event),
		"session_id":       event.SessionID,
		"span_id":          event.SpanID,
		"step":             event.Event,
		"trace_id":         event.TraceID,
	}
	for key, value := range event.Fields {
		fields[key] = value
	}
	return fields
}

func logSpanOutcome(event logging.Event) string {
	if status := stringLogField(event.Fields, "command_status"); status != "" {
		return status
	}
	return "success"
}

func setLogSpanStatus(span trace.Span, event logging.Event) {
	if logSpanOutcome(event) == "failed" {
		span.SetStatus(codes.Error, "command failed")
		return
	}
	span.SetStatus(codes.Ok, "")
}

func mergeCodexLogFields(displayFields map[string]any, rawFields map[string]any) map[string]any {
	fields := make(map[string]any, len(displayFields)+8)
	for key, value := range displayFields {
		fields[key] = value
	}
	for _, key := range []string{
		"session_id",
		"turn_id",
		"turn_count",
		"continuation",
		"duration_ms",
		"phase",
		"stage",
		"state",
	} {
		if value, ok := rawFields[key]; ok {
			fields[key] = value
		}
	}
	return fields
}

func stringLogField(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	value, ok := fields[key]
	if !ok || value == nil {
		return ""
	}
	if typed, ok := value.(string); ok {
		return typed
	}
	return fmt.Sprint(value)
}

func int64LogField(fields map[string]any, key string) (int64, bool) {
	if fields == nil {
		return 0, false
	}
	switch typed := fields[key].(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func logSeverity(level slog.Level) otellog.Severity {
	switch {
	case level >= slog.LevelError:
		return otellog.SeverityError
	case level >= slog.LevelWarn:
		return otellog.SeverityWarn
	case level <= slog.LevelDebug:
		return otellog.SeverityDebug
	default:
		return otellog.SeverityInfo
	}
}

func logAttributes(event logging.Event) []otellog.KeyValue {
	fields := map[string]any{
		"event":            event.Event,
		"issue_id":         event.IssueID,
		"issue_identifier": event.IssueIdentifier,
		"session_id":       event.SessionID,
		"trace_id":         event.TraceID,
		"span_id":          event.SpanID,
	}
	for key, value := range event.Fields {
		if logFieldAllowlist[key] {
			fields[key] = value
		}
	}
	attrs := make([]otellog.KeyValue, 0, len(fields))
	for key, value := range fields {
		if attr, ok := logAttr(key, value); ok {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

func logAttr(key string, value any) (otellog.KeyValue, bool) {
	switch typed := value.(type) {
	case nil:
		return otellog.KeyValue{}, false
	case string:
		if typed == "" {
			return otellog.KeyValue{}, false
		}
		return otellog.String(key, typed), true
	case bool:
		return otellog.Bool(key, typed), true
	case int:
		return otellog.Int(key, typed), true
	case int64:
		return otellog.Int64(key, typed), true
	case float64:
		return otellog.Float64(key, typed), true
	default:
		text := fmt.Sprint(typed)
		if text == "" {
			return otellog.KeyValue{}, false
		}
		return otellog.String(key, text), true
	}
}
