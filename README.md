# oastools-web

[![Go](https://github.com/erraggy/oastools-web/actions/workflows/go.yml/badge.svg)](https://github.com/erraggy/oastools-web/actions/workflows/go.yml)
[![Docker](https://github.com/erraggy/oastools-web/actions/workflows/docker.yml/badge.svg)](https://github.com/erraggy/oastools-web/actions/workflows/docker.yml)

A web application for working with OpenAPI specifications. Powered by the [oastools](https://github.com/erraggy/oastools) Go toolkit.

**Live site:** [oastools.robnrob.com](https://oastools.robnrob.com)

## Features

| Operation | Description |
|-----------|-------------|
| **Validate** | Check OpenAPI specs for errors and warnings with optional strict mode |
| **Convert** | Transform specs between OpenAPI 2.0, 3.0, 3.1, and 3.2 versions |
| **Diff** | Compare two specs and identify breaking vs non-breaking changes |
| **Fix** | Automatically repair common spec issues with dry-run preview |
| **Join** | Merge multiple specs into a single document with collision strategies |
| **Overlay** | Apply overlay documents to modify specs |

All operations support:
- File upload, paste, or URL input
- JSON and YAML formats (auto-detected)
- HTMX-powered partial page updates

## Tech Stack

- **Backend:** Go with oastools `builder.ServerBuilder`
- **Frontend:** HTMX + Go html/template (no JS build step)
- **Hosting:** Google Cloud Run (scale-to-zero)
- **Syntax highlighting:** highlight.js

## Local Development

```bash
# Build and run
make build
make run

# Run tests
make test

# Run all checks (tidy, fmt, vet, lint, test, build)
make check

# Docker build
docker build -t oastools-web .
```

## Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP port | 8080 |
| `LOG_LEVEL` | debug/info/warn/error | info |
| `RATE_LIMIT_RPM` | Requests per minute per IP | 60 |
| `MAX_FILE_SIZE` | Max upload size | 2MB |
| `REQUEST_TIMEOUT` | Processing timeout | 30s |
| `MAX_CONCURRENT_REQUESTS` | Global concurrent limit | 10 |

## License

MIT
