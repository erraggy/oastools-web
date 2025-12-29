# Implementation Phases

## Overview

This document breaks the implementation into discrete phases, each with clear objectives, deliverables, and acceptance criteria. The phases are designed to be completed in focused development sessions, with each phase producing a working increment of the application.

## Phase 1: Project Foundation

### Objective

Establish the repository structure, build system, and basic application skeleton. This phase produces a deployable (though minimal) application that serves static pages.

### Deliverables

The first deliverable is the **repository structure** with the directory layout defined in the overview document, including `cmd/server/`, `internal/`, `static/`, and configuration files.

The second deliverable is the **build system** consisting of a Makefile with targets for build, test, lint, and run, plus a Dockerfile for container builds.

The third deliverable is the **application skeleton** with a main.go that initializes the server, basic configuration loading from environment variables, health check endpoint, and static file serving.

The fourth deliverable is the **template infrastructure** with the base template, landing page, and embedded template loading.

### Implementation Details

**cmd/server/main.go**

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/erraggy/oastools-web/internal/api"
    "github.com/erraggy/oastools-web/internal/config"
)

var version = "dev"

func main() {
    cfg := config.Load()

    logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: cfg.LogLevel,
    }))
    slog.SetDefault(logger)

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
        server.Shutdown(ctx)
    }()

    slog.Info("starting server", "port", cfg.Port, "version", version)
    if err := server.ListenAndServe(); err != http.ErrServerClosed {
        slog.Error("server error", "error", err)
        os.Exit(1)
    }
}
```

**internal/config/config.go**

```go
package config

import (
    "log/slog"
    "os"
    "strconv"
    "time"
)

type Config struct {
    Port           string
    LogLevel       slog.Level
    RateLimitRPM   int
    MaxFileSize    int64
    RequestTimeout time.Duration
}

func Load() *Config {
    return &Config{
        Port:           getEnv("PORT", "8080"),
        LogLevel:       parseLogLevel(getEnv("LOG_LEVEL", "info")),
        RateLimitRPM:   getEnvInt("RATE_LIMIT_RPM", 10),
        MaxFileSize:    getEnvInt64("MAX_FILE_SIZE", 2<<20), // 2MB
        RequestTimeout: getEnvDuration("REQUEST_TIMEOUT", 30*time.Second),
    }
}

func getEnv(key, defaultValue string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.Atoi(v); err == nil {
            return i
        }
    }
    return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
    if v := os.Getenv(key); v != "" {
        if i, err := strconv.ParseInt(v, 10, 64); err == nil {
            return i
        }
    }
    return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
    if v := os.Getenv(key); v != "" {
        if d, err := time.ParseDuration(v); err == nil {
            return d
        }
    }
    return defaultValue
}

func parseLogLevel(s string) slog.Level {
    switch s {
    case "debug":
        return slog.LevelDebug
    case "warn":
        return slog.LevelWarn
    case "error":
        return slog.LevelError
    default:
        return slog.LevelInfo
    }
}
```

### Acceptance Criteria

1. `make build` produces a working binary
2. `make test` runs and passes (skeleton tests)
3. `make lint` passes with no errors
4. `docker build .` produces a valid image
5. Running the application serves the landing page at `/`
6. `/health` returns 200 OK with version information
7. Static files at `/static/*` are served correctly

## Phase 2: Middleware Stack

### Objective

Implement the middleware components that provide security and observability: rate limiting, request size limiting, timeouts, recovery, and logging.

### Deliverables

The **rate limiter** uses `golang.org/x/time/rate` with per-IP token buckets, storing limiters in a sync.Map with periodic cleanup of stale entries.

The **request size limiter** wraps request bodies with `http.MaxBytesReader` to enforce upload limits before processing.

The **timeout handler** uses `http.TimeoutHandler` to enforce per-request processing limits.

The **recovery middleware** catches panics and converts them to 500 responses while logging the stack trace.

The **logging middleware** records request method, path, status, and duration using structured logging.

### Implementation Details

**internal/api/middleware.go**

```go
package api

import (
    "log/slog"
    "net/http"
    "sync"
    "time"

    "golang.org/x/time/rate"
)

// RateLimiter implements per-IP rate limiting
type RateLimiter struct {
    visitors map[string]*visitorInfo
    mu       sync.Mutex
    rate     rate.Limit
    burst    int
}

type visitorInfo struct {
    limiter  *rate.Limiter
    lastSeen time.Time
}

func NewRateLimiter(rpm, burst int) *RateLimiter {
    rl := &RateLimiter{
        visitors: make(map[string]*visitorInfo),
        rate:     rate.Limit(float64(rpm) / 60.0),
        burst:    burst,
    }
    go rl.cleanup()
    return rl
}

func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    v, exists := rl.visitors[ip]
    if !exists {
        limiter := rate.NewLimiter(rl.rate, rl.burst)
        rl.visitors[ip] = &visitorInfo{limiter: limiter, lastSeen: time.Now()}
        return limiter
    }

    v.lastSeen = time.Now()
    return v.limiter
}

func (rl *RateLimiter) cleanup() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        rl.mu.Lock()
        for ip, v := range rl.visitors {
            if time.Since(v.lastSeen) > 3*time.Minute {
                delete(rl.visitors, ip)
            }
        }
        rl.mu.Unlock()
    }
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := realIP(r)
        limiter := rl.getVisitor(ip)

        if !limiter.Allow() {
            slog.Warn("rate limit exceeded", "ip", ip)
            w.Header().Set("Retry-After", "60")
            http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
            return
        }

        next.ServeHTTP(w, r)
    })
}

func realIP(r *http.Request) string {
    // Cloud Run sets X-Forwarded-For
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        return xff
    }
    return r.RemoteAddr
}

// RequestSizeLimiter enforces maximum request body size
func RequestSizeLimiter(maxSize int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            r.Body = http.MaxBytesReader(w, r.Body, maxSize)
            next.ServeHTTP(w, r)
        })
    }
}

// Recovery catches panics and returns 500
func Recovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if err := recover(); err != nil {
                slog.Error("panic recovered", "error", err, "path", r.URL.Path)
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
}

// Logging logs request details
func Logging(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        wrapped := &responseWriter{ResponseWriter: w, status: 200}

        next.ServeHTTP(wrapped, r)

        slog.Info("request",
            "method", r.Method,
            "path", r.URL.Path,
            "status", wrapped.status,
            "duration", time.Since(start),
        )
    })
}

type responseWriter struct {
    http.ResponseWriter
    status int
}

func (w *responseWriter) WriteHeader(status int) {
    w.status = status
    w.ResponseWriter.WriteHeader(status)
}
```

### Acceptance Criteria

1. Rapid requests from same IP receive 429 after burst capacity
2. Requests exceeding size limit receive 413
3. Requests exceeding timeout receive 504
4. Panics in handlers return 500 (not crash server)
5. All requests are logged with method, path, status, duration
6. Rate limiter cleanup removes stale entries

## Phase 3: Validate Operation

### Objective

Implement the first API operation: specification validation. This phase establishes the pattern for all subsequent operations.

### Deliverables

The **validate handler** parses uploaded files, invokes the oastools validator, and returns structured results.

The **validation result template** displays errors, warnings, and statistics.

The **validate page** provides the file upload form with HTMX integration.

### Implementation Details

**internal/api/handlers.go** (validate handler)

```go
package api

import (
    "encoding/json"
    "html/template"
    "net/http"

    "github.com/erraggy/oastools/parser"
    "github.com/erraggy/oastools/validator"
)

type ValidationResult struct {
    Valid      bool                `json:"valid"`
    Version    string              `json:"version"`
    Errors     []ValidationIssue   `json:"errors"`
    Warnings   []ValidationIssue   `json:"warnings"`
    Statistics ValidationStats     `json:"statistics"`
}

type ValidationIssue struct {
    Path     string `json:"path"`
    Message  string `json:"message"`
    Severity string `json:"severity"`
}

type ValidationStats struct {
    Paths      int `json:"paths"`
    Operations int `json:"operations"`
    Schemas    int `json:"schemas"`
    Errors     int `json:"errors"`
    Warnings   int `json:"warnings"`
}

func (h *Handler) handleValidate(w http.ResponseWriter, r *http.Request) {
    // Parse multipart form
    if err := r.ParseMultipartForm(h.cfg.MaxFileSize); err != nil {
        h.renderError(w, r, http.StatusBadRequest, "failed to parse form", err)
        return
    }

    // Get uploaded file
    file, header, err := r.FormFile("spec")
    if err != nil {
        h.renderError(w, r, http.StatusBadRequest, "spec file required", err)
        return
    }
    defer file.Close()

    // Read file content
    content := make([]byte, header.Size)
    if _, err := file.Read(content); err != nil {
        h.renderError(w, r, http.StatusBadRequest, "failed to read file", err)
        return
    }

    // Parse specification
    parseResult, err := parser.Parse(content,
        parser.WithFilePath(header.Filename),
    )
    if err != nil {
        h.renderError(w, r, http.StatusBadRequest, "failed to parse specification", err)
        return
    }

    // Validate using parse-once pattern
    v := validator.New()
    validationResult := v.ValidateParsed(parseResult)

    // Build response
    result := ValidationResult{
        Valid:   len(validationResult.Errors) == 0,
        Version: parseResult.Version.String(),
    }

    for _, e := range validationResult.Errors {
        result.Errors = append(result.Errors, ValidationIssue{
            Path:     e.Path,
            Message:  e.Message,
            Severity: "error",
        })
    }

    for _, w := range validationResult.Warnings {
        result.Warnings = append(result.Warnings, ValidationIssue{
            Path:     w.Path,
            Message:  w.Message,
            Severity: "warning",
        })
    }

    result.Statistics = h.computeStats(parseResult, result)

    h.renderResult(w, r, "validation-result", result)
}

func (h *Handler) renderResult(w http.ResponseWriter, r *http.Request, templateName string, data any) {
    // Check if HTMX request (partial) or full page
    if r.Header.Get("HX-Request") == "true" {
        h.templates.ExecuteTemplate(w, templateName, data)
        return
    }

    // Check Accept header for JSON
    if r.Header.Get("Accept") == "application/json" {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(data)
        return
    }

    // Full page render
    h.templates.ExecuteTemplate(w, templateName, data)
}
```

### Acceptance Criteria

1. Valid specification returns validation result with valid=true
2. Invalid specification returns errors with JSONPath locations
3. Both JSON and YAML specifications are accepted
4. HTMX requests receive partial HTML response
5. JSON Accept header receives JSON response
6. Statistics are computed and returned
7. Parse errors return appropriate error messages

## Phase 4: Convert Operation

### Objective

Implement specification conversion between OpenAPI versions.

### Deliverables

The **convert handler** parses uploaded files, invokes the oastools converter, and returns the converted specification with any conversion issues.

The **conversion result template** displays the converted specification with download/copy functionality.

The **convert page** provides the file upload form with target version selector.

### Implementation Details

The convert handler follows the same pattern as validate but calls `converter.ConvertParsed()` instead of validating. The handler accepts a `target` form field specifying the target version (2.0, 3.0, 3.1, or 3.2).

```go
func (h *Handler) handleConvert(w http.ResponseWriter, r *http.Request) {
    // Parse form and file (similar to validate)
    // ...

    targetVersion := r.FormValue("target")
    target, err := parseTargetVersion(targetVersion)
    if err != nil {
        h.renderError(w, r, http.StatusBadRequest, "invalid target version", err)
        return
    }

    // Convert using parse-once pattern
    c := converter.New()
    convertResult, err := c.ConvertParsed(parseResult, target)
    if err != nil {
        h.renderError(w, r, http.StatusUnprocessableEntity, "conversion failed", err)
        return
    }

    // Serialize result in original format
    var output []byte
    format := detectFormat(content)
    if format == "json" {
        output, _ = json.MarshalIndent(convertResult.Document, "", "  ")
    } else {
        output, _ = yaml.Marshal(convertResult.Document)
    }

    result := ConversionResult{
        SourceVersion: parseResult.Version.String(),
        TargetVersion: target.String(),
        Issues:        mapConversionIssues(convertResult.Issues),
        Result:        string(output),
        Format:        format,
    }

    h.renderResult(w, r, "conversion-result", result)
}
```

### Acceptance Criteria

1. Swagger 2.0 converts to OpenAPI 3.x successfully
2. OpenAPI 3.x converts between minor versions
3. Conversion issues are captured and returned
4. Output format matches input format (JSON or YAML)
5. Download and copy buttons function correctly
6. Invalid target versions return appropriate errors

## Phase 5: Fix Operation

### Objective

Implement automatic specification fixing.

### Deliverables

The **fix handler** parses uploaded files, invokes the oastools fixer, and returns the fixed specification with a list of applied fixes.

The **fix result template** displays the list of fixes and the corrected specification.

The **fix page** provides the file upload form with optional fix configuration.

### Implementation Details

The fix handler follows the established pattern but calls `fixer.FixParsed()`. Optional form fields control which fixes to apply.

### Acceptance Criteria

1. Invalid references are corrected
2. Unused schemas are optionally removed
3. Applied fixes are listed with descriptions
4. Fixed specification validates without the original errors
5. Fix options can be toggled via form fields

## Phase 6: Diff Operation

### Objective

Implement specification comparison.

### Deliverables

The **diff handler** parses two uploaded files, invokes the oastools differ, and returns a structured diff.

The **diff result template** displays changes in a tabular format with breaking change indicators.

The **diff page** provides two file upload fields.

### Implementation Details

The diff handler parses both files and calls `differ.DiffParsed()`.

```go
func (h *Handler) handleDiff(w http.ResponseWriter, r *http.Request) {
    // Parse both files
    baseFile, _, _ := r.FormFile("base")
    headFile, _, _ := r.FormFile("head")
    
    baseResult, _ := parser.Parse(baseContent)
    headResult, _ := parser.Parse(headContent)

    // Diff using parse-once pattern
    d := differ.New()
    diffResult := d.DiffParsed(baseResult, headResult)

    // Map to response structure
    result := DiffResult{
        Summary: DiffSummary{
            Additions:     len(diffResult.Additions),
            Deletions:     len(diffResult.Deletions),
            Modifications: len(diffResult.Modifications),
            Breaking:      countBreaking(diffResult),
        },
        Changes: mapDiffChanges(diffResult),
    }

    h.renderResult(w, r, "diff-result", result)
}
```

### Acceptance Criteria

1. Added paths/operations are detected
2. Removed paths/operations are detected
3. Modified schemas/parameters are detected
4. Breaking changes are flagged
5. Summary statistics are accurate
6. Changes display clear before/after when applicable

## Phase 7: Join Operation

### Objective

Implement specification merging.

### Deliverables

The **join handler** parses multiple uploaded files, invokes the oastools joiner, and returns the merged specification with collision information.

The **join result template** displays collision resolutions and the merged specification.

The **join page** supports 2-5 file uploads with dynamic file addition.

### Implementation Details

The join handler handles multiple file inputs and collision strategy selection.

```go
func (h *Handler) handleJoin(w http.ResponseWriter, r *http.Request) {
    // Parse multiple files
    r.ParseMultipartForm(h.cfg.MaxFileSize * 5)
    files := r.MultipartForm.File["spec[]"]

    if len(files) < 2 || len(files) > 5 {
        h.renderError(w, r, http.StatusBadRequest, "2-5 files required", nil)
        return
    }

    // Parse all specifications
    var parseResults []*parser.ParseResult
    for _, fh := range files {
        file, _ := fh.Open()
        content, _ := io.ReadAll(file)
        result, _ := parser.Parse(content, parser.WithFilePath(fh.Filename))
        parseResults = append(parseResults, result)
        file.Close()
    }

    // Join using parse-once pattern
    j := joiner.New()
    strategy := parseCollisionStrategy(r.FormValue("collisionStrategy"))
    joinResult, err := j.JoinParsed(parseResults, joiner.WithCollisionStrategy(strategy))
    
    // ... build and return result
}
```

### Acceptance Criteria

1. Two specifications merge successfully
2. Up to five specifications merge successfully
3. Collisions are detected and reported
4. Collision strategies (rename, first, error) work correctly
5. Merged specification is valid
6. Dynamic file input addition functions correctly

## Phase 8: Overlay Support

### Objective

Implement OpenAPI Overlay application.

### Deliverables

The **overlay handler** parses a specification and overlay file, invokes the oastools overlay package, and returns the modified specification.

The **overlay result template** displays applied actions and the modified specification.

The **overlay page** provides two file upload fields (spec and overlay).

### Acceptance Criteria

1. Update actions modify target paths
2. Remove actions delete target paths
3. Multiple actions apply in sequence
4. Invalid overlay documents return appropriate errors
5. Applied actions are listed in the result

## Phase 9: Integration and Polish

### Objective

Finalize the application with cross-cutting concerns and polish.

### Deliverables

**OpenAPI specification serving** at `/api/spec` returns the API's own OpenAPI specification, demonstrating meta-level usage of oastools.

**Error handling consistency** ensures all error paths render appropriate responses in both HTML and JSON formats.

**Documentation** including README with usage instructions, contributing guidelines, and deployment documentation.

**Testing** with integration tests covering the full request lifecycle for each operation.

### Acceptance Criteria

1. `/api/spec` returns valid OpenAPI 3.1 specification
2. All error responses follow consistent format
3. README explains how to use the web interface
4. All operations have integration test coverage
5. `make check` passes all quality gates

## Phase 10: Deployment

### Objective

Deploy the application to Google Cloud Run with continuous deployment.

### Deliverables

**Cloud Run service** deployed and accessible via public URL.

**Cloud Build trigger** configured for automatic deployment on main branch pushes.

**Monitoring** with log-based metrics and billing alerts configured.

### Acceptance Criteria

1. Application accessible at Cloud Run URL
2. Push to main triggers automatic deployment
3. Billing alerts configured at $1 threshold
4. Health checks pass
5. All operations function correctly in production environment

## Summary Timeline

| Phase | Description | Sessions |
|-------|-------------|----------|
| 1 | Project Foundation | 1 |
| 2 | Middleware Stack | 1 |
| 3 | Validate Operation | 1 |
| 4 | Convert Operation | 0.5 |
| 5 | Fix Operation | 0.5 |
| 6 | Diff Operation | 0.5 |
| 7 | Join Operation | 0.5 |
| 8 | Overlay Support | 0.5 |
| 9 | Integration and Polish | 1 |
| 10 | Deployment | 1 |

**Total: 7-8 focused development sessions**

Each session assumes 2-4 hours of focused development time. The actual duration may vary based on familiarity with the oastools library and Go web development patterns.
