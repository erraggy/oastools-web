package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/erraggy/oastools-web/static/examples"
	"github.com/erraggy/oastools/builder"
)

// ExampleMetadata describes an available example spec.
type ExampleMetadata struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// handleGetExample serves an example spec by name.
func (h *Handler) handleGetExample(_ context.Context, req *builder.Request) builder.Response {
	nameAny := req.PathParams["name"]
	name, ok := nameAny.(string)
	if !ok || name == "" {
		return builder.Error(http.StatusBadRequest, "missing example name")
	}

	// Sanitize: only allow alphanumeric, dash, underscore, and dot
	// Dots are needed for version-like names (e.g., petstore-3.0)
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isAllowed := isLower || isUpper || isDigit || r == '-' || r == '_' || r == '.'
		if !isAllowed {
			return builder.Error(http.StatusBadRequest, "invalid example name")
		}
	}

	filename := name + ".yaml"
	content, err := examples.FS.ReadFile(filename)
	if err != nil {
		return builder.Error(http.StatusNotFound, "example not found")
	}

	return builder.NewResponse(http.StatusOK).Binary("text/yaml; charset=utf-8", content)
}

// handleListExamples returns metadata for all available examples.
func (h *Handler) handleListExamples(_ context.Context, _ *builder.Request) builder.Response {
	entries, err := examples.FS.ReadDir(".")
	if err != nil {
		return builder.Error(http.StatusInternalServerError, "failed to read examples")
	}

	list := make([]ExampleMetadata, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		list = append(list, ExampleMetadata{
			Name:  name,
			Label: exampleLabel(name),
		})
	}

	return builder.JSON(http.StatusOK, list)
}

// exampleLabel returns a human-readable label for an example name.
func exampleLabel(name string) string {
	labels := map[string]string{
		"petstore-3.0":         "Petstore (Clean)",
		"petstore-2.0":         "Petstore 2.0",
		"petstore-warnings":    "Petstore (With Warnings)",
		"petstore-errors":      "Petstore (With Errors)",
		"petstore-v2":          "Petstore v2 (Safe Changes)",
		"petstore-v3":          "Petstore v3 (Breaking Changes)",
		"petstore-messy":       "Petstore (Messy)",
		"petstore-full":        "Petstore (Full Featured)",
		"users-api":            "Users API",
		"products-api":         "Products API",
		"orders-api":           "Orders API",
		"inventory-api":        "Inventory API",
		"overlay-descriptions": "Add Descriptions",
		"overlay-security":     "Add Security",
		"overlay-public":       "Public API",
	}
	if label, ok := labels[name]; ok {
		return label
	}
	return name
}
