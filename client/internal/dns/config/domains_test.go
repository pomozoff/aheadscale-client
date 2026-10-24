package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractValidDomain(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expected    string
		expectError bool
	}{
		{
			name:     "HTTPS URL with port",
			url:      "https://api.example.com:443",
			expected: "api.example.com",
		},
		{
			name:     "HTTP URL without port",
			url:      "http://signal.example.com",
			expected: "signal.example.com",
		},
		{
			name:     "Host with port (no scheme)",
			url:      "signal.example.com:443",
			expected: "signal.example.com",
		},
		{
			name:     "STUN URL",
			url:      "stun:stun.example.com:443",
			expected: "stun.example.com",
		},
		{
			name:     "STUN URL with different port",
			url:      "stun:stun.example.com:5555",
			expected: "stun.example.com",
		},
		{
			name:     "TURNS URL with query params",
			url:      "turns:turn.example.com:443?transport=tcp",
			expected: "turn.example.com",
		},
		{
			name:     "TURN URL",
			url:      "turn:turn.example.com:3478",
			expected: "turn.example.com",
		},
		{
			name:     "REL URL",
			url:      "rel://relay.example.com:443",
			expected: "relay.example.com",
		},
		{
			name:     "RELS URL",
			url:      "rels://relay.example.com:443",
			expected: "relay.example.com",
		},
		{
			name:     "Raw hostname",
			url:      "example.org",
			expected: "example.org",
		},
		{
			name:        "IP address should be rejected",
			url:         "192.168.1.1",
			expectError: true,
		},
		{
			name:        "IP address with port should be rejected",
			url:         "192.168.1.1:443",
			expectError: true,
		},
		{
			name:        "IPv6 address should be rejected",
			url:         "2001:db8::1",
			expectError: true,
		},
		{
			name:        "HTTP URL with IPv4 should be rejected",
			url:         "http://192.168.1.1:8080",
			expectError: true,
		},
		{
			name:        "HTTPS URL with IPv4 should be rejected",
			url:         "https://10.0.0.1:443",
			expectError: true,
		},
		{
			name:        "STUN URL with IPv4 should be rejected",
			url:         "stun:192.168.1.1:3478",
			expectError: true,
		},
		{
			name:        "TURN URL with IPv4 should be rejected",
			url:         "turn:10.0.0.1:3478",
			expectError: true,
		},
		{
			name:        "TURNS URL with IPv4 should be rejected",
			url:         "turns:172.16.0.1:5349",
			expectError: true,
		},
		{
			name:        "HTTP URL with IPv6 should be rejected",
			url:         "http://[2001:db8::1]:8080",
			expectError: true,
		},
		{
			name:        "HTTPS URL with IPv6 should be rejected",
			url:         "https://[::1]:443",
			expectError: true,
		},
		{
			name:        "STUN URL with IPv6 should be rejected",
			url:         "stun:[2001:db8::1]:3478",
			expectError: true,
		},
		{
			name:        "IPv6 with port should be rejected",
			url:         "[2001:db8::1]:443",
			expectError: true,
		},
		{
			name:        "Localhost IPv4 should be rejected",
			url:         "127.0.0.1:8080",
			expectError: true,
		},
		{
			name:        "Localhost IPv6 should be rejected",
			url:         "[::1]:443",
			expectError: true,
		},
		{
			name:        "REL URL with IPv4 should be rejected",
			url:         "rel://192.168.1.1:443",
			expectError: true,
		},
		{
			name:        "RELS URL with IPv4 should be rejected",
			url:         "rels://10.0.0.1:443",
			expectError: true,
		},
		{
			name:        "Empty URL",
			url:         "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractValidDomain(tt.url)

			if tt.expectError {
				assert.Error(t, err, "Expected error for URL: %s", tt.url)
			} else {
				assert.NoError(t, err, "Unexpected error for URL: %s", tt.url)
				assert.Equal(t, tt.expected, result.SafeString(), "Domain mismatch for URL: %s", tt.url)
			}
		})
	}
}

func TestExtractDomainFromHost(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		expected    string
		expectError bool
	}{
		{
			name:     "Valid domain",
			host:     "example.com",
			expected: "example.com",
		},
		{
			name:     "Subdomain",
			host:     "api.example.com",
			expected: "api.example.com",
		},
		{
			name:        "IPv4 address",
			host:        "192.168.1.1",
			expectError: true,
		},
		{
			name:        "IPv6 address",
			host:        "2001:db8::1",
			expectError: true,
		},
		{
			name:        "Empty host",
			host:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractDomainFromHost(tt.host)

			if tt.expectError {
				assert.Error(t, err, "Expected error for host: %s", tt.host)
			} else {
				assert.NoError(t, err, "Unexpected error for host: %s", tt.host)
				assert.Equal(t, tt.expected, result.SafeString(), "Domain mismatch for host: %s", tt.host)
			}
		})
	}
}
