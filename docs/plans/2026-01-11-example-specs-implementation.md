# Example Specs Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add curated sample OpenAPI specs to each feature page via a "Load Example" dropdown.

**Architecture:** Example specs are embedded via Go's `embed.FS` and served through a new `/api/examples/{name}` endpoint. Each page template includes a dropdown that fetches examples and populates the paste textarea.

**Tech Stack:** Go embed.FS, HTMX, vanilla JavaScript

---

## Task 1: Create Example Specs Directory and Embed

**Files:**
- Create: `static/examples/embed.go`
- Create: `static/examples/petstore-3.0.yaml`

**Step 1: Create the examples embed file**

Create `static/examples/embed.go`:
```go
package examples

import "embed"

//go:embed *.yaml
var FS embed.FS
```

**Step 2: Create the canonical Petstore 3.0 spec**

Create `static/examples/petstore-3.0.yaml` with a clean, valid Petstore spec. Use the standard Petstore example with:
- `/pets` and `/pets/{petId}` paths
- `Pet`, `Error` schemas
- All operations have `operationId`, `summary`, `description`
- Proper responses with descriptions

Source from: https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/examples/v3.0/petstore.yaml

Trim to essentials (~100 lines) to keep examples lightweight.

**Step 3: Verify the file compiles**

Run: `cd /Users/robbie/code/oastools-web/.worktrees/example-specs && go build ./...`
Expected: Build succeeds

**Step 4: Commit**

```bash
git add static/examples/
git commit -m "feat(examples): add examples embed and petstore-3.0 base spec"
```

---

## Task 2: Create Petstore 2.0 Variant

**Files:**
- Create: `static/examples/petstore-2.0.yaml`

**Step 1: Create Swagger 2.0 version**

Create `static/examples/petstore-2.0.yaml` - equivalent to petstore-3.0 but in Swagger 2.0 format:
- `swagger: "2.0"` instead of `openapi: "3.0.0"`
- `definitions` instead of `components/schemas`
- `produces`/`consumes` instead of `content` media types
- `host` and `basePath` instead of `servers`

**Step 2: Validate the spec parses correctly**

Run: `cd /Users/robbie/code/oastools-web/.worktrees/example-specs && go test -run TestExamples -v ./...` (will create this test later)

**Step 3: Commit**

```bash
git add static/examples/petstore-2.0.yaml
git commit -m "feat(examples): add petstore-2.0 for convert page"
```

---

## Task 3: Create Validation Example Specs

**Files:**
- Create: `static/examples/petstore-warnings.yaml`
- Create: `static/examples/petstore-errors.yaml`

**Step 1: Create spec with warnings only**

Create `static/examples/petstore-warnings.yaml` - valid but has best-practice issues:
- Missing `operationId` on some operations
- Missing `description` on operations
- Trailing slash on a path (e.g., `/pets/`)
- Missing `summary` on operations

**Step 2: Create spec with errors**

Create `static/examples/petstore-errors.yaml` - has structural/semantic errors:
- Missing `info.version`
- Broken `$ref` (e.g., `$ref: '#/components/schemas/NonExistent'`)
- Duplicate `operationId` values
- Path parameter in template but not declared (e.g., `/pets/{petId}` without `petId` parameter)
- Missing `responses` object on an operation

**Step 3: Commit**

```bash
git add static/examples/petstore-warnings.yaml static/examples/petstore-errors.yaml
git commit -m "feat(examples): add validation example specs with warnings and errors"
```

---

## Task 4: Create Diff Example Specs

**Files:**
- Create: `static/examples/petstore-v2.yaml`
- Create: `static/examples/petstore-v3.yaml`

**Step 1: Create v2 with safe changes**

Create `static/examples/petstore-v2.yaml` - evolution from petstore-3.0 with non-breaking changes:
- Add new endpoint `/pets/{petId}/photos` (addition)
- Add optional query parameter `limit` to `GET /pets` (addition)
- Add new schema `Photo` (addition)
- Expand enum values (safe change)

**Step 2: Create v3 with breaking changes**

Create `static/examples/petstore-v3.yaml` - breaking changes from petstore-3.0:
- Remove `DELETE /pets/{petId}` endpoint (removal)
- Change `petId` from `integer` to `string` (type change)
- Add required parameter `apiKey` to existing endpoint (breaking)
- Remove property from `Pet` schema (removal)

**Step 3: Commit**

```bash
git add static/examples/petstore-v2.yaml static/examples/petstore-v3.yaml
git commit -m "feat(examples): add diff example specs v2 (safe) and v3 (breaking)"
```

---

## Task 5: Create Fix Example Spec

**Files:**
- Create: `static/examples/petstore-messy.yaml`

**Step 1: Create spec with fixable issues**

Create `static/examples/petstore-messy.yaml` - has issues the fixer can auto-repair:
- Missing path parameter declarations (e.g., `/pets/{petId}` without parameter object)
- Duplicate `operationId` values
- Unused schema in components (never referenced)
- Empty path item (no operations, only parameters)

**Step 2: Commit**

```bash
git add static/examples/petstore-messy.yaml
git commit -m "feat(examples): add petstore-messy with fixable issues"
```

---

## Task 6: Create Join Example Specs

**Files:**
- Create: `static/examples/users-api.yaml`
- Create: `static/examples/products-api.yaml`
- Create: `static/examples/orders-api.yaml`
- Create: `static/examples/inventory-api.yaml`

**Step 1: Create Users API**

Create `static/examples/users-api.yaml` - small microservice spec:
- `GET /users`, `POST /users`, `GET /users/{userId}`
- `User` schema
- Clean, no conflicts with other specs

**Step 2: Create Products API**

Create `static/examples/products-api.yaml`:
- `GET /products`, `POST /products`, `GET /products/{productId}`
- `Product` schema
- Clean, no conflicts

**Step 3: Create Orders API**

Create `static/examples/orders-api.yaml`:
- `GET /orders`, `POST /orders`, `GET /orders/{orderId}`
- `Order` schema (references `Product` by ID, not $ref)
- Clean, no conflicts

**Step 4: Create Inventory API with intentional collisions**

Create `static/examples/inventory-api.yaml`:
- `GET /products` (same path as Products API - collision!)
- `Product` schema (same name as Products API - collision!)
- `GET /inventory`, `POST /inventory/{productId}/stock`

**Step 5: Commit**

```bash
git add static/examples/users-api.yaml static/examples/products-api.yaml static/examples/orders-api.yaml static/examples/inventory-api.yaml
git commit -m "feat(examples): add join example specs (users, products, orders, inventory)"
```

---

## Task 7: Create Overlay Example Specs

**Files:**
- Create: `static/examples/overlay-descriptions.yaml`
- Create: `static/examples/overlay-security.yaml`
- Create: `static/examples/overlay-public.yaml`

**Step 1: Create Add Descriptions overlay**

Create `static/examples/overlay-descriptions.yaml`:
```yaml
overlay: "1.0.0"
info:
  title: Add Descriptions
  version: "1.0.0"
actions:
  - target: $.paths['/pets'].get
    update:
      description: Returns all pets from the system that the user has access to.
  - target: $.paths['/pets'].post
    update:
      description: Creates a new pet in the store. Duplicates are allowed.
  - target: $.paths['/pets/{petId}'].get
    update:
      description: Returns a pet based on a single ID.
```

**Step 2: Create Add Security overlay**

Create `static/examples/overlay-security.yaml`:
```yaml
overlay: "1.0.0"
info:
  title: Add Security
  version: "1.0.0"
actions:
  - target: $.components
    update:
      securitySchemes:
        ApiKeyAuth:
          type: apiKey
          in: header
          name: X-API-Key
  - target: $
    update:
      security:
        - ApiKeyAuth: []
```

**Step 3: Create Public API overlay**

Create `static/examples/overlay-public.yaml`:
```yaml
overlay: "1.0.0"
info:
  title: Public API
  version: "1.0.0"
actions:
  - target: $.paths['/pets/{petId}'].delete
    remove: true
  - target: $.paths['/admin']
    remove: true
  - target: $.info
    update:
      title: Petstore Public API
      description: Public-facing API for pet operations (read-only)
```

**Step 4: Commit**

```bash
git add static/examples/overlay-descriptions.yaml static/examples/overlay-security.yaml static/examples/overlay-public.yaml
git commit -m "feat(examples): add overlay example specs"
```

---

## Task 8: Create Explore Full-Featured Spec

**Files:**
- Create: `static/examples/petstore-full.yaml`

**Step 1: Create extended Petstore with all features**

Create `static/examples/petstore-full.yaml` - showcase all OpenAPI features:
- Multiple tags with descriptions
- Multiple servers with variables
- Security schemes (API key, OAuth2)
- Callbacks
- Links
- Examples on operations and schemas
- `x-*` extensions (e.g., `x-codegen-request-body-name`, `x-internal`)
- Webhooks (if OAS 3.1)
- Deeply nested schemas
- All HTTP methods represented

**Step 2: Commit**

```bash
git add static/examples/petstore-full.yaml
git commit -m "feat(examples): add petstore-full with all OpenAPI features for explore"
```

---

## Task 9: Add Examples API Endpoint

**Files:**
- Create: `internal/api/examples.go`
- Modify: `internal/api/routes.go`
- Modify: `internal/api/handler.go`

**Step 1: Write the failing test**

Create test in `internal/api/examples_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetExample(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name       string
		example    string
		wantStatus int
		wantType   string
	}{
		{
			name:       "valid example",
			example:    "petstore-3.0",
			wantStatus: http.StatusOK,
			wantType:   "text/yaml; charset=utf-8",
		},
		{
			name:       "not found",
			example:    "nonexistent",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/examples/"+tt.example, nil)
			w := httptest.NewRecorder()

			h.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantType != "" && w.Header().Get("Content-Type") != tt.wantType {
				t.Errorf("Content-Type = %q, want %q", w.Header().Get("Content-Type"), tt.wantType)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -run TestHandleGetExample -v ./internal/api/`
Expected: FAIL (handler doesn't exist yet)

**Step 3: Create examples handler**

Create `internal/api/examples.go`:
```go
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/erraggy/oastools-web/static/examples"
	"github.com/erraggy/oastools/builder"
)

// ExampleMetadata describes an available example spec.
type ExampleMetadata struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// handleGetExample serves an example spec by name.
func (h *Handler) handleGetExample(_ context.Context, req *builder.Request) builder.Response {
	name := req.PathParams["name"]
	if name == "" {
		return builder.Error(http.StatusBadRequest, "missing example name")
	}

	// Sanitize: only allow alphanumeric, dash, underscore
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return builder.Error(http.StatusBadRequest, "invalid example name")
		}
	}

	filename := name + ".yaml"
	content, err := examples.FS.ReadFile(filename)
	if err != nil {
		return builder.Error(http.StatusNotFound, "example not found")
	}

	return builder.Raw(http.StatusOK, "text/yaml; charset=utf-8", content)
}

// handleListExamples returns metadata for all available examples.
func (h *Handler) handleListExamples(_ context.Context, _ *builder.Request) builder.Response {
	entries, err := examples.FS.ReadDir(".")
	if err != nil {
		return builder.Error(http.StatusInternalServerError, "failed to read examples")
	}

	var list []ExampleMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		list = append(list, ExampleMetadata{
			Name:  name,
			Label: exampleLabel(name),
		})
	}

	return builder.JSON(http.StatusOK, list)
}

// exampleLabel returns a human-readable label for an example name.
func exampleLabel(name string) string {
	labels := map[string]string{
		"petstore-3.0":          "Petstore (Clean)",
		"petstore-2.0":          "Petstore 2.0",
		"petstore-warnings":     "Petstore (With Warnings)",
		"petstore-errors":       "Petstore (With Errors)",
		"petstore-v2":           "Petstore v2 (Safe Changes)",
		"petstore-v3":           "Petstore v3 (Breaking Changes)",
		"petstore-messy":        "Petstore (Messy)",
		"petstore-full":         "Petstore (Full Featured)",
		"users-api":             "Users API",
		"products-api":          "Products API",
		"orders-api":            "Orders API",
		"inventory-api":         "Inventory API",
		"overlay-descriptions":  "Add Descriptions",
		"overlay-security":      "Add Security",
		"overlay-public":        "Public API",
	}
	if label, ok := labels[name]; ok {
		return label
	}
	return name
}
```

**Step 4: Register routes in routes.go**

Add to `internal/api/routes.go` in `registerOperations`:
```go
// GET /api/examples - list available examples
srv.AddOperation(http.MethodGet, "/api/examples",
	builder.WithOperationID("listExamples"),
	builder.WithSummary("List available example specifications"),
	builder.WithTags("examples"),
	builder.WithResponse(http.StatusOK, []ExampleMetadata{},
		builder.WithResponseDescription("List of available examples"),
	),
	builder.WithHandler(h.handleListExamples),
)

// GET /api/examples/{name} - get example spec content
srv.AddOperation(http.MethodGet, "/api/examples/{name}",
	builder.WithOperationID("getExample"),
	builder.WithSummary("Get example specification content"),
	builder.WithTags("examples"),
	builder.WithPathParam("name", "string",
		builder.WithParamDescription("Example name (without .yaml extension)"),
		builder.WithParamRequired(true),
	),
	builder.WithResponse(http.StatusOK, nil,
		builder.WithResponseDescription("Example specification in YAML format"),
	),
	builder.WithResponse(http.StatusNotFound, ErrorResponse{},
		builder.WithResponseDescription("Example not found"),
	),
	builder.WithHandler(h.handleGetExample),
)
```

**Step 5: Update handler.go imports**

Add import for examples package in `internal/api/handler.go`:
```go
import (
	// ... existing imports
	"github.com/erraggy/oastools-web/static/examples"
)
```

Note: The import may already be used implicitly; verify it compiles.

**Step 6: Run tests**

Run: `go test -run TestHandleGetExample -v ./internal/api/`
Expected: PASS

**Step 7: Run all tests**

Run: `make test`
Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/api/examples.go internal/api/examples_test.go internal/api/routes.go
git commit -m "feat(api): add /api/examples endpoints to serve example specs"
```

---

## Task 10: Add Example Dropdown to Templates

**Files:**
- Modify: `internal/templates/validate.html`
- Modify: `static/js/app.js`

**Step 1: Add dropdown HTML to validate.html**

Update `internal/templates/validate.html`, add after the `<label>Specification</label>` line:
```html
<div class="example-picker">
    <select id="example-select" onchange="loadExample(this, 'spec')">
        <option value="">Load Example...</option>
        <option value="petstore-3.0">Petstore (Clean)</option>
        <option value="petstore-warnings">Petstore (With Warnings)</option>
        <option value="petstore-errors">Petstore (With Errors)</option>
    </select>
</div>
```

**Step 2: Add loadExample function to app.js**

Add to `static/js/app.js`:
```javascript
// Load example spec into input section
async function loadExample(select, fieldName) {
    const exampleName = select.value;
    if (!exampleName) return;

    const section = select.closest('.input-section');
    if (!section) return;

    try {
        const response = await fetch(`/api/examples/${exampleName}`);
        if (!response.ok) {
            throw new Error(`Failed to load example: ${response.statusText}`);
        }
        const content = await response.text();

        // Switch to paste mode
        switchInputMode(section, 'paste');

        // Find and populate the textarea
        const textarea = section.querySelector('textarea');
        if (textarea) {
            textarea.value = content;
            textarea.disabled = false;
        }

        // Reset the select
        select.value = '';
    } catch (error) {
        console.error('Failed to load example:', error);
        alert('Failed to load example. Please try again.');
        select.value = '';
    }
}
```

**Step 3: Build and test manually**

Run: `make build && make run`
Open: http://localhost:8080/validate
Test: Select an example from dropdown, verify it loads into paste textarea

**Step 4: Commit**

```bash
git add internal/templates/validate.html static/js/app.js
git commit -m "feat(ui): add example dropdown to validate page"
```

---

## Task 11: Add Example Dropdowns to Remaining Pages

**Files:**
- Modify: `internal/templates/convert.html`
- Modify: `internal/templates/diff.html`
- Modify: `internal/templates/fix.html`
- Modify: `internal/templates/join.html`
- Modify: `internal/templates/overlay.html`
- Modify: `internal/templates/explore.html`

**Step 1: Update convert.html**

Add dropdown after `<label>Specification</label>`:
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'spec')">
        <option value="">Load Example...</option>
        <option value="petstore-2.0">Petstore 2.0</option>
        <option value="petstore-3.0">Petstore 3.0</option>
    </select>
</div>
```

**Step 2: Update diff.html**

Add dropdown for base spec (after base label):
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'base')">
        <option value="">Load Example...</option>
        <option value="petstore-3.0">Petstore v1 (Base)</option>
        <option value="petstore-v2">Petstore v2 (Safe Changes)</option>
        <option value="petstore-v3">Petstore v3 (Breaking)</option>
    </select>
</div>
```

Add dropdown for head spec (after head label):
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'head')">
        <option value="">Load Example...</option>
        <option value="petstore-3.0">Petstore v1 (Base)</option>
        <option value="petstore-v2">Petstore v2 (Safe Changes)</option>
        <option value="petstore-v3">Petstore v3 (Breaking)</option>
    </select>
</div>
```

**Step 3: Update fix.html**

Add dropdown:
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'spec')">
        <option value="">Load Example...</option>
        <option value="petstore-messy">Petstore (Messy)</option>
        <option value="petstore-3.0">Petstore (Clean)</option>
    </select>
</div>
```

**Step 4: Update join.html**

Note: Join uses multiple file inputs. Add a dropdown that appends to the spec list.
This requires modifying the `loadExample` function or creating a new `loadJoinExample` function.

For simplicity, add examples that users can load one at a time:
```html
<div class="example-picker">
    <select onchange="loadJoinExample(this)">
        <option value="">Add Example...</option>
        <option value="users-api">Users API</option>
        <option value="products-api">Products API</option>
        <option value="orders-api">Orders API</option>
        <option value="inventory-api">Inventory API (Conflicts)</option>
    </select>
</div>
```

Add to app.js:
```javascript
// Load example for join page (adds to paste inputs)
async function loadJoinExample(select) {
    const exampleName = select.value;
    if (!exampleName) return;

    try {
        const response = await fetch(`/api/examples/${exampleName}`);
        if (!response.ok) throw new Error(`Failed to load: ${response.statusText}`);
        const content = await response.text();

        // Find or create a paste textarea for join
        const container = document.querySelector('.join-inputs') || document.querySelector('form');
        // Implementation depends on join page structure
        // For now, alert with content for manual paste

        // Copy to clipboard as fallback
        await navigator.clipboard.writeText(content);
        alert(`${exampleName} copied to clipboard. Paste it into one of the spec inputs.`);

        select.value = '';
    } catch (error) {
        console.error('Failed to load example:', error);
        alert('Failed to load example.');
        select.value = '';
    }
}
```

**Step 5: Update overlay.html**

Add dropdown for base spec:
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'spec')">
        <option value="">Load Example...</option>
        <option value="petstore-3.0">Petstore (Base)</option>
    </select>
</div>
```

Add dropdown for overlay:
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'overlay')">
        <option value="">Load Example...</option>
        <option value="overlay-descriptions">Add Descriptions</option>
        <option value="overlay-security">Add Security</option>
        <option value="overlay-public">Public API</option>
    </select>
</div>
```

**Step 6: Update explore.html**

Add dropdown:
```html
<div class="example-picker">
    <select onchange="loadExample(this, 'spec')">
        <option value="">Load Example...</option>
        <option value="petstore-full">Petstore (Full Featured)</option>
        <option value="petstore-3.0">Petstore (Clean)</option>
    </select>
</div>
```

**Step 7: Commit**

```bash
git add internal/templates/*.html static/js/app.js
git commit -m "feat(ui): add example dropdowns to all feature pages"
```

---

## Task 12: Add CSS Styling for Dropdowns

**Files:**
- Modify: `static/css/style.css`

**Step 1: Add example-picker styles**

Add to `static/css/style.css`:
```css
/* Example picker dropdown */
.example-picker {
    margin-bottom: 0.5rem;
}

.example-picker select {
    padding: 0.375rem 0.75rem;
    font-size: 0.875rem;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    background-color: var(--bg-secondary);
    color: var(--text-primary);
    cursor: pointer;
}

.example-picker select:hover {
    border-color: var(--primary-color);
}

.example-picker select:focus {
    outline: none;
    border-color: var(--primary-color);
    box-shadow: 0 0 0 2px rgba(var(--primary-rgb), 0.2);
}
```

**Step 2: Build and verify styling**

Run: `make build && make run`
Open each page and verify dropdowns look correct

**Step 3: Commit**

```bash
git add static/css/style.css
git commit -m "feat(ui): add example picker dropdown styles"
```

---

## Task 13: Add Integration Tests for Examples

**Files:**
- Modify: `internal/api/examples_test.go`

**Step 1: Add comprehensive tests**

Expand `internal/api/examples_test.go`:
```go
func TestHandleListExamples(t *testing.T) {
	h := minimalHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/examples", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify response contains expected examples
	body := w.Body.String()
	expectedExamples := []string{"petstore-3.0", "petstore-2.0", "petstore-warnings"}
	for _, ex := range expectedExamples {
		if !strings.Contains(body, ex) {
			t.Errorf("response missing example %q", ex)
		}
	}
}

func TestExampleSpecsAreValid(t *testing.T) {
	// Test that all example specs parse correctly
	entries, err := examples.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read examples: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		if strings.HasPrefix(entry.Name(), "overlay-") {
			continue // Skip overlay files, they're not full specs
		}

		t.Run(entry.Name(), func(t *testing.T) {
			content, err := examples.FS.ReadFile(entry.Name())
			if err != nil {
				t.Fatalf("failed to read %s: %v", entry.Name(), err)
			}

			_, err = parser.Parse(content)
			if err != nil && !strings.Contains(entry.Name(), "errors") {
				t.Errorf("failed to parse %s: %v", entry.Name(), err)
			}
		})
	}
}
```

**Step 2: Run tests**

Run: `make test`
Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/api/examples_test.go
git commit -m "test(examples): add integration tests for example specs"
```

---

## Task 14: Update README

**Files:**
- Modify: `README.md`

**Step 1: Add section about examples**

Add to README.md in the Features section:
```markdown
### Example Specifications

Each page includes a "Load Example" dropdown with curated sample specifications:

- **Validate**: Clean spec, spec with warnings, spec with errors
- **Convert**: Petstore 2.0 and 3.0 for version conversion
- **Diff**: Three versions to compare (v1 base, v2 safe changes, v3 breaking changes)
- **Fix**: Messy spec with auto-fixable issues
- **Join**: Four microservice specs (Users, Products, Orders, Inventory)
- **Overlay**: Base spec with three overlay examples
- **Explore**: Full-featured Petstore with all OpenAPI features
```

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add example specs feature to README"
```

---

## Task 15: Final Verification

**Step 1: Run full test suite**

Run: `make test && make lint`
Expected: All tests pass, no lint errors

**Step 2: Manual testing**

Run: `make build && make run`

Test each page:
1. Validate - load each example, verify validation results match expectations
2. Convert - load 2.0, convert to 3.0; load 3.0, convert to 2.0
3. Diff - compare v1 vs v2 (safe), v1 vs v3 (breaking)
4. Fix - load messy, run fix, verify issues are fixed
5. Join - load users + products + orders (clean), add inventory (conflicts)
6. Overlay - load petstore + each overlay, verify transformations
7. Explore - load full-featured, explore all sections

**Step 3: Create final commit if any fixes needed**

```bash
git add -A
git commit -m "fix: address issues found in final verification"
```

---

## Summary

**15 example spec files:**
- petstore-3.0.yaml (canonical clean)
- petstore-2.0.yaml (Swagger 2.0)
- petstore-warnings.yaml (validation warnings)
- petstore-errors.yaml (validation errors)
- petstore-v2.yaml (safe diff changes)
- petstore-v3.yaml (breaking diff changes)
- petstore-messy.yaml (fixable issues)
- petstore-full.yaml (all features for explore)
- users-api.yaml, products-api.yaml, orders-api.yaml, inventory-api.yaml (join examples)
- overlay-descriptions.yaml, overlay-security.yaml, overlay-public.yaml (overlay examples)

**New API endpoints:**
- GET /api/examples - list available examples
- GET /api/examples/{name} - get example content

**UI changes:**
- "Load Example" dropdown on all 7 feature pages
- CSS styling for dropdowns
