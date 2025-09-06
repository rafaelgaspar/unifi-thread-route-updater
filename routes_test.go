package main

import (
	"net"
	"testing"
)

func TestGenerateRoutes(t *testing.T) {
	// Test data - devices and routers in different CIDRs to generate routes
	devices := []DeviceInfo{
		{
			Name:     "Device1",
			IPv6Addr: net.ParseIP("fd00:1111:2222:3333::1"), // Different CIDR from routers
			Services: []string{"_matter._tcp"},
		},
		{
			Name:     "Device2",
			IPv6Addr: net.ParseIP("fd00:1111:2222:3333::2"), // Same CIDR as Device1
			Services: []string{"_matter._tcp"},
		},
	}

	routers := []ThreadBorderRouter{
		{
			Name:     "ThreadRouter1",
			IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"), // Different CIDR from devices, public IPv6
			CIDR:     "2001:4860:4860:1234::/64",
		},
		{
			Name:     "ThreadRouter2",
			IPv6Addr: net.ParseIP("2001:4860:4860:1234::fe"), // Same CIDR as ThreadRouter1, public IPv6
			CIDR:     "2001:4860:4860:1234::/64",
		},
	}

	routes := generateRoutes(devices, routers)

	// Should have 2 routes (1 device CIDR × 2 routers, devices in different CIDR from routers)
	if len(routes) != 2 {
		t.Errorf("Expected 2 routes, got %d", len(routes))
	}

	// All routes should have the device CIDR
	expectedCIDR := "fd00:1111:2222:3333::/64"
	for _, route := range routes {
		if route.CIDR != expectedCIDR {
			t.Errorf("Expected CIDR %s, got %s", expectedCIDR, route.CIDR)
		}
	}
}

// TestGenerateRoutesEdgeCases tests edge cases for route generation
func TestGenerateRoutesEdgeCases(t *testing.T) {
	t.Run("No devices", func(t *testing.T) {
		devices := []DeviceInfo{}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"),
				CIDR:     "2001:4860:4860:1234::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with no devices, got %d", len(routes))
		}
	})

	t.Run("No routers", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::1"),
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with no routers, got %d", len(routes))
		}
	})

	t.Run("Devices and routers in different CIDRs", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1234:5678:9abc::1"),
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:5678::ff"),
				CIDR:     "2001:4860:4860:5678::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		// Should have 1 route (device CIDR -> router)
		if len(routes) != 1 {
			t.Errorf("Expected 1 route with devices and routers in different CIDRs, got %d", len(routes))
		}
	})

	t.Run("Multiple devices in same CIDR with multiple routers", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::1"), // Different CIDR from routers
				Services: []string{"_matter._tcp"},
			},
			{
				Name:     "Device2",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::2"), // Same CIDR as Device1
				Services: []string{"_matter._tcp"},
			},
			{
				Name:     "Device3",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::3"), // Same CIDR as Device1
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"), // Different CIDR from devices, public IPv6
				CIDR:     "2001:4860:4860:1234::/64",
			},
			{
				Name:     "Router2",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::fe"), // Same CIDR as Router1, public IPv6
				CIDR:     "2001:4860:4860:1234::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		expected := len(routers) // 2 routes (one device CIDR -> 2 routers)
		if len(routes) != expected {
			t.Errorf("Expected %d routes, got %d", expected, len(routes))
		}

		// Verify all routes have correct structure
		for _, route := range routes {
			if route.CIDR != "fd00:1111:2222:3333::/64" {
				t.Errorf("Expected CIDR fd00:1111:2222:3333::/64, got %s", route.CIDR)
			}
			if route.RouterName == "" {
				t.Error("Expected router name to be set")
			}
			if route.ThreadRouterIPv6 == "" {
				t.Error("Expected Thread router IPv6 to be set")
			}
		}
	})
}

// TestGenerateRoutesEdgeCasesAdvanced tests more edge cases for route generation
func TestGenerateRoutesEdgeCasesAdvanced(t *testing.T) {
	t.Run("Devices with invalid IPv6 addresses", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: nil, // Invalid IP
				Services: []string{"_matter._tcp"},
			},
			{
				Name:     "Device2",
				IPv6Addr: net.ParseIP("192.168.1.1"), // IPv4 address
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"),
				CIDR:     "2001:4860:4860:1234::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with invalid device IPs, got %d", len(routes))
		}
	})

	t.Run("Routers with invalid IPv6 addresses", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::1"),
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: nil, // Invalid IP
				CIDR:     "2001:4860:4860:1234::/64",
			},
			{
				Name:     "Router2",
				IPv6Addr: net.ParseIP("192.168.1.1"), // IPv4 address
				CIDR:     "2001:4860:4860:1234::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with invalid router IPs, got %d", len(routes))
		}
	})

	t.Run("Devices and routers in same CIDR (should not generate routes)", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::1"),
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"),
				CIDR:     "fd00:1111:2222:3333::/64", // Same CIDR as devices
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes when devices and routers are in same CIDR, got %d", len(routes))
		}
	})

	t.Run("Non-routable device CIDRs should be filtered out", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fe80::1"), // Link-local
				Services: []string{"_matter._tcp"},
			},
			{
				Name:     "Device2",
				IPv6Addr: net.ParseIP("ff02::1"), // Multicast
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"),
				CIDR:     "2001:4860:4860:1234::/64",
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 0 {
			t.Errorf("Expected 0 routes with non-routable device CIDRs, got %d", len(routes))
		}
	})

	t.Run("Router with non-routable CIDR but valid IPv6 address should still generate routes", func(t *testing.T) {
		devices := []DeviceInfo{
			{
				Name:     "Device1",
				IPv6Addr: net.ParseIP("fd00:1111:2222:3333::1"),
				Services: []string{"_matter._tcp"},
			},
		}
		routers := []ThreadBorderRouter{
			{
				Name:     "Router1",
				IPv6Addr: net.ParseIP("2001:4860:4860:1234::ff"), // Valid public IPv6
				CIDR:     "fe80::/64",                            // Link-local CIDR (this is just metadata)
			},
		}

		routes := generateRoutes(devices, routers)
		if len(routes) != 1 {
			t.Errorf("Expected 1 route with valid router IPv6 address, got %d", len(routes))
		}
	})
}

// TestCalculateCIDR64 tests the CIDR calculation function used in route generation
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

// TestCalculateCIDR64EdgeCases tests edge cases for CIDR calculation
func TestCalculateCIDR64EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		expected   string
		shouldFail bool
	}{
		{
			name:       "IPv4 address returns placeholder",
			ip:         "192.168.1.1",
			expected:   "::/64",
			shouldFail: false,
		},
		{
			name:       "Invalid IP should fail",
			ip:         "invalid-ip",
			expected:   "",
			shouldFail: true,
		},
		{
			name:       "Empty string should fail",
			ip:         "",
			expected:   "",
			shouldFail: true,
		},
		{
			name:       "IPv6 with /128 prefix",
			ip:         "2001:db8::1",
			expected:   "2001:db8::/64",
			shouldFail: false,
		},
		{
			name:       "IPv6 with different prefix length",
			ip:         "2001:db8::1",
			expected:   "2001:db8::/64",
			shouldFail: false,
		},
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

// TestIsRoutableCIDR tests the CIDR filtering function used in route generation
func TestIsRoutableCIDR(t *testing.T) {
	tests := []struct {
		name     string
		cidr     string
		expected bool
	}{
		{
			name:     "ULA CIDR should be routable",
			cidr:     "fd00:1234:5678:9abc::/64",
			expected: true,
		},
		{
			name:     "Global unicast CIDR should be routable",
			cidr:     "2001:4860:4860::/64",
			expected: true,
		},
		{
			name:     "Link-local CIDR should not be routable",
			cidr:     "fe80::/64",
			expected: false,
		},
		{
			name:     "Loopback CIDR should not be routable",
			cidr:     "::1/128",
			expected: false,
		},
		{
			name:     "Multicast CIDR should not be routable",
			cidr:     "ff00::/8",
			expected: false,
		},
		{
			name:     "Documentation CIDR should not be routable",
			cidr:     "2001:db8::/32",
			expected: false,
		},
		{
			name:     "Invalid CIDR should not be routable",
			cidr:     "invalid",
			expected: false,
		},
		{
			name:     "Empty CIDR should not be routable",
			cidr:     "",
			expected: false,
		},
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

// TestIsRoutableRouterAddress tests the router address filtering function used in route generation
func TestIsRoutableRouterAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{
			name:     "Public IPv6 address should be routable",
			ip:       "2001:4860:4860::1",
			expected: true,
		},
		{
			name:     "ULA address should not be routable",
			ip:       "fd00:1234:5678:9abc::1",
			expected: false,
		},
		{
			name:     "Link-local address should not be routable",
			ip:       "fe80::1",
			expected: false,
		},
		{
			name:     "Loopback address should not be routable",
			ip:       "::1",
			expected: false,
		},
		{
			name:     "Unspecified address should not be routable",
			ip:       "::",
			expected: false,
		},
		{
			name:     "Multicast address should not be routable",
			ip:       "ff02::1",
			expected: false,
		},
		{
			name:     "Documentation address should not be routable",
			ip:       "2001:db8::1",
			expected: false,
		},
		{
			name:     "Teredo address should not be routable",
			ip:       "2001::1",
			expected: false,
		},
		{
			name:     "6to4 address should not be routable",
			ip:       "2002::1",
			expected: false,
		},
		{
			name:     "IPv4 address should not be routable",
			ip:       "192.168.1.1",
			expected: false,
		},
		{
			name:     "Nil IP should not be routable",
			ip:       "",
			expected: false,
		},
		{
			name:     "Invalid IP should not be routable",
			ip:       "invalid",
			expected: false,
		},
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
