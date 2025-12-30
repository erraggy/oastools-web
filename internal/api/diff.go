package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/differ"
	"github.com/erraggy/oastools/parser"
)

// DiffResponse represents the diff result.
type DiffResponse struct {
	BaseVersion string       `json:"baseVersion"`
	HeadVersion string       `json:"headVersion"`
	Summary     DiffSummary  `json:"summary"`
	Changes     []DiffChange `json:"changes"`
}

// DiffSummary contains summary statistics of the diff.
type DiffSummary struct {
	Total    int `json:"total"`
	Breaking int `json:"breaking"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

// DiffChange represents a single change in the diff.
type DiffChange struct {
	Path     string `json:"path"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func (h *Handler) handleDiff(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Read base input from any supported mode
	baseInput, errResp := h.readInput(r, "base")
	if errResp != nil {
		return errResp
	}

	// Read head input from any supported mode
	headInput, errResp := h.readInput(r, "head")
	if errResp != nil {
		return errResp
	}

	// Parse options
	modeStr := r.FormValue("mode")
	mode := differ.ModeBreaking
	if modeStr == "simple" {
		mode = differ.ModeSimple
	}
	includeInfo := r.FormValue("includeInfo") != "off"
	_ = includeInfo // TODO: Apply includeInfo when library supports it

	// Parse both specifications
	baseResult, err := parser.ParseWithOptions(parser.WithBytes(baseInput.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse base specification: %v", err))
	}

	headResult, err := parser.ParseWithOptions(parser.WithBytes(headInput.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse head specification: %v", err))
	}

	// Diff using parse-once pattern
	d := differ.New()
	d.Mode = mode
	diffResult, err := d.DiffParsed(*baseResult, *headResult)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "DIFF_FAILED",
			fmt.Sprintf("diff operation failed: %v", err))
	}

	// Build response
	result := h.buildDiffResponse(baseResult, headResult, diffResult)

	// Content negotiation
	if wantsHTML(r) {
		return h.renderHTML("diff-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildDiffResponse(baseResult, headResult *parser.ParseResult, diffResult *differ.DiffResult) DiffResponse {
	changes := make([]DiffChange, 0, len(diffResult.Changes))

	for _, change := range diffResult.Changes {
		changes = append(changes, DiffChange{
			Path:     change.Path,
			Type:     string(change.Type),
			Category: string(change.Category),
			Severity: change.Severity.String(),
			Message:  change.Message,
		})
	}

	return DiffResponse{
		BaseVersion: baseResult.Version,
		HeadVersion: headResult.Version,
		Summary: DiffSummary{
			Total:    len(diffResult.Changes),
			Breaking: diffResult.BreakingCount,
			Warnings: diffResult.WarningCount,
			Info:     diffResult.InfoCount,
		},
		Changes: changes,
	}
}
