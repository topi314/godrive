package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/topi314/godrive/server/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// Provider holds OpenTelemetry providers and an optional Prometheus scrape server.
type Provider struct {
	tp            *sdktrace.TracerProvider
	mp            *metric.MeterProvider
	metricsServer *http.Server
}

// Setup configures global TracerProvider / MeterProvider from [otel] config.
// No-op (nil, nil) when otel is disabled.
func Setup(ctx context.Context, cfg *config.OtelConfig, version string) (*Provider, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	instanceID := strings.TrimSpace(cfg.InstanceID)
	if instanceID == "" {
		instanceID = "godrive"
	}
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("godrive"),
			semconv.ServiceVersion(version),
			semconv.ServiceInstanceID(instanceID),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	p := &Provider{}

	if cfg.Trace != nil && strings.TrimSpace(cfg.Trace.Endpoint) != "" {
		tp, err := newTracerProvider(ctx, res, cfg.Trace)
		if err != nil {
			return nil, err
		}
		p.tp = tp
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		slog.Info("otel tracing enabled", slog.String("endpoint", cfg.Trace.Endpoint))
	}

	if cfg.Metrics != nil && strings.TrimSpace(cfg.Metrics.ListenAddr) != "" {
		mp, metricsServer, err := newMeterProvider(res, cfg.Metrics.ListenAddr)
		if err != nil {
			_ = p.Shutdown(context.Background())
			return nil, err
		}
		p.mp = mp
		p.metricsServer = metricsServer
		otel.SetMeterProvider(mp)
		slog.Info("otel metrics enabled", slog.String("listen_addr", cfg.Metrics.ListenAddr))
	}

	if p.tp == nil && p.mp == nil {
		slog.Warn("otel enabled but neither [otel.trace] nor [otel.metrics] is configured")
	}
	return p, nil
}

func newTracerProvider(ctx context.Context, res *resource.Resource, cfg *config.TraceConfig) (*sdktrace.TracerProvider, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(otlpHostPort(cfg.Endpoint)),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exp, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otel trace exporter: %w", err)
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	), nil
}

func newMeterProvider(res *resource.Resource, listenAddr string) (*metric.MeterProvider, *http.Server, error) {
	exp, err := prometheus.New()
	if err != nil {
		return nil, nil, fmt.Errorf("otel prometheus exporter: %w", err)
	}
	mp := metric.NewMeterProvider(
		metric.WithReader(exp),
		metric.WithResource(res),
	)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("otel metrics server failed", slog.Any("err", err))
		}
	}()
	return mp, srv, nil
}

// otlpHostPort strips an optional http(s):// scheme for the gRPC target.
func otlpHostPort(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if strings.Contains(endpoint, "://") {
		if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return endpoint
}

// WrapHTTP adds OpenTelemetry HTTP server instrumentation when providers are active.
func (p *Provider) WrapHTTP(handler http.Handler) http.Handler {
	if p == nil || (p.tp == nil && p.mp == nil) {
		return handler
	}
	return otelhttp.NewHandler(handler, "godrive",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}

// Shutdown flushes exporters and stops the metrics scrape server.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	var errs []error
	if p.metricsServer != nil {
		errs = append(errs, p.metricsServer.Shutdown(ctx))
	}
	if p.mp != nil {
		errs = append(errs, p.mp.Shutdown(ctx))
	}
	if p.tp != nil {
		errs = append(errs, p.tp.Shutdown(ctx))
	}
	return errors.Join(errs...)
}
