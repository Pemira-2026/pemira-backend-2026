package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/pkg/redis"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(handler *HealthHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", handler.CheckHealth)
	r.GET("/health/liveness", handler.LivenessProbe)
	r.GET("/health/readiness", handler.ReadinessProbe)
	return r
}

func TestHealthCheckEndpoints(t *testing.T) {
	// Setup in-memory sqlite db
	cfg := &config.Config{
		ServerEnv:         "development",
		DBDriver:          "sqlite",
		DBName:            ":memory:",
		DBMaxOpenConns:    5,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: 1 * time.Minute,
		RedisRequired:     false,
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	// Redis client nil (simulating dev without Redis)
	redisService := &redis.RedisService{Client: nil}

	healthHandler := NewHealthHandler(db, redisService, cfg)
	router := setupTestRouter(healthHandler)

	// 1. Test /health (in development with Redis down -> degraded, 200 OK)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for degraded dev health check, got %d", w.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse health response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", resp.Status)
	}
	if resp.Checks["database"].Status != "up" {
		t.Errorf("expected database check 'up', got '%s'", resp.Checks["database"].Status)
	}
	if resp.Checks["redis"].Status != "down" {
		t.Errorf("expected redis check 'down', got '%s'", resp.Checks["redis"].Status)
	}

	// 2. Test /health/liveness
	reqLive := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	wLive := httptest.NewRecorder()
	router.ServeHTTP(wLive, reqLive)

	if wLive.Code != http.StatusOK {
		t.Errorf("expected 200 for liveness probe, got %d", wLive.Code)
	}

	// 3. Test /health/readiness
	reqReady := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	wReady := httptest.NewRecorder()
	router.ServeHTTP(wReady, reqReady)

	if wReady.Code != http.StatusOK {
		t.Errorf("expected 200 for readiness probe in dev, got %d", wReady.Code)
	}
}

func TestHealthCheckProductionRedisDownFails(t *testing.T) {
	// Setup production config where Redis is required
	cfg := &config.Config{
		ServerEnv:         "production",
		DBDriver:          "sqlite",
		DBName:            ":memory:",
		DBMaxOpenConns:    5,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: 1 * time.Minute,
		RedisRequired:     true,
	}

	db, err := database.Connect(cfg)
	if err != nil {
		t.Fatalf("failed to connect test db: %v", err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()

	redisService := &redis.RedisService{Client: nil}

	healthHandler := NewHealthHandler(db, redisService, cfg)
	router := setupTestRouter(healthHandler)

	// In production, missing Redis MUST return 503 Service Unavailable
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 when Redis is down in production, got %d", w.Code)
	}

	var resp HealthResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Status != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%s'", resp.Status)
	}

	// Readiness probe must also fail in production when Redis is down
	reqReady := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	wReady := httptest.NewRecorder()
	router.ServeHTTP(wReady, reqReady)

	if wReady.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503 for readiness probe when Redis is down in production, got %d", wReady.Code)
	}
}
