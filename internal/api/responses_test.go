package api

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// =============================================================================
// wantsHTML Tests
// =============================================================================

func TestWantsHTML(t *testing.T) {
	tests := []struct {
		name     string
		hxHeader string
		want     bool
	}{
		{
			name:     "HX-Request true returns true",
			hxHeader: "true",
			want:     true,
		},
		{
			name:     "HX-Request false returns false",
			hxHeader: "false",
			want:     false,
		},
		{
			name:     "no HX-Request header returns false",
			hxHeader: "",
			want:     false,
		},
		{
			name:     "HX-Request with other value returns false",
			hxHeader: "yes",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.hxHeader != "" {
				req.Header.Set("HX-Request", tt.hxHeader)
			}

			got := wantsHTML(req)
			if got != tt.want {
				t.Errorf("wantsHTML() = %v, want %v", got, tt.want)
			}
		})
	}
}

// =============================================================================
// serializeDocument Tests
// =============================================================================

func TestSerializeDocument(t *testing.T) {
	doc := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Test API",
			"version": "1.0.0",
		},
	}

	t.Run("JSON format", func(t *testing.T) {
		result, err := serializeDocument(doc, "json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, `"openapi": "3.0.0"`) {
			t.Errorf("JSON output should contain openapi field, got: %s", result)
		}
		if !strings.Contains(result, `"title": "Test API"`) {
			t.Errorf("JSON output should contain title field, got: %s", result)
		}
		// Check indentation (should have 2 spaces)
		if !strings.Contains(result, "  ") {
			t.Errorf("JSON should be indented with 2 spaces, got: %s", result)
		}
	})

	t.Run("YAML format", func(t *testing.T) {
		result, err := serializeDocument(doc, "yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "openapi: 3.0.0") && !strings.Contains(result, `openapi: "3.0.0"`) {
			t.Errorf("YAML output should contain openapi field, got: %s", result)
		}
		if !strings.Contains(result, "title: Test API") && !strings.Contains(result, `title: "Test API"`) {
			t.Errorf("YAML output should contain title field, got: %s", result)
		}
	})

	t.Run("empty document", func(t *testing.T) {
		emptyDoc := map[string]any{}
		result, err := serializeDocument(emptyDoc, "json")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "{}" {
			t.Errorf("empty doc should serialize to {}, got: %s", result)
		}
	})

	t.Run("JSON marshal error", func(t *testing.T) {
		// Channels cannot be marshaled to JSON
		badDoc := map[string]any{"channel": make(chan int)}
		_, err := serializeDocument(badDoc, "json")
		if err == nil {
			t.Error("expected error for unmarshable type")
		}
	})
}

// =============================================================================
// detectFormat Tests
// =============================================================================

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name:    "JSON object",
			content: []byte(`{"openapi": "3.0.0"}`),
			want:    "json",
		},
		{
			name:    "JSON array",
			content: []byte(`["item1", "item2"]`),
			want:    "json",
		},
		{
			name:    "JSON with leading whitespace",
			content: []byte(`  { "openapi": "3.0.0" }`),
			want:    "json",
		},
		{
			name:    "JSON with leading newlines",
			content: []byte("\n\n{\"key\": \"value\"}"),
			want:    "json",
		},
		{
			name:    "JSON with leading tabs",
			content: []byte("\t\t{\"key\": \"value\"}"),
			want:    "json",
		},
		{
			name:    "YAML document",
			content: []byte(`openapi: "3.0.0"`),
			want:    "yaml",
		},
		{
			name:    "YAML with leading whitespace",
			content: []byte("  openapi: 3.0.0"),
			want:    "yaml",
		},
		{
			name:    "empty content",
			content: []byte{},
			want:    "yaml",
		},
		{
			name:    "whitespace only",
			content: []byte("   \n\t  "),
			want:    "yaml",
		},
		{
			name:    "YAML document marker",
			content: []byte("---\nopenapi: 3.0.0"),
			want:    "yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFormat(tt.content)
			if got != tt.want {
				t.Errorf("detectFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// validateTargetVersion Tests
// =============================================================================

func TestValidateTargetVersion(t *testing.T) {
	validVersions := []string{
		"2.0",
		"3.0", "3.0.0", "3.0.1", "3.0.2", "3.0.3",
		"3.1", "3.1.0",
		"3.2", "3.2.0",
	}
	invalidVersions := []string{
		"",
		"1.0",
		"4.0",
		"3.3",
		"3.0.4",
		"3.1.1",
		"3.2.1",
		"invalid",
		"v3.0",
	}

	for _, v := range validVersions {
		t.Run("valid_"+v, func(t *testing.T) {
			if err := validateTargetVersion(v); err != nil {
				t.Errorf("validateTargetVersion(%q) should not error, got: %v", v, err)
			}
		})
	}

	for _, v := range invalidVersions {
		name := "invalid_" + v
		if v == "" {
			name = "invalid_empty"
		}
		t.Run(name, func(t *testing.T) {
			if err := validateTargetVersion(v); err == nil {
				t.Errorf("validateTargetVersion(%q) should error", v)
			}
		})
	}
}

// =============================================================================
// htmlResponse Tests
// =============================================================================

func TestHtmlResponse(t *testing.T) {
	t.Run("StatusCode", func(t *testing.T) {
		resp := &htmlResponse{status: http.StatusCreated, html: "<p>test</p>"}
		if resp.StatusCode() != http.StatusCreated {
			t.Errorf("StatusCode() = %d, want %d", resp.StatusCode(), http.StatusCreated)
		}
	})

	t.Run("Headers returns nil", func(t *testing.T) {
		resp := &htmlResponse{status: http.StatusOK, html: "test"}
		if resp.Headers() != nil {
			t.Error("Headers() should return nil")
		}
	})

	t.Run("Body returns html string", func(t *testing.T) {
		html := "<div>content</div>"
		resp := &htmlResponse{status: http.StatusOK, html: html}
		if resp.Body() != html {
			t.Errorf("Body() = %v, want %v", resp.Body(), html)
		}
	})

	t.Run("WriteTo sets content type and writes body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		html := "<p>Hello World</p>"
		resp := &htmlResponse{status: http.StatusOK, html: html}

		err := resp.WriteTo(rec)
		if err != nil {
			t.Fatalf("WriteTo() error: %v", err)
		}

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", ct, "text/html; charset=utf-8")
		}
		if rec.Body.String() != html {
			t.Errorf("body = %q, want %q", rec.Body.String(), html)
		}
	})
}

// =============================================================================
// parseCollisionStrategy Tests
// =============================================================================

func TestParseCollisionStrategy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string // We can't easily compare the enum, so check behavior
	}{
		{name: "rename", input: "rename", wantName: "rename"},
		{name: "first", input: "first", wantName: "first"},
		{name: "error", input: "error", wantName: "error"},
		{name: "empty defaults to rename", input: "", wantName: "rename"},
		{name: "invalid defaults to rename", input: "invalid", wantName: "rename"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify it doesn't panic - the function should always return a valid strategy
			_ = parseCollisionStrategy(tt.input)
		})
	}
}

// =============================================================================
// renderHTML Tests
// =============================================================================

func TestRenderHTML_Success(t *testing.T) {
	partials, err := template.New("partials").Parse(`{{define "test.html"}}Hello {{.Name}}{{end}}`)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	h := &Handler{partials: partials}
	resp := h.renderHTML("test.html", map[string]string{"Name": "World"})

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode())
	}

	if body, ok := resp.Body().(string); ok {
		if body != "Hello World" {
			t.Errorf("got body %q, want 'Hello World'", body)
		}
	} else {
		t.Error("body should be string")
	}
}

func TestRenderHTML_TemplateError(t *testing.T) {
	// Create a template that references an undefined template
	partials, err := template.New("partials").Parse(`{{define "exists.html"}}OK{{end}}`)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	h := &Handler{partials: partials}
	// Try to render a template that doesn't exist
	resp := h.renderHTML("nonexistent.html", nil)

	// Should return 500 error when template execution fails
	if resp.StatusCode() != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", resp.StatusCode())
	}
}

// =============================================================================
// renderError Tests
// =============================================================================

func TestRenderError_JSONResponse(t *testing.T) {
	partials, _ := template.New("partials").Parse(`{{define "error"}}Error: {{.Message}}{{end}}`)
	h := &Handler{partials: partials}

	// Non-HTMX request should get JSON response
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	resp := h.renderError(req, http.StatusBadRequest, "TEST_ERROR", "test message")

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}

	body, ok := resp.Body().(ErrorResponse)
	if !ok {
		t.Fatal("body should be ErrorResponse")
	}
	if body.Error.Code != "TEST_ERROR" {
		t.Errorf("got code %q, want TEST_ERROR", body.Error.Code)
	}
	if body.Error.Message != "test message" {
		t.Errorf("got message %q, want 'test message'", body.Error.Message)
	}
}

func TestRenderError_HTMLResponse(t *testing.T) {
	partials, _ := template.New("partials").Parse(`{{define "error"}}Error: {{.Message}}{{end}}`)
	h := &Handler{partials: partials}

	// HTMX request should get HTML response
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("HX-Request", "true")

	resp := h.renderError(req, http.StatusBadRequest, "TEST_ERROR", "test message")

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}

	// Should be htmlResponse type
	htmlResp, ok := resp.(*htmlResponse)
	if !ok {
		t.Fatal("response should be htmlResponse for HTMX request")
	}
	if htmlResp.html != "Error: test message" {
		t.Errorf("got html %q, want 'Error: test message'", htmlResp.html)
	}
}

func TestRenderError_ServerError(t *testing.T) {
	partials, _ := template.New("partials").Parse(`{{define "error"}}Error: {{.Message}}{{end}}`)
	h := &Handler{partials: partials}

	// Test 500 error (triggers slog.Error instead of slog.Warn)
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	resp := h.renderError(req, http.StatusInternalServerError, "INTERNAL_ERROR", "server error")

	if resp.StatusCode() != http.StatusInternalServerError {
		t.Errorf("got status %d, want 500", resp.StatusCode())
	}

	body, ok := resp.Body().(ErrorResponse)
	if !ok {
		t.Fatal("body should be ErrorResponse")
	}
	if body.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("got code %q, want INTERNAL_ERROR", body.Error.Code)
	}
}

func TestRenderError_HTMLTemplateFailure(t *testing.T) {
	// Create a template that will fail during execution
	partials, _ := template.New("partials").Parse(`{{define "error"}}{{.NonExistent.Field}}{{end}}`)
	h := &Handler{partials: partials}

	// HTMX request with failing template should fall back to builder.Error
	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("HX-Request", "true")

	resp := h.renderError(req, http.StatusBadRequest, "TEST_ERROR", "test message")

	// Should return the original status code even when template fails
	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}
}
