package api

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/erraggy/oastools-web/internal/config"
	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/validator"
)

// =============================================================================
// Test Fixtures
// =============================================================================

// validOpenAPI30 is a minimal valid OpenAPI 3.0 spec for testing.
const validOpenAPI30 = `openapi: "3.0.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      summary: Health check
      responses:
        "200":
          description: OK
`

// validOpenAPI31 is a minimal valid OpenAPI 3.1 spec for testing.
const validOpenAPI31 = `openapi: "3.1.0"
info:
  title: Test API
  version: "1.0.0"
paths:
  /health:
    get:
      summary: Health check
      responses:
        "200":
          description: OK
`

// validSwagger20 is a minimal valid Swagger 2.0 spec for testing.
const validSwagger20 = `swagger: "2.0"
info:
  title: Test API
  version: "1.0.0"
host: api.example.com
basePath: /v1
paths:
  /health:
    get:
      summary: Health check
      responses:
        200:
          description: OK
`

// invalidYAML is content that cannot be parsed as YAML.
const invalidYAML = `{{{{not valid yaml`

// validOverlay is a minimal valid overlay document.
const validOverlay = `overlay: "1.0.0"
info:
  title: Test Overlay
  version: "1.0.0"
actions:
  - target: $.info.title
    update: Updated API Title
`

// =============================================================================
// Test Helpers
// =============================================================================

// minimalHandler creates a minimal Handler for testing with required fields.
func minimalHandler(t *testing.T) *Handler {
	t.Helper()

	// Parse minimal templates for testing
	partials, err := template.New("partials").Parse(`
{{define "error"}}Error: {{.Message}}{{end}}
{{define "validation-result.html"}}Valid: {{.Valid}}{{end}}
{{define "convert-result.html"}}Converted{{end}}
{{define "diff-result.html"}}Diff{{end}}
{{define "fix-result.html"}}Fixed{{end}}
{{define "join-result.html"}}Joined{{end}}
{{define "overlay-result.html"}}Overlay Applied{{end}}
`)
	if err != nil {
		t.Fatalf("failed to create test partials: %v", err)
	}

	return &Handler{
		cfg: &config.Config{
			MaxFileSize: 2 << 20, // 2MB
		},
		partials:        partials,
		instruments:     newInstruments(),
		version:         "test-version",
		oastoolsVersion: "test-oastools-version",
	}
}

// createHandlerRequest creates a builder.Request with the given HTTP request.
func createHandlerRequest(r *http.Request) *builder.Request {
	return &builder.Request{HTTPRequest: r}
}

// createMultipartRequestWithFields creates a multipart form request with files and form fields.
func createMultipartRequestWithFields(t *testing.T, path string, files map[string][]byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add files
	for fieldName, content := range files {
		part, err := writer.CreateFormFile(fieldName, fieldName+".yaml")
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("failed to write content: %v", err)
		}
	}

	// Add form fields
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("failed to write field: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// createMultiFileRequest creates a multipart request with multiple files for the same field.
func createMultiFileRequest(t *testing.T, path string, fieldName string, files [][]byte, fields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for i, content := range files {
		part, err := writer.CreateFormFile(fieldName, fmt.Sprintf("spec%d.yaml", i))
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatalf("failed to write content: %v", err)
		}
	}

	// Add form fields
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("failed to write field: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// =============================================================================
// handleHealth Tests
// =============================================================================

func TestHandler_handleHealth(t *testing.T) {
	h := &Handler{
		version: "1.0.0",
		// Note: oastoolsVersion uses getOASToolsVersion() which reads from runtime/debug
		// In tests, this returns "unknown" since there's no build info
	}

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp := h.handleHealth(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode())
	}

	body, ok := resp.Body().(HealthResponse)
	if !ok {
		t.Fatal("body should be HealthResponse")
	}
	if body.Status != "healthy" {
		t.Errorf("got status %q, want \"healthy\"", body.Status)
	}
	if body.Version != "1.0.0" {
		t.Errorf("got version %q, want \"1.0.0\"", body.Version)
	}
	// OASTools version comes from getOASToolsVersion() which uses runtime/debug.ReadBuildInfo()
	// In test context, this returns "unknown" since there's no module build info
	if body.OASTools == "" {
		t.Error("expected non-empty oastools version")
	}
}

// =============================================================================
// handleValidate Tests
// =============================================================================

func TestHandler_handleValidate(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name       string
		spec       []byte
		strict     string
		wantsHTML  bool
		wantStatus int
		wantValid  bool
	}{
		{
			name:       "valid OpenAPI 3.0 spec",
			spec:       []byte(validOpenAPI30),
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
		{
			name:       "valid OpenAPI 3.1 spec",
			spec:       []byte(validOpenAPI31),
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
		{
			name:       "valid Swagger 2.0 spec",
			spec:       []byte(validSwagger20),
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
		{
			name:       "invalid YAML returns 400",
			spec:       []byte(invalidYAML),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid spec with strict mode",
			spec:       []byte(validOpenAPI30),
			strict:     "on",
			wantStatus: http.StatusOK,
			wantValid:  true,
		},
		{
			name:       "HTMX request returns HTML",
			spec:       []byte(validOpenAPI30),
			wantsHTML:  true,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]string{}
			if tt.strict != "" {
				fields["strict"] = tt.strict
			}

			req := createMultipartRequestWithFields(t, "/api/validate",
				map[string][]byte{"spec": tt.spec}, fields)

			if tt.wantsHTML {
				req.Header.Set("HX-Request", "true")
			}

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleValidate(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			if tt.wantStatus == http.StatusOK && !tt.wantsHTML {
				body, ok := resp.Body().(ValidateResponse)
				if !ok {
					t.Fatal("body should be ValidateResponse")
				}
				if body.Valid != tt.wantValid {
					t.Errorf("got valid=%v, want %v", body.Valid, tt.wantValid)
				}
			}
		})
	}
}

func TestHandler_handleValidate_MissingSpec(t *testing.T) {
	h := minimalHandler(t)

	// Create request without spec file
	req := httptest.NewRequest(http.MethodPost, "/api/validate", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}
}

// =============================================================================
// URL Input Mode Tests
// =============================================================================

func TestHandler_URLInputMode_Success(t *testing.T) {
	// Create a test server that serves a valid OpenAPI spec
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(validOpenAPI30))
	}))
	defer server.Close()

	h := minimalHandler(t)
	h.urlFetcher = newTestURLFetcher() // Uses skipHostCheck for localhost tests

	// Create a request with URL input mode
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("input_mode", "url")
	_ = writer.WriteField("spec_url", server.URL+"/openapi.yaml")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("got status %d, want 200", resp.StatusCode())
	}

	body, ok := resp.Body().(ValidateResponse)
	if !ok {
		t.Fatal("body should be ValidateResponse")
	}
	if !body.Valid {
		t.Error("expected valid=true")
	}
}

func TestHandler_URLInputMode_MissingURL(t *testing.T) {
	h := minimalHandler(t)
	h.urlFetcher = newTestURLFetcher()

	// Create a request with URL input mode but no URL provided
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("input_mode", "url")
	// Note: spec_url is NOT provided
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}
}

func TestHandler_URLInputMode_FetchError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	h := minimalHandler(t)
	h.urlFetcher = newTestURLFetcher()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("input_mode", "url")
	_ = writer.WriteField("spec_url", server.URL+"/notfound.yaml")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}
}

func TestHandler_URLInputMode_SizeLimit(t *testing.T) {
	// Create content that is under URLFetcher's 2MB limit but larger than handler's limit
	// Use 100KB of content
	largeContent := "openapi: '3.0.0'\ninfo:\n  title: Test\n  version: '1.0'\npaths: {}\n" + strings.Repeat("x", 100<<10) // 100KB

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeContent))
	}))
	defer server.Close()

	h := minimalHandler(t)
	h.urlFetcher = newTestURLFetcher()
	// Set a small max file size to trigger the handler's size check
	h.cfg.MaxFileSize = 50 << 10 // 50KB - smaller than our 100KB content

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("input_mode", "url")
	_ = writer.WriteField("spec_url", server.URL+"/large.yaml")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	// Should fail because content exceeds handler's max file size (50KB)
	if resp.StatusCode() != http.StatusRequestEntityTooLarge {
		t.Errorf("got status %d, want 413", resp.StatusCode())
	}
}

func TestHandler_URLInputMode_InvalidMode(t *testing.T) {
	h := minimalHandler(t)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("input_mode", "invalid_mode")
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/validate", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	if err := req.ParseMultipartForm(32 << 20); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	resp := h.handleValidate(context.Background(), createHandlerRequest(req))

	if resp.StatusCode() != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", resp.StatusCode())
	}
}

// =============================================================================
// buildValidateResponse Tests
// =============================================================================

func TestHandler_buildValidateResponse_WithWarnings(t *testing.T) {
	h := minimalHandler(t)

	// Create a mock validation result with both errors and warnings
	result := &validator.ValidationResult{
		Valid:   false,
		Version: "3.0.0",
		Errors: []validator.ValidationError{
			{Path: "/paths/test", Message: "test error"},
		},
		Warnings: []validator.ValidationError{
			{Path: "/info", Message: "test warning"},
		},
		ErrorCount:   1,
		WarningCount: 1,
		Stats: parser.DocumentStats{
			PathCount:      5,
			OperationCount: 10,
			SchemaCount:    3,
		},
	}

	resp := h.buildValidateResponse(result)

	if resp.Valid {
		t.Error("expected Valid=false")
	}
	if len(resp.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(resp.Errors))
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
	if resp.Warnings[0].Severity != "warning" {
		t.Errorf("expected severity 'warning', got %q", resp.Warnings[0].Severity)
	}
	if resp.Statistics.Paths != 5 {
		t.Errorf("expected 5 paths, got %d", resp.Statistics.Paths)
	}
}

// =============================================================================
// handleConvert Tests
// =============================================================================

func TestHandler_handleConvert(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name       string
		spec       []byte
		target     string
		wantStatus int
	}{
		{
			name:       "convert 3.0 to 3.1",
			spec:       []byte(validOpenAPI30),
			target:     "3.1",
			wantStatus: http.StatusOK,
		},
		{
			name:       "convert 3.0 to 3.2",
			spec:       []byte(validOpenAPI30),
			target:     "3.2",
			wantStatus: http.StatusOK,
		},
		{
			name:       "convert 2.0 to 3.0",
			spec:       []byte(validSwagger20),
			target:     "3.0",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid target version",
			spec:       []byte(validOpenAPI30),
			target:     "4.0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing target version",
			spec:       []byte(validOpenAPI30),
			target:     "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid YAML spec",
			spec:       []byte(invalidYAML),
			target:     "3.1",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]string{}
			if tt.target != "" {
				fields["target"] = tt.target
			}

			req := createMultipartRequestWithFields(t, "/api/convert",
				map[string][]byte{"spec": tt.spec}, fields)

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleConvert(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
		})
	}
}

func TestHandler_handleConvert_WithOverlays(t *testing.T) {
	h := minimalHandler(t)

	t.Run("convert with valid pre-overlay", func(t *testing.T) {
		req := createMultipartRequestWithFields(t, "/api/convert",
			map[string][]byte{
				"spec":       []byte(validOpenAPI30),
				"preOverlay": []byte(validOverlay),
			},
			map[string]string{"target": "3.1"})

		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleConvert(context.Background(), createHandlerRequest(req))

		if resp.StatusCode() != http.StatusOK {
			t.Errorf("got status %d, want 200", resp.StatusCode())
		}
	})

	t.Run("convert with valid post-overlay", func(t *testing.T) {
		req := createMultipartRequestWithFields(t, "/api/convert",
			map[string][]byte{
				"spec":        []byte(validOpenAPI30),
				"postOverlay": []byte(validOverlay),
			},
			map[string]string{"target": "3.1"})

		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleConvert(context.Background(), createHandlerRequest(req))

		if resp.StatusCode() != http.StatusOK {
			t.Errorf("got status %d, want 200", resp.StatusCode())
		}
	})

	t.Run("convert with invalid pre-overlay", func(t *testing.T) {
		req := createMultipartRequestWithFields(t, "/api/convert",
			map[string][]byte{
				"spec":       []byte(validOpenAPI30),
				"preOverlay": []byte(invalidYAML),
			},
			map[string]string{"target": "3.1"})

		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleConvert(context.Background(), createHandlerRequest(req))

		if resp.StatusCode() != http.StatusBadRequest {
			t.Errorf("got status %d, want 400", resp.StatusCode())
		}
	})

	t.Run("convert with invalid post-overlay", func(t *testing.T) {
		req := createMultipartRequestWithFields(t, "/api/convert",
			map[string][]byte{
				"spec":        []byte(validOpenAPI30),
				"postOverlay": []byte(invalidYAML),
			},
			map[string]string{"target": "3.1"})

		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleConvert(context.Background(), createHandlerRequest(req))

		if resp.StatusCode() != http.StatusBadRequest {
			t.Errorf("got status %d, want 400", resp.StatusCode())
		}
	})

	t.Run("convert with HTMX request returns HTML", func(t *testing.T) {
		req := createMultipartRequestWithFields(t, "/api/convert",
			map[string][]byte{"spec": []byte(validOpenAPI30)},
			map[string]string{"target": "3.1"})
		req.Header.Set("HX-Request", "true")

		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleConvert(context.Background(), createHandlerRequest(req))

		if resp.StatusCode() != http.StatusOK {
			t.Errorf("got status %d, want 200", resp.StatusCode())
		}
		// HTML response type check
		if _, ok := resp.Body().(string); !ok {
			t.Error("expected HTML string response for HTMX request")
		}
	})
}

// =============================================================================
// handleDiff Tests
// =============================================================================

func TestHandler_handleDiff(t *testing.T) {
	h := minimalHandler(t)

	// Create a modified version of the spec
	modifiedSpec := strings.Replace(validOpenAPI30, "Test API", "Modified API", 1)

	tests := []struct {
		name       string
		base       []byte
		head       []byte
		mode       string
		wantStatus int
	}{
		{
			name:       "diff identical specs",
			base:       []byte(validOpenAPI30),
			head:       []byte(validOpenAPI30),
			wantStatus: http.StatusOK,
		},
		{
			name:       "diff with changes",
			base:       []byte(validOpenAPI30),
			head:       []byte(modifiedSpec),
			wantStatus: http.StatusOK,
		},
		{
			name:       "diff with breaking mode",
			base:       []byte(validOpenAPI30),
			head:       []byte(modifiedSpec),
			mode:       "breaking",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid base spec",
			base:       []byte(invalidYAML),
			head:       []byte(validOpenAPI30),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid head spec",
			base:       []byte(validOpenAPI30),
			head:       []byte(invalidYAML),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{
				"base": tt.base,
				"head": tt.head,
			}
			fields := map[string]string{}
			if tt.mode != "" {
				fields["mode"] = tt.mode
			}

			req := createMultipartRequestWithFields(t, "/api/diff", files, fields)

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleDiff(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
		})
	}
}

// =============================================================================
// handleFix Tests
// =============================================================================

func TestHandler_handleFix(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name       string
		spec       []byte
		dryRun     string
		wantStatus int
	}{
		{
			name:       "fix valid spec",
			spec:       []byte(validOpenAPI30),
			wantStatus: http.StatusOK,
		},
		{
			name:       "fix with dry run",
			spec:       []byte(validOpenAPI30),
			dryRun:     "on",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid spec",
			spec:       []byte(invalidYAML),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]string{}
			if tt.dryRun != "" {
				fields["dryRun"] = tt.dryRun
			}

			req := createMultipartRequestWithFields(t, "/api/fix",
				map[string][]byte{"spec": tt.spec}, fields)

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleFix(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
		})
	}
}

// =============================================================================
// handleJoin Tests
// =============================================================================

func TestHandler_handleJoin(t *testing.T) {
	h := minimalHandler(t)

	// Create a second spec for joining
	spec2 := `openapi: "3.0.0"
info:
  title: Second API
  version: "1.0.0"
paths:
  /items:
    get:
      summary: List items
      responses:
        "200":
          description: OK
`

	tests := []struct {
		name       string
		specs      [][]byte
		strategy   string
		wantStatus int
	}{
		{
			name:       "join 2 specs",
			specs:      [][]byte{[]byte(validOpenAPI30), []byte(spec2)},
			strategy:   "rename",
			wantStatus: http.StatusOK,
		},
		{
			name:       "join with first strategy",
			specs:      [][]byte{[]byte(validOpenAPI30), []byte(spec2)},
			strategy:   "first",
			wantStatus: http.StatusOK,
		},
		{
			name:       "too few specs",
			specs:      [][]byte{[]byte(validOpenAPI30)},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "too many specs",
			specs: [][]byte{
				[]byte(validOpenAPI30),
				[]byte(spec2),
				[]byte(validOpenAPI30),
				[]byte(spec2),
				[]byte(validOpenAPI30),
				[]byte(spec2),
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid spec in join",
			specs:      [][]byte{[]byte(validOpenAPI30), []byte(invalidYAML)},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]string{}
			if tt.strategy != "" {
				fields["strategy"] = tt.strategy
			}

			req := createMultiFileRequest(t, "/api/join", "specs", tt.specs, fields)

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleJoin(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
		})
	}
}

// =============================================================================
// handleOverlay Tests
// =============================================================================

func TestHandler_handleOverlay(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name       string
		spec       []byte
		overlay    []byte
		wantStatus int
	}{
		{
			name:       "apply overlay",
			spec:       []byte(validOpenAPI30),
			overlay:    []byte(validOverlay),
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid spec",
			spec:       []byte(invalidYAML),
			overlay:    []byte(validOverlay),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid overlay",
			spec:       []byte(validOpenAPI30),
			overlay:    []byte(invalidYAML),
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string][]byte{
				"spec":    tt.spec,
				"overlay": tt.overlay,
			}

			req := createMultipartRequestWithFields(t, "/api/overlay", files, nil)

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleOverlay(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
		})
	}
}

// =============================================================================
// handleSpec Tests
// =============================================================================

func TestHandler_handleSpec(t *testing.T) {
	// Create a minimal spec for testing
	spec := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":   "Test API",
			"version": "1.0.0",
		},
		"paths": map[string]any{},
	}

	h := &Handler{
		server: &builder.ServerResult{
			Spec: spec,
		},
	}

	tests := []struct {
		name        string
		accept      string
		wantStatus  int
		wantContent string // substring to check in response
	}{
		{
			name:        "default returns YAML",
			accept:      "",
			wantStatus:  http.StatusOK,
			wantContent: "openapi:",
		},
		{
			name:        "Accept text/html returns YAML",
			accept:      "text/html",
			wantStatus:  http.StatusOK,
			wantContent: "openapi:",
		},
		{
			name:        "Accept application/json returns JSON",
			accept:      "application/json",
			wantStatus:  http.StatusOK,
			wantContent: `"openapi"`,
		},
		{
			name:        "Accept with multiple types including JSON",
			accept:      "text/html, application/json",
			wantStatus:  http.StatusOK,
			wantContent: `"openapi"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/spec", nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}

			resp := h.handleSpec(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode(), tt.wantStatus)
			}

			// Check the response body contains expected content
			body := resp.Body()
			var bodyStr string
			switch v := body.(type) {
			case []byte:
				bodyStr = string(v)
			case string:
				bodyStr = v
			default:
				// For JSON responses, the body might be the spec map directly
				// In this case, we just verify the status code is correct
				return
			}

			if !strings.Contains(bodyStr, tt.wantContent) {
				t.Errorf("response body %q does not contain %q", bodyStr, tt.wantContent)
			}
		})
	}
}

// =============================================================================
// Content Negotiation Tests
// =============================================================================

func TestHandler_ContentNegotiation(t *testing.T) {
	h := minimalHandler(t)

	tests := []struct {
		name      string
		hxRequest string
		wantType  string
	}{
		{
			name:      "HTMX request gets HTML",
			hxRequest: "true",
			wantType:  "html",
		},
		{
			name:      "regular request gets JSON",
			hxRequest: "",
			wantType:  "json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createMultipartRequestWithFields(t, "/api/validate",
				map[string][]byte{"spec": []byte(validOpenAPI30)}, nil)

			if tt.hxRequest != "" {
				req.Header.Set("HX-Request", tt.hxRequest)
			}

			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			resp := h.handleValidate(context.Background(), createHandlerRequest(req))

			if resp.StatusCode() != http.StatusOK {
				t.Errorf("got status %d, want 200", resp.StatusCode())
			}

			// Check response type
			if tt.wantType == "html" {
				if _, ok := resp.Body().(string); !ok {
					if _, ok := resp.(*htmlResponse); !ok {
						t.Errorf("expected HTML response for HTMX request")
					}
				}
			} else {
				if _, ok := resp.Body().(ValidateResponse); !ok {
					t.Errorf("expected JSON response for regular request")
				}
			}
		})
	}
}

// TestHandler_handleJoinNewOptions covers the joiner options added for oastools
// v1.66.0 at the HTTP layer, where the unit tests for the helper functions do
// not reach: each new field's rejection path, and a report that has to survive
// the whole handler rather than being handed a pre-built joiner.Consolidation.
func TestHandler_handleJoinNewOptions(t *testing.T) {
	h := minimalHandler(t)

	// Two documents that declare the same schema with the same shape, so a
	// deduplicating strategy has something real to consolidate.
	specA := `openapi: "3.0.3"
info:
  title: A
  version: "1.0.0"
paths:
  /a:
    get:
      operationId: getA
      responses:
        "200":
          description: OK
components:
  schemas:
    Common:
      type: object
      properties:
        id:
          type: string
`
	specB := `openapi: "3.0.3"
info:
  title: B
  version: "1.0.0"
paths:
  /b:
    get:
      operationId: getB
      responses:
        "200":
          description: OK
components:
  schemas:
    Other:
      type: object
      properties:
        id:
          type: string
`

	t.Run("rejects unknown values", func(t *testing.T) {
		tests := []struct {
			name  string
			field string
			value string
		}{
			{name: "equivalence mode", field: "equivalenceMode", value: "sideways"},
			{name: "equivalence docs", field: "equivalenceDocs", value: "maybe"},
			{name: "deduplication scope", field: "dedupScope", value: "everything"},
			{name: "deduplication mode", field: "dedupMode", value: "teleport"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := createMultiFileRequest(t, "/api/join", "specs",
					[][]byte{[]byte(specA), []byte(specB)},
					map[string]string{tt.field: tt.value})
				if err := req.ParseMultipartForm(32 << 20); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}

				resp := h.handleJoin(context.Background(), createHandlerRequest(req))

				if resp.StatusCode() != http.StatusBadRequest {
					t.Errorf("%s=%q got status %d, want %d",
						tt.field, tt.value, resp.StatusCode(), http.StatusBadRequest)
				}
			})
		}
	})

	t.Run("accepts every valid value", func(t *testing.T) {
		tests := []struct {
			name   string
			fields map[string]string
		}{
			{name: "equivalence docs include", fields: map[string]string{"equivalenceDocs": "include"}},
			{name: "equivalence docs ignore", fields: map[string]string{"equivalenceDocs": "ignore"}},
			{name: "scope all", fields: map[string]string{"dedupScope": "all"}},
			{name: "scope generated-only", fields: map[string]string{"dedupScope": "generated-only"}},
			{name: "mode remove", fields: map[string]string{"dedupMode": "remove"}},
			{name: "mode pointer", fields: map[string]string{"dedupMode": "pointer"}},
			{name: "deduplicate or rename strategy", fields: map[string]string{"schemaStrategy": "dedupOrRename"}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := createMultiFileRequest(t, "/api/join", "specs",
					[][]byte{[]byte(specA), []byte(specB)}, tt.fields)
				if err := req.ParseMultipartForm(32 << 20); err != nil {
					t.Fatalf("failed to parse form: %v", err)
				}

				resp := h.handleJoin(context.Background(), createHandlerRequest(req))

				if resp.StatusCode() != http.StatusOK {
					t.Errorf("got status %d, want %d", resp.StatusCode(), http.StatusOK)
				}
			})
		}
	})

	t.Run("reports consolidations end to end", func(t *testing.T) {
		// specB is rewritten to declare Common too, so the two collide and
		// semantic deduplication has a group to fold and report.
		collidingB := strings.Replace(specB, "Other:", "Common:", 1)

		req := createMultiFileRequest(t, "/api/join", "specs",
			[][]byte{[]byte(specA), []byte(collidingB)},
			map[string]string{
				"schemaStrategy":  "rename",
				"semanticDedup":   "on",
				"equivalenceMode": "deep",
				"dedupReport":     "on",
			})
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleJoin(context.Background(), createHandlerRequest(req))
		if resp.StatusCode() != http.StatusOK {
			t.Fatalf("got status %d, want %d", resp.StatusCode(), http.StatusOK)
		}

		result, ok := resp.Body().(JoinResponse)
		if !ok {
			t.Fatalf("body is %T, want JoinResponse", resp.Body())
		}
		if len(result.Consolidations) == 0 {
			t.Fatal("expected the deduplication report to reach the response, got none")
		}
		if result.Consolidations[0].Survivor == "" {
			t.Error("consolidation has no surviving name")
		}
		if len(result.Consolidations[0].Folded) == 0 {
			t.Error("consolidation folded nothing, which the library never reports")
		}
	})

	t.Run("omits the report when it was not requested", func(t *testing.T) {
		collidingB := strings.Replace(specB, "Other:", "Common:", 1)

		req := createMultiFileRequest(t, "/api/join", "specs",
			[][]byte{[]byte(specA), []byte(collidingB)},
			map[string]string{
				"schemaStrategy":  "rename",
				"semanticDedup":   "on",
				"equivalenceMode": "deep",
			})
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		resp := h.handleJoin(context.Background(), createHandlerRequest(req))
		result, ok := resp.Body().(JoinResponse)
		if !ok {
			t.Fatalf("body is %T, want JoinResponse", resp.Body())
		}
		if len(result.Consolidations) != 0 {
			t.Errorf("Consolidations = %v, want none without dedupReport", result.Consolidations)
		}
	})
}
