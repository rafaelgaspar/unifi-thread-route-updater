package main

import (
	"fmt"
	"strings"
	"time"
)

// monitorMatterDevices continuously browses for Matter devices using zeroconf.
func monitorMatterDevices(state *DaemonState, done <-chan struct{}) {
	logInfo("Starting continuous mDNS browse for Matter devices (_matter._tcp)")
	browseMatterDevices(state, done)
}

// monitorThreadBorderRouters continuously browses for Thread Border Routers using zeroconf.
func monitorThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	logInfo("Starting continuous mDNS browse for Thread Border Routers (_meshcop._udp)")
	browseThreadBorderRouters(state, done)
}

// displayCurrentState logs the current state and triggers a route sync.
func displayCurrentState(state *DaemonState) {
	state.mu.Lock()
	routes := generateRoutes(state.ThreadMeshPrefixes, state.ThreadBorderRouters)
	nDevices := len(state.MatterDevices)
	nRouters := len(state.ThreadBorderRouters)
	nPrefixes := len(state.ThreadMeshPrefixes)
	state.mu.Unlock()

	logInfo("Status update: %d Matter devices, %d Thread Border Routers, %d Thread mesh prefixes, %d routes detected",
		nDevices, nRouters, nPrefixes, len(routes))

	state.mu.Lock()
	for p, lastSeen := range state.ThreadMeshPrefixes {
		logDebug("Thread mesh prefix: %s  last seen %v ago", p, time.Since(lastSeen).Round(time.Second))
	}
	for _, r := range state.ThreadBorderRouters {
		for _, ip := range r.IPv6Addrs {
			cidr := calculateCIDR64(ip)
			logDebug("Thread Border Router: %s  ip=%s  cidr=%s  routerRoutable=%v",
				r.Name, ip, cidr, isRoutableRouterAddress(ip))
		}
	}
	state.mu.Unlock()

	if len(routes) > 0 {
		for _, route := range routes {
			logDebug("Detected route: %s -> %s (%s)", route.CIDR, route.ThreadRouterIPv6, route.RouterName)
		}
	} else {
		logWarn("No routes detected (no Thread networks found)")
	}

	if state.UbiquityConfig.Enabled {
		logConfiguredRoutes(state, routes)
		go updateUbiquityRoutes(state, routes)
	}
}

// logConfiguredRoutes fetches and logs the routes currently programmed on the router.
func logConfiguredRoutes(state *DaemonState, detectedRoutes []Route) {
	if !state.UbiquityConfig.hasValidSession() {
		logDebug("No valid session for route status check, skipping")
		return
	}

	configuredRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	if err != nil {
		logWarn("Failed to get configured routes from Ubiquity router: %v", err)
		return
	}

	var threadRoutes []UbiquityStaticRoute
	for _, route := range configuredRoutes {
		if strings.Contains(route.Name, "Thread route via") {
			threadRoutes = append(threadRoutes, route)
		}
	}

	logInfo("Configured routes: %d Thread routes in Ubiquity router", len(threadRoutes))

	state.mu.Lock()
	routeLastSeen := state.RouteLastSeen
	gracePeriod := state.UbiquityConfig.RouteGracePeriod
	state.mu.Unlock()

	for _, route := range threadRoutes {
		stillDetected := false
		for _, detected := range detectedRoutes {
			if detected.CIDR == route.StaticRouteNetwork && detected.ThreadRouterIPv6 == route.StaticRouteNexthop {
				stillDetected = true
				break
			}
		}

		if stillDetected {
			logDebug("Configured route: %s -> %s (%s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
			continue
		}

		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		if lastSeen, seen := routeLastSeen[key]; seen {
			elapsed := time.Since(lastSeen)
			if elapsed < gracePeriod {
				logInfo("Route marked for deletion: %s -> %s (%s) - will be removed in %s",
					route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name,
					formatDuration(gracePeriod-elapsed))
			} else {
				logWarn("Route overdue for deletion: %s -> %s (%s) - grace period expired",
					route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
			}
		} else {
			logInfo("Route marked for deletion: %s -> %s (%s) - will be removed in %s (grace period)",
				route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name,
				formatDuration(gracePeriod))
		}
	}
}
