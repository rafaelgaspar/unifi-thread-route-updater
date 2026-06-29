package main

import (
	"fmt"
	"time"
)

// generateRoutes generates routing entries from discovered devices and routers.
// Each device may have multiple IPv6 addresses (Thread mesh fd:: and LAN 2a02::).
// Each router may also have multiple IPs. We generate a route for every
// (device CIDR, routable router IP) pair where the CIDRs differ.
func generateRoutes(devices []DeviceInfo, routers []ThreadBorderRouter) []Route {
	routeMap := make(map[string]Route)

	deviceCIDRs := make(map[string]bool)
	for _, device := range devices {
		for _, ip := range device.IPv6Addrs {
			if cidr := calculateCIDR64(ip); cidr != "" && isRoutableCIDR(cidr) {
				deviceCIDRs[cidr] = true
			}
		}
	}

	routerCIDRs := make(map[string]bool)
	for _, router := range routers {
		for _, ip := range router.IPv6Addrs {
			if cidr := calculateCIDR64(ip); cidr != "" && isRoutableCIDR(cidr) {
				routerCIDRs[cidr] = true
			}
		}
	}

	for deviceCIDR := range deviceCIDRs {
		if routerCIDRs[deviceCIDR] {
			continue
		}
		for _, router := range routers {
			for _, ip := range router.IPv6Addrs {
				if isRoutableRouterAddress(ip) {
					key := fmt.Sprintf("%s->%s", deviceCIDR, ip.String())
					routeMap[key] = Route{
						CIDR:             deviceCIDR,
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

// periodicRefresh cleans up expired devices and routers every 5 minutes.
func periodicRefresh(state *DaemonState, done <-chan struct{}) {
	runPoller(done, 5*time.Minute, "expiration cleanup", func() error {
		logDebug("Performing periodic expiration cleanup")
		expiredDevices := removeExpiredDevices(state)
		expiredRouters := removeExpiredRouters(state)
		if expiredDevices > 0 || expiredRouters > 0 {
			logInfo("Removed %d expired Matter devices and %d expired Thread Border Routers", expiredDevices, expiredRouters)
		}
		return nil
	})
}

// removeExpiredDevices removes devices that haven't been seen for the expiration period.
func removeExpiredDevices(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	var remaining []DeviceInfo
	removed := 0
	for _, device := range state.MatterDevices {
		if now.Sub(device.LastSeen) > state.UbiquityConfig.DeviceExpiration {
			logDebug("Removing expired Matter device: %s %v - last seen %v ago",
				device.Name, device.IPv6Addrs, now.Sub(device.LastSeen))
			removed++
		} else {
			remaining = append(remaining, device)
		}
	}
	state.MatterDevices = remaining
	return removed
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

// mergeDevices merges newly discovered devices with existing ones, accumulating IPs per device.
func mergeDevices(state *DaemonState, newDevices []DeviceInfo) {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	for _, newDevice := range newDevices {
		found := false
		for i, existing := range state.MatterDevices {
			if existing.Name == newDevice.Name {
				state.MatterDevices[i].LastSeen = now
				for _, ip := range newDevice.IPv6Addrs {
					state.MatterDevices[i].IPv6Addrs = appendUnique(state.MatterDevices[i].IPv6Addrs, ip)
				}
				logDebug("Updated existing Matter device: %s %v", newDevice.Name, state.MatterDevices[i].IPv6Addrs)
				found = true
				break
			}
		}
		if !found {
			newDevice.LastSeen = now
			state.MatterDevices = append(state.MatterDevices, newDevice)
			logDebug("Added new Matter device: %s %v", newDevice.Name, newDevice.IPv6Addrs)
		}
	}
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
