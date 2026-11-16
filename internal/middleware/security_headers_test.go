package middleware

import (
	"net/http"
	"net/http/httptest"
	"pemira-backend/internal/config"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSecurityHeadersDevelopment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders(&config.Config{ServerEnv: "development"}))
	r.GET("/", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, want := range expected {
		if got := w.Header().Get(header); got != want {
			t.Errorf("header %s: expected %q, got %q", header, want, got)
		}
	}
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS must not be emitted outside production, got %q", got)
	}
	if got := w.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("expected a Content-Security-Policy header")
	}
}

func TestSecurityHeadersProductionAddsHSTS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(SecurityHeaders(&config.Config{ServerEnv: "production"}))
	r.GET("/", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("expected HSTS header in production")
	}
}
