# OASTools Web Application Implementation Plan

## Executive Summary

This document outlines the implementation plan for **oastools-web**, a public web application that demonstrates and provides access to the oastools toolkit functionality. The application will allow developers to try validate, convert, diff, fix, and join operations on OpenAPI specifications directly in their browser, with optional overlay support.

The application showcases oastools capabilities by using the toolkit itself to build the web service, specifically leveraging the `builder` package's `ServerBuilder` to construct the HTTP server from a fluent API definition.

## Project Goals

1. **Demonstrate oastools capabilities** through a functional web interface
2. **Lower adoption barriers** by letting developers try before integrating
3. **Dogfood the builder package** by using `ServerBuilder` to construct the web service
4. **Minimize operational costs** through Google Cloud Run's free tier
5. **Keep the implementation minimal** while remaining functional and secure

## Technical Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Hosting** | Google Cloud Run | 2M free requests/month, fast Go cold starts, scale-to-zero |
| **Repository** | Separate repo (`oastools-web`) | Clean separation, independent deployment, different release cycles |
| **Web Framework** | oastools `builder.ServerBuilder` | Dogfooding, demonstrates code-first workflow, stdlib router |
| **Frontend** | HTMX + Go templates | No build step, ~14kb, server-rendered results |
| **Rate Limiting** | `golang.org/x/time/rate` | Stdlib-compatible, in-memory, sufficient for demo traffic |

## Repository Structure

```
github.com/erraggy/oastools-web/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   ├── handlers.go          # Operation handlers (validate, convert, etc.)
│   │   ├── middleware.go        # Rate limiting, timeouts, logging
│   │   └── responses.go         # Response formatting helpers
│   ├── templates/
│   │   ├── base.html            # Base layout with HTMX
│   │   ├── index.html           # Landing page
│   │   ├── partials/            # HTMX response fragments
│   │   │   ├── results.html     # Operation results
│   │   │   ├── diff.html        # Diff visualization
│   │   │   └── errors.html      # Validation errors display
│   │   └── embed.go             # Template embedding
│   └── config/
│       └── config.go            # Configuration from environment
├── static/
│   ├── css/
│   │   └── style.css            # Minimal custom styles
│   └── js/
│       └── app.js               # Minimal JS (progress, file handling)
├── Dockerfile                   # Multi-stage build
├── cloudbuild.yaml              # Cloud Build configuration
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Feature Scope

### Phase 1: Core Operations (MVP)

| Operation | Input | Output | Limits |
|-----------|-------|--------|--------|
| **Validate** | Single spec (JSON/YAML) | Validation report with errors/warnings | 2MB max |
| **Convert** | Single spec + target version | Converted spec + conversion issues | 2MB max |
| **Fix** | Single spec | Fixed spec + list of fixes applied | 2MB max |

### Phase 2: Multi-File Operations

| Operation | Input | Output | Limits |
|-----------|-------|--------|--------|
| **Diff** | Two specs (JSON/YAML) | Structured diff report | 2MB each |
| **Join** | Multiple specs (2-5 files) | Merged spec + collision report | 1MB each, 5 max |

### Phase 3: Overlay Support

| Operation | Input | Output | Limits |
|-----------|-------|--------|--------|
| **Apply Overlay** | Spec + overlay file | Modified spec | 2MB + 500KB |
| **Overlay with operations** | Spec + overlay + operation | Operation result with overlay applied | As above |

## Security Constraints

| Constraint | Value | Rationale |
|------------|-------|-----------|
| Max file size | 2MB (spec), 500KB (overlay) | Prevents memory exhaustion |
| Max files per request | 5 | Limits join operation scope |
| Request timeout | 30 seconds | Prevents long-running abuse |
| Rate limit | 10 requests/minute/IP | Prevents abuse on free tier |
| Concurrent requests | 10 global | Protects Cloud Run resources |

## Document Index

| Document | Purpose |
|----------|---------|
| [01-architecture.md](./01-architecture.md) | Technical architecture and data flow |
| [02-api-design.md](./02-api-design.md) | API endpoints and request/response formats |
| [03-frontend.md](./03-frontend.md) | HTMX frontend implementation |
| [04-deployment.md](./04-deployment.md) | Google Cloud Run deployment |
| [05-implementation-phases.md](./05-implementation-phases.md) | Phased implementation with acceptance criteria |

## Success Metrics

1. **Functional**: All exposed operations work correctly on valid inputs
2. **Secure**: No resource exhaustion or abuse vectors
3. **Cost**: Remains within Cloud Run free tier for expected demo traffic
4. **Discoverable**: Landing page clearly explains available functionality
5. **Demonstrative**: Successfully showcases oastools capabilities

## Timeline Estimate

| Phase | Scope | Effort |
|-------|-------|--------|
| Phase 1 (MVP) | Validate, Convert, Fix | 2-3 sessions |
| Phase 2 | Diff, Join | 1-2 sessions |
| Phase 3 | Overlay support | 1 session |
| Deployment | Cloud Run setup | 1 session |

Total estimated effort: 5-7 focused development sessions.
