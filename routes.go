package main

import (
	"context"
	"fmt"
	"time"

	"github.com/grandcat/zeroconf"
)

// generateRoutes generates routing entries from discovered devices and routers
func generateRoutes(devices []DeviceInfo, routers []ThreadBorderRouter) []Route {
	var routes []Route
	routeMap := make(map[string]Route)

	// Collect unique CIDR blocks from Matter devices
	deviceCIDRs := make(map[string]bool)
	for _, device := range devices {
		deviceCIDR := calculateCIDR64(device.IPv6Addr)
		if deviceCIDR != "" && deviceCIDR != "::/64" && isRoutableCIDR(deviceCIDR) {
			deviceCIDRs[deviceCIDR] = true
		}
	}

	// Collect unique CIDR blocks from Thread Border Routers
	routerCIDRs := make(map[string]bool)
	for _, router := range routers {
		if router.CIDR != "" && router.CIDR != "::/64" && isRoutableCIDR(router.CIDR) {
			routerCIDRs[router.CIDR] = true
		}
	}

	// Generate routes for device CIDRs that are not router CIDRs
	for deviceCIDR := range deviceCIDRs {
		// Skip if this CIDR is the same as a router CIDR (main network)
		if routerCIDRs[deviceCIDR] {
			continue
		}

		// Create routes to all available Thread Border Routers (only public IPv6 addresses)
		for _, router := range routers {
			// Only use routers with public IPv6 addresses (not link-local or ULA)
			if isRoutableRouterAddress(router.IPv6Addr) {
				routeKey := fmt.Sprintf("%s->%s", deviceCIDR, router.IPv6Addr.String())
				route := Route{
					CIDR:             deviceCIDR,
					ThreadRouterIPv6: router.IPv6Addr.String(),
					RouterName:       router.Name,
				}
				routeMap[routeKey] = route
			}
		}
	}

	// Convert map to slice
	for _, route := range routeMap {
		routes = append(routes, route)
	}

	return routes
}

// listenForMatterDevices passively listens for Matter device announcements
func listenForMatterDevices(state *DaemonState, done <-chan struct{}) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		logError("Failed to initialize resolver for Matter devices: %v", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)

	// Start listening for announcements
	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()

		// Use a longer timeout for passive listening
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()

		err := resolver.Browse(ctx, "_matter._tcp", "local.", entries)
		if err != nil {
			logError("Failed to browse for Matter devices: %v", err)
		}
	}()

	// Process announcements as they come in
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		// Add new device or update existing one
		for _, ip := range ipv6Addrs {
			device := DeviceInfo{
				Name:     entry.Instance,
				IPv6Addr: ip,
				Services: []string{"_matter._tcp"},
				LastSeen: time.Now(),
			}

			// Check if device already exists
			found := false
			for i, existingDevice := range state.MatterDevices {
				if existingDevice.Name == device.Name && existingDevice.IPv6Addr.Equal(device.IPv6Addr) {
					state.MatterDevices[i] = device
					found = true
					logDebug("Updated existing Matter device: %s (%s)", device.Name, device.IPv6Addr.String())
					break
				}
			}

			if !found {
				state.MatterDevices = append(state.MatterDevices, device)
				logDebug("Discovered new Matter device: %s (%s)", device.Name, device.IPv6Addr.String())
			}
		}

		state.LastUpdate = time.Now()
	}
}

// listenForThreadBorderRouters passively listens for Thread Border Router announcements
func listenForThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		logError("Failed to initialize resolver for Thread Border Routers: %v", err)
		return
	}

	entries := make(chan *zeroconf.ServiceEntry)

	// Start listening for announcements
	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()

		// Use a longer timeout for passive listening
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()

		err := resolver.Browse(ctx, "_meshcop._udp", "local.", entries)
		if err != nil {
			logError("Failed to browse for Thread Border Routers: %v", err)
		}
	}()

	// Process announcements as they come in
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		// Add new router or update existing one
		for _, ip := range ipv6Addrs {
			router := ThreadBorderRouter{
				Name:     extractRouterName(entry.Instance),
				IPv6Addr: ip,
				CIDR:     calculateCIDR64(ip),
				LastSeen: time.Now(),
			}

			// Check if router already exists
			found := false
			for i, existingRouter := range state.ThreadBorderRouters {
				if existingRouter.Name == router.Name && existingRouter.IPv6Addr.Equal(router.IPv6Addr) {
					state.ThreadBorderRouters[i] = router
					found = true
					logDebug("Updated existing Thread Border Router: %s (%s)", router.Name, router.IPv6Addr.String())
					break
				}
			}

			if !found {
				state.ThreadBorderRouters = append(state.ThreadBorderRouters, router)
				logDebug("Discovered new Thread Border Router: %s (%s)", router.Name, router.IPv6Addr.String())
			}
		}

		state.LastUpdate = time.Now()
	}
}

// periodicRefresh performs a gentle refresh every 5 minutes to catch any devices that might have been missed
func periodicRefresh(state *DaemonState, done <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Always perform gentle refresh and device expiration cleanup
			logDebug("Performing periodic refresh and device expiration cleanup")
			logDebug("Before cleanup: %d Matter devices, %d Thread Border Routers", len(state.MatterDevices), len(state.ThreadBorderRouters))

			// Remove expired devices
			expiredDevices := removeExpiredDevices(state)
			expiredRouters := removeExpiredRouters(state)
			
			if expiredDevices > 0 || expiredRouters > 0 {
				logInfo("Removed %d expired Matter devices and %d expired Thread Border Routers", expiredDevices, expiredRouters)
			}
			
			logDebug("After cleanup: %d Matter devices, %d Thread Border Routers", len(state.MatterDevices), len(state.ThreadBorderRouters))

			// Quick discovery to catch any devices that might have been missed
			devices, err := discoverMatterDevices()
			if err == nil {
				logDebug("Periodic refresh discovered %d Matter devices", len(devices))
				if len(devices) > 0 {
					mergeDevices(state, devices)
				}
			} else {
				logWarn("Periodic refresh failed for Matter devices: %v", err)
			}

			routers, err := discoverThreadBorderRouters()
			if err == nil {
				logDebug("Periodic refresh discovered %d Thread Border Routers", len(routers))
				if len(routers) > 0 {
					mergeRouters(state, routers)
				}
			} else {
				logWarn("Periodic refresh failed for Thread Border Routers: %v", err)
			}
			
			logDebug("After discovery: %d Matter devices, %d Thread Border Routers", len(state.MatterDevices), len(state.ThreadBorderRouters))

			state.LastUpdate = time.Now()
		case <-done:
			return
		}
	}
}

// removeExpiredDevices removes devices that haven't been seen for the expiration period
func removeExpiredDevices(state *DaemonState) int {

	now := time.Now()
	var remainingDevices []DeviceInfo
	removedCount := 0

	for _, device := range state.MatterDevices {
		if now.Sub(device.LastSeen) > state.DeviceExpiration {
			logDebug("Removing expired Matter device: %s (%s) - last seen %v ago",
				device.Name, device.IPv6Addr.String(), now.Sub(device.LastSeen))
			removedCount++
		} else {
			remainingDevices = append(remainingDevices, device)
		}
	}

	state.MatterDevices = remainingDevices
	return removedCount
}

// removeExpiredRouters removes routers that haven't been seen for the expiration period
func removeExpiredRouters(state *DaemonState) int {

	now := time.Now()
	var remainingRouters []ThreadBorderRouter
	removedCount := 0

	for _, router := range state.ThreadBorderRouters {
		if now.Sub(router.LastSeen) > state.DeviceExpiration {
			logDebug("Removing expired Thread Border Router: %s (%s) - last seen %v ago",
				router.Name, router.IPv6Addr.String(), now.Sub(router.LastSeen))
			removedCount++
		} else {
			remainingRouters = append(remainingRouters, router)
		}
	}

	state.ThreadBorderRouters = remainingRouters
	return removedCount
}

// mergeDevices merges newly discovered devices with existing ones
func mergeDevices(state *DaemonState, newDevices []DeviceInfo) {
	logDebug("Merging %d new devices with %d existing devices", len(newDevices), len(state.MatterDevices))
	
	for _, newDevice := range newDevices {
		found := false
		for i, existingDevice := range state.MatterDevices {
			if existingDevice.Name == newDevice.Name && existingDevice.IPv6Addr.Equal(newDevice.IPv6Addr) {
				// Update existing device with new LastSeen time
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
	
	logDebug("After merge: %d total Matter devices", len(state.MatterDevices))
}

// mergeRouters merges newly discovered routers with existing ones
func mergeRouters(state *DaemonState, newRouters []ThreadBorderRouter) {
	for _, newRouter := range newRouters {
		found := false
		for i, existingRouter := range state.ThreadBorderRouters {
			if existingRouter.Name == newRouter.Name && existingRouter.IPv6Addr.Equal(newRouter.IPv6Addr) {
				// Update existing router with new LastSeen time
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
