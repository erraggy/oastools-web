# Implementation Status

**Last Updated:** 2025-12-28
**Live URL:** https://oastools-junhdghxba-uc.a.run.app

## Completed Phases

### Phase 1: Project Foundation ✅
- Repository structure, Makefile, Dockerfile
- cmd/server/main.go with graceful shutdown
- internal/config, internal/templates, static files
- Base template with HTMX, landing page

### Phase 2: Middleware Stack ✅
- Rate limiting (per-IP with golang.org/x/time/rate)
- Recovery (panic handling with stack traces)
- Logging (structured slog)
- Timeout and size limiting
- Concurrency limiter

### Phase 3: Validate Operation + ServerBuilder Refactor ✅
- Refactored from http.ServeMux to oastools builder.ServerBuilder (dogfooding!)
- Handler signature: `func(ctx context.Context, req *builder.Request) builder.Response`
- POST /api/validate - validates OpenAPI specs
- GET /api/spec - serves auto-generated OpenAPI 3.2 spec
- Content negotiation (HX-Request → HTML, Accept: application/json → JSON)
- validate.html form page and validation-result.html partial

### Phase 10: Deployment ✅
- GCP project: oastools-web
- Cloud Run service: oastools (us-central1)
- Artifact Registry: us-central1-docker.pkg.dev/oastools-web/oastools-web
- cloudbuild.yaml for CI/CD
- Scale-to-zero (0-2 instances, 512Mi)

## Remaining Phases

### Phase 4: Convert Operation ⏳
Create POST /api/convert endpoint following the validate pattern:
- Accept spec file + target version (2.0, 3.0, 3.1, 3.2)
- Use `converter.ConvertParsed()` from oastools
- Return converted spec + conversion issues
- Create convert.html and conversion-result.html templates

### Phase 5: Fix Operation ⏳
Create POST /api/fix endpoint:
- Accept spec file + fix options (removeUnusedSchemas, fixInvalidRefs, normalizeFormats)
- Use `fixer.FixParsed()` from oastools
- Return fixed spec + list of applied fixes
- Create fix.html and fix-result.html templates

### Phase 6: Diff Operation ⏳
Create POST /api/diff endpoint:
- Accept two spec files (base, head)
- Use `differ.DiffParsed()` from oastools
- Return structured diff with breaking change flags
- Create diff.html and diff-result.html templates

### Phase 7: Join Operation ⏳
Create POST /api/join endpoint:
- Accept 2-5 spec files + collision strategy (rename, first, error)
- Use `joiner.JoinParsed()` from oastools
- Return merged spec + collision info
- Create join.html and join-result.html templates

### Phase 8: Overlay Support ⏳
Create POST /api/overlay endpoint:
- Accept spec file + overlay file
- Use overlay package from oastools
- Return modified spec + applied actions
- Create overlay.html and overlay-result.html templates

### Phase 9: Integration & Polish 🔶 (Partial)
- [x] /api/spec endpoint (done in Phase 3)
- [ ] Integration tests for all operations
- [ ] README with usage documentation
- [ ] Error response consistency check

## Key Patterns to Follow

### Handler Pattern (from validate.go)
```go
func (h *Handler) handleXxx(_ context.Context, req *builder.Request) builder.Response {
    // 1. Get file(s) from multipart form
    file, _, err := req.HTTPRequest.FormFile("spec")

    // 2. Parse with oastools
    parseResult, err := parser.ParseWithOptions(parser.WithBytes(content))

    // 3. Process with oastools (validate, convert, fix, diff, join, overlay)
    result, err := xxx.XxxParsed(parseResult, ...)

    // 4. Content negotiation
    if wantsHTML(req.HTTPRequest) {
        return h.renderHTML("xxx-result.html", result)
    }
    return builder.JSON(http.StatusOK, result)
}
```

### Route Registration (in routes.go)
```go
srv.AddOperation(http.MethodPost, "/api/xxx",
    builder.WithOperationID("xxxSpec"),
    builder.WithSummary("..."),
    builder.WithTags("operations"),
    builder.WithFileParam("spec", ...),
    builder.WithResponse(http.StatusOK, XxxResponse{}, ...),
    builder.WithHandler(h.handleXxx),
)
```

### Page Routing (in handler.go servePage)
Add case for each new page:
```go
case "/convert":
    templateName = "convert.html"
```

## Files to Create for Each Operation

For operation `xxx`:
1. `internal/api/xxx.go` - handler + response types
2. `internal/templates/xxx.html` - form page
3. `internal/templates/partials/xxx-result.html` - result partial
4. Update `internal/api/routes.go` - add operation
5. Update `internal/api/handler.go` - add page route in servePage()

## Deployment

After implementing, redeploy with:
```bash
gcloud builds submit --config=cloudbuild.yaml --substitutions=SHORT_SHA=$(git rev-parse --short HEAD)
```

## Reference

- oastools library: ~/code/oastools (sibling directory, linked via go.work)
- See planning/05-implementation-phases.md for detailed phase specs
- See CLAUDE.md for project conventions
