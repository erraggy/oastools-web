package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetrics_SkipsStaticFiles(t *testing.T) {
	inst := newInstruments()
	inner, called := testHandler()
	wrapped := Metrics(inst)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/style.css", nil)
	wrapped.ServeHTTP(rec, req)

	if !*called {
		t.Error("handler should have been called")
	}
	// Metrics context should not be set for static files
	if ma := getMetricsAttrs(req.Context()); ma != nil {
		t.Error("metrics attrs should not be set for static files")
	}
}

func TestMetrics_SkipsHealthCheck(t *testing.T) {
	inst := newInstruments()
	inner, called := testHandler()
	wrapped := Metrics(inst)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	wrapped.ServeHTTP(rec, req)

	if !*called {
		t.Error("handler should have been called")
	}
	if ma := getMetricsAttrs(req.Context()); ma != nil {
		t.Error("metrics attrs should not be set for health check")
	}
}

func TestMetrics_SetsContextAttrs(t *testing.T) {
	inst := newInstruments()

	var capturedMA *metricsAttrs
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedMA = getMetricsAttrs(r.Context())
		if capturedMA != nil {
			// Simulate handler enrichment
			capturedMA.enrich("validate", "yaml", 1024)
		}
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Metrics(inst)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/validate", nil)
	wrapped.ServeHTTP(rec, req)

	if capturedMA == nil {
		t.Fatal("metrics attrs should be set for API paths")
	}
	if capturedMA.operation != "validate" {
		t.Errorf("operation = %q, want %q", capturedMA.operation, "validate")
	}
	if capturedMA.format != "yaml" {
		t.Errorf("format = %q, want %q", capturedMA.format, "yaml")
	}
	if capturedMA.inputBytes != 1024 {
		t.Errorf("inputBytes = %d, want 1024", capturedMA.inputBytes)
	}
}

func TestMetrics_DetectsSource(t *testing.T) {
	tests := []struct {
		name       string
		hxRequest  string
		wantSource string
	}{
		{"UI request", "true", "ui"},
		{"API request", "", "api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wire up a ManualReader so we can inspect recorded attributes
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			prev := otel.GetMeterProvider()
			otel.SetMeterProvider(mp)
			t.Cleanup(func() { otel.SetMeterProvider(prev) })

			inst := newInstruments()
			inner, called := testHandler()
			wrapped := Metrics(inst)(inner)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/validate", nil)
			if tt.hxRequest != "" {
				req.Header.Set("HX-Request", tt.hxRequest)
			}
			wrapped.ServeHTTP(rec, req)

			if !*called {
				t.Error("handler should have been called")
			}

			// Collect and inspect the recorded metrics
			var rm metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Fatal(err)
			}

			found := false
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name != "oastools.operation.count" {
						continue
					}
					sum, ok := m.Data.(metricdata.Sum[int64])
					if !ok {
						continue
					}
					for _, dp := range sum.DataPoints {
						if v, ok := dp.Attributes.Value(attribute.Key("source")); ok {
							found = true
							if v.AsString() != tt.wantSource {
								t.Errorf("source = %q, want %q", v.AsString(), tt.wantSource)
							}
						}
					}
				}
			}
			if !found {
				t.Error("source attribute not found in recorded metrics")
			}
		})
	}
}

func TestOperationFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/validate", "validate"},
		{"/api/convert", "convert"},
		{"/api/diff", "diff"},
		{"/api/fix", "fix"},
		{"/api/join", "join"},
		{"/api/overlay", "overlay"},
		{"/api/explore", "explore"},
		{"/api/explore/operations", "explore"},
		{"/api/explore/schemas", "explore"},
		{"/api/spec", "spec"},
		{"/", "unknown"},
		{"/health", "unknown"},
		{"/static/style.css", "unknown"},
		{"/api/", "unknown"},
		{"/api/nonexistent", "unknown"},
		{"", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := operationFromPath(tt.path)
			if got != tt.want {
				t.Errorf("operationFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestMetrics_CapturesErrorStatus(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(prev) })

	inst := newInstruments()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	})

	wrapped := Metrics(inst)(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/validate", nil)
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("got status %d, want 400", rec.Code)
	}

	// Verify requestErrors counter was incremented
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "oastools.operation.errors" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				continue
			}
			for _, dp := range sum.DataPoints {
				if dp.Value >= 1 {
					found = true
					if v, ok := dp.Attributes.Value(attribute.Key("error_code")); ok {
						if v.AsInt64() != http.StatusBadRequest {
							t.Errorf("error_code = %d, want %d", v.AsInt64(), http.StatusBadRequest)
						}
					}
				}
			}
		}
	}
	if !found {
		t.Error("requestErrors metric not recorded for 400 response")
	}
}
