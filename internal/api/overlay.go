package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/overlay"
	"github.com/erraggy/oastools/parser"
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

// maxOverlaySize is the maximum size for overlay files (500KB).
const maxOverlaySize = 500 * 1024

func (h *Handler) handleOverlay(_ context.Context, req *builder.Request) builder.Response {
	// Get spec file from multipart form
	specContent, specFile, errResp := readFormFile(req.HTTPRequest, "spec")
	if errResp != nil {
		return errResp
	}
	defer specFile.Close()

	// Get overlay file from multipart form
	overlayContent, overlayFile, errResp := readFormFile(req.HTTPRequest, "overlay")
	if errResp != nil {
		return errResp
	}
	defer overlayFile.Close()

	// Validate overlay file size (500KB limit)
	if len(overlayContent) > maxOverlaySize {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "FILE_TOO_LARGE",
				Message: "overlay file exceeds 500KB limit",
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
	output, err := serializeDocument(applyResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize overlay result",
			"error", err,
			"format", format,
		)
		return builder.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Code:    "SERIALIZATION_FAILED",
				Message: "failed to serialize overlay result",
			},
		})
	}

	// Build response
	result := h.buildOverlayResponse(parseResult, applyResult, output, format)

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
