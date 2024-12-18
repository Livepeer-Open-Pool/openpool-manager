package storage

import (
	"errors"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a record is not found in the database.
var ErrNotFound = errors.New("record not found")

// Storage encapsulates the database connection and repositories.
type Storage struct {
	//DB               *gorm.DB
	EventLogRepo         EventLogRepository
	RemoteWorkerRepo     RemoteWorkerRepository
	PoolPayoutRepository PoolPayoutRepository
}

// NewDBStorage creates a GORM connection and returns a *Storage struct
func NewDBStorage(dbFilePath string) (*Storage, error) {
	db, err := gorm.Open(sqlite.Open(dbFilePath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	//db.AutoMigrate(&models.RemoteWorker{}, &models.EventLog{}, &models.PoolPayout{})

	// Build and return a storage struct with your repositories
	return &Storage{
		RemoteWorkerRepo:     NewRemoteWorkerRepository(db),
		EventLogRepo:         NewEventLogRepository(db),
		PoolPayoutRepository: NewPoolPayoutRepository(db),
	}, nil
}
