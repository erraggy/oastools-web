package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
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

// errorTemplateData holds data for rendering the error template.
type errorTemplateData struct {
	Message string
	Details string
}

// renderError returns an error response with content negotiation.
// For HTMX requests, renders the error template; otherwise returns JSON.
func (h *Handler) renderError(r *http.Request, status int, code, message string) builder.Response {
	if wantsHTML(r) {
		var buf bytes.Buffer
		if err := h.partials.ExecuteTemplate(&buf, "error", errorTemplateData{
			Message: message,
		}); err != nil {
			slog.Error("error template execution failed", "error", err)
			return builder.Error(status, message)
		}
		return &htmlResponse{status: status, html: buf.String()}
	}
	return builder.JSON(status, ErrorResponse{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// wantsHTML returns true if the request prefers HTML response (HTMX requests).
func wantsHTML(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
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
