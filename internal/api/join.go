package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/joiner"
	"github.com/erraggy/oastools/parser"
)

// JoinResponse represents the join result.
type JoinResponse struct {
	FileCount      int      `json:"fileCount"`
	Version        string   `json:"version"`
	CollisionCount int      `json:"collisionCount"`
	Warnings       []string `json:"warnings"`
	Result         string   `json:"result"`
	Format         string   `json:"format"`
}

// maxJoinFileSize is the maximum size for each file in a join operation (1MB).
const maxJoinFileSize = 1024 * 1024

func (h *Handler) handleJoin(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Parse multipart form - allow up to 5 files at 1MB each (5MB total)
	const joinMaxSize = 5 * maxJoinFileSize
	if err := r.ParseMultipartForm(joinMaxSize); err != nil {
		return h.renderError(r, http.StatusBadRequest, "FORM_PARSE_FAILED",
			fmt.Sprintf("failed to parse form: %v", err))
	}

	// Get all spec files
	files := r.MultipartForm.File["specs"]
	if len(files) < 2 {
		return h.renderError(r, http.StatusBadRequest, "INSUFFICIENT_FILES",
			"at least 2 specification files are required")
	}
	if len(files) > 5 {
		return h.renderError(r, http.StatusBadRequest, "TOO_MANY_FILES",
			"maximum 5 specification files allowed")
	}

	// Parse all specifications
	parseResults := make([]parser.ParseResult, 0, len(files))
	var firstFormat string
	for i, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return h.renderError(r, http.StatusBadRequest, "FILE_OPEN_FAILED",
				fmt.Sprintf("failed to open file %s: %v", fileHeader.Filename, err))
		}

		content, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return h.renderError(r, http.StatusBadRequest, "READ_FAILED",
				fmt.Sprintf("failed to read file %s: %v", fileHeader.Filename, err))
		}

		// Validate per-file size limit (1MB)
		if len(content) > maxJoinFileSize {
			return h.renderError(r, http.StatusBadRequest, "FILE_TOO_LARGE",
				fmt.Sprintf("file %s exceeds 1MB limit", fileHeader.Filename))
		}

		// Track format from first file
		if i == 0 {
			firstFormat = detectFormat(content)
		}

		parseResult, err := parser.ParseWithOptions(parser.WithBytes(content))
		if err != nil {
			return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
				fmt.Sprintf("failed to parse %s: %v", fileHeader.Filename, err))
		}
		parseResults = append(parseResults, *parseResult)
	}

	// Configure joiner with collision strategy
	config := joiner.DefaultConfig()
	config.DefaultStrategy = parseCollisionStrategy(r.FormValue("strategy"))

	// Join using parse-once pattern
	j := joiner.New(config)
	joinResult, err := j.JoinParsed(parseResults)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "JOIN_FAILED",
			fmt.Sprintf("join operation failed: %v", err))
	}

	// Serialize result in first file's format
	output, err := serializeDocument(joinResult.Document, firstFormat)
	if err != nil {
		slog.Error("failed to serialize join result",
			"error", err,
			"format", firstFormat,
		)
		return h.renderError(r, http.StatusInternalServerError, "SERIALIZATION_FAILED",
			"failed to serialize joined specification")
	}

	// Build response
	result := h.buildJoinResponse(parseResults, joinResult, output, firstFormat)

	// Content negotiation
	if wantsHTML(r) {
		return h.renderHTML("join-result.html", result)
	}

	return builder.JSON(http.StatusOK, result)
}

func parseCollisionStrategy(s string) joiner.CollisionStrategy {
	switch s {
	case "first":
		return joiner.StrategyAcceptLeft
	case "error":
		return joiner.StrategyFailOnCollision
	case "rename":
		return joiner.StrategyRenameRight
	default:
		return joiner.StrategyRenameRight // Default: keep left, rename right
	}
}

func (h *Handler) buildJoinResponse(parseResults []parser.ParseResult, joinResult *joiner.JoinResult, output, format string) JoinResponse {
	// Use the resulting spec version
	version := "unknown"
	if len(parseResults) > 0 {
		version = parseResults[0].Version
	}

	return JoinResponse{
		FileCount:      len(parseResults),
		Version:        version,
		CollisionCount: joinResult.CollisionCount,
		Warnings:       joinResult.Warnings,
		Result:         output,
		Format:         format,
	}
}
