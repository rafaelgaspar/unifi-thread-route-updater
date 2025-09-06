package main

import (
	"net"
	"testing"
)

func TestCalculateCIDR64(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{
			name:     "ULA address",
			ip:       "fd00:1234:5678:9abc::1",
			expected: "fd00:1234:5678:9abc::/64",
		},
		{
			name:     "Link-local address",
			ip:       "fe80::1",
			expected: "fe80::/64",
		},
		{
			name:     "Documentation address",
			ip:       "2001:db8::1",
			expected: "2001:db8::/64",
		},
		{
			name:     "Global unicast address",
			ip:       "2001:4860:4860::8888",
			expected: "2001:4860:4860::/64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP: %s", tt.ip)
			}

			result := calculateCIDR64(ip)
			if result != tt.expected {
				t.Errorf("calculateCIDR64(%s) = %s, want %s", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestGenerateRoutes(t *testing.T) {
	// Test data
	devices := []DeviceInfo{
		{
			Name:     "Device1",
			IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::1"),
			Services: []string{"_matter._tcp"},
		},
		{
			Name:     "Device2",
			IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::2"),
			Services: []string{"_matter._tcp"},
		},
	}

	routers := []ThreadBorderRouter{
		{
			Name:     "ThreadRouter1",
			IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::ff"),
			CIDR:     "fd00:1234:5678:9abc::/64",
		},
		{
			Name:     "ThreadRouter2",
			IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::fe"),
			CIDR:     "fd00:1234:5678:9abc::/64",
		},
	}

	routes := generateRoutes(devices, routers)

	// Should have 4 routes (2 devices × 2 routers in same CIDR)
	if len(routes) != 4 {
		t.Errorf("Expected 4 routes, got %d", len(routes))
	}

	// All routes should have the same CIDR
	expectedCIDR := "fd00:1234:5678:9abc::/64"
	for _, route := range routes {
		if route.CIDR != expectedCIDR {
			t.Errorf("Expected CIDR %s, got %s", expectedCIDR, route.CIDR)
		}
	}
}

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
