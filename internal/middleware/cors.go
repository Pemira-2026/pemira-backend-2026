package middleware

import (
	"pemira-backend/internal/config"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupCORS(cfg *config.Config) gin.HandlerFunc {
	corsConfig := cors.DefaultConfig()

	// Whitelist domain Voter Portal dan Admin Dashboard
	allowedOrigins := []string{
		"http://localhost:5173", // Vite default port
		"http://localhost:5174", // Admin dashboard port
		"http://localhost:3000",
	}

	if cfg.VoterFrontendURL != "" {
		allowedOrigins = append(allowedOrigins, cfg.VoterFrontendURL)
	}
	if cfg.AdminFrontendURL != "" {
		allowedOrigins = append(allowedOrigins, cfg.AdminFrontendURL)
	}

	corsConfig.AllowOrigins = allowedOrigins
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{
		"Origin",
		"Content-Length",
		"Content-Type",
		"Authorization",
		"X-Requested-With",
		"x-kpu-secret-key",
		"x-bawaslu-secret-key",
	}
	corsConfig.AllowMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

	return cors.New(corsConfig)
}
