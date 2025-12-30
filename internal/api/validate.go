package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/validator"
	"go.yaml.in/yaml/v4"
)

// ValidateResponse represents the validation result.
type ValidateResponse struct {
	Valid      bool              `json:"valid"`
	Version    string            `json:"version"`
	Errors     []ValidationIssue `json:"errors"`
	Warnings   []ValidationIssue `json:"warnings"`
	Statistics ValidationStats   `json:"statistics"`
}

// ValidationIssue represents a single validation error or warning.
type ValidationIssue struct {
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// ValidationStats contains document statistics.
type ValidationStats struct {
	Paths      int `json:"paths"`
	Operations int `json:"operations"`
	Schemas    int `json:"schemas"`
	Errors     int `json:"errors"`
	Warnings   int `json:"warnings"`
}

// ErrorResponse represents an API error.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information.
type ErrorDetail struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Details []ErrorDetail `json:"details,omitempty"`
}

func (h *Handler) handleValidate(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Read input from any supported mode (file, paste, URL)
	input, errResp := h.readInput(r, "spec")
	if errResp != nil {
		return errResp
	}

	// Parse options
	strict := r.FormValue("strict") == "on"
	includeWarnings := r.FormValue("includeWarnings") != "off" // Default to true (checked by default)

	// Parse using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(input.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse specification: %v", err))
	}

	// Validate using parse-once pattern
	v := validator.New()
	v.StrictMode = strict
	v.IncludeWarnings = includeWarnings
	validationResult, err := v.ValidateParsed(*parseResult)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "VALIDATION_FAILED",
			fmt.Sprintf("validation error: %v", err))
	}

	// Build response
	result := h.buildValidateResponse(parseResult, validationResult)

	// Content negotiation
	if wantsHTML(r) {
		return h.renderHTML("validation-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func (h *Handler) buildValidateResponse(parseResult *parser.ParseResult, valResult *validator.ValidationResult) ValidateResponse {
	errors := make([]ValidationIssue, 0, len(valResult.Errors))
	for _, e := range valResult.Errors {
		errors = append(errors, ValidationIssue{
			Path:     e.Path,
			Message:  e.Message,
			Severity: "error",
		})
	}

	warnings := make([]ValidationIssue, 0, len(valResult.Warnings))
	for _, w := range valResult.Warnings {
		warnings = append(warnings, ValidationIssue{
			Path:     w.Path,
			Message:  w.Message,
			Severity: "warning",
		})
	}

	return ValidateResponse{
		Valid:    valResult.Valid,
		Version:  valResult.Version,
		Errors:   errors,
		Warnings: warnings,
		Statistics: ValidationStats{
			Paths:      valResult.Stats.PathCount,
			Operations: valResult.Stats.OperationCount,
			Schemas:    valResult.Stats.SchemaCount,
			Errors:     valResult.ErrorCount,
			Warnings:   valResult.WarningCount,
		},
	}
}

func (h *Handler) handleSpec(_ context.Context, req *builder.Request) builder.Response {
	accept := req.HTTPRequest.Header.Get("Accept")

	// Return JSON if explicitly requested
	if strings.Contains(accept, "application/json") {
		return builder.JSON(http.StatusOK, h.server.Spec)
	}

	// Default to YAML
	yamlBytes, err := yaml.Marshal(h.server.Spec)
	if err != nil {
		return builder.Error(http.StatusInternalServerError, "failed to serialize spec")
	}

	return builder.NewResponse(http.StatusOK).
		Binary("application/x-yaml", yamlBytes)
}
