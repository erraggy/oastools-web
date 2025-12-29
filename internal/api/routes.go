package api

import (
	"io/fs"
	"net/http"

	"github.com/erraggy/oastools-web/static"
)

func (h *Handler) registerRoutes() {
	// Static files
	staticFS, _ := fs.Sub(static.FS, ".")
	h.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Pages
	h.mux.HandleFunc("GET /", h.handleIndex)
	h.mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Version": h.version,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
