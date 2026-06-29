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

func TestExtractIPv6s(t *testing.T) {
	tests := []struct {
		name      string
		addrs     []net.IP
		wantCount int
	}{
		{
			name:      "Single global IPv6",
			addrs:     []net.IP{net.ParseIP("fd00:1234:5678:9abc::1")},
			wantCount: 1,
		},
		{
			name:      "IPv4 address filtered out",
			addrs:     []net.IP{net.ParseIP("192.168.1.1")},
			wantCount: 0,
		},
		{
			name:      "No addresses",
			addrs:     nil,
			wantCount: 0,
		},
		{
			name:      "Multiple IPv6 addresses",
			addrs:     []net.IP{net.ParseIP("2001:4860:4860::8888"), net.ParseIP("fd00::1")},
			wantCount: 2,
		},
		{
			name:      "Mixed IPv4 and IPv6",
			addrs:     []net.IP{net.ParseIP("192.168.1.1"), net.ParseIP("fd00::1")},
			wantCount: 1,
		},
		{
			name:      "Duplicate IPv6 addresses deduplicated",
			addrs:     []net.IP{net.ParseIP("fd00::1"), net.ParseIP("fd00::1")},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &zeroconf.ServiceEntry{
				AddrIPv6: tt.addrs,
			}
			result := extractIPv6s(entry)
			if len(result) != tt.wantCount {
				t.Errorf("extractIPv6s() returned %d IPs, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"Seconds only", 30 * time.Second, "30s"},
		{"Minutes only", 5 * time.Minute, "5m"},
		{"Hours only", 2 * time.Hour, "2h"},
		{"Hours and minutes", 2*time.Hour + 30*time.Minute, "2h30m"},
		{"Hours with zero minutes", 3 * time.Hour, "3h"},
		{"Less than a minute", 45 * time.Second, "45s"},
		{"One minute", time.Minute, "1m"},
		{"One hour", time.Hour, "1h"},
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
