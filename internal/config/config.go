package config

import (
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	Port                  string
	LogLevel              slog.Level
	RateLimitRPM          int
	RateLimitBurst        int
	MaxFileSize           int64
	RequestTimeout        time.Duration
	MaxConcurrentRequests int
}

// Load creates a Config from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:                  getEnv("PORT", "8080"),
		LogLevel:              parseLogLevel(getEnv("LOG_LEVEL", "info")),
		RateLimitRPM:          getEnvInt("RATE_LIMIT_RPM", 60),
		RateLimitBurst:        getEnvInt("RATE_LIMIT_BURST", 10),
		MaxFileSize:           getEnvInt64("MAX_FILE_SIZE", 2<<20), // 2MB
		RequestTimeout:        getEnvDuration("REQUEST_TIMEOUT", 30*time.Second),
		MaxConcurrentRequests: getEnvInt("MAX_CONCURRENT_REQUESTS", 10),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
