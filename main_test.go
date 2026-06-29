package main

import (
	"testing"
	"time"
)

func TestDaemonStateInit(t *testing.T) {
	state := &DaemonState{
		ThreadBorderRouters: []ThreadBorderRouter{},
		ThreadMeshPrefixes:  make(map[string]time.Time),
		UbiquityConfig:      getUbiquityConfig(),
		AddedRoutes:         make(map[string]bool),
		RouteLastSeen:       make(map[string]time.Time),
	}

	if state.ThreadBorderRouters == nil {
		t.Error("ThreadBorderRouters should be initialised")
	}
	if state.ThreadMeshPrefixes == nil {
		t.Error("ThreadMeshPrefixes should be initialised")
	}
	if state.AddedRoutes == nil {
		t.Error("AddedRoutes should be initialised")
	}
	if state.RouteLastSeen == nil {
		t.Error("RouteLastSeen should be initialised")
	}
}
