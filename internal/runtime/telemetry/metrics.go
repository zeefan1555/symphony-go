package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/metric"
)

func RecordIssueActive(ctx context.Context, provider Facade, delta int64, fields map[string]any) {
	counter, err := activeFacade(provider).Meter().Int64UpDownCounter("symphony_issue_active")
	if err == nil {
		counter.Add(ctx, delta, metric.WithAttributes(MetricAttrs(fields)...))
	}
}

func RecordIssueRetrying(ctx context.Context, provider Facade, delta int64, fields map[string]any) {
	counter, err := activeFacade(provider).Meter().Int64UpDownCounter("symphony_issue_retrying")
	if err == nil {
		counter.Add(ctx, delta, metric.WithAttributes(MetricAttrs(fields)...))
	}
}

func RecordCodexTokens(ctx context.Context, provider Facade, input, output, total int, fields map[string]any) {
	counter, err := activeFacade(provider).Meter().Int64Counter("symphony_codex_tokens_total")
	if err != nil {
		return
	}
	recordTokenDelta(ctx, counter, "input", input, fields)
	recordTokenDelta(ctx, counter, "output", output, fields)
	recordTokenDelta(ctx, counter, "total", total, fields)
}

func recordIssueRun(ctx context.Context, provider Facade, outcome string, fields map[string]any) {
	fields = cloneFields(fields)
	fields["outcome"] = outcome
	counter, err := activeFacade(provider).Meter().Int64Counter("symphony_issue_run_total")
	if err == nil {
		counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(fields)...))
	}
}

func recordTransitionMetrics(ctx context.Context, provider Facade, elapsed time.Duration, fields map[string]any) {
	meter := activeFacade(provider).Meter()
	counter, err := meter.Int64Counter("symphony_issue_transition_total")
	if err == nil {
		counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(fields)...))
	}
	histogram, err := meter.Float64Histogram("symphony_issue_transition_duration_ms")
	if err == nil {
		histogram.Record(ctx, durationMS(elapsed), metric.WithAttributes(MetricAttrs(fields)...))
	}
}

func recordStepMetrics(ctx context.Context, provider Facade, elapsed time.Duration, fields map[string]any, err error) {
	meter := activeFacade(provider).Meter()
	histogram, histogramErr := meter.Float64Histogram("symphony_issue_phase_duration_ms")
	if histogramErr == nil {
		histogram.Record(ctx, durationMS(elapsed), metric.WithAttributes(MetricAttrs(fields)...))
	}
	if fields["step"] == "codex_turn_completed" {
		counter, counterErr := meter.Int64Counter("symphony_codex_turn_total")
		if counterErr == nil {
			counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(fields)...))
		}
		turnHistogram, turnHistogramErr := meter.Float64Histogram("symphony_codex_turn_duration_ms")
		if turnHistogramErr == nil {
			turnHistogram.Record(ctx, durationMS(elapsed), metric.WithAttributes(MetricAttrs(fields)...))
		}
	}
	if fields["step"] == "codex_slow_turn" {
		counter, counterErr := meter.Int64Counter("symphony_codex_slow_turn_total")
		if counterErr == nil {
			counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(selectFields(fields, "phase", "stage", "state", "outcome"))...))
		}
	}
	if err != nil {
		failureFields := cloneFields(fields)
		failureFields["error_type"] = errorType(err)
		counter, counterErr := meter.Int64Counter("symphony_issue_step_failure_total")
		if counterErr == nil {
			counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(failureFields)...))
		}
	}
}

func recordCodexCommandMetrics(ctx context.Context, provider Facade, fields map[string]any) {
	meter := activeFacade(provider).Meter()
	metricFields := selectFields(fields, "phase", "stage", "command_kind", "command_status")
	counter, counterErr := meter.Int64Counter("symphony_codex_command_total")
	if counterErr == nil {
		counter.Add(ctx, 1, metric.WithAttributes(MetricAttrs(metricFields)...))
	}
	if duration, ok := numericDurationMS(fields["duration_ms"]); ok {
		histogram, histogramErr := meter.Float64Histogram("symphony_codex_command_duration_ms")
		if histogramErr == nil {
			histogram.Record(ctx, duration, metric.WithAttributes(MetricAttrs(metricFields)...))
		}
	}
}

func recordTokenDelta(ctx context.Context, counter metric.Int64Counter, tokenType string, value int, fields map[string]any) {
	if value <= 0 {
		return
	}
	fields = cloneFields(fields)
	fields["token_type"] = tokenType
	counter.Add(ctx, int64(value), metric.WithAttributes(MetricAttrs(fields)...))
}

func durationMS(elapsed time.Duration) float64 {
	return float64(elapsed) / float64(time.Millisecond)
}

func numericDurationMS(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func selectFields(fields map[string]any, keys ...string) map[string]any {
	selected := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := fields[key]; ok {
			selected[key] = value
		}
	}
	return selected
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
