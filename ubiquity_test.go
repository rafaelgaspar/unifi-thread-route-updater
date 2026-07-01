package main

import (
	"strings"
	"testing"
	"time"
)

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

	ubiquityRoutes := convertToUbiquityRoutes(routes, "aa:bb:cc:dd:ee:ff")

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

// TestDistanceAllocator verifies compact distance assignment in 1..N per prefix.
func TestDistanceAllocator(t *testing.T) {
	prefix := "fd36:1fa8:d5a::/64"

	t.Run("empty current, batch of three", func(t *testing.T) {
		toAdd := []UbiquityStaticRoute{
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::1"},
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::2"},
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::3"},
		}
		newDistanceAllocator(nil).assign(toAdd)
		if toAdd[0].StaticRouteDistance != 1 || toAdd[1].StaticRouteDistance != 2 || toAdd[2].StaticRouteDistance != 3 {
			t.Errorf("expected distances 1,2,3 got %d,%d,%d",
				toAdd[0].StaticRouteDistance, toAdd[1].StaticRouteDistance, toAdd[2].StaticRouteDistance)
		}
	})

	t.Run("current routes with missing distance field", func(t *testing.T) {
		current := []UbiquityStaticRoute{
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::1", StaticRouteDistance: 0},
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::2", StaticRouteDistance: 0},
		}
		toAdd := []UbiquityStaticRoute{
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::3"},
		}
		newDistanceAllocator(current).assign(toAdd)
		if toAdd[0].StaticRouteDistance != 3 {
			t.Errorf("expected distance 3 when two existing routes omit distance, got %d", toAdd[0].StaticRouteDistance)
		}
	})

	t.Run("fills gap instead of max+1", func(t *testing.T) {
		current := []UbiquityStaticRoute{
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::1", StaticRouteDistance: 1},
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::2", StaticRouteDistance: 4},
		}
		toAdd := []UbiquityStaticRoute{
			{StaticRouteNetwork: prefix, StaticRouteNexthop: "2001::3"},
		}
		newDistanceAllocator(current).assign(toAdd)
		if toAdd[0].StaticRouteDistance != 2 {
			t.Errorf("expected distance 2 (gap fill), got %d", toAdd[0].StaticRouteDistance)
		}
	})
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
