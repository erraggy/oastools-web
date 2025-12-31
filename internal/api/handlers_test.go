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
