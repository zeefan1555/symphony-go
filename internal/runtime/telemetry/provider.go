package telemetry

import (
	"context"
	"errors"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const defaultServiceName = "symphony-go"

type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	tracer         trace.Tracer
	meter          metric.Meter
	logger         otellog.Logger
}

func NewFromEnv(ctx context.Context) (Facade, error) {
	if !endpointConfigured() {
		return NewNoop(), nil
	}
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		serviceName = defaultServiceName
	}
	res := resource.NewWithAttributes("", attribute.String("service.name", serviceName))

	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		return nil, err
	}
	meterExporter, err := newMetricExporter(ctx)
	if err != nil {
		return nil, err
	}
	logExporter, err := newLogExporter(ctx)
	if err != nil {
		return nil, err
	}

	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(meterExporter)),
		sdkmetric.WithResource(res),
	)
	loggerProvider := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	otellogglobal.SetLoggerProvider(loggerProvider)

	return &Provider{
		tracerProvider: tracerProvider,
		meterProvider:  meterProvider,
		loggerProvider: loggerProvider,
		tracer:         tracerProvider.Tracer(scopeName),
		meter:          meterProvider.Meter(scopeName),
		logger:         loggerProvider.Logger(scopeName),
	}, nil
}

func (p *Provider) Enabled() bool {
	return p != nil && p.tracerProvider != nil && p.meterProvider != nil && p.loggerProvider != nil
}

func (p *Provider) Tracer() trace.Tracer {
	if p == nil || p.tracer == nil {
		return NewNoop().Tracer()
	}
	return p.tracer
}

func (p *Provider) Meter() metric.Meter {
	if p == nil || p.meter == nil {
		return NewNoop().Meter()
	}
	return p.meter
}

func (p *Provider) Logger() otellog.Logger {
	if p == nil || p.logger == nil {
		return NewNoop().Logger()
	}
	return p.logger
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var result error
	if p.tracerProvider != nil {
		result = errors.Join(result, p.tracerProvider.Shutdown(ctx))
	}
	if p.meterProvider != nil {
		result = errors.Join(result, p.meterProvider.Shutdown(ctx))
	}
	if p.loggerProvider != nil {
		result = errors.Join(result, p.loggerProvider.Shutdown(ctx))
	}
	return result
}

func endpointConfigured() bool {
	keys := []string{
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_METRICS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	}
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func protocol() string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
	if value == "" {
		return "grpc"
	}
	return value
}

func newTraceExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	if protocol() == "http/protobuf" {
		return otlptracehttp.New(ctx)
	}
	return otlptracegrpc.New(ctx)
}

func newMetricExporter(ctx context.Context) (sdkmetric.Exporter, error) {
	if protocol() == "http/protobuf" {
		return otlpmetrichttp.New(ctx)
	}
	return otlpmetricgrpc.New(ctx)
}

func newLogExporter(ctx context.Context) (sdklog.Exporter, error) {
	if protocol() == "http/protobuf" {
		return otlploghttp.New(ctx)
	}
	return otlploggrpc.New(ctx)
}
