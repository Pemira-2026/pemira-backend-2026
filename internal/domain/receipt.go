package domain

import (
	"time"
)

// ReceiptCommitment menyimpan komitmen hash tanda terima publik secara anonim
// ZERO-LINKABILITY: SAMA SEKALI TIDAK MEMILIKI VOTER_ID, CHAIR_ID, ATAU FK IDENTITAS APAPUN!
type ReceiptCommitment struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	TokenHash string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"tokenHash"` // SHA-256 dari receipt token publik
	CreatedAt time.Time `json:"createdAt"`
}

func (ReceiptCommitment) TableName() string {
	return "receipt_commitments"
}
