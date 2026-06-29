package main

import (
	"fmt"
	"time"
)

// generateRoutes generates routing entries from discovered devices and routers.
func generateRoutes(devices []DeviceInfo, routers []ThreadBorderRouter) []Route {
	routeMap := make(map[string]Route)

	deviceCIDRs := make(map[string]bool)
	for _, device := range devices {
		if cidr := calculateCIDR64(device.IPv6Addr); cidr != "" && isRoutableCIDR(cidr) {
			deviceCIDRs[cidr] = true
		}
	}

	routerCIDRs := make(map[string]bool)
	for _, router := range routers {
		if router.CIDR != "" && isRoutableCIDR(router.CIDR) {
			routerCIDRs[router.CIDR] = true
		}
	}

	for deviceCIDR := range deviceCIDRs {
		if routerCIDRs[deviceCIDR] {
			continue
		}
		for _, router := range routers {
			if isRoutableRouterAddress(router.IPv6Addr) {
				key := fmt.Sprintf("%s->%s", deviceCIDR, router.IPv6Addr.String())
				routeMap[key] = Route{
					CIDR:             deviceCIDR,
					ThreadRouterIPv6: router.IPv6Addr.String(),
					RouterName:       router.Name,
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

// listenForMatterDevices polls for Matter devices every 30 seconds.
func listenForMatterDevices(state *DaemonState, done <-chan struct{}) {
	runPoller(done, 30*time.Second, "Matter device", func() error {
		devices, err := discoverMatterDevices()
		if err != nil {
			return err
		}
		mergeDevices(state, devices)
		return nil
	})
}

// listenForThreadBorderRouters polls for Thread Border Routers every 30 seconds.
func listenForThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	runPoller(done, 30*time.Second, "Thread Border Router", func() error {
		routers, err := discoverThreadBorderRouters()
		if err != nil {
			return err
		}
		mergeRouters(state, routers)
		return nil
	})
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

// removeExpiredRouters removes routers that haven't been seen for the expiration period.
func removeExpiredRouters(state *DaemonState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now()
	var remaining []ThreadBorderRouter
	removed := 0
	for _, router := range state.ThreadBorderRouters {
		if now.Sub(router.LastSeen) > state.UbiquityConfig.DeviceExpiration {
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

// mergeDevices merges newly discovered devices with existing ones.
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

// mergeRouters merges newly discovered routers with existing ones.
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
