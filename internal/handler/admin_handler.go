package handler

import (
	"net/http"
	"pemira-backend/internal/config"
	"pemira-backend/internal/repository"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	voterRepo  repository.VoterRepository
	ballotRepo repository.BallotRepository
	auditRepo  repository.AuditRepository
	cfg        *config.Config
}

func NewAdminHandler(
	voterRepo repository.VoterRepository,
	ballotRepo repository.BallotRepository,
	auditRepo repository.AuditRepository,
	cfg *config.Config,
) *AdminHandler {
	return &AdminHandler{
		voterRepo:  voterRepo,
		ballotRepo: ballotRepo,
		auditRepo:  auditRepo,
		cfg:        cfg,
	}
}

// GetTurnout mengembalikan statistik partisipasi pemilih real-time (tanpa membocorkan perolehan suara)
func (h *AdminHandler) GetTurnout(c *gin.Context) {
	total, voted, err := h.voterRepo.GetTurnoutStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca statistik kehadiran pemilih"})
		return
	}

	percentage := 0.0
	if total > 0 {
		percentage = (float64(voted) / float64(total)) * 100.0
	}

	c.JSON(http.StatusOK, gin.H{
		"totalVoters": total,
		"votedCount":  voted,
		"notVoted":    total - voted,
		"percentage":  percentage,
		"timestamp":   time.Now().Format(time.RFC3339),
	})
}

// GetTallyResults membongkar kotak suara dan menghitung rekapitulasi perolehan suara.
// DILINDUNGI TIME-LOCK & DUAL-CONTROL QUORUM SECRET (KPU + BAWASLU)
func (h *AdminHandler) GetTallyResults(c *gin.Context) {
	// 1. Verifikasi Batas Waktu Pemilihan (Time-Lock)
	electionCfg, err := h.auditRepo.GetElectionConfig()
	if err == nil && electionCfg != nil {
		if time.Now().Before(electionCfg.EndTime) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":     "QUARANTINED: Rekapitulasi suara masih dikarantina. Kotak suara baru dapat dibuka setelah waktu pemilihan resmi berakhir (" + electionCfg.EndTime.Format(time.RFC3339) + ").",
				"timeValid": false,
			})
			return
		}
	}

	// 2. Verifikasi Dual-Control Quorum Secret
	kpuSecret := c.GetHeader("x-kpu-secret-key")
	bawasluSecret := c.GetHeader("x-bawaslu-secret-key")

	if kpuSecret == "" || bawasluSecret == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Diperlukan otorisasi Quorum Ganda (Header 'x-kpu-secret-key' dan 'x-bawaslu-secret-key' wajib disertakan).",
		})
		return
	}

	if kpuSecret != h.cfg.KPUMasterSecret || bawasluSecret != h.cfg.BawasluAuditSecret {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Kunci Quorum KPU atau Bawaslu tidak cocok.",
		})
		return
	}

	// 3. Eksekusi Rekapitulasi Tally
	results, err := h.ballotRepo.GetTallyResults()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal melakukan tally suara: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"status":    "QUORUM_UNLOCKED",
		"tally":     results,
		"unlockedAt": time.Now().Format(time.RFC3339),
	})
}

// GetAuditLogs mengembalikan riwayat jejak audit aktivitas panitia
func (h *AdminHandler) GetAuditLogs(c *gin.Context) {
	logs, err := h.auditRepo.GetRecentLogs(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil log audit"})
		return
	}
	c.JSON(http.StatusOK, logs)
}
