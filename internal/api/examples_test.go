package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erraggy/oastools-web/static/examples"
	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
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
		{
			name:       "invalid characters - path traversal",
			example:    "../etc/passwd",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid characters - semicolon",
			example:    "foo;bar",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty name",
			example:    "",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/examples/"+tt.example, nil)

			builderReq := &builder.Request{
				HTTPRequest: req,
				PathParams:  map[string]any{"name": tt.example},
			}

			resp := h.handleGetExample(context.Background(), builderReq)

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// For successful responses, verify the content type by writing the response
			if tt.wantType != "" && tt.wantStatus == http.StatusOK {
				w := httptest.NewRecorder()
				if err := resp.WriteTo(w); err != nil {
					t.Fatalf("WriteTo failed: %v", err)
				}
				if got := w.Header().Get("Content-Type"); got != tt.wantType {
					t.Errorf("Content-Type = %q, want %q", got, tt.wantType)
				}
			}
		})
	}
}

func TestHandleGetExample_ValidContent(t *testing.T) {
	h := minimalHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/examples/petstore-3.0", nil)
	builderReq := &builder.Request{
		HTTPRequest: req,
		PathParams:  map[string]any{"name": "petstore-3.0"},
	}

	resp := h.handleGetExample(context.Background(), builderReq)

	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusOK)
	}

	// Verify response body contains OpenAPI content
	body := resp.Body()
	content, ok := body.([]byte)
	if !ok {
		t.Fatal("expected []byte response body")
	}
	if !strings.Contains(string(content), "openapi:") {
		t.Error("response body should contain 'openapi:'")
	}
}

func TestHandleListExamples(t *testing.T) {
	h := minimalHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/examples", nil)
	builderReq := &builder.Request{HTTPRequest: req}

	resp := h.handleListExamples(context.Background(), builderReq)

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode(), http.StatusOK)
	}

	// Verify response contains expected examples
	body := resp.Body()
	list, ok := body.([]ExampleMetadata)
	if !ok {
		t.Fatalf("expected []ExampleMetadata, got %T", body)
	}

	// Check we have at least some examples
	if len(list) == 0 {
		t.Error("expected at least one example")
	}

	// Check for expected examples
	expectedExamples := []string{"petstore-3.0", "petstore-2.0", "petstore-warnings"}
	for _, ex := range expectedExamples {
		found := false
		for _, item := range list {
			if item.Name == ex {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("response missing example %q", ex)
		}
	}
}

func TestExampleLabel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "known example",
			input: "petstore-3.0",
			want:  "Petstore (Clean)",
		},
		{
			name:  "unknown example returns name",
			input: "unknown-spec",
			want:  "unknown-spec",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exampleLabel(tt.input)
			if got != tt.want {
				t.Errorf("exampleLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHandleListExamples_ContainsExpected(t *testing.T) {
	h := minimalHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/examples", nil)
	builderReq := &builder.Request{HTTPRequest: req}

	resp := h.handleListExamples(context.Background(), builderReq)

	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode(), http.StatusOK)
	}

	// Verify response contains all expected examples
	body := resp.Body()
	list, ok := body.([]ExampleMetadata)
	if !ok {
		t.Fatalf("expected []ExampleMetadata, got %T", body)
	}

	expectedExamples := []string{
		"petstore-3.0", "petstore-2.0", "petstore-warnings", "petstore-errors",
		"petstore-v2", "petstore-v3", "petstore-messy", "petstore-full",
		"users-api", "products-api", "orders-api", "inventory-api",
		"overlay-descriptions", "overlay-security", "overlay-public",
	}
	for _, ex := range expectedExamples {
		found := false
		for _, item := range list {
			if item.Name == ex {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("response missing example %q", ex)
		}
	}
}

func TestExampleSpecsAreValid(t *testing.T) {
	entries, err := examples.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read examples: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		// Skip overlay files - they're not full specs
		if strings.HasPrefix(entry.Name(), "overlay-") {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			content, err := examples.FS.ReadFile(entry.Name())
			if err != nil {
				t.Fatalf("failed to read %s: %v", entry.Name(), err)
			}

			_, err = parser.ParseWithOptions(parser.WithBytes(content))
			// Allow errors spec to fail parsing (it has intentional errors)
			if err != nil && !strings.Contains(entry.Name(), "errors") {
				t.Errorf("failed to parse %s: %v", entry.Name(), err)
			}
		})
	}
}
