package main

import (
	"log"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/handler"
	"pemira-backend/internal/middleware"
	"pemira-backend/internal/monitoring"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("=== INITIALIZING PEMIRA 2026 CORE BACKEND ENGINE ===")

	// 1. Load Konfigurasi Lingkungan & Validasi Security State
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Fatal: Configuration error:\n%v", err)
	}

	// 2. Hubungkan ke Database Relasional dengan Connection Pooling & Ping Verification
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Fatal: Cannot connect to database: %v", err)
	}

	// 3. Jalankan Migrasi Skema Database (AutoMigrate di dev/opt-in, Verifikasi di prod)
	if err := database.RunMigrations(db, cfg); err != nil {
		log.Fatalf("Fatal: Database migration failed: %v", err)
	}

	// 4. Inisialisasi Data Awal (HANYA dijalankan pada environment development)
	database.SeedInitialData(db, cfg)

	// 5. Hubungkan ke Redis (Distributed Lock & Session Cache)
	redisRequired := cfg.ServerEnv == "production" || cfg.RedisRequired
	redisService, err := redis.NewRedisService(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB, redisRequired)
	if err != nil {
		if redisRequired {
			log.Fatalf("Fatal: Redis connection required in %s mode: %v", cfg.ServerEnv, err)
		} else {
			log.Printf("Warning: Redis connection error in development: %v (Distributed locks disabled)", err)
		}
	} else {
		log.Println("Redis service connected successfully.")
	}
	if redisService != nil {
		defer func() { _ = redisService.Close() }()
	}

	// 6. Inisialisasi Repositori
	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	chairRepo := repository.NewChairRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	// 7. Inisialisasi Layanan Bisnis
	authService := service.NewAuthService(voterRepo, redisService, cfg)
	voteService := service.NewVoteService(db, voterRepo, ballotRepo, chairRepo, auditRepo, redisService, cfg)

	// 8. Inisialisasi Controller / Handler HTTP
	authHandler := handler.NewAuthHandler(authService, voterRepo, chairRepo, cfg)
	voteHandler := handler.NewVoteHandler(voteService)
	candidateHandler := handler.NewCandidateHandler(chairRepo, voterRepo, redisService)
	adminHandler := handler.NewAdminHandler(voterRepo, ballotRepo, auditRepo, redisService, cfg)
	healthHandler := handler.NewHealthHandler(db, redisService, cfg)

	metrics := monitoring.NewMetrics()
	metricsHandler := handler.NewMetricsHandler(metrics, cfg.MetricsToken)
	rateLimiter := middleware.NewRateLimiter(redisService)

	// 9. Inisialisasi Router Gin
	if cfg.ServerEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Only trust proxy headers from known reverse proxies (Nginx/cloudflared
	// on localhost by default). This prevents clients from spoofing ClientIP
	// and bypassing rate limits.
	if len(cfg.TrustedProxies) > 0 {
		if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
			log.Printf("Warning: invalid TRUSTED_PROXIES configuration: %v", err)
		}
	} else {
		_ = r.SetTrustedProxies(nil)
	}

	// Global hardening middleware.
	r.Use(middleware.SecurityHeaders(cfg))
	if cfg.MetricsEnabled {
		r.Use(metrics.Middleware())
	}
	r.Use(middleware.SetupCORS(cfg))

	// Health Checks (Root Level)
	healthLimiter := rateLimiter.Limit("health", 120, time.Minute)
	r.GET("/health", healthLimiter, healthHandler.CheckHealth)
	r.GET("/healthz", healthLimiter, healthHandler.CheckHealth)
	r.GET("/health/liveness", healthLimiter, healthHandler.LivenessProbe)
	r.GET("/health/readiness", healthLimiter, healthHandler.ReadinessProbe)

	// Prometheus-compatible metrics (protect with METRICS_TOKEN in production).
	if cfg.MetricsEnabled {
		r.GET("/metrics", metricsHandler.Serve)
	}

	// =========================================================================
	// ROUTE GROUP: API V1
	// =========================================================================
	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", healthLimiter, healthHandler.CheckHealth)

		// --- 1. Autentikasi Pemilih & SSO Kampus ---
		authRoutes := v1.Group("/auth")
		{
			authRoutes.GET("/google/login", rateLimiter.Limit("auth-google-login", 30, time.Minute), authHandler.GoogleLogin)
			authRoutes.GET("/google/callback", rateLimiter.Limit("auth-google-callback", 30, time.Minute), authHandler.GoogleCallback)
			if cfg.ServerEnv != "production" {
				authRoutes.POST("/mock-login", rateLimiter.Limit("auth-mock-login", 10, time.Minute), authHandler.DevMockLogin) // Simulasi akun tanpa OAuth untuk dev
			}
			authRoutes.GET("/me", rateLimiter.Limit("auth-me", 120, time.Minute), middleware.AuthMiddleware(authService), authHandler.GetMe)
		}

		// --- 2. Bilik Suara & Informasi Paslon ---
		boothRoutes := v1.Group("/booth")
		{
			boothRoutes.GET("/all-chairs", rateLimiter.Limit("booth-all-chairs", 120, time.Minute), candidateHandler.GetAllChairs)
			boothRoutes.GET("/candidates", rateLimiter.Limit("booth-candidates", 120, time.Minute), middleware.AuthMiddleware(authService), candidateHandler.GetBoothCandidates)
		}

		// --- 3. Transaksi Surat Suara (Zero-Linkability) ---
		voteRoutes := v1.Group("/vote")
		{
			voteRoutes.POST("/submit",
				middleware.AuthMiddleware(authService),
				rateLimiter.LimitBy("vote-submit", 10, time.Minute, middleware.VoterKey),
				voteHandler.SubmitVote,
			)
			voteRoutes.GET("/receipt/:code", rateLimiter.Limit("vote-receipt", 60, time.Minute), voteHandler.VerifyReceipt)
		}

		// --- 4. Panel Admin & Rekapitulasi (KPU / Bawaslu / Saksi) ---
		adminRoutes := v1.Group("/admin")
		{
			adminRoutes.GET("/turnout", middleware.AuthMiddleware(authService, "admin", "kpu", "bawaslu", "saksi"), rateLimiter.Limit("admin-turnout", 60, time.Minute), adminHandler.GetTurnout)
			adminRoutes.POST("/tally", middleware.AuthMiddleware(authService, "admin", "kpu", "bawaslu"), rateLimiter.Limit("admin-tally", 10, time.Minute), adminHandler.GetTallyResults)
			adminRoutes.GET("/audit-logs", middleware.AuthMiddleware(authService, "admin", "bawaslu"), rateLimiter.Limit("admin-audit-logs", 30, time.Minute), adminHandler.GetAuditLogs)
			adminRoutes.GET("/election-config", middleware.AuthMiddleware(authService, "admin", "kpu", "bawaslu"), rateLimiter.Limit("admin-config-read", 60, time.Minute), adminHandler.GetElectionConfig)
			adminRoutes.PUT("/election-config", middleware.AuthMiddleware(authService, "admin"), rateLimiter.Limit("admin-config-write", 20, time.Minute), adminHandler.UpdateElectionConfig)
			adminRoutes.POST("/cache/invalidate", middleware.AuthMiddleware(authService, "admin"), rateLimiter.Limit("admin-cache-invalidate", 10, time.Minute), adminHandler.InvalidateCache)
		}
	}

	// Jalankan Server
	addr := ":" + cfg.ServerPort
	log.Printf("Server listening on http://localhost%s (Env: %s)", addr, cfg.ServerEnv)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Fatal: Server failed to start: %v", err)
	}
}
