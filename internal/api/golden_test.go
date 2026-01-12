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
	"regexp"
	"slices"
	"sort"
	"strings"
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

			// Verify HTTP status code
			if rec.Code != http.StatusOK {
				t.Fatalf("unexpected status code: got %d, want %d\nresponse body: %s",
					rec.Code, http.StatusOK, rec.Body.String())
			}

			// Normalize response for comparison
			got := normalizeJSON(t, rec.Body.Bytes())

			if *updateGolden {
				if err := os.WriteFile(tc.goldenFile, got, 0644); err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				t.Logf("updated golden file: %s", tc.goldenFile)
				return
			}

			wantRaw, err := os.ReadFile(tc.goldenFile)
			if err != nil {
				t.Fatalf("failed to read golden file (run with -update-golden to create): %v", err)
			}

			// Normalize the golden file too to ensure consistent comparison
			want := normalizeJSON(t, wantRaw)

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

		part, err := writer.CreateFormFile(field, filepath.Base(path))
		if err != nil {
			_ = file.Close()
			t.Fatalf("failed to create form file: %v", err)
		}

		if _, err := io.Copy(part, file); err != nil {
			_ = file.Close()
			t.Fatalf("failed to copy file content: %v", err)
		}

		// Close file immediately after copying content to avoid leaking file descriptors
		_ = file.Close()
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

	// Sort arrays that may have non-deterministic order
	sortArraysRecursively(v)

	// Re-marshal with consistent formatting
	normalized, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	return append(normalized, '\n')
}

// sortArraysRecursively sorts arrays named "errors" or "warnings" by their JSON representation
// to ensure deterministic ordering for golden file comparison.
func sortArraysRecursively(v any) {
	switch val := v.(type) {
	case map[string]any:
		for key, value := range val {
			if key == "errors" || key == "warnings" {
				if arr, ok := value.([]any); ok {
					// Normalize error messages before sorting
					for i, item := range arr {
						if m, ok := item.(map[string]any); ok {
							arr[i] = normalizeErrorItem(m)
						}
					}
					slices.SortFunc(arr, compareJSONValues)
				}
			}
			sortArraysRecursively(value)
		}
	case []any:
		for _, item := range val {
			sortArraysRecursively(item)
		}
	}
}

// normalizeErrorItem normalizes error messages that contain non-deterministic path references.
// For example, duplicate operationId errors reference paths in iteration order which varies.
func normalizeErrorItem(m map[string]any) map[string]any {
	msg, ok := m["message"].(string)
	if !ok {
		return m
	}

	// Normalize "Duplicate operationId 'X' (first seen at Y)" pattern
	// The path mentioned may vary based on map iteration order
	// Also normalize the error's path field since it varies too
	if strings.Contains(msg, "Duplicate operationId") {
		re := regexp.MustCompile(`Duplicate operationId '([^']+)' \(first seen at ([^)]+)\)`)
		if matches := re.FindStringSubmatch(msg); len(matches) == 3 {
			opID := matches[1]
			firstPath := matches[2]
			// Get the current path and combine both into a deterministic form
			if currentPath, ok := m["path"].(string); ok {
				paths := []string{currentPath, firstPath}
				sort.Strings(paths)
				m["message"] = "Duplicate operationId '" + opID + "' at '" + paths[0] + "' and '" + paths[1] + "'"
				m["path"] = "duplicate:" + opID // Normalize path to avoid iteration-dependent values
			}
		}
	}

	// Normalize "duplicate operationId 'X' at 'Y': previously defined at 'Z'" pattern
	// Both Y and Z paths can vary based on iteration order
	if strings.Contains(msg, "duplicate operationId") && strings.Contains(msg, "previously defined") {
		re := regexp.MustCompile(`duplicate operationId '([^']+)' at '([^']+)': previously defined at '([^']+)'`)
		if matches := re.FindStringSubmatch(msg); len(matches) == 4 {
			// Sort the paths to ensure deterministic order
			paths := []string{matches[2], matches[3]}
			sort.Strings(paths)
			m["message"] = "duplicate operationId '" + matches[1] + "' at '" + paths[0] + "' and '" + paths[1] + "'"
		}
	}

	return m
}

// compareJSONValues compares two values by their JSON string representation.
func compareJSONValues(a, b any) int {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return bytes.Compare(aJSON, bJSON)
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
