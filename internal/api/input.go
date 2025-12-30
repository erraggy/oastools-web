package api

import (
	"fmt"
	"io"
	"net/http"

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
func (h *Handler) readInput(r *http.Request, fieldName string) (*InputSource, builder.Response) {
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
	defer file.Close()

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
	url := r.FormValue(fieldName + "_url")
	if url == "" {
		return nil, h.renderError(r, http.StatusBadRequest, "MISSING_URL",
			fmt.Sprintf("%s URL is required", fieldName))
	}

	content, err := h.urlFetcher.Fetch(r.Context(), url)
	if err != nil {
		return nil, h.renderError(r, http.StatusBadRequest, "FETCH_FAILED",
			fmt.Sprintf("failed to fetch URL: %v", err))
	}

	// Extract filename from URL path
	filename := extractFilenameFromURL(url)

	return &InputSource{
		Content:  content,
		Filename: filename,
		Mode:     "url",
	}, nil
}

// extractFilenameFromURL extracts a filename from a URL, falling back to "remote-spec".
func extractFilenameFromURL(rawURL string) string {
	// Simple extraction: find last path segment
	for i := len(rawURL) - 1; i >= 0; i-- {
		if rawURL[i] == '/' {
			if i < len(rawURL)-1 {
				filename := rawURL[i+1:]
				// Remove query string if present
				for j := 0; j < len(filename); j++ {
					if filename[j] == '?' {
						filename = filename[:j]
						break
					}
				}
				if filename != "" {
					return filename
				}
			}
			break
		}
	}
	return "remote-spec"
}
