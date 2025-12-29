package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/converter"
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
	// Get file from multipart form
	content, file, errResp := readFormFile(req.HTTPRequest, "spec")
	if errResp != nil {
		return errResp
	}
	defer file.Close()

	// Get target version
	targetVersion := req.HTTPRequest.FormValue("target")
	if err := validateTargetVersion(targetVersion); err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "INVALID_TARGET",
				Message: err.Error(),
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

	// Convert using parse-once pattern
	c := converter.New()
	convertResult, err := c.ConvertParsed(*parseResult, targetVersion)
	if err != nil {
		return builder.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: ErrorDetail{
				Code:    "CONVERSION_FAILED",
				Message: fmt.Sprintf("conversion failed: %v", err),
			},
		})
	}

	// Serialize result in preferred format
	format := detectFormat(content)
	output, err := serializeDocument(convertResult.Document, format)
	if err != nil {
		slog.Error("failed to serialize conversion result",
			"error", err,
			"format", format,
		)
		return builder.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Code:    "SERIALIZATION_FAILED",
				Message: "failed to serialize converted specification",
			},
		})
	}

	// Build response
	result := h.buildConvertResponse(parseResult, targetVersion, convertResult, output, format)

	// Content negotiation
	if wantsHTML(req.HTTPRequest) {
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
