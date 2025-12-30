package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/erraggy/oastools/builder"
)

// InputSource represents content from any input mode (file, paste, URL).
type InputSource struct {
	Content  []byte
	Filename string // For file mode, the uploaded filename; for URL mode, derived from URL
	Mode     string // "file", "paste", or "url"
}

// readInput reads spec content from any supported input mode.
// It checks the input_mode form field to determine which source to use.
// The fieldName is the base name for the form fields (e.g., "spec" for spec, spec_content, spec_url).
//
// Mode detection precedence:
//  1. {fieldName}_mode - field-specific mode (e.g., "base_mode" for diff base spec)
//  2. input_mode - generic mode (used for single-input operations like validate)
//  3. "file" - default fallback
//
// This allows operations with multiple inputs (like diff) to use different modes
// for each input, while single-input operations can use the simpler input_mode field.
func (h *Handler) readInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
	// Check field-specific mode first (e.g., "base_mode" for diff operations),
	// then fall back to generic "input_mode" for single-input operations
	mode := r.FormValue(fieldName + "_mode")
	if mode == "" {
		mode = r.FormValue("input_mode")
	}
	if mode == "" {
		mode = "file" // default
	}

	switch mode {
	case "file":
		return h.readFileInput(r, fieldName)
	case "paste":
		return h.readPasteInput(r, fieldName)
	case "url":
		return h.readURLInput(r, fieldName)
	default:
		return nil, h.renderError(r, http.StatusBadRequest, "INVALID_MODE",
			fmt.Sprintf("invalid input mode: %s", mode))
	}
}

// readFileInput reads content from a file upload.
func (h *Handler) readFileInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
	file, header, err := r.FormFile(fieldName)
	if err != nil {
		return nil, h.renderError(r, http.StatusBadRequest, "MISSING_FILE",
			fmt.Sprintf("%s file is required", fieldName))
	}
	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, h.renderError(r, http.StatusBadRequest, "READ_FAILED",
			fmt.Sprintf("failed to read %s file: %v", fieldName, err))
	}

	if len(content) == 0 {
		return nil, h.renderError(r, http.StatusBadRequest, "EMPTY_FILE",
			fmt.Sprintf("%s file is empty", fieldName))
	}

	return &InputSource{
		Content:  content,
		Filename: header.Filename,
		Mode:     "file",
	}, nil
}

// readPasteInput reads content from a pasted textarea.
func (h *Handler) readPasteInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
	content := r.FormValue(fieldName + "_content")
	if content == "" {
		return nil, h.renderError(r, http.StatusBadRequest, "MISSING_CONTENT",
			fmt.Sprintf("%s content is required", fieldName))
	}

	// Check size limit (same as file upload limit)
	if int64(len(content)) > h.cfg.MaxFileSize {
		return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE",
			fmt.Sprintf("content exceeds maximum size of %d bytes", h.cfg.MaxFileSize))
	}

	return &InputSource{
		Content:  []byte(content),
		Filename: "pasted-spec",
		Mode:     "paste",
	}, nil
}

// readURLInput fetches content from a URL.
func (h *Handler) readURLInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
	rawURL := r.FormValue(fieldName + "_url")
	if rawURL == "" {
		return nil, h.renderError(r, http.StatusBadRequest, "MISSING_URL",
			fmt.Sprintf("%s URL is required", fieldName))
	}

	content, err := h.urlFetcher.Fetch(r.Context(), rawURL)
	if err != nil {
		return nil, h.renderError(r, http.StatusBadRequest, "FETCH_FAILED",
			fmt.Sprintf("failed to fetch URL: %v", err))
	}

	// Extract filename from URL path
	filename := extractFilenameFromURL(rawURL)

	return &InputSource{
		Content:  content,
		Filename: filename,
		Mode:     "url",
	}, nil
}

// extractFilenameFromURL extracts a filename from a URL path, falling back to "remote-spec".
func extractFilenameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "remote-spec"
	}

	// Paths ending with "/" are directories, not files
	if parsed.Path == "" || parsed.Path == "/" || parsed.Path[len(parsed.Path)-1] == '/' {
		return "remote-spec"
	}

	// Get the base name from the path (excludes query string automatically)
	filename := path.Base(parsed.Path)

	// path.Base returns "." for empty paths
	if filename == "" || filename == "." {
		return "remote-spec"
	}

	return filename
}
