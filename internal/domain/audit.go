package domain

import (
	"time"
)

// AuditLog merepresentasikan catatan audit jejak aktivitas sistem dan panitia
type AuditLog struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ActorID   string    `gorm:"type:varchar(36);not null;index" json:"actorId"`
	ActorRole string    `gorm:"type:varchar(50);not null" json:"actorRole"`
	Action    string    `gorm:"type:varchar(100);not null;index" json:"action"`
	IPAddress string    `gorm:"type:varchar(45);not null" json:"ipAddress"`
	UserAgent string    `gorm:"type:text" json:"userAgent"`
	Payload   string    `gorm:"type:jsonb" json:"payload,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}

// ElectionConfig konfigurasi window pemilihan (jam mulai dan jam selesai)
type ElectionConfig struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Title             string    `gorm:"type:varchar(150);not null" json:"title"`
	StartTime         time.Time `gorm:"not null" json:"startTime"`
	EndTime           time.Time `gorm:"not null" json:"endTime"`
	IsEmergencyPaused bool      `gorm:"not null;default:false" json:"isEmergencyPaused"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (ElectionConfig) TableName() string {
	return "election_configs"
}
