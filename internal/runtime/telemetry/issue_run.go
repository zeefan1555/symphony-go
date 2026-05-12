package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func StartIssueRun(ctx context.Context, provider Facade, fields map[string]any) (context.Context, EndFunc) {
	provider = activeFacade(provider)
	ctx, span := provider.Tracer().Start(ctx, "issue_run", trace.WithAttributes(Attrs(fields)...))
	return ctx, func(outcome string, err error) {
		if outcome != "" {
			span.SetAttributes(Attrs(map[string]any{"outcome": outcome})...)
		}
		recordSpanError(span, err)
		span.End()
		recordIssueRun(ctx, provider, outcome, fields)
	}
}

func RecordTransition(ctx context.Context, provider Facade, fromState, toState, outcome string, fields map[string]any, err error) {
	_, end := StartTransition(ctx, provider, fromState, toState, fields)
	end(outcome, err)
}

func StartTransition(ctx context.Context, provider Facade, fromState, toState string, fields map[string]any) (context.Context, EndFunc) {
	provider = activeFacade(provider)
	fields = cloneFields(fields)
	fields["from_state"] = fromState
	fields["to_state"] = toState
	startedAt := time.Now()
	ctx, span := provider.Tracer().Start(ctx, "transition "+fromState+" -> "+toState, trace.WithAttributes(Attrs(fields)...))
	return ctx, func(outcome string, err error) {
		if outcome != "" {
			fields["outcome"] = outcome
			span.SetAttributes(Attrs(map[string]any{"outcome": outcome})...)
		}
		recordSpanError(span, err)
		span.End()
		recordTransitionMetrics(ctx, provider, time.Since(startedAt), fields)
	}
}

func StartStep(ctx context.Context, provider Facade, phase, step string, fields map[string]any) (context.Context, EndFunc) {
	provider = activeFacade(provider)
	fields = stepFields(phase, step, "", fields)
	startedAt := time.Now()
	ctx, span := provider.Tracer().Start(ctx, "step "+phase+"/"+step, trace.WithAttributes(Attrs(fields)...))
	return ctx, func(outcome string, err error) {
		if outcome != "" {
			fields["outcome"] = outcome
			span.SetAttributes(Attrs(map[string]any{"outcome": outcome})...)
		}
		recordSpanError(span, err)
		span.End()
		recordStepMetrics(ctx, provider, time.Since(startedAt), fields, err)
	}
}

func RecordStep(ctx context.Context, provider Facade, phase, step, outcome string, fields map[string]any, err error) {
	provider = activeFacade(provider)
	fields = stepFields(phase, step, outcome, fields)
	_, span := provider.Tracer().Start(ctx, "step "+phase+"/"+step, trace.WithAttributes(Attrs(fields)...))
	recordSpanError(span, err)
	span.End()
	recordStepMetrics(ctx, provider, 0, fields, err)
}

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		span.SetStatus(codes.Ok, "")
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func stepFields(phase, step, outcome string, fields map[string]any) map[string]any {
	fields = cloneFields(fields)
	fields["phase"] = phase
	fields["step"] = step
	if outcome != "" {
		fields["outcome"] = outcome
	}
	return fields
}

func cloneFields(fields map[string]any) map[string]any {
	clone := make(map[string]any, len(fields)+4)
	for key, value := range fields {
		clone[key] = value
	}
	return clone
}
