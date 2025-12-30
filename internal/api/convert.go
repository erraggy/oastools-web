package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/converter"
	"github.com/erraggy/oastools/overlay"
	"github.com/erraggy/oastools/parser"
)

// ConvertResponse represents the conversion result.
type ConvertResponse struct {
	SourceVersion string            `json:"sourceVersion"`
	TargetVersion string            `json:"targetVersion"`
	Issues        []ConversionIssue `json:"issues"`
	Result        string            `json:"result"`
	Format        string            `json:"format"`
}

// ConversionIssue represents a single conversion issue or warning.
type ConversionIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (h *Handler) handleConvert(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Read input from any supported mode (file, paste, URL)
	input, errResp := h.readInput(r, "spec")
	if errResp != nil {
		return errResp
	}

	// Get target version
	targetVersion := r.FormValue("target")
	if err := validateTargetVersion(targetVersion); err != nil {
		return h.renderError(r, http.StatusBadRequest, "INVALID_TARGET", err.Error())
	}

	// Parse options
	strict := r.FormValue("strict") == "on"
	_ = strict // TODO: Apply strict mode when library supports it

	// Read optional overlay files
	var preOverlay, postOverlay *overlay.Overlay
	if preFile, _, err := r.FormFile("preOverlay"); err == nil {
		defer func() { _ = preFile.Close() }()
		preContent, _ := io.ReadAll(preFile)
		if len(preContent) > 0 {
			preOverlay, err = overlay.ParseOverlay(preContent)
			if err != nil {
				return h.renderError(r, http.StatusBadRequest, "INVALID_PRE_OVERLAY",
					fmt.Sprintf("failed to parse pre-conversion overlay: %v", err))
			}
		}
	}
	if postFile, _, err := r.FormFile("postOverlay"); err == nil {
		defer func() { _ = postFile.Close() }()
		postContent, _ := io.ReadAll(postFile)
		if len(postContent) > 0 {
			postOverlay, err = overlay.ParseOverlay(postContent)
			if err != nil {
				return h.renderError(r, http.StatusBadRequest, "INVALID_POST_OVERLAY",
					fmt.Sprintf("failed to parse post-conversion overlay: %v", err))
			}
		}
	}

	// Parse using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(input.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse specification: %v", err))
	}

	// Build conversion options
	opts := []converter.Option{
		converter.WithParsed(*parseResult),
		converter.WithTargetVersion(targetVersion),
	}
	if preOverlay != nil {
		opts = append(opts, converter.WithPreConversionOverlay(preOverlay))
	}
	if postOverlay != nil {
		opts = append(opts, converter.WithPostConversionOverlay(postOverlay))
	}

	// Convert using options pattern
	convertResult, err := converter.ConvertWithOptions(opts...)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "CONVERSION_FAILED",
			fmt.Sprintf("conversion failed: %v", err))
	}

	// Serialize result in preferred format
	format := detectFormat(input.Content)
	output, err := serializeDocument(convertResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize conversion result",
			"error", err,
			"format", format,
		)
		return h.renderError(r, http.StatusInternalServerError, "SERIALIZATION_FAILED",
			"failed to serialize converted specification")
	}

	// Build response
	result := h.buildConvertResponse(parseResult, targetVersion, convertResult, output, format)

	// Content negotiation
	if wantsHTML(r) {
		return h.renderHTML("convert-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildConvertResponse(parseResult *parser.ParseResult, targetVersion string, convResult *converter.ConversionResult, output, format string) ConvertResponse {
	issues := make([]ConversionIssue, 0, len(convResult.Issues))
	for _, issue := range convResult.Issues {
		issues = append(issues, ConversionIssue{
			Path:    issue.Path,
			Message: issue.Message,
		})
	}

	return ConvertResponse{
		SourceVersion: parseResult.Version,
		TargetVersion: targetVersion,
		Issues:        issues,
		Result:        output,
		Format:        format,
	}
}

// validateTargetVersion checks if the target version is valid.
func validateTargetVersion(version string) error {
	switch version {
	case "2.0", "3.0", "3.0.0", "3.0.1", "3.0.2", "3.0.3", "3.1", "3.1.0", "3.2", "3.2.0":
		return nil
	default:
		return fmt.Errorf("unsupported version: %s (valid: 2.0, 3.0, 3.1, 3.2)", version)
	}
}

// detectFormat detects whether the content is JSON or YAML.
func detectFormat(content []byte) string {
	for _, b := range content {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return "json"
		default:
			return "yaml"
		}
	}
	return "yaml"
}
