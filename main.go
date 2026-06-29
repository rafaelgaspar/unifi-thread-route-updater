package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	initLogLevel()

	logInfo("Thread Route Updater starting...")

	config := getUbiquityConfig()
	haCfg := getHomeAssistantConfig()

	state := &DaemonState{
		ThreadBorderRouters: []ThreadBorderRouter{},
		ThreadMeshPrefixes:  make(map[string]time.Time),
		UbiquityConfig:      config,
		HomeAssistantConfig: haCfg,
		AddedRoutes:         make(map[string]bool),
		RouteLastSeen:       make(map[string]time.Time),
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan struct{})

	go monitorThreadBorderRouters(state, done)
	go browseMatterDevices(state, done)
	go pollHomeAssistant(state, done)
	go periodicRefresh(state, done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			displayCurrentState(state)
		case sig := <-sigChan:
			logInfo("Received signal %v, shutting down", sig)
			close(done)
			return
		}
	}
}
