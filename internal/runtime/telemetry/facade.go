package telemetry

import (
	"context"

	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const scopeName = "symphony-go/internal/runtime/telemetry"

type EndFunc func(outcome string, err error)

type Facade interface {
	Enabled() bool
	Tracer() trace.Tracer
	Meter() metric.Meter
	Logger() otellog.Logger
	Shutdown(context.Context) error
}

func TraceFields(ctx context.Context) map[string]any {
	span := trace.SpanContextFromContext(ctx)
	if !span.IsValid() {
		return nil
	}
	return map[string]any{
		"trace_id": span.TraceID().String(),
		"span_id":  span.SpanID().String(),
	}
}

func activeFacade(provider Facade) Facade {
	if provider == nil {
		return NewNoop()
	}
	return provider
}
