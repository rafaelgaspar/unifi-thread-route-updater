package main

import (
	"os"
	"testing"
)

// TestInitLogLevel tests the log level initialization function
func TestInitLogLevel(t *testing.T) {
	tests := []struct {
		name          string
		envValue      string
		expectedLevel LogLevel
	}{
		{
			name:          "DEBUG level",
			envValue:      "DEBUG",
			expectedLevel: DEBUG,
		},
		{
			name:          "INFO level",
			envValue:      "INFO",
			expectedLevel: INFO,
		},
		{
			name:          "WARN level",
			envValue:      "WARN",
			expectedLevel: WARN,
		},
		{
			name:          "WARNING level",
			envValue:      "WARNING",
			expectedLevel: WARN,
		},
		{
			name:          "ERROR level",
			envValue:      "ERROR",
			expectedLevel: ERROR,
		},
		{
			name:          "Invalid level defaults to INFO",
			envValue:      "INVALID",
			expectedLevel: INFO,
		},
		{
			name:          "Empty value defaults to INFO",
			envValue:      "",
			expectedLevel: INFO,
		},
		{
			name:          "Case insensitive",
			envValue:      "debug",
			expectedLevel: DEBUG,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalValue := os.Getenv("LOG_LEVEL")
			originalLevel := currentLogLevel

			// Set test value
			if err := os.Setenv("LOG_LEVEL", tt.envValue); err != nil {
				t.Fatalf("Failed to set LOG_LEVEL: %v", err)
			}

			// Reset to default before testing
			currentLogLevel = INFO

			// Test the function
			initLogLevel()

			// Check result
			if currentLogLevel != tt.expectedLevel {
				t.Errorf("initLogLevel() with LOG_LEVEL=%s set level to %v, want %v",
					tt.envValue, currentLogLevel, tt.expectedLevel)
			}

			// Restore original values
			if err := os.Setenv("LOG_LEVEL", originalValue); err != nil {
				t.Errorf("Failed to restore LOG_LEVEL: %v", err)
			}
			currentLogLevel = originalLevel
		})
	}
}

// TestLoggingFunctions tests the logging functions with different log levels
func TestLoggingFunctions(t *testing.T) {
	// Save original level
	originalLevel := currentLogLevel
	defer func() { currentLogLevel = originalLevel }()

	// Test each log level
	levels := []struct {
		level LogLevel
		name  string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
	}

	for _, level := range levels {
		t.Run(level.name, func(t *testing.T) {
			currentLogLevel = level.level

			// Test that logDebug only logs when level is DEBUG or lower
			if level.level <= DEBUG {
				// Should log (we can't easily test the actual output, but we can test it doesn't panic)
				logDebug("Test debug message")
			}

			// Test that logInfo only logs when level is INFO or lower
			if level.level <= INFO {
				logInfo("Test info message")
			}

			// Test that logWarn only logs when level is WARN or lower
			if level.level <= WARN {
				logWarn("Test warning message")
			}

			// Test that logError always logs
			logError("Test error message")
		})
	}
}
