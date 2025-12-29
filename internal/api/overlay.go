package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/overlay"
	"github.com/erraggy/oastools/parser"
	"go.yaml.in/yaml/v4"
)

// OverlayResponse represents the overlay application result.
type OverlayResponse struct {
	Version        string          `json:"version"`
	ActionsApplied int             `json:"actionsApplied"`
	ActionsSkipped int             `json:"actionsSkipped"`
	Changes        []OverlayChange `json:"changes"`
	Result         string          `json:"result"`
	Format         string          `json:"format"`
}

// OverlayChange represents a single overlay change that was applied.
type OverlayChange struct {
	Target     string `json:"target"`
	Operation  string `json:"operation"`
	MatchCount int    `json:"matchCount"`
}

func (h *Handler) handleOverlay(_ context.Context, req *builder.Request) builder.Response {
	// Get spec file from multipart form
	specFile, _, err := req.HTTPRequest.FormFile("spec")
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "MISSING_FILE",
				Message: "spec file is required",
			},
		})
	}
	defer specFile.Close()

	specContent, err := io.ReadAll(specFile)
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "READ_FAILED",
				Message: fmt.Sprintf("failed to read spec file: %v", err),
			},
		})
	}

	// Get overlay file from multipart form
	overlayFile, _, err := req.HTTPRequest.FormFile("overlay")
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "MISSING_FILE",
				Message: "overlay file is required",
			},
		})
	}
	defer overlayFile.Close()

	overlayContent, err := io.ReadAll(overlayFile)
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "READ_FAILED",
				Message: fmt.Sprintf("failed to read overlay file: %v", err),
			},
		})
	}

	// Parse spec using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(specContent))
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "PARSE_FAILED",
				Message: fmt.Sprintf("failed to parse specification: %v", err),
			},
		})
	}

	// Parse overlay document
	overlayDoc, err := overlay.ParseOverlay(overlayContent)
	if err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "OVERLAY_PARSE_FAILED",
				Message: fmt.Sprintf("failed to parse overlay document: %v", err),
			},
		})
	}

	// Apply overlay using parse-once pattern
	applier := overlay.NewApplier()
	applyResult, err := applier.ApplyParsed(parseResult, overlayDoc)
	if err != nil {
		return builder.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: ErrorDetail{
				Code:    "OVERLAY_FAILED",
				Message: fmt.Sprintf("overlay application failed: %v", err),
			},
		})
	}

	// Serialize result in original format
	format := detectFormat(specContent)
	var output []byte
	if format == "json" {
		output, _ = json.MarshalIndent(applyResult.Document, "", "  ")
	} else {
		output, _ = yaml.Marshal(applyResult.Document)
	}

	// Build response
	result := h.buildOverlayResponse(parseResult, applyResult, string(output), format)

	// Content negotiation
	if wantsHTML(req.HTTPRequest) {
		return h.renderHTML("overlay-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildOverlayResponse(parseResult *parser.ParseResult, applyResult *overlay.ApplyResult, output, format string) OverlayResponse {
	changes := make([]OverlayChange, 0, len(applyResult.Changes))
	for _, change := range applyResult.Changes {
		changes = append(changes, OverlayChange{
			Target:     change.Target,
			Operation:  change.Operation,
			MatchCount: change.MatchCount,
		})
	}

	return OverlayResponse{
		Version:        parseResult.Version,
		ActionsApplied: applyResult.ActionsApplied,
		ActionsSkipped: applyResult.ActionsSkipped,
		Changes:        changes,
		Result:         output,
		Format:         format,
	}
}
