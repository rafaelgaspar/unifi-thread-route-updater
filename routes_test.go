package main

import (
	"net"
	"testing"
	"time"
)

func prefixMap(prefixes ...string) map[string]time.Time {
	m := make(map[string]time.Time)
	for _, p := range prefixes {
		m[p] = time.Now()
	}
	return m
}

func TestGenerateRoutes(t *testing.T) {
	prefixes := prefixMap("fd00:1111:2222:3333::/64")

	routers := []ThreadBorderRouter{
		{Name: "ThreadRouter1", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::ff")}},
		{Name: "ThreadRouter2", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::fe")}},
	}

	routes := generateRoutes(prefixes, routers)

	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}
	for _, route := range routes {
		if route.CIDR != "fd00:1111:2222:3333::/64" {
			t.Errorf("Expected CIDR fd00:1111:2222:3333::/64, got %s", route.CIDR)
		}
	}
}

func TestGenerateRoutesEdgeCases(t *testing.T) {
	t.Run("No prefixes", func(t *testing.T) {
		routes := generateRoutes(nil, []ThreadBorderRouter{
			{Name: "Router1", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::ff")}},
		})
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with no prefixes, got %d", len(routes))
		}
	})

	t.Run("No routers", func(t *testing.T) {
		routes := generateRoutes(prefixMap("fd00:1234:5678:9abc::/64"), nil)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with no routers, got %d", len(routes))
		}
	})

	t.Run("Multiple prefixes with multiple routers", func(t *testing.T) {
		prefixes := prefixMap("fd00:1111:2222:3333::/64", "fd00:4444:5555:6666::/64")
		routers := []ThreadBorderRouter{
			{Name: "Router1", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::ff")}},
			{Name: "Router2", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::fe")}},
		}

		routes := generateRoutes(prefixes, routers)
		expected := len(prefixes) * len(routers) // 4 routes
		if len(routes) != expected {
			t.Errorf("Expected %d routes, got %d", expected, len(routes))
		}
	})

	t.Run("Router with non-routable IP generates no routes", func(t *testing.T) {
		routes := generateRoutes(
			prefixMap("fd00:1111:2222:3333::/64"),
			[]ThreadBorderRouter{
				{Name: "Router1", IPv6Addrs: []net.IP{net.ParseIP("fe80::1")}},
			},
		)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with non-routable router IP, got %d", len(routes))
		}
	})

	t.Run("Router with mixed IPs uses only routable ones", func(t *testing.T) {
		routes := generateRoutes(
			prefixMap("fd00:1111:2222:3333::/64"),
			[]ThreadBorderRouter{
				{
					Name: "Router1",
					IPv6Addrs: []net.IP{
						net.ParseIP("fe80::1"),                 // link-local, skipped
						net.ParseIP("2001:4860:4860:1234::ff"), // routable
					},
				},
			},
		)
		if len(routes) != 1 {
			t.Errorf("Expected 1 route (only routable IP used), got %d", len(routes))
		}
	})

	t.Run("Deduplication: same prefix produces one route per router", func(t *testing.T) {
		// Maps deduplicate keys automatically
		prefixes := prefixMap("fd00:1111:2222:3333::/64")
		routes := generateRoutes(
			prefixes,
			[]ThreadBorderRouter{
				{Name: "Router1", IPv6Addrs: []net.IP{net.ParseIP("2001:4860:4860:1234::ff")}},
			},
		)
		if len(routes) != 1 {
			t.Errorf("Expected 1 route, got %d", len(routes))
		}
	})
}

func TestCalculateCIDR64(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected string
	}{
		{"ULA address", "fd00:1234:5678:9abc::1", "fd00:1234:5678:9abc::/64"},
		{"Link-local address", "fe80::1", "fe80::/64"},
		{"Documentation address", "2001:db8::1", "2001:db8::/64"},
		{"Global unicast address", "2001:4860:4860::8888", "2001:4860:4860::/64"},
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

func TestCalculateCIDR64EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		expected   string
		shouldFail bool
	}{
		{"IPv4 address returns empty string", "192.168.1.1", "", false},
		{"Invalid IP should fail", "invalid-ip", "", true},
		{"Empty string should fail", "", "", true},
		{"IPv6 with /128 prefix", "2001:db8::1", "2001:db8::/64", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if tt.shouldFail {
				if ip != nil {
					t.Errorf("Expected IP parsing to fail for %s, but got %v", tt.ip, ip)
				}
				return
			}
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

func TestIsRoutableCIDR(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		expected bool
	}{
		{"ULA CIDR", "fd00:1234:5678:9abc::/64", true},
		{"Global unicast CIDR", "2001:4860:4860::/64", true},
		{"Link-local CIDR", "fe80::/64", false},
		{"Loopback CIDR", "::1/128", false},
		{"Multicast CIDR", "ff00::/8", false},
		{"Documentation CIDR", "2001:db8::/32", false},
		{"Invalid CIDR", "invalid", false},
		{"Empty CIDR", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRoutableCIDR(tt.cidr)
			if result != tt.expected {
				t.Errorf("isRoutableCIDR(%s) = %v, want %v", tt.cidr, result, tt.expected)
			}
		})
	}
}

func TestIsRoutableRouterAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"Public IPv6", "2001:4860:4860::1", true},
		{"ULA address", "fd00:1234:5678:9abc::1", false},
		{"Link-local", "fe80::1", false},
		{"Loopback", "::1", false},
		{"Unspecified", "::", false},
		{"Multicast", "ff02::1", false},
		{"Documentation", "2001:db8::1", false},
		{"Teredo", "2001::1", false},
		{"6to4", "2002::1", false},
		{"IPv4", "192.168.1.1", false},
		{"Nil IP", "", false},
		{"Invalid IP", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip net.IP
			if tt.ip != "" {
				ip = net.ParseIP(tt.ip)
			}
			result := isRoutableRouterAddress(ip)
			if result != tt.expected {
				t.Errorf("isRoutableRouterAddress(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

