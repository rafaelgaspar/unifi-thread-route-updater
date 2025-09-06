package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Initialize logging level from environment
	initLogLevel()

	logInfo("Thread Route Updater Daemon starting...")
	logInfo("Monitoring for Matter devices and Thread Border Routers")
	logInfo("Press Ctrl+C to stop")

	// Create initial state
	state := &DaemonState{
		MatterDevices:       []DeviceInfo{},
		ThreadBorderRouters: []ThreadBorderRouter{},
		Routes:              []Route{},
		LastUpdate:          time.Now(),
		UbiquityConfig:      getUbiquityConfig(),
		AddedRoutes:         make(map[string]bool),
		RouteLastSeen:       make(map[string]time.Time),
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create done channel for graceful shutdown
	done := make(chan struct{})

	// Start continuous monitoring
	go monitorMatterDevices(state, done)
	go monitorThreadBorderRouters(state, done)

	// Periodic refresh every 5 minutes to catch devices that might have been missed
	go periodicRefresh(state, done)

	// Display loop
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			displayCurrentState(state)
		case sig := <-sigChan:
			fmt.Printf("\n🛑 Received signal %v, shutting down gracefully...\n", sig)
			close(done)
			return
		}
	}
}
