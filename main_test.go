package main

import (
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/grandcat/zeroconf"
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

// TestInitLogLevel tests the log level initialization function
func TestInitLogLevel(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedLevel LogLevel
	}{
		{
			name:          "DEBUG level",
			envValue:      "DEBUG",
			expectedLevel: DEBUG,
		},
		{
			name:          "INFO level",
			envValue:      "INFO",
			expectedLevel: INFO,
		},
		{
			name:          "WARN level",
			envValue:      "WARN",
			expectedLevel: WARN,
		},
		{
			name:          "WARNING level",
			envValue:      "WARNING",
			expectedLevel: WARN,
		},
		{
			name:          "ERROR level",
			envValue:      "ERROR",
			expectedLevel: ERROR,
		},
		{
			name:          "Invalid level defaults to INFO",
			envValue:      "INVALID",
			expectedLevel: INFO,
		},
		{
			name:          "Empty value defaults to INFO",
			envValue:      "",
			expectedLevel: INFO,
		},
		{
			name:          "Case insensitive",
			envValue:      "debug",
			expectedLevel: DEBUG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("LOG_LEVEL")
			originalLevel := currentLogLevel

			// Set test value
			if err := os.Setenv("LOG_LEVEL", tt.envValue); err != nil {
				t.Fatalf("Failed to set LOG_LEVEL: %v", err)
			}

			// Reset to default before testing
			currentLogLevel = INFO

			// Test the function
			initLogLevel()

			// Check result
			if currentLogLevel != tt.expectedLevel {
				t.Errorf("initLogLevel() with LOG_LEVEL=%s set level to %v, want %v",
					tt.envValue, currentLogLevel, tt.expectedLevel)
			}

			// Restore original values
			if err := os.Setenv("LOG_LEVEL", originalValue); err != nil {
				t.Errorf("Failed to restore LOG_LEVEL: %v", err)
			}
			currentLogLevel = originalLevel
		})
	}
}

// TestLoggingFunctions tests the logging functions with different log levels
func TestLoggingFunctions(t *testing.T) {
	// Save original level
	originalLevel := currentLogLevel
	defer func() { currentLogLevel = originalLevel }()

	// Test each log level
	levels := []struct {
		level LogLevel
		name  string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
	}

	for _, level := range levels {
		t.Run(level.name, func(t *testing.T) {
			currentLogLevel = level.level

			// Test that logDebug only logs when level is DEBUG or lower
			if level.level <= DEBUG {
				// Should log (we can't easily test the actual output, but we can test it doesn't panic)
				logDebug("Test debug message")
			}

			// Test that logInfo only logs when level is INFO or lower
			if level.level <= INFO {
				logInfo("Test info message")
			}

			// Test that logWarn only logs when level is WARN or lower
			if level.level <= WARN {
				logWarn("Test warning message")
			}

			// Test that logError always logs
			logError("Test error message")
		})
	}
}

// TestIsRoutableCIDR tests the CIDR filtering function
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

// TestConvertToUbiquityRoutes tests the conversion to Ubiquiti route format
func TestConvertToUbiquityRoutes(t *testing.T) {
	routes := []Route{
		{
			CIDR:             "fd00:1234:5678:9abc::/64",
			ThreadRouterIPv6: "fd00:1234:5678:9abc::ff",
			RouterName:       "Test Router",
		},
		{
			CIDR:             "fd00:5678:9abc:def0::/64",
			ThreadRouterIPv6: "fd00:5678:9abc:def0::fe",
			RouterName:       "Another Router",
		},
	}

	ubiquityRoutes := convertToUbiquityRoutes(routes)

	if len(ubiquityRoutes) != len(routes) {
		t.Errorf("Expected %d Ubiquiti routes, got %d", len(routes), len(ubiquityRoutes))
	}

	for i, ubiquityRoute := range ubiquityRoutes {
		originalRoute := routes[i]

		// Check required fields
		if !ubiquityRoute.Enabled {
			t.Error("Expected route to be enabled")
		}

		if ubiquityRoute.Name == "" {
			t.Error("Expected route name to be set")
		}

		if ubiquityRoute.StaticRouteNetwork != originalRoute.CIDR {
			t.Errorf("Expected StaticRouteNetwork %s, got %s",
				originalRoute.CIDR, ubiquityRoute.StaticRouteNetwork)
		}

		if ubiquityRoute.StaticRouteNexthop != originalRoute.ThreadRouterIPv6 {
			t.Errorf("Expected StaticRouteNexthop %s, got %s",
				originalRoute.ThreadRouterIPv6, ubiquityRoute.StaticRouteNexthop)
		}

		// Check that name contains router name
		if !strings.Contains(ubiquityRoute.Name, originalRoute.RouterName) {
			t.Errorf("Expected route name to contain router name '%s', got '%s'",
				originalRoute.RouterName, ubiquityRoute.Name)
		}
	}
}

// TestCompareRoutes tests the route comparison function
func TestCompareRoutes(t *testing.T) {
	current := []UbiquityStaticRoute{
		{
			ID:                 "route1",
			StaticRouteNetwork: "fd00:1234:5678:9abc::/64",
			StaticRouteNexthop: "fd00:1234:5678:9abc::ff",
			Name:               "Thread route via Router1",
		},
		{
			ID:                 "route2",
			StaticRouteNetwork: "fd00:5678:9abc:def0::/64",
			StaticRouteNexthop: "fd00:5678:9abc:def0::fe",
			Name:               "Thread route via Router2",
		},
	}

	desired := []UbiquityStaticRoute{
		{
			StaticRouteNetwork: "fd00:1234:5678:9abc::/64",
			StaticRouteNexthop: "fd00:1234:5678:9abc::ff",
			Name:               "Thread route via Router1",
		},
		{
			StaticRouteNetwork: "fd00:9999:8888:7777::/64",
			StaticRouteNexthop: "fd00:9999:8888:7777::aa",
			Name:               "Thread route via Router3",
		},
	}

	toAdd, toDelete := compareRoutes(current, desired)

	// Should add 1 new route (Router3)
	if len(toAdd) != 1 {
		t.Errorf("Expected 1 route to add, got %d", len(toAdd))
	}

	// Should delete 1 route (Router2)
	if len(toDelete) != 1 {
		t.Errorf("Expected 1 route to delete, got %d", len(toDelete))
	}

	// Check the route to add
	if toAdd[0].StaticRouteNetwork != "fd00:9999:8888:7777::/64" {
		t.Errorf("Expected route to add with network fd00:9999:8888:7777::/64, got %s",
			toAdd[0].StaticRouteNetwork)
	}

	// Check the route to delete
	if toDelete[0].ID != "route2" {
		t.Errorf("Expected route to delete with ID route2, got %s", toDelete[0].ID)
	}
}

// TestGetUbiquityConfig tests the configuration loading function
func TestGetUbiquityConfig(t *testing.T) {
	// Save original environment variables
	originalVars := map[string]string{
		"UBIQUITY_ROUTER_HOSTNAME": os.Getenv("UBIQUITY_ROUTER_HOSTNAME"),
		"UBIQUITY_USERNAME":        os.Getenv("UBIQUITY_USERNAME"),
		"UBIQUITY_PASSWORD":        os.Getenv("UBIQUITY_PASSWORD"),
		"UBIQUITY_INSECURE_SSL":    os.Getenv("UBIQUITY_INSECURE_SSL"),
		"UBIQUITY_ENABLED":         os.Getenv("UBIQUITY_ENABLED"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				if err := os.Unsetenv(key); err != nil {
					t.Errorf("Failed to unset %s: %v", key, err)
				}
			} else {
				if err := os.Setenv(key, value); err != nil {
					t.Errorf("Failed to set %s: %v", key, err)
				}
			}
		}
	}()

	t.Run("Default configuration", func(t *testing.T) {
		// Clear all environment variables
		for key := range originalVars {
			if err := os.Unsetenv(key); err != nil {
				t.Errorf("Failed to unset %s: %v", key, err)
			}
		}

		config := getUbiquityConfig()

		// Check defaults (the function has hardcoded defaults)
		if config.RouterHostname != "unifi.local" {
			t.Errorf("Expected RouterHostname 'unifi.local', got %s", config.RouterHostname)
		}
		if config.Username != "ubnt" {
			t.Errorf("Expected Username 'ubnt', got %s", config.Username)
		}
		if config.Password != "ubnt" {
			t.Errorf("Expected Password 'ubnt', got %s", config.Password)
		}
		if config.APIBaseURL != "https://unifi.local" {
			t.Errorf("Expected APIBaseURL 'https://unifi.local', got %s", config.APIBaseURL)
		}
		if config.InsecureSSL {
			t.Error("Expected InsecureSSL to be false")
		}
		if config.Enabled {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("Configuration with environment variables", func(t *testing.T) {
		// Set test environment variables
		if err := os.Setenv("UBIQUITY_ROUTER_HOSTNAME", "test-router.local"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_ROUTER_HOSTNAME: %v", err)
		}
		if err := os.Setenv("UBIQUITY_USERNAME", "testuser"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_USERNAME: %v", err)
		}
		if err := os.Setenv("UBIQUITY_PASSWORD", "testpass"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_PASSWORD: %v", err)
		}
		if err := os.Setenv("UBIQUITY_INSECURE_SSL", "true"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_INSECURE_SSL: %v", err)
		}
		if err := os.Setenv("UBIQUITY_ENABLED", "true"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_ENABLED: %v", err)
		}

		config := getUbiquityConfig()

		// Check values
		if config.RouterHostname != "test-router.local" {
			t.Errorf("Expected RouterHostname 'test-router.local', got %s", config.RouterHostname)
		}
		if config.Username != "testuser" {
			t.Errorf("Expected Username 'testuser', got %s", config.Username)
		}
		if config.Password != "testpass" {
			t.Errorf("Expected Password 'testpass', got %s", config.Password)
		}
		if !config.InsecureSSL {
			t.Error("Expected InsecureSSL to be true")
		}
		if !config.Enabled {
			t.Error("Expected Enabled to be true")
		}
	})
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

// TestIsRoutableRouterAddress tests the router address filtering function
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

// TestCompareRoutesWithGracePeriod tests the grace period logic for route comparison
func TestCompareRoutesWithGracePeriod(t *testing.T) {
	now := time.Now()
	gracePeriod := 10 * time.Minute

	tests := []struct {
		name           string
		current        []UbiquityStaticRoute
		desired        []UbiquityStaticRoute
		routeLastSeen  map[string]time.Time
		gracePeriod    time.Duration
		expectedAdd    int
		expectedRemove int
	}{
		{
			name:           "No routes to add or remove",
			current:        []UbiquityStaticRoute{},
			desired:        []UbiquityStaticRoute{},
			routeLastSeen:  map[string]time.Time{},
			gracePeriod:    gracePeriod,
			expectedAdd:    0,
			expectedRemove: 0,
		},
		{
			name:    "Add new route",
			current: []UbiquityStaticRoute{},
			desired: []UbiquityStaticRoute{
				{
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
			},
			routeLastSeen:  map[string]time.Time{},
			gracePeriod:    gracePeriod,
			expectedAdd:    1,
			expectedRemove: 0,
		},
		{
			name: "Route never seen before gets grace period",
			current: []UbiquityStaticRoute{
				{
					ID:                 "route1",
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
			},
			desired:        []UbiquityStaticRoute{},
			routeLastSeen:  map[string]time.Time{},
			gracePeriod:    gracePeriod,
			expectedAdd:    0,
			expectedRemove: 0, // Gets grace period when never seen before
		},
		{
			name: "Route within grace period should not be removed",
			current: []UbiquityStaticRoute{
				{
					ID:                 "route1",
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
			},
			desired: []UbiquityStaticRoute{},
			routeLastSeen: map[string]time.Time{
				"fd00:1111:2222:3333::/64->2001:4860:4860:1234::ff": now.Add(-5 * time.Minute), // 5 minutes ago
			},
			gracePeriod:    gracePeriod,
			expectedAdd:    0,
			expectedRemove: 0, // Should not be removed yet
		},
		{
			name: "Route beyond grace period should be removed",
			current: []UbiquityStaticRoute{
				{
					ID:                 "route1",
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
			},
			desired: []UbiquityStaticRoute{},
			routeLastSeen: map[string]time.Time{
				"fd00:1111:2222:3333::/64->2001:4860:4860:1234::ff": now.Add(-15 * time.Minute), // 15 minutes ago
			},
			gracePeriod:    gracePeriod,
			expectedAdd:    0,
			expectedRemove: 1, // Should be removed
		},
		{
			name: "Mixed scenario: add new, keep existing, remove old",
			current: []UbiquityStaticRoute{
				{
					ID:                 "route1",
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
				{
					ID:                 "route2",
					StaticRouteNetwork: "fd00:2222:3333:4444::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::fe",
					Name:               "Thread route via Router2",
				},
			},
			desired: []UbiquityStaticRoute{
				{
					StaticRouteNetwork: "fd00:1111:2222:3333::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::ff",
					Name:               "Thread route via Router1",
				},
				{
					StaticRouteNetwork: "fd00:3333:4444:5555::/64",
					StaticRouteNexthop: "2001:4860:4860:1234::fd",
					Name:               "Thread route via Router3",
				},
			},
			routeLastSeen: map[string]time.Time{
				"fd00:2222:3333:4444::/64->2001:4860:4860:1234::fe": now.Add(-15 * time.Minute), // Old, should be removed
			},
			gracePeriod:    gracePeriod,
			expectedAdd:    1, // New route
			expectedRemove: 1, // Old route beyond grace period
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toAdd, toRemove := compareRoutesWithGracePeriod(tt.current, tt.desired, tt.routeLastSeen, tt.gracePeriod)

			if len(toAdd) != tt.expectedAdd {
				t.Errorf("Expected %d routes to add, got %d", tt.expectedAdd, len(toAdd))
			}

			if len(toRemove) != tt.expectedRemove {
				t.Errorf("Expected %d routes to remove, got %d", tt.expectedRemove, len(toRemove))
			}
		})
	}
}

// TestCreateHTTPClient tests the HTTP client creation with different configurations
func TestCreateHTTPClient(t *testing.T) {
	tests := []struct {
		name           string
		config         UbiquityConfig
		expectInsecure bool
	}{
		{
			name: "Secure SSL configuration",
			config: UbiquityConfig{
				InsecureSSL: false,
			},
			expectInsecure: false,
		},
		{
			name: "Insecure SSL configuration",
			config: UbiquityConfig{
				InsecureSSL: true,
			},
			expectInsecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := createHTTPClient(tt.config)

			// Check that client is not nil
			if client == nil {
				t.Fatal("Expected HTTP client to be created, got nil")
			}

			// Check timeout is set
			if client.Timeout != 30*time.Second {
				t.Errorf("Expected timeout to be 30s, got %v", client.Timeout)
			}

			// Check transport is configured
			if client.Transport == nil {
				t.Fatal("Expected transport to be configured, got nil")
			}

			// For more detailed testing, we would need to access the transport's TLS config
			// This is a basic smoke test to ensure the function works
		})
	}
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

// TestGetUbiquityConfigEdgeCases tests edge cases for configuration parsing
func TestGetUbiquityConfigEdgeCases(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"UBIQUITY_ROUTER_HOSTNAME": os.Getenv("UBIQUITY_ROUTER_HOSTNAME"),
		"UBIQUITY_USERNAME":        os.Getenv("UBIQUITY_USERNAME"),
		"UBIQUITY_PASSWORD":        os.Getenv("UBIQUITY_PASSWORD"),
		"UBIQUITY_INSECURE_SSL":    os.Getenv("UBIQUITY_INSECURE_SSL"),
		"ROUTE_GRACE_PERIOD":       os.Getenv("ROUTE_GRACE_PERIOD"),
	}

	// Restore environment after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, value)
			}
		}
	}()

	t.Run("Invalid grace period should use default", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "invalid-duration")
		config := getUbiquityConfig()
		expected := 10 * time.Minute // Default grace period
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v for invalid duration, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Empty grace period should use default", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "")
		config := getUbiquityConfig()
		expected := 10 * time.Minute // Default grace period
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v for empty duration, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Valid grace period should be parsed", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "5m")
		config := getUbiquityConfig()
		expected := 5 * time.Minute
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Insecure SSL should be parsed correctly", func(t *testing.T) {
		_ = os.Setenv("UBIQUITY_INSECURE_SSL", "true")
		config := getUbiquityConfig()
		if !config.InsecureSSL {
			t.Errorf("Expected InsecureSSL to be true, got false")
		}
	})

	t.Run("Secure SSL should be parsed correctly", func(t *testing.T) {
		_ = os.Setenv("UBIQUITY_INSECURE_SSL", "false")
		config := getUbiquityConfig()
		if config.InsecureSSL {
			t.Errorf("Expected InsecureSSL to be false, got true")
		}
	})
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
