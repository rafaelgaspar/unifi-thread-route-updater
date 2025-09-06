package main

import (
	"os"
	"testing"
	"time"
)

// TestGetUbiquityConfig tests the configuration loading function
func TestGetUbiquityConfig(t *testing.T) {
	// Save original environment variables
	originalVars := map[string]string{
		"UBIQUITY_ROUTER_HOSTNAME": os.Getenv("UBIQUITY_ROUTER_HOSTNAME"),
		"UBIQUITY_USERNAME":        os.Getenv("UBIQUITY_USERNAME"),
		"UBIQUITY_PASSWORD":        os.Getenv("UBIQUITY_PASSWORD"),
		"UBIQUITY_INSECURE_SSL":    os.Getenv("UBIQUITY_INSECURE_SSL"),
		"UBIQUITY_ENABLED":         os.Getenv("UBIQUITY_ENABLED"),
	}

	// Clean up after test
	defer func() {
		for key, value := range originalVars {
			if value == "" {
				if err := os.Unsetenv(key); err != nil {
					t.Errorf("Failed to unset %s: %v", key, err)
				}
			} else {
				if err := os.Setenv(key, value); err != nil {
					t.Errorf("Failed to set %s: %v", key, err)
				}
			}
		}
	}()

	t.Run("Default configuration", func(t *testing.T) {
		// Clear all environment variables
		for key := range originalVars {
			if err := os.Unsetenv(key); err != nil {
				t.Errorf("Failed to unset %s: %v", key, err)
			}
		}

		config := getUbiquityConfig()

		// Check defaults (the function has hardcoded defaults)
		if config.RouterHostname != "unifi.local" {
			t.Errorf("Expected RouterHostname 'unifi.local', got %s", config.RouterHostname)
		}
		if config.Username != "ubnt" {
			t.Errorf("Expected Username 'ubnt', got %s", config.Username)
		}
		if config.Password != "ubnt" {
			t.Errorf("Expected Password 'ubnt', got %s", config.Password)
		}
		if config.APIBaseURL != "https://unifi.local" {
			t.Errorf("Expected APIBaseURL 'https://unifi.local', got %s", config.APIBaseURL)
		}
		if config.InsecureSSL {
			t.Error("Expected InsecureSSL to be false")
		}
		if config.Enabled {
			t.Error("Expected Enabled to be false")
		}
	})

	t.Run("Configuration with environment variables", func(t *testing.T) {
		// Set test environment variables
		if err := os.Setenv("UBIQUITY_ROUTER_HOSTNAME", "test-router.local"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_ROUTER_HOSTNAME: %v", err)
		}
		if err := os.Setenv("UBIQUITY_USERNAME", "testuser"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_USERNAME: %v", err)
		}
		if err := os.Setenv("UBIQUITY_PASSWORD", "testpass"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_PASSWORD: %v", err)
		}
		if err := os.Setenv("UBIQUITY_INSECURE_SSL", "true"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_INSECURE_SSL: %v", err)
		}
		if err := os.Setenv("UBIQUITY_ENABLED", "true"); err != nil {
			t.Fatalf("Failed to set UBIQUITY_ENABLED: %v", err)
		}

		config := getUbiquityConfig()

		// Check values
		if config.RouterHostname != "test-router.local" {
			t.Errorf("Expected RouterHostname 'test-router.local', got %s", config.RouterHostname)
		}
		if config.Username != "testuser" {
			t.Errorf("Expected Username 'testuser', got %s", config.Username)
		}
		if config.Password != "testpass" {
			t.Errorf("Expected Password 'testpass', got %s", config.Password)
		}
		if !config.InsecureSSL {
			t.Error("Expected InsecureSSL to be true")
		}
		if !config.Enabled {
			t.Error("Expected Enabled to be true")
		}
	})
}

// TestGetUbiquityConfigEdgeCases tests edge cases for configuration parsing
func TestGetUbiquityConfigEdgeCases(t *testing.T) {
	// Save original environment
	originalEnv := map[string]string{
		"UBIQUITY_ROUTER_HOSTNAME": os.Getenv("UBIQUITY_ROUTER_HOSTNAME"),
		"UBIQUITY_USERNAME":        os.Getenv("UBIQUITY_USERNAME"),
		"UBIQUITY_PASSWORD":        os.Getenv("UBIQUITY_PASSWORD"),
		"UBIQUITY_INSECURE_SSL":    os.Getenv("UBIQUITY_INSECURE_SSL"),
		"ROUTE_GRACE_PERIOD":       os.Getenv("ROUTE_GRACE_PERIOD"),
	}

	// Restore environment after test
	defer func() {
		for key, value := range originalEnv {
			if value == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, value)
			}
		}
	}()

	t.Run("Invalid grace period should use default", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "invalid-duration")
		config := getUbiquityConfig()
		expected := 10 * time.Minute // Default grace period
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v for invalid duration, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Empty grace period should use default", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "")
		config := getUbiquityConfig()
		expected := 10 * time.Minute // Default grace period
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v for empty duration, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Valid grace period should be parsed", func(t *testing.T) {
		_ = os.Setenv("ROUTE_GRACE_PERIOD", "5m")
		config := getUbiquityConfig()
		expected := 5 * time.Minute
		if config.RouteGracePeriod != expected {
			t.Errorf("Expected grace period %v, got %v", expected, config.RouteGracePeriod)
		}
	})

	t.Run("Insecure SSL should be parsed correctly", func(t *testing.T) {
		_ = os.Setenv("UBIQUITY_INSECURE_SSL", "true")
		config := getUbiquityConfig()
		if !config.InsecureSSL {
			t.Errorf("Expected InsecureSSL to be true, got false")
		}
	})

	t.Run("Secure SSL should be parsed correctly", func(t *testing.T) {
		_ = os.Setenv("UBIQUITY_INSECURE_SSL", "false")
		config := getUbiquityConfig()
		if config.InsecureSSL {
			t.Errorf("Expected InsecureSSL to be false, got true")
		}
	})
}
