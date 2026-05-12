package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestMetricsUseLowCardinalityAttributes(t *testing.T) {
	provider, reader := newMetricTestProvider()
	ctx := context.Background()

	runCtx, endRun := StartIssueRun(ctx, provider, highCardinalityFields())
	transitionCtx, endTransition := StartTransition(runCtx, provider, "Todo", "In Progress", highCardinalityFields())
	endTransition("success", nil)
	stepCtx, endStep := StartStep(transitionCtx, provider, "implementer", "workspace_prepared", highCardinalityFields())
	endStep("success", nil)
	RecordStep(stepCtx, provider, "implementer", "codex_turn_completed", "success", highCardinalityFields(), nil)
	RecordStep(stepCtx, provider, "implementer", "workspace_prepared", "error", highCardinalityFields(), errors.New("boom"))
	RecordCodexTokens(stepCtx, provider, 1, 2, 3, highCardinalityFields())
	RecordIssueActive(stepCtx, provider, 1, highCardinalityFields())
	RecordIssueRetrying(stepCtx, provider, 1, highCardinalityFields())
	endRun("done", nil)

	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &metrics); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	names := metricNames(metrics)
	for _, want := range []string{
		"symphony_issue_run_total",
		"symphony_issue_transition_total",
		"symphony_issue_transition_duration_ms",
		"symphony_issue_phase_duration_ms",
		"symphony_issue_step_failure_total",
		"symphony_codex_turn_total",
		"symphony_codex_tokens_total",
		"symphony_issue_active",
		"symphony_issue_retrying",
	} {
		if !names[want] {
			t.Fatalf("metric names = %#v, missing %q", names, want)
		}
	}
	assertNoBlockedMetricAttrs(t, metrics)
}

func highCardinalityFields() map[string]any {
	return map[string]any{
		"issue_id":         "issue-1",
		"issue_identifier": "ZEE-1",
		"session_id":       "session-1",
		"thread_id":        "thread-1",
		"turn_id":          "turn-1",
		"workspace_path":   "/tmp/ZEE-1",
		"phase":            "implementer",
		"stage":            "running_agent",
		"outcome":          "success",
	}
}

func newMetricTestProvider() (Facade, *sdkmetric.ManualReader) {
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	return testMetricFacade{
		tracer: nooptrace.NewTracerProvider().Tracer("test"),
		meter:  meterProvider.Meter("test"),
	}, reader
}

type testMetricFacade struct {
	tracer trace.Tracer
	meter  metric.Meter
}

func (p testMetricFacade) Enabled() bool {
	return true
}

func (p testMetricFacade) Tracer() trace.Tracer {
	return p.tracer
}

func (p testMetricFacade) Meter() metric.Meter {
	return p.meter
}

func (p testMetricFacade) Shutdown(context.Context) error {
	return nil
}

func metricNames(metrics metricdata.ResourceMetrics) map[string]bool {
	names := map[string]bool{}
	for _, scopeMetrics := range metrics.ScopeMetrics {
		for _, item := range scopeMetrics.Metrics {
			names[item.Name] = true
		}
	}
	return names
}

func assertNoBlockedMetricAttrs(t *testing.T, metrics metricdata.ResourceMetrics) {
	t.Helper()
	for _, scopeMetrics := range metrics.ScopeMetrics {
		for _, item := range scopeMetrics.Metrics {
			for _, attrs := range dataPointAttributes(item.Data) {
				for _, attr := range attrs.ToSlice() {
					if metricLabelBlocklist[string(attr.Key)] {
						t.Fatalf("metric %q has blocked attribute %q", item.Name, attr.Key)
					}
				}
			}
		}
	}
}

func dataPointAttributes(data metricdata.Aggregation) []attribute.Set {
	switch typed := data.(type) {
	case metricdata.Sum[int64]:
		sets := make([]attribute.Set, 0, len(typed.DataPoints))
		for _, point := range typed.DataPoints {
			sets = append(sets, point.Attributes)
		}
		return sets
	case metricdata.Sum[float64]:
		sets := make([]attribute.Set, 0, len(typed.DataPoints))
		for _, point := range typed.DataPoints {
			sets = append(sets, point.Attributes)
		}
		return sets
	case metricdata.Histogram[int64]:
		sets := make([]attribute.Set, 0, len(typed.DataPoints))
		for _, point := range typed.DataPoints {
			sets = append(sets, point.Attributes)
		}
		return sets
	case metricdata.Histogram[float64]:
		sets := make([]attribute.Set, 0, len(typed.DataPoints))
		for _, point := range typed.DataPoints {
			sets = append(sets, point.Attributes)
		}
		return sets
	default:
		return nil
	}
}
