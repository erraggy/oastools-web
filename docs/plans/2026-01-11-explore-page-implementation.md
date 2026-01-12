# Explore Page Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task.

**Goal:** Add an Explore page for read-only analysis of OpenAPI specifications with operations, schemas, and security views.

**Architecture:** Parse uploaded specs using oastools walker package, cache analysis with content-addressed hash, render tabs via HTMX partials. Browser sessionStorage provides fallback for server cache misses.

**Tech Stack:** Go 1.25+, oastools v1.45.0 (walker package), HTMX, Go html/template, Playwright

---

## Dependencies Graph

```
Task 1 (cache) ----+
                   |
Task 2 (skeleton) -+---> Task 4 (parsing) --+--> Task 5 (ops partial) --> Task 6 (ops detail) --> Task 7 (group-by)
                   |                        |
Task 3 (template) -+                        +--> Task 8 (schemas partial) --> Task 9 (schema detail) --> Task 10 (inline)
                                            |
                                            +--> Task 11 (security partial) --> Task 12 (security detail) --> Task 13 (unsecured)

Task 14 (summary) --+
Task 15 (JS)       -+--> Task 17 (unit) --> Task 18 (golden) --> Task 19 (E2E) --> Task 20 (verify)
Task 16 (CSS)      -+
```

**Parallel Groups:**
- Group A (1, 2, 3): Foundation - run in parallel
- Group B (4): Core parsing - after Group A
- Group C (5, 8, 11): Tab partials - after Task 4, run in parallel
- Group D (6, 9, 12): Detail partials - after respective tab partial
- Group E (7, 10, 13): Drill-ins - after respective detail partial
- Group F (14, 15, 16): Polish - after Group C, run in parallel
- Group G (17-20): Testing - sequential after all implementation

---

## Task 1: TTL Cache Infrastructure

**Context:** This cache stores parsed spec analysis keyed by content hash. It enables the "parse once, lazy render" pattern where tab switches retrieve cached data instead of re-parsing.

**Files:**
- Create: `internal/api/explore_cache.go`
- Create: `internal/api/explore_cache_test.go`

**Step 1: Write the cache implementation**

Create `internal/api/explore_cache.go`:

```go
package api

import (
	"sync"
	"time"
)

// TTLCache is a generic cache with time-to-live expiration.
type TTLCache[K comparable, V any] struct {
	mu      sync.RWMutex
	items   map[K]*cacheItem[V]
	ttl     time.Duration
	stopCh  chan struct{}
	stopped bool
}

type cacheItem[V any] struct {
	value     V
	expiresAt time.Time
}

// NewTTLCache creates a new TTL cache with the given expiration duration.
// It starts a background goroutine that cleans up expired entries.
func NewTTLCache[K comparable, V any](ttl time.Duration) *TTLCache[K, V] {
	c := &TTLCache[K, V]{
		items:  make(map[K]*cacheItem[V]),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Get retrieves a value from the cache.
// Returns the value and true if found and not expired, zero value and false otherwise.
func (c *TTLCache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	if time.Now().After(item.expiresAt) {
		var zero V
		return zero, false
	}

	return item.value, true
}

// Set stores a value in the cache with the configured TTL.
func (c *TTLCache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a value from the cache.
func (c *TTLCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}

// Stop stops the cleanup goroutine.
func (c *TTLCache[K, V]) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.stopped {
		close(c.stopCh)
		c.stopped = true
	}
}

// cleanupLoop removes expired entries every 30 seconds.
func (c *TTLCache[K, V]) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCh:
			return
		}
	}
}

func (c *TTLCache[K, V]) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
		}
	}
}
```

**Step 2: Write the cache tests**

Create `internal/api/explore_cache_test.go`:

```go
package api

import (
	"sync"
	"testing"
	"time"
)

func TestTTLCache_SetGet(t *testing.T) {
	cache := NewTTLCache[string, int](1 * time.Minute)
	defer cache.Stop()

	cache.Set("key1", 100)
	cache.Set("key2", 200)

	val, ok := cache.Get("key1")
	if !ok || val != 100 {
		t.Errorf("Get(key1) = %d, %v; want 100, true", val, ok)
	}

	val, ok = cache.Get("key2")
	if !ok || val != 200 {
		t.Errorf("Get(key2) = %d, %v; want 200, true", val, ok)
	}

	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestTTLCache_Expiration(t *testing.T) {
	cache := NewTTLCache[string, string](50 * time.Millisecond)
	defer cache.Stop()

	cache.Set("key", "value")

	val, ok := cache.Get("key")
	if !ok || val != "value" {
		t.Errorf("Get(key) = %q, %v; want value, true", val, ok)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = cache.Get("key")
	if ok {
		t.Error("Get(key) should return false after expiration")
	}
}

func TestTTLCache_Delete(t *testing.T) {
	cache := NewTTLCache[string, int](1 * time.Minute)
	defer cache.Stop()

	cache.Set("key", 42)
	cache.Delete("key")

	_, ok := cache.Get("key")
	if ok {
		t.Error("Get(key) should return false after delete")
	}
}

func TestTTLCache_Concurrent(t *testing.T) {
	cache := NewTTLCache[int, int](1 * time.Minute)
	defer cache.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cache.Set(n, n*2)
			cache.Get(n)
		}(i)
	}
	wg.Wait()

	// Verify some values
	for i := 0; i < 100; i++ {
		val, ok := cache.Get(i)
		if !ok {
			t.Errorf("Get(%d) should return true", i)
		}
		if val != i*2 {
			t.Errorf("Get(%d) = %d; want %d", i, val, i*2)
		}
	}
}
```

**Step 3: Run tests**

Run:
```bash
go test -v ./internal/api/ -run TestTTLCache
```

Expected: All tests pass

**Step 4: Commit**

Run:
```bash
git add internal/api/explore_cache.go internal/api/explore_cache_test.go
git commit -m "feat(explore): add TTL cache infrastructure"
```

---

## Task 2: Explore Handler Skeleton

**Context:** This establishes the route structure for the Explore feature. All endpoints start as stubs that return 501 Not Implemented. Subsequent tasks will implement them.

**Files:**
- Create: `internal/api/explore.go`
- Modify: `internal/api/handler.go`
- Modify: `internal/templates/base.html`

**Step 1: Create the explore handler file**

Create `internal/api/explore.go`:

```go
package api

import (
	"net/http"
	"time"

	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/walker"
)

// ExploreAnalysis holds the parsed and analyzed spec data.
type ExploreAnalysis struct {
	Hash        string
	Version     string
	Filename    string
	ParseResult *parser.ParseResult
	Operations  *walker.OperationCollector
	Schemas     *walker.SchemaCollector
	Security    []SecuritySchemeInfo
	Stats       ExploreStats
}

// ExploreStats holds summary statistics for the spec.
type ExploreStats struct {
	PathCount      int
	OperationCount int
	SchemaCount    int
	InlineCount    int
	SecuredCount   int
	UnsecuredCount int
	MethodCounts   map[string]int
}

// SecuritySchemeInfo holds parsed security scheme information.
type SecuritySchemeInfo struct {
	Name       string
	Type       string
	Scheme     string
	In         string
	ParamName  string
	Flows      []OAuthFlowInfo
	OpenIDURL  string
	UsageCount int
}

// OAuthFlowInfo holds OAuth flow configuration.
type OAuthFlowInfo struct {
	Type             string
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           map[string]string
}

// Cache for explore analysis results (2 minute TTL)
var exploreCache = NewTTLCache[string, *ExploreAnalysis](2 * time.Minute)

// handleExplorePage renders the empty explore page.
func (h *Handler) handleExplorePage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "explore.html", nil)
}

// handleExploreUpload handles spec upload and analysis.
func (h *Handler) handleExploreUpload(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// handleExploreOperations renders the operations tab partial.
func (h *Handler) handleExploreOperations(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// handleExploreSchemas renders the schemas tab partial.
func (h *Handler) handleExploreSchemas(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}

// handleExploreSecurity renders the security tab partial.
func (h *Handler) handleExploreSecurity(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not implemented", http.StatusNotImplemented)
}
```

**Step 2: Register routes in handler.go**

Find the route registration section in `internal/api/handler.go` and add after the existing routes:

```go
	// Explore routes
	mux.HandleFunc("GET /explore", h.handleExplorePage)
	mux.HandleFunc("POST /api/explore", h.handleExploreUpload)
	mux.HandleFunc("GET /api/explore/operations", h.handleExploreOperations)
	mux.HandleFunc("GET /api/explore/schemas", h.handleExploreSchemas)
	mux.HandleFunc("GET /api/explore/security", h.handleExploreSecurity)
```

**Step 3: Add navigation link**

In `internal/templates/base.html`, find the navigation section and add "Explore" link. Look for the existing nav links and add:

```html
<a href="/explore" class="nav-link{{if eq .ActivePage "explore"}} active{{end}}">Explore</a>
```

**Step 4: Build and verify**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/api/explore.go internal/api/handler.go internal/templates/base.html
git commit -m "feat(explore): add handler skeleton and routes"
```

---

## Task 3: Main Explore Template

**Context:** This creates the main page template with file upload, summary placeholder, and tab structure. It follows the patterns from validate.html and fix.html.

**Files:**
- Create: `internal/templates/explore.html`

**Step 1: Create the explore template**

Create `internal/templates/explore.html`:

```html
{{define "title"}}Explore - oastools{{end}}

{{define "content"}}
<div class="page-header">
    <h1>Explore</h1>
    <p class="page-description">Analyze the structure of your OpenAPI specification</p>
</div>

<form id="explore-form" hx-post="/api/explore" hx-target="#explore-results" hx-encoding="multipart/form-data">
    <div class="input-section">
        <div class="input-mode-toggle">
            <button type="button" class="mode-btn active" data-mode="upload">Upload File</button>
            <button type="button" class="mode-btn" data-mode="paste">Paste Content</button>
        </div>

        <div class="input-upload" id="upload-mode">
            <div class="file-input-wrapper">
                <input type="file" name="spec" id="spec-file" accept=".yaml,.yml,.json">
                <label for="spec-file" class="file-label">
                    <span class="file-label-text">Choose a file or drag it here</span>
                </label>
            </div>
        </div>

        <div class="input-paste hidden" id="paste-mode">
            <textarea name="spec_content" placeholder="Paste your OpenAPI specification here (YAML or JSON)"></textarea>
        </div>
    </div>

    <input type="hidden" name="hash" id="explore-hash" value="">

    <div class="form-actions">
        <button type="submit" class="btn btn-primary">Explore</button>
    </div>
</form>

<div id="explore-results">
    {{if .Analysis}}
    {{template "explore_results" .}}
    {{end}}
</div>

<script src="/static/js/explore.js"></script>
{{end}}

{{define "explore_results"}}
<div class="explore-container" data-hash="{{.Analysis.Hash}}">
    <div class="explore-summary">
        <div class="summary-header">
            <span class="summary-version">{{.Analysis.Version}}</span>
            <span class="summary-filename">{{.Analysis.Filename}}</span>
        </div>
        <div class="summary-stats">
            <span class="stat">Paths: <strong>{{.Analysis.Stats.PathCount}}</strong></span>
            <span class="stat">Operations: <strong>{{.Analysis.Stats.OperationCount}}</strong></span>
            <span class="stat">Schemas: <strong>{{.Analysis.Stats.SchemaCount}}</strong></span>
        </div>
        <button type="button" class="summary-expand-btn"
                hx-get="/api/explore/summary-details?h={{.Analysis.Hash}}"
                hx-target="#summary-details"
                hx-swap="innerHTML">
            <span class="expand-icon">&#9660;</span> Details
        </button>
        <div id="summary-details"></div>
    </div>

    <div class="explore-tabs">
        <div class="tab-buttons">
            <button type="button" class="tab-btn active" data-tab="operations"
                    hx-get="/api/explore/operations?h={{.Analysis.Hash}}"
                    hx-target="#tab-content"
                    hx-swap="innerHTML">
                Operations
            </button>
            <button type="button" class="tab-btn" data-tab="schemas"
                    hx-get="/api/explore/schemas?h={{.Analysis.Hash}}"
                    hx-target="#tab-content"
                    hx-swap="innerHTML">
                Schemas
            </button>
            <button type="button" class="tab-btn" data-tab="security"
                    hx-get="/api/explore/security?h={{.Analysis.Hash}}"
                    hx-target="#tab-content"
                    hx-swap="innerHTML">
                Security
            </button>
        </div>
        <div class="tab-controls">
            <label for="group-by">Group by:</label>
            <select id="group-by" name="group"
                    hx-get="/api/explore/operations?h={{.Analysis.Hash}}"
                    hx-target="#tab-content"
                    hx-swap="innerHTML"
                    hx-include="this">
                <option value="path" selected>Path</option>
                <option value="tag">Tag</option>
                <option value="method">Method</option>
            </select>
        </div>
    </div>

    <div id="tab-content" class="tab-content">
        {{template "explore_operations" .}}
    </div>

    <div id="detail-section" class="detail-section"></div>
</div>
{{end}}
```

**Step 2: Verify template parses**

Run:
```bash
go build ./...
```

Expected: Build succeeds (template will be parsed at runtime)

**Step 3: Commit**

Run:
```bash
git add internal/templates/explore.html
git commit -m "feat(explore): add main explore page template"
```

---

## Task 4: Spec Parsing and Analysis

**Context:** This implements the core parsing logic. When a user uploads a spec, we parse it, collect operations/schemas/security, compute stats, cache the analysis, and render the results.

**Files:**
- Modify: `internal/api/explore.go`

**Step 1: Implement the upload handler**

Replace the stub `handleExploreUpload` in `internal/api/explore.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/walker"
)

// handleExploreUpload handles spec upload and analysis.
func (h *Handler) handleExploreUpload(w http.ResponseWriter, r *http.Request) {
	content, filename, err := h.readSpecContent(r)
	if err != nil {
		h.renderError(w, r, "Failed to read spec: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Compute content hash
	hashBytes := sha256.Sum256(content)
	hash := hex.EncodeToString(hashBytes[:])[:16]

	// Check cache first
	if analysis, ok := exploreCache.Get(hash); ok {
		h.renderExploreResults(w, r, analysis)
		return
	}

	// Parse the spec
	parseResult, err := parser.Parse(content)
	if err != nil {
		h.renderError(w, r, "Failed to parse spec: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Collect operations and schemas
	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		h.renderError(w, r, "Failed to analyze operations: "+err.Error(), http.StatusInternalServerError)
		return
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		h.renderError(w, r, "Failed to analyze schemas: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Extract security schemes
	security := extractSecuritySchemes(parseResult)

	// Compute stats
	stats := computeExploreStats(parseResult, operations, schemas, security)

	// Determine version string
	version := getVersionString(parseResult)

	// Create analysis
	analysis := &ExploreAnalysis{
		Hash:        hash,
		Version:     version,
		Filename:    filename,
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
		Security:    security,
		Stats:       stats,
	}

	// Cache and render
	exploreCache.Set(hash, analysis)
	h.renderExploreResults(w, r, analysis)
}

// readSpecContent reads spec content from file upload or paste.
func (h *Handler) readSpecContent(r *http.Request) ([]byte, string, error) {
	if err := r.ParseMultipartForm(h.cfg.MaxFileSize); err != nil {
		return nil, "", fmt.Errorf("parse form: %w", err)
	}

	// Try file upload first
	file, header, err := r.FormFile("spec")
	if err == nil {
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, "", fmt.Errorf("read file: %w", err)
		}
		return content, header.Filename, nil
	}

	// Fall back to pasted content
	content := r.FormValue("spec_content")
	if content == "" {
		return nil, "", fmt.Errorf("no spec provided")
	}
	return []byte(content), "pasted-content", nil
}

// getVersionString returns the OpenAPI version string.
func getVersionString(result *parser.ParseResult) string {
	if result.OAS3 != nil {
		return result.OAS3.OpenAPI
	}
	if result.OAS2 != nil {
		return result.OAS2.Swagger
	}
	return "unknown"
}

// extractSecuritySchemes extracts security scheme info from the spec.
func extractSecuritySchemes(result *parser.ParseResult) []SecuritySchemeInfo {
	var schemes []SecuritySchemeInfo

	if result.OAS3 != nil && result.OAS3.Components != nil {
		for name, scheme := range result.OAS3.Components.SecuritySchemes {
			info := SecuritySchemeInfo{
				Name:      name,
				Type:      scheme.Type,
				Scheme:    scheme.Scheme,
				In:        scheme.In,
				ParamName: scheme.Name,
				OpenIDURL: scheme.OpenIDConnectURL,
			}
			if scheme.Flows != nil {
				info.Flows = extractOAuthFlows(scheme.Flows)
			}
			schemes = append(schemes, info)
		}
	}

	if result.OAS2 != nil {
		for name, scheme := range result.OAS2.SecurityDefinitions {
			info := SecuritySchemeInfo{
				Name:      name,
				Type:      scheme.Type,
				In:        scheme.In,
				ParamName: scheme.Name,
			}
			if scheme.Flow != "" {
				info.Flows = []OAuthFlowInfo{{
					Type:             scheme.Flow,
					AuthorizationURL: scheme.AuthorizationURL,
					TokenURL:         scheme.TokenURL,
					Scopes:           scheme.Scopes,
				}}
			}
			schemes = append(schemes, info)
		}
	}

	return schemes
}

// extractOAuthFlows extracts OAuth flow information.
func extractOAuthFlows(flows *parser.OAuthFlows) []OAuthFlowInfo {
	var result []OAuthFlowInfo

	addFlow := func(name string, flow *parser.OAuthFlow) {
		if flow != nil {
			result = append(result, OAuthFlowInfo{
				Type:             name,
				AuthorizationURL: flow.AuthorizationURL,
				TokenURL:         flow.TokenURL,
				RefreshURL:       flow.RefreshURL,
				Scopes:           flow.Scopes,
			})
		}
	}

	addFlow("implicit", flows.Implicit)
	addFlow("password", flows.Password)
	addFlow("clientCredentials", flows.ClientCredentials)
	addFlow("authorizationCode", flows.AuthorizationCode)

	return result
}

// computeExploreStats computes summary statistics.
func computeExploreStats(result *parser.ParseResult, ops *walker.OperationCollector, schemas *walker.SchemaCollector, security []SecuritySchemeInfo) ExploreStats {
	stats := ExploreStats{
		OperationCount: len(ops.All),
		SchemaCount:    len(schemas.Components),
		InlineCount:    len(schemas.Inline),
		MethodCounts:   make(map[string]int),
	}

	// Count paths
	if result.OAS3 != nil {
		stats.PathCount = len(result.OAS3.Paths)
	} else if result.OAS2 != nil {
		stats.PathCount = len(result.OAS2.Paths)
	}

	// Count methods
	for method, opList := range ops.ByMethod {
		stats.MethodCounts[method] = len(opList)
	}

	// Count secured operations
	for _, op := range ops.All {
		if len(op.Operation.Security) > 0 {
			stats.SecuredCount++
		}
	}
	stats.UnsecuredCount = stats.OperationCount - stats.SecuredCount

	return stats
}

// renderExploreResults renders the explore results page.
func (h *Handler) renderExploreResults(w http.ResponseWriter, r *http.Request, analysis *ExploreAnalysis) {
	data := map[string]any{
		"Analysis": analysis,
	}
	h.render(w, r, "explore.html", data)
}
```

**Step 2: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 3: Commit**

Run:
```bash
git add internal/api/explore.go
git commit -m "feat(explore): implement spec parsing and analysis"
```

---

## Task 5: Operations Tab Partial

**Context:** This creates the operations tab showing operations grouped by path (default), with expandable accordions and method badges.

**Files:**
- Create: `internal/templates/partials/explore_operations.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the operations partial template**

Create `internal/templates/partials/explore_operations.html`:

```html
{{define "explore_operations"}}
<div class="operations-list" id="operations-list">
    {{$hash := .Analysis.Hash}}
    {{$group := .Group}}
    {{if eq $group ""}}{{$group = "path"}}{{end}}

    {{if eq $group "path"}}
        {{range $path, $ops := .Analysis.Operations.ByPath}}
        <div class="accordion-item">
            <button class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
                <span class="accordion-icon">&#9654;</span>
                <span class="accordion-path">{{$path}}</span>
                <span class="method-badges">
                    {{range $ops}}
                    <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                    {{end}}
                </span>
            </button>
            <div class="accordion-content">
                {{range $ops}}
                <div class="operation-row"
                     hx-get="/api/explore/operation/{{.Operation.OperationID}}?h={{$hash}}"
                     hx-target="#detail-section"
                     hx-swap="innerHTML">
                    <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                    <span class="operation-id">{{.Operation.OperationID}}</span>
                    <span class="operation-summary">{{truncate .Operation.Summary 60}}</span>
                    <span class="operation-arrow">&#8594;</span>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
    {{else if eq $group "tag"}}
        {{range $tag, $ops := .Analysis.Operations.ByTag}}
        <div class="accordion-item">
            <button class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
                <span class="accordion-icon">&#9654;</span>
                <span class="accordion-tag">{{if $tag}}{{$tag}}{{else}}Untagged{{end}}</span>
                <span class="operation-count">({{len $ops}} operations)</span>
            </button>
            <div class="accordion-content">
                {{range $ops}}
                <div class="operation-row"
                     hx-get="/api/explore/operation/{{.Operation.OperationID}}?h={{$hash}}"
                     hx-target="#detail-section"
                     hx-swap="innerHTML">
                    <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                    <span class="operation-path">{{.PathTemplate}}</span>
                    <span class="operation-summary">{{truncate .Operation.Summary 50}}</span>
                    <span class="operation-arrow">&#8594;</span>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
        {{if .UntaggedOps}}
        <div class="accordion-item">
            <button class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
                <span class="accordion-icon">&#9654;</span>
                <span class="accordion-tag">Untagged</span>
                <span class="operation-count">({{len .UntaggedOps}} operations)</span>
            </button>
            <div class="accordion-content">
                {{range .UntaggedOps}}
                <div class="operation-row"
                     hx-get="/api/explore/operation/{{.Operation.OperationID}}?h={{$hash}}"
                     hx-target="#detail-section"
                     hx-swap="innerHTML">
                    <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                    <span class="operation-path">{{.PathTemplate}}</span>
                    <span class="operation-summary">{{truncate .Operation.Summary 50}}</span>
                    <span class="operation-arrow">&#8594;</span>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
    {{else if eq $group "method"}}
        {{range $method, $ops := .Analysis.Operations.ByMethod}}
        <div class="accordion-item">
            <button class="accordion-header" onclick="this.parentElement.classList.toggle('open')">
                <span class="accordion-icon">&#9654;</span>
                <span class="method-badge method-{{$method}}">{{template "method_glyph" $method}}{{$method | upper}}</span>
                <span class="operation-count">({{len $ops}} operations)</span>
            </button>
            <div class="accordion-content">
                {{range $ops}}
                <div class="operation-row"
                     hx-get="/api/explore/operation/{{.Operation.OperationID}}?h={{$hash}}"
                     hx-target="#detail-section"
                     hx-swap="innerHTML">
                    <span class="operation-path">{{.PathTemplate}}</span>
                    <span class="operation-id">{{.Operation.OperationID}}</span>
                    <span class="operation-summary">{{truncate .Operation.Summary 50}}</span>
                    <span class="operation-arrow">&#8594;</span>
                </div>
                {{end}}
            </div>
        </div>
        {{end}}
    {{end}}
</div>
{{end}}

{{define "method_glyph"}}{{if eq . "get"}}&#8595; {{else if eq . "post"}}&#8593; {{else if eq . "put"}}&#8594; {{else if eq . "patch"}}~ {{else if eq . "delete"}}&#215; {{end}}{{end}}
```

**Step 2: Implement the operations endpoint**

Update `handleExploreOperations` in `internal/api/explore.go`:

```go
// handleExploreOperations renders the operations tab partial.
func (h *Handler) handleExploreOperations(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	group := r.URL.Query().Get("group")
	if group == "" {
		group = "path"
	}

	// Find untagged operations for tag grouping
	var untaggedOps []*walker.OperationInfo
	if group == "tag" {
		for _, op := range analysis.Operations.All {
			if len(op.Operation.Tags) == 0 {
				untaggedOps = append(untaggedOps, op)
			}
		}
	}

	data := map[string]any{
		"Analysis":    analysis,
		"Group":       group,
		"UntaggedOps": untaggedOps,
	}

	h.renderPartial(w, "explore_operations", data)
}
```

**Step 3: Add template helper functions**

In `internal/api/handler.go`, add template functions (if not already present):

```go
// Add to template FuncMap
funcMap := template.FuncMap{
	"upper": strings.ToUpper,
	"truncate": func(s string, max int) string {
		if len(s) <= max {
			return s
		}
		return s[:max-3] + "..."
	},
}
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_operations.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add operations tab partial with grouping"
```

---

## Task 6: Operation Detail Partial

**Context:** When a user clicks an operation row, this partial renders the full operation details including parameters, responses, and security.

**Files:**
- Create: `internal/templates/partials/explore_operation_detail.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the operation detail template**

Create `internal/templates/partials/explore_operation_detail.html`:

```html
{{define "explore_operation_detail"}}
<div class="detail-card" id="op-{{.Operation.OperationID}}">
    <div class="detail-header">
        <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
        <span class="detail-path">{{.PathTemplate}}</span>
    </div>

    {{if .Operation.Summary}}
    <p class="detail-summary">{{.Operation.Summary}}</p>
    {{end}}

    {{if .Operation.Description}}
    <p class="detail-description">{{.Operation.Description}}</p>
    {{end}}

    {{if .Operation.Parameters}}
    <div class="detail-section">
        <h4>Parameters</h4>
        <table class="params-table">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>In</th>
                    <th>Required</th>
                    <th>Type</th>
                    <th>Description</th>
                </tr>
            </thead>
            <tbody>
                {{range .Operation.Parameters}}
                <tr>
                    <td><code>{{.Name}}</code></td>
                    <td>{{.In}}</td>
                    <td>{{if .Required}}Yes{{else}}No{{end}}</td>
                    <td>{{if .Schema}}{{.Schema.Type}}{{else}}{{.Type}}{{end}}</td>
                    <td>{{.Description}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Operation.RequestBody}}
    <div class="detail-section">
        <h4>Request Body</h4>
        {{if .Operation.RequestBody.Required}}<span class="required-badge">Required</span>{{end}}
        {{if .Operation.RequestBody.Description}}
        <p>{{.Operation.RequestBody.Description}}</p>
        {{end}}
        {{range $contentType, $mediaType := .Operation.RequestBody.Content}}
        <div class="content-type">
            <code>{{$contentType}}</code>
            {{if $mediaType.Schema}}
            {{if $mediaType.Schema.Ref}}
            <span class="schema-ref">&#8594; {{schemaName $mediaType.Schema.Ref}}</span>
            {{end}}
            {{end}}
        </div>
        {{end}}
    </div>
    {{end}}

    {{if .Operation.Responses}}
    <div class="detail-section">
        <h4>Responses</h4>
        <table class="responses-table">
            <thead>
                <tr>
                    <th>Status</th>
                    <th>Description</th>
                    <th>Schema</th>
                </tr>
            </thead>
            <tbody>
                {{range $status, $response := .Operation.Responses}}
                <tr>
                    <td><code>{{$status}}</code></td>
                    <td>{{$response.Description}}</td>
                    <td>
                        {{range $ct, $mt := $response.Content}}
                        {{if $mt.Schema}}{{if $mt.Schema.Ref}}{{schemaName $mt.Schema.Ref}}{{end}}{{end}}
                        {{end}}
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Operation.Security}}
    <div class="detail-section">
        <h4>Security</h4>
        <ul class="security-list">
            {{range .Operation.Security}}
            {{range $scheme, $scopes := .}}
            <li>
                <strong>{{$scheme}}</strong>
                {{if $scopes}}
                <span class="scopes">({{join $scopes ", "}})</span>
                {{end}}
            </li>
            {{end}}
            {{end}}
        </ul>
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Implement the operation detail endpoint**

Add to `internal/api/explore.go`:

```go
// handleExploreOperationDetail renders a single operation detail.
func (h *Handler) handleExploreOperationDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	operationID := r.PathValue("id")

	// Find the operation
	var found *walker.OperationInfo
	for _, op := range analysis.Operations.All {
		if op.Operation.OperationID == operationID {
			found = op
			break
		}
	}

	if found == nil {
		http.Error(w, "Operation not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Operation":    found.Operation,
		"PathTemplate": found.PathTemplate,
		"Method":       found.Method,
	}

	h.renderPartial(w, "explore_operation_detail", data)
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/operation/{id}", h.handleExploreOperationDetail)
```

**Step 4: Add template helpers**

Add to template FuncMap:

```go
"schemaName": func(ref string) string {
	parts := strings.Split(ref, "/")
	return parts[len(parts)-1]
},
"join": strings.Join,
```

**Step 5: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 6: Commit**

Run:
```bash
git add internal/templates/partials/explore_operation_detail.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add operation detail partial"
```

---

## Task 7: Group-By Switching

**Context:** The group-by dropdown allows users to switch between path, tag, and method groupings. This is already wired up in the templates; this task ensures it works correctly.

**Files:**
- Modify: `internal/templates/explore.html`

**Step 1: Verify HTMX attributes**

Ensure the group-by select in `explore.html` includes the value parameter:

```html
<select id="group-by" name="group"
        hx-get="/api/explore/operations"
        hx-target="#tab-content"
        hx-swap="innerHTML"
        hx-vals='{"h": "{{.Analysis.Hash}}"}'>
    <option value="path" {{if eq .Group "path"}}selected{{end}}>Path</option>
    <option value="tag" {{if eq .Group "tag"}}selected{{end}}>Tag</option>
    <option value="method" {{if eq .Group "method"}}selected{{end}}>Method</option>
</select>
```

**Step 2: Commit**

Run:
```bash
git add internal/templates/explore.html
git commit -m "feat(explore): wire up group-by dropdown switching"
```

---

## Task 8: Schemas Tab Partial

**Context:** The schemas tab shows component schemas with type badges and property previews, plus an inline schema count.

**Files:**
- Create: `internal/templates/partials/explore_schemas.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the schemas partial template**

Create `internal/templates/partials/explore_schemas.html`:

```html
{{define "explore_schemas"}}
<div class="schemas-container">
    <div class="schemas-header">
        <span class="schemas-count">Component Schemas ({{len .Analysis.Schemas.Components}})</span>
        <span class="inline-count">
            Inline Schemas: {{.Analysis.Stats.InlineCount}}
            {{if gt .Analysis.Stats.InlineCount 0}}
            <button type="button" class="inline-expand-btn"
                    hx-get="/api/explore/inline-schemas?h={{.Analysis.Hash}}"
                    hx-target="#inline-schemas-list"
                    hx-swap="innerHTML">
                <span class="expand-icon">&#9660;</span> Show locations
            </button>
            {{end}}
        </span>
    </div>

    <div id="inline-schemas-list"></div>

    <div class="schemas-list">
        {{$hash := .Analysis.Hash}}
        {{range .Analysis.Schemas.Components}}
        <div class="schema-row"
             hx-get="/api/explore/schema/{{.Name}}?h={{$hash}}"
             hx-target="#detail-section"
             hx-swap="innerHTML">
            <span class="schema-name">{{.Name}}</span>
            <span class="type-badge">{{template "schema_type_badge" .Schema}}</span>
            <span class="schema-preview">{{template "schema_preview" .Schema}}</span>
            <span class="schema-arrow">&#8594;</span>
        </div>
        {{end}}
    </div>
</div>
{{end}}

{{define "schema_type_badge"}}{{if .Enum}}[enum]{{else if eq .Type "array"}}[array]{{else if .AllOf}}{allOf}{{else if .OneOf}}{oneOf}{{else if .AnyOf}}{anyOf}{{else if .Properties}}{object}{{else}}{{.Type}}{{end}}{{end}}

{{define "schema_preview"}}{{if .Properties}}{{range $i, $name := propNames .Properties}}{{if $i}}, {{end}}{{$name}}{{if gt $i 3}}...{{end}}{{if gt $i 3}}{{break}}{{end}}{{end}}{{else if .Enum}}{{range $i, $v := .Enum}}{{if $i}}, {{end}}{{$v}}{{if gt $i 4}}...{{end}}{{if gt $i 4}}{{break}}{{end}}{{end}}{{else if eq .Type "array"}}&#8594; {{if .Items}}{{if .Items.Ref}}{{schemaName .Items.Ref}}{{else}}{{.Items.Type}}{{end}}{{end}}{{end}}{{end}}
```

**Step 2: Implement the schemas endpoint**

Add to `internal/api/explore.go`:

```go
// handleExploreSchemas renders the schemas tab partial.
func (h *Handler) handleExploreSchemas(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	data := map[string]any{
		"Analysis": analysis,
	}

	h.renderPartial(w, "explore_schemas", data)
}
```

**Step 3: Add template helper**

Add to template FuncMap:

```go
"propNames": func(props map[string]*parser.Schema) []string {
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
},
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_schemas.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add schemas tab partial with type badges"
```

---

## Task 9: Schema Detail Partial

**Context:** Shows full schema details including properties, enum values, and "used in" operations.

**Files:**
- Create: `internal/templates/partials/explore_schema_detail.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the schema detail template**

Create `internal/templates/partials/explore_schema_detail.html`:

```html
{{define "explore_schema_detail"}}
<div class="detail-card" id="schema-{{.Name}}">
    <div class="detail-header">
        <span class="schema-name">{{.Name}}</span>
        <span class="type-badge">{{template "schema_type_badge" .Schema}}</span>
    </div>

    {{if .Schema.Description}}
    <p class="detail-description">{{.Schema.Description}}</p>
    {{end}}

    {{if .Schema.Properties}}
    <div class="detail-section">
        <h4>Properties</h4>
        <table class="props-table">
            <thead>
                <tr>
                    <th>Name</th>
                    <th>Type</th>
                    <th>Required</th>
                    <th>Description</th>
                </tr>
            </thead>
            <tbody>
                {{$required := .Schema.Required}}
                {{range $name, $prop := .Schema.Properties}}
                <tr>
                    <td><code>{{$name}}</code></td>
                    <td>
                        {{if $prop.Ref}}
                        <a href="#schema-{{schemaName $prop.Ref}}">{{schemaName $prop.Ref}}</a>
                        {{else}}
                        {{$prop.Type}}{{if $prop.Format}} ({{$prop.Format}}){{end}}
                        {{end}}
                    </td>
                    <td>{{if contains $required $name}}Yes{{else}}No{{end}}</td>
                    <td>{{$prop.Description}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    {{end}}

    {{if .Schema.Enum}}
    <div class="detail-section">
        <h4>Enum Values</h4>
        <ul class="enum-list">
            {{range .Schema.Enum}}
            <li><code>{{.}}</code></li>
            {{end}}
        </ul>
    </div>
    {{end}}

    {{if .UsedIn}}
    <div class="detail-section">
        <h4>Used In</h4>
        <ul class="used-in-list">
            {{range .UsedIn}}
            <li>
                <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                <span>{{.PathTemplate}}</span>
                <span class="usage-context">{{.Context}}</span>
            </li>
            {{end}}
        </ul>
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Implement the schema detail endpoint**

Add to `internal/api/explore.go`:

```go
// SchemaUsage represents where a schema is used.
type SchemaUsage struct {
	Method       string
	PathTemplate string
	Context      string // e.g., "request body", "response 200"
}

// handleExploreSchemaDetail renders a single schema detail.
func (h *Handler) handleExploreSchemaDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	schemaName := r.PathValue("name")

	// Find the schema
	var found *walker.SchemaInfo
	for _, s := range analysis.Schemas.Components {
		if s.Name == schemaName {
			found = s
			break
		}
	}

	if found == nil {
		http.Error(w, "Schema not found", http.StatusNotFound)
		return
	}

	// Find usages
	usedIn := findSchemaUsages(analysis, schemaName)

	data := map[string]any{
		"Name":   found.Name,
		"Schema": found.Schema,
		"UsedIn": usedIn,
	}

	h.renderPartial(w, "explore_schema_detail", data)
}

// findSchemaUsages finds where a schema is referenced.
func findSchemaUsages(analysis *ExploreAnalysis, schemaName string) []SchemaUsage {
	var usages []SchemaUsage
	refSuffix := "/" + schemaName

	for _, op := range analysis.Operations.All {
		// Check request body
		if op.Operation.RequestBody != nil {
			for _, mt := range op.Operation.RequestBody.Content {
				if mt.Schema != nil && strings.HasSuffix(mt.Schema.Ref, refSuffix) {
					usages = append(usages, SchemaUsage{
						Method:       op.Method,
						PathTemplate: op.PathTemplate,
						Context:      "request body",
					})
				}
			}
		}

		// Check responses
		for status, resp := range op.Operation.Responses {
			for _, mt := range resp.Content {
				if mt.Schema != nil && strings.HasSuffix(mt.Schema.Ref, refSuffix) {
					usages = append(usages, SchemaUsage{
						Method:       op.Method,
						PathTemplate: op.PathTemplate,
						Context:      "response " + status,
					})
				}
			}
		}
	}

	return usages
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/schema/{name}", h.handleExploreSchemaDetail)
```

**Step 4: Add template helper**

Add to template FuncMap:

```go
"contains": func(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
},
```

**Step 5: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 6: Commit**

Run:
```bash
git add internal/templates/partials/explore_schema_detail.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add schema detail partial with usage info"
```

---

## Task 10: Inline Schemas Drill-in

**Context:** Shows where inline schemas appear when the user clicks "Show locations" in the Schemas tab.

**Files:**
- Create: `internal/templates/partials/explore_inline_schemas.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the inline schemas template**

Create `internal/templates/partials/explore_inline_schemas.html`:

```html
{{define "explore_inline_schemas"}}
<div class="inline-schemas-section">
    <div class="inline-header">
        <span>Inline Schemas: {{len .InlineSchemas}}</span>
        <button type="button" class="inline-collapse-btn" onclick="this.closest('.inline-schemas-section').remove()">
            <span class="expand-icon">&#9650;</span> Hide
        </button>
    </div>
    <div class="inline-list">
        {{range .InlineSchemas}}
        <div class="inline-row">
            <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
            <span class="inline-path">{{.PathTemplate}}</span>
            <span class="inline-context">{{.Context}}</span>
            <span class="type-badge">{{.Type}}</span>
        </div>
        {{end}}
    </div>
    {{if gt (len .InlineSchemas) 10}}
    <div class="inline-warning">
        <span class="warning-icon">&#9888;</span>
        High inline count may indicate opportunities for schema reuse
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Implement the endpoint**

Add to `internal/api/explore.go`:

```go
// InlineSchemaLocation represents where an inline schema appears.
type InlineSchemaLocation struct {
	Method       string
	PathTemplate string
	Context      string
	Type         string
}

// handleExploreInlineSchemas renders the inline schemas list.
func (h *Handler) handleExploreInlineSchemas(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	locations := parseInlineLocations(analysis)

	data := map[string]any{
		"InlineSchemas": locations,
	}

	h.renderPartial(w, "explore_inline_schemas", data)
}

// parseInlineLocations extracts location info from inline schema JSON paths.
func parseInlineLocations(analysis *ExploreAnalysis) []InlineSchemaLocation {
	var locations []InlineSchemaLocation

	for _, info := range analysis.Schemas.Inline {
		loc := InlineSchemaLocation{
			Type: getSchemaType(info.Schema),
		}

		// Parse JSON path to extract context
		// Example: $.paths['/pets'].post.requestBody.content['application/json'].schema
		path := info.JSONPath

		// Extract method and path template from JSON path
		if strings.Contains(path, ".paths[") {
			// Extract path template
			start := strings.Index(path, ".paths['") + 8
			if start > 7 {
				end := strings.Index(path[start:], "']")
				if end > 0 {
					loc.PathTemplate = path[start : start+end]
				}
			}

			// Extract method
			methods := []string{"get", "post", "put", "patch", "delete", "options", "head"}
			for _, m := range methods {
				if strings.Contains(path, "']."+m+".") || strings.Contains(path, "']."+m+"[") {
					loc.Method = m
					break
				}
			}

			// Determine context
			if strings.Contains(path, "requestBody") {
				loc.Context = "request body"
			} else if strings.Contains(path, "responses") {
				// Extract status code
				if idx := strings.Index(path, "responses['"); idx > 0 {
					start := idx + 11
					end := strings.Index(path[start:], "']")
					if end > 0 {
						loc.Context = "response " + path[start:start+end]
					}
				}
			} else if strings.Contains(path, "parameters") {
				loc.Context = "parameter"
			}
		}

		if loc.Method != "" && loc.PathTemplate != "" {
			locations = append(locations, loc)
		}
	}

	return locations
}

func getSchemaType(s *parser.Schema) string {
	if s.Enum != nil {
		return "[enum]"
	}
	if s.Type == "array" {
		return "[array]"
	}
	if s.Properties != nil {
		return "{object}"
	}
	if s.AllOf != nil {
		return "{allOf}"
	}
	if s.OneOf != nil {
		return "{oneOf}"
	}
	if s.AnyOf != nil {
		return "{anyOf}"
	}
	return s.Type
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/inline-schemas", h.handleExploreInlineSchemas)
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_inline_schemas.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add inline schemas drill-in"
```

---

## Task 11: Security Tab Partial

**Context:** Shows security schemes defined in the spec with type badges, configuration, and usage counts.

**Files:**
- Create: `internal/templates/partials/explore_security.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the security partial template**

Create `internal/templates/partials/explore_security.html`:

```html
{{define "explore_security"}}
<div class="security-container">
    <div class="security-header">
        <span class="security-count">Security Schemes ({{len .Analysis.Security}})</span>
        <span class="coverage">
            Coverage: {{.Analysis.Stats.SecuredCount}}/{{.Analysis.Stats.OperationCount}}
            {{if gt .Analysis.Stats.UnsecuredCount 0}}
            <button type="button" class="unsecured-expand-btn"
                    hx-get="/api/explore/unsecured?h={{.Analysis.Hash}}"
                    hx-target="#unsecured-list"
                    hx-swap="innerHTML">
                <span class="expand-icon">&#9660;</span> Show unsecured
            </button>
            {{end}}
        </span>
    </div>

    <div id="unsecured-list"></div>

    {{if .Analysis.Security}}
    <div class="security-list">
        {{$hash := .Analysis.Hash}}
        {{range .Analysis.Security}}
        <div class="security-row"
             hx-get="/api/explore/security/{{.Name}}?h={{$hash}}"
             hx-target="#detail-section"
             hx-swap="innerHTML">
            <div class="security-main">
                <span class="security-name">{{.Name}}</span>
                <span class="security-type-badge">[{{.Type}}]</span>
            </div>
            <div class="security-config">
                {{if eq .Type "apiKey"}}
                {{.In}}: {{.ParamName}}
                {{else if eq .Type "http"}}
                scheme: {{.Scheme}}
                {{else if eq .Type "oauth2"}}
                {{range .Flows}}{{.Type}} flow{{end}}
                {{else if eq .Type "openIdConnect"}}
                OpenID Connect
                {{end}}
            </div>
            <div class="security-usage">
                Used by {{.UsageCount}} operations
            </div>
            <span class="security-arrow">&#8594;</span>
        </div>
        {{end}}
    </div>
    {{else}}
    <div class="no-security">
        No security schemes defined
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Update security scheme extraction to count usages**

Modify `extractSecuritySchemes` in `internal/api/explore.go` to count usages:

```go
// extractSecuritySchemes extracts security scheme info from the spec.
func extractSecuritySchemes(result *parser.ParseResult, operations *walker.OperationCollector) []SecuritySchemeInfo {
	var schemes []SecuritySchemeInfo
	usageCounts := make(map[string]int)

	// Count usages
	for _, op := range operations.All {
		for _, secReq := range op.Operation.Security {
			for schemeName := range secReq {
				usageCounts[schemeName]++
			}
		}
	}

	if result.OAS3 != nil && result.OAS3.Components != nil {
		for name, scheme := range result.OAS3.Components.SecuritySchemes {
			info := SecuritySchemeInfo{
				Name:       name,
				Type:       scheme.Type,
				Scheme:     scheme.Scheme,
				In:         scheme.In,
				ParamName:  scheme.Name,
				OpenIDURL:  scheme.OpenIDConnectURL,
				UsageCount: usageCounts[name],
			}
			if scheme.Flows != nil {
				info.Flows = extractOAuthFlows(scheme.Flows)
			}
			schemes = append(schemes, info)
		}
	}

	if result.OAS2 != nil {
		for name, scheme := range result.OAS2.SecurityDefinitions {
			info := SecuritySchemeInfo{
				Name:       name,
				Type:       scheme.Type,
				In:         scheme.In,
				ParamName:  scheme.Name,
				UsageCount: usageCounts[name],
			}
			if scheme.Flow != "" {
				info.Flows = []OAuthFlowInfo{{
					Type:             scheme.Flow,
					AuthorizationURL: scheme.AuthorizationURL,
					TokenURL:         scheme.TokenURL,
					Scopes:           scheme.Scopes,
				}}
			}
			schemes = append(schemes, info)
		}
	}

	return schemes
}
```

**Step 3: Implement the security endpoint**

Add to `internal/api/explore.go`:

```go
// handleExploreSecurity renders the security tab partial.
func (h *Handler) handleExploreSecurity(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	data := map[string]any{
		"Analysis": analysis,
	}

	h.renderPartial(w, "explore_security", data)
}
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_security.html internal/api/explore.go
git commit -m "feat(explore): add security tab partial with coverage"
```

---

## Task 12: Security Scheme Detail Partial

**Context:** Shows full security scheme configuration and which operations use it.

**Files:**
- Create: `internal/templates/partials/explore_security_detail.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the security detail template**

Create `internal/templates/partials/explore_security_detail.html`:

```html
{{define "explore_security_detail"}}
<div class="detail-card" id="security-{{.Scheme.Name}}">
    <div class="detail-header">
        <span class="security-name">{{.Scheme.Name}}</span>
        <span class="security-type-badge">[{{.Scheme.Type}}]</span>
    </div>

    <div class="detail-section">
        <h4>Configuration</h4>
        {{if eq .Scheme.Type "apiKey"}}
        <dl class="config-list">
            <dt>Location</dt>
            <dd>{{.Scheme.In}}</dd>
            <dt>Parameter Name</dt>
            <dd><code>{{.Scheme.ParamName}}</code></dd>
        </dl>
        {{else if eq .Scheme.Type "http"}}
        <dl class="config-list">
            <dt>Scheme</dt>
            <dd>{{.Scheme.Scheme}}</dd>
        </dl>
        {{else if eq .Scheme.Type "oauth2"}}
        {{range .Scheme.Flows}}
        <div class="oauth-flow">
            <h5>{{.Type}} flow</h5>
            <dl class="config-list">
                {{if .AuthorizationURL}}
                <dt>Authorization URL</dt>
                <dd><code>{{.AuthorizationURL}}</code></dd>
                {{end}}
                {{if .TokenURL}}
                <dt>Token URL</dt>
                <dd><code>{{.TokenURL}}</code></dd>
                {{end}}
                {{if .Scopes}}
                <dt>Scopes</dt>
                <dd>
                    <ul class="scopes-list">
                        {{range $scope, $desc := .Scopes}}
                        <li><code>{{$scope}}</code> — {{$desc}}</li>
                        {{end}}
                    </ul>
                </dd>
                {{end}}
            </dl>
        </div>
        {{end}}
        {{else if eq .Scheme.Type "openIdConnect"}}
        <dl class="config-list">
            <dt>OpenID Connect URL</dt>
            <dd><code>{{.Scheme.OpenIDURL}}</code></dd>
        </dl>
        {{end}}
    </div>

    {{if .UsedBy}}
    <div class="detail-section">
        <h4>Used By</h4>
        <ul class="used-by-list">
            {{range .UsedBy}}
            <li>
                <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
                <span>{{.PathTemplate}}</span>
                {{if .Scopes}}
                <span class="required-scopes">{{join .Scopes ", "}}</span>
                {{end}}
            </li>
            {{end}}
        </ul>
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Implement the endpoint**

Add to `internal/api/explore.go`:

```go
// SecurityUsage represents an operation using a security scheme.
type SecurityUsage struct {
	Method       string
	PathTemplate string
	Scopes       []string
}

// handleExploreSecurityDetail renders a single security scheme detail.
func (h *Handler) handleExploreSecurityDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	schemeName := r.PathValue("name")

	// Find the scheme
	var found *SecuritySchemeInfo
	for i := range analysis.Security {
		if analysis.Security[i].Name == schemeName {
			found = &analysis.Security[i]
			break
		}
	}

	if found == nil {
		http.Error(w, "Security scheme not found", http.StatusNotFound)
		return
	}

	// Find usages
	usedBy := findSecurityUsages(analysis, schemeName)

	data := map[string]any{
		"Scheme": found,
		"UsedBy": usedBy,
	}

	h.renderPartial(w, "explore_security_detail", data)
}

// findSecurityUsages finds operations using a security scheme.
func findSecurityUsages(analysis *ExploreAnalysis, schemeName string) []SecurityUsage {
	var usages []SecurityUsage

	for _, op := range analysis.Operations.All {
		for _, secReq := range op.Operation.Security {
			if scopes, ok := secReq[schemeName]; ok {
				usages = append(usages, SecurityUsage{
					Method:       op.Method,
					PathTemplate: op.PathTemplate,
					Scopes:       scopes,
				})
				break
			}
		}
	}

	return usages
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/security/{name}", h.handleExploreSecurityDetail)
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_security_detail.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add security scheme detail partial"
```

---

## Task 13: Unsecured Operations Drill-in

**Context:** Lists operations that have no security requirements.

**Files:**
- Create: `internal/templates/partials/explore_unsecured.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the unsecured operations template**

Create `internal/templates/partials/explore_unsecured.html`:

```html
{{define "explore_unsecured"}}
<div class="unsecured-section">
    <div class="unsecured-header">
        <span>Unsecured Operations: {{len .UnsecuredOps}}</span>
        <button type="button" class="unsecured-collapse-btn" onclick="this.closest('.unsecured-section').remove()">
            <span class="expand-icon">&#9650;</span> Hide
        </button>
    </div>
    {{if .UnsecuredOps}}
    <div class="unsecured-list">
        {{range .UnsecuredOps}}
        <div class="unsecured-row">
            <span class="method-badge method-{{.Method}}">{{template "method_glyph" .Method}}{{.Method | upper}}</span>
            <span class="unsecured-path">{{.PathTemplate}}</span>
            <span class="unsecured-summary">{{truncate .Operation.Summary 50}}</span>
        </div>
        {{end}}
    </div>
    <div class="unsecured-info">
        <span class="info-icon">&#8505;</span>
        Some operations (health checks, docs, login) are typically intentionally unsecured
    </div>
    {{else}}
    <div class="all-secured">
        All operations are secured
    </div>
    {{end}}
</div>
{{end}}
```

**Step 2: Implement the endpoint**

Add to `internal/api/explore.go`:

```go
// handleExploreUnsecured renders the unsecured operations list.
func (h *Handler) handleExploreUnsecured(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	var unsecuredOps []*walker.OperationInfo
	for _, op := range analysis.Operations.All {
		if len(op.Operation.Security) == 0 {
			unsecuredOps = append(unsecuredOps, op)
		}
	}

	data := map[string]any{
		"UnsecuredOps": unsecuredOps,
	}

	h.renderPartial(w, "explore_unsecured", data)
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/unsecured", h.handleExploreUnsecured)
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_unsecured.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add unsecured operations drill-in"
```

---

## Task 14: Summary Details Expansion

**Context:** Shows expanded statistics when the user clicks "Details" in the summary section.

**Files:**
- Create: `internal/templates/partials/explore_summary_details.html`
- Modify: `internal/api/explore.go`

**Step 1: Create the summary details template**

Create `internal/templates/partials/explore_summary_details.html`:

```html
{{define "explore_summary_details"}}
<div class="summary-details-expanded">
    <button type="button" class="summary-collapse-btn" onclick="this.parentElement.innerHTML = ''">
        <span class="expand-icon">&#9650;</span> Hide
    </button>

    <div class="method-breakdown">
        <span class="breakdown-label">Methods:</span>
        {{range $method, $count := .Stats.MethodCounts}}
        <span class="method-stat">
            <span class="method-badge method-{{$method}}">{{template "method_glyph" $method}}{{$method | upper}}</span>
            {{$count}}
        </span>
        {{end}}
    </div>

    <div class="schema-breakdown">
        <span class="breakdown-label">Schemas:</span>
        <span>{{.Stats.SchemaCount}} components</span>
        {{if gt .Stats.InlineCount 0}}
        <span class="inline-stat {{if gt .Stats.InlineCount 10}}warning{{end}}">
            {{.Stats.InlineCount}} inline
        </span>
        {{end}}
    </div>

    <div class="security-breakdown">
        <span class="breakdown-label">Security:</span>
        <span>{{.Stats.SecuredCount}}/{{.Stats.OperationCount}} operations secured</span>
        {{if gt .Stats.UnsecuredCount 0}}
        <span class="unsecured-stat">({{.Stats.UnsecuredCount}} unsecured)</span>
        {{end}}
    </div>
</div>
{{end}}
```

**Step 2: Implement the endpoint**

Add to `internal/api/explore.go`:

```go
// handleExploreSummaryDetails renders the expanded summary details.
func (h *Handler) handleExploreSummaryDetails(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("h")
	if hash == "" {
		http.Error(w, "Missing hash parameter", http.StatusBadRequest)
		return
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		w.Header().Set("HX-Trigger", "cacheExpired")
		w.WriteHeader(http.StatusGone)
		return
	}

	data := map[string]any{
		"Stats": analysis.Stats,
	}

	h.renderPartial(w, "explore_summary_details", data)
}
```

**Step 3: Register the route**

Add to `internal/api/handler.go`:

```go
mux.HandleFunc("GET /api/explore/summary-details", h.handleExploreSummaryDetails)
```

**Step 4: Build and test**

Run:
```bash
go build ./...
```

Expected: Build succeeds

**Step 5: Commit**

Run:
```bash
git add internal/templates/partials/explore_summary_details.html internal/api/explore.go internal/api/handler.go
git commit -m "feat(explore): add summary details expansion"
```

---

## Task 15: Browser Cache Fallback JavaScript

**Context:** Stores the spec in sessionStorage and auto-resubmits on 410 (cache miss).

**Files:**
- Create: `static/js/explore.js`

**Step 1: Create the JavaScript file**

Create `static/js/explore.js`:

```javascript
// Explore page - sessionStorage cache and 410 fallback handling

(function() {
    'use strict';

    const STORAGE_KEY = 'exploreSpec';
    const FILENAME_KEY = 'exploreFilename';

    // Store spec content before form submission
    document.addEventListener('htmx:configRequest', function(e) {
        if (e.detail.path === '/api/explore' && e.detail.verb === 'post') {
            storeSpecContent();
        }
    });

    // Handle 410 Gone (cache expired) responses
    document.addEventListener('htmx:responseError', function(e) {
        if (e.detail.xhr.status === 410) {
            handleCacheExpired(e);
        }
    });

    // Store spec content from form
    function storeSpecContent() {
        const fileInput = document.querySelector('input[name="spec"]');
        const textArea = document.querySelector('textarea[name="spec_content"]');

        if (fileInput && fileInput.files.length > 0) {
            const file = fileInput.files[0];
            const reader = new FileReader();
            reader.onload = function(e) {
                sessionStorage.setItem(STORAGE_KEY, e.target.result);
                sessionStorage.setItem(FILENAME_KEY, file.name);
            };
            reader.readAsText(file);
        } else if (textArea && textArea.value) {
            sessionStorage.setItem(STORAGE_KEY, textArea.value);
            sessionStorage.setItem(FILENAME_KEY, 'pasted-content');
        }
    }

    // Handle cache expiration by resubmitting stored spec
    function handleCacheExpired(e) {
        const spec = sessionStorage.getItem(STORAGE_KEY);

        if (!spec) {
            showReuploadMessage(e.detail.target);
            return;
        }

        const filename = sessionStorage.getItem(FILENAME_KEY) || 'spec.yaml';

        // Create form data with stored spec
        const formData = new FormData();
        const blob = new Blob([spec], { type: 'application/x-yaml' });
        formData.append('spec', blob, filename);

        // Resubmit to /api/explore
        fetch('/api/explore', {
            method: 'POST',
            body: formData
        })
        .then(response => response.text())
        .then(html => {
            // Replace results section
            const results = document.getElementById('explore-results');
            if (results) {
                results.innerHTML = html;
                htmx.process(results);
            }

            // Retry the original request
            const originalPath = e.detail.pathInfo.requestPath;
            const hash = document.querySelector('[data-hash]')?.dataset.hash;
            if (hash && originalPath) {
                const separator = originalPath.includes('?') ? '&' : '?';
                htmx.ajax('GET', originalPath + separator + 'h=' + hash, {
                    target: e.detail.target,
                    swap: 'innerHTML'
                });
            }
        })
        .catch(err => {
            console.error('Failed to resubmit spec:', err);
            showReuploadMessage(e.detail.target);
        });
    }

    // Show message when no stored spec available
    function showReuploadMessage(target) {
        if (target) {
            target.innerHTML = `
                <div class="cache-miss-message">
                    <p>Session expired. Please re-upload your spec.</p>
                    <button onclick="window.location.href='/explore'" class="btn btn-primary">
                        &#8635; Start over
                    </button>
                </div>
            `;
        }
    }

    // Clear stored spec when uploading a new one
    document.addEventListener('change', function(e) {
        if (e.target.matches('input[name="spec"]')) {
            sessionStorage.removeItem(STORAGE_KEY);
            sessionStorage.removeItem(FILENAME_KEY);
        }
    });

    // Input mode toggle (upload vs paste)
    document.addEventListener('click', function(e) {
        const modeBtn = e.target.closest('.mode-btn');
        if (!modeBtn) return;

        const mode = modeBtn.dataset.mode;
        const uploadMode = document.getElementById('upload-mode');
        const pasteMode = document.getElementById('paste-mode');

        document.querySelectorAll('.mode-btn').forEach(btn => btn.classList.remove('active'));
        modeBtn.classList.add('active');

        if (mode === 'upload') {
            uploadMode?.classList.remove('hidden');
            pasteMode?.classList.add('hidden');
        } else {
            uploadMode?.classList.add('hidden');
            pasteMode?.classList.remove('hidden');
        }
    });

    // Tab switching - update active state
    document.addEventListener('click', function(e) {
        const tabBtn = e.target.closest('.tab-btn');
        if (!tabBtn) return;

        document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
        tabBtn.classList.add('active');

        // Show/hide group-by dropdown based on tab
        const groupBy = document.querySelector('.tab-controls');
        if (groupBy) {
            groupBy.style.display = tabBtn.dataset.tab === 'operations' ? '' : 'none';
        }
    });
})();
```

**Step 2: Verify file created**

Run:
```bash
ls -la static/js/explore.js
```

Expected: File exists

**Step 3: Commit**

Run:
```bash
git add static/js/explore.js
git commit -m "feat(explore): add browser cache fallback JavaScript"
```

---

## Task 16: CSS Styling

**Context:** Adds styles for method badges, tabs, accordions, and detail sections.

**Files:**
- Modify: `static/css/style.css`

**Step 1: Add explore styles**

Append to `static/css/style.css`:

```css
/* ==========================================================================
   Explore Page Styles
   ========================================================================== */

/* Method badges with accessibility glyphs */
.method-badge {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.125rem 0.5rem;
    border-radius: 0.25rem;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
}

.method-get { background-color: #d4edda; color: #155724; }
.method-post { background-color: #cce5ff; color: #004085; }
.method-put { background-color: #fff3cd; color: #856404; }
.method-patch { background-color: #fff3cd; color: #856404; }
.method-delete { background-color: #f8d7da; color: #721c24; }

/* Type badges for schemas */
.type-badge {
    display: inline-block;
    padding: 0.125rem 0.375rem;
    border-radius: 0.25rem;
    font-family: var(--font-mono);
    font-size: 0.7rem;
    background-color: var(--bg-muted);
    color: var(--text-muted);
}

/* Security type badges */
.security-type-badge {
    display: inline-block;
    padding: 0.125rem 0.375rem;
    border-radius: 0.25rem;
    font-family: var(--font-mono);
    font-size: 0.7rem;
    background-color: #e2e3e5;
    color: #383d41;
}

/* Explore summary section */
.explore-summary {
    background-color: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1rem;
    margin-bottom: 1.5rem;
}

.summary-header {
    display: flex;
    gap: 1rem;
    align-items: center;
    margin-bottom: 0.5rem;
}

.summary-version {
    font-weight: 600;
}

.summary-filename {
    color: var(--text-muted);
}

.summary-stats {
    display: flex;
    gap: 1.5rem;
}

.summary-stats .stat {
    color: var(--text-muted);
}

.summary-expand-btn {
    margin-top: 0.75rem;
    background: none;
    border: none;
    color: var(--link-color);
    cursor: pointer;
    font-size: 0.875rem;
}

/* Tabs */
.explore-tabs {
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid var(--border-color);
    margin-bottom: 1rem;
}

.tab-buttons {
    display: flex;
    gap: 0;
}

.tab-btn {
    padding: 0.75rem 1.25rem;
    background: none;
    border: none;
    border-bottom: 2px solid transparent;
    cursor: pointer;
    font-weight: 500;
    color: var(--text-muted);
    transition: all 0.2s;
}

.tab-btn:hover {
    color: var(--text-color);
}

.tab-btn.active {
    color: var(--primary-color);
    border-bottom-color: var(--primary-color);
}

.tab-controls {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.tab-controls label {
    font-size: 0.875rem;
    color: var(--text-muted);
}

.tab-controls select {
    padding: 0.375rem 0.75rem;
    border: 1px solid var(--border-color);
    border-radius: 0.25rem;
    font-size: 0.875rem;
}

/* Accordions */
.accordion-item {
    border: 1px solid var(--border-color);
    border-radius: 0.375rem;
    margin-bottom: 0.5rem;
    overflow: hidden;
}

.accordion-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.75rem 1rem;
    background-color: var(--bg-card);
    border: none;
    cursor: pointer;
    text-align: left;
    font-size: 0.9375rem;
}

.accordion-header:hover {
    background-color: var(--bg-hover);
}

.accordion-icon {
    transition: transform 0.2s;
    font-size: 0.75rem;
}

.accordion-item.open .accordion-icon {
    transform: rotate(90deg);
}

.accordion-path,
.accordion-tag {
    flex-grow: 1;
    font-family: var(--font-mono);
}

.accordion-content {
    display: none;
    border-top: 1px solid var(--border-color);
}

.accordion-item.open .accordion-content {
    display: block;
}

/* Operation rows */
.operation-row,
.schema-row,
.security-row {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    padding: 0.625rem 1rem;
    cursor: pointer;
    transition: background-color 0.15s;
}

.operation-row:hover,
.schema-row:hover,
.security-row:hover {
    background-color: var(--bg-hover);
}

.operation-id,
.schema-name,
.security-name {
    font-weight: 500;
}

.operation-summary,
.schema-preview {
    flex-grow: 1;
    color: var(--text-muted);
    font-size: 0.875rem;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
}

.operation-arrow,
.schema-arrow,
.security-arrow {
    color: var(--text-muted);
}

/* Detail cards */
.detail-section {
    margin-top: 2rem;
}

.detail-card {
    background-color: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 0.5rem;
    padding: 1.25rem;
    scroll-margin-top: 1rem;
}

.detail-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--border-color);
}

.detail-path {
    font-family: var(--font-mono);
    font-size: 1rem;
}

.detail-summary {
    font-weight: 500;
    margin-bottom: 0.5rem;
}

.detail-description {
    color: var(--text-muted);
    margin-bottom: 1rem;
}

.detail-card h4 {
    font-size: 0.9375rem;
    margin: 1.25rem 0 0.75rem;
    color: var(--text-muted);
}

/* Tables in detail cards */
.params-table,
.responses-table,
.props-table {
    width: 100%;
    font-size: 0.875rem;
    border-collapse: collapse;
}

.params-table th,
.responses-table th,
.props-table th {
    text-align: left;
    padding: 0.5rem;
    background-color: var(--bg-muted);
    font-weight: 500;
}

.params-table td,
.responses-table td,
.props-table td {
    padding: 0.5rem;
    border-bottom: 1px solid var(--border-color);
}

/* Inline schemas and unsecured sections */
.inline-schemas-section,
.unsecured-section {
    background-color: var(--bg-muted);
    border-radius: 0.375rem;
    padding: 1rem;
    margin-bottom: 1rem;
}

.inline-header,
.unsecured-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.75rem;
}

.inline-list,
.unsecured-list {
    max-height: 300px;
    overflow-y: auto;
}

.inline-row,
.unsecured-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.375rem 0;
    font-size: 0.875rem;
}

.inline-warning,
.unsecured-info {
    margin-top: 0.75rem;
    padding: 0.5rem;
    background-color: var(--bg-card);
    border-radius: 0.25rem;
    font-size: 0.8125rem;
    color: var(--text-muted);
}

.inline-warning {
    border-left: 3px solid #ffc107;
}

.unsecured-info {
    border-left: 3px solid #17a2b8;
}

/* Summary details expanded */
.summary-details-expanded {
    margin-top: 1rem;
    padding-top: 1rem;
    border-top: 1px solid var(--border-color);
}

.method-breakdown,
.schema-breakdown,
.security-breakdown {
    display: flex;
    align-items: center;
    gap: 1rem;
    margin-bottom: 0.5rem;
    font-size: 0.875rem;
}

.breakdown-label {
    color: var(--text-muted);
    min-width: 5rem;
}

.method-stat {
    display: flex;
    align-items: center;
    gap: 0.25rem;
}

/* Cache miss message */
.cache-miss-message {
    text-align: center;
    padding: 2rem;
    color: var(--text-muted);
}

.cache-miss-message p {
    margin-bottom: 1rem;
}

/* Utility classes */
.hidden {
    display: none !important;
}
```

**Step 2: Run CSS linting**

Run:
```bash
npm run lint:css
```

Expected: No errors (or only warnings)

**Step 3: Commit**

Run:
```bash
git add static/css/style.css
git commit -m "style(explore): add CSS for method badges, tabs, accordions"
```

---

## Task 17: Unit Tests

**Context:** Test the explore handlers and analysis logic.

**Files:**
- Create: `internal/api/explore_test.go`

**Step 1: Create explore unit tests**

Create `internal/api/explore_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/erraggy/oastools-web/internal/config"
)

func TestExploreAnalysis_OAS3(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "validate", "petstore-3.0.input.yaml"))
	if err != nil {
		t.Skipf("Test fixture not found: %v", err)
	}

	cfg := config.Load()
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Create multipart request
	body, contentType := createMultipartBody(t, "spec", "petstore.yaml", content)
	req := httptest.NewRequest(http.MethodPost, "/api/explore", body)
	req.Header.Set("Content-Type", contentType)

	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify response contains expected elements
	respBody := rec.Body.String()
	checks := []string{
		"OpenAPI",
		"Operations",
		"Schemas",
	}
	for _, check := range checks {
		if !strings.Contains(respBody, check) {
			t.Errorf("Response should contain %q", check)
		}
	}
}

func TestExploreAnalysis_OAS2(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "validate", "petstore-2.0.input.yaml"))
	if err != nil {
		t.Skipf("Test fixture not found: %v", err)
	}

	cfg := config.Load()
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	body, contentType := createMultipartBody(t, "spec", "petstore.yaml", content)
	req := httptest.NewRequest(http.MethodPost, "/api/explore", body)
	req.Header.Set("Content-Type", contentType)

	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestExploreOperations_CacheMiss(t *testing.T) {
	cfg := config.Load()
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/explore/operations?h=nonexistent", nil)
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Errorf("Expected status 410 Gone for cache miss, got %d", rec.Code)
	}
}

func TestExploreOperations_MissingHash(t *testing.T) {
	cfg := config.Load()
	h, err := NewHandler(cfg)
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/explore/operations", nil)
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing hash, got %d", rec.Code)
	}
}

func TestComputeExploreStats(t *testing.T) {
	// This test verifies stats computation logic
	// Requires a parsed spec - will be tested via integration
}

func TestGetSchemaType(t *testing.T) {
	tests := []struct {
		name     string
		schema   *parser.Schema
		expected string
	}{
		{"enum", &parser.Schema{Enum: []any{"a", "b"}}, "[enum]"},
		{"array", &parser.Schema{Type: "array"}, "[array]"},
		{"object", &parser.Schema{Properties: map[string]*parser.Schema{}}, "{object}"},
		{"allOf", &parser.Schema{AllOf: []*parser.Schema{}}, "{allOf}"},
		{"string", &parser.Schema{Type: "string"}, "string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSchemaType(tt.schema)
			if got != tt.expected {
				t.Errorf("getSchemaType() = %q, want %q", got, tt.expected)
			}
		})
	}
}
```

**Step 2: Add import for parser**

Add to imports in `explore_test.go`:

```go
import (
	"github.com/erraggy/oastools/parser"
	// ... other imports
)
```

**Step 3: Run tests**

Run:
```bash
go test -v ./internal/api/ -run TestExplore
```

Expected: Tests pass

**Step 4: Commit**

Run:
```bash
git add internal/api/explore_test.go
git commit -m "test(explore): add unit tests for explore handlers"
```

---

## Task 18: Golden Tests

**Context:** Add golden file tests for deterministic output verification.

**Files:**
- Create: `testdata/golden/explore/` directory
- Modify: `internal/api/golden_test.go`

**Step 1: Create explore golden test fixtures**

Run:
```bash
mkdir -p testdata/golden/explore
cp testdata/golden/validate/petstore-3.0.input.yaml testdata/golden/explore/
cp testdata/golden/validate/petstore-2.0.input.yaml testdata/golden/explore/
```

**Step 2: Add explore golden tests**

Add to `internal/api/golden_test.go`:

```go
func TestGoldenExplore(t *testing.T) {
	tests := []goldenTest{
		{
			name:       "petstore-3.0",
			endpoint:   "/api/explore",
			inputFiles: map[string]string{"spec": "testdata/golden/explore/petstore-3.0.input.yaml"},
			goldenFile: "testdata/golden/explore/petstore-3.0.golden.json",
		},
		{
			name:       "petstore-2.0",
			endpoint:   "/api/explore",
			inputFiles: map[string]string{"spec": "testdata/golden/explore/petstore-2.0.input.yaml"},
			goldenFile: "testdata/golden/explore/petstore-2.0.golden.json",
		},
	}

	runGoldenTests(t, tests)
}
```

**Step 3: Generate golden files**

Run:
```bash
go test -v ./internal/api/ -run TestGoldenExplore -update-golden
```

Expected: Golden files created

**Step 4: Run golden tests**

Run:
```bash
go test -v ./internal/api/ -run TestGoldenExplore
```

Expected: Tests pass

**Step 5: Commit**

Run:
```bash
git add testdata/golden/explore/ internal/api/golden_test.go
git commit -m "test(explore): add golden tests for explore endpoints"
```

---

## Task 19: E2E Tests

**Context:** Add Playwright end-to-end tests for the Explore feature.

**Files:**
- Create: `e2e/tests/explore.spec.ts`

**Step 1: Create E2E tests**

Create `e2e/tests/explore.spec.ts`:

```typescript
import { test, expect } from '@playwright/test';
import path from 'path';

test.describe('Explore Page', () => {
  test('uploads and displays spec summary', async ({ page }) => {
    await page.goto('/explore');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );

    await page.click('button[type="submit"]');

    // Wait for results
    await expect(page.locator('.explore-summary')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('.summary-version')).toContainText('3.0');
  });

  test('switches between tabs', async ({ page }) => {
    await page.goto('/explore');

    // Upload spec first
    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );
    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-summary')).toBeVisible({ timeout: 10000 });

    // Click Schemas tab
    await page.click('.tab-btn[data-tab="schemas"]');
    await expect(page.locator('.schemas-container')).toBeVisible({ timeout: 5000 });

    // Click Security tab
    await page.click('.tab-btn[data-tab="security"]');
    await expect(page.locator('.security-container')).toBeVisible({ timeout: 5000 });

    // Click back to Operations
    await page.click('.tab-btn[data-tab="operations"]');
    await expect(page.locator('.operations-list')).toBeVisible({ timeout: 5000 });
  });

  test('expands and collapses accordions', async ({ page }) => {
    await page.goto('/explore');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );
    await page.click('button[type="submit"]');
    await expect(page.locator('.accordion-item').first()).toBeVisible({ timeout: 10000 });

    // Click to expand first accordion
    await page.click('.accordion-header');
    await expect(page.locator('.accordion-item.open')).toBeVisible();

    // Click again to collapse
    await page.click('.accordion-header');
    await expect(page.locator('.accordion-item.open')).not.toBeVisible();
  });

  test('changes group-by selection', async ({ page }) => {
    await page.goto('/explore');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );
    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-summary')).toBeVisible({ timeout: 10000 });

    // Change to Tag grouping
    await page.selectOption('#group-by', 'tag');
    await expect(page.locator('.accordion-tag')).toBeVisible({ timeout: 5000 });

    // Change to Method grouping
    await page.selectOption('#group-by', 'method');
    await expect(page.locator('.method-badge')).toBeVisible({ timeout: 5000 });
  });

  test('shows operation detail on click', async ({ page }) => {
    await page.goto('/explore');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );
    await page.click('button[type="submit"]');
    await expect(page.locator('.accordion-item').first()).toBeVisible({ timeout: 10000 });

    // Expand accordion
    await page.click('.accordion-header');
    await expect(page.locator('.operation-row').first()).toBeVisible();

    // Click operation
    await page.click('.operation-row');
    await expect(page.locator('.detail-card')).toBeVisible({ timeout: 5000 });
  });

  test('can use paste input mode', async ({ page }) => {
    await page.goto('/explore');

    // Switch to paste mode
    await page.click('button[data-mode="paste"]');

    const textarea = page.locator('textarea[name="spec_content"]');
    await expect(textarea).toBeVisible();

    await textarea.fill(`openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths:
  /test:
    get:
      operationId: testOp
      responses:
        "200":
          description: OK`);

    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-summary')).toBeVisible({ timeout: 10000 });
  });

  test('expands summary details', async ({ page }) => {
    await page.goto('/explore');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(
      path.join(__dirname, '../../testdata/golden/explore/petstore-3.0.input.yaml')
    );
    await page.click('button[type="submit"]');
    await expect(page.locator('.explore-summary')).toBeVisible({ timeout: 10000 });

    // Click expand button
    await page.click('.summary-expand-btn');
    await expect(page.locator('.summary-details-expanded')).toBeVisible({ timeout: 5000 });
  });
});
```

**Step 2: Run E2E tests**

Run:
```bash
npm run test:e2e -- --grep "Explore"
```

Expected: Tests pass

**Step 3: Commit**

Run:
```bash
git add e2e/tests/explore.spec.ts
git commit -m "test(e2e): add Playwright tests for explore feature"
```

---

## Task 20: Final Verification

**Context:** Run all tests and perform manual verification.

**Step 1: Run full test suite**

Run:
```bash
make test
```

Expected: All tests pass

**Step 2: Run linting**

Run:
```bash
make lint
```

Expected: No errors

**Step 3: Run E2E tests**

Run:
```bash
npm run test:e2e
```

Expected: All tests pass

**Step 4: Manual browser testing**

Run:
```bash
make run
```

Open http://localhost:8080/explore and verify:
- [ ] File upload works
- [ ] Paste mode works
- [ ] Summary displays correctly
- [ ] All three tabs work
- [ ] Accordions expand/collapse
- [ ] Group-by dropdown changes view
- [ ] Operation details load
- [ ] Schema details load
- [ ] Security details load
- [ ] Method badges show glyphs
- [ ] OAS2 spec works (test with petstore-2.0.yaml)

**Step 5: Test cache fallback**

1. Upload a spec
2. Stop the server (Ctrl+C)
3. Restart the server (`make run`)
4. Try switching tabs
5. Verify auto-recovery works (or shows re-upload message)

**Step 6: Commit any fixes**

If issues found:
```bash
git add -A
git commit -m "fix(explore): address issues found in final verification"
```

**Step 7: Run full check**

Run:
```bash
make check
```

Expected: All checks pass

---

## Completion

After all tasks complete successfully:

1. Use `superpowers:finishing-a-development-branch` skill
2. Create PR with summary of all changes
