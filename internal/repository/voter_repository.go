package repository

import (
	"pemira-backend/internal/domain"
	"time"

	"gorm.io/gorm"
)

type VoterRepository interface {
	FindByEmail(email string) (*domain.Voter, error)
	FindByID(id string) (*domain.Voter, error)
	Create(voter *domain.Voter) error
	BatchCreate(voters []domain.Voter) error
	AtomicMarkVoted(tx *gorm.DB, voterID string, receiptHash string) (bool, error)
	GetTurnoutStats() (total int64, voted int64, err error)
}

type voterRepository struct {
	db *gorm.DB
}

func NewVoterRepository(db *gorm.DB) VoterRepository {
	return &voterRepository{db: db}
}

func (r *voterRepository) FindByEmail(email string) (*domain.Voter, error) {
	var voter domain.Voter
	err := r.db.Where("email = ?", email).First(&voter).Error
	if err != nil {
		return nil, err
	}
	return &voter, nil
}

func (r *voterRepository) FindByID(id string) (*domain.Voter, error) {
	var voter domain.Voter
	err := r.db.Where("id = ?", id).First(&voter).Error
	if err != nil {
		return nil, err
	}
	return &voter, nil
}

func (r *voterRepository) Create(voter *domain.Voter) error {
	return r.db.Create(voter).Error
}

func (r *voterRepository) BatchCreate(voters []domain.Voter) error {
	return r.db.CreateInBatches(voters, 500).Error
}

// AtomicMarkVoted mengeksekusi conditional update langsung di SQL:
// UPDATE voters SET has_voted = TRUE, voted_at = NOW(), receipt_hash = ? WHERE id = ? AND has_voted = FALSE;
// Mengembalikan true jika baris terupdate (1), false jika sudah pernah memilih (0).
func (r *voterRepository) AtomicMarkVoted(tx *gorm.DB, voterID string, receiptHash string) (bool, error) {
	now := time.Now()
	res := tx.Model(&domain.Voter{}).
		Where("id = ? AND has_voted = ?", voterID, false).
		Updates(map[string]interface{}{
			"has_voted":    true,
			"voted_at":     now,
			"receipt_hash": receiptHash,
			"updated_at":   now,
		})

	if res.Error != nil {
		return false, res.Error
	}

	return res.RowsAffected > 0, nil
}

func (r *voterRepository) GetTurnoutStats() (total int64, voted int64, err error) {
	if err = r.db.Model(&domain.Voter{}).Count(&total).Error; err != nil {
		return 0, 0, err
	}
	if err = r.db.Model(&domain.Voter{}).Where("has_voted = ?", true).Count(&voted).Error; err != nil {
		return 0, 0, err
	}
	return total, voted, nil
}
