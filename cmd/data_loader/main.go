package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"plugin"

	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/tasks"
	"github.com/Livepeer-Open-Pool/openpool-plugin/pkg"
)

// loadPlugin loads the plugin dynamically from a `.so` file
func loadPlugin(path string, cfgRaw json.RawMessage) (pkg.Plugin, error) {
	// Open the compiled plugin file
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	// Lookup the `NewPlugin` symbol
	symbol, err := p.Lookup("NewPlugin")
	if err != nil {
		return nil, fmt.Errorf("failed to lookup NewPlugin in %s: %w", path, err)
	}

	// Ensure the symbol has the correct function signature
	newPluginFunc, ok := symbol.(func(json.RawMessage) (pkg.Plugin, error))
	if !ok {
		return nil, fmt.Errorf("NewPlugin has incorrect signature in %s", path)
	}

	// Call the function to initialize the plugin
	return newPluginFunc(cfgRaw)
}

func main() {
	// Parse CLI flags
	configFileName := flag.String("config", "/etc/pool/config.json", "Open Pool Configuration file to use")
	flag.Parse()
	fmt.Printf("Using config file: %s\n", *configFileName)

	// Load configuration from JSON file
	cfg, err := config.LoadConfig(*configFileName)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
		return
	}

	// Initialize database storage
	dbStorage, err := storage.NewDBStorage(cfg.DataStorageFilePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
		return
	}

	// Marshal the plugin-specific configuration
	pluginConfigRaw, err := json.Marshal(cfg.PluginConfig)
	if err != nil {
		log.Fatalf("Failed to marshal plugin config: %v", err)
		return
	}

	// Load the plugin dynamically
	loadedPlugin, err := loadPlugin(cfg.PluginPath, pluginConfigRaw)
	if err != nil {
		log.Fatalf("Error loading plugin from %s: %v", cfg.PluginPath, err)
		return
	}

	log.Printf("Successfully loaded plugin: %s \n", cfg.PluginPath)

	fmt.Println("Starting the Data Loader...")
	// Start the data loader with the plugin
	tasks.StartDataLoader(dbStorage, loadedPlugin)
}
