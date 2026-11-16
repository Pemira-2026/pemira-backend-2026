package middleware

import (
	"net/http"
	"net/http/httptest"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/service"
	"testing"

	"github.com/gin-gonic/gin"
)

func testAuthService(t *testing.T) (service.AuthService, string, string) {
	t.Helper()
	cfg := &config.Config{JWTSecret: "test-jwt-secret-long-enough-32-chars-at-least!"}
	svc := service.NewAuthService(nil, nil, cfg)

	voter := &domain.Voter{
		ID:    "voter-1",
		NIM:   "123",
		Email: "voter@stu.pnj.ac.id",
		Role:  "voter",
		Major: "Informatika",
	}
	token, err := svc.GenerateJWT(voter)
	if err != nil {
		t.Fatalf("failed to generate jwt: %v", err)
	}

	admin := &domain.Voter{
		ID:    "admin-1",
		NIM:   "456",
		Email: "admin@stu.pnj.ac.id",
		Role:  "admin",
		Major: "Informatika",
	}
	adminToken, err := svc.GenerateJWT(admin)
	if err != nil {
		t.Fatalf("failed to generate admin jwt: %v", err)
	}
	return svc, token, adminToken
}

func TestAuthMiddlewareRejectsInvalidAndMissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, _ := testAuthService(t)

	r := gin.New()
	r.GET("/protected", AuthMiddleware(svc), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// Missing Authorization
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing token, got %d", w.Code)
	}

	// Invalid JWT
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-jwt")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestAuthMiddlewareRoleSeparation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, voterToken, adminToken := testAuthService(t)

	r := gin.New()
	r.GET("/admin", AuthMiddleware(svc, "admin", "kpu", "bawaslu"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Voter cannot access admin route.
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+voterToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for voter on admin route, got %d", w.Code)
	}

	// Admin can access.
	req = httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", w.Code)
	}
}
