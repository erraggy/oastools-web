package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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

	// Read all spec inputs (file or paste mode)
	sources, errResp := h.readMultipleInputs(r, "specs", maxJoinFileSize, 2, 5)
	if errResp != nil {
		return errResp
	}

	// Parse all specifications
	parseResults := make([]parser.ParseResult, 0, len(sources))
	var firstFormat string
	var totalInputBytes int
	for i, src := range sources {
		totalInputBytes += len(src.Content)

		// Track format from first input
		if i == 0 {
			firstFormat = detectFormat(src.Content)
		}

		parseStart := time.Now()
		parseResult, err := parser.ParseWithOptions(
			parser.WithBytes(src.Content),
			parser.WithSourceName(src.Filename),
		)
		h.instruments.recordPackageDuration(r.Context(), "parser", parseStart)
		if err != nil {
			return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
				fmt.Sprintf("failed to parse %s: %v", src.Filename, err))
		}
		parseResults = append(parseResults, *parseResult)
	}

	// Enrich metrics context
	if ma := getMetricsAttrs(r.Context()); ma != nil {
		ma.enrich("join", firstFormat, totalInputBytes)
	}

	// Record spec complexity from first spec
	if len(parseResults) > 0 {
		stats := parser.GetDocumentStats(parseResults[0].Document)
		h.instruments.recordSpecComplexity(r.Context(), "joiner", parseResults[0].Version,
			stats.PathCount, stats.OperationCount, stats.SchemaCount, totalInputBytes)
	}

	// Configure joiner with collision strategy
	config := joiner.DefaultConfig()
	config.DefaultStrategy = parseCollisionStrategy(r.FormValue("strategy"))

	// Apply advanced options when specified (empty means "same as default")
	if ps := r.FormValue("pathStrategy"); ps != "" {
		config.PathStrategy = parseCollisionStrategy(ps)
	}
	if ss := r.FormValue("schemaStrategy"); ss != "" {
		config.SchemaStrategy = parseSchemaStrategy(ss)
	}
	if em := r.FormValue("equivalenceMode"); em != "" {
		config.EquivalenceMode = em
	} else if config.SchemaStrategy == joiner.StrategyDeduplicateEquivalent {
		config.EquivalenceMode = string(joiner.EquivalenceModeDeep)
	}
	if r.FormValue("semanticDedup") == "on" {
		config.SemanticDeduplication = true
	}

	// Join using parse-once pattern
	j := joiner.New(config)
	joinStart := time.Now()
	joinResult, err := j.JoinParsed(parseResults)
	h.instruments.recordPackageDuration(r.Context(), "joiner", joinStart)
	if err != nil {
		return h.renderError(r, http.StatusUnprocessableEntity, "JOIN_FAILED",
			formatJoinError(err))
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

// parseSchemaStrategy maps form values to joiner strategies for schemas.
// This extends parseCollisionStrategy with "deduplicate" which only applies to schemas.
func parseSchemaStrategy(s string) joiner.CollisionStrategy {
	if s == "deduplicate" {
		return joiner.StrategyDeduplicateEquivalent
	}
	return parseCollisionStrategy(s)
}

// formatJoinError rewrites joiner errors into user-friendly messages.
// The joiner library produces CLI-oriented messages (e.g., "set --path-strategy to 'accept-left'")
// that reference flags and internal strategy names. This translates them for the web UI.
func formatJoinError(err error) string {
	var ce *joiner.CollisionError
	if !errors.As(err, &ce) {
		return fmt.Sprintf("join operation failed: %v", err)
	}

	// Map internal strategy names to UI labels
	strategyLabel := map[joiner.CollisionStrategy]string{
		joiner.StrategyFailOnCollision: "Error",
		joiner.StrategyAcceptLeft:      "First",
		joiner.StrategyRenameRight:     "Rename",
	}

	strategy := "Error"
	if label, ok := strategyLabel[ce.Strategy]; ok {
		strategy = label
	}

	return fmt.Sprintf("Collision in %s: '%s' is defined in both %s and %s. "+
		"Current strategy: %s. Change the Collision Strategy to \"First\" (keep first occurrence) "+
		"or \"Rename\" (append suffix to duplicates) to resolve.",
		ce.Section, ce.Key, ce.FirstFile, ce.SecondFile, strategy)
}

func (h *Handler) buildJoinResponse(parseResults []parser.ParseResult, joinResult *joiner.JoinResult, output, format string) JoinResponse {
	// Use the resulting spec version
	version := versionUnknown
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
