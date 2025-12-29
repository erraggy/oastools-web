package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/fixer"
	"github.com/erraggy/oastools/parser"
)

// FixResponse represents the fix result.
type FixResponse struct {
	Version string     `json:"version"`
	Fixes   []FixEntry `json:"fixes"`
	Result  string     `json:"result"`
	Format  string     `json:"format"`
}

// FixEntry represents a single applied fix.
type FixEntry struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Type        string `json:"type"`
}

func (h *Handler) handleFix(_ context.Context, req *builder.Request) builder.Response {
	// Get file from multipart form
	content, file, errResp := readFormFile(req.HTTPRequest, "spec")
	if errResp != nil {
		return errResp
	}
	defer file.Close()

	// Parse using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(content))
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "PARSE_FAILED",
				Message: fmt.Sprintf("failed to parse specification: %v", err),
			},
		})
	}

	// Configure fixer based on form options
	f := fixer.New()

	// EnabledFixes controls which fix types to apply
	// Setting to nil enables all fix types
	var enabledFixes []fixer.FixType

	// Add fix types based on checkboxes
	if req.HTTPRequest.FormValue("fixMissingParams") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeMissingPathParameter)
	}
	if req.HTTPRequest.FormValue("removeUnusedSchemas") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypePrunedUnusedSchema)
	}
	if req.HTTPRequest.FormValue("fixInvalidNames") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypeRenamedGenericSchema)
	}
	if req.HTTPRequest.FormValue("pruneEmptyPaths") == "on" {
		enabledFixes = append(enabledFixes, fixer.FixTypePrunedEmptyPath)
	}

	// If checkboxes selected, use only those fix types; otherwise nil enables all
	if len(enabledFixes) > 0 {
		f.EnabledFixes = enabledFixes
	}

	// Fix using parse-once pattern
	fixResult, err := f.FixParsed(*parseResult)
	if err != nil {
		return builder.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: ErrorDetail{
				Code:    "FIX_FAILED",
				Message: fmt.Sprintf("fix operation failed: %v", err),
			},
		})
	}

	// Serialize result in original format
	format := detectFormat(content)
	output, err := serializeDocument(fixResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize fix result",
			"error", err,
			"format", format,
		)
		return builder.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Code:    "SERIALIZATION_FAILED",
				Message: "failed to serialize fixed specification",
			},
		})
	}

	// Build response
	result := h.buildFixResponse(parseResult, fixResult, output, format)

	// Content negotiation
	if wantsHTML(req.HTTPRequest) {
		return h.renderHTML("fix-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildFixResponse(parseResult *parser.ParseResult, fixResult *fixer.FixResult, output, format string) FixResponse {
	fixes := make([]FixEntry, 0, len(fixResult.Fixes))
	for _, fix := range fixResult.Fixes {
		fixes = append(fixes, FixEntry{
			Path:        fix.Path,
			Description: fix.Description,
			Type:        string(fix.Type),
		})
	}

	return FixResponse{
		Version: parseResult.Version,
		Fixes:   fixes,
		Result:  output,
		Format:  format,
	}
}
