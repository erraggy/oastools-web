package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/fixer"
	"github.com/erraggy/oastools/parser"
	"go.yaml.in/yaml/v4"
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
	file, _, err := req.HTTPRequest.FormFile("spec")
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "MISSING_FILE",
				Message: "spec file is required",
			},
		})
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "READ_FAILED",
				Message: fmt.Sprintf("failed to read file: %v", err),
			},
		})
	}

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
	// Default is only FixTypeMissingPathParameter for performance
	// Setting to nil enables all fix types
	enabledFixes := []fixer.FixType{}

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

	// If no checkboxes selected, enable all fixes
	if len(enabledFixes) == 0 {
		f.EnabledFixes = nil // nil enables all fix types
	} else {
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
	var output []byte
	if format == "json" {
		output, _ = json.MarshalIndent(fixResult.Document, "", "  ")
	} else {
		output, _ = yaml.Marshal(fixResult.Document)
	}

	// Build response
	result := h.buildFixResponse(parseResult, fixResult, string(output), format)

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
