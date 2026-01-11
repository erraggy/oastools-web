package api

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/erraggy/oastools-web/internal/config"
)

var updateGolden = flag.Bool("update-golden", false, "update golden files")

// goldenTest represents a single golden file test case.
type goldenTest struct {
	name       string
	endpoint   string
	inputFiles map[string]string // field name -> file path
	formValues map[string]string // additional form values
	goldenFile string
}

func TestGoldenValidate(t *testing.T) {
	tests := []goldenTest{
		{
			name:       "petstore-3.0",
			endpoint:   "/api/validate",
			inputFiles: map[string]string{"spec": "testdata/golden/validate/petstore-3.0.input.yaml"},
			goldenFile: "testdata/golden/validate/petstore-3.0.golden.json",
		},
		{
			name:       "minimal-oas3",
			endpoint:   "/api/validate",
			inputFiles: map[string]string{"spec": "testdata/golden/validate/minimal-oas3.input.yaml"},
			goldenFile: "testdata/golden/validate/minimal-oas3.golden.json",
		},
		{
			name:       "invalid-oas3",
			endpoint:   "/api/validate",
			inputFiles: map[string]string{"spec": "testdata/golden/validate/invalid-oas3.input.yaml"},
			goldenFile: "testdata/golden/validate/invalid-oas3.golden.json",
		},
	}

	runGoldenTests(t, tests)
}

func TestGoldenFix(t *testing.T) {
	tests := []goldenTest{
		{
			name:       "petstore-3.0",
			endpoint:   "/api/fix",
			inputFiles: map[string]string{"spec": "testdata/golden/fix/petstore-3.0.input.yaml"},
			goldenFile: "testdata/golden/fix/petstore-3.0.golden.json",
		},
	}

	runGoldenTests(t, tests)
}

func TestGoldenConvert(t *testing.T) {
	tests := []goldenTest{
		{
			name:       "oas2-to-oas3",
			endpoint:   "/api/convert",
			inputFiles: map[string]string{"spec": "testdata/golden/convert/petstore-2.0.input.yaml"},
			formValues: map[string]string{"target": "3.0"},
			goldenFile: "testdata/golden/convert/petstore-2.0-to-3.0.golden.json",
		},
	}

	runGoldenTests(t, tests)
}

func runGoldenTests(t *testing.T, tests []goldenTest) {
	// Change to repo root for testdata paths
	if err := os.Chdir(findRepoRoot(t)); err != nil {
		t.Fatalf("failed to change to repo root: %v", err)
	}

	cfg := config.Load()
	handler, err := NewHandler(cfg, "test-version")
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Build multipart request
			body, contentType := buildMultipartRequest(t, tc.inputFiles, tc.formValues)

			req := httptest.NewRequest(http.MethodPost, tc.endpoint, body)
			req.Header.Set("Content-Type", contentType)
			req.Header.Set("Accept", "application/json")

			rec := httptest.NewRecorder()
			handler.server.Handler.ServeHTTP(rec, req)

			// Normalize response for comparison
			got := normalizeJSON(t, rec.Body.Bytes())

			if *updateGolden {
				if err := os.WriteFile(tc.goldenFile, got, 0644); err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				t.Logf("updated golden file: %s", tc.goldenFile)
				return
			}

			want, err := os.ReadFile(tc.goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file (run with -update-golden to create): %v", err)
			}

			if !bytes.Equal(got, want) {
				t.Errorf("response mismatch\ngot:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func buildMultipartRequest(t *testing.T, files map[string]string, values map[string]string) (*bytes.Buffer, string) {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for field, path := range files {
		file, err := os.Open(path)
		if err != nil {
			t.Fatalf("failed to open %s: %v", path, err)
		}
		defer file.Close()

		part, err := writer.CreateFormFile(field, filepath.Base(path))
		if err != nil {
			t.Fatalf("failed to create form file: %v", err)
		}

		if _, err := io.Copy(part, file); err != nil {
			t.Fatalf("failed to copy file content: %v", err)
		}
	}

	for field, value := range values {
		if err := writer.WriteField(field, value); err != nil {
			t.Fatalf("failed to write field %s: %v", field, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close writer: %v", err)
	}

	return body, writer.FormDataContentType()
}

func normalizeJSON(t *testing.T, data []byte) []byte {
	t.Helper()

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		// Not JSON, return as-is
		return data
	}

	// Re-marshal with consistent formatting
	normalized, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	return append(normalized, '\n')
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
