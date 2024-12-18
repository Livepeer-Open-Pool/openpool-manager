package api

import (
	"github.com/Livepeer-Open-Pool/openpool-manager-api/config"
	"github.com/Livepeer-Open-Pool/openpool-manager-api/storage"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
)

// Handler struct to hold dependencies
type Handler struct {
	Storage *storage.Storage
	Config  *config.Config
}

// GetPoolTranscoders legacy pool endpoint
func (h *Handler) GetPoolTranscoders(c *gin.Context) {
	log.Println("[handlers::GetPoolTranscoders]")
	// Fetch all remote workers
	workers, err := h.Storage.RemoteWorkerRepo.FindAll()
	if err != nil {
		log.Printf("Failed to fetch remote workers: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch remote workers"})
		return
	}
	c.JSON(http.StatusOK, workers)
}

// GetPoolStatus  legacy pool endpoint
func (h *Handler) GetPoolStatus(c *gin.Context) {
	log.Println("[handlers::GetPoolStatus] ")
	cfg := h.Config
	originalJSON := []map[string]interface{}{
		{
			"Commission": cfg.PoolCommissionRate,
			"Version":    cfg.Version,
			//"BasePrice":    "0.00", //todo: is base price needed? the pool api will only have access to job data for price info
			"TotalPayouts": "0.00", //TODO: need to get total payouts
		},
	}

	workers, err := h.Storage.RemoteWorkerRepo.FindAll()

	if err != nil {
		c.JSON(http.StatusInternalServerError, originalJSON)
	}

	// Calculate total fees paid for all workers
	var totalFeesPaid int64
	for _, worker := range workers {
		totalFeesPaid += worker.PaidFees
	}

	// Update the JSON response with the total sum of PaidFees
	originalJSON[0]["TotalPayouts"] = totalFeesPaid

	c.JSON(http.StatusOK, originalJSON)
}
