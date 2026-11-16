package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/pkg/crypto"
	"pemira-backend/pkg/redis"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrAlreadyVoted       = errors.New("anda sudah menggunakan hak suara anda")
	ErrElectionClosed     = errors.New("bilik suara saat ini sedang ditutup atau belum dimulai")
	ErrInvalidCandidate   = errors.New("kandidat yang dipilih tidak valid atau tidak sesuai dengan hak pilih anda")
	ErrConcurrentConflict = errors.New("transaksi sedang berlangsung, silakan tunggu beberapa saat")
	ErrTurnstileInvalid   = errors.New("verifikasi human tidak valid")
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
	// 1. Validasi Window Waktu Pemilihan (Dengan Cache Redis 30 detik untuk High Concurrency)
	electionCfg, err := s.getElectionConfigCached(ctx)
	if err != nil || electionCfg == nil {
		return nil, fmt.Errorf("gagal membaca konfigurasi pemilu: %w", err)
	}
	now := time.Now()
	if electionCfg.IsEmergencyPaused || now.Before(electionCfg.StartTime) || now.After(electionCfg.EndTime) {
		return nil, ErrElectionClosed
	}

	// 2. Verifikasi Data Pemilih Awal
	voter, err := s.voterRepo.FindByID(voterID)
	if err != nil {
		return nil, errors.New("data pemilih tidak ditemukan di DPT")
	}
	if voter.HasVoted {
		return nil, ErrAlreadyVoted
	}

	// 3. Verifikasi Cloudflare Turnstile Bot Protection
	if err := s.verifyTurnstile(ctx, req.TurnstileToken); err != nil {
		return nil, err
	}

	// 4. LAYER 1: Redis Distributed Lock dengan Ownership Token
	lockKey := fmt.Sprintf("lock:vote:%s", voterID)
	lockToken := uuid.New().String()
	if s.redisService != nil && s.redisService.IsAvailable() {
		acquired, err := s.redisService.AcquireLockWithToken(ctx, lockKey, lockToken, 15*time.Second)
		if err != nil {
			if errors.Is(err, redis.ErrRedisUnavailable) {
				if s.cfg.ServerEnv == "production" || s.cfg.RedisRequired {
					return nil, fmt.Errorf("layanan antrean terdistribusi (Redis) sedang tidak tersedia: %w", err)
				}
			} else {
				return nil, fmt.Errorf("gagal memperoleh kunci antrean voting: %w", err)
			}
		} else if !acquired {
			return nil, ErrConcurrentConflict
		} else {
			defer func() {
				_ = s.redisService.ReleaseLockWithToken(ctx, lockKey, lockToken)
			}()
		}
	} else if s.cfg.ServerEnv == "production" || s.cfg.RedisRequired {
		return nil, fmt.Errorf("layanan antrean terdistribusi (Redis) wajib aktif di production: %w", redis.ErrRedisUnavailable)
	}

	// 5. Validasi Kursi & Kandidat (In-Memory Lookup untuk Menghapus N+1 Query)
	eligibleChairs, err := s.getEligibleChairsCached(ctx, voter.Major)
	if err != nil {
		return nil, fmt.Errorf("gagal memvalidasi hak kursi: %w", err)
	}

	if len(req.Votes) == 0 {
		return nil, fmt.Errorf("%w: pilihan suara tidak boleh kosong", ErrInvalidCandidate)
	}

	chairMaxSelection := make(map[uint]int)
	validCandidateChair := make(map[string]uint)
	for _, chair := range eligibleChairs {
		maxSel := chair.MaxSelection
		if maxSel <= 0 {
			maxSel = 1
		}
		chairMaxSelection[chair.ID] = maxSel
		for _, cand := range chair.Candidates {
			validCandidateChair[cand.ID] = chair.ID
		}
	}

	seenChairCount := make(map[uint]int)
	seenCandidates := make(map[string]bool)
	var ballotsToInsert []domain.Ballot

	for _, voteItem := range req.Votes {
		maxSel, chairEligible := chairMaxSelection[voteItem.ChairID]
		if !chairEligible {
			return nil, fmt.Errorf("%w: kursi ID %d di luar hak jurusan", ErrInvalidCandidate, voteItem.ChairID)
		}

		candChairID, candExists := validCandidateChair[voteItem.CandidateGroupID]
		if !candExists || candChairID != voteItem.ChairID {
			return nil, fmt.Errorf("%w: kandidat ID %s tidak sesuai dengan kursi ID %d", ErrInvalidCandidate, voteItem.CandidateGroupID, voteItem.ChairID)
		}

		if seenCandidates[voteItem.CandidateGroupID] {
			return nil, fmt.Errorf("%w: kandidat ID %s dipilih lebih dari sekali", ErrInvalidCandidate, voteItem.CandidateGroupID)
		}
		seenCandidates[voteItem.CandidateGroupID] = true

		seenChairCount[voteItem.ChairID]++
		if seenChairCount[voteItem.ChairID] > maxSel {
			return nil, fmt.Errorf("%w: jumlah pilihan untuk kursi ID %d melebihi batas maksimal (%d)", ErrInvalidCandidate, voteItem.ChairID, maxSel)
		}

		// Micro-batch jitter: acak selisih milidetik waktu ballot untuk mencegah timing attack correlation
		jitterMs := rand.Intn(1000)
		ballotTimestamp := time.Now().Add(time.Duration(jitterMs) * time.Millisecond)

		ballotsToInsert = append(ballotsToInsert, domain.Ballot{
			ID:               uuid.New().String(),
			ChairID:          voteItem.ChairID,
			CandidateGroupID: voteItem.CandidateGroupID,
			CreatedAt:        ballotTimestamp,
		})
	}

	// 6. Generate Cryptographic Voting Receipt (HMAC-SHA256)
	receiptToken, receiptHash := crypto.GenerateReceiptToken(voterID, s.cfg.ReceiptSecretKey, s.cfg.ReceiptSalt)

	// 7. LAYER 2: Atomic Database Transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("gagal memulai transaksi database: %w", tx.Error)
	}

	// Atomic Conditional Update: UPDATE voters SET has_voted = TRUE, voted_at = TRUNC(NOW(), 1 minute) WHERE id = ? AND has_voted = FALSE;
	success, err := s.voterRepo.AtomicMarkVoted(tx, voterID, receiptHash)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("kesalahan database saat update status suara: %w", err)
	}
	if !success {
		tx.Rollback()
		return nil, ErrAlreadyVoted
	}

	// Simpan surat suara ke kotak suara digital (ZERO LINKABILITY)
	if err := s.ballotRepo.InsertBatch(tx, ballotsToInsert); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal memasukkan surat suara ke kotak suara: %w", err)
	}

	// Simpan komitmen hash bukti tanda terima publik ke tabel receipt_commitments (ZERO LINKABILITY)
	receiptCommitment := &domain.ReceiptCommitment{
		ID:        uuid.New().String(),
		TokenHash: crypto.HashString(receiptToken),
		CreatedAt: time.Now(),
	}
	if err := s.ballotRepo.InsertReceiptCommitment(tx, receiptCommitment); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("gagal menyimpan komitmen tanda terima: %w", err)
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

// GetReceiptVerification memverifikasi keabsahan tanda terima dengan mencocokkan SHA-256 hash di tabel receipt_commitments
func (s *voteService) GetReceiptVerification(token string) (bool, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return false, nil
	}
	tokenHash := crypto.HashString(trimmed)
	return s.ballotRepo.ReceiptCommitmentExists(tokenHash)
}

func (s *voteService) getElectionConfigCached(ctx context.Context) (*domain.ElectionConfig, error) {
	cacheKey := "cache:election_config"
	if s.redisService != nil && s.redisService.IsAvailable() {
		val, err := s.redisService.Get(ctx, cacheKey)
		if err == nil && val != "" {
			var cfg domain.ElectionConfig
			if err := json.Unmarshal([]byte(val), &cfg); err == nil {
				return &cfg, nil
			}
		}
	}

	cfg, err := s.auditRepo.GetElectionConfig()
	if err != nil {
		return nil, err
	}

	if s.redisService != nil && s.redisService.IsAvailable() && cfg != nil {
		if bytes, err := json.Marshal(cfg); err == nil {
			_ = s.redisService.SetWithTTL(ctx, cacheKey, string(bytes), 30*time.Second)
		}
	}

	return cfg, nil
}

func (s *voteService) getEligibleChairsCached(ctx context.Context, major string) ([]domain.Chair, error) {
	cacheKey := fmt.Sprintf("cache:eligible_chairs:%s", major)
	if s.redisService != nil && s.redisService.IsAvailable() {
		val, err := s.redisService.Get(ctx, cacheKey)
		if err == nil && val != "" {
			var chairs []domain.Chair
			if err := json.Unmarshal([]byte(val), &chairs); err == nil {
				return chairs, nil
			}
		}
	}

	chairs, err := s.chairRepo.FindEligibleForMajor(major)
	if err != nil {
		return nil, err
	}

	if s.redisService != nil && s.redisService.IsAvailable() {
		if bytes, err := json.Marshal(chairs); err == nil {
			_ = s.redisService.SetWithTTL(ctx, cacheKey, string(bytes), 30*time.Second)
		}
	}

	return chairs, nil
}

func (s *voteService) verifyTurnstile(ctx context.Context, token string) error {
	if !s.cfg.EnableTurnstileVerify {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%w: token turnstile wajib disertakan", ErrTurnstileInvalid)
	}

	verifyURL := s.cfg.TurnstileVerifyURL
	if verifyURL == "" {
		verifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	}
	timeout := s.cfg.TurnstileTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	form := url.Values{
		"secret":   {s.cfg.TurnstileSecretKey},
		"response": {token},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, verifyURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("gagal menyiapkan verifikasi turnstile: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("gagal menghubungi cloudflare turnstile: %w", err)
	}
	defer response.Body.Close()

	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("respons verifikasi turnstile tidak valid: %w", err)
	}
	if response.StatusCode != http.StatusOK || !result.Success {
		return ErrTurnstileInvalid
	}
	return nil
}
