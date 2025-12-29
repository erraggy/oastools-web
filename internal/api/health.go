package api

import (
	"context"
	"net/http"
	"runtime/debug"

	"github.com/erraggy/oastools/builder"
)

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status   string `json:"status"`
	Version  string `json:"version"`
	OASTools string `json:"oastools"`
}

func (h *Handler) handleHealth(_ context.Context, _ *builder.Request) builder.Response {
	return builder.JSON(http.StatusOK, HealthResponse{
		Status:   "healthy",
		Version:  h.version,
		OASTools: getOASToolsVersion(),
	})
}

// getOASToolsVersion returns the oastools module version from build info.
func getOASToolsVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	for _, dep := range info.Deps {
		if dep.Path == "github.com/erraggy/oastools" {
			return dep.Version
		}
	}

	return "unknown"
}
