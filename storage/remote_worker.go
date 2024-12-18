package storage

import (
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"gorm.io/gorm"
)

// RemoteWorkerRepository defines methods for interacting with remote workers.
type RemoteWorkerRepository interface {
	FindAll() ([]models.RemoteWorker, error)

	// SetWorkerConnection sets or creates a worker’s connection status, but only for records
	// associated with the provided endpointHash.
	SetWorkerConnection(ethAddress string, connected bool, region string, nodeType string, connection string, endpointHash string) error

	// DisconnectWorkers marks all workers associated with a specific endpointHash as disconnected.
	DisconnectWorkers(endpointHash string) error

	// AddPendingFees increments the pending fees for the worker matching the provided criteria and endpointHash.
	AddPendingFees(ethAddress string, nodeType string, region string, fee int64, endpointHash string) error

	// AddPaidFees increments the paid fees (decrement the pending fees) for the worker matching the provided criteria and endpointHash.
	AddPaidFees(ethAddress string, nodeType string, region string, fee int64, endpointHash string) error
}

type remoteWorkerRepo struct {
	db *gorm.DB
}

// NewRemoteWorkerRepository returns a new instance of RemoteWorkerRepository.
func NewRemoteWorkerRepository(db *gorm.DB) RemoteWorkerRepository {
	return &remoteWorkerRepo{db: db}
}

// FindAll fetches all workers in the DB.
func (r *remoteWorkerRepo) FindAll() ([]models.RemoteWorker, error) {
	var workers []models.RemoteWorker
	if err := r.db.Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}

func (r *remoteWorkerRepo) SetWorkerConnection(ethAddress string, connected bool, region string, nodeType string, connection string, endpointHash string) error {
	// Update both fields in a single call
	result := r.db.Model(&models.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ? AND endpoint_hash = ?", ethAddress, region, nodeType, endpointHash).
		Updates(map[string]interface{}{
			"is_connected": connected,
			"connection":   connection,
		})
	if result.Error != nil {
		return result.Error
	}
	// If no matching record was found, create a new one with the connection info
	if result.RowsAffected == 0 {
		worker := models.RemoteWorker{
			EthAddress:   ethAddress,
			Region:       region,
			NodeType:     nodeType,
			IsConnected:  connected,
			Connection:   connection,
			EndpointHash: endpointHash,
		}
		return r.db.Create(&worker).Error
	}
	return nil
}

// DisconnectWorkers updates all workers associated with the given endpointHash, marking them as disconnected.
func (r *remoteWorkerRepo) DisconnectWorkers(endpointHash string) error {
	result := r.db.Model(&models.RemoteWorker{}).
		Where("endpoint_hash = ?", endpointHash).
		Update("is_connected", false).
		Update("connection", "")
	return result.Error
}

// AddPendingFees increments the pending fees for a remote worker matching the provided criteria and endpointHash.
func (r *remoteWorkerRepo) AddPendingFees(ethAddress string, nodeType string, region string, fee int64, endpointHash string) error {
	result := r.db.Model(&models.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ? AND endpoint_hash = ?", ethAddress, region, nodeType, endpointHash).
		Update("pending_fees", gorm.Expr("pending_fees + ?", fee))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// If no matching record exists, you might choose to create one.
		worker := models.RemoteWorker{
			EthAddress:   ethAddress,
			Region:       region,
			NodeType:     nodeType,
			IsConnected:  false, // Default to disconnected.
			PendingFees:  fee,
			EndpointHash: endpointHash,
		}
		return r.db.Create(&worker).Error
	}
	return nil
}

// AddPaidFees increments the paid fees (decrement the pending fees) for the worker matching the provided criteria and endpointHash.
func (r *remoteWorkerRepo) AddPaidFees(ethAddress string, nodeType string, region string, fee int64, endpointHash string) error {
	result := r.db.Model(&models.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ? AND endpoint_hash = ?", ethAddress, region, nodeType, endpointHash).
		Update("paid_fees", gorm.Expr("paid_fees + ?", fee)).
		Update("pending_fees", gorm.Expr("pending_fees - ?", fee))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {

		return fmt.Errorf("failed to find remote worker [%s] to update paid fees", ethAddress)
	}
	return nil
}
