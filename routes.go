package main

import (
	"fmt"
	"time"
)

// generateRoutes generates routing entries from RA-discovered Thread mesh prefixes
// and border routers. For each Thread mesh prefix × each routable border router IP,
// one route is created. Border router IPs are stable (MAC-based EUI-64); prefixes
// are dynamic and sourced from ICMPv6 Router Advertisements.
func generateRoutes(meshPrefixes map[string]time.Time, routers []ThreadBorderRouter) []Route {
	routeMap := make(map[string]Route)

	for prefix := range meshPrefixes {
		for _, router := range routers {
			for _, ip := range router.IPv6Addrs {
				if isRoutableRouterAddress(ip) {
					key := fmt.Sprintf("%s->%s", prefix, ip.String())
					routeMap[key] = Route{
						CIDR:             prefix,
						ThreadRouterIPv6: ip.String(),
						RouterName:       router.Name,
					}
				}
			}
		}
	}

	routes := make([]Route, 0, len(routeMap))
	for _, route := range routeMap {
		routes = append(routes, route)
	}
	return routes
}

// runPoller calls fn on every tick until done is closed.
func runPoller(done <-chan struct{}, interval time.Duration, label string, fn func() error) {
	if err := fn(); err != nil {
		logWarn("%s poll failed: %v", label, err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := fn(); err != nil {
				logWarn("%s poll failed: %v", label, err)
			}
		case <-done:
			return
		}
	}
}

// periodicRefresh cleans up expired routers and Thread mesh prefixes every 5 minutes.
func periodicRefresh(state *DaemonState, done <-chan struct{}) {
	runPoller(done, 5*time.Minute, "expiration cleanup", func() error {
		logDebug("Performing periodic expiration cleanup")
		expiredRouters := removeExpiredRouters(state)
		expiredPrefixes := removeExpiredPrefixes(state)
		if expiredRouters > 0 || expiredPrefixes > 0 {
			logInfo("Removed %d expired Thread Border Routers, %d expired Thread mesh prefixes",
				expiredRouters, expiredPrefixes)
		}
		return nil
	})
}

// removeExpiredRouters removes routers that haven't been seen for the expiration period.
func removeExpiredRouters(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	var remaining []ThreadBorderRouter
	removed := 0
	for _, router := range state.ThreadBorderRouters {
		if now.Sub(router.LastSeen) > state.UbiquityConfig.DeviceExpiration {
			logDebug("Removing expired Thread Border Router: %s %v - last seen %v ago",
				router.Name, router.IPv6Addrs, now.Sub(router.LastSeen))
			removed++
		} else {
			remaining = append(remaining, router)
		}
	}
	state.ThreadBorderRouters = remaining
	return removed
}

// removeExpiredPrefixes removes Thread mesh prefixes not seen in an RA for the grace period.
func removeExpiredPrefixes(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	removed := 0
	for prefix, lastSeen := range state.ThreadMeshPrefixes {
		if now.Sub(lastSeen) > state.UbiquityConfig.RouteGracePeriod {
			logDebug("Removing expired Thread mesh prefix: %s - last seen %v ago", prefix, now.Sub(lastSeen))
			delete(state.ThreadMeshPrefixes, prefix)
			removed++
		}
	}
	return removed
}

// mergeRouters merges newly discovered routers with existing ones, accumulating IPs per router.
func mergeRouters(state *DaemonState, newRouters []ThreadBorderRouter) {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	for _, newRouter := range newRouters {
		found := false
		for i, existing := range state.ThreadBorderRouters {
			if existing.Name == newRouter.Name {
				state.ThreadBorderRouters[i].LastSeen = now
				for _, ip := range newRouter.IPv6Addrs {
					state.ThreadBorderRouters[i].IPv6Addrs = appendUnique(state.ThreadBorderRouters[i].IPv6Addrs, ip)
				}
				logDebug("Updated existing Thread Border Router: %s %v", newRouter.Name, state.ThreadBorderRouters[i].IPv6Addrs)
				found = true
				break
			}
		}
		if !found {
			newRouter.LastSeen = now
			state.ThreadBorderRouters = append(state.ThreadBorderRouters, newRouter)
			logDebug("Added new Thread Border Router: %s %v", newRouter.Name, newRouter.IPv6Addrs)
		}
	}
}
