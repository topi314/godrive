package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/topi314/godrive/godrive"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

func resources(cfg godrive.OtelConfig) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(Name),
		semconv.ServiceNamespace(Namespace),
		semconv.ServiceInstanceID(cfg.InstanceID),
		semconv.ServiceVersion(Version),
	)
}

func newTracer(cfg godrive.OtelConfig) (trace.Tracer, error) {
	if cfg.Trace == nil {
		return nil, nil
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Trace.Endpoint),
	}
	if cfg.Trace.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resources(cfg)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return otel.Tracer(Name), nil
}

func newMeter(cfg godrive.OtelConfig) (metric.Meter, error) {
	if cfg.Metrics == nil {
		return nil, nil
	}

	exp, err := prometheus.New()
	if err != nil {
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exp),
		sdkmetric.WithResource(resources(cfg)),
	)
	otel.SetMeterProvider(mp)

	go func() {
		server := &http.Server{
			Addr:    cfg.Metrics.ListenAddr,
			Handler: promhttp.Handler(),
		}
		if listenErr := server.ListenAndServe(); listenErr != nil && listenErr != http.ErrServerClosed {
			slog.Error("failed to listen metrics server", slog.Any("err", listenErr))
		}
	}()

	return mp.Meter(Name), nil
}
