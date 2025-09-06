package main

import (
	"fmt"
	"time"
)

// monitorMatterDevices continuously monitors for Matter devices
func monitorMatterDevices(state *DaemonState, done <-chan struct{}) {
	// Initial discovery
	devices, err := discoverMatterDevices()
	if err != nil {
		fmt.Printf("❌ Error discovering Matter devices: %v\n", err)
	} else {
		state.MatterDevices = devices
		state.LastUpdate = time.Now()
	}

	// Then just listen for announcements (passive monitoring)
	listenForMatterDevices(state, done)
}

// monitorThreadBorderRouters continuously monitors for Thread Border Routers
func monitorThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	// Initial discovery
	routers, err := discoverThreadBorderRouters()
	if err != nil {
		fmt.Printf("❌ Error discovering Thread Border Routers: %v\n", err)
	} else {
		state.ThreadBorderRouters = routers
		state.LastUpdate = time.Now()
	}

	// Then just listen for announcements (passive monitoring)
	listenForThreadBorderRouters(state, done)
}

// displayCurrentState displays the current state of discovered devices and routes
func displayCurrentState(state *DaemonState) {
	// Clear screen and show header
	fmt.Print("\033[2J\033[H")
	fmt.Println("🔍 Thread Route Updater Daemon - Live Status")
	fmt.Println("==================================================")
	fmt.Printf("📅 Last Update: %s\n", state.LastUpdate.Format("15:04:05"))
	fmt.Println()

	// Show discovered devices
	fmt.Printf("📱 Matter Devices: %d\n", len(state.MatterDevices))
	for i, device := range state.MatterDevices {
		if i < 5 { // Show first 5 devices
			fmt.Printf("  • %s -> %s\n", device.Name, device.IPv6Addr.String())
		} else if i == 5 {
			fmt.Printf("  ... and %d more\n", len(state.MatterDevices)-5)
		}
	}
	fmt.Println()

	// Show discovered Thread Border Routers
	fmt.Printf("🌐 Thread Border Routers: %d\n", len(state.ThreadBorderRouters))
	for _, router := range state.ThreadBorderRouters {
		fmt.Printf("  • %s -> %s (%s)\n", router.Name, router.IPv6Addr.String(), router.CIDR)
	}
	fmt.Println()

	// Generate and show current routes
	routes := generateRoutes(state.MatterDevices, state.ThreadBorderRouters)
	state.Routes = routes

	if len(routes) > 0 {
		fmt.Println("🛣️  Current Routes:")
		for _, route := range routes {
			fmt.Printf("  %s -> %s (%s)\n", route.CIDR, route.ThreadRouterIPv6, route.RouterName)
		}

		// Update Ubiquity router if enabled
		if state.UbiquityConfig.Enabled {
			go updateUbiquityRoutes(state, routes)
		}
	} else {
		fmt.Println("⚠️  No routes available (no Thread networks detected)")
	}

	fmt.Println()
	fmt.Println("🔄 Monitoring... (Next update in 5s)")
}
