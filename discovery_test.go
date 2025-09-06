package main

import (
	"net"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
)

func TestExtractRouterName(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		expected string
	}{
		{
			name:     "Standard FQDN",
			fqdn:     "ThreadRouter1._meshcop._udp.local.",
			expected: "ThreadRouter1",
		},
		{
			name:     "Simple name",
			fqdn:     "Router1",
			expected: "Router1",
		},
		{
			name:     "Name with underscores",
			fqdn:     "Thread_Border_Router._meshcop._udp.local.",
			expected: "Thread_Border_Router",
		},
		{
			name:     "Name with escaped spaces and parentheses",
			fqdn:     "Living\\ Room\\ Apple\\ TV\\ \\(4\\)._meshcop._udp.local.",
			expected: "Living Room Apple TV (4)",
		},
		{
			name:     "Empty string",
			fqdn:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRouterName(tt.fqdn)
			if result != tt.expected {
				t.Errorf("extractRouterName(%s) = %s, want %s", tt.fqdn, result, tt.expected)
			}
		})
	}
}

// TestExtractRouterNameEdgeCases tests edge cases for router name extraction
func TestExtractRouterNameEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		fqdn     string
		expected string
	}{
		{
			name:     "FQDN with multiple dots",
			fqdn:     "router.subdomain.domain.local.",
			expected: "router",
		},
		{
			name:     "FQDN with special characters",
			fqdn:     "router-123._meshcop._udp.local.",
			expected: "router-123",
		},
		{
			name:     "FQDN with numbers",
			fqdn:     "router123._meshcop._udp.local.",
			expected: "router123",
		},
		{
			name:     "Single dot",
			fqdn:     "router.",
			expected: "router",
		},
		{
			name:     "No dots",
			fqdn:     "router",
			expected: "router",
		},
		{
			name:     "Only dots",
			fqdn:     "...",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRouterName(tt.fqdn)
			if result != tt.expected {
				t.Errorf("extractRouterName(%s) = %s, want %s", tt.fqdn, result, tt.expected)
			}
		})
	}
}

// TestExtractIPv6Addresses tests the IPv6 address extraction from mDNS entries
func TestExtractIPv6Addresses(t *testing.T) {
	tests := []struct {
		name     string
		entry    *zeroconf.ServiceEntry
		expected int // Expected number of IPv6 addresses
	}{
		{
			name: "Entry with IPv6 addresses",
			entry: &zeroconf.ServiceEntry{
				AddrIPv4: []net.IP{net.ParseIP("192.168.1.1")},
				AddrIPv6: []net.IP{
					net.ParseIP("fd00:1234:5678:9abc::1"),
					net.ParseIP("fe80::1"),
				},
			},
			expected: 2,
		},
		{
			name: "Entry with only IPv4 addresses",
			entry: &zeroconf.ServiceEntry{
				AddrIPv4: []net.IP{net.ParseIP("192.168.1.1")},
				AddrIPv6: []net.IP{},
			},
			expected: 0,
		},
		{
			name: "Entry with no addresses",
			entry: &zeroconf.ServiceEntry{
				AddrIPv4: []net.IP{},
				AddrIPv6: []net.IP{},
			},
			expected: 0,
		},
		{
			name: "Entry with mixed valid and invalid IPv6",
			entry: &zeroconf.ServiceEntry{
				AddrIPv4: []net.IP{},
				AddrIPv6: []net.IP{
					net.ParseIP("fd00:1234:5678:9abc::1"),
					nil, // Invalid IP
					net.ParseIP("2001:4860:4860::8888"),
				},
			},
			expected: 2, // Should filter out nil IPs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractIPv6Addresses(tt.entry)
			if len(result) != tt.expected {
				t.Errorf("extractIPv6Addresses() returned %d addresses, want %d", len(result), tt.expected)
			}
		})
	}
}

// TestFormatDuration tests the duration formatting function
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "Seconds only",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "Minutes only",
			duration: 5 * time.Minute,
			expected: "5m",
		},
		{
			name:     "Hours only",
			duration: 2 * time.Hour,
			expected: "2h",
		},
		{
			name:     "Hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2h30m",
		},
		{
			name:     "Hours with zero minutes",
			duration: 3 * time.Hour,
			expected: "3h",
		},
		{
			name:     "Less than a minute",
			duration: 45 * time.Second,
			expected: "45s",
		},
		{
			name:     "Zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "Very short duration",
			duration: 500 * time.Millisecond,
			expected: "0s", // Less than 1 second rounds to 0
		},
		{
			name:     "Long duration with minutes",
			duration: 25*time.Hour + 45*time.Minute,
			expected: "25h45m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %s, want %s", tt.duration, result, tt.expected)
			}
		})
	}
}
