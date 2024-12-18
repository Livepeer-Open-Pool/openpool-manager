package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"github.com/Livepeer-Open-Pool/openpool-plugin/pkg"
	"log"
	"net/http"
	"net/url"
	"time"
)

func hashEndpoint(endpoint string) string {
	h := sha256.New()
	h.Write([]byte(endpoint))
	return hex.EncodeToString(h.Sum(nil))
}

func StartDataLoader(
	dbStorage *storage.Storage,
	loadedPlugin pkg.Plugin,
) {
	endpointsStr, err := loadedPlugin.GetEndpoints()
	if err != nil {
		log.Fatalf("Error retrieving endpoints from plugin: %v", err)
		return
	}

	var endpoints []url.URL
	for _, epStr := range endpointsStr {
		parsed, err := url.Parse(epStr)
		if err != nil {
			log.Printf("Error parsing endpoint URL %s: %v", epStr, err)
			continue
		}
		endpoints = append(endpoints, *parsed)
	}

	//TODO: need to get fetch interval from the config
	interval := 60 * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Polling data from plugin-provided endpoints...")
		for _, ep := range endpoints {
			go func(endpoint url.URL) {
				endpointHash := hashEndpoint(endpoint.String())
				lastCheckTime := dbStorage.EventLogRepo.GetMaxTimestamp(endpointHash)
				endpointCopy := endpoint
				query := endpointCopy.Query()
				query.Set("lastCheckTime", lastCheckTime.Format(time.RFC3339))
				endpointCopy.RawQuery = query.Encode()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := fetchAndApplyEvents(ctx, &endpointCopy, loadedPlugin, lastCheckTime, dbStorage.EventLogRepo, endpointHash); err != nil {
					log.Printf("Polling error from %s: %v", endpointCopy.String(), err)
				} else {
					log.Printf("Polling successful from %s", endpointCopy.String())
				}
			}(ep)
		}
	}
}

func fetchAndApplyEvents(
	ctx context.Context,
	parsedURL *url.URL,
	loadedPlugin pkg.Plugin,
	lastCheckTime *time.Time,
	eventLogRepo storage.EventLogRepository,
	endpointHash string,
) error {
	query := parsedURL.Query()
	query.Set("lastCheckTime", lastCheckTime.Format(time.RFC3339))
	parsedURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("creating HTTP request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received non-OK HTTP status: %s", resp.Status)
	}

	var data []models.PoolEvent
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decoding response body: %w", err)
	}

	fmt.Printf("[data_loader::fetchAndApplyEvents] retrieved %d events\n", len(data))

	if err := loadedPlugin.Apply(data, endpointHash); err != nil {
		return fmt.Errorf("applying plugin events: %w", err)
	}

	for _, event := range data {
		if err := event.ParsePayload(); err != nil {
			log.Printf("Error parsing payload for event ID %d: %v", event.ID, err)
			continue
		}

		eventType := event.ParsedPayload.EventType
		payload := event.ParsedPayload.Payload

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Error marshalling payload for event ID %d: %v", event.ID, err)
			continue
		}
		eventData := string(payloadBytes)

		eventLog := &models.EventLog{
			Type:         eventType,
			Data:         eventData,
			CreatedAt:    event.DT,
			EndpointHash: endpointHash,
		}

		if err := eventLogRepo.Create(eventLog); err != nil {
			log.Printf("Error storing event in EventLog: %v", err)
			continue
		}
	}

	return nil
}
