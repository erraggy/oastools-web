package api

import (
	"context"
	"testing"

	"github.com/erraggy/oastools/parser"
)

// dupOperationIDSpec has two operations sharing an operationId, which the
// parser reports as an error and the fixer repairs by renaming the second.
const dupOperationIDSpec = `openapi: 3.0.3
info:
  title: Dup
  version: 1.0.0
paths:
  /a:
    get:
      operationId: listThings
      responses:
        '200':
          description: ok
  /b:
    get:
      operationId: listThings
      responses:
        '200':
          description: ok
`

// cleanSpec parses without any errors.
const cleanSpec = `openapi: 3.0.3
info:
  title: Clean
  version: 1.0.0
paths:
  /a:
    get:
      operationId: listThings
      responses:
        '200':
          description: ok
`

// dupOperationIDErrors returns the errors the parser reports for
// dupOperationIDSpec, so tests compare against real messages rather than
// synthetic ones.
func dupOperationIDErrors(t *testing.T) []error {
	t.Helper()

	result, err := parser.ParseWithOptions(parser.WithBytes([]byte(dupOperationIDSpec)))
	if err != nil {
		t.Fatalf("failed to parse test spec: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Fatalf("expected the test spec to report parse errors, got none")
	}
	return result.Errors
}

func TestClassifyParseErrors(t *testing.T) {
	h := minimalHandler(t)
	ctx := context.Background()

	t.Run("counts a net reduction when the output reports fewer issues", func(t *testing.T) {
		inputErrs := dupOperationIDErrors(t)

		verdict := h.classifyParseErrors(ctx, inputErrs, cleanSpec, false)

		if !verdict.known {
			t.Fatal("expected a known verdict for a non-dry run with parseable output")
		}
		if len(verdict.remaining) != 0 {
			t.Errorf("remaining = %v, want none", verdict.remaining)
		}
		if verdict.netReduction != len(inputErrs) {
			t.Errorf("netReduction = %d, want %d", verdict.netReduction, len(inputErrs))
		}
		if len(verdict.submitted) != len(inputErrs) {
			t.Errorf("submitted = %d issues, want %d", len(verdict.submitted), len(inputErrs))
		}
	})

	t.Run("reports the output's own issues when nothing was repaired", func(t *testing.T) {
		inputErrs := dupOperationIDErrors(t)

		// Output identical to the input: nothing was repaired.
		verdict := h.classifyParseErrors(ctx, inputErrs, dupOperationIDSpec, false)

		if !verdict.known {
			t.Fatal("expected a known verdict")
		}
		if len(verdict.remaining) != len(inputErrs) {
			t.Errorf("remaining = %d, want %d", len(verdict.remaining), len(inputErrs))
		}
		if verdict.netReduction != 0 {
			t.Errorf("netReduction = %d, want 0", verdict.netReduction)
		}
	})

	t.Run("is stable across repeated runs of the same document", func(t *testing.T) {
		// The parser names the locations in a duplicate-operationId message in
		// map-iteration order, so the wording varies between parses of the same
		// bytes. The verdict must not vary with it.
		for i := range 40 {
			verdict := h.classifyParseErrors(ctx, dupOperationIDErrors(t), dupOperationIDSpec, false)

			if verdict.netReduction != 0 || len(verdict.remaining) != 1 {
				t.Fatalf("run %d: netReduction = %d, remaining = %d; want a steady 0 resolved and 1 remaining",
					i, verdict.netReduction, len(verdict.remaining))
			}
		}
	})

	t.Run("never reports a net reduction larger than the submitted count", func(t *testing.T) {
		// An output with more issues than the input must not yield a negative
		// count dressed up as progress.
		verdict := h.classifyParseErrors(ctx, nil, dupOperationIDSpec, false)

		if verdict.netReduction != 0 {
			t.Errorf("netReduction = %d, want 0", verdict.netReduction)
		}
		if len(verdict.remaining) == 0 {
			t.Error("expected the output's issues to be reported")
		}
	})

	t.Run("a dry run yields no verdict", func(t *testing.T) {
		inputErrs := dupOperationIDErrors(t)

		verdict := h.classifyParseErrors(ctx, inputErrs, dupOperationIDSpec, true)

		if verdict.known {
			t.Error("expected no verdict for a dry run")
		}
		if !verdict.dryRun {
			t.Error("expected dryRun to be set, so the page can tell this from a failed re-parse")
		}
		if verdict.netReduction != 0 {
			t.Errorf("netReduction = %d, want 0 when no verdict is available", verdict.netReduction)
		}
		if len(verdict.remaining) != 0 {
			t.Errorf("remaining = %v, want none when nothing was re-checked", verdict.remaining)
		}
		if len(verdict.submitted) != len(inputErrs) {
			t.Errorf("submitted = %d issues, want %d", len(verdict.submitted), len(inputErrs))
		}
	})

	t.Run("no verdict when the output cannot be parsed", func(t *testing.T) {
		inputErrs := dupOperationIDErrors(t)

		verdict := h.classifyParseErrors(ctx, inputErrs, "\x00not a spec at all", false)

		if verdict.known {
			t.Error("expected no verdict when the output cannot be re-parsed")
		}
		if verdict.dryRun {
			t.Error("expected dryRun to stay false: this is a re-parse failure, not a preview")
		}
		if len(verdict.submitted) != len(inputErrs) {
			t.Errorf("submitted = %d issues, want the input errors reported without a verdict", len(verdict.submitted))
		}
	})

	t.Run("a clean document yields nothing to report", func(t *testing.T) {
		verdict := h.classifyParseErrors(ctx, nil, cleanSpec, false)

		if len(verdict.submitted) != 0 || len(verdict.remaining) != 0 {
			t.Errorf("submitted = %v, remaining = %v; want both empty", verdict.submitted, verdict.remaining)
		}
		if verdict.netReduction != 0 {
			t.Errorf("netReduction = %d, want 0", verdict.netReduction)
		}
	})
}
