package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// metricsAttrs holds metric attributes that handlers enrich during request processing.
// The Metrics middleware creates this in context; handlers populate it as they parse input.
type metricsAttrs struct {
	operation  string
	format     string
	inputBytes int64
}

// enrich sets the metrics attributes for the current request.
// It accepts inputBytes as int so callers can pass len() directly.
func (ma *metricsAttrs) enrich(operation, format string, inputBytes int) {
	ma.operation = operation
	ma.format = format
	ma.inputBytes = int64(inputBytes)
}

type metricsCtxKey struct{}

func withMetricsAttrs(ctx context.Context, ma *metricsAttrs) context.Context {
	return context.WithValue(ctx, metricsCtxKey{}, ma)
}

func getMetricsAttrs(ctx context.Context) *metricsAttrs {
	ma, _ := ctx.Value(metricsCtxKey{}).(*metricsAttrs)
	return ma
}

// instruments holds all OTel metric instruments for the application.
// Safe for concurrent use. When no MeterProvider is configured, all instruments
// are no-ops with zero overhead.
type instruments struct {
	// Tier 1: Web app metrics (recorded by middleware)
	requestDuration metric.Float64Histogram
	requestCount    metric.Int64Counter
	requestErrors   metric.Int64Counter
	inputSize       metric.Int64Histogram

	// Tier 2: Library metrics (recorded by handlers)
	packageDuration metric.Float64Histogram
	specPaths       metric.Int64Histogram
	specOperations  metric.Int64Histogram
	specSchemas     metric.Int64Histogram
	specInputBytes  metric.Int64Histogram
}

func newInstruments() *instruments {
	meter := otel.Meter("oastools-web")

	// Tier 1 instruments
	requestDuration, _ := meter.Float64Histogram("oastools.operation.duration",
		metric.WithDescription("Total request duration for API operations"),
		metric.WithUnit("s"))
	requestCount, _ := meter.Int64Counter("oastools.operation.count",
		metric.WithDescription("Total operations processed"))
	requestErrors, _ := meter.Int64Counter("oastools.operation.errors",
		metric.WithDescription("Error count by type"))
	inputSize, _ := meter.Int64Histogram("oastools.operation.input_size",
		metric.WithDescription("Input file size in bytes"),
		metric.WithUnit("By"))

	// Tier 2 instruments
	packageDuration, _ := meter.Float64Histogram("oastools.package.duration",
		metric.WithDescription("Time spent inside an oastools package call"),
		metric.WithUnit("s"))
	specPaths, _ := meter.Int64Histogram("oastools.spec.paths",
		metric.WithDescription("Path count from parsed spec"))
	specOperations, _ := meter.Int64Histogram("oastools.spec.operations",
		metric.WithDescription("Operation count from parsed spec"))
	specSchemas, _ := meter.Int64Histogram("oastools.spec.schemas",
		metric.WithDescription("Schema count from parsed spec"))
	specInputBytes, _ := meter.Int64Histogram("oastools.spec.input_bytes",
		metric.WithDescription("Raw input size in bytes"),
		metric.WithUnit("By"))

	return &instruments{
		requestDuration: requestDuration,
		requestCount:    requestCount,
		requestErrors:   requestErrors,
		inputSize:       inputSize,
		packageDuration: packageDuration,
		specPaths:       specPaths,
		specOperations:  specOperations,
		specSchemas:     specSchemas,
		specInputBytes:  specInputBytes,
	}
}

// Metrics returns middleware that records Tier 1 web app metrics.
// It skips static file and health check paths.
func Metrics(inst *instruments) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip static files and health checks
			if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Create metrics context for handler enrichment
			ma := &metricsAttrs{}
			r = r.WithContext(withMetricsAttrs(r.Context(), ma))

			// Wrap response writer to capture status
			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			// Derive attributes after handler returns
			duration := time.Since(start).Seconds()
			operation := ma.operation
			if operation == "" {
				operation = operationFromPath(r.URL.Path)
			}

			status := "success"
			if wrapped.status >= 400 {
				status = "error"
			}

			source := "api"
			if r.Header.Get("HX-Request") != "" {
				source = "ui"
			}

			// Record Tier 1 metrics
			attrs := metric.WithAttributes(
				attribute.String("operation", operation),
				attribute.String("status", status),
				attribute.String("source", source),
			)

			inst.requestDuration.Record(r.Context(), duration,
				attrs,
				metric.WithAttributes(attribute.String("format", ma.format)),
			)
			inst.requestCount.Add(r.Context(), 1, attrs)

			if wrapped.status >= 400 {
				inst.requestErrors.Add(r.Context(), 1,
					metric.WithAttributes(
						attribute.String("operation", operation),
						attribute.Int("error_code", wrapped.status),
						attribute.String("source", source),
					),
				)
			}

			if ma.inputBytes > 0 {
				inst.inputSize.Record(r.Context(), ma.inputBytes,
					metric.WithAttributes(attribute.String("operation", operation)),
				)
			}
		})
	}
}

// operationFromPath extracts the operation name from a URL path.
// e.g., "/api/validate" -> "validate", "/api/explore/operations" -> "explore"
func operationFromPath(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/")
	if trimmed == path {
		return "unknown"
	}
	if idx := strings.Index(trimmed, "/"); idx != -1 {
		trimmed = trimmed[:idx]
	}
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

// recordPackageDuration records how long an oastools package call took.
func (inst *instruments) recordPackageDuration(ctx context.Context, pkg string, start time.Time) {
	ma := getMetricsAttrs(ctx)
	attrs := []attribute.KeyValue{
		attribute.String("package", pkg),
	}
	if ma != nil && ma.format != "" {
		attrs = append(attrs, attribute.String("format", ma.format))
	}
	inst.packageDuration.Record(ctx, time.Since(start).Seconds(),
		metric.WithAttributes(attrs...))
}

// recordSpecComplexity records spec complexity metrics after a successful parse.
func (inst *instruments) recordSpecComplexity(ctx context.Context, pkg, specVersion string, pathCount, opCount, schemaCount, inputBytes int) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("package", pkg),
		attribute.String("spec_version", specVersion),
	}

	inst.specPaths.Record(ctx, int64(pathCount),
		metric.WithAttributes(baseAttrs...))
	inst.specOperations.Record(ctx, int64(opCount),
		metric.WithAttributes(baseAttrs...))
	inst.specSchemas.Record(ctx, int64(schemaCount),
		metric.WithAttributes(baseAttrs...))

	if inputBytes > 0 {
		ma := getMetricsAttrs(ctx)
		format := ""
		if ma != nil {
			format = ma.format
		}
		inst.specInputBytes.Record(ctx, int64(inputBytes),
			metric.WithAttributes(
				attribute.String("package", pkg),
				attribute.String("format", format),
			))
	}
}
