package main

import (
	"fmt"
	"os"
	"time"
)

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
