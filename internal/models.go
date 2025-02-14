package internal

import "time"

type EventLog struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Type      string `json:"type"`
	Data      string `json:"data"`
	CreatedAt int64  `json:"created_at" gorm:"autoUpdateTime"`
}

// PoolPayout represents the pool payout record.
type PoolPayout struct {
	ID         int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	EthAddress string    `json:"ethAddress"`
	TxHash     string    `json:"txHash"`
	Fees       int64     `json:"fees"`
	CreatedAt  time.Time `json:"createdAt" gorm:"autoUpdateTime"`
}

func (pp PoolPayout) GetID() string {
	return pp.TxHash
}
func (pp PoolPayout) GetTimestamp() int64 {
	return pp.CreatedAt.Unix()
}
func (pp PoolPayout) GetAmount() int64 {
	return pp.Fees
}

// RemoteWorker represents a worker. The unique composite key is built from EthAddress, NodeType, and Region.
type RemoteWorker struct {
	EthAddress  string    `json:"ethAddress" gorm:"primaryKey;not null"`
	NodeType    string    `json:"nodeType" gorm:"primaryKey;not null"`
	Region      string    `json:"region" gorm:"primaryKey;not null"`
	IsConnected bool      `json:"is_connected"`
	PendingFees int64     `json:"pending_fees"`
	PaidFees    int64     `json:"paid_fees"`
	LastUpdated time.Time `json:"last_updated" gorm:"autoUpdateTime"`
	Connection  string    `json:"connection,omitempty"`
}

func (rw RemoteWorker) GetID() string {
	return rw.EthAddress
}
func (rw RemoteWorker) GetPaidFees() int64 {
	return rw.PaidFees
}
func (rw RemoteWorker) GetPendingFees() int64 {
	return rw.PendingFees
}
func (rw RemoteWorker) GetNodeType() string {
	return rw.NodeType
}
func (rw RemoteWorker) GetRegion() string {
	return rw.Region
}
