package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"
)

const (
	urlFetchTimeout    = 10 * time.Second
	maxURLResponseSize = 2 << 20 // 2MB - same as file upload limit
)

// blockedHosts contains hosts that should never be fetched for security reasons.
// This includes localhost variants and cloud metadata endpoints.
var blockedHosts = map[string]bool{
	// Localhost variants
	"localhost": true,
	"127.0.0.1": true,
	"0.0.0.0":   true,
	"::1":       true,

	// Cloud metadata endpoints (prevent SSRF attacks)
	"metadata.google.internal":     true, // GCP
	"169.254.169.254":              true, // AWS/GCP/Azure metadata
	"metadata.azure.internal":      true, // Azure
	"metadata.alibaba.internal":    true, // Alibaba Cloud
	"100.100.100.200":              true, // Alibaba Cloud metadata
	"fd00:ec2::254":                true, // AWS IPv6 metadata
	"169.254.170.2":                true, // ECS task metadata
	"kubernetes.default.svc":       true, // Kubernetes
	"kubernetes.default":           true, // Kubernetes
	"kubernetes":                   true, // Kubernetes
	"instance-data":                true, // DigitalOcean
	"169.254.169.123":              true, // Amazon Time Sync
	"2600:1f18:4254:5100::1":       true, // AWS NTP IPv6
	"fd00:1::1":                    true, // Oracle Cloud metadata
	"192.0.0.192":                  true, // Oracle Cloud metadata
	"link-local":                   true, // Link-local
}

// blockedHostPrefixes contains hostname prefixes that should be blocked.
var blockedHostPrefixes = []string{
	"10.",      // Private network
	"172.16.",  // Private network (172.16.0.0 - 172.31.255.255 range)
	"172.17.",
	"172.18.",
	"172.19.",
	"172.20.",
	"172.21.",
	"172.22.",
	"172.23.",
	"172.24.",
	"172.25.",
	"172.26.",
	"172.27.",
	"172.28.",
	"172.29.",
	"172.30.",
	"172.31.",
	"192.168.", // Private network
	"fc00:",    // IPv6 unique local
	"fd",       // IPv6 unique local
	"fe80:",    // IPv6 link-local
}

// URLFetcher fetches content from URLs with security controls.
type URLFetcher struct {
	client    *http.Client
	userAgent string
}

// NewURLFetcher creates a new URL fetcher with security controls.
func NewURLFetcher(version, oastoolsVersion string) *URLFetcher {
	// Build User-Agent with version and Go runtime info
	userAgent := fmt.Sprintf("oastools-web/%s (oastools/%s; Go/%s; %s/%s)",
		version, oastoolsVersion, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	return &URLFetcher{
		userAgent: userAgent,
		client: &http.Client{
			Timeout: urlFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Limit redirects
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects (max 5)")
				}
				// Check if redirect target is blocked
				if isBlockedHost(req.URL.Host) {
					return fmt.Errorf("redirect to blocked host: %s", req.URL.Host)
				}
				return nil
			},
		},
	}
}

// Fetch retrieves content from a URL with security validation.
func (f *URLFetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	// Parse and validate URL
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Normalize scheme
	parsed.Scheme = strings.ToLower(parsed.Scheme)

	// Only allow HTTP/HTTPS
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("only http/https URLs are supported, got: %s", parsed.Scheme)
	}

	// Check for blocked hosts
	if isBlockedHost(parsed.Host) {
		return nil, fmt.Errorf("blocked host: %s", parsed.Host)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("User-Agent", f.userAgent)
	req.Header.Set("Accept", "application/json, application/yaml, application/x-yaml, text/yaml, text/plain, */*")

	// Perform request
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Validate content type if provided
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !isValidContentType(contentType) {
		return nil, fmt.Errorf("unexpected content type: %s (expected JSON or YAML)", contentType)
	}

	// Read response with size limit
	// Read one extra byte to detect if content exceeds limit
	limited := io.LimitReader(resp.Body, maxURLResponseSize+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if we hit the size limit
	if len(content) > maxURLResponseSize {
		return nil, fmt.Errorf("response exceeds maximum size of %d bytes", maxURLResponseSize)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("response body is empty")
	}

	return content, nil
}

// isBlockedHost checks if a host should be blocked.
func isBlockedHost(host string) bool {
	// Remove port if present
	hostOnly := host
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		// Handle IPv6 addresses with brackets
		if bracketIdx := strings.LastIndex(host, "]"); bracketIdx > colonIdx {
			// Port is after the bracket, e.g., [::1]:8080
			hostOnly = host[:colonIdx]
		} else if !strings.Contains(host[:colonIdx], ":") {
			// Regular hostname:port or IPv4:port
			hostOnly = host[:colonIdx]
		}
		// If host contains multiple colons without brackets, it's an IPv6 without port
	}

	// Normalize to lowercase
	hostOnly = strings.ToLower(hostOnly)

	// Remove brackets from IPv6 addresses
	hostOnly = strings.Trim(hostOnly, "[]")

	// Check exact match
	if blockedHosts[hostOnly] {
		return true
	}

	// Check prefixes
	for _, prefix := range blockedHostPrefixes {
		if strings.HasPrefix(hostOnly, prefix) {
			return true
		}
	}

	return false
}

// isValidContentType checks if a content type is acceptable for OpenAPI specs.
func isValidContentType(contentType string) bool {
	// Normalize to lowercase and get just the MIME type
	ct := strings.ToLower(contentType)
	if semicolonIdx := strings.Index(ct, ";"); semicolonIdx != -1 {
		ct = strings.TrimSpace(ct[:semicolonIdx])
	}

	validTypes := []string{
		"application/json",
		"application/yaml",
		"application/x-yaml",
		"text/yaml",
		"text/x-yaml",
		"text/plain",
		"text/html", // Some servers incorrectly serve YAML as HTML
		"application/octet-stream",
	}

	for _, valid := range validTypes {
		if ct == valid {
			return true
		}
	}

	return false
}
