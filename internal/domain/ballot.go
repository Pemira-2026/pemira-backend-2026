package domain

import (
	"time"
)

// Ballot merepresentasikan surat suara digital di dalam kotak suara anonim.
// ZERO LINKABILITY: SAMA SEKALI TIDAK MEMILIKI VOTER_ID ATAU FK IDENTITAS APAPUN!
type Ballot struct {
	ID               string    `gorm:"type:varchar(36);primaryKey" json:"id"` // UUID v4 acak
	ChairID          uint      `gorm:"not null;index" json:"chairId"`
	CandidateGroupID string    `gorm:"type:varchar(36);not null;index" json:"candidateGroupId"`
	CreatedAt        time.Time `json:"createdAt"`
}

func (Ballot) TableName() string {
	return "ballots"
}

// SingleVoteItem merepresentasikan satu pilihan per kursi pemilihan
type SingleVoteItem struct {
	ChairID          uint   `json:"chairId" binding:"required"`
	CandidateGroupID string `json:"candidateGroupId" binding:"required"`
}

// SubmitVoteRequest merepresentasikan payload yang dikirim dari bilik suara
type SubmitVoteRequest struct {
	TurnstileToken string           `json:"turnstileToken"` // Cloudflare Turnstile token
	Votes          []SingleVoteItem `json:"votes" binding:"required,min=1"`
}

// SubmitVoteResponse merepresentasikan response balik setelah suara tersimpan atomik
type SubmitVoteResponse struct {
	Success      bool   `json:"success"`
	ReceiptToken string `json:"receiptToken"` // Bukti digital HMAC-SHA256
	Message      string `json:"message"`
	Timestamp    string `json:"timestamp"`
}

// TallyResultItem hasil rekapitulasi per paslon (Hanya bisa dibuka saat Quorum terpenuhi)
type TallyResultItem struct {
	ChairID          uint   `json:"chairId"`
	ChairName        string `json:"chairName"`
	CandidateGroupID string `json:"candidateGroupId"`
	BallotNumber     int    `json:"ballotNumber"`
	LeaderName       string `json:"leaderName"`
	ViceName         string `json:"viceName"`
	TotalVotes       int64  `json:"totalVotes"`
	Percentage       float64 `json:"percentage"`
}
