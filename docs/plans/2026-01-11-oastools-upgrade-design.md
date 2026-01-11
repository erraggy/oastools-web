# oastools Dependency Upgrade & Feature Enhancement

**Date:** 2026-01-11
**Status:** Approved
**Scope:** Update oastools v1.36.1 → v1.45.0, add new features, comprehensive testing

## Summary

Update the oastools dependency from v1.36.1 to v1.45.0, exposing new fixer and validator options in the web UI. Add comprehensive regression testing using golden files and Playwright E2E tests.

## Goals

1. Update oastools dependency without regressions
2. Expose new fixer capabilities (duplicate operationId, CSV enum expansion, empty schema names)
3. Expose new validator option (validate-structure)
4. Establish testing infrastructure for future changes
5. Document design for future Explore page (follow-up PR)

## Non-Goals

- Implementing the Explore page (deferred to follow-up PR)
- Changing existing UI layout beyond adding new options
- Adding new operations beyond what oastools already supports

---

## Testing Strategy

### Three-Layer Approach

| Layer | Tool | Purpose | Speed |
|-------|------|---------|-------|
| Unit | Go tests | Handler logic, input parsing, error cases | Fast |
| Golden files | Go tests | Deterministic output verification | Fast |
| E2E | Playwright | Full browser UI verification | Slower |

### Golden File Structure

```
testdata/
├── golden/
│   ├── validate/
│   │   ├── petstore-3.0.input.yaml
│   │   └── petstore-3.0.golden.json
│   ├── fix/
│   │   ├── invalid-names.input.yaml
│   │   └── invalid-names.golden.json
│   ├── convert/
│   ├── diff/
│   ├── join/
│   └── overlay/
```

### Test Fixtures

Copy from `~/code/oastools/testdata/`:
- `petstore-3.0.yaml`, `petstore-2.0.yaml` — real-world validation
- `minimal-oas2.yaml`, `minimal-oas3.yaml` — minimal valid specs
- `invalid-oas2.yaml`, `invalid-oas3.yaml` — validation error cases
- `join-base-3.0.yaml`, `join-extension-3.0.yaml` — join operations
- Overlay fixtures from `testdata/overlay/`

### Playwright E2E Structure

```
e2e/
├── playwright.config.ts
├── fixtures/
│   └── (symlink or copy from testdata/golden/)
└── tests/
    ├── validate.spec.ts
    ├── fix.spec.ts
    ├── convert.spec.ts
    ├── diff.spec.ts
    ├── join.spec.ts
    └── overlay.spec.ts
```

### E2E Test Pattern

```typescript
test('validate shows errors for invalid spec', async ({ page }) => {
  await page.goto('/validate');
  await page.setInputFiles('input[name="spec"]', 'fixtures/invalid-oas3.yaml');
  await page.click('button[type="submit"]');
  await expect(page.locator('.validation-errors')).toBeVisible();
  await expect(page.locator('.error-count')).toContainText('3 errors');
});
```

### CI Integration

- Golden tests: Run on every PR (fast)
- Playwright tests: Run on every PR against local server

---

## Fixer Enhancements

### New Fix Types

| Fix Type | Constant | Description | UI Placement |
|----------|----------|-------------|--------------|
| Duplicate operationId | `FixTypeDuplicateOperationId` | Detect and rename duplicate operationIds | Main section |
| CSV enum expansion | `FixTypeExpandedCSVEnum` | Auto-expand CSV enum strings to arrays | Advanced |
| Empty schema names | `FixTypeRenamedEmptySchema` | Detect and rename empty schema names | Advanced |

### UI Layout

```
┌─────────────────────────────────────────────────┐
│ Fix Options                                     │
├─────────────────────────────────────────────────┤
│ ☑ Fix missing path parameters                  │
│ ☑ Remove unused schemas                        │
│ ☑ Fix invalid schema names                     │
│ ☑ Prune empty paths                            │
│ ☑ Fix duplicate operationIds        ← NEW      │
│                                                 │
│ ▶ Advanced Options                              │
│   ┌─────────────────────────────────────────┐  │
│   │ ☐ Expand CSV enums to arrays            │  │
│   │ ☐ Fix empty schema names                │  │
│   │ ☐ Dry run (preview only)                │  │
│   │ ☐ Infer types                           │  │
│   └─────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Backend Changes

File: `internal/api/fix.go`

```go
// Add new fix type handlers
if r.FormValue("fixDuplicateOperationIds") == "on" {
    enabledFixes = append(enabledFixes, fixer.FixTypeDuplicateOperationId)
}
if r.FormValue("expandCSVEnums") == "on" {
    enabledFixes = append(enabledFixes, fixer.FixTypeExpandedCSVEnum)
}
if r.FormValue("fixEmptySchemaNames") == "on" {
    enabledFixes = append(enabledFixes, fixer.FixTypeRenamedEmptySchema)
}
```

---

## Validator Enhancements

### New Option

| Option | Field | Description | Default | UI Placement |
|--------|-------|-------------|---------|--------------|
| Validate structure | `ValidateStructure` | Basic structure validation during parsing | true | Advanced |

**Distinction from strict mode:**
- `Strict`: Semantic validation beyond spec requirements
- `ValidateStructure`: Basic structure validation during parsing

### UI Layout

```
┌─────────────────────────────────────────────────┐
│ Validation Options                              │
├─────────────────────────────────────────────────┤
│ ☑ Include warnings                             │
│ ☐ Strict mode                                  │
│                                                 │
│ ▶ Advanced Options                              │
│   ┌─────────────────────────────────────────┐  │
│   │ ☑ Validate structure (default: on)      │  │
│   │   Skip to parse partially invalid specs │  │
│   └─────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

### Backend Changes

File: `internal/api/validate.go`

```go
validateStructure := r.FormValue("validateStructure") != "off" // Default true
v := validator.New()
v.StrictMode = strict
v.IncludeWarnings = includeWarnings
v.ValidateStructure = validateStructure
```

---

## Future: Explore Page (Follow-up PR)

### Purpose

Read-only analysis view of an OpenAPI spec using the walker package.

### Planned Features

| Feature | Walker Function | Display |
|---------|-----------------|---------|
| Operations by tag | `CollectOperations().ByTag` | Grouped accordion |
| Operations by method | `CollectOperations().ByMethod` | Color-coded badges |
| Schema inventory | `CollectSchemas().Components` | List with type indicators |
| Inline vs component | `CollectSchemas().Inline` vs `.Components` | Counts |
| Spec statistics | Custom walker | Paths, operations, schemas |

### Proposed UI

```
┌─────────────────────────────────────────────────┐
│ Explore                                         │
├─────────────────────────────────────────────────┤
│ [Upload spec or paste]                          │
│                                                 │
│ ┌─ Summary ───────────────────────────────────┐ │
│ │ Version: OAS 3.0.3                          │ │
│ │ Paths: 12 | Operations: 28 | Schemas: 15   │ │
│ └─────────────────────────────────────────────┘ │
│                                                 │
│ [Operations] [Schemas] [Security]   ← tabs      │
│                                                 │
│ ▼ pets (8 operations)                           │
│   GET  /pets         List all pets              │
│   POST /pets         Create a pet               │
│   GET  /pets/{id}    Get pet by ID              │
│ ▼ users (4 operations)                          │
│   ...                                           │
└─────────────────────────────────────────────────┘
```

---

## Implementation Plan

### Phase 1: Testing Infrastructure
1. Create `testdata/golden/` directory structure
2. Copy test fixtures from oastools repo
3. Implement golden file test harness
4. Set up Playwright with basic smoke tests

### Phase 2: Dependency Update
1. Update `go.mod` to oastools v1.45.0
2. Run `go mod tidy`
3. Fix any compile errors from API changes
4. Run existing tests
5. Run golden tests to detect output changes
6. Investigate and accept/fix any differences

### Phase 3: Fixer Enhancements
1. Add new form fields to fix template
2. Implement Advanced Options collapsible section
3. Update fix.go handler for new fix types
4. Add golden tests for new fix types
5. Add Playwright tests for fix page

### Phase 4: Validator Enhancements
1. Add validateStructure form field to validate template
2. Implement Advanced Options collapsible section
3. Update validate.go handler
4. Add golden tests for validate-structure option
5. Add Playwright tests for validate page

### Phase 5: Final Verification
1. Run full test suite
2. Manual browser testing of all operations
3. Review CI pipeline passes
4. Create PR

---

## Files to Modify

| File | Changes |
|------|---------|
| `go.mod` | Update oastools version |
| `internal/api/fix.go` | Add new fix type handlers |
| `internal/api/validate.go` | Add validateStructure handler |
| `internal/templates/fix.html` | New checkboxes, Advanced section |
| `internal/templates/validate.html` | Advanced section with validateStructure |
| `static/styles.css` | Collapsible Advanced section styles |

## New Files

| File | Purpose |
|------|---------|
| `testdata/golden/**` | Golden file test fixtures |
| `internal/api/golden_test.go` | Golden file test harness |
| `e2e/playwright.config.ts` | Playwright configuration |
| `e2e/tests/*.spec.ts` | E2E test files |
| `Makefile` | Add `test-e2e` target |

---

## Risks and Mitigations

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| API breaking changes in oastools | Low | Golden tests will detect; walker breaking change doesn't affect us |
| Output format changes | Medium | Golden tests will detect; review and update baselines if acceptable |
| New fix types have edge cases | Low | Copy test fixtures that exercise edge cases from oastools |
| Playwright flakiness | Medium | Use explicit waits, retry logic, run against local server |

---

## Success Criteria

1. All existing tests pass
2. Golden tests establish baseline and pass
3. Playwright E2E tests cover all operations
4. New fixer options visible and functional in UI
5. New validator option visible and functional in UI
6. CI pipeline green
7. No regressions in production after deploy
