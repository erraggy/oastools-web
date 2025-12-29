package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/joiner"
	"github.com/erraggy/oastools/parser"
	"go.yaml.in/yaml/v4"
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

func (h *Handler) handleJoin(_ context.Context, req *builder.Request) builder.Response {
	// Parse multipart form - allow up to 5 files at 1MB each
	if err := req.HTTPRequest.ParseMultipartForm(5 * h.cfg.MaxFileSize); err != nil {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "FORM_PARSE_FAILED",
				Message: fmt.Sprintf("failed to parse form: %v", err),
			},
		})
	}

	// Get all spec files
	files := req.HTTPRequest.MultipartForm.File["specs"]
	if len(files) < 2 {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "INSUFFICIENT_FILES",
				Message: "at least 2 specification files are required",
			},
		})
	}
	if len(files) > 5 {
		return builder.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Code:    "TOO_MANY_FILES",
				Message: "maximum 5 specification files allowed",
			},
		})
	}

	// Parse all specifications
	parseResults := make([]parser.ParseResult, 0, len(files))
	var firstFormat string
	for i, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			return builder.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Code:    "FILE_OPEN_FAILED",
					Message: fmt.Sprintf("failed to open file %s: %v", fileHeader.Filename, err),
				},
			})
		}

		content, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			return builder.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Code:    "READ_FAILED",
					Message: fmt.Sprintf("failed to read file %s: %v", fileHeader.Filename, err),
				},
			})
		}

		// Track format from first file
		if i == 0 {
			firstFormat = detectFormat(content)
		}

		parseResult, err := parser.ParseWithOptions(parser.WithBytes(content))
		if err != nil {
			return builder.JSON(http.StatusBadRequest, ErrorResponse{
				Error: ErrorDetail{
					Code:    "PARSE_FAILED",
					Message: fmt.Sprintf("failed to parse %s: %v", fileHeader.Filename, err),
				},
			})
		}
		parseResults = append(parseResults, *parseResult)
	}

	// Configure joiner with collision strategy
	config := joiner.DefaultConfig()
	config.DefaultStrategy = parseCollisionStrategy(req.HTTPRequest.FormValue("strategy"))

	// Join using parse-once pattern
	j := joiner.New(config)
	joinResult, err := j.JoinParsed(parseResults)
	if err != nil {
		return builder.JSON(http.StatusUnprocessableEntity, ErrorResponse{
			Error: ErrorDetail{
				Code:    "JOIN_FAILED",
				Message: fmt.Sprintf("join operation failed: %v", err),
			},
		})
	}

	// Serialize result in first file's format
	var output []byte
	if firstFormat == "json" {
		output, _ = json.MarshalIndent(joinResult.Document, "", "  ")
	} else {
		output, _ = yaml.Marshal(joinResult.Document)
	}

	// Build response
	result := h.buildJoinResponse(parseResults, joinResult, string(output), firstFormat)

	// Content negotiation
	if wantsHTML(req.HTTPRequest) {
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
