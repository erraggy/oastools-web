package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/erraggy/oastools/builder"
	"github.com/erraggy/oastools/parser"
	"github.com/erraggy/oastools/walker"
)

// ExploreAnalysis holds the parsed and analyzed spec data.
type ExploreAnalysis struct {
	Hash        string
	Version     string
	Filename    string
	ParseResult *parser.ParseResult
	Operations  *walker.OperationCollector
	Schemas     *walker.SchemaCollector
	Security    []SecuritySchemeInfo
	Stats       ExploreStats
}

// ExploreStats holds summary statistics for the spec.
type ExploreStats struct {
	PathCount      int
	OperationCount int
	SchemaCount    int
	InlineCount    int
	SecuredCount   int
	UnsecuredCount int
	MethodCounts   map[string]int
}

// SecuritySchemeInfo holds parsed security scheme information.
type SecuritySchemeInfo struct {
	Name       string
	Type       string
	Scheme     string
	In         string
	ParamName  string
	Flows      []OAuthFlowInfo
	OpenIDURL  string
	UsageCount int
}

// OAuthFlowInfo holds OAuth flow configuration.
type OAuthFlowInfo struct {
	Type             string
	AuthorizationURL string
	TokenURL         string
	RefreshURL       string
	Scopes           map[string]string
}

// SchemaUsage represents where a schema is used.
type SchemaUsage struct {
	Method       string
	PathTemplate string
	Context      string // e.g., "request body", "response 200"
}

// SecurityUsage represents an operation that uses a security scheme.
type SecurityUsage struct {
	Method       string
	PathTemplate string
	Scopes       []string
}

// UnsecuredOperation represents an operation with no security requirements.
type UnsecuredOperation struct {
	Method       string
	PathTemplate string
	Summary      string
}

// Cache for explore analysis results (2 minute TTL).
var exploreCache = NewTTLCache[string, *ExploreAnalysis](2 * time.Minute)

// handleExploreUpload handles spec upload and analysis.
func (h *Handler) handleExploreUpload(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest

	// Read input from any supported mode (file, paste, URL)
	input, errResp := h.readInput(r, "spec")
	if errResp != nil {
		return errResp
	}

	// Compute SHA256 hash of content (first 16 chars)
	hash := computeHash(input.Content)

	// Check cache - if hit, return cached analysis
	if cached, ok := exploreCache.Get(hash); ok {
		return h.renderExploreResult(r, cached)
	}

	// Parse spec using oastools
	parseResult, err := parser.ParseWithOptions(parser.WithBytes(input.Content))
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "PARSE_FAILED",
			fmt.Sprintf("failed to parse specification: %v", err))
	}

	// Collect operations using walker
	operations, err := walker.CollectOperations(parseResult)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "WALK_FAILED",
			fmt.Sprintf("failed to collect operations: %v", err))
	}

	// Collect schemas using walker
	schemas, err := walker.CollectSchemas(parseResult)
	if err != nil {
		return h.renderError(r, http.StatusBadRequest, "WALK_FAILED",
			fmt.Sprintf("failed to collect schemas: %v", err))
	}

	// Extract security schemes from parseResult
	security := extractSecuritySchemes(parseResult, operations)

	// Compute stats
	stats := computeExploreStats(parseResult, operations, schemas)

	// Get version string
	version := getVersionString(parseResult)

	// Create ExploreAnalysis struct
	analysis := &ExploreAnalysis{
		Hash:        hash,
		Version:     version,
		Filename:    input.Filename,
		ParseResult: parseResult,
		Operations:  operations,
		Schemas:     schemas,
		Security:    security,
		Stats:       stats,
	}

	// Cache it
	exploreCache.Set(hash, analysis)

	return h.renderExploreResult(r, analysis)
}

// renderExploreResult renders the explore result with content negotiation.
func (h *Handler) renderExploreResult(r *http.Request, analysis *ExploreAnalysis) builder.Response {
	if wantsHTML(r) {
		return h.renderHTML("explore_results", map[string]any{
			"Analysis": analysis,
		})
	}

	// JSON response
	return builder.JSON(http.StatusOK, map[string]any{
		"hash":     analysis.Hash,
		"version":  analysis.Version,
		"filename": analysis.Filename,
		"stats":    analysis.Stats,
		"security": analysis.Security,
	})
}

// computeHash computes SHA256 hash of content and returns first 16 characters.
func computeHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])[:16]
}

// getVersionString returns the OpenAPI version string from the parse result.
func getVersionString(result *parser.ParseResult) string {
	if result == nil {
		return ""
	}
	return result.Version
}

// extractSecuritySchemes extracts security scheme info from the parse result.
func extractSecuritySchemes(result *parser.ParseResult, operations *walker.OperationCollector) []SecuritySchemeInfo {
	if result == nil {
		return nil
	}

	var schemes map[string]*parser.SecurityScheme

	// Get security schemes based on OAS version
	if doc, ok := result.OAS3Document(); ok {
		if doc.Components != nil {
			schemes = doc.Components.SecuritySchemes
		}
	} else if doc, ok := result.OAS2Document(); ok {
		schemes = doc.SecurityDefinitions
	}

	if schemes == nil {
		return nil
	}

	// Count usage of each security scheme (considers global security)
	usageCounts := countSecurityUsage(result, operations)

	// Build security scheme info list
	var securityInfos []SecuritySchemeInfo
	for name, scheme := range schemes {
		if scheme == nil {
			continue
		}

		info := SecuritySchemeInfo{
			Name:       name,
			Type:       scheme.Type,
			Scheme:     scheme.Scheme,
			In:         scheme.In,
			ParamName:  scheme.Name,
			OpenIDURL:  scheme.OpenIDConnectURL,
			UsageCount: usageCounts[name],
		}

		// Extract OAuth flows for OAS 3.0+
		if scheme.Flows != nil {
			info.Flows = extractOAuthFlows(scheme.Flows)
		}

		// Handle OAS 2.0 OAuth flow (single flow per scheme)
		if scheme.Flow != "" {
			info.Flows = []OAuthFlowInfo{
				{
					Type:             scheme.Flow,
					AuthorizationURL: scheme.AuthorizationURL,
					TokenURL:         scheme.TokenURL,
					Scopes:           scheme.Scopes,
				},
			}
		}

		securityInfos = append(securityInfos, info)
	}

	return securityInfos
}

// countSecurityUsage counts how many operations use each security scheme.
// It considers both operation-level and global security requirements.
func countSecurityUsage(
	result *parser.ParseResult,
	operations *walker.OperationCollector,
) map[string]int {
	counts := make(map[string]int)
	if operations == nil {
		return counts
	}

	// Get global security requirements
	var globalSecurity []parser.SecurityRequirement
	if result != nil {
		if doc, ok := result.OAS3Document(); ok {
			globalSecurity = doc.Security
		} else if doc, ok := result.OAS2Document(); ok {
			globalSecurity = doc.Security
		}
	}

	for _, opInfo := range operations.All {
		if opInfo.Operation == nil {
			continue
		}

		// Determine which security requirements apply to this operation
		var effectiveSecurity []parser.SecurityRequirement
		if opInfo.Operation.Security != nil {
			// Operation has explicit security (could be empty [] to disable)
			effectiveSecurity = opInfo.Operation.Security
		} else {
			// Operation inherits global security
			effectiveSecurity = globalSecurity
		}

		for _, secReq := range effectiveSecurity {
			for schemeName := range secReq {
				counts[schemeName]++
			}
		}
	}

	return counts
}

// extractOAuthFlows extracts OAuth flow details from OAS 3.0+ flows.
func extractOAuthFlows(flows *parser.OAuthFlows) []OAuthFlowInfo {
	if flows == nil {
		return nil
	}

	var flowInfos []OAuthFlowInfo

	if flows.Implicit != nil {
		flowInfos = append(flowInfos, OAuthFlowInfo{
			Type:             "implicit",
			AuthorizationURL: flows.Implicit.AuthorizationURL,
			RefreshURL:       flows.Implicit.RefreshURL,
			Scopes:           flows.Implicit.Scopes,
		})
	}

	if flows.Password != nil {
		flowInfos = append(flowInfos, OAuthFlowInfo{
			Type:       "password",
			TokenURL:   flows.Password.TokenURL,
			RefreshURL: flows.Password.RefreshURL,
			Scopes:     flows.Password.Scopes,
		})
	}

	if flows.ClientCredentials != nil {
		flowInfos = append(flowInfos, OAuthFlowInfo{
			Type:       "clientCredentials",
			TokenURL:   flows.ClientCredentials.TokenURL,
			RefreshURL: flows.ClientCredentials.RefreshURL,
			Scopes:     flows.ClientCredentials.Scopes,
		})
	}

	if flows.AuthorizationCode != nil {
		flowInfos = append(flowInfos, OAuthFlowInfo{
			Type:             "authorizationCode",
			AuthorizationURL: flows.AuthorizationCode.AuthorizationURL,
			TokenURL:         flows.AuthorizationCode.TokenURL,
			RefreshURL:       flows.AuthorizationCode.RefreshURL,
			Scopes:           flows.AuthorizationCode.Scopes,
		})
	}

	return flowInfos
}

// computeExploreStats computes summary statistics for the spec.
func computeExploreStats(
	result *parser.ParseResult,
	ops *walker.OperationCollector,
	schemas *walker.SchemaCollector,
) ExploreStats {
	stats := ExploreStats{
		MethodCounts: make(map[string]int),
	}

	// Path count and global security check
	var hasGlobalSecurity bool
	if result != nil {
		if doc, ok := result.OAS3Document(); ok {
			if doc.Paths != nil {
				stats.PathCount = len(doc.Paths)
			}
			hasGlobalSecurity = len(doc.Security) > 0
		} else if doc, ok := result.OAS2Document(); ok {
			if doc.Paths != nil {
				stats.PathCount = len(doc.Paths)
			}
			hasGlobalSecurity = len(doc.Security) > 0
		}
	}

	// Operation count and method counts
	if ops != nil {
		stats.OperationCount = len(ops.All)

		// Count operations by method and secured status
		for _, opInfo := range ops.All {
			method := strings.ToUpper(opInfo.Method)
			stats.MethodCounts[method]++

			// Check if operation has security requirements
			// An operation is secured if:
			// 1. It has its own non-empty security requirements, OR
			// 2. It has no explicit security AND the document has global security
			// An operation with an empty security array [] explicitly disables security
			if opInfo.Operation != nil {
				opSec := opInfo.Operation.Security
				if opSec != nil {
					// Explicit security defined (even empty [] means explicitly unsecured)
					if len(opSec) > 0 {
						stats.SecuredCount++
					} else {
						stats.UnsecuredCount++
					}
				} else if hasGlobalSecurity {
					// Inherits global security
					stats.SecuredCount++
				} else {
					stats.UnsecuredCount++
				}
			} else {
				stats.UnsecuredCount++
			}
		}
	}

	// Schema counts
	if schemas != nil {
		stats.SchemaCount = len(schemas.Components)
		stats.InlineCount = len(schemas.Inline)
	}

	return stats
}

// handleExploreOperations renders the operations tab partial.
func (h *Handler) handleExploreOperations(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	group := r.URL.Query().Get("group")
	if group == "" {
		group = "path"
	}

	data := map[string]any{
		"Analysis": analysis,
		"Group":    group,
	}

	return h.renderHTML("explore_operations", data)
}

// handleExploreSchemas renders the schemas tab partial.
func (h *Handler) handleExploreSchemas(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	return h.renderHTML("explore_schemas", map[string]any{
		"Analysis": analysis,
	})
}

// handleExploreSecurity renders the security tab partial.
func (h *Handler) handleExploreSecurity(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	return h.renderHTML("explore_security", map[string]any{
		"Analysis": analysis,
	})
}

// handleExploreOperationDetail renders the operation detail partial.
func (h *Handler) handleExploreOperationDetail(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	path := r.URL.Query().Get("path")
	method := r.URL.Query().Get("method")

	if path == "" || method == "" {
		return builder.Error(http.StatusBadRequest, "Missing path or method parameter")
	}

	// Find the operation
	var found *walker.OperationInfo
	for _, op := range analysis.Operations.All {
		if op.PathTemplate == path && op.Method == method {
			found = op
			break
		}
	}

	if found == nil {
		return builder.Error(http.StatusNotFound, "Operation not found")
	}

	// Build operationID - use the operation's ID or generate one from method+path
	operationID := found.Operation.OperationID
	if operationID == "" {
		// Generate a simple ID from method and path
		pathID := strings.ReplaceAll(found.PathTemplate, "/", "-")
		pathID = strings.ReplaceAll(pathID, "{", "")
		pathID = strings.ReplaceAll(pathID, "}", "")
		operationID = found.Method + pathID
	}

	data := map[string]any{
		"Operation":    found.Operation,
		"PathTemplate": found.PathTemplate,
		"Method":       found.Method,
		"OperationID":  operationID,
	}

	return h.renderHTML("explore_operation_detail", data)
}

// handleExploreSchemaDetail renders the schema detail partial.
func (h *Handler) handleExploreSchemaDetail(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		return builder.Error(http.StatusBadRequest, "Missing name parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	// Find schema in components by name
	var foundSchema *parser.Schema
	for _, schemaInfo := range analysis.Schemas.Components {
		if schemaInfo.Name == name {
			foundSchema = schemaInfo.Schema
			break
		}
	}

	if foundSchema == nil {
		return builder.Error(http.StatusNotFound, "Schema not found")
	}

	// Find usages of this schema
	usages := findSchemaUsages(analysis, name)

	data := map[string]any{
		"Name":   name,
		"Schema": foundSchema,
		"UsedIn": usages,
	}

	return h.renderHTML("explore_schema_detail", data)
}

// InlineSchemaLocation represents where an inline schema appears.
type InlineSchemaLocation struct {
	Method       string
	PathTemplate string
	Context      string
	Type         string
}

// handleExploreInlineSchemas renders the inline schemas drill-in partial.
func (h *Handler) handleExploreInlineSchemas(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	locations := parseInlineLocations(analysis)

	return h.renderHTML("explore_inline_schemas", map[string]any{
		"InlineSchemas": locations,
	})
}

// parseInlineLocations parses JSON paths from inline schemas to extract location details.
// JSON path format: $.paths['/pets'].post.requestBody.content['application/json'].schema
func parseInlineLocations(analysis *ExploreAnalysis) []InlineSchemaLocation {
	if analysis == nil || analysis.Schemas == nil {
		return nil
	}

	var locations []InlineSchemaLocation
	for _, schemaInfo := range analysis.Schemas.Inline {
		loc := InlineSchemaLocation{
			Type: getSchemaType(schemaInfo.Schema),
		}

		jsonPath := schemaInfo.JSONPath

		// Extract path template from $.paths['/pets']
		if idx := strings.Index(jsonPath, "$.paths['"); idx != -1 {
			start := idx + len("$.paths['")
			end := strings.Index(jsonPath[start:], "']")
			if end != -1 {
				loc.PathTemplate = jsonPath[start : start+end]
			}
		}

		// Extract method (get, post, put, patch, delete)
		remaining := jsonPath
		if loc.PathTemplate != "" {
			remaining = jsonPath[strings.Index(jsonPath, "'].")+3:]
		}
		methods := []string{"get", "post", "put", "patch", "delete", "options", "head", "trace"}
		for _, m := range methods {
			if strings.HasPrefix(remaining, m+".") || strings.HasPrefix(remaining, m+"[") || remaining == m {
				loc.Method = m
				remaining = strings.TrimPrefix(remaining, m)
				remaining = strings.TrimPrefix(remaining, ".")
				break
			}
		}

		// Extract context from requestBody/responses/parameters
		switch {
		case strings.Contains(remaining, "requestBody"):
			loc.Context = "request body"
		case strings.Contains(remaining, "responses"):
			// Try to extract status code
			if idx := strings.Index(remaining, "responses['"); idx != -1 {
				start := idx + len("responses['")
				end := strings.Index(remaining[start:], "']")
				if end != -1 {
					loc.Context = "response " + remaining[start:start+end]
				}
			} else if strings.Contains(remaining, "responses.default") {
				loc.Context = "response default"
			} else {
				loc.Context = "response"
			}
		case strings.Contains(remaining, "parameters"):
			loc.Context = "parameter"
		default:
			loc.Context = "schema"
		}

		locations = append(locations, loc)
	}

	return locations
}

// getSchemaType returns a human-readable type badge for a schema.
func getSchemaType(s *parser.Schema) string {
	if s == nil {
		return ""
	}
	if s.Enum != nil {
		return "[enum]"
	}
	t := formatTypeString(s.Type)
	if t == "array" {
		return "[array]"
	}
	if s.Properties != nil {
		return "{object}"
	}
	if s.AllOf != nil {
		return "{allOf}"
	}
	if s.OneOf != nil {
		return "{oneOf}"
	}
	if s.AnyOf != nil {
		return "{anyOf}"
	}
	return t
}

// formatTypeString converts Type (string or []any) to string.
func formatTypeString(t any) string {
	switch v := t.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != jsonNullType {
				return s
			}
		}
	}
	return ""
}

// findSchemaUsages finds all operations that reference the given schema.
func findSchemaUsages(analysis *ExploreAnalysis, schemaName string) []SchemaUsage {
	var usages []SchemaUsage
	refSuffix := "/" + schemaName

	for _, op := range analysis.Operations.All {
		// Check request body
		if op.Operation.RequestBody != nil {
			for _, mt := range op.Operation.RequestBody.Content {
				if mt.Schema != nil && strings.HasSuffix(mt.Schema.Ref, refSuffix) {
					usages = append(usages, SchemaUsage{
						Method:       op.Method,
						PathTemplate: op.PathTemplate,
						Context:      "request body",
					})
				}
			}
		}

		// Check responses - note: Responses has Codes map and Default
		if op.Operation.Responses != nil {
			for status, resp := range op.Operation.Responses.Codes {
				for _, mt := range resp.Content {
					if mt.Schema != nil && strings.HasSuffix(mt.Schema.Ref, refSuffix) {
						usages = append(usages, SchemaUsage{
							Method:       op.Method,
							PathTemplate: op.PathTemplate,
							Context:      "response " + status,
						})
					}
				}
			}
		}
	}

	return usages
}

// handleExploreSecurityDetail renders the security scheme detail partial.
func (h *Handler) handleExploreSecurityDetail(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		return builder.Error(http.StatusBadRequest, "Missing name parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	// Find the security scheme by name
	var foundScheme *SecuritySchemeInfo
	for i := range analysis.Security {
		if analysis.Security[i].Name == name {
			foundScheme = &analysis.Security[i]
			break
		}
	}

	if foundScheme == nil {
		return builder.Error(http.StatusNotFound, "Security scheme not found")
	}

	// Find usages of this security scheme
	usages := findSecurityUsages(analysis, name)

	data := map[string]any{
		"Scheme": foundScheme,
		"UsedBy": usages,
	}

	return h.renderHTML("explore_security_detail", data)
}

// findSecurityUsages finds all operations that use the given security scheme.
func findSecurityUsages(analysis *ExploreAnalysis, schemeName string) []SecurityUsage {
	var usages []SecurityUsage
	if analysis == nil || analysis.Operations == nil {
		return usages
	}

	// Get global security requirements
	var globalSecurity []parser.SecurityRequirement
	if analysis.ParseResult != nil {
		if doc, ok := analysis.ParseResult.OAS3Document(); ok {
			globalSecurity = doc.Security
		} else if doc, ok := analysis.ParseResult.OAS2Document(); ok {
			globalSecurity = doc.Security
		}
	}

	for _, opInfo := range analysis.Operations.All {
		if opInfo.Operation == nil {
			continue
		}

		// Determine which security requirements apply to this operation
		var effectiveSecurity []parser.SecurityRequirement
		if opInfo.Operation.Security != nil {
			effectiveSecurity = opInfo.Operation.Security
		} else {
			effectiveSecurity = globalSecurity
		}

		// Check if this scheme is used
		for _, secReq := range effectiveSecurity {
			if scopes, found := secReq[schemeName]; found {
				usages = append(usages, SecurityUsage{
					Method:       opInfo.Method,
					PathTemplate: opInfo.PathTemplate,
					Scopes:       scopes,
				})
				break // Only add once per operation
			}
		}
	}

	return usages
}

// handleExploreUnsecured renders the unsecured operations partial.
func (h *Handler) handleExploreUnsecured(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	// Find unsecured operations
	unsecured := findUnsecuredOperations(analysis)

	return h.renderHTML("explore_unsecured", map[string]any{
		"UnsecuredOps": unsecured,
	})
}

// findUnsecuredOperations finds all operations with no security requirements.
func findUnsecuredOperations(analysis *ExploreAnalysis) []UnsecuredOperation {
	var unsecured []UnsecuredOperation
	if analysis == nil || analysis.Operations == nil {
		return unsecured
	}

	// Check if document has global security
	var hasGlobalSecurity bool
	if analysis.ParseResult != nil {
		if doc, ok := analysis.ParseResult.OAS3Document(); ok {
			hasGlobalSecurity = len(doc.Security) > 0
		} else if doc, ok := analysis.ParseResult.OAS2Document(); ok {
			hasGlobalSecurity = len(doc.Security) > 0
		}
	}

	for _, opInfo := range analysis.Operations.All {
		if opInfo.Operation == nil {
			unsecured = append(unsecured, UnsecuredOperation{
				Method:       opInfo.Method,
				PathTemplate: opInfo.PathTemplate,
			})
			continue
		}

		opSec := opInfo.Operation.Security

		// An operation is unsecured if:
		// 1. It has explicit empty security [] (opSec != nil && len(opSec) == 0), OR
		// 2. It has nil security AND no document-level global security
		if opSec != nil {
			if len(opSec) == 0 {
				// Explicitly unsecured with []
				unsecured = append(unsecured, UnsecuredOperation{
					Method:       opInfo.Method,
					PathTemplate: opInfo.PathTemplate,
					Summary:      opInfo.Operation.Summary,
				})
			}
		} else if !hasGlobalSecurity {
			// No explicit security and no global security
			unsecured = append(unsecured, UnsecuredOperation{
				Method:       opInfo.Method,
				PathTemplate: opInfo.PathTemplate,
				Summary:      opInfo.Operation.Summary,
			})
		}
	}

	return unsecured
}

// handleExploreSummaryDetails renders the summary details expansion partial.
func (h *Handler) handleExploreSummaryDetails(_ context.Context, req *builder.Request) builder.Response {
	r := req.HTTPRequest
	hash := r.URL.Query().Get("h")
	if hash == "" {
		return builder.Error(http.StatusBadRequest, "Missing hash parameter")
	}

	analysis, ok := exploreCache.Get(hash)
	if !ok {
		return &cacheExpiredResponse{}
	}

	// Compute security coverage percentage
	var coveragePercent int
	if analysis.Stats.OperationCount > 0 {
		coveragePercent = (analysis.Stats.SecuredCount * 100) / analysis.Stats.OperationCount
	}

	return h.renderHTML("explore_summary_details", map[string]any{
		"Analysis":        analysis,
		"CoveragePercent": coveragePercent,
	})
}
