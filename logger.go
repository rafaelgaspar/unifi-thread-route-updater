package main

import (
	"log"
	"os"
	"strings"
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
