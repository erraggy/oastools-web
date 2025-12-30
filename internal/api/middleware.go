package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter implements per-IP rate limiting using token buckets.
type RateLimiter struct {
	visitors sync.Map
	rate     rate.Limit
	burst    int
	done     chan struct{}
}

type visitorInfo struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // Unix nano timestamp for safe concurrent access
}

// NewRateLimiter creates a rate limiter with the given requests per minute and burst size.
func NewRateLimiter(rpm, burst int) *RateLimiter {
	rl := &RateLimiter{
		rate:  rate.Limit(float64(rpm) / 60.0),
		burst: burst,
		done:  make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the cleanup goroutine. Call during graceful shutdown.
func (rl *RateLimiter) Stop() {
	close(rl.done)
}

func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	now := time.Now().UnixNano()

	// Try to load existing visitor first
	if v, exists := rl.visitors.Load(ip); exists {
		vi, ok := v.(*visitorInfo)
		if !ok {
			slog.Error("unexpected type in rate limiter visitors map",
				"ip", ip,
				"type", fmt.Sprintf("%T", v),
			)
			// Fall through to create new limiter
		} else {
			vi.lastSeen.Store(now)
			return vi.limiter
		}
	}

	// Create new visitor atomically to prevent race condition
	newInfo := &visitorInfo{
		limiter: rate.NewLimiter(rl.rate, rl.burst),
	}
	newInfo.lastSeen.Store(now)

	// LoadOrStore ensures only one limiter is created per IP
	actual, _ := rl.visitors.LoadOrStore(ip, newInfo)
	vi := actual.(*visitorInfo)
	vi.lastSeen.Store(now)
	return vi.limiter
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-3 * time.Minute).UnixNano()
			rl.visitors.Range(func(key, value any) bool {
				vi, ok := value.(*visitorInfo)
				if !ok {
					slog.Error("unexpected type in rate limiter during cleanup",
						"key", key,
						"type", fmt.Sprintf("%T", value),
					)
					rl.visitors.Delete(key) // Clean up bad entry
					return true
				}
				if vi.lastSeen.Load() < cutoff {
					rl.visitors.Delete(key)
				}
				return true
			})
		}
	}
}

// Middleware returns an HTTP middleware that enforces rate limits.
// Static files and health checks are excluded from rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting for static files and health checks
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		ip := realIP(r)
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			slog.Warn("rate limit exceeded", "ip", ip, "path", r.URL.Path)
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP, respecting X-Forwarded-For from Cloud Run.
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs; first is the client
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Strip port from RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// Recovery catches panics and returns 500 Internal Server Error.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"method", r.Method,
					"path", r.URL.Path,
					"ip", realIP(r),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logging logs request details including method, path, status, and duration.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration", time.Since(start),
			"ip", realIP(r),
		)
	})
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController compatibility.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// RequestSizeLimiter returns middleware that enforces maximum request body size.
func RequestSizeLimiter(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				slog.Warn("request body too large",
					"content_length", r.ContentLength,
					"max_bytes", maxBytes,
					"ip", realIP(r),
					"path", r.URL.Path,
				)
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout returns middleware that enforces a request processing timeout.
func Timeout(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, d, "request timeout")
	}
}

// ConcurrencyLimiter returns middleware that limits concurrent requests globally.
func ConcurrencyLimiter(maxConcurrent int) func(http.Handler) http.Handler {
	sem := make(chan struct{}, maxConcurrent)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				next.ServeHTTP(w, r)
			default:
				slog.Warn("server busy, rejecting request",
					"max_concurrent", maxConcurrent,
					"ip", realIP(r),
					"path", r.URL.Path,
				)
				http.Error(w, "server busy", http.StatusServiceUnavailable)
			}
		})
	}
}

// StaticCache wraps a handler to add Cache-Control headers for static assets.
// Uses immutable caching since embedded files only change on deployment.
func StaticCache(maxAge time.Duration) func(http.Handler) http.Handler {
	cacheControl := fmt.Sprintf("public, max-age=%d, immutable", int(maxAge.Seconds()))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", cacheControl)
			next.ServeHTTP(w, r)
		})
	}
}
