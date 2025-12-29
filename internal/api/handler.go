package api

import (
	"html/template"
	"net/http"

	"github.com/erraggy/oastools-web/internal/config"
	"github.com/erraggy/oastools-web/internal/templates"
)

// Handler is the main HTTP handler for the application.
type Handler struct {
	cfg         *config.Config
	version     string
	templates   *template.Template
	mux         *http.ServeMux
	rateLimiter *RateLimiter
	handler     http.Handler
}

// NewHandler creates a new Handler with the given configuration.
func NewHandler(cfg *config.Config, version string) (*Handler, error) {
	tmpl, err := template.ParseFS(templates.FS, "*.html", "partials/*.html")
	if err != nil {
		return nil, err
	}

	h := &Handler{
		cfg:         cfg,
		version:     version,
		templates:   tmpl,
		mux:         http.NewServeMux(),
		rateLimiter: NewRateLimiter(cfg.RateLimitRPM, cfg.RateLimitBurst),
	}

	h.registerRoutes()
	h.buildMiddlewareChain()
	return h, nil
}

// buildMiddlewareChain wraps the mux with middleware (outermost first).
func (h *Handler) buildMiddlewareChain() {
	// Build chain from inside out (last applied = outermost)
	var handler http.Handler = h.mux

	// Size limit (innermost - applied last)
	handler = RequestSizeLimiter(h.cfg.MaxFileSize)(handler)

	// Timeout
	handler = Timeout(h.cfg.RequestTimeout)(handler)

	// Rate limiting
	handler = h.rateLimiter.Middleware(handler)

	// Recovery (catches panics)
	handler = Recovery(handler)

	// Logging (outermost - logs everything including errors)
	handler = Logging(handler)

	h.handler = handler
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}
