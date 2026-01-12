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

// Ensure cache is cleaned up between tests
func init() {
	// Use a short TTL for testing
	exploreCache = NewTTLCache[string, *ExploreAnalysis](2 * time.Minute)
}
