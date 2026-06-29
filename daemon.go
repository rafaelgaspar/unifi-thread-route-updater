package main

import (
	"fmt"
	"strings"
	"time"
)

// monitorThreadBorderRouters continuously browses for Thread Border Routers using zeroconf.
func monitorThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	logInfo("Starting Thread Border Router discovery...")
	browseThreadBorderRouters(state, done)
}

// displayCurrentState logs the current state and triggers a route sync.
func displayCurrentState(state *DaemonState) {
	state.mu.Lock()
	routes := generateRoutes(state.ThreadMeshPrefixes, state.ThreadBorderRouters)
	nRouters := len(state.ThreadBorderRouters)
	nPrefixes := len(state.ThreadMeshPrefixes)
	state.mu.Unlock()

	logInfo("Status: %d border routers, %d prefixes, %d routes", nRouters, nPrefixes, len(routes))

	state.mu.Lock()
	for p, lastSeen := range state.ThreadMeshPrefixes {
		logDebug("Thread mesh prefix: %s last-seen=%s", p, time.Since(lastSeen).Round(time.Second))
	}
	for _, r := range state.ThreadBorderRouters {
		for _, ip := range r.IPv6Addrs {
			cidr := calculateCIDR64(ip)
			logDebug("TBR %s: ip=%s cidr=%s routable=%v", r.Name, ip, cidr, isRoutableRouterAddress(ip))
		}
	}
	state.mu.Unlock()

	if len(routes) > 0 {
		for _, route := range routes {
			logDebug("Route detected: %s -> %s (%s)", route.CIDR, route.ThreadRouterIPv6, route.RouterName)
		}
	} else {
		logWarn("No routes detected: no Thread networks found")
	}

	if state.UbiquityConfig.Enabled {
		logConfiguredRoutes(state, routes)
		go updateUbiquityRoutes(state, routes)
	}
}

// logConfiguredRoutes fetches and logs the routes currently programmed on the router.
func logConfiguredRoutes(state *DaemonState, detectedRoutes []Route) {
	if !state.UbiquityConfig.hasValidSession() {
		logDebug("Skipping route status check: no valid session")
		return
	}

	configuredRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	if err != nil {
		logWarn("UniFi: failed to get configured routes: %v", err)
		return
	}

	var threadRoutes []UbiquityStaticRoute
	for _, route := range configuredRoutes {
		if strings.Contains(route.Name, "Thread route via") {
			threadRoutes = append(threadRoutes, route)
		}
	}

	logInfo("UniFi: %d Thread routes configured", len(threadRoutes))

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
			logDebug("Route configured: %s -> %s (%s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
			continue
		}

		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		if lastSeen, seen := routeLastSeen[key]; seen {
			elapsed := time.Since(lastSeen)
			if elapsed < gracePeriod {
				logInfo("Route queued for deletion: %s -> %s (%s), removing in %s",
					route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name,
					formatDuration(gracePeriod-elapsed))
			} else {
				logWarn("Route grace period expired: %s -> %s (%s)",
					route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
			}
		} else {
			logInfo("Route queued for deletion: %s -> %s (%s), removing in %s",
				route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name,
				formatDuration(gracePeriod))
		}
	}
}
