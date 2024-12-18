package main

import (
	"encoding/json"
	"fmt"
	pool "github.com/Livepeer-Open-Pool/openpool-plugin"
	"github.com/Livepeer-Open-Pool/openpool-plugin/config"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)

type DataLoaderPlugin struct {
	store          pool.StorageInterface
	apiEndpoints   map[string]string
	nodeTypes      map[string]string
	lastCheck      map[string]time.Time
	receivedJobs   map[string]JobReceived
	poolCommission int64
	region         string
}

// JobReceived represents the payload for "job-received" events.
type JobReceived struct {
	EthAddress string `json:"ethAddress"`
	ModelID    string `json:"modelID"`
	NodeType   string `json:"nodeType"`
	Pipeline   string `json:"pipeline"`
	RequestID  string `json:"requestID"`
	TaskID     int    `json:"taskID"`
}

// JobProcessed represents the payload for "job-processed" events.
type JobProcessed struct {
	ComputeUnits        int    `json:"computeUnits"`
	Fees                int64  `json:"fees"`
	NodeType            string `json:"nodeType"`
	PricePerComputeUnit int    `json:"pricePerComputeUnit"`
	RequestID           string `json:"requestID"`
	ResponseTime        int64  `json:"responseTime"`
	EthAddress          string `json:"ethAddress,omitempty"`
	Pipeline            string `json:"pipeline,omitempty"`
	ModelID             string `json:"modelID,omitempty"`
}

// OrchestratorReset represents an empty payload for "orchestrator-reset" events.
type OrchestratorReset struct{}

// WorkerConnected represents the payload for "worker-connected" events.
type WorkerConnected struct {
	Connection string `json:"connection"`
	EthAddress string `json:"ethAddress"`
	NodeType   string `json:"nodeType"`
}

// WorkerDisconnected represents the payload for "worker-disconnected" events.
type WorkerDisconnected struct {
	EthAddress string `json:"ethAddress"`
	NodeType   string `json:"nodeType"`
}

// Ensure DataLoaderPlugin implements PluginInterface
var _ pool.PluginInterface = (*DataLoaderPlugin)(nil)

func (p *DataLoaderPlugin) Init(cfg config.Config, store pool.StorageInterface) {
	p.store = store
	p.apiEndpoints = make(map[string]string)
	p.nodeTypes = make(map[string]string)
	p.receivedJobs = make(map[string]JobReceived)
	p.poolCommission = int64(cfg.PoolCommissionRate) // Convert float64 to int64
	p.region = cfg.Region

	// Initialize API endpoints and node types from config
	for _, source := range cfg.DataLoaderPluginConfig.DataSources {
		p.apiEndpoints[source.NodeType] = source.Endpoint
		p.nodeTypes[source.NodeType] = source.NodeType
	}

	// Initialize lastCheck map
	p.lastCheck = make(map[string]time.Time)
	maxTimestamp, err := p.store.GetLastEventTimestamp()

	if err != nil {
		log.Printf("Error fetching max timestamp: %v\n", err)
		maxTimestamp = time.Time{}
	}
	log.Printf("GetLastEventTimestamp: %v\n", maxTimestamp)

	for nodeType := range p.apiEndpoints {
		p.lastCheck[nodeType] = maxTimestamp
	}
}

func (p *DataLoaderPlugin) Start() {
	fmt.Println("DataLoader started.")

	var wg sync.WaitGroup
	for nodeType, endpoint := range p.apiEndpoints {
		wg.Add(1)
		go func(nodeType, endpoint string) {
			defer wg.Done()
			fmt.Println("DataLoader about to process:", nodeType)

			p.fetchAndStoreEvents(nodeType, endpoint)
		}(nodeType, endpoint)
	}
	wg.Wait()
	time.Sleep(500 * time.Second)
}
func (p *DataLoaderPlugin) fetchAndStoreEvents(nodeType, endpoint string) {
	url := fmt.Sprintf("%s?lastCheckTime=%s", endpoint, p.lastCheck[nodeType].UTC().Format(time.RFC3339))

	fmt.Println("Fetching events from:", url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error fetching %s events: %v\n", nodeType, err)
		return
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response for %s: %v\n", nodeType, err)
		return
	}

	var rawEvents []struct {
		ID      int    `json:"ID"`
		Payload string `json:"Payload"`
		Version int    `json:"Version"`
		DT      string `json:"DT"`
	}

	if err := json.Unmarshal(body, &rawEvents); err != nil {
		fmt.Printf("Error parsing JSON for %s: %v\n", nodeType, err)
		return
	}

	for _, raw := range rawEvents {
		var parsedPayload struct {
			EventType string `json:"event_type"`
			Payload   json.RawMessage
		}

		if err := json.Unmarshal([]byte(raw.Payload), &parsedPayload); err != nil {
			log.Printf("Error parsing payload JSON for event ID %d: %v\n", raw.ID, err)
			continue
		}
		parsedTime, err := time.Parse(time.RFC3339, raw.DT)
		if err != nil {
			log.Printf("Error parsing timestamp for event ID %d: %v\n", raw.ID, err)
			continue
		}

		p.store.AddEvent(models.DefaultPoolEvent{
			Timestamp: parsedTime.UTC().Unix(),
			Data:      raw.Payload,
			Type:      parsedPayload.EventType,
		})

		switch parsedPayload.EventType {
		case "orchestrator-reset":
			var payload OrchestratorReset
			if err := json.Unmarshal(parsedPayload.Payload, &payload); err != nil {
				log.Printf("Invalid payload for orchestrator-reset event ID %d: %v\n", raw.ID, err)
				continue
			}

			if err := p.store.ResetWorkersOnlineStatus(p.region, nodeType); err != nil {
				log.Printf("Failed to reset Workers Online Status for orchestrator-reset event ID %d: %v\n", raw.ID, err)
				continue
			}
		case "worker-connected":
			var payload WorkerConnected
			if err := json.Unmarshal(parsedPayload.Payload, &payload); err != nil {
				log.Printf("Invalid payload for worker-connected event ID %d: %v\n", raw.ID, err)
				continue
			}
			if err := p.store.UpdateWorkerStatus(payload.EthAddress, true, p.region, nodeType); err != nil {
				log.Printf("Failed to update worker [%s] to online. worker-connected event ID %d: %v\n", payload.EthAddress, raw.ID, err)
				continue
			}

		case "worker-disconnected":
			var payload WorkerDisconnected
			if err := json.Unmarshal(parsedPayload.Payload, &payload); err != nil {
				log.Printf("Invalid payload for worker-disconnected event ID %d: %v\n", raw.ID, err)
				continue
			}
			if err := p.store.UpdateWorkerStatus(payload.EthAddress, false, p.region, nodeType); err != nil {
				log.Printf("Failed to update worker [%s] to offline. worker-disconnected event ID %d: %v\n", payload.EthAddress, raw.ID, err)
				continue
			}
		case "job-received":
			var payload JobReceived
			if err := json.Unmarshal(parsedPayload.Payload, &payload); err != nil {
				log.Printf("Invalid payload for job-received event ID %d: %v\n", raw.ID, err)
				continue
			}

			if payload.NodeType == "ai" {
				p.receivedJobs[payload.RequestID] = payload
			}

		case "job-processed":
			var payload JobProcessed
			if err := json.Unmarshal(parsedPayload.Payload, &payload); err != nil {
				log.Printf("Invalid payload for job-processed event ID %d: %v\n", raw.ID, err)
				continue
			}

			feeAfterCommission := payload.Fees * (1 - p.poolCommission/100)

			if payload.NodeType == "ai" {
				if received, exists := p.receivedJobs[payload.RequestID]; exists {
					payload.EthAddress = received.EthAddress
					delete(p.receivedJobs, payload.RequestID)
				}
			}
			if err := p.store.AddPendingFees(payload.EthAddress, feeAfterCommission, p.region, nodeType); err != nil {
				log.Printf("Failed to update worker [%s] pending fess. job-processed event ID %d: %v\n", payload.EthAddress, raw.ID, err)
				continue
			}
		default:
			log.Printf("Unknown event type '%s' for event ID %d", parsedPayload.EventType, raw.ID)
			continue
		}

		if parsedTime.After(p.lastCheck[nodeType]) {
			p.lastCheck[nodeType] = parsedTime
		}
	}

	fmt.Printf("Fetched %d new %s events\n", len(rawEvents), nodeType)
}

// Exported symbol for plugin loading
var PluginInstance DataLoaderPlugin
