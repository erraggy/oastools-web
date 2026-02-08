# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

oastools-web is a public web application demonstrating the oastools Go toolkit. Users can validate, convert, diff, fix, and join OpenAPI specifications directly in the browser. The application itself is built using oastools' `builder.ServerBuilder`, serving as both a demonstration and dogfooding exercise.

The main `oastools` library repo is located at `~/code/oastools` (sibling directory).

## Development Workflow

### Branch Strategy
- Create a working branch from latest `main` before making any changes
- Branch naming: `<category>/<kebab-case-subject>`
- Categories: `feat`, `fix`, `chore`, `doc`, `ci`, `refactor`, `test`
- Examples: `feat/project-foundation`, `fix/rate-limiter-cleanup`

### Merging PRs
- Branch protections require review approval to merge
- Since the repo owner cannot self-review, use `gh pr merge --admin --squash` for owner PRs
- Remote branches auto-delete on merge; only local branch cleanup needed

### Commit Conventions
- Conventional commits format
- Subject line: max 72 characters
- Body: GitHub-flavored markdown with subheadings

```
feat: add validation endpoint handler

## Changes
- Add POST /api/validate handler
- Implement multipart file parsing

## Reasoning
Establishes the pattern for all subsequent operation handlers.
```

## Build Commands

```bash
make build        # Build the server binary
make test         # Run tests
make lint         # Run linting (Go + JS)
make check        # Run all checks (lint + test) - REQUIRED before pushing
make run          # Run the server locally
docker build .    # Build container image
```

### Pre-Push Requirements
Before pushing any changes to origin:
1. Run `make check` and ensure it passes with clean output
2. If linting makes automatic fixes (e.g., formatting), stage and include those changes
3. Address any errors or warnings before pushing

## Architecture

### Tech Stack
- **Backend**: Go with oastools `builder.ServerBuilder`
- **Frontend**: HTMX + Go html/template (no JS build step)
- **Hosting**: Google Cloud Run (scale-to-zero)

### Key Directories
```
cmd/server/           # Application entry point
internal/api/         # Handlers, middleware, response helpers
internal/templates/   # Go templates (embedded via embed.FS)
internal/config/      # Environment-based configuration
static/               # CSS and minimal JS
```

### Request Flow
1. Cloud Run receives request
2. Middleware chain: logging → recovery → rate limit → concurrency → timeout → size limit
3. Handler parses multipart upload, calls oastools library
4. Template renders HTML partial (HTMX) or JSON (Accept header)

### Parse-Once Pattern
All operations use the `*Parsed` variant of oastools functions to avoid re-parsing:
```go
parseResult, _ := parser.Parse(content)
validator.ValidateParsed(parseResult)    // Not validator.Validate(content)
converter.ConvertParsed(parseResult, v)  // Not converter.Convert(content, v)
```

### Content Negotiation
- `HX-Request` header → return HTML partial for HTMX swap
- `Accept: application/json` → return JSON response
- Default → full HTML page

## Configuration

Environment variables:
- `PORT` - HTTP port (default: 8080, set by Cloud Run)
- `LOG_LEVEL` - debug/info/warn/error (default: info)
- `RATE_LIMIT_RPM` - requests per minute per IP (default: 60)
- `MAX_FILE_SIZE` - max upload bytes (default: 2MB)
- `REQUEST_TIMEOUT` - processing timeout (default: 30s)
- `MAX_CONCURRENT_REQUESTS` - global concurrent request limit (default: 10)

## Security Constraints

- Max file size: 2MB (spec), 500KB (overlay), 1MB each for join (5 max)
- Request timeout: 30 seconds
- Rate limit: 60 req/min/IP with burst of 10 (excludes static files)
- Global concurrent requests: 10

## Deployment

The app deploys to Cloud Run via Cloud Build trigger on version tags (`v*.*.*`):
```bash
gcloud run deploy oastools-web \
    --region us-central1 \
    --memory 512Mi \
    --max-instances 2 \
    --min-instances 0
```

Automated oastools dependency updates are deployed directly via `gcloud builds submit` from the `update-oastools.yml` GitHub Actions workflow (no version tag needed).

Rollback: `gcloud run services update-traffic oastools-web --to-revisions=<revision>=100`

## Tool Preferences

- **GitHub resources**: Always use `gh` CLI over WebFetch for accessing GitHub (issues, PRs, releases, etc.)
