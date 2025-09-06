package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/grandcat/zeroconf"
)

// LogLevel represents the logging severity level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var (
	currentLogLevel LogLevel = INFO
)

// initLogLevel initializes the logging level from environment variable
func initLogLevel() {
	levelStr := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		currentLogLevel = DEBUG
	case "INFO":
		currentLogLevel = INFO
	case "WARN", "WARNING":
		currentLogLevel = WARN
	case "ERROR":
		currentLogLevel = ERROR
	default:
		currentLogLevel = INFO
	}
}

// logDebug logs debug messages
func logDebug(format string, args ...interface{}) {
	if currentLogLevel <= DEBUG {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// logInfo logs info messages
func logInfo(format string, args ...interface{}) {
	if currentLogLevel <= INFO {
		log.Printf("[INFO] "+format, args...)
	}
}

// logWarn logs warning messages
func logWarn(format string, args ...interface{}) {
	if currentLogLevel <= WARN {
		log.Printf("[WARN] "+format, args...)
	}
}

// logError logs error messages
func logError(format string, args ...interface{}) {
	if currentLogLevel <= ERROR {
		log.Printf("[ERROR] "+format, args...)
	}
}

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

// discoverMatterDevices discovers Matter devices using mDNS
func discoverMatterDevices() ([]DeviceInfo, error) {
	var devices []DeviceInfo
	serviceType := "_matter._tcp"

	serviceDevices, err := discoverService(serviceType, "Matter")
	if err != nil {
		return devices, fmt.Errorf("error discovering %s: %v", serviceType, err)
	}
	devices = append(devices, serviceDevices...)

	return devices, nil
}

// discoverThreadBorderRouters discovers Thread Border Routers using mDNS
func discoverThreadBorderRouters() ([]ThreadBorderRouter, error) {
	var routers []ThreadBorderRouter
	serviceType := "_meshcop._udp"

	serviceRouters, err := discoverThreadService(serviceType)
	if err != nil {
		return routers, fmt.Errorf("error discovering %s: %v", serviceType, err)
	}
	routers = append(routers, serviceRouters...)

	return routers, nil
}

// discoverService discovers services of a specific type
func discoverService(serviceType, deviceType string) ([]DeviceInfo, error) {
	var devices []DeviceInfo

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Browse for services
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return devices, fmt.Errorf("failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	done := make(chan bool)

	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()
		err := resolver.Browse(ctx, serviceType, "local.", entries)
		if err != nil {
			fmt.Printf("Failed to browse: %v\n", err)
		}
	}()

	// Process entries
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		for _, ip := range ipv6Addrs {
			device := DeviceInfo{
				Name:     entry.Instance,
				IPv6Addr: ip,
				Services: []string{serviceType},
			}
			devices = append(devices, device)
		}
	}

	// Signal that we're done processing
	close(done)

	return devices, nil
}

// discoverThreadService discovers Thread services
func discoverThreadService(serviceType string) ([]ThreadBorderRouter, error) {
	var routers []ThreadBorderRouter

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Browse for services
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return routers, fmt.Errorf("failed to initialize resolver: %v", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	done := make(chan bool)

	go func() {
		defer func() {
			select {
			case <-done:
				// Channel already closed
			default:
				close(entries)
			}
		}()
		err := resolver.Browse(ctx, serviceType, "local.", entries)
		if err != nil {
			fmt.Printf("Failed to browse: %v\n", err)
		}
	}()

	// Process entries
	for entry := range entries {
		if entry == nil {
			continue
		}

		ipv6Addrs := extractIPv6Addresses(entry)
		if len(ipv6Addrs) == 0 {
			continue
		}

		for _, ip := range ipv6Addrs {
			router := ThreadBorderRouter{
				Name:     extractRouterName(entry.Instance),
				IPv6Addr: ip,
				CIDR:     calculateCIDR64(ip),
			}
			routers = append(routers, router)
		}
	}

	// Signal that we're done processing
	close(done)

	return routers, nil
}

// extractIPv6Addresses extracts IPv6 addresses from zeroconf entry
func extractIPv6Addresses(entry *zeroconf.ServiceEntry) []net.IP {
	var ipv6Addrs []net.IP

	// Only use real IPv6 addresses, not IPv4 mapped addresses
	if entry.AddrIPv6 != nil {
		for _, ip := range entry.AddrIPv6 {
			// Check if it's a real IPv6 address (not IPv4 mapped)
			if ip.To4() == nil && ip.To16() != nil {
				ipv6Addrs = append(ipv6Addrs, ip)
			}
		}
	}

	// For Thread networks, we need IPv6 addresses
	// If no IPv6 addresses found, skip this device
	if len(ipv6Addrs) == 0 {
		return ipv6Addrs
	}

	return ipv6Addrs
}

// calculateCIDR64 calculates the /64 CIDR block for an IPv6 address
func calculateCIDR64(ip net.IP) string {
	if ip == nil {
		return ""
	}

	// For IPv4 addresses, return a placeholder
	if ip.To4() != nil {
		return "::/64"
	}

	// For IPv6 addresses, calculate /64 CIDR
	if ip.To16() != nil {
		// Take the first 8 bytes (64 bits) and set the rest to 0
		cidr := make(net.IP, 16)
		copy(cidr, ip[:8])
		return fmt.Sprintf("%s/64", cidr.String())
	}

	return ""
}

// extractRouterName extracts the simple router name from its FQDN
func extractRouterName(fqdn string) string {
	if idx := strings.Index(fqdn, "."); idx != -1 {
		return fqdn[:idx]
	}
	return fqdn
}

// formatDuration formats a duration to a human-readable string (e.g., "1h30m", "45m", "30s")
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		if minutes == 0 {
			return fmt.Sprintf("%dh", hours)
		}
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
}

// isRoutableCIDR checks if a CIDR block is routable (not link-local, loopback, etc.)
func isRoutableCIDR(cidr string) bool {
	// Parse the CIDR to get the network
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	ip := network.IP

	// Check for non-routable IPv6 address ranges
	// fe80::/10 - Link-local addresses
	if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
		return false
	}

	// ::1/128 - Loopback address
	if ip.Equal(net.ParseIP("::1")) {
		return false
	}

	// ::/128 - Unspecified address
	if ip.Equal(net.ParseIP("::")) {
		return false
	}

	// ff00::/8 - Multicast addresses
	if ip[0] == 0xff {
		return false
	}

	// 2001:db8::/32 - Documentation prefix (should not be routed)
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
		return false
	}

	// 2001::/32 - Teredo tunneling (usually not routed)
	if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
		return false
	}

	// 2002::/16 - 6to4 tunneling (deprecated, usually not routed)
	if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
		return false
	}

	// Note: fdc0::/7 (Unique Local Addresses) are valid for Thread Networks
	// but Thread Border Routers should use public IPv6 addresses

	return true
}

// isRoutableRouterAddress checks if a Thread Border Router IPv6 address is routable
// Thread Border Routers should only use public IPv6 addresses, not link-local or ULA
func isRoutableRouterAddress(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// For IPv4 addresses, return false (we only want IPv6)
	if ip.To4() != nil {
		return false
	}

	// For IPv6 addresses, check for non-routable ranges
	if ip.To16() != nil {
		// fe80::/10 - Link-local addresses
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return false
		}

		// ::1/128 - Loopback address
		if ip.Equal(net.ParseIP("::1")) {
			return false
		}

		// ::/128 - Unspecified address
		if ip.Equal(net.ParseIP("::")) {
			return false
		}

		// ff00::/8 - Multicast addresses
		if ip[0] == 0xff {
			return false
		}

		// fdc0::/7 - Unique Local Addresses (ULA) - Thread Border Routers should use public addresses
		if len(ip) >= 1 && (ip[0]&0xfe) == 0xfc {
			return false
		}

		// 2001:db8::/32 - Documentation prefix
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8 {
			return false
		}

		// 2001::/32 - Teredo tunneling
		if len(ip) >= 4 && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x00 && ip[3] == 0x00 {
			return false
		}

		// 2002::/16 - 6to4 tunneling
		if len(ip) >= 2 && ip[0] == 0x20 && ip[1] == 0x02 {
			return false
		}
	}

	return true
}

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
		fmt.Printf("❌ Failed to initialize resolver for Matter devices: %v\n", err)
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
			fmt.Printf("❌ Failed to browse for Matter devices: %v\n", err)
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
			}

			// Check if device already exists
			found := false
			for i, existingDevice := range state.MatterDevices {
				if existingDevice.Name == device.Name && existingDevice.IPv6Addr.Equal(device.IPv6Addr) {
					state.MatterDevices[i] = device
					found = true
					break
				}
			}

			if !found {
				state.MatterDevices = append(state.MatterDevices, device)
			}
		}

		state.LastUpdate = time.Now()
	}
}

// listenForThreadBorderRouters passively listens for Thread Border Router announcements
func listenForThreadBorderRouters(state *DaemonState, done <-chan struct{}) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		fmt.Printf("❌ Failed to initialize resolver for Thread Border Routers: %v\n", err)
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
			fmt.Printf("❌ Failed to browse for Thread Border Routers: %v\n", err)
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
			}

			// Check if router already exists
			found := false
			for i, existingRouter := range state.ThreadBorderRouters {
				if existingRouter.Name == router.Name && existingRouter.IPv6Addr.Equal(router.IPv6Addr) {
					state.ThreadBorderRouters[i] = router
					found = true
					break
				}
			}

			if !found {
				state.ThreadBorderRouters = append(state.ThreadBorderRouters, router)
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
			// Gentle refresh - only if we haven't seen updates in a while
			if time.Since(state.LastUpdate) > 2*time.Minute {
				fmt.Println("🔄 Performing gentle refresh...")

				// Quick discovery without overwhelming the network
				devices, err := discoverMatterDevices()
				if err == nil && len(devices) > 0 {
					state.MatterDevices = devices
				}

				routers, err := discoverThreadBorderRouters()
				if err == nil && len(routers) > 0 {
					state.ThreadBorderRouters = routers
				}

				state.LastUpdate = time.Now()
			}
		case <-done:
			return
		}
	}
}

// getUbiquityConfig returns the Ubiquity router configuration
func getUbiquityConfig() UbiquityConfig {
	// Get configuration from environment variables or use defaults
	routerHostname := os.Getenv("UBIQUITY_ROUTER_HOSTNAME")
	if routerHostname == "" {
		routerHostname = "unifi.local" // Default router hostname
	}

	username := os.Getenv("UBIQUITY_USERNAME")
	if username == "" {
		username = "ubnt" // Default username
	}

	password := os.Getenv("UBIQUITY_PASSWORD")
	if password == "" {
		password = "ubnt" // Default password
	}

	enabled := os.Getenv("UBIQUITY_ENABLED") == "true"

	// Parse route grace period from environment variable
	gracePeriodStr := os.Getenv("ROUTE_GRACE_PERIOD")
	gracePeriod := 10 * time.Minute // Default: 10 minutes
	if gracePeriodStr != "" {
		if parsed, err := time.ParseDuration(gracePeriodStr); err == nil {
			gracePeriod = parsed
		} else {
			fmt.Printf("⚠️ Invalid ROUTE_GRACE_PERIOD format '%s', using default 10m\n", gracePeriodStr)
		}
	}

	return UbiquityConfig{
		RouterHostname:   routerHostname,
		Username:         username,
		Password:         password,
		APIBaseURL:       fmt.Sprintf("https://%s", routerHostname),
		InsecureSSL:      os.Getenv("UBIQUITY_INSECURE_SSL") == "true",
		Enabled:          enabled,
		RouteGracePeriod: gracePeriod,
	}
}

// updateUbiquityRoutes updates the static routes on the Ubiquity router
func updateUbiquityRoutes(state *DaemonState, routes []Route) {
	if !state.UbiquityConfig.Enabled {
		return
	}

	fmt.Println("🔄 Updating Ubiquity router static routes...")

	// Check if we have valid session tokens and they're not too old
	// Only re-authenticate if we don't have tokens or they're expired
	currentTime := time.Now().Unix()
	timeSinceLastLogin := currentTime - state.UbiquityConfig.LastLoginTime

	if state.UbiquityConfig.SessionCookie == "" || state.UbiquityConfig.CSRFToken == "" {
		fmt.Println("🔐 No valid session tokens, authenticating...")
		err := loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to login to Ubiquity router: %v\n", err)
			return
		}
	} else if timeSinceLastLogin > 300 { // 5 minutes
		fmt.Printf("🔐 Session tokens expired (%d seconds old), re-authenticating...\n", timeSinceLastLogin)
		err := loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to login to Ubiquity router: %v\n", err)
			return
		}
	} else {
		logDebug("Using existing session tokens (%d seconds old)", timeSinceLastLogin)
	}

	// Get current routes from router
	currentRoutes, err := getUbiquityStaticRoutes(state.UbiquityConfig)
	fmt.Printf("🔍 getUbiquityStaticRoutes returned %d routes, error: %v\n", len(currentRoutes), err)
	if err != nil {
		fmt.Printf("❌ Failed to get current routes: %v\n", err)
		// If we get a rate limit error, don't try to re-login immediately
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "AUTHENTICATION_FAILED_LIMIT_REACHED") {
			fmt.Println("⚠️ Rate limit reached, skipping this update cycle...")
			// Clear all session tokens to force fresh login next time
			state.UbiquityConfig.SessionToken = ""
			state.UbiquityConfig.SessionCookie = ""
			state.UbiquityConfig.CSRFToken = ""
			return
		}
		// For other auth errors, try to re-login and retry once
		state.UbiquityConfig.SessionToken = ""
		state.UbiquityConfig.SessionCookie = ""
		state.UbiquityConfig.CSRFToken = ""
		err = loginToUbiquity(&state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to re-login to Ubiquity router: %v\n", err)
			return
		}
		currentRoutes, err = getUbiquityStaticRoutes(state.UbiquityConfig)
		if err != nil {
			fmt.Printf("❌ Failed to get current routes after re-login: %v\n", err)
			return
		}
	}

	// Convert our routes to Ubiquity format
	desiredRoutes := convertToUbiquityRoutes(routes)

	// Update last seen time for current desired routes
	routeUpdateTime := time.Now()
	for _, route := range desiredRoutes {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		state.RouteLastSeen[key] = routeUpdateTime
	}

	// Debug: Show current routes from API (only if there are routes to remove)
	if len(currentRoutes) > 0 {
		logDebug("Current routes from API (%d total)", len(currentRoutes))
		for _, route := range currentRoutes {
			logDebug("  - %s -> %s (ID: %s, Name: %s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID, route.Name)
		}
	}

	// Find routes to add and remove (with grace period consideration)
	routesToAdd, routesToRemove := compareRoutesWithGracePeriod(currentRoutes, desiredRoutes, state.RouteLastSeen, state.UbiquityConfig.RouteGracePeriod)

	// Show summary if there are changes or if we have routes being tracked
	if len(routesToAdd) > 0 || len(routesToRemove) > 0 || len(state.RouteLastSeen) > 0 {
		fmt.Printf("🔄 Route changes: +%d routes, -%d routes (grace period: %s)\n",
			len(routesToAdd), len(routesToRemove), formatDuration(state.UbiquityConfig.RouteGracePeriod))

		// Show grace period status for tracked routes
		if len(state.RouteLastSeen) > 0 {
			currentTime := time.Now()
			for key, lastSeen := range state.RouteLastSeen {
				timeSinceLastSeen := currentTime.Sub(lastSeen)
				if timeSinceLastSeen < state.UbiquityConfig.RouteGracePeriod {
					remaining := state.UbiquityConfig.RouteGracePeriod - timeSinceLastSeen
					remainingStr := formatDuration(remaining)
					fmt.Printf("⏳ Route %s still within grace period (%s remaining)\n", key, remainingStr)
				}
			}
		}
	}

	// Filter out routes we've already added (in-memory tracking)
	var newRoutesToAdd []UbiquityStaticRoute
	for _, route := range routesToAdd {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		if !state.AddedRoutes[key] {
			newRoutesToAdd = append(newRoutesToAdd, route)
			state.AddedRoutes[key] = true // Mark as added
		}
	}
	routesToAdd = newRoutesToAdd

	// Add a small delay after adding routes to allow them to be indexed
	if len(routesToAdd) > 0 {
		time.Sleep(2 * time.Second)
	}

	// Remove old routes
	for _, route := range routesToRemove {
		fmt.Printf("🗑️  Attempting to delete route: %s -> %s (ID: %s)\n",
			route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID)
		if err := deleteUbiquityStaticRoute(state.UbiquityConfig, route.ID); err != nil {
			fmt.Printf("❌ Failed to delete route %s (ID: %s): %v\n", route.StaticRouteNetwork, route.ID, err)
			// If the route ID is invalid, it might have been manually deleted
			// Remove it from our tracking to prevent repeated attempts
			if strings.Contains(err.Error(), "IdInvalid") {
				fmt.Printf("⚠️  Route ID invalid, likely already deleted. Removing from tracking.\n")
				// Remove from in-memory tracking
				key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
				delete(state.RouteLastSeen, key)
				delete(state.AddedRoutes, key)
			}
		} else {
			fmt.Printf("✅ Deleted route: %s -> %s\n", route.StaticRouteNetwork, route.StaticRouteNexthop)
		}
	}

	// Add new routes
	for _, route := range routesToAdd {
		if err := addUbiquityStaticRoute(state.UbiquityConfig, route); err != nil {
			fmt.Printf("❌ Failed to add route %s: %v\n", route.StaticRouteNetwork, err)
		} else {
			fmt.Printf("✅ Added route: %s -> %s (%s)\n", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
		}
	}

	if len(routesToAdd) == 0 && len(routesToRemove) == 0 {
		fmt.Println("✅ Ubiquity routes are up to date")
	}
}

// getUbiquityStaticRoutes retrieves current static routes from the router
func getUbiquityStaticRoutes(config UbiquityConfig) ([]UbiquityStaticRoute, error) {
	client := createHTTPClient(config)

	// Try multiple endpoints to find the correct one for reading routes
	// Prioritize the correct endpoint that actually returns routes
	endpoints := []string{
		fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing", config.APIBaseURL),
		fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/static-route", config.APIBaseURL),
		fmt.Sprintf("%s/api/s/default/rest/routing/static-route", config.APIBaseURL),
	}

	for _, url := range endpoints {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}

		// Add session authentication
		req.Header.Set("Content-Type", "application/json")

		// Use session cookie as Authorization header
		if config.SessionCookie != "" {
			req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
		}

		if config.CSRFToken != "" {
			req.Header.Set("X-CSRF-Token", config.CSRFToken)
		}

		if config.SessionCookie != "" {
			req.AddCookie(&http.Cookie{
				Name:  "TOKEN",
				Value: config.SessionCookie,
			})
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			var apiResp UbiquityAPIResponse
			if err := json.Unmarshal(body, &apiResp); err != nil {
				continue
			}

			if apiResp.Meta.RC == "ok" {
				return apiResp.Data, nil
			}
		}
	}

	// If all endpoints failed, return empty array but log the issue
	fmt.Printf("⚠️ All endpoints failed, returning empty routes array\n")
	return []UbiquityStaticRoute{}, nil
}

// addUbiquityStaticRoute adds a new static route to the router
func addUbiquityStaticRoute(config UbiquityConfig, route UbiquityStaticRoute) error {
	client := createHTTPClient(config)

	// Try the UDM Pro/UCG Max endpoint first
	url := fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/static-route", config.APIBaseURL)

	jsonData, err := json.Marshal(route)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Add session authentication
	req.Header.Set("Content-Type", "application/json")

	// Use session cookie as Authorization header
	if config.SessionCookie != "" {
		req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
	}

	if config.CSRFToken != "" {
		// Use CSRF token in X-CSRF-Token header
		req.Header.Set("X-CSRF-Token", config.CSRFToken)
	}
	if config.SessionCookie != "" {
		req.AddCookie(&http.Cookie{
			Name:  "TOKEN",
			Value: config.SessionCookie,
		})
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// deleteUbiquityStaticRoute deletes a static route from the router
func deleteUbiquityStaticRoute(config UbiquityConfig, routeID string) error {
	client := createHTTPClient(config)

	// Try both endpoints - legacy first, then UDM Pro
	endpoints := []string{
		fmt.Sprintf("%s/api/s/default/rest/routing/static-route/%s", config.APIBaseURL, routeID),
		fmt.Sprintf("%s/proxy/network/api/s/default/rest/routing/static-route/%s", config.APIBaseURL, routeID),
	}

	var lastErr error
	for i, url := range endpoints {
		req, err := http.NewRequest("DELETE", url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		// Add session authentication
		req.Header.Set("Content-Type", "application/json")

		// Use session cookie as Authorization header
		if config.SessionCookie != "" {
			req.Header.Set("Authorization", "Bearer "+config.SessionCookie)
		}

		if config.CSRFToken != "" {
			// Use CSRF token in X-CSRF-Token header
			req.Header.Set("X-CSRF-Token", config.CSRFToken)
		}
		if config.SessionCookie != "" {
			req.AddCookie(&http.Cookie{
				Name:  "TOKEN",
				Value: config.SessionCookie,
			})
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		// Check response status
		if resp.StatusCode == http.StatusOK {
			fmt.Printf("🔍 DELETE response: %d - Success (endpoint %d)\n", resp.StatusCode, i+1)
			resp.Body.Close()
			return nil
		}

		// Log the error and try next endpoint
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("🔍 DELETE response: %d - %s (endpoint %d)\n", resp.StatusCode, string(body), i+1)
		resp.Body.Close()
		lastErr = fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// If we get here, all endpoints failed
	return fmt.Errorf("all deletion endpoints failed, last error: %v", lastErr)
}

// createHTTPClient creates an HTTP client with appropriate settings
func createHTTPClient(config UbiquityConfig) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSSL,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
}

// convertToUbiquityRoutes converts our Route format to Ubiquity format
func convertToUbiquityRoutes(routes []Route) []UbiquityStaticRoute {
	var ubiquityRoutes []UbiquityStaticRoute

	for _, route := range routes {
		// Remove escaping from router name for cleaner display
		cleanRouterName := strings.ReplaceAll(route.RouterName, "\\", "")

		// Use the correct Ubiquiti field structure
		ubiquityRoute := UbiquityStaticRoute{
			Enabled:            true,
			Name:               fmt.Sprintf("Thread route via %s", cleanRouterName),
			Type:               "static-route",
			StaticRouteNexthop: route.ThreadRouterIPv6,
			StaticRouteNetwork: route.CIDR,
			StaticRouteType:    "nexthop-route",
			GatewayType:        "default",
			GatewayDevice:      "1c:0b:8b:12:64:88", // TODO: Get actual gateway device MAC
		}
		ubiquityRoutes = append(ubiquityRoutes, ubiquityRoute)
	}

	return ubiquityRoutes
}

// compareRoutesWithGracePeriod compares current and desired routes with grace period consideration
func compareRoutesWithGracePeriod(current, desired []UbiquityStaticRoute, routeLastSeen map[string]time.Time, gracePeriod time.Duration) ([]UbiquityStaticRoute, []UbiquityStaticRoute) {
	var toAdd []UbiquityStaticRoute
	var toRemove []UbiquityStaticRoute
	currentTime := time.Now()

	// Debug: Show what we're comparing (only in debug mode)
	logDebug("Comparing routes - Current: %d, Desired: %d", len(current), len(desired))
	for _, route := range current {
		logDebug("  Current: %s -> %s (ID: %s, Name: %s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.ID, route.Name)
	}
	for _, route := range desired {
		logDebug("  Desired: %s -> %s (Name: %s)", route.StaticRouteNetwork, route.StaticRouteNexthop, route.Name)
	}

	// Create a map of desired routes for quick lookup
	desiredMap := make(map[string]UbiquityStaticRoute)
	for _, route := range desired {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		desiredMap[key] = route
	}

	// Find routes to remove (in current but not in desired)
	for _, currentRoute := range current {
		key := fmt.Sprintf("%s->%s", currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop)
		if _, exists := desiredMap[key]; !exists {
			// Only remove Thread routes (routes with our name pattern)
			if strings.Contains(currentRoute.Name, "Thread route via") {
				// Check if we should respect grace period
				if lastSeen, hasLastSeen := routeLastSeen[key]; hasLastSeen {
					timeSinceLastSeen := currentTime.Sub(lastSeen)
					if timeSinceLastSeen < gracePeriod {
						remaining := gracePeriod - timeSinceLastSeen
						remainingStr := formatDuration(remaining)
						fmt.Printf("⏳ Route %s -> %s still within grace period (%s remaining), not removing\n",
							currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop, remainingStr)
						continue
					}
				} else {
					// Route was never seen before - treat as if it was just seen to give it grace period
					gracePeriodStr := formatDuration(gracePeriod)
					fmt.Printf("⏳ Route %s -> %s never seen before, giving grace period (%s), not removing\n",
						currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop, gracePeriodStr)
					// Mark it as seen now so it gets the full grace period
					routeLastSeen[key] = currentTime
					continue
				}
				logDebug("🗑️ Marking route for removal: %s -> %s (ID: %s)",
					currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop, currentRoute.ID)
				toRemove = append(toRemove, currentRoute)
			}
		}
	}

	// Find routes to add (in desired but not in current)
	currentMap := make(map[string]bool)
	for _, route := range current {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		currentMap[key] = true
	}

	for _, desiredRoute := range desired {
		key := fmt.Sprintf("%s->%s", desiredRoute.StaticRouteNetwork, desiredRoute.StaticRouteNexthop)
		if !currentMap[key] {
			toAdd = append(toAdd, desiredRoute)
		}
	}

	return toAdd, toRemove
}

// compareRoutes compares current and desired routes to find what needs to be added/removed
// This is the original function kept for backward compatibility
func compareRoutes(current, desired []UbiquityStaticRoute) ([]UbiquityStaticRoute, []UbiquityStaticRoute) {
	var toAdd []UbiquityStaticRoute
	var toRemove []UbiquityStaticRoute

	// Create a map of desired routes for quick lookup
	desiredMap := make(map[string]UbiquityStaticRoute)
	for _, route := range desired {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		desiredMap[key] = route
	}

	// Find routes to remove (in current but not in desired)
	for _, currentRoute := range current {
		key := fmt.Sprintf("%s->%s", currentRoute.StaticRouteNetwork, currentRoute.StaticRouteNexthop)
		if _, exists := desiredMap[key]; !exists {
			// Only remove Thread routes (routes with our name pattern)
			if strings.Contains(currentRoute.Name, "Thread route via") {
				toRemove = append(toRemove, currentRoute)
			}
		}
	}

	// Find routes to add (in desired but not in current)
	currentMap := make(map[string]bool)
	for _, route := range current {
		key := fmt.Sprintf("%s->%s", route.StaticRouteNetwork, route.StaticRouteNexthop)
		currentMap[key] = true
	}

	for _, desiredRoute := range desired {
		key := fmt.Sprintf("%s->%s", desiredRoute.StaticRouteNetwork, desiredRoute.StaticRouteNexthop)
		if !currentMap[key] {
			toAdd = append(toAdd, desiredRoute)
		}
	}

	return toAdd, toRemove
}

// loginToUbiquity authenticates with the Ubiquity router and gets a session token
func loginToUbiquity(config *UbiquityConfig) error {
	client := createHTTPClient(*config)

	// Login endpoint
	url := fmt.Sprintf("%s/api/auth/login", config.APIBaseURL)

	loginReq := UbiquityLoginRequest{
		Username: config.Username,
		Password: config.Password,
	}

	jsonData, err := json.Marshal(loginReq)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			fmt.Printf("⚠️ Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	// Read the response body first to debug
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read login response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Try to parse as the expected format first
	var loginResp UbiquityLoginResponse
	if err := json.Unmarshal(body, &loginResp); err == nil && loginResp.Meta.RC == "ok" {
		// Standard format with meta.rc
		if len(loginResp.Data) > 0 {
			config.SessionToken = loginResp.Data[0].XCsrfToken
		}
	} else {
		// Alternative format - direct user profile response
		var userProfile map[string]interface{}
		if err := json.Unmarshal(body, &userProfile); err != nil {
			return fmt.Errorf("failed to parse login response: %v, body: %s", err, string(body))
		}

		// Check if we have a valid user profile
		if username, ok := userProfile["username"].(string); ok && username == config.Username {
			// Login successful
			// Use the device token as the session token
			if deviceToken, ok := userProfile["deviceToken"].(string); ok {
				config.SessionToken = deviceToken
				config.LastLoginTime = time.Now().Unix()
			}
		} else {
			return fmt.Errorf("login failed: invalid user profile, body: %s", string(body))
		}
	}

	// Extract CSRF token from response headers (but don't override device token)
	csrfToken := resp.Header.Get("X-CSRF-Token")
	if csrfToken != "" {
		config.CSRFToken = csrfToken
	}

	// Also set the session cookie
	for _, cookie := range resp.Cookies() {
		// Ubiquity uses TOKEN cookie instead of unifises
		if cookie.Name == "TOKEN" || cookie.Name == "unifises" {
			// Store the session cookie value for future requests
			config.SessionCookie = cookie.Value
		}
	}

	return nil
}
