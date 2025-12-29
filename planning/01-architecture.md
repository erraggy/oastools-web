# Architecture Design

## Overview

The oastools-web application follows a straightforward server-side architecture where the Go backend handles all processing logic and serves HTML responses. HTMX enables dynamic page updates without requiring a JavaScript framework or build process.

## System Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Google Cloud Run                                │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         oastools-web Container                          │ │
│  │                                                                          │ │
│  │  ┌──────────────┐    ┌──────────────┐    ┌────────────────────────────┐ │ │
│  │  │   Middleware │    │   Handlers   │    │      oastools Library      │ │ │
│  │  │              │    │              │    │                            │ │ │
│  │  │ • Rate Limit │───▶│ • validate   │───▶│ • parser.Parse()           │ │ │
│  │  │ • Timeout    │    │ • convert    │    │ • validator.Validate()     │ │ │
│  │  │ • Recovery   │    │ • diff       │    │ • converter.Convert()      │ │ │
│  │  │ • Logging    │    │ • fix        │    │ • differ.Diff()            │ │ │
│  │  │ • Size Limit │    │ • join       │    │ • fixer.Fix()              │ │ │
│  │  └──────────────┘    │ • overlay    │    │ • joiner.Join()            │ │ │
│  │                      └──────────────┘    │ • overlay.Apply()          │ │ │
│  │                             │            └────────────────────────────┘ │ │
│  │                             ▼                                            │ │
│  │                      ┌──────────────┐                                    │ │
│  │                      │  Templates   │                                    │ │
│  │                      │              │                                    │ │
│  │                      │ • Full pages │                                    │ │
│  │                      │ • Partials   │                                    │ │
│  │                      └──────────────┘                                    │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
                                      ▲
                                      │ HTTPS
                                      ▼
                              ┌──────────────┐
                              │    Browser   │
                              │              │
                              │ • HTMX       │
                              │ • HTML/CSS   │
                              │ • File input │
                              └──────────────┘
```

## Request Flow

The request flow demonstrates how a validation request moves through the system.

**Step 1: Request arrives at Cloud Run**. The browser sends a multipart form POST with the uploaded OpenAPI specification file. Cloud Run's load balancer routes the request to an available container instance.

**Step 2: Middleware chain executes**. The request passes through the middleware stack in order: rate limiting checks the IP against the token bucket, the timeout handler wraps the request with a 30-second deadline, the recovery middleware catches any panics, and the size limiter enforces the 2MB maximum.

**Step 3: Handler processes the request**. The validate handler reads the uploaded file, determines its format (JSON or YAML), and invokes the oastools library. Using the parse-once pattern, it calls `parser.Parse()` to get a `ParseResult`, then passes that to `validator.ValidateParsed()`.

**Step 4: Response rendering**. The handler passes validation results to the template engine. For full page loads, the base template wraps the results partial. For HTMX requests (detected via the `HX-Request` header), only the results partial is rendered.

**Step 5: Response delivery**. The rendered HTML returns to the browser. HTMX swaps the content into the designated target element without a full page reload.

## Using oastools ServerBuilder

The web service itself is built using the `builder.ServerBuilder`, demonstrating the code-first workflow that oastools enables. This approach provides several advantages: the API definition and implementation coexist in the same codebase, request validation comes automatically through the httpvalidator integration, and the OpenAPI specification for the web service itself can be served at `/openapi.yaml`.

```go
func buildServer() (*builder.ServerResult, error) {
    srv := builder.NewServerBuilder(parser.OASVersion310,
        builder.WithServerInfo("oastools-web", "1.0.0"),
        builder.WithStdlibRouter(),
    )

    // Add common middleware
    srv.Use(rateLimitMiddleware)
    srv.Use(timeoutMiddleware)
    srv.Use(recoveryMiddleware)

    // Define and implement operations
    srv.AddOperation(http.MethodPost, "/api/validate",
        builder.WithSummary("Validate an OpenAPI specification"),
        builder.WithRequestBody(builder.FileUpload("spec")),
        builder.WithResponse(http.StatusOK, ValidationResult{}),
        builder.WithHandler(validateHandler),
    )

    // ... additional operations

    return srv.BuildServer()
}
```

## Data Flow: Validate Operation

This sequence illustrates the complete data flow for a validation request.

```
Browser                    Server                     oastools
   │                          │                           │
   │  POST /api/validate      │                           │
   │  (multipart/form-data)   │                           │
   │─────────────────────────▶│                           │
   │                          │                           │
   │                          │  Parse multipart form     │
   │                          │  Extract file bytes       │
   │                          │                           │
   │                          │  parser.Parse(bytes)      │
   │                          │──────────────────────────▶│
   │                          │                           │
   │                          │◀──────────────────────────│
   │                          │  ParseResult              │
   │                          │                           │
   │                          │  validator.ValidateParsed │
   │                          │──────────────────────────▶│
   │                          │                           │
   │                          │◀──────────────────────────│
   │                          │  ValidationResult         │
   │                          │                           │
   │                          │  Render template          │
   │                          │                           │
   │  HTML response           │                           │
   │◀─────────────────────────│                           │
   │                          │                           │
   │  HTMX swaps content      │                           │
   │                          │                           │
```

## Data Flow: Diff Operation (Two Files)

The diff operation demonstrates handling multiple file uploads with the parse-once pattern.

```
Browser                    Server                     oastools
   │                          │                           │
   │  POST /api/diff          │                           │
   │  (2 files)               │                           │
   │─────────────────────────▶│                           │
   │                          │                           │
   │                          │  Extract both files       │
   │                          │                           │
   │                          │  parser.Parse(fileA)      │
   │                          │──────────────────────────▶│
   │                          │◀──────────────────────────│
   │                          │                           │
   │                          │  parser.Parse(fileB)      │
   │                          │──────────────────────────▶│
   │                          │◀──────────────────────────│
   │                          │                           │
   │                          │  differ.DiffParsed(A, B)  │
   │                          │──────────────────────────▶│
   │                          │                           │
   │                          │◀──────────────────────────│
   │                          │  DiffResult               │
   │                          │                           │
   │  HTML diff view          │                           │
   │◀─────────────────────────│                           │
```

## Middleware Stack

The middleware executes in the order shown, with the first middleware being the outermost wrapper.

**Rate Limiter** uses a token bucket algorithm with 10 tokens per minute per IP address and a burst capacity of 3 requests. This prevents abuse while allowing legitimate usage spikes. The implementation uses `golang.org/x/time/rate` with an in-memory map of IP addresses to limiters.

**Timeout Handler** wraps each request with a 30-second deadline using `http.TimeoutHandler`. This prevents any single request from consuming resources indefinitely. The oastools library operations are designed to be interruptible via context cancellation.

**Recovery Middleware** catches panics from handler code and converts them to 500 Internal Server Error responses. This prevents a single bad request from crashing the container. The panic is logged for debugging purposes.

**Request Size Limiter** uses `http.MaxBytesReader` to enforce the 2MB file size limit at the HTTP layer, preventing large uploads from consuming memory before they reach handler code.

**Logging Middleware** records request method, path, response status, and duration. In Cloud Run, these logs integrate with Cloud Logging for monitoring and debugging.

## Template Architecture

Templates use Go's `html/template` package with an embedded filesystem for single-binary deployment.

The base template (`base.html`) provides the HTML document structure, includes HTMX from CDN, defines the navigation and layout, and declares blocks that child templates fill.

Page templates extend the base template and define the main content area. Each operation (validate, convert, diff, etc.) has its own page template with the appropriate file upload form.

Partial templates render operation results and are designed to work both within full page loads and as HTMX response fragments. When the server detects the `HX-Request` header, it renders only the partial without the base template wrapper.

```
templates/
├── base.html              # Document shell, HTMX script, navigation
├── index.html             # Landing page with operation cards
├── validate.html          # Validation form page
├── convert.html           # Conversion form page
├── diff.html              # Diff form page (two file inputs)
├── fix.html               # Fix form page
├── join.html              # Join form page (multiple file inputs)
└── partials/
    ├── validation-result.html   # Validation errors/warnings display
    ├── conversion-result.html   # Converted spec + issues
    ├── diff-result.html         # Side-by-side or unified diff view
    ├── fix-result.html          # Applied fixes list + result spec
    ├── join-result.html         # Merged spec + collision info
    └── error.html               # Generic error display
```

## Error Handling Strategy

Errors are categorized into client errors (4xx) and server errors (5xx), each with appropriate responses.

**Client errors** include invalid file format (not JSON or YAML), file too large (exceeds 2MB limit), too many files (for join operation), rate limit exceeded, and unsupported conversion target. These render a user-friendly error partial explaining the issue and suggesting remediation.

**Server errors** include oastools library failures, template rendering failures, and unexpected panics. These render a generic error message without exposing internal details. The actual error is logged for debugging.

**Validation failures** are distinct from errors. When a specification fails validation, this is a successful operation that returns validation results, not an error. The validation result partial displays errors and warnings in a structured format with JSONPath locations.

## Performance Considerations

**Parse-Once Pattern**: Every operation that follows parsing (validate, convert, diff, fix, join) uses the `*Parsed` variant that accepts a `ParseResult`. This avoids re-parsing the same document multiple times.

**Template Caching**: Templates are parsed once at startup using `template.Must()` and `embed.FS`. Each request reuses the cached template structure.

**Memory Management**: File uploads stream directly to handlers via `multipart.Reader`. The 2MB limit ensures each request has bounded memory requirements. After processing, no references to the uploaded data are retained.

**Cold Start Optimization**: The compiled Go binary starts in under 100ms. Template parsing and oastools initialization add minimal overhead. Cloud Run instances become ready to serve within 500ms typically.

## Stateless Design

The application maintains no state between requests. Each request is independent and self-contained. This design choice provides several benefits: Cloud Run can scale instances without coordination, any instance can handle any request, and failure of one instance does not affect others.

The rate limiter's in-memory state is per-instance, meaning a user could exceed the intended rate limit by having requests routed to different instances. For demo-level traffic, this acceptable tradeoff avoids the complexity of distributed rate limiting.
