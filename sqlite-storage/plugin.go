package main

import (
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-manager/internal"
	pool "github.com/Livepeer-Open-Pool/openpool-plugin"
	"github.com/Livepeer-Open-Pool/openpool-plugin/config"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"log"
	"sync"
	"time"
)

// SqliteStoragePlugin provides shared storage.
type SqliteStoragePlugin struct {
	mu     sync.Mutex
	db     *gorm.DB
	config *config.Config
}

// Ensure StoragePlugin implements pool.StorageInterface ✅
var _ pool.StorageInterface = &SqliteStoragePlugin{}

// NewSqliteStoragePlugin returns a new NewSqliteStoragePlugin instance.
func NewSqliteStoragePlugin() pool.StorageInterface {
	return &SqliteStoragePlugin{}
}

func (s *SqliteStoragePlugin) Init(config *config.Config) {
	fmt.Println("Storage plugin initialized.")
	gormDb, err := gorm.Open(sqlite.Open(config.DataStorageFilePath), &gorm.Config{})
	if err != nil {
		fmt.Println("Failed to connect to sqlite storage", err)
		panic("failed to connect to storage")
	}

	// AutoMigrate or any other DB initialization here.
	err = gormDb.AutoMigrate(&internal.RemoteWorker{}, &internal.EventLog{}, &internal.PoolPayout{})
	if err != nil {
		fmt.Println("Failed to connect to sqlite storage", err)
		//return nil, err
	}
	s.db = gormDb
	s.config = config
}

// AddEvent stores an event.
func (s *SqliteStoragePlugin) AddEvent(event models.PoolEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Create(&internal.EventLog{Type: event.GetType(), Data: event.GetData(), CreatedAt: event.GetTimestamp()}).Error
}
func (s *SqliteStoragePlugin) GetLastEventTimestamp() (time.Time, error) {
	var maxUnixTime int64

	err := s.db.Model(&internal.EventLog{}).Select("MAX(created_at)").Where("created_at is not null").Scan(&maxUnixTime).Error
	if err != nil {
		return time.Time{}, err
	}

	// If no timestamp exists, return zero time
	if maxUnixTime == 0 {
		return time.Time{}, nil
	}

	// Convert Unix timestamp to time.Time
	return time.Unix(maxUnixTime, 0), nil
}
func (s *SqliteStoragePlugin) GetFilteredWorkers() ([]models.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	//TODO: Basic check only makes sure they are online, need to add performance based checks here.... NEED iNPUT CRITIERA TOO
	var remoteWorkers []*internal.RemoteWorker
	if err := s.db.Find(&remoteWorkers).Where("is_connected = ?", true).Error; err != nil {
		return nil, err
	}

	workers := make([]models.Worker, len(remoteWorkers))
	for i, rw := range remoteWorkers {
		workers[i] = rw
	}
	return workers, nil
}

func (s *SqliteStoragePlugin) GetWorkers() ([]models.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var remoteWorkers []*internal.RemoteWorker
	if err := s.db.Find(&remoteWorkers).Error; err != nil {
		return nil, err
	}

	workers := make([]models.Worker, len(remoteWorkers))
	for i, rw := range remoteWorkers {
		workers[i] = rw
	}
	return workers, nil
}

func (s *SqliteStoragePlugin) UpdateWorkerStatus(ethAddress string, connected bool, region string, nodeType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&internal.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ?", ethAddress, region, nodeType).
		Updates(map[string]interface{}{
			"is_connected": connected,
		})
	if result.Error != nil {
		return result.Error
	}
	// If no matching record was found, create a new one with the connection info
	if result.RowsAffected == 0 {
		worker := internal.RemoteWorker{
			EthAddress:  ethAddress,
			Region:      region,
			NodeType:    nodeType,
			IsConnected: connected,
		}
		return s.db.Create(&worker).Error
	}
	return nil
}

func (s *SqliteStoragePlugin) ResetWorkersOnlineStatus(region string, nodeType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&internal.RemoteWorker{}).
		Where("region = ? AND node_type = ?", region, nodeType).
		Update("is_connected", false)
	return result.Error
}

func (s *SqliteStoragePlugin) AddPendingFees(ethAddress string, amount int64, region string, nodeType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&internal.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ?", ethAddress, region, nodeType).
		Update("pending_fees", gorm.Expr("pending_fees + ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// If no matching record exists, you might choose to create one.
		worker := internal.RemoteWorker{
			EthAddress:  ethAddress,
			Region:      region,
			NodeType:    nodeType,
			IsConnected: false,
			PendingFees: amount,
		}
		return s.db.Create(&worker).Error
	}
	return nil
}

// RecordPayout stores a payout.
func (s *SqliteStoragePlugin) AddPaidFees(ethAddress string, amount int64, txHash string, region string, nodeType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&internal.RemoteWorker{}).
		Where("eth_address = ? AND region = ? AND node_type = ?", ethAddress, region, nodeType).
		Update("paid_fees", gorm.Expr("paid_fees + ?", amount)).
		Update("pending_fees", gorm.Expr("pending_fees - ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {

		return fmt.Errorf("failed to find remote worker [%s] to update paid fees", ethAddress)
	}
	// Create the pool payout record.
	payout := &internal.PoolPayout{
		EthAddress: ethAddress,
		TxHash:     txHash,
		Fees:       amount,
	}
	if err := s.db.Create(payout); err != nil {
		log.Printf("Payout of %v wei sent and recorded for worker %s", amount, ethAddress)
	} else {
		log.Printf("Failed to create pool payout record for worker %s: %v", ethAddress, err)
	}

	return nil
}

func (s *SqliteStoragePlugin) GetPendingFees() (float64, error) {
	var totalPendingFees int64
	err := s.db.Model(&internal.RemoteWorker{}).Select("SUM(pending_fees)").Scan(&totalPendingFees).Error
	if err != nil {
		return 0, err
	}

	// Convert wei to ether
	ether := float64(totalPendingFees) / 1e18
	return ether, nil
}

func (s *SqliteStoragePlugin) GetPaidFees() (float64, error) {
	var totalPaidFees int64
	err := s.db.Model(&internal.RemoteWorker{}).Select("SUM(paid_fees)").Scan(&totalPaidFees).Error
	if err != nil {
		return 0, err
	}

	// Convert wei to ether
	ether := float64(totalPaidFees) / 1e18
	return ether, nil
}

// Exported symbol for plugin loading
var PluginInstance SqliteStoragePlugin
