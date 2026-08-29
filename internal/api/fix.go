package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/fixer"
	"github.com/erraggy/oastools/parser"
)

// FixResponse represents the fix result.
type FixResponse struct {
	Version string     `json:"version"`
	Fixes   []FixEntry `json:"fixes"`
	// SubmittedIssues are the problems the parser reported about the document
	// that was submitted. They describe the input, so the fixes above may well
	// have resolved some of them; RemainingIssues is what actually survived.
	SubmittedIssues []string `json:"submittedIssues,omitempty"`
	// RemainingIssues are the problems the fixed output still reports. This is
	// the authoritative "what is still wrong" list.
	RemainingIssues []string `json:"remainingIssues,omitempty"`
	// NetIssueReduction is how many fewer problems the output reports than the
	// submitted document did. It is a net figure, not a count of issues
	// resolved: fixing one problem while introducing another nets to zero.
	// Only a RemainingIssues of length zero proves every problem was resolved.
	// Meaningful only when VerdictKnown is true.
	NetIssueReduction int `json:"netIssueReduction"`
	// VerdictKnown reports whether the output could be re-checked at all.
	VerdictKnown bool `json:"verdictKnown"`
	// DryRun reports that no verdict exists because nothing was applied, as
	// opposed to the output being unparseable. Both leave VerdictKnown false
	// and the page explains them differently.
	DryRun bool `json:"dryRun"`
	// AllFixTypesRun reports that every fix type was enabled. When false the
	// user selected a subset, so a remaining issue may simply have no selected
	// fix rather than no available one.
	AllFixTypesRun bool   `json:"allFixTypesRun"`
	Result         string `json:"result"`
	Format         string `json:"format"`
}

// parseVerdict summarizes the parse errors of the submitted document against
// those of the fixed output.
//
// It deliberately compares counts and reports the output's own issues, rather
// than pairing each input error with an output one. Several parser messages
// name the locations involved in whichever order the document's maps happened
// to iterate, so one defect is described two ways across two parses of the same
// bytes. Matching on message text therefore reports the input's phrasing as
// resolved and the output's as newly introduced when nothing changed at all.
type parseVerdict struct {
	submitted []string
	remaining []string
	// netReduction is len(submitted)-len(remaining), floored at zero. It is a
	// net figure: one issue resolved alongside one introduced nets to zero.
	netReduction int
	known        bool
	dryRun       bool
}

// FixEntry represents a single applied fix.
type FixEntry struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

func (h *Handler) handleFix(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Read input from any supported mode (file, paste, URL)
	input, errResp := h.readInput(r, "spec")
	if errResp != nil {
		return errResp
	}

	// Enrich metrics context
	format := detectFormat(input.Content)
	if ma := getMetricsAttrs(r.Context()); ma != nil {
		ma.enrich("fix", format, len(input.Content))
	}

	// Parse using oastools
	parseStart := time.Now()
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(input.Content))
	h.instruments.recordPackageDuration(r.Context(), "parser", parseStart)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse specification: %v", err))
	}

	// Record spec complexity
	stats := parser.GetDocumentStats(parseResult.Document)
	h.instruments.recordSpecComplexity(r.Context(), "fixer", parseResult.Version,
		stats.PathCount, stats.OperationCount, stats.SchemaCount, len(input.Content))

	// Configure fixer based on form options
	f := fixer.New()

	// Advanced options
	dryRun := r.FormValue("dryRun") == "on"
	inferTypes := r.FormValue("inferTypes") == "on"
	f.DryRun = dryRun
	f.InferTypes = inferTypes

	// EnabledFixes controls which fix types to apply
	// Setting to nil enables all fix types
	var enabledFixes []fixer.FixType

	// Add fix types based on checkboxes
	if r.FormValue("fixMissingParams") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeMissingPathParameter)
	}
	if r.FormValue("fixPathParamsNotRequired") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypePathParameterNotRequired)
	}
	if r.FormValue("removeUnusedSchemas") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypePrunedUnusedSchema)
	}
	if r.FormValue("fixInvalidNames") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeRenamedGenericSchema)
	}
	if r.FormValue("pruneEmptyPaths") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypePrunedEmptyPath)
	}
	if r.FormValue("fixDuplicateOperationIds") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeDuplicateOperationId)
	}
	if r.FormValue("expandCSVEnums") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeEnumCSVExpanded)
	}
	if r.FormValue("stubMissingRefs") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeStubMissingRef)
	}

	// If checkboxes are selected, apply only those fix types. Otherwise apply
	// every fix type: the fixer reads an empty slice as "all", whereas leaving
	// EnabledFixes alone would silently apply only fixer.DefaultEnabledFixes().
	allFixTypesRun := len(enabledFixes) == 0
	if allFixTypesRun {
		enabledFixes = []fixer.FixType{}
	} else if !dryRun {
		// Ownership of the document is only ours to give away once the fixer is
		// actually going to rewrite it. A dry run keeps the copy so the parsed
		// input survives intact, which matters because not every fix type in
		// the fixer honours DryRun.
		f.MutableInput = true
	}
	f.EnabledFixes = enabledFixes

	// Fix using parse-once pattern
	fixStart := time.Now()
	fixResult, err := f.FixParsed(*parseResult)
	h.instruments.recordPackageDuration(r.Context(), "fixer", fixStart)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "FIX_FAILED",
			fmt.Sprintf("fix operation failed: %v", err))
	}

	// Serialize result in original format
	output, err := serializeDocument(fixResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize fix result",
			"error", err,
			"format", format,
		)
		return h.renderError(r, http.StatusInternalServerError, "SERIALIZATION_FAILED",
			"failed to serialize fixed specification")
	}

	// Compare the submitted document's parse errors against the fixed output's,
	// so the page can say which of them fixing actually resolved. A dry run
	// leaves the document untouched, so it yields no verdict to report.
	verdict := h.classifyParseErrors(r.Context(), fixResult.ParseErrors, output, dryRun)

	// Build response
	result := h.buildFixResponse(parseResult, fixResult, output, format, verdict, allFixTypesRun)

	// Content negotiation
	if wantsHTML(r) {
		return h.renderHTML("fix-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildFixResponse(parseResult *parser.ParseResult, fixResult *fixer.FixResult, output, format string, verdict parseVerdict, allFixTypesRun bool) FixResponse {
	fixes := make([]FixEntry, 0, len(fixResult.Fixes))
	for _, fix := range fixResult.Fixes {
		fixes = append(fixes, FixEntry{
			Path:        fix.Path,
			Description: fix.Description,
			Type:        string(fix.Type),
		})
	}

	return FixResponse{
		Version:           parseResult.Version,
		Fixes:             fixes,
		SubmittedIssues:   verdict.submitted,
		RemainingIssues:   verdict.remaining,
		NetIssueReduction: verdict.netReduction,
		VerdictKnown:      verdict.known,
		DryRun:            verdict.dryRun,
		AllFixTypesRun:    allFixTypesRun,
		Result:            output,
		Format:            format,
	}
}

// classifyParseErrors summarizes what fixing resolved, by re-parsing the output
// and comparing how many problems it still reports against how many the
// submitted document had.
//
// It reports no per-issue verdict on purpose. The parser names the locations in
// several of its messages in map-iteration order, so the same defect is phrased
// two ways across two parses; pairing input messages against output ones then
// flaps between "resolved" and "newly introduced" for a document that never
// changed. Counts are stable under that reordering, and the output's own issue
// list is accurate for the document actually produced.
//
// The verdict is left unknown when dryRun is set (the document was not
// modified, so re-parsing would trivially report every input issue) or when the
// output cannot be re-parsed. Callers then report the submitted issues alone,
// and parseVerdict.dryRun tells the two cases apart so the page can say which
// happened.
func (h *Handler) classifyParseErrors(ctx context.Context, inputErrs []error, output string, dryRun bool) parseVerdict {
	verdict := parseVerdict{submitted: messages(inputErrs), dryRun: dryRun}
	if dryRun {
		return verdict
	}

	remaining, ok := h.reparseErrors(ctx, output)
	if !ok {
		return verdict
	}

	verdict.remaining = remaining
	verdict.known = true
	// A net figure, floored at zero. Fixing can introduce an issue as well as
	// clear one, so this understates rather than overstates what was resolved;
	// an empty remaining list is the only proof that everything was resolved.
	verdict.netReduction = max(len(verdict.submitted)-len(remaining), 0)

	return verdict
}

// messages renders errors for display, preserving the order they were reported.
func messages(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}

	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

// reparseErrors parses the serialized output and returns the parse errors it
// reports. The second result is false when the output could not be parsed at
// all, leaving the comparison unavailable rather than wrong.
func (h *Handler) reparseErrors(ctx context.Context, output string) ([]string, bool) {
	start := time.Now()
	result, err := parser.ParseWithOptions(parser.WithBytes([]byte(output)))
	h.instruments.recordPackageDuration(ctx, "parser", start)
	if err != nil || result == nil {
		slog.Warn("could not re-parse fixed output to verify parse errors", "error", err)
		return nil, false
	}

	return messages(result.Errors), true
}
