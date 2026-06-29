package main

import (
	"fmt"
	"strings"
	"time"
)

// monitorMatterDevices performs an initial discovery then polls every 30 seconds.
func monitorMatterDevices(state *DaemonState, done <-chan struct{}) {
	devices, err := discoverMatterDevices()
	if err != nil {
		logError("Error discovering Matter devices: %v", err)
	} else {
		mergeDevices(state, devices)
		logInfo("Initial Matter device discovery completed: %d devices found", len(devices))
	}
	listenForMatterDevices(state, done)
}

// monitorThreadBorderRouters performs an initial discovery then polls every 30 seconds.
func monitorThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	routers, err := discoverThreadBorderRouters()
	if err != nil {
		logError("Error discovering Thread Border Routers: %v", err)
	} else {
		mergeRouters(state, routers)
		logInfo("Initial Thread Border Router discovery completed: %d routers found", len(routers))
	}
	listenForThreadBorderRouters(state, done)
}

// displayCurrentState logs the current state and triggers a route sync.
func displayCurrentState(state *DaemonState) {
	state.mu.Lock()
	routes := generateRoutes(state.MatterDevices, state.ThreadBorderRouters)
	nDevices := len(state.MatterDevices)
	nRouters := len(state.ThreadBorderRouters)
	state.mu.Unlock()

	logInfo("Status update: %d Matter devices, %d Thread Border Routers, %d routes detected",
		nDevices, nRouters, len(routes))

	state.mu.Lock()
	for _, d := range state.MatterDevices {
		cidr := calculateCIDR64(d.IPv6Addr)
		logDebug("Matter device: %s  ip=%s  cidr=%s  routable=%v", d.Name, d.IPv6Addr, cidr, isRoutableCIDR(cidr))
	}
	for _, r := range state.ThreadBorderRouters {
		logDebug("Thread Border Router: %s  ip=%s  cidr=%s  routerRoutable=%v  cidrRoutable=%v",
			r.Name, r.IPv6Addr, r.CIDR, isRoutableRouterAddress(r.IPv6Addr), isRoutableCIDR(r.CIDR))
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
