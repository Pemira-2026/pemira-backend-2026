package middleware

import (
	"pemira-backend/internal/config"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders applies defensive HTTP response headers. The API only serves
// JSON, so CSP is intentionally restrictive. HSTS is only emitted in
// production; TLS termination is handled by Cloudflare/Nginx in front of the
// Go process.
func SecurityHeaders(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("X-Permitted-Cross-Domain-Policies", "none")
		h.Set("Cache-Control", "no-store")

		if cfg.ServerEnv == "production" {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		c.Next()
	}
}
