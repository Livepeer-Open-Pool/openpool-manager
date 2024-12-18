package storage

import (
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"gorm.io/gorm"
)

// PoolPayoutRepository defines methods to interact with pool_payout records.
type PoolPayoutRepository interface {
	Create(payout *models.PoolPayout) error
	FindAll() ([]models.PoolPayout, error)
}

type poolPayoutRepo struct {
	db *gorm.DB
}

// NewPoolPayoutRepository returns a new instance of PoolPayoutRepository.
func NewPoolPayoutRepository(db *gorm.DB) PoolPayoutRepository {
	return &poolPayoutRepo{db: db}
}

// Create inserts a new worker record.
func (r *poolPayoutRepo) Create(worker *models.PoolPayout) error {
	return r.db.Create(worker).Error
}

// FindAll fetches all workers in the DB.
func (r *poolPayoutRepo) FindAll() ([]models.PoolPayout, error) {
	var workers []models.PoolPayout
	if err := r.db.Find(&workers).Error; err != nil {
		return nil, err
	}
	return workers, nil
}
