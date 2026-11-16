package monitoring

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsMiddlewareAndPrometheusOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics()

	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/boom", func(c *gin.Context) { c.JSON(http.StatusInternalServerError, gin.H{"error": true}) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	var buf bytes.Buffer
	m.WritePrometheus(&buf)
	output := buf.String()

	for _, want := range []string{
		"pemira_http_requests_total",
		`path="/ok"`,
		`path="/boom"`,
		"pemira_http_request_duration_seconds_sum",
		"pemira_http_errors_total",
		"pemira_go_goroutines",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("expected metrics output to contain %q", want)
		}
	}
}
