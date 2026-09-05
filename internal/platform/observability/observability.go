// Package observability owns the process-wide OpenTelemetry providers.
package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Provider struct {
	traces  *sdktrace.TracerProvider
	metrics *sdkmetric.MeterProvider
}

// Init is intentionally a no-op until an OTLP endpoint is configured.
func Init(ctx context.Context, serviceName, version, environment string) (*Provider, error) {
	if strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "true") || !configured() {
		return &Provider{}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("deployment.environment.name", environment),
		),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}
	metricExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	p := &Provider{
		traces: sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		),
		metrics: sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
			sdkmetric.WithResource(res),
		),
	}
	otel.SetTracerProvider(p.traces)
	otel.SetMeterProvider(p.metrics)
	if err := otelruntime.Start(otelruntime.WithMeterProvider(p.metrics)); err != nil {
		_ = p.Shutdown(ctx)
		return nil, fmt.Errorf("start runtime metrics: %w", err)
	}
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return p, nil
}

func configured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != ""
}

func (p *Provider) Shutdown(ctx context.Context) error {
	var errs []error
	if p.metrics != nil {
		errs = append(errs, p.metrics.Shutdown(ctx))
	}
	if p.traces != nil {
		errs = append(errs, p.traces.Shutdown(ctx))
	}
	return errors.Join(errs...)
}
