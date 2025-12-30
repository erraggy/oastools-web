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
	r := req.HTTPRequest

	// Read spec input from any supported mode
	specInput, errResp := h.readInput(r, "spec")
	if errResp != nil {
		return errResp
	}

	// Read overlay input with 500KB limit (stricter than default for overlays)
	overlayInput, errResp := h.readInputWithLimit(r, "overlay", maxOverlaySize)
	if errResp != nil {
		return errResp
	}

	// Parse spec using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(specInput.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse specification: %v", err))
	}

	// Parse overlay document
	overlayDoc, err := overlay.ParseOverlay(overlayInput.Content)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "OVERLAY_PARSE_FAILED",
			fmt.Sprintf("failed to parse overlay document: %v", err))
	}

	// Apply overlay using parse-once pattern
	applier := overlay.NewApplier()
	applyResult, err := applier.ApplyParsed(parseResult, overlayDoc)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "OVERLAY_FAILED",
			fmt.Sprintf("overlay application failed: %v", err))
	}

	// Serialize result in original format
	format := detectFormat(specInput.Content)
	output, err := serializeDocument(applyResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize overlay result",
			"error", err,
			"format", format,
		)
		return h.renderError(r, http.StatusInternalServerError, "SERIALIZATION_FAILED",
			"failed to serialize overlay result")
	}

	// Build response
	result := h.buildOverlayResponse(parseResult, applyResult, output, format)

	// Content negotiation
	if wantsHTML(r) {
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
