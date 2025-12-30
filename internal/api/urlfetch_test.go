package api

import (
	"net"
	"testing"
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
		{"IPv6 unique local fc00", "fc00::1", true},
		{"IPv6 unique local fd", "fd12:3456::1", true},
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
		{"text/html", "text/html", true},
		{"octet-stream", "application/octet-stream", true},
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
