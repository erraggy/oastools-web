package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erraggy/oastools-web/internal/api"
	"github.com/erraggy/oastools-web/internal/config"

	"cloud.google.com/go/compute/metadata"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"
)

var version = "dev"

func main() {
	cfg := config.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	// MeterProvider must be set before NewHandler, which calls otel.Meter()
	// to create instruments. When disabled, the default no-op provider is used.
	if cfg.MetricsEnabled {
		mp, err := initMeterProvider(context.Background(), version)
		if err != nil {
			slog.Warn("metrics disabled: failed to init meter provider", "error", err)
		} else {
			otel.SetMeterProvider(mp)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := mp.Shutdown(ctx); err != nil {
					slog.Error("meter provider shutdown error", "error", err)
				}
			}()
			slog.Info("metrics enabled")
		}
	}

	handler, err := api.NewHandler(cfg, version)
	if err != nil {
		slog.Error("failed to create handler", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		slog.Info("shutting down server")
		handler.Stop() // Stop background goroutines
		if err := server.Shutdown(ctx); err != nil {
			slog.Error("shutdown error", "error", err)
		}
	}()

	slog.Info("starting server", "port", cfg.Port, "version", version)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// initMeterProvider creates an OTel MeterProvider that exports to Google Cloud
// Monitoring via OTLP gRPC. ADC credentials are attached via per-RPC gRPC credentials.
func initMeterProvider(ctx context.Context, appVersion string) (*metric.MeterProvider, error) {
	creds, err := oauth.NewApplicationDefault(ctx, "https://www.googleapis.com/auth/monitoring.write")
	if err != nil {
		return nil, fmt.Errorf("creating GCP credentials: %w", err)
	}

	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpointURL("https://telemetry.googleapis.com:443"),
		otlpmetricgrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName("oastools-web"),
		semconv.ServiceVersion(appVersion),
	}
	// telemetry.googleapis.com requires gcp.project_id in the OTel resource.
	// OnGCE() probes the metadata server on first call (has its own internal timeout).
	if metadata.OnGCE() {
		projectID, err := metadata.ProjectIDWithContext(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetching GCP project ID from metadata: %w", err)
		}
		attrs = append(attrs, attribute.String("gcp.project_id", projectID))
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(attrs...),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(exporter,
			metric.WithInterval(60*time.Second),
		)),
	)

	return mp, nil
}
