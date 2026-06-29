package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

// getMDNSInterface returns the network interface to use for mDNS queries, or nil for all interfaces.
func getMDNSInterface() *net.Interface {
	name := os.Getenv("MDNS_INTERFACE")
	if name == "" {
		return nil
	}
	iface, err := net.InterfaceByName(name)
	if err != nil {
		logWarn("MDNS_INTERFACE %q not found: %v — using all interfaces", name, err)
		return nil
	}
	return iface
}

// getUbiquityConfig returns the Ubiquity router configuration from environment variables.
func getUbiquityConfig() UbiquityConfig {
	routerHostname := envOrDefault("UBIQUITY_ROUTER_HOSTNAME", "unifi.local")
	username := envOrDefault("UBIQUITY_USERNAME", "ubnt")
	password := envOrDefault("UBIQUITY_PASSWORD", "ubnt")

	return UbiquityConfig{
		RouterHostname:   routerHostname,
		Username:         username,
		Password:         password,
		APIBaseURL:       fmt.Sprintf("https://%s", routerHostname),
		InsecureSSL:      os.Getenv("UBIQUITY_INSECURE_SSL") == "true",
		Enabled:          os.Getenv("UBIQUITY_ENABLED") == "true",
		GatewayDevice:    os.Getenv("UBIQUITY_GATEWAY_DEVICE"),
		RouteGracePeriod: parseDurationEnv("ROUTE_GRACE_PERIOD", 10*time.Minute),
		DeviceExpiration: parseDurationEnv("DEVICE_EXPIRATION", 10*time.Minute),
	}
}

// envOrDefault returns the environment variable value or a fallback if unset.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseDurationEnv parses a duration from an environment variable, falling back to def on error or absence.
func parseDurationEnv(key string, def time.Duration) time.Duration {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		logWarn("Invalid %s format %q, using default %s", key, s, def)
		return def
	}
	return d
}
