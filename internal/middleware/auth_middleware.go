package middleware

import (
	"net/http"
	"pemira-backend/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(authService service.AuthService, requiredRole ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format. Use 'Bearer <token>'"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := authService.ValidateJWT(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token: " + err.Error()})
			c.Abort()
			return
		}

		// Role Checking jika disyaratkan
		if len(requiredRole) > 0 {
			authorized := false
			for _, role := range requiredRole {
				if claims.Role == role || claims.Role == "admin" {
					authorized = true
					break
				}
			}
			if !authorized {
				c.JSON(http.StatusForbidden, gin.H{"error": "You do not have permission to access this resource"})
				c.Abort()
				return
			}
		}

		c.Set("claims", claims)
		c.Set("voterId", claims.VoterID)
		c.Next()
	}
}
