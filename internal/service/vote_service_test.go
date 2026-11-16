package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/pkg/redis"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestVoteService(t *testing.T) (*gorm.DB, VoteService, *domain.Voter, *domain.Chair, *domain.CandidateGroup, *domain.CandidateGroup) {
	cfg := &config.Config{
		ServerEnv:             "test",
		DBDriver:              "sqlite",
		DBName:                ":memory:",
		DBMaxOpenConns:        5,
		DBMaxIdleConns:        2,
		DBConnMaxLifetime:     1 * time.Minute,
		JWTSecret:             "test-jwt-secret-long-enough-32-chars-at-least!",
		ReceiptSecretKey:      "test-receipt-secret-key-32-chars-long!",
		ReceiptSalt:           "test-receipt-salt-16-chars!",
		EnableTurnstileVerify: false,
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}

	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	// Active election config
	now := time.Now()
	eCfg := domain.ElectionConfig{
		Title:             "Pemira Test",
		StartTime:         now.Add(-1 * time.Hour),
		EndTime:           now.Add(1 * time.Hour),
		IsEmergencyPaused: false,
	}
	db.Create(&eCfg)

	// Chair with MaxSelection = 1
	chair := domain.Chair{
		Name:         "Chair BEM",
		Scope:        "ALL",
		MaxSelection: 1,
		IsActive:     true,
	}
	db.Create(&chair)

	// Two candidates for Chair
	cand1 := domain.CandidateGroup{
		ID:           uuid.New().String(),
		ChairID:      chair.ID,
		BallotNumber: 1,
		LeaderName:   "Leader One",
	}
	cand2 := domain.CandidateGroup{
		ID:           uuid.New().String(),
		ChairID:      chair.ID,
		BallotNumber: 2,
		LeaderName:   "Leader Two",
	}
	db.Create(&cand1)
	db.Create(&cand2)

	// Voter
	voter := domain.Voter{
		ID:         "voter-test-1",
		NIM:        "12345678",
		FullName:   "Test Voter",
		Email:      "test@univ.ac.id",
		Faculty:    "Teknik",
		Major:      "Informatika",
		Generation: 2023,
		Role:       "voter",
		HasVoted:   false,
	}
	db.Create(&voter)

	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	chairRepo := repository.NewChairRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	redisService := &redis.RedisService{Client: nil}

	svc := NewVoteService(db, voterRepo, ballotRepo, chairRepo, auditRepo, redisService, cfg)
	return db, svc, &voter, &chair, &cand1, &cand2
}

func TestSubmitVoteRejectDuplicateChairAndExceedMaxSelection(t *testing.T) {
	db, svc, voter, chair, cand1, cand2 := setupTestVoteService(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	ctx := context.Background()

	// Request has 2 selections for a chair with MaxSelection = 1
	req := domain.SubmitVoteRequest{
		Votes: []domain.SingleVoteItem{
			{ChairID: chair.ID, CandidateGroupID: cand1.ID},
			{ChairID: chair.ID, CandidateGroupID: cand2.ID},
		},
	}

	_, err := svc.SubmitVote(ctx, voter.ID, req)
	if err == nil {
		t.Fatalf("expected error when chair selection exceeds max_selection, got nil")
	}
	if !errors.Is(err, ErrInvalidCandidate) {
		t.Errorf("expected ErrInvalidCandidate, got %v", err)
	}

	// Verify voter has not voted in DB
	var dbVoter domain.Voter
	db.First(&dbVoter, "id = ?", voter.ID)
	if dbVoter.HasVoted {
		t.Errorf("voter should not be marked as voted on failed submission")
	}
}

func TestSubmitVoteRejectDuplicateCandidateInSameChair(t *testing.T) {
	db, svc, voter, chair, cand1, _ := setupTestVoteService(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	ctx := context.Background()

	// Update chair to allow 2 selections
	db.Model(&domain.Chair{}).Where("id = ?", chair.ID).Update("max_selection", 2)

	// Send duplicate candidate
	req := domain.SubmitVoteRequest{
		Votes: []domain.SingleVoteItem{
			{ChairID: chair.ID, CandidateGroupID: cand1.ID},
			{ChairID: chair.ID, CandidateGroupID: cand1.ID},
		},
	}

	_, err := svc.SubmitVote(ctx, voter.ID, req)
	if err == nil {
		t.Fatalf("expected error when same candidate is duplicated, got nil")
	}
	if !errors.Is(err, ErrInvalidCandidate) {
		t.Errorf("expected ErrInvalidCandidate, got %v", err)
	}
}

func TestVerifyReceiptReal(t *testing.T) {
	db, svc, voter, chair, cand1, _ := setupTestVoteService(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	ctx := context.Background()

	// 1. Check fake token returns false
	valid, err := svc.GetReceiptVerification("VOTE-RCPT-FAKE-TOKEN-12345")
	if err != nil {
		t.Fatalf("unexpected error verifying fake token: %v", err)
	}
	if valid {
		t.Errorf("fake token must not be valid")
	}

	// 2. Submit valid vote
	req := domain.SubmitVoteRequest{
		Votes: []domain.SingleVoteItem{
			{ChairID: chair.ID, CandidateGroupID: cand1.ID},
		},
	}

	res, err := svc.SubmitVote(ctx, voter.ID, req)
	if err != nil {
		t.Fatalf("expected vote submission to succeed, got %v", err)
	}
	if res.ReceiptToken == "" {
		t.Fatalf("expected receipt token to be generated")
	}

	// 3. Verify real token returns true
	validReal, err := svc.GetReceiptVerification(res.ReceiptToken)
	if err != nil {
		t.Fatalf("unexpected error verifying real token: %v", err)
	}
	if !validReal {
		t.Errorf("real receipt token must be verified as true")
	}
}

func TestSubmitVoteTurnstileRequiredWhenEnabled(t *testing.T) {
	cfg := &config.Config{
		ServerEnv:             "test",
		DBDriver:              "sqlite",
		DBName:                ":memory:",
		DBMaxOpenConns:        5,
		DBMaxIdleConns:        2,
		DBConnMaxLifetime:     1 * time.Minute,
		JWTSecret:             "test-jwt-secret-long-enough-32-chars-at-least!",
		ReceiptSecretKey:      "test-receipt-secret-key-32-chars-long!",
		ReceiptSalt:           "test-receipt-salt-16-chars!",
		EnableTurnstileVerify: true,
		TurnstileSecretKey:    "test-turnstile-secret",
	}

	db, _ := database.Connect(cfg)
	_ = database.AutoMigrate(db)

	now := time.Now()
	db.Create(&domain.ElectionConfig{
		Title:             "Pemira Test",
		StartTime:         now.Add(-1 * time.Hour),
		EndTime:           now.Add(1 * time.Hour),
		IsEmergencyPaused: false,
	})

	chair := domain.Chair{Name: "BEM", Scope: "ALL", MaxSelection: 1, IsActive: true}
	db.Create(&chair)
	cand := domain.CandidateGroup{ID: uuid.New().String(), ChairID: chair.ID, BallotNumber: 1, LeaderName: "Leader"}
	db.Create(&cand)
	voter := domain.Voter{ID: "v1", NIM: "1", FullName: "A", Email: "a@u.ac.id", Faculty: "T", Major: "M", Role: "voter"}
	db.Create(&voter)

	svc := NewVoteService(
		db,
		repository.NewVoterRepository(db),
		repository.NewBallotRepository(db),
		repository.NewChairRepository(db),
		repository.NewAuditRepository(db),
		&redis.RedisService{Client: nil},
		cfg,
	)

	// Missing TurnstileToken
	req := domain.SubmitVoteRequest{
		TurnstileToken: "",
		Votes: []domain.SingleVoteItem{
			{ChairID: chair.ID, CandidateGroupID: cand.ID},
		},
	}

	_, err := svc.SubmitVote(context.Background(), voter.ID, req)
	if err == nil {
		t.Fatalf("expected error when turnstile is enabled but token is empty, got nil")
	}
	if !errors.Is(err, ErrTurnstileInvalid) {
		t.Errorf("expected ErrTurnstileInvalid, got %v", err)
	}
}

func TestSubmitVoteAllowsMultipleSelectionsWhenMaxSelectionGreaterThanOne(t *testing.T) {
	db, svc, voter, chair, cand1, cand2 := setupTestVoteService(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	db.Model(&domain.Chair{}).Where("id = ?", chair.ID).Update("max_selection", 2)

	res, err := svc.SubmitVote(context.Background(), voter.ID, domain.SubmitVoteRequest{
		Votes: []domain.SingleVoteItem{
			{ChairID: chair.ID, CandidateGroupID: cand1.ID},
			{ChairID: chair.ID, CandidateGroupID: cand2.ID},
		},
	})
	if err != nil {
		t.Fatalf("expected multi-selection vote to succeed, got %v", err)
	}
	if res == nil || !res.Success {
		t.Fatalf("expected successful response, got %+v", res)
	}
}

func setupConcurrentVoteService(t *testing.T) (*gorm.DB, VoteService, *domain.Voter, *domain.Chair, *domain.CandidateGroup) {
	t.Helper()

	cfg := &config.Config{
		ServerEnv:             "test",
		DBDriver:              "sqlite",
		DBName:                filepath.Join(t.TempDir(), "vote-concurrent.db"),
		DBMaxOpenConns:        1,
		DBMaxIdleConns:        1,
		DBConnMaxLifetime:     time.Minute,
		JWTSecret:             "test-jwt-secret-long-enough-32-chars-at-least!",
		ReceiptSecretKey:      "test-receipt-secret-key-32-chars-long!",
		ReceiptSalt:           "test-receipt-salt-16-chars!",
		EnableTurnstileVerify: false,
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })

	now := time.Now()
	db.Create(&domain.ElectionConfig{
		Title:     "Pemira Test",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	})
	chair := domain.Chair{Name: "Chair BEM", Scope: "ALL", MaxSelection: 1, IsActive: true}
	db.Create(&chair)
	cand := domain.CandidateGroup{ID: uuid.New().String(), ChairID: chair.ID, BallotNumber: 1, LeaderName: "Leader"}
	db.Create(&cand)
	voter := domain.Voter{
		ID:         "voter-concurrent",
		NIM:        "99999999",
		FullName:   "Concurrent Voter",
		Email:      "concurrent@univ.ac.id",
		Faculty:    "Teknik",
		Major:      "Informatika",
		Generation: 2023,
		Role:       "voter",
	}
	db.Create(&voter)

	svc := NewVoteService(
		db,
		repository.NewVoterRepository(db),
		repository.NewBallotRepository(db),
		repository.NewChairRepository(db),
		repository.NewAuditRepository(db),
		&redis.RedisService{Client: nil},
		cfg,
	)
	return db, svc, &voter, &chair, &cand
}

func TestSubmitVoteConcurrentDoubleSubmitOnlyOneWins(t *testing.T) {
	_, svc, voter, chair, cand := setupConcurrentVoteService(t)

	const workers = 8
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successes int32
	var alreadyVoted int32

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.SubmitVote(context.Background(), voter.ID, domain.SubmitVoteRequest{
				Votes: []domain.SingleVoteItem{
					{ChairID: chair.ID, CandidateGroupID: cand.ID},
				},
			})
			if err == nil {
				atomic.AddInt32(&successes, 1)
				return
			}
			if errors.Is(err, ErrAlreadyVoted) {
				atomic.AddInt32(&alreadyVoted, 1)
			}
		}()
	}

	close(start)
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful vote, got %d (alreadyVoted=%d)", successes, alreadyVoted)
	}
	if alreadyVoted != workers-1 {
		t.Errorf("expected %d already-voted rejections, got %d", workers-1, alreadyVoted)
	}
}

func TestSubmitVoteTurnstileFailureResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	}))
	defer ts.Close()

	db, svc, voter, chair, cand := setupVoteServiceWithTurnstile(t, ts.URL, time.Second)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	_, err := svc.SubmitVote(context.Background(), voter.ID, domain.SubmitVoteRequest{
		TurnstileToken: "bad-token",
		Votes:          []domain.SingleVoteItem{{ChairID: chair.ID, CandidateGroupID: cand.ID}},
	})
	if !errors.Is(err, ErrTurnstileInvalid) {
		t.Fatalf("expected ErrTurnstileInvalid, got %v", err)
	}
}

func TestSubmitVoteTurnstileTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer ts.Close()

	db, svc, voter, chair, cand := setupVoteServiceWithTurnstile(t, ts.URL, 50*time.Millisecond)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	_, err := svc.SubmitVote(context.Background(), voter.ID, domain.SubmitVoteRequest{
		TurnstileToken: "slow-token",
		Votes:          []domain.SingleVoteItem{{ChairID: chair.ID, CandidateGroupID: cand.ID}},
	})
	if err == nil {
		t.Fatal("expected timeout error from Turnstile verification")
	}
}

func setupVoteServiceWithTurnstile(t *testing.T, verifyURL string, timeout time.Duration) (*gorm.DB, VoteService, *domain.Voter, *domain.Chair, *domain.CandidateGroup) {
	t.Helper()

	cfg := &config.Config{
		ServerEnv:             "test",
		DBDriver:              "sqlite",
		DBName:                ":memory:",
		DBMaxOpenConns:        5,
		DBMaxIdleConns:        2,
		DBConnMaxLifetime:     time.Minute,
		JWTSecret:             "test-jwt-secret-long-enough-32-chars-at-least!",
		ReceiptSecretKey:      "test-receipt-secret-key-32-chars-long!",
		ReceiptSalt:           "test-receipt-salt-16-chars!",
		EnableTurnstileVerify: true,
		TurnstileSecretKey:    "test-turnstile-secret",
		TurnstileVerifyURL:    verifyURL,
		TurnstileTimeout:      timeout,
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}

	now := time.Now()
	db.Create(&domain.ElectionConfig{
		Title:     "Pemira Test",
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(1 * time.Hour),
	})
	chair := domain.Chair{Name: "Chair BEM", Scope: "ALL", MaxSelection: 1, IsActive: true}
	db.Create(&chair)
	cand := domain.CandidateGroup{ID: uuid.New().String(), ChairID: chair.ID, BallotNumber: 1, LeaderName: "Leader"}
	db.Create(&cand)
	voter := domain.Voter{
		ID:         "voter-turnstile",
		NIM:        "88888888",
		FullName:   "Turnstile Voter",
		Email:      "turnstile@univ.ac.id",
		Faculty:    "Teknik",
		Major:      "Informatika",
		Generation: 2023,
		Role:       "voter",
	}
	db.Create(&voter)

	svc := NewVoteService(
		db,
		repository.NewVoterRepository(db),
		repository.NewBallotRepository(db),
		repository.NewChairRepository(db),
		repository.NewAuditRepository(db),
		&redis.RedisService{Client: nil},
		cfg,
	)
	return db, svc, &voter, &chair, &cand
}

func TestElectionConfigCacheFallsBackWhenRedisDown(t *testing.T) {
	db, svc, _, _, _, _ := setupTestVoteService(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	voteSvc, ok := svc.(*voteService)
	if !ok {
		t.Fatal("unexpected VoteService implementation")
	}
	cfg, err := voteSvc.getElectionConfigCached(context.Background())
	if err != nil {
		t.Fatalf("expected DB fallback when Redis is down, got %v", err)
	}
	if cfg == nil || cfg.Title == "" {
		t.Fatalf("expected election config from DB fallback, got %+v", cfg)
	}
}

func TestInvalidateElectionCachesWithRedisDownDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("cache invalidation panicked with Redis down: %v", r)
		}
	}()
	InvalidateElectionConfigCache(context.Background(), &redis.RedisService{Client: nil})
	InvalidateAllElectionCaches(context.Background(), nil)
	InvalidateCandidateCaches(context.Background(), &redis.RedisService{Client: nil})
}
