package main

import (
	"time"
)

// monitorMatterDevices continuously monitors for Matter devices
func monitorMatterDevices(state *DaemonState, done <-chan struct{}) {
	// Initial discovery
	devices, err := discoverMatterDevices()
	if err != nil {
		logError("Error discovering Matter devices: %v", err)
	} else {
		state.MatterDevices = devices
		state.LastUpdate = time.Now()
		logInfo("Initial Matter device discovery completed: %d devices found", len(devices))
		logDebug("Matter devices discovered: %+v", devices)
	}

	// Then just listen for announcements (passive monitoring)
	listenForMatterDevices(state, done)
}

// monitorThreadBorderRouters continuously monitors for Thread Border Routers
func monitorThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	// Initial discovery
	routers, err := discoverThreadBorderRouters()
	if err != nil {
		logError("Error discovering Thread Border Routers: %v", err)
	} else {
		state.ThreadBorderRouters = routers
		state.LastUpdate = time.Now()
		logInfo("Initial Thread Border Router discovery completed: %d routers found", len(routers))
		logDebug("Thread Border Routers discovered: %+v", routers)
	}

	// Then just listen for announcements (passive monitoring)
	listenForThreadBorderRouters(state, done)
}

// displayCurrentState logs the current state of discovered devices and routes
func displayCurrentState(state *DaemonState) {
	// Generate current routes
	routes := generateRoutes(state.MatterDevices, state.ThreadBorderRouters)
	state.Routes = routes

	// Log current status
	logInfo("Status update: %d Matter devices, %d Thread Border Routers, %d routes", 
		len(state.MatterDevices), len(state.ThreadBorderRouters), len(routes))

	// Debug logging for detailed device information
	logDebug("Matter devices: %+v", state.MatterDevices)
	logDebug("Thread Border Routers: %+v", state.ThreadBorderRouters)
	logDebug("Generated routes: %+v", routes)

	// Log route changes if any
	if len(routes) > 0 {
		logInfo("Active routes: %d routes configured", len(routes))
		for _, route := range routes {
			logDebug("Route: %s -> %s (%s)", route.CIDR, route.ThreadRouterIPv6, route.RouterName)
		}

		// Update Ubiquity router if enabled
		if state.UbiquityConfig.Enabled {
			go updateUbiquityRoutes(state, routes)
		}
	} else {
		logWarn("No routes available (no Thread networks detected)")
	}
}
