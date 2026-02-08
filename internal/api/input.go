package api

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/erraggy/oastools/builder"
)

const (
	defaultRemoteFilename = "remote-spec"

	// Input mode constants
	inputModeFile  = "file"
	inputModePaste = "paste"
	inputModeURL   = "url"
)

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
		mode = inputModeFile // default
	}

	switch mode {
	case inputModeFile:
		return h.readFileInputWithLimit(r, fieldName, maxSize)
	case inputModePaste:
		return h.readPasteInputWithLimit(r, fieldName, maxSize)
	case inputModeURL:
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

	content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, h.renderError(r, http.StatusBadRequest, "READ_FAILED",
			fmt.Sprintf("failed to read %s file: %v", fieldName, err))
	}

	if len(content) == 0 {
		return nil, h.renderError(r, http.StatusBadRequest, "EMPTY_FILE",
			fmt.Sprintf("%s file is empty", fieldName))
	}

	if int64(len(content)) > maxSize {
		return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
			fmt.Sprintf("file exceeds maximum size of %d bytes", maxSize))
	}

	return &InputSource{
		Content:  content,
		Filename: header.Filename,
		Mode:     inputModeFile,
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
		Mode:     inputModePaste,
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
		Mode:     inputModeURL,
	}, nil
}

// readMultipleInputs reads multiple spec inputs from file uploads or pasted content.
// It checks input_mode to determine the source and delegates to mode-specific readers
// that validate count bounds and per-item size. URL mode is not supported for multiple inputs.
func (h *Handler) readMultipleInputs(r *http.Request, fieldName string, maxSize int64, minCount, maxCount int) ([]*InputSource, builder.Response) { //nolint:unparam // fieldName is parameterized for consistency with readInputWithLimit
	mode := r.FormValue("input_mode")
	if mode == "" {
		mode = inputModeFile
	}

	switch mode {
	case inputModeFile:
		return h.readMultipleFileInputs(r, fieldName, maxSize, minCount, maxCount)
	case inputModePaste:
		return h.readMultiplePasteInputs(r, fieldName, maxSize, minCount, maxCount)
	case inputModeURL:
		return nil, h.renderError(r, http.StatusBadRequest, "UNSUPPORTED_MODE",
			"URL mode is not supported for multiple inputs")
	default:
		return nil, h.renderError(r, http.StatusBadRequest, "INVALID_MODE",
			fmt.Sprintf("invalid input mode: %s", mode))
	}
}

// readMultipleFileInputs reads multiple files from a multipart form field.
func (h *Handler) readMultipleFileInputs(r *http.Request, fieldName string, maxSize int64, minCount, maxCount int) ([]*InputSource, builder.Response) {
	if r.MultipartForm == nil {
		return nil, h.renderError(r, http.StatusBadRequest, "FORM_NOT_PARSED",
			"multipart form data is required")
	}

	files := r.MultipartForm.File[fieldName]
	if len(files) < minCount {
		return nil, h.renderError(r, http.StatusBadRequest, "INSUFFICIENT_FILES",
			fmt.Sprintf("at least %d specification files are required", minCount))
	}
	if len(files) > maxCount {
		return nil, h.renderError(r, http.StatusBadRequest, "TOO_MANY_FILES",
			fmt.Sprintf("maximum %d specification files allowed", maxCount))
	}

	sources := make([]*InputSource, 0, len(files))
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return nil, h.renderError(r, http.StatusBadRequest, "FILE_OPEN_FAILED",
				fmt.Sprintf("failed to open file %s: %v", fileHeader.Filename, err))
		}

		content, err := io.ReadAll(io.LimitReader(file, maxSize+1))
		_ = file.Close()
		if err != nil {
			return nil, h.renderError(r, http.StatusBadRequest, "READ_FAILED",
				fmt.Sprintf("failed to read file %s: %v", fileHeader.Filename, err))
		}

		if len(content) == 0 {
			return nil, h.renderError(r, http.StatusBadRequest, "EMPTY_FILE",
				fmt.Sprintf("file %s is empty", fileHeader.Filename))
		}

		if int64(len(content)) > maxSize {
			return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
				fmt.Sprintf("file %s exceeds maximum size of %d bytes", fileHeader.Filename, maxSize))
		}

		sources = append(sources, &InputSource{
			Content:  content,
			Filename: fileHeader.Filename,
			Mode:     inputModeFile,
		})
	}

	return sources, nil
}

// readMultiplePasteInputs reads multiple pasted specs from form values.
func (h *Handler) readMultiplePasteInputs(r *http.Request, fieldName string, maxSize int64, minCount, maxCount int) ([]*InputSource, builder.Response) {
	values := r.Form[fieldName+"_content"]

	// Filter out empty values
	var contents []string
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			contents = append(contents, v)
		}
	}

	if len(contents) < minCount {
		return nil, h.renderError(r, http.StatusBadRequest, "INSUFFICIENT_SPECS",
			fmt.Sprintf("at least %d specifications are required", minCount))
	}
	if len(contents) > maxCount {
		return nil, h.renderError(r, http.StatusBadRequest, "TOO_MANY_SPECS",
			fmt.Sprintf("maximum %d specifications allowed", maxCount))
	}

	sources := make([]*InputSource, 0, len(contents))
	for i, content := range contents {
		if int64(len(content)) > maxSize {
			return nil, h.renderError(r, http.StatusRequestEntityTooLarge, "CONTENT_TOO_LARGE",
				fmt.Sprintf("spec %d exceeds maximum size of %d bytes", i+1, maxSize))
		}

		sources = append(sources, &InputSource{
			Content:  []byte(content),
			Filename: fmt.Sprintf("pasted-spec-%d", i+1),
			Mode:     inputModePaste,
		})
	}

	return sources, nil
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
