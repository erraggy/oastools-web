# OpenTelemetry Metrics Design

## Goal

Add operational and library-level performance metrics to oastools-web using the OpenTelemetry Go SDK, exported directly to Google Cloud Monitoring via the OTLP gRPC endpoint (`telemetry.googleapis.com`).

Two tiers of metrics:
1. **Web app metrics** -- per-request operation counts, durations, error rates
2. **Library metrics** -- oastools phase timing (parse/operate/serialize) and spec complexity

## Metrics Inventory

### Tier 1: Web App Metrics (middleware-level)

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oastools.operation.duration` | Histogram | `operation`, `status`, `format`, `source` | Total request duration for API operations |
| `oastools.operation.count` | Counter | `operation`, `status`, `source` | Total operations processed |
| `oastools.operation.errors` | Counter | `operation`, `error_code`, `source` | Error count by type |
| `oastools.operation.input_size` | Histogram | `operation` | Input file size in bytes |

### Tier 2: Library Metrics (handler-level)

Library metrics use a `package` label whose values match the oastools Go package names exactly,
making it trivial to map a metric back to its source package.

**Package-to-function mapping:**

| `package` label | Go import | Function timed |
|---|---|---|
| `parser` | `github.com/erraggy/oastools/parser` | `parser.ParseWithOptions` |
| `validator` | `github.com/erraggy/oastools/validator` | `validator.ValidateParsed` |
| `converter` | `github.com/erraggy/oastools/converter` | `converter.ConvertWithOptions` |
| `differ` | `github.com/erraggy/oastools/differ` | `differ.DiffParsed` |
| `fixer` | `github.com/erraggy/oastools/fixer` | `fixer.FixParsed` |
| `joiner` | `github.com/erraggy/oastools/joiner` | `joiner.JoinParsed` |
| `overlay` | `github.com/erraggy/oastools/overlay` | `overlay.ApplyParsed` |
| `walker` | `github.com/erraggy/oastools/walker` | `walker.*` (explore) |

**Phase timing (histograms):**

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oastools.package.duration` | Histogram | `package`, `spec_version`, `format` | Time spent inside an oastools package call |
| `oastools.serialize.duration` | Histogram | `format` | Time in `serializeDocument` (web-app code, not oastools) |

The single `oastools.package.duration` metric with the `package` label replaces separate
per-phase metrics. This lets you query all packages on one chart, filter to a single package,
or compare `parser` vs `validator` durations side-by-side.

Example queries in Cloud Monitoring:
- **Parse time across all operations:** `package="parser"`
- **Validator vs fixer performance:** `package=~"validator|fixer"`
- **All oastools time for a given spec version:** `spec_version="3.1"`

**Spec complexity (histograms recorded per-request):**

| Metric | Type | Labels | Description |
|---|---|---|---|
| `oastools.spec.paths` | Histogram | `package`, `spec_version` | Path count from parsed spec |
| `oastools.spec.operations` | Histogram | `package`, `spec_version` | Operation count |
| `oastools.spec.schemas` | Histogram | `package`, `spec_version` | Schema count |
| `oastools.spec.input_bytes` | Histogram | `package`, `format` | Raw input size |

Spec complexity metrics also use the `package` label so you can correlate
"validator is slow when schemas > 200" directly.

### Common Labels

| Label | Values | Description |
|---|---|---|
| `operation` | validate, convert, diff, fix, join, overlay, explore | Web API operation name (Tier 1 only) |
| `package` | parser, validator, converter, differ, fixer, joiner, overlay, walker | oastools Go package name (Tier 2 only) |
| `status` | success, error, timeout | Operation outcome |
| `format` | json, yaml | Spec file format |
| `source` | ui, api | Request origin (HTMX vs direct API) |
| `spec_version` | 2.0, 3.0, 3.1, 3.2 | Detected OpenAPI version |

### Label Design: `operation` vs `package`

Tier 1 metrics use `operation` (the web endpoint name: "validate", "fix", etc.).
Tier 2 metrics use `package` (the oastools Go package: "validator", "fixer", etc.).

These are deliberately kept separate to maintain a clean boundary:
- `operation` = "what did the user ask the web app to do?"
- `package` = "which oastools package did the work?"

This matters because some operations invoke multiple packages (e.g., `convert` calls
both `parser` and `converter`, and optionally `overlay`). The `package` label on Tier 2
metrics accurately attributes time to the specific library component.

## Architecture

### Instrumentation Approach

**Middleware + per-handler enrichment:**

- A new `Metrics` middleware in `middleware.go` handles Tier 1 timing, counting, and error tracking for every `/api/*` request
- It derives `operation` from the URL path, `status` from the response code, and `source` from the `HX-Request` header
- Individual handlers record Tier 2 metrics inline by wrapping oastools calls with timing and recording spec complexity from `ParseResult`

### Middleware Chain Position

```
Logging → Metrics → Recovery → RateLimit → Concurrency → Timeout → SizeLimit → route
```

`Metrics` sits just inside `Logging` so it captures the full processing duration (including recovery), while `Logging` continues to log everything as before.

### Context-Based Enrichment

Handlers write additional metric attributes (format, input_size) to the request context. The `Metrics` middleware reads these after the handler returns to enrich the recorded metrics.

```go
// In handler:
setMetricAttr(r, "format", format)
setMetricAttr(r, "input_size", len(input.Content))

// In Metrics middleware (after handler returns):
attrs := getMetricAttrs(r)
```

## Code Layout

| File | Change |
|---|---|
| `internal/api/metrics.go` | **New** -- OTel meter, metric instruments, `Metrics()` middleware, context helpers |
| `internal/api/handler.go` | Add `Metrics` to `buildMiddlewareChain` |
| `internal/config/config.go` | Add `MetricsEnabled` bool, `OTLPEndpoint` string |
| `cmd/server/main.go` | Initialize OTel `MeterProvider`, defer shutdown |
| `go.mod` | Add OTel SDK dependencies |
| Handlers: `validate.go`, `convert.go`, `diff.go`, `fix.go`, `join.go`, `overlay.go`, `explore.go` | Wrap oastools calls with `recordPackageDuration(ctx, "parser", func() { ... })` pattern; record spec complexity |

## OTel Setup

### Provider Initialization (`cmd/server/main.go`)

- Create OTLP gRPC exporter targeting `telemetry.googleapis.com:443`
- Create `MeterProvider` with `PeriodicReader` (60s export interval)
- Set as global meter provider via `otel.SetMeterProvider`
- Shutdown in graceful shutdown path

### Meter Initialization (`internal/api/metrics.go`)

- Get a `Meter` named `oastools-web` from the global provider
- Create all instruments (histograms, counters) at initialization time
- Instruments are safe for concurrent use

### Configuration (`internal/config/config.go`)

| Env Var | Default | Description |
|---|---|---|
| `METRICS_ENABLED` | `true` | Enable/disable metrics export |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `telemetry.googleapis.com:443` | OTLP gRPC endpoint |

### Local Development

When `METRICS_ENABLED=false`, no exporter is created. OTel instruments become no-ops via the default noop provider, adding zero overhead.

## Dependencies

New Go modules:
- `go.opentelemetry.io/otel`
- `go.opentelemetry.io/otel/metric`
- `go.opentelemetry.io/otel/sdk`
- `go.opentelemetry.io/otel/sdk/metric`
- `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc`
- `google.golang.org/grpc` (transitive)

## Testing Strategy

**Unit tested:**
- `Metrics` middleware -- uses OTel's `sdkmetric.NewManualReader` with in-memory assertions to verify correct labels, durations, and counts
- Context helpers (`setMetricAttr` / `getMetricAttrs`)

**Covered by existing tests:**
- Handler integration via `handlers_test.go` and `golden_test.go` -- confirms the new middleware doesn't break the request chain (runs with noop exporter in tests)

**Verified by deployment:**
- Deploy to Cloud Run, confirm metrics appear in Cloud Monitoring Metrics Explorer under `oastools.operation.*` and `oastools.parse.*` namespaces

## GCP Pricing Impact

- Cloud Run system metrics: free (already available, no code needed)
- Custom metrics: ~11 metric descriptors, well within the 150 MiB/month free ingestion tier for a low-traffic tool
- No sidecar container costs -- direct OTLP export from the application
