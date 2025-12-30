package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"go.yaml.in/yaml/v4"
)

// htmlResponse implements builder.Response for HTML content.
type htmlResponse struct {
	status int
	html   string
}

func (r *htmlResponse) StatusCode() int      { return r.status }
func (r *htmlResponse) Headers() http.Header { return nil }
func (r *htmlResponse) Body() any            { return r.html }
func (r *htmlResponse) WriteTo(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(r.status)
	_, err := w.Write([]byte(r.html))
	return err
}

// renderHTML renders a partial template to an HTML response.
func (h *Handler) renderHTML(templateName string, data any) builder.Response {
	var buf bytes.Buffer
	if err := h.partials.ExecuteTemplate(&buf, templateName, data); err != nil {
		slog.Error("template execution failed",
			"template", templateName,
			"error", err,
		)
		return builder.Error(http.StatusInternalServerError, "template error")
	}
	return &htmlResponse{status: http.StatusOK, html: buf.String()}
}

// wantsHTML returns true if the request prefers HTML response (HTMX requests).
func wantsHTML(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// readFormFile reads a file from the multipart form and returns its contents.
// Returns nil content and an error response if the file cannot be read.
func readFormFile(r *http.Request, fieldName string) ([]byte, multipart.File, builder.Response) {
	file, _, err := r.FormFile(fieldName)
	if err != nil {
		return nil, nil, builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "MISSING_FILE",
				Message: fmt.Sprintf("%s file is required", fieldName),
			},
		})
	}

	content, err := io.ReadAll(file)
	if err != nil {
		file.Close()
		return nil, nil, builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "READ_FAILED",
				Message: fmt.Sprintf("failed to read %s file: %v", fieldName, err),
			},
		})
	}

	return content, file, nil
}

// serializeDocument serializes a document to JSON or YAML based on the format.
// Returns the serialized output and any error that occurred.
func serializeDocument(doc any, format string) (string, error) {
	var output []byte
	var err error
	if format == "json" {
		output, err = json.MarshalIndent(doc, "", "  ")
	} else {
		output, err = yaml.Marshal(doc)
	}
	if err != nil {
		return "", err
	}
	return string(output), nil
}
