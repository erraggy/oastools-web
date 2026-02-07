package config

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear any env vars that might affect the test
	for _, key := range []string{
		"PORT", "LOG_LEVEL", "RATE_LIMIT_RPM", "RATE_LIMIT_BURST",
		"MAX_FILE_SIZE", "REQUEST_TIMEOUT", "MAX_CONCURRENT_REQUESTS",
		"METRICS_ENABLED",
	} {
		t.Setenv(key, "")
	}

	cfg := Load()

	tests := []struct {
		name     string
		got      any
		expected any
	}{
		{"Port", cfg.Port, "8080"},
		{"LogLevel", cfg.LogLevel, slog.LevelInfo},
		{"RateLimitRPM", cfg.RateLimitRPM, 60},
		{"RateLimitBurst", cfg.RateLimitBurst, 10},
		{"MaxFileSize", cfg.MaxFileSize, int64(2 << 20)}, // 2MB
		{"RequestTimeout", cfg.RequestTimeout, 30 * time.Second},
		{"MaxConcurrentRequests", cfg.MaxConcurrentRequests, 10},
		{"MetricsEnabled", cfg.MetricsEnabled, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

func TestLoad_EnvironmentOverrides(t *testing.T) {
	// Set all env vars to custom values
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("RATE_LIMIT_RPM", "120")
	t.Setenv("RATE_LIMIT_BURST", "20")
	t.Setenv("MAX_FILE_SIZE", "4194304") // 4MB
	t.Setenv("REQUEST_TIMEOUT", "1m")
	t.Setenv("MAX_CONCURRENT_REQUESTS", "25")
	t.Setenv("METRICS_ENABLED", "true")

	cfg := Load()

	tests := []struct {
		name     string
		got      any
		expected any
	}{
		{"Port", cfg.Port, "9090"},
		{"LogLevel", cfg.LogLevel, slog.LevelDebug},
		{"RateLimitRPM", cfg.RateLimitRPM, 120},
		{"RateLimitBurst", cfg.RateLimitBurst, 20},
		{"MaxFileSize", cfg.MaxFileSize, int64(4194304)},
		{"RequestTimeout", cfg.RequestTimeout, time.Minute},
		{"MaxConcurrentRequests", cfg.MaxConcurrentRequests, 25},
		{"MetricsEnabled", cfg.MetricsEnabled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

func TestGetEnvInt_InvalidValue(t *testing.T) {
	// When env var contains non-integer, should return default
	t.Setenv("RATE_LIMIT_RPM", "not-a-number")

	cfg := Load()

	if cfg.RateLimitRPM != 60 {
		t.Errorf("expected default 60, got %d", cfg.RateLimitRPM)
	}
}

func TestGetEnvInt64_InvalidValue(t *testing.T) {
	// When env var contains non-integer, should return default
	t.Setenv("MAX_FILE_SIZE", "invalid")

	cfg := Load()

	if cfg.MaxFileSize != 2<<20 {
		t.Errorf("expected default 2MB, got %d", cfg.MaxFileSize)
	}
}

func TestGetEnvDuration_InvalidValue(t *testing.T) {
	// When env var contains invalid duration, should return default
	t.Setenv("REQUEST_TIMEOUT", "xyz")

	cfg := Load()

	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("expected default 30s, got %v", cfg.RequestTimeout)
	}
}

func TestParseLogLevel_AllLevels(t *testing.T) {
	tests := []struct {
		level    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // Falls back to info
		{"", slog.LevelInfo},        // Empty falls back to info
		{"DEBUG", slog.LevelInfo},   // Case sensitive, falls back
		{"WARNING", slog.LevelInfo}, // Invalid, falls back
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", tt.level)
			cfg := Load()
			if cfg.LogLevel != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.level, cfg.LogLevel, tt.expected)
			}
		})
	}
}

func TestGetEnvBool_Values(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"TRUE", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"YES", true},
		{"false", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"NO", false},
		{"", false},        // Empty falls back to default (false)
		{"invalid", false}, // Unrecognized falls back to default (false)
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv("METRICS_ENABLED", tt.value)
			cfg := Load()
			if cfg.MetricsEnabled != tt.expected {
				t.Errorf("getEnvBool(%q) = %v, want %v", tt.value, cfg.MetricsEnabled, tt.expected)
			}
		})
	}
}

func TestGetEnv_EmptyVsFallback(t *testing.T) {
	// Test that empty string returns default, not empty
	t.Setenv("PORT", "")
	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default '8080' for empty PORT, got %q", cfg.Port)
	}
}
