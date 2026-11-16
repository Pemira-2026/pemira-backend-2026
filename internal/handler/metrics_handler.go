package handler

import (
	"net/http"
	"strings"

	"pemira-backend/internal/monitoring"

	"github.com/gin-gonic/gin"
)

type MetricsHandler struct {
	metrics *monitoring.Metrics
	token   string
}

func NewMetricsHandler(metrics *monitoring.Metrics, token string) *MetricsHandler {
	return &MetricsHandler{metrics: metrics, token: token}
}

// Serve exposes Prometheus metrics. When METRICS_TOKEN is configured, the
// caller must send "Authorization: Bearer <token>" or "X-Metrics-Token".
func (h *MetricsHandler) Serve(c *gin.Context) {
	if h.token != "" {
		provided := c.GetHeader("X-Metrics-Token")
		if provided == "" {
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}
		}
		if provided != h.token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "metrics token required"})
			return
		}
	}

	c.Header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	h.metrics.WritePrometheus(c.Writer)
}
