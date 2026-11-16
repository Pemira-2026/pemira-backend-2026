package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func setupTestAdminHandler(t *testing.T) (*gorm.DB, *AdminHandler, *config.Config, *domain.ElectionConfig) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		ServerEnv:          "test",
		DBDriver:           "sqlite",
		DBName:             ":memory:",
		DBMaxOpenConns:     5,
		DBMaxIdleConns:     2,
		DBConnMaxLifetime:  1 * time.Minute,
		KPUMasterSecret:    "kpu-secret-test-12345",
		BawasluAuditSecret: "bawaslu-secret-test-12345",
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}

	_ = database.AutoMigrate(db)

	now := time.Now()
	// Default: Election ended 1 hour ago
	eCfg := domain.ElectionConfig{
		Title:             "Pemira Test",
		StartTime:         now.Add(-2 * time.Hour),
		EndTime:           now.Add(-1 * time.Hour),
		IsEmergencyPaused: false,
	}
	db.Create(&eCfg)

	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	handler := NewAdminHandler(voterRepo, ballotRepo, auditRepo, &redis.RedisService{Client: nil}, cfg)
	return db, handler, cfg, &eCfg
}

func TestGetTallyResultsFailClosedOnConfigError(t *testing.T) {
	db, handler, _, eCfg := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// Delete election config to trigger error / nil config
	db.Delete(eCfg)

	r := gin.New()
	r.POST("/admin/tally", handler.GetTallyResults)

	req := httptest.NewRequest(http.MethodPost, "/admin/tally", nil)
	req.Header.Set("x-kpu-secret-key", "kpu-secret-test-12345")
	req.Header.Set("x-bawaslu-secret-key", "bawaslu-secret-test-12345")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Must fail-closed with 500
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected fail-closed 500 when ElectionConfig is missing, got %d", w.Code)
	}
}

func TestGetTallyResultsQuarantinedBeforeEndTime(t *testing.T) {
	db, handler, _, eCfg := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// Update EndTime to 2 hours in the future
	db.Model(eCfg).Update("end_time", time.Now().Add(2*time.Hour))

	r := gin.New()
	r.POST("/admin/tally", handler.GetTallyResults)

	req := httptest.NewRequest(http.MethodPost, "/admin/tally", nil)
	req.Header.Set("x-kpu-secret-key", "kpu-secret-test-12345")
	req.Header.Set("x-bawaslu-secret-key", "bawaslu-secret-test-12345")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Must be rejected with 403 Forbidden (QUARANTINED)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden when election is still ongoing, got %d", w.Code)
	}
}

func TestGetTallyResultsRejectsInvalidQuorumKeys(t *testing.T) {
	db, handler, _, _ := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	r := gin.New()
	r.POST("/admin/tally", handler.GetTallyResults)

	// Wrong KPU key
	req := httptest.NewRequest(http.MethodPost, "/admin/tally", nil)
	req.Header.Set("x-kpu-secret-key", "wrong-kpu-key")
	req.Header.Set("x-bawaslu-secret-key", "bawaslu-secret-test-12345")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for wrong quorum keys, got %d", w.Code)
	}

	// Verify audit log recorded failure
	var auditCount int64
	db.Model(&domain.AuditLog{}).Where("action = ?", "TALLY_QUORUM_FAILED").Count(&auditCount)
	if auditCount == 0 {
		t.Errorf("expected TALLY_QUORUM_FAILED to be recorded in audit_logs")
	}
}

func TestGetTallyResultsCalculatesPercentageAccurately(t *testing.T) {
	db, handler, _, _ := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// Seed chair & candidates
	chair := domain.Chair{Name: "Presiden Mahasiswa", Scope: "ALL", MaxSelection: 1, IsActive: true}
	db.Create(&chair)

	cand1 := domain.CandidateGroup{ID: uuid.New().String(), ChairID: chair.ID, BallotNumber: 1, LeaderName: "Paslon 1"}
	cand2 := domain.CandidateGroup{ID: uuid.New().String(), ChairID: chair.ID, BallotNumber: 2, LeaderName: "Paslon 2"}
	db.Create(&cand1)
	db.Create(&cand2)

	// Seed ballots: 3 votes for Paslon 1, 1 vote for Paslon 2 -> 75% vs 25%
	db.Create(&domain.Ballot{ID: uuid.New().String(), ChairID: chair.ID, CandidateGroupID: cand1.ID})
	db.Create(&domain.Ballot{ID: uuid.New().String(), ChairID: chair.ID, CandidateGroupID: cand1.ID})
	db.Create(&domain.Ballot{ID: uuid.New().String(), ChairID: chair.ID, CandidateGroupID: cand1.ID})
	db.Create(&domain.Ballot{ID: uuid.New().String(), ChairID: chair.ID, CandidateGroupID: cand2.ID})

	r := gin.New()
	r.POST("/admin/tally", handler.GetTallyResults)

	req := httptest.NewRequest(http.MethodPost, "/admin/tally", nil)
	req.Header.Set("x-kpu-secret-key", "kpu-secret-test-12345")
	req.Header.Set("x-bawaslu-secret-key", "bawaslu-secret-test-12345")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid quorum tally, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success bool                     `json:"success"`
		Status  string                   `json:"status"`
		Tally   []domain.TallyResultItem `json:"tally"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Tally) != 2 {
		t.Fatalf("expected 2 tally result items, got %d", len(resp.Tally))
	}

	// Check percentages: Paslon 1 should be 75%, Paslon 2 should be 25%
	if resp.Tally[0].Percentage != 75.0 {
		t.Errorf("expected 75.0%% for Paslon 1, got %f", resp.Tally[0].Percentage)
	}
	if resp.Tally[1].Percentage != 25.0 {
		t.Errorf("expected 25.0%% for Paslon 2, got %f", resp.Tally[1].Percentage)
	}

	// Verify audit log recorded success
	var auditCount int64
	db.Model(&domain.AuditLog{}).Where("action = ?", "TALLY_UNLOCKED").Count(&auditCount)
	if auditCount == 0 {
		t.Errorf("expected TALLY_UNLOCKED in audit_logs")
	}
}

func TestDevMockLoginDisabledInProduction(t *testing.T) {
	prodCfg := &config.Config{
		ServerEnv: "production",
	}

	authHandler := NewAuthHandler(
		nil,
		nil,
		nil,
		prodCfg,
	)

	r := gin.New()
	r.POST("/auth/mock-login", authHandler.DevMockLogin)

	body := []byte(`{"email":"test@univ.ac.id"}`)
	req := httptest.NewRequest(http.MethodPost, "/auth/mock-login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for mock login in production, got %d", w.Code)
	}
}

func TestStatelessOAuthStateFallback(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "test-jwt-secret-for-stateless-oauth-state-32chars!",
	}
	// Redis nil -> triggers stateless fallback
	authSvc := service.NewAuthService(nil, &redis.RedisService{Client: nil}, cfg)

	ctx := context.Background()

	// 1. Generate state
	state, err := authSvc.GenerateOAuthState(ctx)
	if err != nil {
		t.Fatalf("expected successful stateless state generation, got %v", err)
	}

	// 2. Validate valid state
	valid, err := authSvc.ValidateOAuthState(ctx, state)
	if err != nil || !valid {
		t.Errorf("expected valid stateless state verification, got (%v, %v)", valid, err)
	}

	// 3. Validate tampered state fails
	tamperedState := state + "tampered"
	validTampered, err := authSvc.ValidateOAuthState(ctx, tamperedState)
	if validTampered || err == nil {
		t.Errorf("expected tampered state to fail validation, got (%v, %v)", validTampered, err)
	}
}

func TestUpdateElectionConfigInvalidatesAndAudits(t *testing.T) {
	db, handler, _, eCfg := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	r := gin.New()
	r.PUT("/admin/election-config", handler.UpdateElectionConfig)

	body := []byte(`{"title":"Pemira Revisi","isEmergencyPaused":true}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/election-config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for config update, got %d: %s", w.Code, w.Body.String())
	}

	var updated domain.ElectionConfig
	if err := db.First(&updated).Error; err != nil {
		t.Fatalf("failed to read updated config: %v", err)
	}
	if updated.Title != "Pemira Revisi" || !updated.IsEmergencyPaused {
		t.Errorf("config update not persisted: %+v", updated)
	}

	var auditCount int64
	db.Model(&domain.AuditLog{}).Where("action = ?", "CONFIG_CHANGE").Count(&auditCount)
	if auditCount == 0 {
		t.Error("expected CONFIG_CHANGE audit log")
	}

	_ = eCfg
}

func TestUpdateElectionConfigRejectsInvalidWindow(t *testing.T) {
	db, handler, _, _ := setupTestAdminHandler(t)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	r := gin.New()
	r.PUT("/admin/election-config", handler.UpdateElectionConfig)

	body := []byte(`{"startTime":"2026-12-11T17:00:00+07:00","endTime":"2026-12-10T08:00:00+07:00"}`)
	req := httptest.NewRequest(http.MethodPut, "/admin/election-config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for startTime >= endTime, got %d", w.Code)
	}
}
