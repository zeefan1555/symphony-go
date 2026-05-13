package telemetry

import (
	"context"
	"reflect"
	"testing"

	otellog "go.opentelemetry.io/otel/log"
	nooplog "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestNewFromEnvReturnsNoopWithoutEndpoint(t *testing.T) {
	clearOTelEnv(t)

	provider, err := NewFromEnv(context.Background())
	if err != nil {
		t.Fatalf("NewFromEnv() error = %v", err)
	}
	if provider.Enabled() {
		t.Fatal("provider should be disabled without OTLP endpoint")
	}
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestTraceFieldsReturnsIDsFromContext(t *testing.T) {
	traceID := trace.TraceID{1, 2, 3}
	spanID := trace.SpanID{4, 5, 6}
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  spanID,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)

	fields := TraceFields(ctx)
	if fields["trace_id"] != traceID.String() || fields["span_id"] != spanID.String() {
		t.Fatalf("TraceFields() = %#v, want trace/span IDs", fields)
	}
}

func TestStartIssueRunAndRecordTransitionCreateSpans(t *testing.T) {
	provider, recorder := newTestProvider()

	ctx, end := StartIssueRun(context.Background(), provider, map[string]any{
		"issue_identifier": "ZEE-1",
		"state":            "Todo",
	})
	RecordTransition(ctx, provider, "Todo", "In Progress", "success", map[string]any{
		"issue_identifier": "ZEE-1",
	}, nil)
	end("done", nil)

	ended := recorder.Ended()
	if len(ended) != 2 {
		t.Fatalf("ended spans = %d, want 2", len(ended))
	}
	names := []string{ended[0].Name(), ended[1].Name()}
	want := []string{"transition Todo -> In Progress", "issue_run"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("span names = %#v, want %#v", names, want)
	}
	if ended[0].Parent().SpanID() != ended[1].SpanContext().SpanID() {
		t.Fatalf("transition parent = %s, want issue_run span %s", ended[0].Parent().SpanID(), ended[1].SpanContext().SpanID())
	}
	if ended[1].EndTime().Before(ended[0].EndTime()) {
		t.Fatalf("issue_run ended at %s before child transition ended at %s", ended[1].EndTime(), ended[0].EndTime())
	}
	attrs := spanAttrs(ended[1])
	if attrs["outcome"] != "done" {
		t.Fatalf("issue_run attrs = %#v, want final outcome", attrs)
	}
}

func TestMetricAttrsDropsHighCardinalityLabels(t *testing.T) {
	attrs := MetricAttrs(map[string]any{
		"issue_id":         "issue-1",
		"issue_identifier": "ZEE-1",
		"additions":        1,
		"changed_lines":    2,
		"command":          "git status --short",
		"cwd":              "/tmp/ZEE-1",
		"deletions":        1,
		"file":             "SMOKE.md",
		"file_count":       1,
		"file_locations":   "SMOKE.md:1-2",
		"files":            "SMOKE.md",
		"line_end":         2,
		"line_start":       1,
		"message":          "bounded but trace/log only",
		"session_id":       "session",
		"source_file":      "internal/service/issueflow/flow.go",
		"source_function":  "internal/service/issueflow.RunIssueTrunk",
		"source_line":      72,
		"thread_id":        "thread",
		"turn_id":          "turn",
		"workspace_path":   "/tmp/ZEE-1",
		"phase":            "implementation",
		"outcome":          "success",
	})
	got := map[string]string{}
	for _, attr := range attrs {
		got[string(attr.Key)] = attr.Value.AsString()
	}
	want := map[string]string{"phase": "implementation", "outcome": "success"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MetricAttrs() = %#v, want %#v", got, want)
	}
}

func spanAttrs(span sdktrace.ReadOnlySpan) map[string]string {
	attrs := map[string]string{}
	for _, attr := range span.Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsString()
	}
	return attrs
}

type testFacade struct {
	tracer trace.Tracer
	meter  metric.Meter
	logger otellog.Logger
}

func newTestProvider() (Facade, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	return testFacade{
		tracer: traceProvider.Tracer("test"),
		meter:  noopmetric.NewMeterProvider().Meter("test"),
		logger: nooplog.NewLoggerProvider().Logger("test"),
	}, recorder
}

func (p testFacade) Enabled() bool {
	return true
}

func (p testFacade) Tracer() trace.Tracer {
	return p.tracer
}

func (p testFacade) Meter() metric.Meter {
	return p.meter
}

func (p testFacade) Logger() otellog.Logger {
	return p.logger
}

func (p testFacade) Shutdown(context.Context) error {
	return nil
}

func clearOTelEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OTEL_SERVICE_NAME",
		"OTEL_EXPORTER_OTLP_PROTOCOL",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
	} {
		t.Setenv(key, "")
	}
}
