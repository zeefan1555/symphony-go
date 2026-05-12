package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

type noopProvider struct {
	tracer trace.Tracer
	meter  metric.Meter
}

func NewNoop() Facade {
	return noopProvider{
		tracer: nooptrace.NewTracerProvider().Tracer(scopeName),
		meter:  noopmetric.NewMeterProvider().Meter(scopeName),
	}
}

func (p noopProvider) Enabled() bool {
	return false
}

func (p noopProvider) Tracer() trace.Tracer {
	return p.tracer
}

func (p noopProvider) Meter() metric.Meter {
	return p.meter
}

func (p noopProvider) Shutdown(context.Context) error {
	return nil
}
