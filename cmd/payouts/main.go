package main

import (
	"flag"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/tasks"
	"log"
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
	fmt.Printf("Starting the Payout Loop...\n")

	tasks.StartPayoutLoop(dbStorage, &cfg.PayoutLoopConfig)
}
