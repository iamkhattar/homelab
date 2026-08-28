// Package observability configures Butler's OpenTelemetry providers. An empty
// OTLP endpoint keeps local tests deterministic while preserving no-op API
// instrumentation.
package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.39.0"
)

type Shutdown func(context.Context) error

func Setup(ctx context.Context, version string) (Shutdown, error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	resource, err := sdkresource.New(ctx, sdkresource.WithAttributes(
		semconv.ServiceName("butler"),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		return nil, fmt.Errorf("creating OpenTelemetry resource: %w", err)
	}
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, fmt.Errorf("creating OTLP trace exporter: %w", err)
	}
	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(endpoint))
	if err != nil {
		return nil, fmt.Errorf("creating OTLP metric exporter: %w", err)
	}
	traces := sdktrace.NewTracerProvider(sdktrace.WithBatcher(traceExporter), sdktrace.WithResource(resource))
	metrics := metric.NewMeterProvider(metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(30*time.Second))), metric.WithResource(resource))
	otel.SetTracerProvider(traces)
	otel.SetMeterProvider(metrics)
	return func(ctx context.Context) error {
		return errors.Join(metrics.Shutdown(ctx), traces.Shutdown(ctx))
	}, nil
}
