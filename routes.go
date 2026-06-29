package main

import (
	"fmt"
	"time"
)

// generateRoutes generates routing entries from discovered devices and routers
func generateRoutes(devices []DeviceInfo, routers []ThreadBorderRouter) []Route {
	var routes []Route
	routeMap := make(map[string]Route)

	deviceCIDRs := make(map[string]bool)
	for _, device := range devices {
		deviceCIDR := calculateCIDR64(device.IPv6Addr)
		if deviceCIDR != "" && deviceCIDR != "::/64" && isRoutableCIDR(deviceCIDR) {
			deviceCIDRs[deviceCIDR] = true
		}
	}

	routerCIDRs := make(map[string]bool)
	for _, router := range routers {
		if router.CIDR != "" && router.CIDR != "::/64" && isRoutableCIDR(router.CIDR) {
			routerCIDRs[router.CIDR] = true
		}
	}

	for deviceCIDR := range deviceCIDRs {
		if routerCIDRs[deviceCIDR] {
			continue
		}
		for _, router := range routers {
			if isRoutableRouterAddress(router.IPv6Addr) {
				routeKey := fmt.Sprintf("%s->%s", deviceCIDR, router.IPv6Addr.String())
				routeMap[routeKey] = Route{
					CIDR:             deviceCIDR,
					ThreadRouterIPv6: router.IPv6Addr.String(),
					RouterName:       router.Name,
				}
			}
		}
	}

	for _, route := range routeMap {
		routes = append(routes, route)
	}
	return routes
}

// listenForMatterDevices polls for Matter device announcements every 30 seconds.
func listenForMatterDevices(state *DaemonState, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			devices, err := discoverMatterDevices()
			if err != nil {
				logWarn("Matter device poll failed: %v", err)
				continue
			}
			if len(devices) > 0 {
				mergeDevices(state, devices)
				state.LastUpdate = time.Now()
			}
		case <-done:
			return
		}
	}
}

// listenForThreadBorderRouters polls for Thread Border Router announcements every 30 seconds.
func listenForThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			routers, err := discoverThreadBorderRouters()
			if err != nil {
				logWarn("Thread Border Router poll failed: %v", err)
				continue
			}
			if len(routers) > 0 {
				mergeRouters(state, routers)
				state.LastUpdate = time.Now()
			}
		case <-done:
			return
		}
	}
}

// periodicRefresh cleans up expired devices and routers every 5 minutes.
func periodicRefresh(state *DaemonState, done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logDebug("Performing periodic expiration cleanup")
			expiredDevices := removeExpiredDevices(state)
			expiredRouters := removeExpiredRouters(state)
			if expiredDevices > 0 || expiredRouters > 0 {
				logInfo("Removed %d expired Matter devices and %d expired Thread Border Routers", expiredDevices, expiredRouters)
			}
			state.LastUpdate = time.Now()
		case <-done:
			return
		}
	}
}

// removeExpiredDevices removes devices that haven't been seen for the expiration period
func removeExpiredDevices(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	var remaining []DeviceInfo
	removed := 0
	for _, device := range state.MatterDevices {
		if now.Sub(device.LastSeen) > state.DeviceExpiration {
			logDebug("Removing expired Matter device: %s (%s) - last seen %v ago",
				device.Name, device.IPv6Addr.String(), now.Sub(device.LastSeen))
			removed++
		} else {
			remaining = append(remaining, device)
		}
	}
	state.MatterDevices = remaining
	return removed
}

// removeExpiredRouters removes routers that haven't been seen for the expiration period
func removeExpiredRouters(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	var remaining []ThreadBorderRouter
	removed := 0
	for _, router := range state.ThreadBorderRouters {
		if now.Sub(router.LastSeen) > state.DeviceExpiration {
			logDebug("Removing expired Thread Border Router: %s (%s) - last seen %v ago",
				router.Name, router.IPv6Addr.String(), now.Sub(router.LastSeen))
			removed++
		} else {
			remaining = append(remaining, router)
		}
	}
	state.ThreadBorderRouters = remaining
	return removed
}

// mergeDevices merges newly discovered devices with existing ones
func mergeDevices(state *DaemonState, newDevices []DeviceInfo) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, newDevice := range newDevices {
		found := false
		for i, existing := range state.MatterDevices {
			if existing.Name == newDevice.Name && existing.IPv6Addr.Equal(newDevice.IPv6Addr) {
				state.MatterDevices[i] = newDevice
				found = true
				logDebug("Updated existing Matter device: %s (%s)", newDevice.Name, newDevice.IPv6Addr.String())
				break
			}
		}
		if !found {
			state.MatterDevices = append(state.MatterDevices, newDevice)
			logDebug("Added new Matter device: %s (%s)", newDevice.Name, newDevice.IPv6Addr.String())
		}
	}
}

// mergeRouters merges newly discovered routers with existing ones
func mergeRouters(state *DaemonState, newRouters []ThreadBorderRouter) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, newRouter := range newRouters {
		found := false
		for i, existing := range state.ThreadBorderRouters {
			if existing.Name == newRouter.Name && existing.IPv6Addr.Equal(newRouter.IPv6Addr) {
				state.ThreadBorderRouters[i] = newRouter
				found = true
				logDebug("Updated existing Thread Border Router: %s (%s)", newRouter.Name, newRouter.IPv6Addr.String())
				break
			}
		}
		if !found {
			state.ThreadBorderRouters = append(state.ThreadBorderRouters, newRouter)
			logDebug("Added new Thread Border Router: %s (%s)", newRouter.Name, newRouter.IPv6Addr.String())
		}
	}
}
