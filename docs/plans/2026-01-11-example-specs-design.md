# Example Specs Design

**Date:** 2026-01-11
**Status:** Approved

## Overview

Provide curated sample OpenAPI specs on each page that demonstrate the feature's capabilities through a "Load Example" dropdown picker.

### Sourcing Strategy

Petstore as the recognizable base, with surgical modifications to create variants that illustrate specific scenarios.

### Spec Format

All examples in YAML (more readable), OpenAPI 3.0 as the primary version unless the feature specifically needs 2.0.

## Per-Page Examples

| Page | Examples | Purpose |
|------|----------|---------|
| **Validate** | "Petstore (Clean)", "With Warnings", "With Errors" | Show validation outcomes: pass, warnings only, errors |
| **Convert** | "Petstore 2.0", "Petstore 3.0" | Enable upgrade (2.0→3.x) and downgrade (3.0→2.0) exploration |
| **Diff** | "Petstore v1", "Petstore v2", "Petstore v3" | Mix-and-match to see safe changes (v1↔v2) or breaking changes (v1↔v3) |
| **Fix** | "Petstore (Messy)" | Contains multiple fixable issues: missing path params, duplicate operationIds, unused schemas |
| **Join** | "Users API", "Products API", "Orders API", "Inventory API" | First three merge cleanly; Inventory overlaps with Products to show collision handling |
| **Overlay** | Base: "Petstore" + Overlays: "Add Descriptions", "Add Security", "Public API" | Show different transformation use cases |
| **Explore** | "Petstore (Full Featured)" | Extended with tags, security, extensions, callbacks, links — all the bells and whistles |

## Example Details

### Validate Page

| Example | Content |
|---------|---------|
| **Petstore (Clean)** | Valid spec, no warnings — passes all validation modes |
| **With Warnings** | Valid but has best-practice issues: missing `operationId`, missing descriptions, trailing slashes |
| **With Errors** | Structural/semantic errors: broken `$ref` references, missing required fields, duplicate operationIds |

### Convert Page

| Example | Content |
|---------|---------|
| **Petstore 2.0** | Swagger 2.0 spec for upgrade to 3.0/3.1/3.2 |
| **Petstore 3.0** | OAS 3.0 spec for downgrade to 2.0 or upgrade to 3.1/3.2 |

### Diff Page

| Example | Content |
|---------|---------|
| **Petstore v1** | Baseline spec |
| **Petstore v2** | Safe evolution: new endpoints, optional params added |
| **Petstore v3** | Breaking changes: removed endpoints, changed types, required params added |

Comparing v1↔v2 shows safe changes; v1↔v3 or v2↔v3 shows breaking changes.

### Fix Page

| Example | Content |
|---------|---------|
| **Petstore (Messy)** | Multiple fixable issues: missing path params, duplicate operationIds, unused schemas, empty paths |

### Join Page

| Example | Content |
|---------|---------|
| **Users API** | User management endpoints |
| **Products API** | Product catalog endpoints |
| **Orders API** | Order processing endpoints |
| **Inventory API** | Intentionally overlaps with Products (shared `Product` schema, similar paths) |

Users+Products+Orders merge cleanly. Adding Inventory triggers collision handling.

### Overlay Page

| Example | Content |
|---------|---------|
| **Base: Petstore** | Uses `petstore-3.0.yaml` |
| **Add Descriptions** | Fills in missing operation summaries and descriptions |
| **Add Security** | Applies API key or OAuth security requirements |
| **Public API** | Removes internal/admin endpoints for external docs |

### Explore Page

| Example | Content |
|---------|---------|
| **Petstore (Full Featured)** | Extended with all features: multiple tags, security schemes, server variables, nested schemas, `x-*` extensions, examples, callbacks, links |

## File Structure

```
static/examples/
├── petstore-3.0.yaml           # Canonical clean 3.0 (Validate, Convert, Overlay base, Diff v1)
├── petstore-2.0.yaml           # Swagger 2.0 version
├── petstore-warnings.yaml      # Validate: warnings only
├── petstore-errors.yaml        # Validate: errors
├── petstore-v2.yaml            # Diff: safe changes
├── petstore-v3.yaml            # Diff: breaking changes
├── petstore-messy.yaml         # Fix: fixable issues
├── petstore-full.yaml          # Explore: extended with all features
├── users-api.yaml              # Join
├── products-api.yaml           # Join
├── orders-api.yaml             # Join
├── inventory-api.yaml          # Join (collisions)
├── overlay-descriptions.yaml   # Overlay
├── overlay-security.yaml       # Overlay
└── overlay-public.yaml         # Overlay
```

**15 files total** (with reuse of `petstore-3.0.yaml` across multiple pages)

## UI/UX

### Dropdown Placement

- Position near the file input, before the upload button
- Label: "Load Example" with a dropdown arrow
- Appears on all 7 feature pages

### Dropdown Behavior

```
┌─────────────────────────────────┐
│ Load Example ▾                  │
├─────────────────────────────────┤
│ Petstore (Clean)                │
│ Petstore (With Warnings)        │
│ Petstore (With Errors)          │
└─────────────────────────────────┘
```

- Selecting an example fetches the YAML content via `/api/examples/{name}`
- Automatically switches input mode to "Paste" and populates the textarea
- Clears any previously uploaded file
- For multi-input pages (Diff, Join, Overlay): separate dropdowns for each input field

### Loading Mechanism

- Embed examples via Go's `embed.FS` (already used for static assets)
- Add `/api/examples/{name}` endpoint to serve example content
- Frontend: dropdown triggers fetch, populates the input textarea

## Implementation Notes

### Reuse Strategy

Several pages use the same canonical Petstore:
- `petstore-3.0.yaml` serves as: Validate (Clean), Convert (3.0 source), Overlay (base), Diff (v1)

This reduces maintenance by having one source of truth for the clean baseline.

### Validation Example Details

Based on oastools validator capabilities:

**Warnings (best-practice issues):**
- Missing `operationId` on operations
- Missing `summary` or `description`
- Trailing slashes on paths (e.g., `/users/`)
- Non-standard HTTP status codes in StrictMode

**Errors (structural/semantic issues):**
- Missing required `info.version`
- Broken `$ref` references (`#/components/schemas/NonExistent`)
- Duplicate `operationId` values
- Path parameters in template but not declared
- Missing `responses` object on operations

### Fix Example Details

Based on oastools fixer capabilities, include:
- Missing path parameters (e.g., `/users/{userId}` without `userId` declared)
- Duplicate `operationId` values
- Unused schema definitions
- Invalid schema names with brackets (e.g., `Response[User]`)
