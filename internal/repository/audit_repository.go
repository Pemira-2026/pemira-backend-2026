package repository

import (
	"pemira-backend/internal/domain"

	"gorm.io/gorm"
)

type AuditRepository interface {
	LogAction(log *domain.AuditLog) error
	GetRecentLogs(limit int) ([]domain.AuditLog, error)
	GetElectionConfig() (*domain.ElectionConfig, error)
	UpdateElectionConfig(cfg *domain.ElectionConfig) error
}

type auditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) AuditRepository {
	return &auditRepository{db: db}
}

func (r *auditRepository) LogAction(log *domain.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditRepository) GetRecentLogs(limit int) ([]domain.AuditLog, error) {
	var logs []domain.AuditLog
	err := r.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

func (r *auditRepository) GetElectionConfig() (*domain.ElectionConfig, error) {
	var cfg domain.ElectionConfig
	err := r.db.First(&cfg).Error
	return &cfg, err
}

func (r *auditRepository) UpdateElectionConfig(cfg *domain.ElectionConfig) error {
	return r.db.Save(cfg).Error
}
