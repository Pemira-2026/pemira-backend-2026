package domain

import (
	"time"
)

// CandidateGroup merepresentasikan pasangan calon (Ketua & Wakil)
type CandidateGroup struct {
	ID           string    `gorm:"type:varchar(36);primaryKey" json:"id"` // UUID
	ChairID      uint      `gorm:"not null;index" json:"chairId"`
	BallotNumber int       `gorm:"not null" json:"ballotNumber"` // Nomor urut paslon (1, 2, 3...)
	LeaderName   string    `gorm:"type:varchar(150);not null" json:"leaderName"`
	ViceName     string    `gorm:"type:varchar(150)" json:"viceName,omitempty"`
	Vision       string    `gorm:"type:text" json:"vision,omitempty"`
	Mission      string    `gorm:"type:text" json:"mission,omitempty"`
	WorkPrograms string    `gorm:"type:text" json:"workPrograms,omitempty"`
	ImageURL     string    `gorm:"type:varchar(500)" json:"imageUrl,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (CandidateGroup) TableName() string {
	return "candidate_groups"
}
