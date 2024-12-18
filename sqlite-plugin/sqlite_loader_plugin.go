package main

import (
	"encoding/json"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"github.com/Livepeer-Open-Pool/openpool-plugin/pkg"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
)

// DataSource represents a single data source as provided in the PluginConfig.
type DataSource struct {
	Endpoint string `json:"endpoint"`
	NodeType string `json:"nodeType"`
}

// SqliteLoaderPluginConfig is the plugin-specific configuration expected in PluginConfig.
type SqliteLoaderPluginConfig struct {
	DBPath         string       `json:"db_path"`
	Region         string       `json:"region"`
	DataSources    []DataSource `json:"data_sources"`
	PoolCommission float64      `json:"pool_commission"`
}

// SqliteLoaderPlugin implements app_plugin.Plugin.
type SqliteLoaderPlugin struct {
	db               *gorm.DB
	remoteWorkerRepo storage.RemoteWorkerRepository
	region           string
	dataSources      []DataSource
	poolCommission   float64
}

// NewPlugin is the exported constructor. The main app calls this via plugin.Lookup("NewPlugin").
func NewPlugin(rawCfg json.RawMessage) (pkg.Plugin, error) {
	fmt.Println("PLUGIN LOADING")

	var cfg SqliteLoaderPluginConfig
	if len(rawCfg) > 0 {
		if err := json.Unmarshal(rawCfg, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse plugin config: %w", err)
		}
	}

	gdb, err := gorm.Open(sqlite.Open(cfg.DBPath), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	fmt.Println("PLUGIN db opened")

	// Auto-migrate our models.
	if err := gdb.AutoMigrate(&models.RemoteWorker{}, &models.EventLog{}, &models.PoolPayout{}); err != nil {
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}
	fmt.Println("PLUGIN db migrated")

	remoteWorkerRepo := storage.NewRemoteWorkerRepository(gdb)

	plugin := &SqliteLoaderPlugin{
		db:               gdb,
		remoteWorkerRepo: remoteWorkerRepo,
		region:           cfg.Region,
		dataSources:      cfg.DataSources,
		poolCommission:   cfg.PoolCommission,
	}
	fmt.Println("PLUGIN LOADED")
	return plugin, nil
}

// Apply processes events in a GORM transaction, using the provided endpointHash to restrict updates.
func (p *SqliteLoaderPlugin) Apply(events []models.PoolEvent, endpointHash string) error {
	// Run all event processing in a single transaction.
	return p.db.Transaction(func(tx *gorm.DB) error {
		// Create a repository instance scoped to this transaction.
		r := storage.NewRemoteWorkerRepository(tx)

		// Create an in-memory map to hold job-received events for AI workers (keyed by RequestID).
		receivedJobs := make(map[string]models.JobReceivedPayload)

		for _, event := range events {
			log.Printf("Parsing payload for event: %v", event)

			// Parse the JSON payload into its proper struct.
			if err := event.ParsePayload(); err != nil {
				log.Printf("Failed to parse payload for event ID %d: %v", event.ID, err)
				continue // skip malformed events
			}

			switch event.ParsedPayload.EventType {

			case "worker-connected":
				payload, ok := event.ParsedPayload.Payload.(models.RemoteWorker)
				if !ok {
					log.Printf("invalid payload type for worker-connected event ID %d", event.ID)
					continue
				}
				log.Printf("handling worker-connected for ethAddress %s, nodeType %s, connection %s",
					payload.EthAddress, payload.NodeType, payload.Connection)
				// Update worker connection (including connection info)
				if err := r.SetWorkerConnection(payload.EthAddress, true, p.region, payload.NodeType, payload.Connection, endpointHash); err != nil {
					log.Printf("failed to handle worker-connected for event ID %d: %v", event.ID, err)
					continue
				}

			case "worker-disconnected":
				payload, ok := event.ParsedPayload.Payload.(models.RemoteWorker)
				if !ok {
					log.Printf("Invalid payload type for worker-disconnected event ID %d", event.ID)
					continue
				}
				log.Printf("Handling worker-disconnected for ethAddress %s, nodeType %s",
					payload.EthAddress, payload.NodeType)
				if err := r.SetWorkerConnection(payload.EthAddress, false, p.region, payload.NodeType, payload.Connection, endpointHash); err != nil {
					log.Printf("Failed to handle worker-disconnected for event ID %d: %v", event.ID, err)
					continue
				}

			case "orchestrator-reset":
				// For orchestrator-reset, only disconnect workers that belong to the same endpoint hash.
				log.Printf("Handling orchestrator-reset for event ID %d", event.ID)
				if err := r.DisconnectWorkers(endpointHash); err != nil {
					log.Printf("Failed to handle orchestrator-reset for event ID %d: %v", event.ID, err)
					continue
				}

			case "job-received":
				// Process the job-received event.
				payload, ok := event.ParsedPayload.Payload.(models.JobReceivedPayload)
				if !ok {
					log.Printf("Invalid payload type for job-received event ID %d", event.ID)
					continue
				}
				log.Printf("Handling job-received for EthAddress %s, nodeType %s, requestID %s",
					payload.EthAddress, payload.NodeType, payload.RequestID)
				// For AI node types, record the received job (by RequestID) for later matching.
				if payload.NodeType == "ai" {
					receivedJobs[payload.RequestID] = payload
				} else if payload.NodeType == "transcode" {
					log.Printf("job-received event for transcoding node type does not affect fees processing")
				} else {
					log.Printf("job-received event for unknown node type '%s'", payload.NodeType)
				}

			case "job-processed":
				// Process the job-processed event.
				payload, ok := event.ParsedPayload.Payload.(models.JobProcessedPayload)
				if !ok {
					log.Printf("Invalid payload type for job-processed event ID %d", event.ID)
					continue
				}

				// Calculate fees.
				feeAfterCommission := (payload.Fees * (100 - int64(p.poolCommission*100))) / 100

				// For AI nodes, require that a matching job-received event exists from the same endpoint.
				if payload.NodeType == "ai" {
					received, exists := receivedJobs[payload.RequestID]
					if !exists {
						log.Printf("No matching job-received event for job-processed event ID %d (requestID: %s); skipping fees update", event.ID, payload.RequestID)
						continue
					}
					// Use the EthAddress from the job-received payload.
					payload.EthAddress = received.EthAddress
					// remove the matching job-received entry once used.
					delete(receivedJobs, payload.RequestID)
				}

				log.Printf("Job-processed event - updating pending fees for EthAddress %s, nodeType %s, requestID %s, original fees %d, after commission %d",
					payload.EthAddress, payload.NodeType, payload.RequestID, payload.Fees, feeAfterCommission)

				if err := r.AddPendingFees(payload.EthAddress, payload.NodeType, p.region, feeAfterCommission, endpointHash); err != nil {
					log.Printf("Failed to add pending fees for job-processed event ID %d: %v", event.ID, err)
					continue
				}

			default:
				log.Printf("Unknown event type '%s' for event ID %d", event.ParsedPayload.EventType, event.ID)
				continue
			}
		}

		return nil // commit transaction
	})
}

// GetEndpoints returns a slice of endpoint URLs provided in the plugin configuration.
func (p *SqliteLoaderPlugin) GetEndpoints() ([]string, error) {
	endpoints := make([]string, 0, len(p.dataSources))
	for _, ds := range p.dataSources {
		endpoints = append(endpoints, ds.Endpoint)
	}
	return endpoints, nil
}

// Close closes the underlying GORM connection pool.
func (p *SqliteLoaderPlugin) Close() error {
	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
