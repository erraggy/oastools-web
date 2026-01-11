# oastools Upgrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Update oastools from v1.36.1 to v1.45.0, expose new fixer/validator features, add comprehensive testing.

**Architecture:** Add golden file tests for deterministic output verification, Playwright E2E tests for browser UI coverage. Update handlers and templates to expose new fix types and validator options. Use collapsible Advanced Options sections in UI.

**Tech Stack:** Go 1.25, oastools v1.45.0, Playwright, HTMX, Go html/template

---

## Task 1: Copy Test Fixtures from oastools

**Files:**
- Create: `testdata/golden/validate/petstore-3.0.input.yaml`
- Create: `testdata/golden/validate/minimal-oas3.input.yaml`
- Create: `testdata/golden/validate/invalid-oas3.input.yaml`
- Create: `testdata/golden/fix/petstore-3.0.input.yaml`
- Create: `testdata/golden/join/base-3.0.input.yaml`
- Create: `testdata/golden/join/extension-3.0.input.yaml`

**Step 1: Create testdata directory structure**

Run:
```bash
mkdir -p testdata/golden/{validate,fix,convert,diff,join,overlay}
```

**Step 2: Copy test fixtures from oastools repo**

Run:
```bash
cp ~/code/oastools/testdata/petstore-3.0.yaml testdata/golden/validate/petstore-3.0.input.yaml
cp ~/code/oastools/testdata/petstore-2.0.yaml testdata/golden/validate/petstore-2.0.input.yaml
cp ~/code/oastools/testdata/minimal-oas3.yaml testdata/golden/validate/minimal-oas3.input.yaml
cp ~/code/oastools/testdata/minimal-oas2.yaml testdata/golden/validate/minimal-oas2.input.yaml
cp ~/code/oastools/testdata/invalid-oas3.yaml testdata/golden/validate/invalid-oas3.input.yaml
cp ~/code/oastools/testdata/invalid-oas2.yaml testdata/golden/validate/invalid-oas2.input.yaml
cp ~/code/oastools/testdata/petstore-3.0.yaml testdata/golden/fix/petstore-3.0.input.yaml
cp ~/code/oastools/testdata/petstore-2.0.yaml testdata/golden/convert/petstore-2.0.input.yaml
cp ~/code/oastools/testdata/petstore-3.0.yaml testdata/golden/convert/petstore-3.0.input.yaml
cp ~/code/oastools/testdata/petstore-v1.yaml testdata/golden/diff/petstore-v1.input.yaml
cp ~/code/oastools/testdata/petstore-v2.yaml testdata/golden/diff/petstore-v2.input.yaml
cp ~/code/oastools/testdata/join-base-3.0.yaml testdata/golden/join/base-3.0.input.yaml
cp ~/code/oastools/testdata/join-extension-3.0.yaml testdata/golden/join/extension-3.0.input.yaml
cp ~/code/oastools/testdata/overlay/base.yaml testdata/golden/overlay/base.input.yaml
cp ~/code/oastools/testdata/overlay/overlay.yaml testdata/golden/overlay/overlay.input.yaml
```

**Step 3: Verify files copied**

Run:
```bash
find testdata/golden -name "*.yaml" | wc -l
```
Expected: `15` (or similar count)

**Step 4: Commit**

Run:
```bash
git add testdata/
git commit -m "test: add golden test fixtures from oastools repo"
```

---

## Task 2: Create Golden File Test Harness

**Files:**
- Create: `internal/api/golden_test.go`

**Step 1: Write the golden test infrastructure**

Create `internal/api/golden_test.go`:

```go
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
			formValues: map[string]string{"targetVersion": "3.0"},
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
	handler, err := NewHandler(cfg)
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
			handler.server.ServeHTTP(rec, req)

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
```

**Step 2: Run test to verify it compiles (will fail without golden files)**

Run:
```bash
go test -v ./internal/api/ -run TestGolden -count=1 2>&1 | head -20
```
Expected: Tests run but fail with "failed to read golden file"

**Step 3: Generate initial golden files**

Run:
```bash
go test -v ./internal/api/ -run TestGolden -update-golden
```
Expected: Golden files created

**Step 4: Verify golden files exist**

Run:
```bash
find testdata/golden -name "*.golden.json" | head -10
```
Expected: List of golden files

**Step 5: Run tests to confirm they pass**

Run:
```bash
go test -v ./internal/api/ -run TestGolden
```
Expected: All tests pass

**Step 6: Commit**

Run:
```bash
git add internal/api/golden_test.go testdata/golden/
git commit -m "test: add golden file test harness with initial baselines"
```

---

## Task 3: Set Up Playwright E2E Tests

**Files:**
- Create: `e2e/playwright.config.ts`
- Create: `e2e/tests/validate.spec.ts`
- Create: `e2e/tests/fix.spec.ts`
- Modify: `package.json`
- Modify: `Makefile`

**Step 1: Create Playwright config**

Create `e2e/playwright.config.ts`:

```typescript
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:8080',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'make run',
    url: 'http://localhost:8080/health',
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
  },
});
```

**Step 2: Create validate E2E test**

Create `e2e/tests/validate.spec.ts`:

```typescript
import { test, expect } from '@playwright/test';
import path from 'path';

test.describe('Validate Page', () => {
  test('validates a valid OpenAPI spec', async ({ page }) => {
    await page.goto('/validate');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/validate/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result
    await expect(page.locator('#result')).toContainText('Valid', { timeout: 10000 });
  });

  test('shows errors for invalid spec', async ({ page }) => {
    await page.goto('/validate');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/validate/invalid-oas3.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for error result
    await expect(page.locator('#result')).toContainText('error', { timeout: 10000 });
  });

  test('can use paste input mode', async ({ page }) => {
    await page.goto('/validate');

    // Switch to paste mode
    await page.click('button[data-mode="paste"]');

    const textarea = page.locator('textarea[name="spec_content"]');
    await expect(textarea).toBeEnabled();

    await textarea.fill(`openapi: "3.0.0"
info:
  title: Test
  version: "1.0"
paths: {}`);

    await page.click('button[type="submit"]');

    await expect(page.locator('#result')).toContainText('Valid', { timeout: 10000 });
  });
});
```

**Step 3: Create fix E2E test**

Create `e2e/tests/fix.spec.ts`:

```typescript
import { test, expect } from '@playwright/test';
import path from 'path';

test.describe('Fix Page', () => {
  test('fixes a spec and shows results', async ({ page }) => {
    await page.goto('/fix');

    const fileInput = page.locator('input[name="spec"]');
    await fileInput.setInputFiles(path.join(__dirname, '../../testdata/golden/fix/petstore-3.0.input.yaml'));

    await page.click('button[type="submit"]');

    // Wait for result - should show fixes or "no fixes needed"
    await expect(page.locator('#result')).toBeVisible({ timeout: 10000 });
  });

  test('can toggle advanced options', async ({ page }) => {
    await page.goto('/fix');

    // Advanced options should be collapsed by default
    const advancedContent = page.locator('.advanced-options .advanced-content');
    await expect(advancedContent).not.toBeVisible();

    // Click to expand
    await page.click('.advanced-options summary');
    await expect(advancedContent).toBeVisible();

    // Dry run checkbox should be present
    await expect(page.locator('input[name="dryRun"]')).toBeVisible();
  });
});
```

**Step 4: Update package.json scripts**

Add to `package.json` scripts section (after line 7):

```json
"test:e2e": "npx playwright test --config=e2e/playwright.config.ts",
"test:e2e:ui": "npx playwright test --config=e2e/playwright.config.ts --ui"
```

**Step 5: Add Makefile target**

Add to `Makefile` (after line 29):

```makefile
## test-e2e: Run Playwright E2E tests
test-e2e: build
	@echo "Running E2E tests..."
	npm run test:e2e
```

**Step 6: Install Playwright browsers**

Run:
```bash
npx playwright install chromium
```

**Step 7: Run E2E tests to verify setup**

Run:
```bash
npm run test:e2e
```
Expected: Tests pass

**Step 8: Commit**

Run:
```bash
git add e2e/ package.json Makefile
git commit -m "test: add Playwright E2E test infrastructure"
```

---

## Task 4: Update oastools Dependency

**Files:**
- Modify: `go.mod`

**Step 1: Update dependency**

Run:
```bash
go get github.com/erraggy/oastools@v1.45.0
go mod tidy
```

**Step 2: Verify build compiles**

Run:
```bash
go build ./...
```
Expected: No errors

**Step 3: Run existing tests**

Run:
```bash
go test ./...
```
Expected: All tests pass

**Step 4: Run golden tests to check for regressions**

Run:
```bash
go test -v ./internal/api/ -run TestGolden
```
Expected: All tests pass (or document acceptable differences)

**Step 5: Commit**

Run:
```bash
git add go.mod go.sum
git commit -m "deps: update oastools to v1.45.0"
```

---

## Task 5: Add New Fixer Options to Template

**Files:**
- Modify: `internal/templates/fix.html`

**Step 1: Add duplicate operationId checkbox to main options**

In `internal/templates/fix.html`, after line 55 (pruneEmptyPaths checkbox), add:

```html
                <label class="checkbox-label">
                    <input type="checkbox" name="fixDuplicateOperationIds">
                    Fix duplicate operation IDs
                </label>
```

**Step 2: Add new advanced options**

In `internal/templates/fix.html`, after line 74 (inferTypes checkbox div), add:

```html
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="expandCSVEnums">
                        Expand CSV enum strings to arrays
                    </label>
                </div>
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="fixEmptySchemaNames">
                        Fix empty schema names
                    </label>
                </div>
```

**Step 3: Verify template renders**

Run:
```bash
go build ./cmd/server && ./bin/oastools-web &
sleep 2 && curl -s http://localhost:8080/fix | grep -c "fixDuplicateOperationIds"
pkill oastools-web
```
Expected: `1`

**Step 4: Commit**

Run:
```bash
git add internal/templates/fix.html
git commit -m "feat(ui): add new fixer options to fix page"
```

---

## Task 6: Add New Fixer Handler Logic

**Files:**
- Modify: `internal/api/fix.go`

**Step 1: Add handler logic for new fix types**

In `internal/api/fix.go`, after line 70 (pruneEmptyPaths block), add:

```go
	if r.FormValue("fixDuplicateOperationIds") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeDuplicateOperationId)
	}
	if r.FormValue("expandCSVEnums") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeEnumCSVExpanded)
	}
	if r.FormValue("fixEmptySchemaNames") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeRenamedEmptySchema)
	}
```

**Step 2: Verify build**

Run:
```bash
go build ./...
```
Expected: No errors

**Step 3: Run tests**

Run:
```bash
go test ./internal/api/... -v
```
Expected: All tests pass

**Step 4: Commit**

Run:
```bash
git add internal/api/fix.go
git commit -m "feat(api): add handler support for new fixer options"
```

---

## Task 7: Add Validate Structure Option to Template

**Files:**
- Modify: `internal/templates/validate.html`

**Step 1: Add validateStructure checkbox to advanced options**

In `internal/templates/validate.html`, after line 52 (includeWarnings checkbox div), add:

```html
                <div class="form-group">
                    <label class="checkbox-label">
                        <input type="checkbox" name="validateStructure" checked>
                        Validate structure during parsing
                    </label>
                    <small class="option-hint">Uncheck to parse partially invalid specs</small>
                </div>
```

**Step 2: Verify template renders**

Run:
```bash
go build ./cmd/server && ./bin/oastools-web &
sleep 2 && curl -s http://localhost:8080/validate | grep -c "validateStructure"
pkill oastools-web
```
Expected: `1`

**Step 3: Commit**

Run:
```bash
git add internal/templates/validate.html
git commit -m "feat(ui): add validate-structure option to validate page"
```

---

## Task 8: Add Validate Structure Handler Logic

**Files:**
- Modify: `internal/api/validate.go`

**Step 1: Add handler logic for validateStructure**

In `internal/api/validate.go`, after line 63 (includeWarnings line), add:

```go
	validateStructure := r.FormValue("validateStructure") != "off"
```

**Step 2: Set the validator field**

In `internal/api/validate.go`, after line 75 (v.IncludeWarnings line), add:

```go
	v.ValidateStructure = validateStructure
```

**Step 3: Verify build**

Run:
```bash
go build ./...
```
Expected: No errors

**Step 4: Run tests**

Run:
```bash
go test ./internal/api/... -v
```
Expected: All tests pass

**Step 5: Commit**

Run:
```bash
git add internal/api/validate.go
git commit -m "feat(api): add validate-structure option support"
```

---

## Task 9: Add CSS for Option Hints

**Files:**
- Modify: `static/css/style.css`

**Step 1: Add option-hint style**

Add to `static/css/style.css` (find the form styling section):

```css
.option-hint {
    display: block;
    margin-left: 1.75rem;
    margin-top: 0.25rem;
    color: var(--text-muted);
    font-size: 0.85rem;
}
```

**Step 2: Verify CSS loads**

Run:
```bash
grep -c "option-hint" static/css/style.css
```
Expected: `1` or more

**Step 3: Commit**

Run:
```bash
git add static/css/style.css
git commit -m "style: add option-hint styling for form descriptions"
```

---

## Task 10: Update Golden Files for New Version

**Files:**
- Modify: `testdata/golden/**/*.golden.json`

**Step 1: Regenerate golden files with new oastools version**

Run:
```bash
go test ./internal/api/ -run TestGolden -update-golden
```

**Step 2: Review changes**

Run:
```bash
git diff testdata/golden/
```
Expected: Review output differences are acceptable

**Step 3: Run golden tests to confirm**

Run:
```bash
go test ./internal/api/ -run TestGolden -v
```
Expected: All tests pass

**Step 4: Commit**

Run:
```bash
git add testdata/golden/
git commit -m "test: update golden files for oastools v1.45.0"
```

---

## Task 11: Add E2E Tests for New Options

**Files:**
- Modify: `e2e/tests/fix.spec.ts`
- Modify: `e2e/tests/validate.spec.ts`

**Step 1: Add fix page new options test**

Add to `e2e/tests/fix.spec.ts`:

```typescript
test('new fix options are present', async ({ page }) => {
  await page.goto('/fix');

  // Main section should have duplicate operationIds option
  await expect(page.locator('input[name="fixDuplicateOperationIds"]')).toBeVisible();

  // Expand advanced options
  await page.click('.advanced-options summary');

  // New advanced options should be visible
  await expect(page.locator('input[name="expandCSVEnums"]')).toBeVisible();
  await expect(page.locator('input[name="fixEmptySchemaNames"]')).toBeVisible();
});
```

**Step 2: Add validate page new options test**

Add to `e2e/tests/validate.spec.ts`:

```typescript
test('validateStructure option is present and checked by default', async ({ page }) => {
  await page.goto('/validate');

  // Expand advanced options
  await page.click('.advanced-options summary');

  const checkbox = page.locator('input[name="validateStructure"]');
  await expect(checkbox).toBeVisible();
  await expect(checkbox).toBeChecked();
});
```

**Step 3: Run E2E tests**

Run:
```bash
npm run test:e2e
```
Expected: All tests pass

**Step 4: Commit**

Run:
```bash
git add e2e/tests/
git commit -m "test(e2e): add tests for new fix and validate options"
```

---

## Task 12: Final Verification

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

**Step 4: Manual smoke test**

Run:
```bash
make run
```

Open browser to http://localhost:8080 and verify:
- [ ] Validate page: advanced options expand, validateStructure checkbox visible and checked
- [ ] Fix page: duplicate operationIds in main options, CSV enums and empty schema names in advanced
- [ ] All operations complete successfully with test files

**Step 5: Run full check suite**

Run:
```bash
make check
```
Expected: All checks pass

**Step 6: Final commit (if any uncommitted changes)**

Run:
```bash
git status
```

If changes exist:
```bash
git add -A
git commit -m "chore: final cleanup"
```

---

## Task 13: Create Pull Request

**Step 1: Push branch**

Run:
```bash
git push -u origin HEAD
```

**Step 2: Create PR**

Run:
```bash
gh pr create --title "feat: upgrade oastools to v1.45.0 with new fixer/validator options" --body "$(cat <<'EOF'
## Summary
- Update oastools dependency from v1.36.1 to v1.45.0
- Add new fixer options: duplicate operationId, CSV enum expansion, empty schema names
- Add new validator option: validate-structure
- Add golden file test infrastructure
- Add Playwright E2E tests

## Test plan
- [x] Golden file tests pass
- [x] Playwright E2E tests pass
- [x] Manual browser testing of all operations
- [x] `make check` passes

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Parallelization Notes for Subagent Execution

The following tasks can run in parallel:

**Parallel Group 1 (Testing Infrastructure):**
- Task 1: Copy Test Fixtures
- Task 3: Set Up Playwright (after fixtures exist)

**Parallel Group 2 (After dependency update):**
- Task 5 + Task 6: Fixer enhancements (template + handler)
- Task 7 + Task 8: Validator enhancements (template + handler)
- Task 9: CSS styling

**Sequential Dependencies:**
- Task 2 depends on Task 1
- Task 4 depends on Task 2 (need golden baselines before upgrade)
- Task 10 depends on Task 4
- Task 11 depends on Tasks 5-8
- Tasks 12-13 are final and sequential
