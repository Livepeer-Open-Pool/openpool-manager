package main

import (
	"encoding/json"
	"fmt"
	pool "github.com/Livepeer-Open-Pool/openpool-plugin"
	"github.com/Livepeer-Open-Pool/openpool-plugin/config"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"net/http"
)

type APIPlugin struct {
	store          pool.StorageInterface
	commissionRate float64
	region         string
	version        string
	portNumber     int
}

var _ pool.PluginInterface = (*APIPlugin)(nil)

func (p *APIPlugin) Init(cfg config.Config, store pool.StorageInterface) {
	p.store = store
	p.commissionRate = cfg.PoolCommissionRate
	p.region = cfg.Region
	p.version = cfg.Version
	p.portNumber = cfg.APIConfig.ServerPort
	//TODO: need a way to get Nodetypes dynamically from store
	//TODO: need a way to get total payout dynamically from store
}

func (p *APIPlugin) Start() {
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {

		status := []map[string]interface{}{
			{
				"Commission":   p.commissionRate,
				"TotalPayouts": 0.00,
				"TotalPending": 0.00,
				"Version":      p.version,
				"Region":       p.region,
				"NodeTypes":    []string{"AI", "Transcoding"},
			},
		}
		totalPaid, err := p.store.GetPaidFees()
		totalPending, err2 := p.store.GetPendingFees()

		if err != nil || err2 != nil {
			//TODO: handle error!!
		} else {
			status = []map[string]interface{}{
				{
					"Commission":   p.commissionRate,
					"TotalPayouts": totalPaid,
					"TotalPending": totalPending,
					"Version":      p.version,
					"Region":       p.region,
					"NodeTypes":    []string{"AI", "Transcoding"},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	http.HandleFunc("/workers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		//TODO: Basic check only makes sure they are filtered,NEED iNPUT CRITIERA To do a proper search...(job type, model, pipeline>??)
		// Check for the `filtered` query parameter
		query := r.URL.Query()
		filtered := query.Get("filtered") == "true"

		var workers []models.Worker
		var err error

		if filtered {
			workers, err = p.store.GetFilteredWorkers()
		} else {
			workers, err = p.store.GetWorkers()
		}

		if err != nil {
			// Handle the error properly by returning a 500 response with a meaningful message
			http.Error(w, fmt.Sprintf(`{"error": "failed to retrieve workers: %v"}`, err), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(workers)
	})

	fmt.Println("API server running on :8080")
	portStr := fmt.Sprintf(":%d", p.portNumber)
	http.ListenAndServe(portStr, nil)
}

// Exported symbol for plugin loading
var PluginInstance APIPlugin
