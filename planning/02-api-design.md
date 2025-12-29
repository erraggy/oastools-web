# API Design

## Overview

The oastools-web API exposes a RESTful interface for OpenAPI specification processing operations. All endpoints accept multipart form data for file uploads and return either HTML (for browser requests) or JSON (for programmatic access). The API is designed using the oastools `builder.ServerBuilder`, making the web service itself a demonstration of the toolkit's capabilities.

## Base URL

The production base URL will be assigned by Cloud Run, following the pattern `https://oastools-web-<hash>-<region>.a.run.app`. A custom domain can be configured later if desired.

## Content Negotiation

The API supports content negotiation via the `Accept` header. When the `Accept` header includes `text/html` or when the `HX-Request` header is present, the response is rendered as HTML suitable for browser display. When the `Accept` header specifies `application/json`, the response is returned as structured JSON for programmatic consumption.

This dual-format approach allows the same API endpoints to serve both the interactive web interface and potential CLI or programmatic integrations.

## Authentication

The API requires no authentication. Rate limiting by IP address provides abuse protection for the free public service.

## Common Response Headers

All responses include the following headers.

The `X-RateLimit-Remaining` header indicates the number of requests remaining in the current rate limit window.

The `X-RateLimit-Reset` header provides the Unix timestamp when the rate limit window resets.

The `X-Request-ID` header contains a unique identifier for request tracing in logs.

## Error Response Format

Error responses follow a consistent structure across all endpoints.

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "The uploaded file is not a valid OpenAPI specification",
    "details": [
      {
        "path": "$.paths./pets.get",
        "message": "Operation is missing required 'responses' field"
      }
    ]
  }
}
```

The `code` field contains a machine-readable error code. The `message` field provides a human-readable description. The `details` array contains additional context when available, such as specific validation issues or processing errors.

## Endpoints

### POST /api/validate

Validates an OpenAPI specification and returns a detailed validation report.

**Request**

The request body is `multipart/form-data` containing a single file field named `spec`. The file must be a valid JSON or YAML document. Maximum file size is 2MB.

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="spec"; filename="openapi.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
info:
  title: Pet Store
  version: "1.0.0"
...
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "valid": false,
  "version": "3.1.0",
  "errors": [
    {
      "path": "$.paths./pets.get.responses",
      "message": "At least one response must be defined",
      "severity": "error"
    }
  ],
  "warnings": [
    {
      "path": "$.info.description",
      "message": "API description is recommended",
      "severity": "warning"
    }
  ],
  "statistics": {
    "paths": 5,
    "operations": 12,
    "schemas": 8,
    "errors": 1,
    "warnings": 1
  }
}
```

**Error Responses**

A 400 Bad Request response indicates the uploaded file could not be parsed as JSON or YAML, or the file exceeds the size limit.

A 415 Unsupported Media Type response indicates the content type is not `multipart/form-data`.

A 429 Too Many Requests response indicates the rate limit has been exceeded.

### POST /api/convert

Converts an OpenAPI specification between versions.

**Request**

The request body is `multipart/form-data` containing a file field named `spec` and a form field named `target` specifying the target version.

Supported target versions are `2.0`, `3.0`, `3.1`, and `3.2`.

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="spec"; filename="openapi.yaml"
Content-Type: application/x-yaml

swagger: "2.0"
...
------FormBoundary
Content-Disposition: form-data; name="target"

3.1
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "sourceVersion": "2.0",
  "targetVersion": "3.1.0",
  "issues": [
    {
      "path": "$.definitions.Pet.discriminator",
      "message": "Discriminator converted to object format",
      "severity": "info"
    },
    {
      "path": "$.securityDefinitions.oauth2",
      "message": "OAuth2 implicit flow not supported in 3.1, converted to authorizationCode",
      "severity": "warning"
    }
  ],
  "result": "openapi: \"3.1.0\"\ninfo:\n  title: Pet Store\n...",
  "format": "yaml"
}
```

The `result` field contains the converted specification in the same format (JSON or YAML) as the input. The `issues` array contains conversion notes, warnings, and any critical issues encountered.

**Error Responses**

A 400 Bad Request response indicates an invalid target version or unparseable input.

A 422 Unprocessable Entity response indicates the conversion cannot be completed due to incompatible features.

### POST /api/diff

Compares two OpenAPI specifications and returns a structured diff.

**Request**

The request body is `multipart/form-data` containing two file fields named `base` and `head`. Both files must be parseable OpenAPI specifications.

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="base"; filename="v1.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary
Content-Disposition: form-data; name="head"; filename="v2.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "baseVersion": "3.1.0",
  "headVersion": "3.1.0",
  "summary": {
    "additions": 3,
    "deletions": 1,
    "modifications": 5,
    "breaking": 2
  },
  "changes": [
    {
      "type": "addition",
      "path": "$.paths./users",
      "description": "New path added",
      "breaking": false
    },
    {
      "type": "deletion",
      "path": "$.paths./legacy",
      "description": "Path removed",
      "breaking": true
    },
    {
      "type": "modification",
      "path": "$.paths./pets.get.parameters[0]",
      "description": "Parameter 'limit' type changed from string to integer",
      "breaking": true,
      "before": "string",
      "after": "integer"
    }
  ]
}
```

The `breaking` flag indicates changes that could break existing clients.

### POST /api/fix

Applies automatic fixes to an OpenAPI specification.

**Request**

The request body is `multipart/form-data` containing a file field named `spec` and optional form fields for fix options.

Available fix options include `removeUnusedSchemas` (boolean), `fixInvalidRefs` (boolean), and `normalizeFormats` (boolean). All options default to `true`.

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="spec"; filename="openapi.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary
Content-Disposition: form-data; name="removeUnusedSchemas"

true
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "version": "3.1.0",
  "fixes": [
    {
      "path": "$.components.schemas.UnusedSchema",
      "action": "removed",
      "reason": "Schema is not referenced"
    },
    {
      "path": "$.paths./pets.get.responses.200.content.application/json.schema.$ref",
      "action": "corrected",
      "reason": "Invalid reference '#/definitions/Pet' corrected to '#/components/schemas/Pet'"
    }
  ],
  "result": "openapi: \"3.1.0\"\ninfo:\n  title: Pet Store\n...",
  "format": "yaml"
}
```

### POST /api/join

Merges multiple OpenAPI specifications into a single document.

**Request**

The request body is `multipart/form-data` containing 2 to 5 file fields named `spec[]` or `spec[0]`, `spec[1]`, etc. An optional form field named `collisionStrategy` specifies how to handle naming conflicts.

Collision strategies include `rename` (default, appends suffix), `first` (keeps first definition), and `error` (fails on collision).

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="spec[]"; filename="pets.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary
Content-Disposition: form-data; name="spec[]"; filename="users.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary
Content-Disposition: form-data; name="collisionStrategy"

rename
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "sourceCount": 2,
  "targetVersion": "3.1.0",
  "collisions": [
    {
      "type": "schema",
      "name": "Error",
      "sources": ["pets.yaml", "users.yaml"],
      "resolution": "Renamed to Error_1 and Error_2"
    }
  ],
  "statistics": {
    "paths": 8,
    "operations": 20,
    "schemas": 15
  },
  "result": "openapi: \"3.1.0\"\ninfo:\n  title: Merged API\n...",
  "format": "yaml"
}
```

**Error Responses**

A 400 Bad Request response indicates fewer than 2 files or more than 5 files were provided.

A 422 Unprocessable Entity response indicates collision strategy is `error` and collisions were detected.

### POST /api/overlay

Applies an OpenAPI Overlay to a specification.

**Request**

The request body is `multipart/form-data` containing a file field named `spec` (the target specification) and a file field named `overlay` (the overlay document).

```
Content-Type: multipart/form-data; boundary=----FormBoundary

------FormBoundary
Content-Disposition: form-data; name="spec"; filename="openapi.yaml"
Content-Type: application/x-yaml

openapi: "3.1.0"
...
------FormBoundary
Content-Disposition: form-data; name="overlay"; filename="overlay.yaml"
Content-Type: application/x-yaml

overlay: "1.0.0"
actions:
  - target: "$.info.description"
    update: "Updated description"
...
------FormBoundary--
```

**Response (200 OK)**

```json
{
  "specVersion": "3.1.0",
  "overlayVersion": "1.0.0",
  "actionsApplied": 3,
  "actions": [
    {
      "target": "$.info.description",
      "type": "update",
      "matched": 1
    }
  ],
  "result": "openapi: \"3.1.0\"\ninfo:\n  title: Pet Store\n  description: Updated description\n...",
  "format": "yaml"
}
```

### GET /api/spec

Returns the OpenAPI specification for the oastools-web API itself.

**Request**

No request body. The `Accept` header determines the response format.

**Response (200 OK)**

Returns the OpenAPI 3.1 specification for the web service API in YAML format (default) or JSON format (if `Accept: application/json` is specified).

### GET /health

Returns the health status of the service.

**Response (200 OK)**

```json
{
  "status": "healthy",
  "version": "1.0.0",
  "oastools": "1.33.0"
}
```

## Rate Limiting

All `/api/*` endpoints are rate limited to 10 requests per minute per IP address with a burst capacity of 3 requests. When the rate limit is exceeded, the API returns a 429 Too Many Requests response with a `Retry-After` header indicating when the client can retry.

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Please wait before making more requests.",
    "retryAfter": 45
  }
}
```

## File Size Limits

Individual specification files are limited to 2MB. Overlay files are limited to 500KB. For the join endpoint, each file is limited to 1MB with a maximum of 5 files total.

When a file exceeds the size limit, the API returns a 413 Payload Too Large response.

```json
{
  "error": {
    "code": "FILE_TOO_LARGE",
    "message": "File exceeds maximum size of 2MB",
    "maxSize": 2097152,
    "actualSize": 3145728
  }
}
```

## Timeout Behavior

All requests have a 30-second processing timeout. If processing exceeds this limit, the API returns a 504 Gateway Timeout response.

```json
{
  "error": {
    "code": "TIMEOUT",
    "message": "Request processing exceeded 30 second limit"
  }
}
```
