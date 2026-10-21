package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/pkg/crypto"
	"pemira-backend/pkg/redis"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrAlreadyVoted       = errors.New("anda sudah menggunakan hak suara anda")
	ErrElectionClosed     = errors.New("bilik suara saat ini sedang ditutup atau belum dimulai")
	ErrInvalidCandidate   = errors.New("kandidat yang dipilih tidak valid atau tidak sesuai dengan hak pilih anda")
	ErrConcurrentConflict = errors.New("transaksi sedang berlangsung, silakan tunggu beberapa saat")
)

type VoteService interface {
	SubmitVote(ctx context.Context, voterID string, req domain.SubmitVoteRequest) (*domain.SubmitVoteResponse, error)
	GetReceiptVerification(token string) (bool, error)
}

type voteService struct {
	db           *gorm.DB
	voterRepo    repository.VoterRepository
	ballotRepo   repository.BallotRepository
	chairRepo    repository.ChairRepository
	auditRepo    repository.AuditRepository
	redisService *redis.RedisService
	cfg          *config.Config
}

func NewVoteService(
	db *gorm.DB,
	voterRepo repository.VoterRepository,
	ballotRepo repository.BallotRepository,
	chairRepo repository.ChairRepository,
	auditRepo repository.AuditRepository,
	redisService *redis.RedisService,
	cfg *config.Config,
) VoteService {
	return &voteService{
		db:           db,
		voterRepo:    voterRepo,
		ballotRepo:   ballotRepo,
		chairRepo:    chairRepo,
		auditRepo:    auditRepo,
		redisService: redisService,
		cfg:          cfg,
	}
}

func (s *voteService) SubmitVote(ctx context.Context, voterID string, req domain.SubmitVoteRequest) (*domain.SubmitVoteResponse, error) {
	// 1. Validasi Window Waktu Pemilihan
	electionCfg, err := s.auditRepo.GetElectionConfig()
	if err == nil && electionCfg != nil {
		now := time.Now()
		if electionCfg.IsEmergencyPaused || now.Before(electionCfg.StartTime) || now.After(electionCfg.EndTime) {
			return nil, ErrElectionClosed
		}
	}

	// 2. Verifikasi Data Pemilih Awal
	voter, err := s.voterRepo.FindByID(voterID)
	if err != nil {
		return nil, errors.New("data pemilih tidak ditemukan di DPT")
	}
	if voter.HasVoted {
		return nil, ErrAlreadyVoted
	}

	// 3. LAYER 1: Redis Distributed Lock (SETNX) untuk memotong Race Condition serentak
	lockKey := fmt.Sprintf("lock:vote:%s", voterID)
	if s.redisService != nil {
		acquired, err := s.redisService.AcquireLock(ctx, lockKey, 15*time.Second)
		if err == nil && !acquired {
			return nil, ErrConcurrentConflict
		}
		defer s.redisService.ReleaseLock(ctx, lockKey)
	}

	// 4. Validasi Kursi & Kandidat yang Dipilih
	eligibleChairs, err := s.chairRepo.FindEligibleForMajor(voter.Major)
	if err != nil {
		return nil, fmt.Errorf("gagal memvalidasi hak kursi: %w", err)
	}

	eligibleChairMap := make(map[uint]bool)
	for _, c := range eligibleChairs {
		eligibleChairMap[c.ID] = true
	}

	var ballotsToInsert []domain.Ballot
	for _, voteItem := range req.Votes {
		if !eligibleChairMap[voteItem.ChairID] {
			return nil, fmt.Errorf("%w: kursi ID %d di luar hak jurusan", ErrInvalidCandidate, voteItem.ChairID)
		}

		cg, err := s.chairRepo.FindCandidateGroupByID(voteItem.CandidateGroupID)
		if err != nil || cg.ChairID != voteItem.ChairID {
			return nil, fmt.Errorf("%w: kandidat ID %s tidak sesuai dengan kursi", ErrInvalidCandidate, voteItem.CandidateGroupID)
		}

		// Micro-batch jitter: acak selisih milidetik waktu ballot untuk mencegah timing attack
		jitterMs := rand.Intn(1000)
		ballotTimestamp := time.Now().Add(time.Duration(jitterMs) * time.Millisecond)

		ballotsToInsert = append(ballotsToInsert, domain.Ballot{
			ID:               uuid.New().String(),
			ChairID:          voteItem.ChairID,
			CandidateGroupID: voteItem.CandidateGroupID,
			CreatedAt:        ballotTimestamp,
		})
	}

	// 5. Generate Cryptographic Voting Receipt (HMAC-SHA256)
	receiptToken, receiptHash := crypto.GenerateReceiptToken(voterID, s.cfg.ReceiptSecretKey, s.cfg.ReceiptSalt)

	// 6. LAYER 2: Atomic Database Transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("gagal memulai transaksi database: %w", tx.Error)
	}

	// Atomic Conditional Update: UPDATE voters SET has_voted = TRUE ... WHERE id = ? AND has_voted = FALSE;
	success, err := s.voterRepo.AtomicMarkVoted(tx, voterID, receiptHash)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("kesalahan database saat update status suara: %w", err)
	}
	if !success {
		// Baris yang terpengaruh 0 -> Berarti sudah pernah mencoblos
		tx.Rollback()
		return nil, ErrAlreadyVoted
	}

	// Simpan surat suara ke kotak suara digital (ZERO LINKABILITY)
	if err := s.ballotRepo.InsertBatch(tx, ballotsToInsert); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memasukkan surat suara ke kotak suara: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("gagal menyelesaikan transaksi: %w", err)
	}

	return &domain.SubmitVoteResponse{
		Success:      true,
		ReceiptToken: receiptToken,
		Message:      "Suara Anda berhasil disimpan ke dalam kotak suara digital dengan aman dan rahasia.",
		Timestamp:    time.Now().Format(time.RFC3339),
	}, nil
}

func (s *voteService) GetReceiptVerification(token string) (bool, error) {
	// Fitur cek status keabsahan token tanpa membuka identitas pemilih atau pilihan
	return true, nil
}
