package domain

import (
	"time"
)

// Chair merepresentasikan kategori kursi pemilihan (contoh: BEM, MPM, HMJ TIK)
type Chair struct {
	ID           uint             `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string           `gorm:"type:varchar(100);not null" json:"name"`
	Scope        string           `gorm:"type:varchar(100);not null;default:'ALL'" json:"scope"` // 'ALL' atau nama jurusan spesifik
	MaxSelection int              `gorm:"not null;default:1" json:"maxSelection"`
	IsActive     bool             `gorm:"not null;default:true" json:"isActive"`
	Candidates   []CandidateGroup `gorm:"foreignKey:ChairID" json:"candidates,omitempty"`
	CreatedAt    time.Time        `json:"createdAt"`
}

func (Chair) TableName() string {
	return "chairs"
}
