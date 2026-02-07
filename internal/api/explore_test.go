package api

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/erraggy/oastools-web/internal/config"
	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/walker"
)

// validOpenAPI30WithOperations is an OpenAPI spec with multiple operations for testing.
const validOpenAPI30WithOperations = `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /pets:
    get:
      summary: List all pets
      operationId: listPets
      tags:
        - pets
      responses:
        "200":
          description: A list of pets
    post:
      summary: Create a pet
      operationId: createPet
      tags:
        - pets
      responses:
        "201":
          description: Pet created
  /pets/{petId}:
    get:
      summary: Get a pet by ID
      operationId: getPet
      tags:
        - pets
      responses:
        "200":
          description: A pet
    delete:
      summary: Delete a pet
      operationId: deletePet
      tags:
        - pets
      responses:
        "204":
          description: Pet deleted
`

// exploreTestHandler creates a handler for explore tests with the explore_operations template.
func exploreTestHandler(t *testing.T) *Handler {
	t.Helper()

	partials, err := template.New("partials").Parse(`
{{define "error"}}Error: {{.Message}}{{end}}
{{define "explore_operations"}}
<div class="operations-list">
{{if eq .Group "path"}}
  {{range $path, $ops := .Analysis.Operations.ByPath}}
    <div class="group">{{$path}}: {{len $ops}} ops</div>
  {{end}}
{{else if eq .Group "tag"}}
  {{range $tag, $ops := .Analysis.Operations.ByTag}}
    <div class="group">{{$tag}}: {{len $ops}} ops</div>
  {{end}}
{{else if eq .Group "method"}}
  {{range $method, $ops := .Analysis.Operations.ByMethod}}
    <div class="group">{{$method}}: {{len $ops}} ops</div>
  {{end}}
{{end}}
</div>
{{end}}
{{define "explore_operation_detail"}}
<div class="detail-card" id="op-{{.OperationID}}">
  <span class="method-badge">{{.Method}}</span>
  <span class="detail-path">{{.PathTemplate}}</span>
  {{if .Operation.Summary}}<p class="detail-summary">{{.Operation.Summary}}</p>{{end}}
  {{if .Operation.Description}}<p class="detail-description">{{.Operation.Description}}</p>{{end}}
  {{if .Operation.Parameters}}<div class="params">{{len .Operation.Parameters}} params</div>{{end}}
  {{if .Operation.Responses}}<div class="responses">has responses</div>{{end}}
</div>
{{end}}
{{define "explore_schemas"}}
<div class="schemas-container">
  <div class="schemas-header">
    <span class="schemas-count">Component Schemas ({{len .Analysis.Schemas.Components}})</span>
    <span class="inline-count">Inline Schemas: {{.Analysis.Stats.InlineCount}}</span>
  </div>
  <div class="schemas-list">
    {{range .Analysis.Schemas.Components}}
    <div class="schema-row">{{.Name}}</div>
    {{end}}
  </div>
</div>
{{end}}
`)
	if err != nil {
		t.Fatalf("failed to create test partials: %v", err)
	}

	return &Handler{
		cfg: &config.Config{
			MaxFileSize: 2 << 20,
		},
		partials:        partials,
		instruments:     newInstruments(),
		version:         "test-version",
		oastoolsVersion: "test-oastools-version",
	}
}

// setupTestAnalysis creates and caches a test analysis for the given hash.
func setupTestAnalysis(t *testing.T, hash string) {
	t.Helper()

	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithOperations)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		t.Fatalf("failed to collect schemas: %v", err)
	}

	analysis := &ExploreAnalysis{
		Hash:        hash,
		Version:     "3.0.0",
		Filename:    "test.yaml",
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
		Stats: ExploreStats{
			PathCount:      2,
			OperationCount: 4,
		},
	}

	exploreCache.Set(hash, analysis)
}

func TestHandler_handleExploreOperations(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "default grouping by path",
			queryParams:  "h=testhash1",
			setupHash:    "testhash1",
			wantStatus:   http.StatusOK,
			wantContains: "/pets:",
		},
		{
			name:         "explicit path grouping",
			queryParams:  "h=testhash2&group=path",
			setupHash:    "testhash2",
			wantStatus:   http.StatusOK,
			wantContains: "/pets/{petId}:",
		},
		{
			name:         "tag grouping",
			queryParams:  "h=testhash3&group=tag",
			setupHash:    "testhash3",
			wantStatus:   http.StatusOK,
			wantContains: "pets:",
		},
		{
			name:         "method grouping",
			queryParams:  "h=testhash4&group=method",
			setupHash:    "testhash4",
			wantStatus:   http.StatusOK,
			wantContains: "get:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := exploreTestHandler(t)

			// Setup cache if needed
			if tt.setupHash != "" {
				setupTestAnalysis(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/operations?"+tt.queryParams, nil)
			resp := h.handleExploreOperations(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// Check for cache expired response
			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			// Check response body contains expected content
			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

func TestHandler_handleExploreOperationDetail(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent&path=/pets&method=get",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "missing path parameter",
			queryParams:  "h=opdetail1&method=get",
			setupHash:    "opdetail1",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing path or method",
		},
		{
			name:         "missing method parameter",
			queryParams:  "h=opdetail2&path=/pets",
			setupHash:    "opdetail2",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing path or method",
		},
		{
			name:         "operation not found",
			queryParams:  "h=opdetail3&path=/nonexistent&method=get",
			setupHash:    "opdetail3",
			wantStatus:   http.StatusNotFound,
			wantContains: "Operation not found",
		},
		{
			name:         "successful operation detail",
			queryParams:  "h=opdetail4&path=/pets&method=get",
			setupHash:    "opdetail4",
			wantStatus:   http.StatusOK,
			wantContains: "List all pets",
		},
		{
			name:         "operation with path parameter",
			queryParams:  "h=opdetail5&path=/pets/{petId}&method=get",
			setupHash:    "opdetail5",
			wantStatus:   http.StatusOK,
			wantContains: "Get a pet by ID",
		},
		{
			name:         "uses operationId when available",
			queryParams:  "h=opdetail6&path=/pets&method=post",
			setupHash:    "opdetail6",
			wantStatus:   http.StatusOK,
			wantContains: "op-createPet",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := exploreTestHandler(t)

			// Setup cache if needed
			if tt.setupHash != "" {
				setupTestAnalysis(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/operation?"+tt.queryParams, nil)
			resp := h.handleExploreOperationDetail(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// Check for cache expired response
			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			// Check response body contains expected content
			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

func TestHandler_handleExploreSchemas(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "successful schemas render",
			queryParams:  "h=schemahash1",
			setupHash:    "schemahash1",
			wantStatus:   http.StatusOK,
			wantContains: "schemas-container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := exploreTestHandler(t)

			// Setup cache if needed
			if tt.setupHash != "" {
				setupTestAnalysis(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/schemas?"+tt.queryParams, nil)
			resp := h.handleExploreSchemas(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// Check for cache expired response
			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			// Check response body contains expected content
			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

func TestCacheExpiredResponse(t *testing.T) {
	resp := &cacheExpiredResponse{}

	if resp.StatusCode() != http.StatusGone {
		t.Errorf("StatusCode() = %d, want %d", resp.StatusCode(), http.StatusGone)
	}

	if resp.Headers() != nil {
		t.Errorf("Headers() = %v, want nil", resp.Headers())
	}

	if resp.Body() != nil {
		t.Errorf("Body() = %v, want nil", resp.Body())
	}

	rec := httptest.NewRecorder()
	if err := resp.WriteTo(rec); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	if rec.Code != http.StatusGone {
		t.Errorf("response code = %d, want %d", rec.Code, http.StatusGone)
	}

	if rec.Header().Get("HX-Trigger") != "cacheExpired" {
		t.Errorf("HX-Trigger header = %q, want cacheExpired", rec.Header().Get("HX-Trigger"))
	}
}

func TestExploreCache_Integration(t *testing.T) {
	// Test that the explore cache works correctly with the handler
	hash := "integration-test-hash"
	setupTestAnalysis(t, hash)
	t.Cleanup(func() {
		exploreCache.Delete(hash)
	})

	// Verify cache hit
	analysis, ok := exploreCache.Get(hash)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if analysis.Hash != hash {
		t.Errorf("analysis.Hash = %q, want %q", analysis.Hash, hash)
	}
	if analysis.Operations == nil {
		t.Fatal("expected operations to be set")
	}
	if len(analysis.Operations.All) != 4 {
		t.Errorf("operations count = %d, want 4", len(analysis.Operations.All))
	}

	// Verify groupings
	if len(analysis.Operations.ByPath) != 2 {
		t.Errorf("ByPath groups = %d, want 2", len(analysis.Operations.ByPath))
	}
	if len(analysis.Operations.ByTag) != 1 {
		t.Errorf("ByTag groups = %d, want 1", len(analysis.Operations.ByTag))
	}
	if len(analysis.Operations.ByMethod) != 3 { // get, post, delete
		t.Errorf("ByMethod groups = %d, want 3", len(analysis.Operations.ByMethod))
	}
}

// validOpenAPI30WithInlineSchemas is an OpenAPI spec with inline schemas for testing.
const validOpenAPI30WithInlineSchemas = `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /pets:
    post:
      summary: Create a pet
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
      responses:
        "200":
          description: A pet
          content:
            application/json:
              schema:
                type: object
                properties:
                  id:
                    type: integer
        "400":
          description: Error
          content:
            application/json:
              schema:
                type: object
                properties:
                  message:
                    type: string
  /items:
    get:
      summary: List items
      parameters:
        - name: filter
          in: query
          schema:
            type: string
            enum: [active, inactive]
      responses:
        "200":
          description: Items list
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
`

// inlineSchemaTestHandler creates a handler for inline schema tests.
func inlineSchemaTestHandler(t *testing.T) *Handler {
	t.Helper()

	partials, err := template.New("partials").Parse(`
{{define "error"}}Error: {{.Message}}{{end}}
{{define "explore_inline_schemas"}}
<div class="inline-schemas-section">
    <div class="inline-header">
        <span>Inline Schemas: {{len .InlineSchemas}}</span>
    </div>
    <div class="inline-list">
        {{range .InlineSchemas}}
        <div class="inline-row">
            <span class="method-badge">{{.Method}}</span>
            <span class="inline-path">{{.PathTemplate}}</span>
            <span class="inline-context">({{.Context}})</span>
            <span class="type-badge">{{.Type}}</span>
        </div>
        {{end}}
    </div>
    {{if gt (len .InlineSchemas) 10}}
    <div class="inline-warning">Warning</div>
    {{end}}
</div>
{{end}}
`)
	if err != nil {
		t.Fatalf("failed to create test partials: %v", err)
	}

	return &Handler{
		cfg: &config.Config{
			MaxFileSize: 2 << 20,
		},
		partials:        partials,
		instruments:     newInstruments(),
		version:         "test-version",
		oastoolsVersion: "test-oastools-version",
	}
}

// setupInlineSchemaTestAnalysis creates and caches an analysis with inline schemas.
func setupInlineSchemaTestAnalysis(t *testing.T, hash string) {
	t.Helper()

	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithInlineSchemas)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		t.Fatalf("failed to collect schemas: %v", err)
	}

	analysis := &ExploreAnalysis{
		Hash:        hash,
		Version:     "3.0.0",
		Filename:    "test.yaml",
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
		Stats: ExploreStats{
			PathCount:      2,
			OperationCount: 2,
			InlineCount:    len(schemas.Inline),
		},
	}

	exploreCache.Set(hash, analysis)
}

func TestHandler_handleExploreInlineSchemas(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   []string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{"Missing hash parameter"},
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:        "successful inline schemas render",
			queryParams: "h=inlinehash1",
			setupHash:   "inlinehash1",
			wantStatus:  http.StatusOK,
			wantContains: []string{
				"inline-schemas-section",
				"Inline Schemas:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := inlineSchemaTestHandler(t)

			// Setup cache if needed
			if tt.setupHash != "" {
				setupInlineSchemaTestAnalysis(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/inline-schemas?"+tt.queryParams, nil)
			resp := h.handleExploreInlineSchemas(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// Check for cache expired response
			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			// Check response body contains expected content
			if len(tt.wantContains) > 0 {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				for _, want := range tt.wantContains {
					if !strings.Contains(body, want) {
						t.Errorf("body = %q, want contains %q", body, want)
					}
				}
			}
		})
	}
}

func TestParseInlineLocations(t *testing.T) {
	tests := []struct {
		name     string
		jsonPath string
		want     InlineSchemaLocation
	}{
		{
			name:     "request body schema",
			jsonPath: "$.paths['/pets'].post.requestBody.content['application/json'].schema",
			want: InlineSchemaLocation{
				Method:       "post",
				PathTemplate: "/pets",
				Context:      "request body",
			},
		},
		{
			name:     "response 200 schema",
			jsonPath: "$.paths['/pets'].get.responses['200'].content['application/json'].schema",
			want: InlineSchemaLocation{
				Method:       "get",
				PathTemplate: "/pets",
				Context:      "response 200",
			},
		},
		{
			name:     "response default schema",
			jsonPath: "$.paths['/pets'].get.responses.default.content['application/json'].schema",
			want: InlineSchemaLocation{
				Method:       "get",
				PathTemplate: "/pets",
				Context:      "response default",
			},
		},
		{
			name:     "parameter schema",
			jsonPath: "$.paths['/items'].get.parameters[0].schema",
			want: InlineSchemaLocation{
				Method:       "get",
				PathTemplate: "/items",
				Context:      "parameter",
			},
		},
		{
			name:     "path with special characters",
			jsonPath: "$.paths['/pets/{petId}'].put.requestBody.content['application/json'].schema",
			want: InlineSchemaLocation{
				Method:       "put",
				PathTemplate: "/pets/{petId}",
				Context:      "request body",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock analysis with a single inline schema
			analysis := &ExploreAnalysis{
				Schemas: &walker.SchemaCollector{
					Inline: []*walker.SchemaInfo{
						{JSONPath: tt.jsonPath, Schema: &parser.Schema{}},
					},
				},
			}

			locations := parseInlineLocations(analysis)
			if len(locations) != 1 {
				t.Fatalf("got %d locations, want 1", len(locations))
			}

			got := locations[0]
			if got.Method != tt.want.Method {
				t.Errorf("Method = %q, want %q", got.Method, tt.want.Method)
			}
			if got.PathTemplate != tt.want.PathTemplate {
				t.Errorf("PathTemplate = %q, want %q", got.PathTemplate, tt.want.PathTemplate)
			}
			if got.Context != tt.want.Context {
				t.Errorf("Context = %q, want %q", got.Context, tt.want.Context)
			}
		})
	}
}

func TestParseInlineLocations_NilCases(t *testing.T) {
	// Test nil analysis
	if got := parseInlineLocations(nil); got != nil {
		t.Errorf("parseInlineLocations(nil) = %v, want nil", got)
	}

	// Test nil schemas
	analysis := &ExploreAnalysis{Schemas: nil}
	if got := parseInlineLocations(analysis); got != nil {
		t.Errorf("parseInlineLocations(nil schemas) = %v, want nil", got)
	}

	// Test empty inline schemas
	analysis = &ExploreAnalysis{
		Schemas: &walker.SchemaCollector{
			Inline: []*walker.SchemaInfo{},
		},
	}
	got := parseInlineLocations(analysis)
	if len(got) != 0 {
		t.Errorf("parseInlineLocations(empty) = %v, want empty slice", got)
	}
}

func TestGetSchemaType(t *testing.T) {
	tests := []struct {
		name   string
		schema *parser.Schema
		want   string
	}{
		{
			name:   "nil schema",
			schema: nil,
			want:   "",
		},
		{
			name:   "enum schema",
			schema: &parser.Schema{Enum: []any{"a", "b", "c"}},
			want:   "[enum]",
		},
		{
			name:   "array schema",
			schema: &parser.Schema{Type: "array"},
			want:   "[array]",
		},
		{
			name:   "object with properties",
			schema: &parser.Schema{Properties: map[string]*parser.Schema{"id": {}}},
			want:   "{object}",
		},
		{
			name:   "allOf schema",
			schema: &parser.Schema{AllOf: []*parser.Schema{{}, {}}},
			want:   "{allOf}",
		},
		{
			name:   "oneOf schema",
			schema: &parser.Schema{OneOf: []*parser.Schema{{}, {}}},
			want:   "{oneOf}",
		},
		{
			name:   "anyOf schema",
			schema: &parser.Schema{AnyOf: []*parser.Schema{{}, {}}},
			want:   "{anyOf}",
		},
		{
			name:   "string type",
			schema: &parser.Schema{Type: "string"},
			want:   "string",
		},
		{
			name:   "integer type",
			schema: &parser.Schema{Type: "integer"},
			want:   "integer",
		},
		{
			name:   "OAS 3.1 nullable type array",
			schema: &parser.Schema{Type: []any{"string", "null"}},
			want:   "string",
		},
		{
			name:   "OAS 3.1 null-only type",
			schema: &parser.Schema{Type: []any{"null"}},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSchemaType(tt.schema)
			if got != tt.want {
				t.Errorf("getSchemaType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatTypeString(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "string type",
			input: "string",
			want:  "string",
		},
		{
			name:  "array with non-null type",
			input: []any{"string", "null"},
			want:  "string",
		},
		{
			name:  "array with null first",
			input: []any{"null", "integer"},
			want:  "integer",
		},
		{
			name:  "array with only null",
			input: []any{"null"},
			want:  "",
		},
		{
			name:  "nil input",
			input: nil,
			want:  "",
		},
		{
			name:  "unexpected type",
			input: 123,
			want:  "",
		},
		{
			name:  "empty array",
			input: []any{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTypeString(tt.input)
			if got != tt.want {
				t.Errorf("formatTypeString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// validOpenAPI30WithSecurity is an OpenAPI spec with security schemes for testing.
const validOpenAPI30WithSecurity = `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
security:
  - bearerAuth: []
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
    apiKey:
      type: apiKey
      in: header
      name: X-API-Key
  schemas:
    Pet:
      type: object
      properties:
        id:
          type: integer
        name:
          type: string
    Error:
      type: object
      properties:
        message:
          type: string
paths:
  /pets:
    get:
      summary: List all pets
      operationId: listPets
      responses:
        "200":
          description: A list of pets
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
    post:
      summary: Create a pet
      operationId: createPet
      security:
        - apiKey: []
      requestBody:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/Pet'
      responses:
        "201":
          description: Pet created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Pet'
        "400":
          description: Bad request
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Error'
  /public:
    get:
      summary: Public endpoint
      operationId: publicEndpoint
      security: []
      responses:
        "200":
          description: Public data
`

// validOpenAPI20WithSecurity is an OpenAPI 2.0 spec with security for testing.
const validOpenAPI20WithSecurity = `swagger: "2.0"
info:
  title: Test API
  version: "1.0.0"
security:
  - apiKey: []
securityDefinitions:
  apiKey:
    type: apiKey
    in: header
    name: X-API-Key
  oauth2:
    type: oauth2
    flow: accessCode
    authorizationUrl: https://example.com/oauth/authorize
    tokenUrl: https://example.com/oauth/token
    scopes:
      read: Read access
      write: Write access
paths:
  /items:
    get:
      summary: List items
      operationId: listItems
      responses:
        "200":
          description: A list of items
`

func TestComputeHash(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		wantLen int
	}{
		{
			name:    "basic content",
			content: []byte("openapi: 3.0.0"),
			wantLen: 16,
		},
		{
			name:    "empty content",
			content: []byte{},
			wantLen: 16,
		},
		{
			name:    "large content",
			content: []byte(validOpenAPI30WithOperations),
			wantLen: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeHash(tt.content)
			if len(got) != tt.wantLen {
				t.Errorf("computeHash() length = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	content := []byte("openapi: 3.0.0")
	hash1 := computeHash(content)
	hash2 := computeHash(content)

	if hash1 != hash2 {
		t.Errorf("computeHash() not deterministic: %q != %q", hash1, hash2)
	}
}

func TestComputeHash_DifferentContentDifferentHash(t *testing.T) {
	content1 := []byte("openapi: 3.0.0")
	content2 := []byte("openapi: 3.1.0")
	hash1 := computeHash(content1)
	hash2 := computeHash(content2)

	if hash1 == hash2 {
		t.Error("computeHash() produced same hash for different content")
	}
}

func TestExtractSecuritySchemes(t *testing.T) {
	tests := []struct {
		name       string
		specYAML   string
		wantCount  int
		wantScheme string
		wantType   string
	}{
		{
			name:       "OAS3 with security schemes",
			specYAML:   validOpenAPI30WithSecurity,
			wantCount:  2,
			wantScheme: "bearerAuth",
			wantType:   "http",
		},
		{
			name:       "OAS2 with security definitions",
			specYAML:   validOpenAPI20WithSecurity,
			wantCount:  2,
			wantScheme: "apiKey",
			wantType:   "apiKey",
		},
		{
			name:      "spec without security",
			specYAML:  validOpenAPI30WithOperations,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(tt.specYAML)))
			if err != nil {
				t.Fatalf("failed to parse test spec: %v", err)
			}

			operations, err := walker.CollectOperations(parseResult)
			if err != nil {
				t.Fatalf("failed to collect operations: %v", err)
			}

			got := extractSecuritySchemes(parseResult, operations)

			if len(got) != tt.wantCount {
				t.Errorf("extractSecuritySchemes() returned %d schemes, want %d", len(got), tt.wantCount)
			}

			if tt.wantScheme != "" {
				found := false
				for _, scheme := range got {
					if scheme.Name == tt.wantScheme && scheme.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractSecuritySchemes() did not find scheme %q with type %q", tt.wantScheme, tt.wantType)
				}
			}
		})
	}
}

func TestExtractSecuritySchemes_NilCases(t *testing.T) {
	// Test nil result
	if got := extractSecuritySchemes(nil, nil); got != nil {
		t.Errorf("extractSecuritySchemes(nil, nil) = %v, want nil", got)
	}
}

func TestExtractSecuritySchemes_UsageCount(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithSecurity)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemes := extractSecuritySchemes(parseResult, operations)

	// Find bearerAuth - should be used by listPets (inherits global security)
	var bearerAuth *SecuritySchemeInfo
	var apiKeyScheme *SecuritySchemeInfo
	for i := range schemes {
		if schemes[i].Name == "bearerAuth" {
			bearerAuth = &schemes[i]
		}
		if schemes[i].Name == "apiKey" {
			apiKeyScheme = &schemes[i]
		}
	}

	if bearerAuth == nil {
		t.Fatal("bearerAuth scheme not found")
	}
	// listPets inherits global bearerAuth security
	if bearerAuth.UsageCount != 1 {
		t.Errorf("bearerAuth.UsageCount = %d, want 1", bearerAuth.UsageCount)
	}

	if apiKeyScheme == nil {
		t.Fatal("apiKey scheme not found")
	}
	// createPet explicitly uses apiKey
	if apiKeyScheme.UsageCount != 1 {
		t.Errorf("apiKey.UsageCount = %d, want 1", apiKeyScheme.UsageCount)
	}
}

func TestComputeExploreStats(t *testing.T) {
	tests := []struct {
		name             string
		specYAML         string
		wantPathCount    int
		wantOpCount      int
		wantSecuredCount int
		wantUnsecured    int
	}{
		{
			name:             "spec with security",
			specYAML:         validOpenAPI30WithSecurity,
			wantPathCount:    2,
			wantOpCount:      3,
			wantSecuredCount: 2, // listPets (inherits global) + createPet (explicit apiKey)
			wantUnsecured:    1, // publicEndpoint (explicit empty [])
		},
		{
			name:             "spec without security",
			specYAML:         validOpenAPI30WithOperations,
			wantPathCount:    2,
			wantOpCount:      4,
			wantSecuredCount: 0,
			wantUnsecured:    4, // All unsecured
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(tt.specYAML)))
			if err != nil {
				t.Fatalf("failed to parse test spec: %v", err)
			}

			operations, err := walker.CollectOperations(parseResult)
			if err != nil {
				t.Fatalf("failed to collect operations: %v", err)
			}

			schemas, err := walker.CollectSchemas(parseResult)
			if err != nil {
				t.Fatalf("failed to collect schemas: %v", err)
			}

			stats := computeExploreStats(parseResult, operations, schemas)

			if stats.PathCount != tt.wantPathCount {
				t.Errorf("PathCount = %d, want %d", stats.PathCount, tt.wantPathCount)
			}
			if stats.OperationCount != tt.wantOpCount {
				t.Errorf("OperationCount = %d, want %d", stats.OperationCount, tt.wantOpCount)
			}
			if stats.SecuredCount != tt.wantSecuredCount {
				t.Errorf("SecuredCount = %d, want %d", stats.SecuredCount, tt.wantSecuredCount)
			}
			if stats.UnsecuredCount != tt.wantUnsecured {
				t.Errorf("UnsecuredCount = %d, want %d", stats.UnsecuredCount, tt.wantUnsecured)
			}
		})
	}
}

func TestComputeExploreStats_NilCases(t *testing.T) {
	stats := computeExploreStats(nil, nil, nil)

	if stats.PathCount != 0 {
		t.Errorf("PathCount = %d, want 0", stats.PathCount)
	}
	if stats.OperationCount != 0 {
		t.Errorf("OperationCount = %d, want 0", stats.OperationCount)
	}
	if stats.MethodCounts == nil {
		t.Error("MethodCounts should be initialized even with nil inputs")
	}
}

func TestComputeExploreStats_MethodCounts(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithOperations)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		t.Fatalf("failed to collect schemas: %v", err)
	}

	stats := computeExploreStats(parseResult, operations, schemas)

	// The spec has: GET /pets, POST /pets, GET /pets/{petId}, DELETE /pets/{petId}
	if stats.MethodCounts["GET"] != 2 {
		t.Errorf("GET count = %d, want 2", stats.MethodCounts["GET"])
	}
	if stats.MethodCounts["POST"] != 1 {
		t.Errorf("POST count = %d, want 1", stats.MethodCounts["POST"])
	}
	if stats.MethodCounts["DELETE"] != 1 {
		t.Errorf("DELETE count = %d, want 1", stats.MethodCounts["DELETE"])
	}
}

// setupTestAnalysisWithSecurity creates and caches a test analysis with security schemes.
func setupTestAnalysisWithSecurity(t *testing.T, hash string) {
	t.Helper()

	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithSecurity)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		t.Fatalf("failed to collect schemas: %v", err)
	}

	security := extractSecuritySchemes(parseResult, operations)
	stats := computeExploreStats(parseResult, operations, schemas)

	analysis := &ExploreAnalysis{
		Hash:        hash,
		Version:     "3.0.0",
		Filename:    "test.yaml",
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
		Security:    security,
		Stats:       stats,
	}

	exploreCache.Set(hash, analysis)
}

func TestFindSchemaUsages(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithSecurity)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		t.Fatalf("failed to collect schemas: %v", err)
	}

	analysis := &ExploreAnalysis{
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
	}

	tests := []struct {
		name       string
		schemaName string
		wantCount  int
		wantUsage  *SchemaUsage
	}{
		{
			name:       "Pet schema used in multiple places",
			schemaName: "Pet",
			wantCount:  3, // GET response, POST request body, POST response
			wantUsage: &SchemaUsage{
				Method:       "get",
				PathTemplate: "/pets",
				Context:      "response 200",
			},
		},
		{
			name:       "Error schema used in one place",
			schemaName: "Error",
			wantCount:  1, // POST 400 response
			wantUsage: &SchemaUsage{
				Method:       "post",
				PathTemplate: "/pets",
				Context:      "response 400",
			},
		},
		{
			name:       "nonexistent schema",
			schemaName: "NonExistent",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usages := findSchemaUsages(analysis, tt.schemaName)

			if len(usages) != tt.wantCount {
				t.Errorf("findSchemaUsages(%q) returned %d usages, want %d", tt.schemaName, len(usages), tt.wantCount)
			}

			if tt.wantUsage != nil && tt.wantCount > 0 {
				found := false
				for _, u := range usages {
					if u.Method == tt.wantUsage.Method &&
						u.PathTemplate == tt.wantUsage.PathTemplate &&
						u.Context == tt.wantUsage.Context {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("findSchemaUsages(%q) did not find expected usage %+v", tt.schemaName, tt.wantUsage)
				}
			}
		})
	}
}

func TestFindSecurityUsages(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithSecurity)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	analysis := &ExploreAnalysis{
		ParseResult: parseResult,
		Operations:  operations,
	}

	tests := []struct {
		name       string
		schemeName string
		wantCount  int
		wantPath   string
	}{
		{
			name:       "bearerAuth used via global security",
			schemeName: "bearerAuth",
			wantCount:  1, // listPets inherits global
			wantPath:   "/pets",
		},
		{
			name:       "apiKey used explicitly",
			schemeName: "apiKey",
			wantCount:  1, // createPet
			wantPath:   "/pets",
		},
		{
			name:       "nonexistent scheme",
			schemeName: "nonexistent",
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usages := findSecurityUsages(analysis, tt.schemeName)

			if len(usages) != tt.wantCount {
				t.Errorf("findSecurityUsages(%q) returned %d usages, want %d", tt.schemeName, len(usages), tt.wantCount)
			}

			if tt.wantPath != "" && len(usages) > 0 {
				found := false
				for _, u := range usages {
					if u.PathTemplate == tt.wantPath {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("findSecurityUsages(%q) did not find expected path %q", tt.schemeName, tt.wantPath)
				}
			}
		})
	}
}

func TestFindSecurityUsages_NilCases(t *testing.T) {
	// Test nil analysis
	if got := findSecurityUsages(nil, "test"); len(got) != 0 {
		t.Errorf("findSecurityUsages(nil, ...) = %v, want empty slice", got)
	}

	// Test nil operations
	analysis := &ExploreAnalysis{Operations: nil}
	if got := findSecurityUsages(analysis, "test"); len(got) != 0 {
		t.Errorf("findSecurityUsages(nil operations) = %v, want empty slice", got)
	}
}

// securityTestHandler creates a handler for security tests with appropriate templates.
func securityTestHandler(t *testing.T) *Handler {
	t.Helper()

	partials, err := template.New("partials").Parse(`
{{define "error"}}Error: {{.Message}}{{end}}
{{define "explore_security"}}
<div class="security-container">
  <div class="security-header">Security Schemes: {{len .Analysis.Security}}</div>
  {{range .Analysis.Security}}
  <div class="security-row">{{.Name}}: {{.Type}} ({{.UsageCount}} uses)</div>
  {{end}}
</div>
{{end}}
{{define "explore_schema_detail"}}
<div class="schema-detail">
  <h3>{{.Name}}</h3>
  <div class="usages">Used in: {{len .UsedIn}} places</div>
</div>
{{end}}
{{define "explore_security_detail"}}
<div class="security-detail">
  <h3>{{.Scheme.Name}}</h3>
  <div class="type">Type: {{.Scheme.Type}}</div>
  <div class="usages">Used by: {{len .UsedBy}} operations</div>
</div>
{{end}}
`)
	if err != nil {
		t.Fatalf("failed to create test partials: %v", err)
	}

	return &Handler{
		cfg: &config.Config{
			MaxFileSize: 2 << 20,
		},
		partials:        partials,
		instruments:     newInstruments(),
		version:         "test-version",
		oastoolsVersion: "test-oastools-version",
	}
}

func TestHandler_handleExploreSecurity(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "successful security render",
			queryParams:  "h=securityhash1",
			setupHash:    "securityhash1",
			wantStatus:   http.StatusOK,
			wantContains: "security-container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := securityTestHandler(t)

			if tt.setupHash != "" {
				setupTestAnalysisWithSecurity(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/security?"+tt.queryParams, nil)
			resp := h.handleExploreSecurity(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

func TestHandler_handleExploreSchemaDetail(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:         "missing name parameter",
			queryParams:  "h=schemadetail1",
			setupHash:    "schemadetail1",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing name parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent&name=Pet",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "schema not found",
			queryParams:  "h=schemadetail2&name=NonExistent",
			setupHash:    "schemadetail2",
			wantStatus:   http.StatusNotFound,
			wantContains: "Schema not found",
		},
		{
			name:         "successful schema detail",
			queryParams:  "h=schemadetail3&name=Pet",
			setupHash:    "schemadetail3",
			wantStatus:   http.StatusOK,
			wantContains: "schema-detail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := securityTestHandler(t)

			if tt.setupHash != "" {
				setupTestAnalysisWithSecurity(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/schema?"+tt.queryParams, nil)
			resp := h.handleExploreSchemaDetail(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

func TestHandler_handleExploreSecurityDetail(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupHash      string
		wantStatus     int
		wantContains   string
		wantCacheEvent bool
	}{
		{
			name:         "missing hash parameter",
			queryParams:  "",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing hash parameter",
		},
		{
			name:         "missing name parameter",
			queryParams:  "h=secdetail1",
			setupHash:    "secdetail1",
			wantStatus:   http.StatusBadRequest,
			wantContains: "Missing name parameter",
		},
		{
			name:           "cache miss returns 410 Gone",
			queryParams:    "h=nonexistent&name=bearerAuth",
			wantStatus:     http.StatusGone,
			wantCacheEvent: true,
		},
		{
			name:         "security scheme not found",
			queryParams:  "h=secdetail2&name=NonExistent",
			setupHash:    "secdetail2",
			wantStatus:   http.StatusNotFound,
			wantContains: "Security scheme not found",
		},
		{
			name:         "successful security detail",
			queryParams:  "h=secdetail3&name=bearerAuth",
			setupHash:    "secdetail3",
			wantStatus:   http.StatusOK,
			wantContains: "security-detail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := securityTestHandler(t)

			if tt.setupHash != "" {
				setupTestAnalysisWithSecurity(t, tt.setupHash)
				t.Cleanup(func() {
					exploreCache.Delete(tt.setupHash)
				})
			}

			req := httptest.NewRequest(http.MethodGet, "/api/explore/security-detail?"+tt.queryParams, nil)
			resp := h.handleExploreSecurityDetail(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if tt.wantCacheEvent {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if rec.Header().Get("HX-Trigger") != "cacheExpired" {
					t.Errorf("expected HX-Trigger=cacheExpired header")
				}
				return
			}

			if tt.wantContains != "" {
				rec := httptest.NewRecorder()
				if err := resp.WriteTo(rec); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				body := rec.Body.String()
				if !strings.Contains(body, tt.wantContains) {
					t.Errorf("body = %q, want contains %q", body, tt.wantContains)
				}
			}
		})
	}
}

// uploadTestHandler creates a handler for upload tests with appropriate templates.
func uploadTestHandler(t *testing.T) *Handler {
	t.Helper()

	partials, err := template.New("partials").Parse(`
{{define "error"}}Error: {{.Message}}{{end}}
{{define "explore_results"}}
<div class="explore-results" data-hash="{{.Analysis.Hash}}">
  <h2>{{.Analysis.Filename}}</h2>
  <div class="version">Version: {{.Analysis.Version}}</div>
  <div class="stats">
    <span>Paths: {{.Analysis.Stats.PathCount}}</span>
    <span>Operations: {{.Analysis.Stats.OperationCount}}</span>
  </div>
</div>
{{end}}
`)
	if err != nil {
		t.Fatalf("failed to create test partials: %v", err)
	}

	return &Handler{
		cfg: &config.Config{
			MaxFileSize: 2 << 20,
		},
		partials:        partials,
		instruments:     newInstruments(),
		version:         "test-version",
		oastoolsVersion: "test-oastools-version",
	}
}

func TestHandler_handleExploreUpload(t *testing.T) {
	tests := []struct {
		name         string
		specContent  string
		inputMode    string
		wantStatus   int
		wantContains string
		isHTMX       bool
	}{
		{
			name:         "valid OAS3 spec via paste (HTML)",
			specContent:  validOpenAPI30WithOperations,
			inputMode:    "paste",
			wantStatus:   http.StatusOK,
			wantContains: "explore-results",
			isHTMX:       true,
		},
		{
			name:         "valid OAS3 spec via paste (JSON)",
			specContent:  validOpenAPI30WithOperations,
			inputMode:    "paste",
			wantStatus:   http.StatusOK,
			wantContains: `"hash"`,
			isHTMX:       false,
		},
		{
			name:         "invalid spec",
			specContent:  "not: valid: yaml: content",
			inputMode:    "paste",
			wantStatus:   http.StatusBadRequest,
			wantContains: "PARSE_FAILED",
			isHTMX:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := uploadTestHandler(t)

			// Create form request
			form := "input_mode=" + tt.inputMode + "&spec_content=" + tt.specContent
			req := httptest.NewRequest(http.MethodPost, "/api/explore", strings.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tt.isHTMX {
				req.Header.Set("HX-Request", "true")
			}

			resp := h.handleExploreUpload(context.Background(), &builder.Request{HTTPRequest: req})

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			rec := httptest.NewRecorder()
			if err := resp.WriteTo(rec); err != nil {
				t.Fatalf("WriteTo failed: %v", err)
			}
			body := rec.Body.String()

			if !strings.Contains(body, tt.wantContains) {
				t.Errorf("body = %q, want contains %q", body, tt.wantContains)
			}
		})
	}
}

func TestHandler_handleExploreUpload_CacheHit(t *testing.T) {
	h := uploadTestHandler(t)

	// Upload spec first time
	form := "input_mode=paste&spec_content=" + validOpenAPI30WithOperations
	req := httptest.NewRequest(http.MethodPost, "/api/explore", strings.NewReader(form))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp1 := h.handleExploreUpload(context.Background(), &builder.Request{HTTPRequest: req})
	if resp1.StatusCode() != http.StatusOK {
		t.Fatalf("first upload failed with status %d", resp1.StatusCode())
	}

	// Upload same spec again - should hit cache
	req2 := httptest.NewRequest(http.MethodPost, "/api/explore", strings.NewReader(form))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp2 := h.handleExploreUpload(context.Background(), &builder.Request{HTTPRequest: req2})
	if resp2.StatusCode() != http.StatusOK {
		t.Errorf("cache hit upload failed with status %d", resp2.StatusCode())
	}
}

func TestRenderExploreResult(t *testing.T) {
	h := uploadTestHandler(t)

	analysis := &ExploreAnalysis{
		Hash:     "testhash",
		Version:  "3.0.0",
		Filename: "test.yaml",
		Stats: ExploreStats{
			PathCount:      5,
			OperationCount: 10,
		},
	}

	tests := []struct {
		name         string
		isHTMX       bool
		wantContains string
	}{
		{
			name:         "HTML response",
			isHTMX:       true,
			wantContains: "explore-results",
		},
		{
			name:         "JSON response",
			isHTMX:       false,
			wantContains: `"hash":"testhash"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/explore", nil)
			if tt.isHTMX {
				req.Header.Set("HX-Request", "true")
			}

			resp := h.renderExploreResult(req, analysis)

			rec := httptest.NewRecorder()
			if err := resp.WriteTo(rec); err != nil {
				t.Fatalf("WriteTo failed: %v", err)
			}
			body := rec.Body.String()

			if !strings.Contains(body, tt.wantContains) {
				t.Errorf("body = %q, want contains %q", body, tt.wantContains)
			}
		})
	}
}

func TestExtractOAuthFlows(t *testing.T) {
	tests := []struct {
		name      string
		flows     *parser.OAuthFlows
		wantCount int
		wantTypes []string
	}{
		{
			name:      "nil flows",
			flows:     nil,
			wantCount: 0,
		},
		{
			name: "implicit flow",
			flows: &parser.OAuthFlows{
				Implicit: &parser.OAuthFlow{
					AuthorizationURL: "https://example.com/auth",
					Scopes:           map[string]string{"read": "Read access"},
				},
			},
			wantCount: 1,
			wantTypes: []string{"implicit"},
		},
		{
			name: "all flows",
			flows: &parser.OAuthFlows{
				Implicit: &parser.OAuthFlow{
					AuthorizationURL: "https://example.com/auth",
				},
				Password: &parser.OAuthFlow{
					TokenURL: "https://example.com/token",
				},
				ClientCredentials: &parser.OAuthFlow{
					TokenURL: "https://example.com/token",
				},
				AuthorizationCode: &parser.OAuthFlow{
					AuthorizationURL: "https://example.com/auth",
					TokenURL:         "https://example.com/token",
				},
			},
			wantCount: 4,
			wantTypes: []string{"implicit", "password", "clientCredentials", "authorizationCode"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractOAuthFlows(tt.flows)

			if len(got) != tt.wantCount {
				t.Errorf("extractOAuthFlows() returned %d flows, want %d", len(got), tt.wantCount)
			}

			for _, wantType := range tt.wantTypes {
				found := false
				for _, flow := range got {
					if flow.Type == wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractOAuthFlows() did not return flow type %q", wantType)
				}
			}
		})
	}
}

func TestCountSecurityUsage(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithSecurity)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		t.Fatalf("failed to collect operations: %v", err)
	}

	counts := countSecurityUsage(parseResult, operations)

	// bearerAuth is used via global security by listPets
	if counts["bearerAuth"] != 1 {
		t.Errorf("bearerAuth count = %d, want 1", counts["bearerAuth"])
	}

	// apiKey is used explicitly by createPet
	if counts["apiKey"] != 1 {
		t.Errorf("apiKey count = %d, want 1", counts["apiKey"])
	}
}

func TestCountSecurityUsage_NilOperations(t *testing.T) {
	counts := countSecurityUsage(nil, nil)
	if len(counts) != 0 {
		t.Errorf("countSecurityUsage(nil, nil) = %v, want empty map", counts)
	}
}

func TestGetVersionString(t *testing.T) {
	tests := []struct {
		name   string
		result *parser.ParseResult
		want   string
	}{
		{
			name:   "nil result",
			result: nil,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getVersionString(tt.result)
			if got != tt.want {
				t.Errorf("getVersionString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetVersionString_WithParsedSpec(t *testing.T) {
	parseResult, err := parser.ParseWithOptions(parser.WithBytes([]byte(validOpenAPI30WithOperations)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}

	got := getVersionString(parseResult)
	if got != "3.0.0" {
		t.Errorf("getVersionString() = %q, want %q", got, "3.0.0")
	}
}

// Ensure cache is cleaned up between tests
func init() {
	// Use a short TTL for testing with no entry limit
	exploreCache = NewTTLCache[string, *ExploreAnalysis](2*time.Minute, 0)
}
