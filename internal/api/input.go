package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/erraggy/oastools/builder"
)

const defaultRemoteFilename = "remote-spec"

// InputSource represents content from any input mode (file, paste, URL).
type InputSource struct {
	Content  []byte
	Filename string // file: uploaded filename; paste: "pasted-spec"; url: derived from URL path
	Mode     string // "file", "paste", or "url"
}

// readInput reads spec content from any supported input mode using the default MaxFileSize limit.
// See readInputWithLimit for custom size limits.
func (h *Handler) readInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
	return h.readInputWithLimit(r, fieldName, h.cfg.MaxFileSize)
}

// readInputWithLimit reads spec content from any supported input mode with a custom size limit.
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
func (h *Handler) readInputWithLimit(r *http.Request, fieldName string, maxSize int64) (*InputSource, builder.Response) {
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
		return h.readFileInputWithLimit(r, fieldName, maxSize)
	case "paste":
		return h.readPasteInputWithLimit(r, fieldName, maxSize)
	case "url":
		return h.readURLInputWithLimit(r, fieldName, maxSize)
	default:
		return nil, h.renderError(r, http.StatusBadRequest, "INVALID_MODE",
			fmt.Sprintf("invalid input mode: %s", mode))
	}
}

// readFileInputWithLimit reads content from a file upload with a custom size limit.
func (h *Handler) readFileInputWithLimit(r *http.Request, fieldName string, maxSize int64) (*InputSource, builder.Response) {
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

	// Check size limit
	if int64(len(content)) > maxSize {
		return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
			fmt.Sprintf("file exceeds maximum size of %d bytes", maxSize))
	}

	return &InputSource{
		Content:  content,
		Filename: header.Filename,
		Mode:     "file",
	}, nil
}

// readPasteInputWithLimit reads content from a pasted textarea with a custom size limit.
func (h *Handler) readPasteInputWithLimit(r *http.Request, fieldName string, maxSize int64) (*InputSource, builder.Response) {
	content := r.FormValue(fieldName + "_content")
	if content == "" {
		return nil, h.renderError(r, http.StatusBadRequest, "MISSING_CONTENT",
			fmt.Sprintf("%s content is required", fieldName))
	}

	// Check size limit
	if int64(len(content)) > maxSize {
		return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE",
			fmt.Sprintf("content exceeds maximum size of %d bytes", maxSize))
	}

	return &InputSource{
		Content:  []byte(content),
		Filename: "pasted-spec",
		Mode:     "paste",
	}, nil
}

// readURLInputWithLimit fetches content from a URL with a custom size limit.
func (h *Handler) readURLInputWithLimit(r *http.Request, fieldName string, maxSize int64) (*InputSource, builder.Response) {
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

	// Check size limit (URLFetcher has its own 2MB limit, but operation may need stricter)
	if int64(len(content)) > maxSize {
		return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "URL_CONTENT_TOO_LARGE",
			fmt.Sprintf("fetched content exceeds maximum size of %d bytes", maxSize))
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
		return defaultRemoteFilename
	}

	// Paths ending with "/" are directories, not files
	if parsed.Path == "" || parsed.Path == "/" || parsed.Path[len(parsed.Path)-1] == '/' {
		return defaultRemoteFilename
	}

	// Get the base name from the path (excludes query string automatically)
	filename := path.Base(parsed.Path)

	// path.Base returns "." for empty paths
	if filename == "" || filename == "." {
		return defaultRemoteFilename
	}

	return filename
}
