package api

import (
	"bytes"
	"net/http"

	"github.com/erraggy/oastools/builder"
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

// renderHTML renders a template to an HTML response.
func (h *Handler) renderHTML(templateName string, data any) builder.Response {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return builder.Error(http.StatusInternalServerError, "template error")
	}
	return &htmlResponse{status: http.StatusOK, html: buf.String()}
}

// wantsHTML returns true if the request prefers HTML response.
// This is determined by the HX-Request header (HTMX requests) or Accept header.
func wantsHTML(r *http.Request) bool {
	if r.Header.Get("HX-Request") == "true" {
		return true
	}
	accept := r.Header.Get("Accept")
	// Very simple check - if explicitly asking for JSON, don't return HTML
	if accept == "application/json" {
		return false
	}
	return false // Default to JSON for API endpoints
}
