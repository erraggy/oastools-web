# Explore Page Design

**Date:** 2026-01-11
**Status:** Approved
**Scope:** New Explore page for read-only analysis of OpenAPI specifications

## Summary

Add an Explore page that provides read-only analysis of OpenAPI specifications using the oastools walker package. The page serves two audiences: developers onboarding to unfamiliar APIs and architects/reviewers auditing API structure.

## Goals

1. Display operations grouped by path (default), tag, or method
2. Display component schemas with type badges and property previews
3. Display security schemes with coverage statistics
4. Support both OAS2 (Swagger) and OAS3.x specifications
5. Provide drill-in details for operations, schemas, and security schemes
6. Use browser sessionStorage for cache resilience across server restarts

## Non-Goals

- Editing or modifying specifications (read-only view)
- Generating client/server code
- Full Swagger UI replacement (this is a lighter analysis tool)
- URL fetch for remote specs (file upload and paste only)

---

## User Interface

### Page Structure

```
+-------------------------------------------------------------+
|  Explore                                                    |
+-------------------------------------------------------------+
|  [Upload spec file] or [Paste]                              |
|                                                             |
|  +- Summary --------------------------------------------+   |
|  | OpenAPI 3.0.3 - petstore.yaml                        |   |
|  | Paths: 12  |  Operations: 28  |  Schemas: 15         |   |
|  |                                          [v Details] |   |
|  +------------------------------------------------------+   |
|                                                             |
|  [Operations]  [Schemas]  [Security]     [Group by: Path v] |
|  -----------------------------------------------------------+
|  (tab content)                                              |
+-------------------------------------------------------------+
```

### Summary Section

Minimal by default:
- OpenAPI version (2.0, 3.0.x, 3.1.x)
- Filename or "Pasted content"
- Path, operation, and schema counts

Expanded details (on click):
- Method breakdown with glyphs
- Inline schema count with audit note
- Security coverage percentage

### Operations Tab

Default view groups operations by URL path:

```
+-------------------------------------------------------------+
|  > /pets                                   |GET   ^POST     |
|  v /pets/{petId}                      |GET  ->PUT  xDELETE  |
|    +------------------------------------------------------+ |
|    | | GET    getPetById      Returns a pet by ID      -> | |
|    | -> PUT   updatePet       Update an existing pet   -> | |
|    | x DELETE deletePet       Deletes a pet            -> | |
|    +------------------------------------------------------+ |
|  > /pets/{petId}/photos                    |GET   ^POST     |
+-------------------------------------------------------------+
```

Method badges include accessibility glyphs:

| Method | Glyph | Color  | Meaning          |
|--------|-------|--------|------------------|
| GET    | |     | green  | retrieve/download |
| POST   | ^     | blue   | create/upload     |
| PUT    | ->    | orange | replace/send      |
| PATCH  | ~     | orange | partial update    |
| DELETE | x     | red    | remove            |

Group-by dropdown switches between: Path (default), Tag, Method

Clicking an operation scrolls to its detail section showing:
- Method, path, description
- Parameters table
- Responses table
- Security requirements

### Schemas Tab

```
+-------------------------------------------------------------+
|  Component Schemas (15)              Inline Schemas: 8      |
|                                      [v Show locations]     |
|  +--------------------------------------------------------+ |
|  | Pet              {object}   id, name, status, category | |
|  | Category         {object}   id, name                   | |
|  | Status           [enum]     available, pending, sold   | |
|  | PetArray         [array]    -> Pet                     | |
|  +--------------------------------------------------------+ |
+-------------------------------------------------------------+
```

Type badges:

| Badge     | Meaning                    |
|-----------|----------------------------|
| {object}  | Object with properties     |
| [array]   | Array type                 |
| [enum]    | Enum values                |
| string    | Primitive                  |
| {allOf}   | Composed schema            |
| {oneOf}   | Polymorphic                |

Clicking a schema scrolls to detail section showing:
- Full property list with types
- "Used in" operations list

Inline schemas drill-in shows where inline schemas appear.

### Security Tab

```
+-------------------------------------------------------------+
|  Security Schemes (3)                    Coverage: 24/28    |
|                                          [v Show unsecured] |
|  +--------------------------------------------------------+ |
|  | api_key           [apiKey]    header: X-API-Key        | |
|  |                               Used by 20 operations    | |
|  |                                                        | |
|  | petstore_auth     [oauth2]    implicit flow            | |
|  |                               Scopes: read:pets, ...   | |
|  |                               Used by 8 operations     | |
|  +--------------------------------------------------------+ |
+-------------------------------------------------------------+
```

Security scheme type badges: [apiKey], [http], [oauth2], [openIdConnect], [mutualTLS]

Clicking a scheme scrolls to detail section showing:
- Full configuration
- "Used by" operations with required scopes

Unsecured operations drill-in lists operations with no security.

---

## Architecture

### Parse-Once, Lazy-Render with Browser Fallback

1. User uploads spec
2. JavaScript stores content in sessionStorage
3. Server parses spec, computes hash, caches analysis (2min TTL)
4. Response includes hash in hidden field
5. Tab switches use hash to retrieve cached analysis
6. On cache miss (410), JavaScript auto-resubmits from sessionStorage

```
User uploads spec
       |
       v
JS: sessionStorage.setItem('exploreSpec', content)
       |
       v
POST /api/explore (multipart with spec)
       |
       v
Server: parse, cache (2min TTL), return hash
       |
       v
Response: full page with Operations tab + hash
       |
       v
User clicks [Schemas] tab
       |
       v
HTMX: GET /api/explore/schemas?h={hash}
       |
       +-- Cache HIT --> Return schemas partial
       |
       +-- Cache MISS --> Return 410 Gone
                              |
                              v
                         JS: intercept 410
                         POST spec from sessionStorage
                              |
                              v
                         Server: re-parse, continue
```

### API Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/explore` | GET | Empty explore page |
| `/api/explore` | POST | Upload, parse, cache, return full page |
| `/api/explore/operations` | GET | Operations tab partial |
| `/api/explore/schemas` | GET | Schemas tab partial |
| `/api/explore/security` | GET | Security tab partial |
| `/api/explore/summary-details` | GET | Expanded summary stats |
| `/api/explore/inline-schemas` | GET | Inline schema locations |
| `/api/explore/unsecured` | GET | Unsecured operations list |
| `/api/explore/operation/{id}` | GET | Single operation detail |
| `/api/explore/schema/{name}` | GET | Single schema detail |
| `/api/explore/security/{name}` | GET | Single security scheme detail |

All GET endpoints require `?h={hash}` parameter.

### Data Structures

```go
type ExploreAnalysis struct {
    Hash        string
    Version     string                      // "2.0", "3.0.3", etc.
    Filename    string
    ParseResult *parser.ParseResult
    Operations  *walker.OperationCollector
    Schemas     *walker.SchemaCollector
    Security    []SecuritySchemeInfo
    Stats       ExploreStats
}

type ExploreStats struct {
    PathCount      int
    OperationCount int
    SchemaCount    int            // component schemas only
    InlineCount    int
    SecuredCount   int
    UnsecuredCount int
    MethodCounts   map[string]int // "get": 5, "post": 3, etc.
}

type SecuritySchemeInfo struct {
    Name        string
    Type        string // apiKey, http, oauth2, openIdConnect, mutualTLS
    Scheme      string // for http: basic, bearer
    In          string // for apiKey: header, query, cookie
    ParamName   string // for apiKey
    Flows       []OAuthFlowInfo
    OpenIDURL   string
    UsageCount  int
}
```

---

## Files

### New Files

| File | Purpose |
|------|---------|
| `internal/api/explore.go` | Main handler |
| `internal/api/explore_partials.go` | HTMX partial handlers |
| `internal/api/explore_cache.go` | TTL cache |
| `internal/api/explore_test.go` | Unit tests |
| `internal/templates/explore.html` | Main template |
| `internal/templates/partials/explore_*.html` | Tab and detail partials (7 files) |
| `static/js/explore.js` | sessionStorage + cache fallback |
| `e2e/tests/explore.spec.ts` | E2E tests |
| `testdata/golden/explore/` | Golden test fixtures |

### Modified Files

| File | Changes |
|------|---------|
| `internal/api/handler.go` | Register explore routes |
| `internal/templates/base.html` | Add "Explore" nav link |
| `static/css/style.css` | Method badges, tabs, accordions |
| `internal/api/golden_test.go` | Add explore golden tests |

---

## Testing Strategy

### Three Layers

| Layer | Purpose | Files |
|-------|---------|-------|
| Unit | Cache, analysis, grouping logic | `explore_test.go` |
| Golden | Deterministic output verification | `golden_test.go` |
| E2E | Full browser UI flows | `explore.spec.ts` |

### Test Coverage

- OAS2 (Swagger) spec parsing
- OAS3.x spec parsing
- All three grouping modes (path, tag, method)
- All tab partials
- Cache hit and miss scenarios
- Browser fallback (E2E)

---

## Accessibility

Method badges use both glyphs AND colors to distinguish HTTP methods:
- Color alone is insufficient for colorblind users
- Glyphs provide shape-based differentiation
- Text labels (GET, POST, etc.) provide explicit identification

---

## Implementation Plan

See `docs/plans/2026-01-11-explore-page-implementation.md` for the detailed task-by-task implementation plan structured for subagent-driven development.
