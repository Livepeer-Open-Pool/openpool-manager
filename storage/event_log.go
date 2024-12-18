package storage

import (
	"fmt"
	"github.com/Livepeer-Open-Pool/openpool-plugin/models"
	"time"

	"gorm.io/gorm"
)

type EventLogRepository interface {
	Create(event *models.EventLog) error
	GetMaxTimestamp(endpointHash string) *time.Time
}

type eventLogRepo struct {
	db *gorm.DB
}

func NewEventLogRepository(db *gorm.DB) EventLogRepository {
	return &eventLogRepo{db: db}
}

func (r *eventLogRepo) Create(event *models.EventLog) error {
	return r.db.Create(event).Error
}

func (r *eventLogRepo) GetMaxTimestamp(endpointHash string) *time.Time {
	var createdAtStr string
	defaultTime := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)

	if err := r.db.Raw("SELECT max(created_at) FROM event_logs WHERE endpoint_hash = ?", endpointHash).Row().Scan(&createdAtStr); err != nil {
		fmt.Println("GetMaxTimestamp error scanning max(created_at):", err)
		return &defaultTime
	}

	parsedTime, err := time.Parse("2006-01-02 15:04:05-07:00", createdAtStr)
	if err != nil {
		fmt.Println("GetMaxTimestamp error parsing time:", err)
		return &defaultTime
	}

	fmt.Println("Parsed CreatedAt:", parsedTime)
	return &parsedTime
}
