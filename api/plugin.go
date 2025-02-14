package main

import (
	"encoding/json"
	"fmt"
	pool "github.com/Livepeer-Open-Pool/openpool-plugin"
	"github.com/Livepeer-Open-Pool/openpool-plugin/config"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strconv"
)

type APIPlugin struct {
	store          pool.StorageInterface
	commissionRate float64
	region         string
	version        string
	portNumber     int
	logger         *log.Entry
}

var _ pool.PluginInterface = (*APIPlugin)(nil)

func (p *APIPlugin) Init(cfg config.Config, store pool.StorageInterface) {
	p.logger = log.WithFields(log.Fields{
		"component": "APIPlugin",
		"region":    cfg.Region,
		"version":   cfg.Version,
	})
	p.logger.Info("Initializing APIPlugin")

	p.store = store
	p.commissionRate = cfg.PoolCommissionRate
	p.region = cfg.Region
	p.version = cfg.Version
	p.portNumber = cfg.APIConfig.ServerPort
	//TODO: need a way to get Nodetypes dynamically from store
	//TODO: need a way to get total payout dynamically from store

	p.logger.WithFields(log.Fields{
		"commissionRate": p.commissionRate,
		"portNumber":     p.portNumber,
	}).Info("APIPlugin configuration loaded")
}

func (p *APIPlugin) Start() {
	logServer := p.logger.WithField("port", p.portNumber)

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		// Log request at debug level, including request method and path
		logServer.WithFields(log.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
		}).Debug("Handling /status request")
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
			// Log the error but proceed with a default (or partial) status
			logServer.WithFields(log.Fields{
				"errorPaid":    err,
				"errorPending": err2,
			}).Error("Failed to fetch fees from store")
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
		if err := json.NewEncoder(w).Encode(status); err != nil {
			logServer.WithError(err).Warn("Failed to encode /status response")
		}
	})

	http.HandleFunc("/workers", func(w http.ResponseWriter, r *http.Request) {
		logServer.WithFields(log.Fields{
			"method": r.Method,
			"path":   r.URL.Path,
		}).Debug("Handling /workers request")

		w.Header().Set("Content-Type", "application/json")

		//TODO: Basic check only makes sure they are filtered,NEED iNPUT CRITIERA To do a proper search...(job type, model, pipeline>??)
		// Check for the `filtered` query parameter
		query := r.URL.Query()
		filtered := query.Get("filtered") == "true"

		logServer.WithField("filtered", filtered).Debug("Query param parsed")

		var workers []models.Worker
		var err error

		if filtered {
			workers, err = p.store.GetFilteredWorkers()
		} else {
			workers, err = p.store.GetWorkers()
		}

		if err != nil {
			logServer.WithError(err).Error("Failed to retrieve workers")
			// Handle the error properly by returning a 500 response with a meaningful message
			http.Error(w, fmt.Sprintf(`{"error": "failed to retrieve workers: %v"}`, err), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(workers); err != nil {
			logServer.WithError(err).Warn("Failed to encode /workers response")
		}
	})

	// Start the server
	portStr := ":" + strconv.Itoa(p.portNumber)
	logServer.WithField("address", portStr).Info("Starting API server")
	if err := http.ListenAndServe(portStr, nil); err != nil {
		logServer.WithError(err).Fatal("Failed to start HTTP server")
	}
}

// Exported symbol for plugin loading
var PluginInstance APIPlugin
