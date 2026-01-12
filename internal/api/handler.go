package api

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/erraggy/oastools-web/internal/config"
	"github.com/erraggy/oastools-web/internal/templates"
	"github.com/erraggy/oastools-web/static"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
)

// Handler is the main HTTP handler for the application.
type Handler struct {
	cfg             *config.Config
	version         string
	oastoolsVersion string
	templates       map[string]*template.Template // page name -> cloned template with that page's blocks
	partials        *template.Template            // shared partials for result rendering
	rateLimiter     *RateLimiter
	urlFetcher      *URLFetcher
	server          *builder.ServerResult
	handler         http.Handler
	staticFS        http.Handler
}

// NewHandler creates a new Handler with the given configuration.
func NewHandler(cfg *config.Config, version string) (*Handler, error) {
	// Parse base template
	base, err := template.ParseFS(templates.FS, "base.html")
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}

	// Parse partials for result rendering
	partials, err := template.ParseFS(templates.FS, "partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse partials: %w", err)
	}

	// Parse each page template into a cloned base to avoid block collisions
	pageTemplates := make(map[string]*template.Template)
	pages := []string{"index.html", "validate.html", "convert.html", "diff.html", "fix.html", "join.html", "overlay.html", "explore.html"}
	for _, page := range pages {
		// Clone base template so each page gets its own copy
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone base for %s: %w", page, err)
		}
		// Parse page template into the clone
		_, err = clone.ParseFS(templates.FS, page)
		if err != nil {
			return nil, fmt.Errorf("parse page %s: %w", page, err)
		}
		pageTemplates[page] = clone
	}

	staticSub, err := fs.Sub(static.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to access static files: %w", err)
	}

	// Wrap static file server with caching headers (1 year cache, immutable)
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticSub)))
	cachedStaticHandler := StaticCache(365 * 24 * time.Hour)(staticHandler)

	oastoolsVersion := getOASToolsVersion()

	h := &Handler{
		cfg:             cfg,
		version:         version,
		oastoolsVersion: oastoolsVersion,
		templates:       pageTemplates,
		partials:        partials,
		rateLimiter:     NewRateLimiter(cfg.RateLimitRPM, cfg.RateLimitBurst),
		urlFetcher:      NewURLFetcher(version, oastoolsVersion),
		staticFS:        cachedStaticHandler,
	}

	srv := h.buildServer()
	result, err := srv.BuildServer()
	if err != nil {
		return nil, fmt.Errorf("build server: %w", err)
	}
	h.server = result

	h.buildMiddlewareChain()
	return h, nil
}

// buildServer creates the ServerBuilder with all API operations.
func (h *Handler) buildServer() *builder.ServerBuilder {
	srv := builder.NewServerBuilder(parser.OASVersion320, builder.WithoutValidation()).
		SetTitle("oastools API").
		SetVersion(h.version).
		SetDescription("OpenAPI specification toolkit - validate, convert, diff, fix, join, and overlay specs")

	h.registerOperations(srv)

	return srv
}

// buildMiddlewareChain wraps the composite handler with middleware (outermost first).
func (h *Handler) buildMiddlewareChain() {
	// Build chain from inside out (last applied = outermost)
	var handler http.Handler = http.HandlerFunc(h.route)

	// Size limit (innermost - applied last)
	handler = RequestSizeLimiter(h.cfg.MaxFileSize)(handler)

	// Timeout
	handler = Timeout(h.cfg.RequestTimeout)(handler)

	// Concurrency limit (global)
	handler = ConcurrencyLimiter(h.cfg.MaxConcurrentRequests)(handler)

	// Rate limiting (per-IP)
	handler = h.rateLimiter.Middleware(handler)

	// Recovery (catches panics)
	handler = Recovery(handler)

	// Logging (outermost - logs everything including errors)
	handler = Logging(handler)

	h.handler = handler
}

// route dispatches requests to the appropriate handler.
func (h *Handler) route(w http.ResponseWriter, r *http.Request) {
	// Static files
	if strings.HasPrefix(r.URL.Path, "/static/") {
		h.staticFS.ServeHTTP(w, r)
		return
	}

	// API routes via ServerBuilder
	if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
		h.server.Handler.ServeHTTP(w, r)
		return
	}

	// HTML pages
	h.servePage(w, r)
}

// servePage renders HTML pages for non-API routes.
func (h *Handler) servePage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Version":         h.version,
		"OastoolsVersion": h.oastoolsVersion,
	}

	var templateName string
	switch r.URL.Path {
	case "/":
		templateName = "index.html"
	case "/validate":
		templateName = "validate.html"
	case "/convert":
		templateName = "convert.html"
	case "/diff":
		templateName = "diff.html"
	case "/fix":
		templateName = "fix.html"
	case "/join":
		templateName = "join.html"
	case "/overlay":
		templateName = "overlay.html"
	case "/explore":
		templateName = "explore.html"
	default:
		http.NotFound(w, r)
		return
	}

	// Get the page-specific template (each page has its own cloned base)
	tmpl, ok := h.templates[templateName]
	if !ok {
		slog.Error("template not found", "template", templateName)
		http.Error(w, "template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Execute the page template - it calls {{template "base" .}} which uses this page's block definitions
	if err := tmpl.ExecuteTemplate(w, templateName, data); err != nil {
		slog.Error("page template execution failed",
			"template", templateName,
			"path", r.URL.Path,
			"error", err,
		)
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}

// Stop performs cleanup for graceful shutdown.
func (h *Handler) Stop() {
	h.rateLimiter.Stop()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.handler.ServeHTTP(w, r)
}
