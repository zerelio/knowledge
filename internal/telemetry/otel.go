// Package telemetry provides structured logging, tracing, and metrics.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Providers bundles the OTel trace and metric providers.
type Providers struct {
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *metric.MeterProvider
	shutdown       []func(context.Context) error
}

// Shutdown flushes and closes all OTel providers.
func (p *Providers) Shutdown(ctx context.Context) {
	for _, fn := range p.shutdown {
		fn(ctx) //nolint:errcheck
	}
}

// Setup initialises OTel tracing + Prometheus metrics.
// If ZERELIO_OTEL_ENABLED != "true" it returns a no-op setup so the rest
// of the codebase doesn't need to branch.
func Setup(ctx context.Context, serviceName, version string, logger *slog.Logger) (*Providers, error) {
	enabled := os.Getenv("ZERELIO_OTEL_ENABLED") == "true"

	// ---- Tracing ----
	var traceExp sdktrace.SpanExporter
	if enabled {
		var err error
		traceExp, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, err
		}
		logger.Info("otel tracing enabled (stdout exporter)")
	} else {
		traceExp = tracetest.NewNoopExporter()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
	)

	// ---- Metrics (Prometheus) ----
	promExp, err := otelprometheus.New()
	if err != nil {
		return nil, err
	}
	mp := metric.NewMeterProvider(metric.WithReader(promExp))

	providers := &Providers{
		TracerProvider: tp,
		MeterProvider:  mp,
		shutdown: []func(context.Context) error{
			tp.Shutdown,
			mp.Shutdown,
		},
	}

	if enabled {
		logger.Info("otel metrics enabled (prometheus exporter at /metrics)")
	}
	return providers, nil
}

// InstrumentHandler wraps an http.Handler with OTel trace + metric instrumentation.
func InstrumentHandler(h http.Handler, operation string) http.Handler {
	return otelhttp.NewHandler(h, operation)
}

// MetricsHandler returns an http.Handler that serves Prometheus metrics.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
