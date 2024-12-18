package manager

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/api"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/tasks"
	"github.com/Livepeer-Open-Pool/openpool-plugin/pkg"
	"github.com/gin-gonic/gin"
	"log"
	"os"
	"os/signal"
	"plugin"
	"strconv"
	"sync"
	"syscall"
)

// loadPlugin dynamically loads a `.so` plugin
func loadPlugin(path string, cfgRaw json.RawMessage) (pkg.Plugin, error) {
	p, err := plugin.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open plugin %s: %w", path, err)
	}

	symbol, err := p.Lookup("NewPlugin")
	if err != nil {
		return nil, fmt.Errorf("failed to lookup NewPlugin in %s: %w", path, err)
	}

	newPluginFunc, ok := symbol.(func(json.RawMessage) (pkg.Plugin, error))
	if !ok {
		return nil, fmt.Errorf("NewPlugin has incorrect signature in %s", path)
	}

	return newPluginFunc(cfgRaw)
}

func startPayoutLoop(dbStorage *storage.Storage, cfg *config.Config, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting payout loop...")
	tasks.StartPayoutLoop(dbStorage, &cfg.PayoutLoopConfig)
}

func startAPIServer(dbStorage *storage.Storage, cfg *config.Config, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Starting API server...")

	router := gin.Default()
	h := &api.Handler{Storage: dbStorage, Config: cfg}

	router.GET("/status", h.GetPoolStatus)
	router.GET("/transcoders", h.GetPoolTranscoders)

	portStr := ":" + strconv.Itoa(cfg.APIConfig.ServerPort)
	log.Printf("API server running on %s\n", portStr)
	if err := router.Run(portStr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func startPluginDataLoader(dbStorage *storage.Storage, cfg *config.Config, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Loading plugin...")

	pluginConfigRaw, err := json.Marshal(cfg.PluginConfig)
	if err != nil {
		log.Fatalf("Failed to marshal plugin config: %v", err)
		return
	}

	loadedPlugin, err := loadPlugin(cfg.PluginPath, pluginConfigRaw)
	if err != nil {
		log.Fatalf("Error loading plugin: %v", err)
		return
	}

	log.Printf("Successfully loaded plugin: %s", cfg.PluginPath)
	log.Printf("Starting Data Loader ...")
	tasks.StartDataLoader(dbStorage, loadedPlugin)
}

func main() {
	configFileName := flag.String("config", "/etc/pool/config.json", "Open Pool Configuration file")
	flag.Parse()

	log.Printf("Using config file: %s\n", *configFileName)

	cfg, err := config.LoadConfig(*configFileName)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	dbStorage, err := storage.NewDBStorage(cfg.DataStorageFilePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	var wg sync.WaitGroup

	// Start components based on config.json
	if cfg.PluginPath != "" {
		wg.Add(1)
		go startPluginDataLoader(dbStorage, cfg, &wg)
	}

	if cfg.PayoutLoopConfig.RPCUrl != "" {
		wg.Add(1)
		go startPayoutLoop(dbStorage, cfg, &wg)
	}

	if cfg.APIConfig.ServerPort != 0 {
		wg.Add(1)
		go startAPIServer(dbStorage, cfg, &wg)
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down...")
	wg.Wait()
	log.Println("All services stopped.")
}
