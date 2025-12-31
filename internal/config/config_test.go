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

func TestGetEnv_EmptyVsFallback(t *testing.T) {
	// Test that empty string returns default, not empty
	t.Setenv("PORT", "")
	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default '8080' for empty PORT, got %q", cfg.Port)
	}
}
