package api

import (
	"context"
	"fmt"
	"io"
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

	// Get base file from multipart form
	baseFile, _, err := r.FormFile("base")
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "MISSING_FILE", "base spec file is required")
	}
	defer baseFile.Close()

	baseContent, err := io.ReadAll(baseFile)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "READ_FAILED",
			fmt.Sprintf("failed to read base file: %v", err))
	}

	// Get head file from multipart form
	headFile, _, err := r.FormFile("head")
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "MISSING_FILE", "head spec file is required")
	}
	defer headFile.Close()

	headContent, err := io.ReadAll(headFile)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "READ_FAILED",
			fmt.Sprintf("failed to read head file: %v", err))
	}

	// Parse both specifications
	baseResult, err := parser.ParseWithOptions(parser.WithBytes(baseContent))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse base specification: %v", err))
	}

	headResult, err := parser.ParseWithOptions(parser.WithBytes(headContent))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse head specification: %v", err))
	}

	// Diff using parse-once pattern with breaking change mode
	d := differ.New()
	d.Mode = differ.ModeBreaking
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
			Severity: string(change.Severity),
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
