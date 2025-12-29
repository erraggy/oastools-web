# Phase 3: ServerBuilder Refactor + Validate Endpoint

## Overview

Refactor oastools-web to use `builder.ServerBuilder` from the oastools library, then implement the validate endpoint. This achieves the project's dogfooding goal - using oastools to build the oastools demo.

## Goals

1. Replace manual `http.ServeMux` with `builder.ServerBuilder`
2. Auto-generate OpenAPI spec for the web API itself
3. Implement POST /api/validate with content negotiation
4. Maintain all existing middleware functionality

---

## Part 1: ServerBuilder Refactor

### Files to Modify

```
internal/api/handler.go      # Replace ServeMux with ServerBuilder
internal/api/routes.go       # Convert to ServerBuilder operations
internal/api/health.go       # Convert handler signature
internal/api/responses.go    # NEW: Content negotiation + HTML response type
cmd/server/main.go          # Minor updates for new handler API
go.mod                      # Add oastools dependency
```

### Key Changes

#### 1. Handler Struct Refactor

**Before:**
```go
type Handler struct {
    cfg         *config.Config
    version     string
    templates   *template.Template
    mux         *http.ServeMux
    rateLimiter *RateLimiter
    handler     http.Handler
}
```

**After:**
```go
type Handler struct {
    cfg         *config.Config
    version     string
    templates   *template.Template
    rateLimiter *RateLimiter
    server      *builder.ServerResult
}
```

#### 2. NewHandler Refactor

```go
func NewHandler(cfg *config.Config, version string) (*Handler, error) {
    tmpl, err := template.ParseFS(templates.FS, "*.html", "partials/*.html")
    if err != nil {
        return nil, err
    }

    h := &Handler{
        cfg:         cfg,
        version:     version,
        templates:   tmpl,
        rateLimiter: NewRateLimiter(cfg.RateLimitRPM, cfg.RateLimitBurst),
    }

    srv := h.buildServer()
    result, err := srv.BuildServer()
    if err != nil {
        return nil, fmt.Errorf("build server: %w", err)
    }

    h.server = result
    return h, nil
}

func (h *Handler) buildServer() *builder.ServerBuilder {
    srv := builder.NewServerBuilder(parser.OASVersion320, builder.WithoutValidation()).
        SetTitle("oastools API").
        SetVersion(h.version).
        SetDescription("OpenAPI specification toolkit - validate, convert, diff, fix, and join specs")

    // Add operations
    h.registerOperations(srv)

    // Add middleware (first = outermost)
    srv.Use(
        Logging,
        Recovery,
        h.rateLimiter.Middleware,
        ConcurrencyLimiter(h.cfg.MaxConcurrentRequests),
        Timeout(h.cfg.RequestTimeout),
        RequestSizeLimiter(h.cfg.MaxFileSize),
    )

    return srv
}
```

#### 3. Handler Signature Change

**Before:** `func(w http.ResponseWriter, r *http.Request)`
**After:** `func(ctx context.Context, req *builder.Request) builder.Response`

**Health endpoint example:**
```go
func (h *Handler) handleHealth(ctx context.Context, req *builder.Request) builder.Response {
    return builder.JSON(http.StatusOK, HealthResponse{
        Status:   "healthy",
        Version:  h.version,
        OASTools: "1.33.0",
    })
}
```

#### 4. Static Files + Landing Page

ServerBuilder handles API routes. Static files and HTML pages need special handling:

```go
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Static files
    if strings.HasPrefix(r.URL.Path, "/static/") {
        h.serveStatic(w, r)
        return
    }

    // HTML pages (not API)
    if !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/health" {
        h.servePage(w, r)
        return
    }

    // API routes via ServerBuilder
    h.server.Handler.ServeHTTP(w, r)
}
```

---

## Part 2: Validate Endpoint

### API Definition

```go
// Request/Response types
type ValidateResponse struct {
    Valid      bool              `json:"valid"`
    Version    string            `json:"version"`
    Errors     []ValidationIssue `json:"errors"`
    Warnings   []ValidationIssue `json:"warnings"`
    Statistics ValidationStats   `json:"statistics"`
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
```

### Operation Registration

```go
srv.AddOperation(http.MethodPost, "/api/validate",
    builder.WithOperationID("validateSpec"),
    builder.WithSummary("Validate an OpenAPI specification"),
    builder.WithTags("operations"),
    builder.WithFileParam("spec",
        builder.WithParamDescription("OpenAPI specification file (JSON or YAML)"),
        builder.WithParamRequired(true),
    ),
    builder.WithResponse(http.StatusOK, ValidateResponse{},
        builder.WithResponseDescription("Validation result"),
    ),
    builder.WithResponse(http.StatusBadRequest, ErrorResponse{},
        builder.WithResponseDescription("Invalid request or unparseable file"),
    ),
)

srv.Handle(http.MethodPost, "/api/validate", h.handleValidate)
```

### Handler Implementation

```go
func (h *Handler) handleValidate(ctx context.Context, req *builder.Request) builder.Response {
    // Get file from multipart form
    file, header, err := req.HTTPRequest.FormFile("spec")
    if err != nil {
        return builder.Error(http.StatusBadRequest, "spec file required")
    }
    defer file.Close()

    content, err := io.ReadAll(file)
    if err != nil {
        return builder.Error(http.StatusBadRequest, "failed to read file")
    }

    // Parse using oastools
    parseResult, err := parser.Parse(content, parser.WithFilePath(header.Filename))
    if err != nil {
        return builder.Error(http.StatusBadRequest, fmt.Sprintf("failed to parse: %v", err))
    }

    // Validate using parse-once pattern
    v := validator.New()
    validationResult := v.ValidateParsed(parseResult)

    // Build response
    result := h.buildValidateResponse(parseResult, validationResult)

    // Content negotiation
    if req.HTTPRequest.Header.Get("HX-Request") == "true" {
        return h.renderHTML("validation-result", result)
    }

    return builder.JSON(http.StatusOK, result)
}
```

### Content Negotiation Helper

```go
// responses.go

type htmlResponse struct {
    status int
    html   string
}

func (r *htmlResponse) StatusCode() int       { return r.status }
func (r *htmlResponse) Headers() http.Header  { return nil }
func (r *htmlResponse) Body() any             { return r.html }
func (r *htmlResponse) WriteTo(w http.ResponseWriter) error {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(r.status)
    _, err := w.Write([]byte(r.html))
    return err
}

func (h *Handler) renderHTML(templateName string, data any) builder.Response {
    var buf bytes.Buffer
    if err := h.templates.ExecuteTemplate(&buf, templateName, data); err != nil {
        return builder.Error(http.StatusInternalServerError, "template error")
    }
    return &htmlResponse{status: http.StatusOK, html: buf.String()}
}
```

---

## Part 3: Templates

### Files to Create

```
internal/templates/validate.html                    # Form page
internal/templates/partials/validation-result.html  # HTMX result partial
```

### validate.html

```html
{{template "base" .}}

{{define "title"}}Validate - oastools{{end}}

{{define "content"}}
<section class="operation-page">
    <h1>Validate OpenAPI Specification</h1>
    <p>Check your specification for errors and warnings.</p>

    <form hx-post="/api/validate"
          hx-target="#result"
          hx-indicator="#spinner"
          enctype="multipart/form-data">
        <div class="form-group">
            <label for="spec">Specification File</label>
            <input type="file" id="spec" name="spec" accept=".json,.yaml,.yml" required>
        </div>
        <button type="submit" class="btn">
            Validate
            <span id="spinner" class="spinner htmx-indicator"></span>
        </button>
    </form>

    <div id="result"></div>
</section>
{{end}}
```

### partials/validation-result.html

```html
{{define "validation-result"}}
<div class="result">
    <div class="result-header">
        <h2>Validation Result</h2>
        {{if .Valid}}
        <span class="badge badge-success">Valid</span>
        {{else}}
        <span class="badge badge-error">Invalid</span>
        {{end}}
    </div>

    <p>OpenAPI Version: {{.Version}}</p>

    {{if .Errors}}
    <h3>Errors ({{len .Errors}})</h3>
    <ul class="issue-list">
        {{range .Errors}}
        <li class="issue-item">
            <span class="issue-path">{{.Path}}</span>
            <p>{{.Message}}</p>
        </li>
        {{end}}
    </ul>
    {{end}}

    {{if .Warnings}}
    <h3>Warnings ({{len .Warnings}})</h3>
    <ul class="issue-list">
        {{range .Warnings}}
        <li class="issue-item">
            <span class="issue-path">{{.Path}}</span>
            <p>{{.Message}}</p>
        </li>
        {{end}}
    </ul>
    {{end}}

    <h3>Statistics</h3>
    <dl>
        <dt>Paths</dt><dd>{{.Statistics.Paths}}</dd>
        <dt>Operations</dt><dd>{{.Statistics.Operations}}</dd>
        <dt>Schemas</dt><dd>{{.Statistics.Schemas}}</dd>
    </dl>
</div>
{{end}}
```

---

## Part 4: Self-Hosted Spec

ServerBuilder automatically generates the OpenAPI spec. Expose it:

```go
srv.AddOperation(http.MethodGet, "/api/spec",
    builder.WithOperationID("getAPISpec"),
    builder.WithSummary("Get this API's OpenAPI specification"),
    builder.WithResponse(http.StatusOK, nil,
        builder.WithResponseDescription("OpenAPI 3.2 specification"),
    ),
)

srv.Handle(http.MethodGet, "/api/spec", func(ctx context.Context, req *builder.Request) builder.Response {
    // h.server.Spec contains the built OAS document
    // Serialize based on Accept header
    if strings.Contains(req.HTTPRequest.Header.Get("Accept"), "application/json") {
        return builder.JSON(http.StatusOK, h.server.Spec)
    }
    // Default YAML
    yamlBytes, _ := yaml.Marshal(h.server.Spec)
    return builder.NewResponse(http.StatusOK).
        Header("Content-Type", "application/x-yaml").
        Binary("application/x-yaml", yamlBytes)
})
```

---

## Implementation Order

1. **Add oastools dependency** to go.mod
2. **Create responses.go** with HTML response type
3. **Refactor handler.go** to use ServerBuilder
4. **Convert health.go** to new handler signature
5. **Update routes.go** with operation definitions
6. **Create validate.go** with handler
7. **Create templates** (validate.html, validation-result.html)
8. **Add /api/spec endpoint**
9. **Update main.go** if needed
10. **Test everything**

---

## Acceptance Criteria

- [ ] Server starts and serves landing page
- [ ] `/health` returns JSON health status
- [ ] `/api/spec` returns the auto-generated OpenAPI spec
- [ ] GET `/validate` serves the form page
- [ ] POST `/api/validate` with JSON spec returns JSON result
- [ ] POST `/api/validate` with YAML spec returns JSON result
- [ ] POST `/api/validate` with `HX-Request` header returns HTML partial
- [ ] Invalid specs return validation errors with JSONPath locations
- [ ] Rate limiting, timeouts, and other middleware still work
- [ ] Static files still served at `/static/*`

---

## Dependencies

```go
require (
    github.com/erraggy/oastools v1.33.0
    golang.org/x/time v0.9.0
)
```

Note: Using go.work for local development links to sibling oastools directory.
