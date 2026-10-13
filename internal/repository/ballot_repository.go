package repository

import (
	"pemira-backend/internal/domain"

	"gorm.io/gorm"
)

type BallotRepository interface {
	InsertBatch(tx *gorm.DB, ballots []domain.Ballot) error
	GetTallyResults() ([]domain.TallyResultItem, error)
}

type ballotRepository struct {
	db *gorm.DB
}

func NewBallotRepository(db *gorm.DB) BallotRepository {
	return &ballotRepository{db: db}
}

// InsertBatch menyimpan surat suara ke dalam kotak suara digital (ballots)
// SAMA SEKALI TIDAK MENYIMPAN VOTER_ID ATAU FK IDENTITAS!
func (r *ballotRepository) InsertBatch(tx *gorm.DB, ballots []domain.Ballot) error {
	return tx.Create(&ballots).Error
}

// GetTallyResults menghitung perolehan suara per kandidat (Hanya dibuka setelah jam pemilihan ditutup)
func (r *ballotRepository) GetTallyResults() ([]domain.TallyResultItem, error) {
	var results []domain.TallyResultItem

	query := `
		SELECT 
			c.id AS chair_id,
			c.name AS chair_name,
			cg.id AS candidate_group_id,
			cg.ballot_number,
			cg.leader_name,
			COALESCE(cg.vice_name, '') AS vice_name,
			COUNT(b.id) AS total_votes
		FROM chairs c
		JOIN candidate_groups cg ON cg.chair_id = c.id
		LEFT JOIN ballots b ON b.candidate_group_id = cg.id
		GROUP BY c.id, c.name, cg.id, cg.ballot_number, cg.leader_name, cg.vice_name
		ORDER BY c.id ASC, cg.ballot_number ASC
	`

	if err := r.db.Raw(query).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}
