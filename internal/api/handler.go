package api

import (
	"html/template"
	"net/http"

	"github.com/erraggy/oastools-web/internal/config"
	"github.com/erraggy/oastools-web/internal/templates"
)

// Handler is the main HTTP handler for the application.
type Handler struct {
	cfg       *config.Config
	version   string
	templates *template.Template
	mux       *http.ServeMux
}

// NewHandler creates a new Handler with the given configuration.
func NewHandler(cfg *config.Config, version string) (*Handler, error) {
	tmpl, err := template.ParseFS(templates.FS, "*.html", "partials/*.html")
	if err != nil {
		return nil, err
	}

	h := &Handler{
		cfg:       cfg,
		version:   version,
		templates: tmpl,
		mux:       http.NewServeMux(),
	}

	h.registerRoutes()
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
