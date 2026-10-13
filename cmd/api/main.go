package main

import (
	"log"
	"net/http"
	"pemira-backend/internal/config"
	"pemira-backend/internal/database"
	"pemira-backend/internal/handler"
	"pemira-backend/internal/middleware"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

func main() {
	log.Println("=== INITIALIZING PEMIRA 2026 CORE BACKEND ENGINE ===")

	// 1. Load Konfigurasi Lingkungan
	cfg := config.LoadConfig()

	// 2. Hubungkan ke Database Relasional
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Fatal: Cannot connect to database: %v", err)
	}

	// 3. Jalankan Auto-Migration Skema Database Zero-Linkability
	if err := database.AutoMigrate(db); err != nil {
		log.Fatalf("Fatal: Database migration failed: %v", err)
	}
	database.SeedInitialData(db)

	// 4. Hubungkan ke Redis (In-Memory Lock & Caching)
	redisService, err := redis.NewRedisService(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Printf("Warning: Redis connection error: %v (Distributed locks will fallback to DB-level locks)", err)
	} else {
		log.Println("Redis service connected successfully.")
	}

	// 5. Inisialisasi Repositori
	voterRepo := repository.NewVoterRepository(db)
	ballotRepo := repository.NewBallotRepository(db)
	chairRepo := repository.NewChairRepository(db)
	auditRepo := repository.NewAuditRepository(db)

	// 6. Inisialisasi Layanan Bisnis
	authService := service.NewAuthService(voterRepo, redisService, cfg)
	voteService := service.NewVoteService(db, voterRepo, ballotRepo, chairRepo, auditRepo, redisService, cfg)

	// 7. Inisialisasi Controller / Handler HTTP
	authHandler := handler.NewAuthHandler(authService, voterRepo, chairRepo, cfg)
	voteHandler := handler.NewVoteHandler(voteService)
	candidateHandler := handler.NewCandidateHandler(chairRepo, voterRepo)
	adminHandler := handler.NewAdminHandler(voterRepo, ballotRepo, auditRepo, cfg)

	// 8. Inisialisasi Router Gin
	if cfg.ServerEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Setup CORS
	r.Use(middleware.SetupCORS(cfg))

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "healthy",
			"service": "pemira-2026-backend",
			"engine":  "Zero-Linkability Ballot Engine",
		})
	})

	// =========================================================================
	// ROUTE GROUP: API V1
	// =========================================================================
	v1 := r.Group("/api/v1")
	{
		// --- 1. Autentikasi Pemilih & SSO Kampus ---
		authRoutes := v1.Group("/auth")
		{
			authRoutes.GET("/google/login", authHandler.GoogleLogin)
			authRoutes.GET("/google/callback", authHandler.GoogleCallback)
			authRoutes.POST("/mock-login", authHandler.DevMockLogin) // Simulasi akun tanpa OAuth untuk dev
			authRoutes.GET("/me", middleware.AuthMiddleware(authService), authHandler.GetMe)
		}

		// --- 2. Bilik Suara & Informasi Paslon ---
		boothRoutes := v1.Group("/booth")
		{
			boothRoutes.GET("/all-chairs", candidateHandler.GetAllChairs)
			boothRoutes.GET("/candidates", middleware.AuthMiddleware(authService), candidateHandler.GetBoothCandidates)
		}

		// --- 3. Transaksi Surat Suara (Zero-Linkability) ---
		voteRoutes := v1.Group("/vote")
		{
			voteRoutes.POST("/submit", middleware.AuthMiddleware(authService), voteHandler.SubmitVote)
			voteRoutes.GET("/receipt/:code", voteHandler.VerifyReceipt)
		}

		// --- 4. Panel Admin & Rekapitulasi (KPU / Bawaslu / Saksi) ---
		adminRoutes := v1.Group("/admin")
		{
			adminRoutes.GET("/turnout", adminHandler.GetTurnout)
			adminRoutes.POST("/tally", adminHandler.GetTallyResults)
			adminRoutes.GET("/audit-logs", middleware.AuthMiddleware(authService, "admin"), adminHandler.GetAuditLogs)
		}
	}

	// Jalankan Server
	addr := ":" + cfg.ServerPort
	log.Printf("Server listening on http://localhost%s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Fatal: Server failed to start: %v", err)
	}
}
