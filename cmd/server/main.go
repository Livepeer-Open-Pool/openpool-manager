package main

import (
	"flag"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/api"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/gin-gonic/gin"
	"log"
	"strconv"
)

func main() {
	configFileName := flag.String("config", "/etc/pool/config.json", "Open Pool Configuration file to use")
	flag.Parse()
	fmt.Printf("Using config file: %s\n", *configFileName)

	cfg, err := config.LoadConfig(*configFileName)
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
		return
	}

	dbStorage, err := storage.NewDBStorage(cfg.DataStorageFilePath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
		return
	}
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create a new Gin router
	router := gin.Default()

	// Initialize the handler with the storage
	h := &api.Handler{
		Storage: dbStorage,
		Config:  cfg,
	}

	// Legacy Pool Endpoints
	router.GET("/status", h.GetPoolStatus)
	router.GET("/transcoders", h.GetPoolTranscoders)

	// Start the HTTP server
	portStr := ":" + strconv.Itoa(cfg.APIConfig.ServerPort)

	log.Println("Starting server on ", portStr)
	if err := router.Run(portStr); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
