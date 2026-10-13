package repository

import (
	"pemira-backend/internal/domain"

	"gorm.io/gorm"
)

type ChairRepository interface {
	FindAllActive() ([]domain.Chair, error)
	FindEligibleForMajor(major string) ([]domain.Chair, error)
	FindCandidateGroupByID(id string) (*domain.CandidateGroup, error)
}

type chairRepository struct {
	db *gorm.DB
}

func NewChairRepository(db *gorm.DB) ChairRepository {
	return &chairRepository{db: db}
}

func (r *chairRepository) FindAllActive() ([]domain.Chair, error) {
	var chairs []domain.Chair
	err := r.db.Preload("Candidates", func(db *gorm.DB) *gorm.DB {
		return db.Order("candidate_groups.ballot_number ASC")
	}).Where("is_active = ?", true).Order("id ASC").Find(&chairs).Error
	return chairs, err
}

func (r *chairRepository) FindEligibleForMajor(major string) ([]domain.Chair, error) {
	var chairs []domain.Chair
	err := r.db.Preload("Candidates", func(db *gorm.DB) *gorm.DB {
		return db.Order("candidate_groups.ballot_number ASC")
	}).Where("is_active = ? AND (scope = 'ALL' OR scope = ?)", true, major).
		Order("id ASC").Find(&chairs).Error
	return chairs, err
}

func (r *chairRepository) FindCandidateGroupByID(id string) (*domain.CandidateGroup, error) {
	var cg domain.CandidateGroup
	err := r.db.Where("id = ?", id).First(&cg).Error
	if err != nil {
		return nil, err
	}
	return &cg, nil
}
