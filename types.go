package main

import (
	"net"
	"time"
)

// LogLevel represents the logging severity level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// DeviceInfo represents a discovered Matter device
type DeviceInfo struct {
	Name     string
	IPv6Addr net.IP
	Services []string
}

// ThreadBorderRouter represents a discovered Thread Border Router
type ThreadBorderRouter struct {
	Name     string
	IPv6Addr net.IP
	CIDR     string
}

// Route represents a routing entry
type Route struct {
	CIDR             string
	ThreadRouterIPv6 string
	RouterName       string
}

// DaemonState holds the current state of discovered devices and routers
type DaemonState struct {
	MatterDevices       []DeviceInfo
	ThreadBorderRouters []ThreadBorderRouter
	Routes              []Route
	LastUpdate          time.Time
	UbiquityConfig      UbiquityConfig
	AddedRoutes         map[string]bool      // Track routes we've added to prevent duplicates
	RouteLastSeen       map[string]time.Time // Track when each route was last seen
}

// UbiquityConfig holds configuration for Ubiquity router API
type UbiquityConfig struct {
	RouterHostname   string
	Username         string
	Password         string
	APIBaseURL       string
	InsecureSSL      bool
	Enabled          bool
	SessionToken     string        // Device token for API requests
	CSRFToken        string        // CSRF token for API requests
	SessionCookie    string        // Session cookie for API requests
	LastLoginTime    int64         // Timestamp of last successful login
	RouteGracePeriod time.Duration // Grace period before removing routes
}

// UbiquityStaticRoute represents a static route in Ubiquity format
type UbiquityStaticRoute struct {
	ID                 string `json:"_id,omitempty"`
	Enabled            bool   `json:"enabled"`
	Name               string `json:"name"`
	Type               string `json:"type"`
	StaticRouteNexthop string `json:"static-route_nexthop"`
	StaticRouteNetwork string `json:"static-route_network"`
	StaticRouteType    string `json:"static-route_type"`
	GatewayType        string `json:"gateway_type"`
	GatewayDevice      string `json:"gateway_device"`
	SiteID             string `json:"site_id,omitempty"`
}

// UbiquityAPIResponse represents the API response structure
type UbiquityAPIResponse struct {
	Meta struct {
		RC string `json:"rc"`
	} `json:"meta"`
	Data []UbiquityStaticRoute `json:"data,omitempty"`
}

// UbiquityLoginRequest represents the login request
type UbiquityLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UbiquityLoginResponse represents the login response
type UbiquityLoginResponse struct {
	Meta struct {
		RC string `json:"rc"`
	} `json:"meta"`
	Data []struct {
		XCsrfToken string `json:"x-csrf-token"`
	} `json:"data"`
}
