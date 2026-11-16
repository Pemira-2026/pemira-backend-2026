package domain

import (
	"time"
)

// Voter merepresentasikan identitas mahasiswa pada Daftar Pemilih Tetap (DPT)
type Voter struct {
	ID          string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	NIM         string     `gorm:"type:varchar(25);uniqueIndex;not null" json:"nim"`
	FullName    string     `gorm:"type:varchar(150);not null" json:"fullName"`
	Email       string     `gorm:"type:varchar(150);uniqueIndex;not null" json:"email"`
	Faculty     string     `gorm:"type:varchar(100);not null" json:"faculty"`
	Major       string     `gorm:"type:varchar(100);index;not null" json:"major"`
	Generation  int        `gorm:"not null" json:"generation"`
	Role        string     `gorm:"type:varchar(20);not null;default:'voter'" json:"role"` // 'voter', 'admin', 'kpu', 'bawaslu'
	HasVoted    bool       `gorm:"not null;default:false;index" json:"hasVoted"`
	VotedAt     *time.Time `json:"votedAt,omitempty"`
	ReceiptHash *string    `gorm:"type:varchar(64)" json:"receiptHash,omitempty"` // HMAC-SHA256 hash bukti coblos
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

func (Voter) TableName() string {
	return "voters"
}

// DTO untuk profile pemilih yang sedang login
type VoterProfileResponse struct {
	ID             string         `json:"id"`
	NIM            string         `json:"nim"`
	FullName       string         `json:"fullName"`
	Email          string         `json:"email"`
	Faculty        string         `json:"faculty"`
	Major          string         `json:"major"`
	Generation     int            `json:"generation"`
	Role           string         `json:"role"`
	HasVoted       bool           `json:"hasVoted"`
	EligibleChairs []ChairSummary `json:"eligibleChairs"`
}

type ChairSummary struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
