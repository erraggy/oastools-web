package api

import (
	"net/http"

	"github.com/erraggy/oastools/builder"
)

// registerOperations adds all API operations to the ServerBuilder.
func (h *Handler) registerOperations(srv *builder.ServerBuilder) {
	// Health check
	srv.AddOperation(http.MethodGet, "/health",
		builder.WithOperationID("getHealth"),
		builder.WithSummary("Health check endpoint"),
		builder.WithTags("system"),
		builder.WithResponse(http.StatusOK, HealthResponse{},
			builder.WithResponseDescription("Service health status"),
		),
		builder.WithHandler(h.handleHealth),
	)

	// GET /api/spec - returns this API's OpenAPI specification
	srv.AddOperation(http.MethodGet, "/api/spec",
		builder.WithOperationID("getAPISpec"),
		builder.WithSummary("Get this API's OpenAPI specification"),
		builder.WithTags("system"),
		builder.WithResponse(http.StatusOK, nil,
			builder.WithResponseDescription("OpenAPI 3.2 specification in YAML or JSON"),
		),
		builder.WithHandler(h.handleSpec),
	)

	// POST /api/validate - validate an OpenAPI specification
	srv.AddOperation(http.MethodPost, "/api/validate",
		builder.WithOperationID("validateSpec"),
		builder.WithSummary("Validate an OpenAPI specification"),
		builder.WithDescription("Validates an OpenAPI specification and returns a detailed validation report with errors and warnings."),
		builder.WithTags("operations"),
		builder.WithFileParam("spec",
			builder.WithParamDescription("OpenAPI specification file (JSON or YAML)"),
			builder.WithParamRequired(true),
		),
		builder.WithResponse(http.StatusOK, ValidateResponse{},
			builder.WithResponseDescription("Validation result"),
		),
		builder.WithResponse(http.StatusBadRequest, ErrorResponse{},
			builder.WithResponseDescription("Invalid request or unparseable file"),
		),
		builder.WithHandler(h.handleValidate),
	)
}
