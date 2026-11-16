package handler

import (
	"context"
	"fmt"
	"net/http"
	"pemira-backend/internal/config"
	"pemira-backend/pkg/redis"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ComponentHealth struct {
	Status  string `json:"status"`            // "up" | "down"
	Latency string `json:"latency,omitempty"` // e.g. "1.2ms"
	Message string `json:"message,omitempty"`
}

type HealthResponse struct {
	Status      string                     `json:"status"` // "healthy" | "degraded" | "unhealthy"
	Service     string                     `json:"service"`
	Environment string                     `json:"environment"`
	Timestamp   string                     `json:"timestamp"`
	Checks      map[string]ComponentHealth `json:"checks"`
}

type HealthHandler struct {
	db           *gorm.DB
	redisService *redis.RedisService
	cfg          *config.Config
}

func NewHealthHandler(db *gorm.DB, redisService *redis.RedisService, cfg *config.Config) *HealthHandler {
	return &HealthHandler{
		db:           db,
		redisService: redisService,
		cfg:          cfg,
	}
}

// CheckHealth melakukan pengecekan mendalam ke Database dan Redis
func (h *HealthHandler) CheckHealth(c *gin.Context) {
	checks := make(map[string]ComponentHealth)
	allHealthy := true
	dbHealthy := true
	redisHealthy := true

	// 1. Check Database Health
	ctxDB, cancelDB := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancelDB()

	startDB := time.Now()
	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(ctxDB) != nil {
		dbHealthy = false
		allHealthy = false
		errMsg := "database connection failed"
		if err != nil {
			errMsg = err.Error()
		}
		checks["database"] = ComponentHealth{
			Status:  "down",
			Message: errMsg,
		}
	} else {
		checks["database"] = ComponentHealth{
			Status:  "up",
			Latency: fmt.Sprintf("%.2fms", float64(time.Since(startDB).Microseconds())/1000.0),
		}
	}

	// 2. Check Redis Health
	ctxRedis, cancelRedis := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancelRedis()

	startRedis := time.Now()
	if h.redisService == nil || !h.redisService.IsAvailable() || h.redisService.Ping(ctxRedis) != nil {
		redisHealthy = false
		if h.cfg.ServerEnv == "production" || h.cfg.RedisRequired {
			allHealthy = false
		}
		checks["redis"] = ComponentHealth{
			Status:  "down",
			Message: "redis service is unavailable",
		}
	} else {
		checks["redis"] = ComponentHealth{
			Status:  "up",
			Latency: fmt.Sprintf("%.2fms", float64(time.Since(startRedis).Microseconds())/1000.0),
		}
	}

	// 3. Tentukan status agregat
	overallStatus := "healthy"
	httpStatusCode := http.StatusOK

	if !dbHealthy {
		overallStatus = "unhealthy"
		httpStatusCode = http.StatusServiceUnavailable
	} else if !redisHealthy {
		if h.cfg.ServerEnv == "production" || h.cfg.RedisRequired {
			overallStatus = "unhealthy"
			httpStatusCode = http.StatusServiceUnavailable
		} else {
			overallStatus = "degraded"
			httpStatusCode = http.StatusOK
		}
	} else if !allHealthy {
		overallStatus = "unhealthy"
		httpStatusCode = http.StatusServiceUnavailable
	}

	resp := HealthResponse{
		Status:      overallStatus,
		Service:     "pemira-2026-backend",
		Environment: h.cfg.ServerEnv,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Checks:      checks,
	}

	c.JSON(httpStatusCode, resp)
}

// LivenessProbe endpoint ringan untuk kubernetes liveness probe
func (h *HealthHandler) LivenessProbe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "alive",
		"service": "pemira-2026-backend",
	})
}

// ReadinessProbe memvalidasi apakah aplikasi siap melayani lalu lintas pemilih
func (h *HealthHandler) ReadinessProbe(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	sqlDB, err := h.db.DB()
	if err != nil || sqlDB.PingContext(ctx) != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": "database not ready",
		})
		return
	}

	if (h.cfg.ServerEnv == "production" || h.cfg.RedisRequired) &&
		(h.redisService == nil || !h.redisService.IsAvailable() || h.redisService.Ping(ctx) != nil) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not_ready",
			"reason": "redis distributed lock not ready in production",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}
