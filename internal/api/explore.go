package api

import (
	"context"
	"net/http"
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

// Cache for explore analysis results (2 minute TTL).
// This will be used by the explore handlers when implemented.
//
//nolint:unused // Will be used when handlers are implemented
var exploreCache = NewTTLCache[string, *ExploreAnalysis](2 * time.Minute)

// handleExploreUpload handles spec upload and analysis.
func (h *Handler) handleExploreUpload(_ context.Context, _ *builder.Request) builder.Response {
	return builder.Error(http.StatusNotImplemented, "Not implemented")
}

// handleExploreOperations renders the operations tab partial.
func (h *Handler) handleExploreOperations(_ context.Context, _ *builder.Request) builder.Response {
	return builder.Error(http.StatusNotImplemented, "Not implemented")
}

// handleExploreSchemas renders the schemas tab partial.
func (h *Handler) handleExploreSchemas(_ context.Context, _ *builder.Request) builder.Response {
	return builder.Error(http.StatusNotImplemented, "Not implemented")
}

// handleExploreSecurity renders the security tab partial.
func (h *Handler) handleExploreSecurity(_ context.Context, _ *builder.Request) builder.Response {
	return builder.Error(http.StatusNotImplemented, "Not implemented")
}
