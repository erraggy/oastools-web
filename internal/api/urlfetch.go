package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	urlFetchTimeout    = 10 * time.Second
	maxURLResponseSize = 2 * 1024 * 1024 // 2MB - same as file upload limit
)

// blockedHosts contains hosts that should never be fetched for security reasons.
// This includes localhost variants and cloud metadata endpoints.
var blockedHosts = map[string]bool{
	// Localhost variants
	"localhost": true,
	"127.0.0.1": true,
	"0.0.0.0":   true,
	"::1":       true,

	// Cloud metadata hostnames (prevent SSRF attacks)
	// Note: IP-based metadata endpoints are caught by isBlockedIP() after DNS resolution
	"metadata.google.internal":  true, // GCP
	"metadata.azure.internal":   true, // Azure
	"metadata.alibaba.internal": true, // Alibaba Cloud
	"kubernetes.default.svc":    true, // Kubernetes
	"kubernetes.default":        true, // Kubernetes
	"kubernetes":                true, // Kubernetes
	"instance-data":             true, // DigitalOcean
	"link-local":                true, // Link-local
}

// blockedHostPrefixes contains hostname prefixes that should be blocked.
var blockedHostPrefixes = []string{
	"10.",     // Private network
	"172.16.", // Private network (172.16.0.0 - 172.31.255.255 range)
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

	// Custom dialer that validates resolved IP addresses to prevent DNS rebinding attacks
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Resolve the address first
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			// Resolve DNS
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}

			// Check each resolved IP against blocklist
			for _, ip := range ips {
				if isBlockedIP(ip.IP) {
					return nil, fmt.Errorf("resolved IP %s is blocked (DNS rebinding protection)", ip.IP)
				}
			}

			// Connect using the first valid IP
			if len(ips) == 0 {
				return nil, fmt.Errorf("no addresses found for host")
			}

			// Dial using resolved IP
			resolvedAddr := net.JoinHostPort(ips[0].IP.String(), port)
			return dialer.DialContext(ctx, network, resolvedAddr)
		},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  true,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	return &URLFetcher{
		userAgent: userAgent,
		client: &http.Client{
			Timeout:   urlFetchTimeout,
			Transport: transport,
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

// isBlockedIP checks if an IP address should be blocked.
// This provides DNS rebinding protection by checking resolved IPs.
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Block loopback
	if ip.IsLoopback() {
		return true
	}

	// Block private networks
	if ip.IsPrivate() {
		return true
	}

	// Block link-local addresses
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Block unspecified addresses (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return true
	}

	// Block multicast
	if ip.IsMulticast() {
		return true
	}

	// Additional cloud metadata IP checks
	metadataIPs := []string{
		"169.254.169.254", // AWS/GCP/Azure metadata
		"169.254.170.2",   // ECS task metadata
		"169.254.169.123", // Amazon Time Sync
		"100.100.100.200", // Alibaba Cloud metadata
		"192.0.0.192",     // Oracle Cloud metadata
	}
	for _, blocked := range metadataIPs {
		if ip.Equal(net.ParseIP(blocked)) {
			return true
		}
	}

	return false
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

	// Create request with context using normalized URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
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
	defer func() { _ = resp.Body.Close() }()

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

	// Check if content exceeds size limit
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
	var hostOnly string

	// Use stdlib to split host:port - handles IPv4, IPv6, and bracketed IPv6
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = h
	} else {
		// No port - strip brackets from IPv6 if present (e.g., "[::1]")
		hostOnly = strings.Trim(host, "[]")
	}

	// Normalize to lowercase
	hostOnly = strings.ToLower(hostOnly)

	// Check exact match
	if blockedHosts[hostOnly] {
		return true
	}

	// Check prefixes using slices.ContainsFunc
	return slices.ContainsFunc(blockedHostPrefixes, func(prefix string) bool {
		return strings.HasPrefix(hostOnly, prefix)
	})
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
