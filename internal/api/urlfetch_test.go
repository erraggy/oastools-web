package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsBlockedHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		blocked bool
	}{
		// Localhost variants
		{"localhost", "localhost", true},
		{"localhost with port", "localhost:8080", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1 with port", "127.0.0.1:8080", true},
		{"0.0.0.0", "0.0.0.0", true},
		{"::1", "::1", true},
		{"::1 with brackets", "[::1]", true},
		{"::1 with brackets and port", "[::1]:8080", true},

		// Cloud metadata hostnames (IPs are caught by isBlockedIP after DNS resolution)
		{"GCP metadata", "metadata.google.internal", true},
		{"Azure metadata", "metadata.azure.internal", true},
		{"Kubernetes default", "kubernetes.default.svc", true},

		// Private network ranges
		{"10.x.x.x", "10.0.0.1", true},
		{"10.x.x.x with port", "10.255.255.255:443", true},
		{"172.16.x.x", "172.16.0.1", true},
		{"172.31.x.x", "172.31.255.255", true},
		{"192.168.x.x", "192.168.0.1", true},
		{"192.168.x.x with port", "192.168.1.1:8080", true},

		// IPv6 private ranges
		// Note: fc00::/7 (unique local) addresses are NOT blocked by isBlockedHost
		// because the range can't be efficiently prefix-matched. They're caught by
		// isBlockedIP() after DNS resolution via ip.IsPrivate().
		{"IPv6 unique local fc00", "fc00::1", false},
		{"IPv6 unique local fd", "fd12:3456::1", false},
		{"IPv6 link-local", "fe80::1", true},

		// Public addresses (should not be blocked)
		{"google.com", "google.com", false},
		{"google.com with port", "google.com:443", false},
		{"GitHub", "raw.githubusercontent.com", false},
		{"public IP", "8.8.8.8", false},
		{"public IP with port", "1.1.1.1:443", false},
		{"example.com", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBlockedHost(tt.host)
			if got != tt.blocked {
				t.Errorf("isBlockedHost(%q) = %v, want %v", tt.host, got, tt.blocked)
			}
		})
	}
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		blocked bool
	}{
		// Loopback
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv6 loopback", "::1", true},

		// Unspecified
		{"IPv4 unspecified", "0.0.0.0", true},
		{"IPv6 unspecified", "::", true},

		// Private networks
		{"10.x.x.x", "10.0.0.1", true},
		{"172.16.x.x", "172.16.0.1", true},
		{"172.31.x.x", "172.31.255.255", true},
		{"192.168.x.x", "192.168.1.1", true},

		// Link-local
		{"IPv4 link-local", "169.254.1.1", true},
		{"IPv6 link-local", "fe80::1", true},

		// Cloud metadata IPs
		{"AWS/GCP/Azure metadata", "169.254.169.254", true},
		{"ECS task metadata", "169.254.170.2", true},
		{"Amazon Time Sync", "169.254.169.123", true},
		{"Alibaba Cloud metadata", "100.100.100.200", true},
		{"Oracle Cloud metadata", "192.0.0.192", true},

		// Multicast
		{"IPv4 multicast", "224.0.0.1", true},
		{"IPv6 multicast", "ff02::1", true},

		// Public IPs (should not be blocked)
		{"Google DNS", "8.8.8.8", false},
		{"Cloudflare DNS", "1.1.1.1", false},
		{"Public IPv6", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}
			got := isBlockedIP(ip)
			if got != tt.blocked {
				t.Errorf("isBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
			}
		})
	}
}

func TestIsBlockedIP_NilIP(t *testing.T) {
	if !isBlockedIP(nil) {
		t.Error("isBlockedIP(nil) should return true")
	}
}

func TestIsValidContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		valid       bool
	}{
		{"JSON", "application/json", true},
		{"JSON with charset", "application/json; charset=utf-8", true},
		{"YAML", "application/yaml", true},
		{"YAML x-", "application/x-yaml", true},
		{"text/yaml", "text/yaml", true},
		{"text/x-yaml", "text/x-yaml", true},
		{"text/plain", "text/plain", true},
		{"octet-stream", "application/octet-stream", true},
		{"text/html rejected", "text/html", false},
		{"invalid type", "image/png", false},
		{"executable", "application/x-executable", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidContentType(tt.contentType)
			if got != tt.valid {
				t.Errorf("isValidContentType(%q) = %v, want %v", tt.contentType, got, tt.valid)
			}
		})
	}
}

func TestExtractFilenameFromURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"simple filename", "https://example.com/api.yaml", "api.yaml"},
		{"with query string", "https://example.com/spec.json?version=2", "spec.json"},
		{"nested path", "https://github.com/owner/repo/blob/main/openapi.yaml", "openapi.yaml"},
		{"no filename", "https://example.com/", "remote-spec"},
		{"trailing slash", "https://example.com/api/", "remote-spec"},
		{"just host", "https://example.com", "remote-spec"},
		{"invalid URL", "://invalid", "remote-spec"},
		{"empty path", "https://example.com", "remote-spec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFilenameFromURL(tt.url)
			if got != tt.expected {
				t.Errorf("extractFilenameFromURL(%q) = %q, want %q", tt.url, got, tt.expected)
			}
		})
	}
}

// Integration tests for URLFetcher.Fetch() using httptest

// newTestURLFetcher creates a URLFetcher that bypasses SSRF protections for testing.
// This allows tests to use httptest.NewServer which binds to 127.0.0.1.
// Only use this for testing non-security functionality.
func newTestURLFetcher() *URLFetcher {
	return &URLFetcher{
		userAgent:     "test-agent",
		skipHostCheck: true, // bypass SSRF check for localhost test servers
		client: &http.Client{
			Timeout: urlFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects (max 5)")
				}
				return nil
			},
		},
	}
}

func TestURLFetcher_Fetch_Success(t *testing.T) {
	content := `{"openapi": "3.0.0"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	result, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != content {
		t.Errorf("got %q, want %q", string(result), content)
	}
}

func TestURLFetcher_Fetch_InvalidScheme(t *testing.T) {
	fetcher := NewURLFetcher("test", "test")

	tests := []struct {
		name   string
		url    string
		errMsg string
	}{
		{"file scheme", "file:///etc/passwd", "only http/https URLs are supported"},
		{"ftp scheme", "ftp://example.com/spec.yaml", "only http/https URLs are supported"},
		{"javascript scheme", "javascript:alert(1)", "only http/https URLs are supported"},
		{"data scheme", "data:text/plain,hello", "only http/https URLs are supported"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fetcher.Fetch(context.Background(), tt.url)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestURLFetcher_Fetch_BlockedHost(t *testing.T) {
	fetcher := NewURLFetcher("test", "test")

	tests := []struct {
		name string
		url  string
	}{
		{"localhost", "http://localhost/spec.yaml"},
		{"127.0.0.1", "http://127.0.0.1/spec.yaml"},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/"},
		{"private 10.x", "http://10.0.0.1/spec.yaml"},
		{"private 192.168.x", "http://192.168.1.1/spec.yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fetcher.Fetch(context.Background(), tt.url)
			if err == nil {
				t.Fatal("expected error for blocked host, got nil")
			}
			if !strings.Contains(err.Error(), "blocked") {
				t.Errorf("error %q should indicate host is blocked", err.Error())
			}
		})
	}
}

func TestURLFetcher_Fetch_NonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"not found", http.StatusNotFound},
		{"server error", http.StatusInternalServerError},
		{"forbidden", http.StatusForbidden},
		{"unauthorized", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			fetcher := newTestURLFetcher()
			_, err := fetcher.Fetch(context.Background(), server.URL)
			if err == nil {
				t.Fatal("expected error for non-200 status, got nil")
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("status %d", tt.statusCode)) {
				t.Errorf("error %q should mention status code %d", err.Error(), tt.statusCode)
			}
		})
	}
}

func TestURLFetcher_Fetch_InvalidContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for invalid content type, got nil")
	}
	if !strings.Contains(err.Error(), "content type") {
		t.Errorf("error %q should mention content type", err.Error())
	}
}

func TestURLFetcher_Fetch_SizeLimit(t *testing.T) {
	// Create content larger than maxURLResponseSize (2MB)
	largeContent := strings.Repeat("x", maxURLResponseSize+1000)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeContent))
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for oversized response, got nil")
	}
	if !strings.Contains(err.Error(), "maximum size") {
		t.Errorf("error %q should mention maximum size", err.Error())
	}
}

func TestURLFetcher_Fetch_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write nothing
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error %q should mention empty", err.Error())
	}
}

func TestURLFetcher_Fetch_TooManyRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always redirect to self
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error for too many redirects, got nil")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("error %q should mention redirect", err.Error())
	}
}

func TestURLFetcher_Fetch_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	fetcher := newTestURLFetcher()
	_, err := fetcher.Fetch(ctx, server.URL)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	// Error should indicate context deadline or cancellation
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "deadline") && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error %q should indicate context cancellation", err.Error())
	}
}

func TestURLFetcher_Fetch_NoContentTypeHeader(t *testing.T) {
	// Server that doesn't set Content-Type should succeed (we accept empty content-type)
	content := `{"openapi": "3.0.0"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't set Content-Type header
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
	defer server.Close()

	fetcher := newTestURLFetcher()
	result, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != content {
		t.Errorf("got %q, want %q", string(result), content)
	}
}

func TestURLFetcher_UserAgent(t *testing.T) {
	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	// Create a test fetcher with custom user agent to verify it's sent
	fetcher := &URLFetcher{
		userAgent:     "oastools-web/1.0.0 (oastools/2.0.0)",
		skipHostCheck: true,
		client:        &http.Client{Timeout: urlFetchTimeout},
	}
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify User-Agent contains version info
	if !strings.Contains(receivedUserAgent, "oastools-web/1.0.0") {
		t.Errorf("User-Agent %q should contain oastools-web version", receivedUserAgent)
	}
	if !strings.Contains(receivedUserAgent, "oastools/2.0.0") {
		t.Errorf("User-Agent %q should contain oastools version", receivedUserAgent)
	}
}
