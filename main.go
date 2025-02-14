package main

import (
	"flag"
	"github.com/Livepeer-Open-Pool/openpool-plugin/cmd"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

func main() {
	initGlobalLogger()

	componentLogLevels := map[string]log.Level{
		"SqliteStoragePlugin": parseEnvOrFallback("LOG_LEVEL_STORAGE", log.InfoLevel),
		"DataLoaderPlugin":    parseEnvOrFallback("LOG_LEVEL_DATALOADER", log.InfoLevel),
		"APIPlugin":           parseEnvOrFallback("LOG_LEVEL_API", log.InfoLevel),
		"PayoutLoopPlugin":    parseEnvOrFallback("LOG_LEVEL_PAYOUTLOOP", log.InfoLevel),
	}

	log.AddHook(&componentLogLevelHook{levels: componentLogLevels})
	logger := log.WithFields(log.Fields{"service": "open-pool-manager"})
	logger.Info("Starting up the application")

	configFileName := flag.String("config", "/etc/open-pool/config.json", "Open Pool Configuration file to use")
	flag.Parse()

	cmd.Run(*configFileName)
}

func initGlobalLogger() {
	// Example: read a global LOG_LEVEL from the environment
	logLevelEnv, exists := os.LookupEnv("LOG_LEVEL")
	if !exists || logLevelEnv == "" {
		logLevelEnv = "info"
	}

	lvl, err := log.ParseLevel(strings.ToLower(logLevelEnv))
	if err != nil {
		lvl = log.InfoLevel
	}

	log.SetLevel(lvl)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	log.WithField("global_level", lvl).Info("Initialized global logger")
}

// componentLogLevelHook is a custom Logrus hook that enforces a per-component log level.
type componentLogLevelHook struct {
	// A map from component name -> minimum level
	levels map[string]log.Level
}

// Fire is called for every log entry. We can drop or modify the entry.
func (h *componentLogLevelHook) Fire(entry *log.Entry) error {
	// Check if there's a "component" field
	comp, ok := entry.Data["component"].(string)
	if !ok {
		// If no component is set, just respect the global level.
		return nil
	}

	// If a specific level for this component is set, check if the entry passes
	desiredLevel, found := h.levels[comp]
	if found {
		// If the log entry is below the component's level, we drop it.
		if entry.Level < desiredLevel {
			// We can “abort” this entry by returning a special error
			// or you can manipulate it. Logrus doesn't have an official "skip" mechanism,
			// so returning nil means we do nothing. We'll do an approach of raising the level
			// so that the entry won't be displayed if it's below threshold:
			entry.Level = log.TraceLevel - 1 // Something below all recognized levels
		}
	}

	return nil
}

// Levels returns the log levels where this hook should fire
func (h *componentLogLevelHook) Levels() []log.Level {
	// We want to handle *all* levels and filter ourselves
	return log.AllLevels
}

// parseEnvOrFallback attempts to read an env variable for log level,
// falls back to the provided default if not set or invalid.
func parseEnvOrFallback(envVar string, fallback log.Level) log.Level {
	val := os.Getenv(envVar)
	if val == "" {
		return fallback
	}

	// Attempt to parse the string as a log level
	level, err := log.ParseLevel(strings.ToLower(val))
	if err != nil {
		return fallback
	}
	return level
}
