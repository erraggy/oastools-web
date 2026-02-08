package api

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/erraggy/oastools-web/internal/config"
)

// createMultipartRequest creates a multipart form request with a file field.
func createMultipartRequest(t *testing.T, fieldName, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/test", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// createFormRequest creates a form request with field values.
func createFormRequest(t *testing.T, values map[string]string) *http.Request {
	t.Helper()
	form := make([]string, 0, len(values))
	for k, v := range values {
		form = append(form, k+"="+v)
	}
	body := strings.Join(form, "&")
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestHandler_readFileInputWithLimit(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{MaxFileSize: 1024}, // 1KB limit
	}

	tests := []struct {
		name        string
		content     []byte
		maxSize     int64
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid file",
			content:     []byte("openapi: 3.0.0"),
			maxSize:     1024,
			expectError: false,
		},
		{
			name:        "empty file",
			content:     []byte{},
			maxSize:     1024,
			expectError: true,
			errorCode:   "EMPTY_FILE",
		},
		{
			name:        "file exceeds limit",
			content:     bytes.Repeat([]byte("x"), 2000),
			maxSize:     1024,
			expectError: true,
			errorCode:   "FILE_TOO_LARGE",
		},
		{
			name:        "file at exact limit",
			content:     bytes.Repeat([]byte("x"), 1024),
			maxSize:     1024,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createMultipartRequest(t, "spec", "test.yaml", tt.content)

			// Parse multipart form
			if err := req.ParseMultipartForm(32 << 20); err != nil {
				t.Fatalf("failed to parse multipart form: %v", err)
			}

			input, errResp := h.readFileInputWithLimit(req, "spec", tt.maxSize)

			if tt.expectError {
				if errResp == nil {
					t.Error("expected error response, got nil")
				}
			} else {
				if errResp != nil {
					t.Errorf("unexpected error response: %v", errResp)
				}
				if input == nil {
					t.Error("expected input, got nil")
				} else {
					if !bytes.Equal(input.Content, tt.content) {
						t.Error("content mismatch")
					}
					if input.Mode != "file" {
						t.Errorf("expected mode 'file', got %q", input.Mode)
					}
				}
			}
		})
	}
}

func TestHandler_readPasteInputWithLimit(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{MaxFileSize: 1024},
	}

	tests := []struct {
		name        string
		content     string
		maxSize     int64
		expectError bool
		errorCode   string
	}{
		{
			name:        "valid paste",
			content:     "openapi: 3.0.0",
			maxSize:     1024,
			expectError: false,
		},
		{
			name:        "empty paste",
			content:     "",
			maxSize:     1024,
			expectError: true,
			errorCode:   "MISSING_CONTENT",
		},
		{
			name:        "paste exceeds limit",
			content:     strings.Repeat("x", 2000),
			maxSize:     1024,
			expectError: true,
			errorCode:   "CONTENT_TOO_LARGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createFormRequest(t, map[string]string{
				"spec_content": tt.content,
			})

			// Parse form
			if err := req.ParseForm(); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			input, errResp := h.readPasteInputWithLimit(req, "spec", tt.maxSize)

			if tt.expectError {
				if errResp == nil {
					t.Error("expected error response, got nil")
				}
			} else {
				if errResp != nil {
					t.Errorf("unexpected error response: %v", errResp)
				}
				if input == nil {
					t.Error("expected input, got nil")
				} else {
					if string(input.Content) != tt.content {
						t.Error("content mismatch")
					}
					if input.Mode != "paste" {
						t.Errorf("expected mode 'paste', got %q", input.Mode)
					}
					if input.Filename != "pasted-spec" {
						t.Errorf("expected filename 'pasted-spec', got %q", input.Filename)
					}
				}
			}
		})
	}
}

func TestHandler_readInputWithLimit_ModeDetection(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{MaxFileSize: 1024},
	}

	tests := []struct {
		name         string
		fieldMode    string // e.g., "spec_mode"
		inputMode    string // generic "input_mode"
		expectedMode string
	}{
		{
			name:         "field-specific mode takes precedence",
			fieldMode:    "paste",
			inputMode:    "file",
			expectedMode: "paste",
		},
		{
			name:         "fallback to input_mode",
			fieldMode:    "",
			inputMode:    "paste",
			expectedMode: "paste",
		},
		{
			name:         "default to file when no mode specified",
			fieldMode:    "",
			inputMode:    "",
			expectedMode: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the full readInputWithLimit without mocking,
			// but we can verify mode detection by checking which error we get.
			// For "file" mode without a file, we get MISSING_FILE.
			// For "paste" mode without content, we get MISSING_CONTENT.

			values := map[string]string{}
			if tt.fieldMode != "" {
				values["spec_mode"] = tt.fieldMode
			}
			if tt.inputMode != "" {
				values["input_mode"] = tt.inputMode
			}

			req := createFormRequest(t, values)
			if err := req.ParseForm(); err != nil {
				t.Fatalf("failed to parse form: %v", err)
			}

			_, errResp := h.readInputWithLimit(req, "spec", 1024)

			// We expect an error since we didn't provide any content
			if errResp == nil {
				t.Fatal("expected error response, got nil")
			}

			// Check the error type matches expected mode
			// file mode -> MISSING_FILE, paste mode -> MISSING_CONTENT
			body := errResp.Body()
			bodyStr, ok := body.(string)
			if !ok {
				// Try JSON response
				if resp, ok := body.(ErrorResponse); ok {
					bodyStr = resp.Error.Code
				}
			}

			switch tt.expectedMode {
			case "file":
				if !strings.Contains(bodyStr, "MISSING_FILE") && !strings.Contains(bodyStr, "file") {
					t.Errorf("expected file mode error, got: %v", bodyStr)
				}
			case "paste":
				if !strings.Contains(bodyStr, "MISSING_CONTENT") && !strings.Contains(bodyStr, "content") {
					t.Errorf("expected paste mode error, got: %v", bodyStr)
				}
			}
		})
	}
}

func TestHandler_readInputWithLimit_InvalidMode(t *testing.T) {
	h := &Handler{
		cfg: &config.Config{MaxFileSize: 1024},
	}

	req := createFormRequest(t, map[string]string{
		"input_mode": "invalid_mode",
	})
	if err := req.ParseForm(); err != nil {
		t.Fatalf("failed to parse form: %v", err)
	}

	_, errResp := h.readInputWithLimit(req, "spec", 1024)
	if errResp == nil {
		t.Fatal("expected error response for invalid mode")
	}
}

// =============================================================================
// readMultipleInputs Tests
// =============================================================================

// createMultiValueFormRequest creates a form request supporting repeated field names.
func createMultiValueFormRequest(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestHandler_readMultipleInputs_ModeDispatch(t *testing.T) {
	h := &Handler{cfg: &config.Config{MaxFileSize: 1024}}

	t.Run("defaults to file mode", func(t *testing.T) {
		// No input_mode field → file mode → FORM_NOT_PARSED (no multipart form)
		req := createFormRequest(t, map[string]string{})
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		_, errResp := h.readMultipleInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error response")
		}
	})

	t.Run("paste mode routes to paste handler", func(t *testing.T) {
		form := url.Values{}
		form.Set("input_mode", "paste")
		form.Add("specs_content", "spec one")
		form.Add("specs_content", "spec two")

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		sources, errResp := h.readMultipleInputs(req, "specs", 1024, 2, 5)
		if errResp != nil {
			t.Fatalf("unexpected error: %v", errResp.Body())
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}
		if sources[0].Mode != "paste" {
			t.Errorf("expected paste mode, got %q", sources[0].Mode)
		}
	})

	t.Run("url mode returns unsupported error", func(t *testing.T) {
		req := createFormRequest(t, map[string]string{
			"input_mode": "url",
		})
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error response for unsupported mode")
		}
	})

	t.Run("invalid mode returns error", func(t *testing.T) {
		req := createFormRequest(t, map[string]string{
			"input_mode": "bogus",
		})
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error response for invalid mode")
		}
	})
}

// =============================================================================
// readMultipleFileInputs Tests
// =============================================================================

func TestHandler_readMultipleFileInputs(t *testing.T) {
	h := &Handler{cfg: &config.Config{MaxFileSize: 1024}}

	t.Run("valid files", func(t *testing.T) {
		files := [][]byte{[]byte("spec one"), []byte("spec two")}
		req := createMultiFileRequest(t, "/test", "specs", files, nil)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		sources, errResp := h.readMultipleFileInputs(req, "specs", 1024, 2, 5)
		if errResp != nil {
			t.Fatalf("unexpected error: %v", errResp.Body())
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}
		for _, s := range sources {
			if s.Mode != "file" {
				t.Errorf("expected file mode, got %q", s.Mode)
			}
		}
	})

	t.Run("nil MultipartForm returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		_, errResp := h.readMultipleFileInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for nil MultipartForm")
		}
	})

	t.Run("too few files", func(t *testing.T) {
		files := [][]byte{[]byte("only one")}
		req := createMultiFileRequest(t, "/test", "specs", files, nil)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleFileInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for too few files")
		}
	})

	t.Run("too many files", func(t *testing.T) {
		files := make([][]byte, 6)
		for i := range files {
			files[i] = []byte(fmt.Sprintf("spec %d", i))
		}
		req := createMultiFileRequest(t, "/test", "files", files, nil)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleFileInputs(req, "files", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for too many files")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		files := [][]byte{[]byte("valid spec"), {}}
		req := createMultiFileRequest(t, "/test", "specs", files, nil)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleFileInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for empty file")
		}
	})

	t.Run("file exceeds size limit", func(t *testing.T) {
		files := [][]byte{[]byte("small"), bytes.Repeat([]byte("x"), 2000)}
		req := createMultiFileRequest(t, "/test", "specs", files, nil)
		if err := req.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultipleFileInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for oversized file")
		}
	})
}

// =============================================================================
// readMultiplePasteInputs Tests
// =============================================================================

func TestHandler_readMultiplePasteInputs(t *testing.T) {
	h := &Handler{cfg: &config.Config{MaxFileSize: 1024}}

	t.Run("valid pastes", func(t *testing.T) {
		form := url.Values{}
		form.Add("specs_content", "spec one")
		form.Add("specs_content", "spec two")

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		sources, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp != nil {
			t.Fatalf("unexpected error: %v", errResp.Body())
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources, got %d", len(sources))
		}
		if string(sources[0].Content) != "spec one" {
			t.Errorf("unexpected content: %q", sources[0].Content)
		}
		if sources[0].Filename != "pasted-spec-1" {
			t.Errorf("expected filename pasted-spec-1, got %q", sources[0].Filename)
		}
		if sources[1].Mode != "paste" {
			t.Errorf("expected paste mode, got %q", sources[1].Mode)
		}
	})

	t.Run("filters empty values", func(t *testing.T) {
		form := url.Values{}
		form.Add("specs_content", "spec one")
		form.Add("specs_content", "")
		form.Add("specs_content", "   ")
		form.Add("specs_content", "spec two")

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		sources, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp != nil {
			t.Fatalf("unexpected error: %v", errResp.Body())
		}
		if len(sources) != 2 {
			t.Errorf("expected 2 sources after filtering, got %d", len(sources))
		}
	})

	t.Run("too few after filtering", func(t *testing.T) {
		form := url.Values{}
		form.Add("specs_content", "only one")
		form.Add("specs_content", "")

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for insufficient specs after filtering")
		}
	})

	t.Run("too many pastes", func(t *testing.T) {
		form := url.Values{}
		for i := 0; i < 6; i++ {
			form.Add("specs_content", fmt.Sprintf("spec %d", i))
		}

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for too many specs")
		}
	})

	t.Run("paste exceeds size limit", func(t *testing.T) {
		form := url.Values{}
		form.Add("specs_content", "small")
		form.Add("specs_content", strings.Repeat("x", 2000))

		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for oversized paste")
		}
	})

	t.Run("no values submitted", func(t *testing.T) {
		form := url.Values{}
		req := createMultiValueFormRequest(t, form)
		if err := req.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		_, errResp := h.readMultiplePasteInputs(req, "specs", 1024, 2, 5)
		if errResp == nil {
			t.Fatal("expected error for no specs")
		}
	})
}
