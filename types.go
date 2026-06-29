package main

import (
	"net"
	"sync"
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
	Name      string
	IPv6Addr  net.IP
	Services  []string
	LastSeen  time.Time
}

// ThreadBorderRouter represents a discovered Thread Border Router
type ThreadBorderRouter struct {
	Name     string
	IPv6Addr net.IP
	CIDR     string
	LastSeen time.Time
}

// Route represents a routing entry
type Route struct {
	CIDR             string
	ThreadRouterIPv6 string
	RouterName       string
}

// DaemonState holds the current state of discovered devices and routers
type DaemonState struct {
	mu                  sync.Mutex
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
	GatewayDevice    string
	CSRFToken        string
	SessionCookie    string
	LastLogin        time.Time
	RouteGracePeriod time.Duration
	DeviceExpiration time.Duration
}

// hasValidSession returns true if the session is present and less than 5 minutes old.
func (c *UbiquityConfig) hasValidSession() bool {
	return c.SessionCookie != "" && c.CSRFToken != "" && time.Since(c.LastLogin) < 5*time.Minute
}

// clearSession invalidates the cached session tokens.
func (c *UbiquityConfig) clearSession() {
	c.SessionCookie = ""
	c.CSRFToken = ""
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
