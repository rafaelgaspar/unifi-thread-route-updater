package main

import (
	"testing"
	"time"
)

func TestDaemonStateInit(t *testing.T) {
	state := &DaemonState{
		MatterDevices:       []DeviceInfo{},
		ThreadBorderRouters: []ThreadBorderRouter{},
		UbiquityConfig:      getUbiquityConfig(),
		AddedRoutes:         make(map[string]bool),
		RouteLastSeen:       make(map[string]time.Time),
	}

	if state.MatterDevices == nil {
		t.Error("MatterDevices should be initialised")
	}
	if state.ThreadBorderRouters == nil {
		t.Error("ThreadBorderRouters should be initialised")
	}
	if state.AddedRoutes == nil {
		t.Error("AddedRoutes should be initialised")
	}
	if state.RouteLastSeen == nil {
		t.Error("RouteLastSeen should be initialised")
	}
}
