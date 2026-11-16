package handler

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"pemira-backend/pkg/redis"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	voterRepo    repository.VoterRepository
	ballotRepo   repository.BallotRepository
	auditRepo    repository.AuditRepository
	redisService *redis.RedisService
	cfg          *config.Config
}

func NewAdminHandler(
	voterRepo repository.VoterRepository,
	ballotRepo repository.BallotRepository,
	auditRepo repository.AuditRepository,
	redisService *redis.RedisService,
	cfg *config.Config,
) *AdminHandler {
	return &AdminHandler{
		voterRepo:    voterRepo,
		ballotRepo:   ballotRepo,
		auditRepo:    auditRepo,
		redisService: redisService,
		cfg:          cfg,
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
// DILINDUNGI FAIL-CLOSED TIME-LOCK & DUAL-CONTROL QUORUM SECRET (KPU + BAWASLU)
func (h *AdminHandler) GetTallyResults(c *gin.Context) {
	actorID := "anonymous"
	actorRole := "unknown"
	if claims, exists := c.Get("claims"); exists {
		if jwtClaims, ok := claims.(*service.JWTClaims); ok {
			actorID = jwtClaims.VoterID
			actorRole = jwtClaims.Role
		}
	}

	// 1. Verifikasi Batas Waktu Pemilihan (FAIL-CLOSED Time-Lock)
	electionCfg, err := h.auditRepo.GetElectionConfig()
	if err != nil || electionCfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Gagal membaca konfigurasi jendela pemilu. Rekapitulasi suara dikunci demi keamanan.",
		})
		return
	}
	if time.Now().Before(electionCfg.EndTime) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":     "QUARANTINED: Rekapitulasi suara masih dikarantina. Kotak suara baru dapat dibuka setelah waktu pemilihan resmi berakhir (" + electionCfg.EndTime.Format(time.RFC3339) + ").",
			"timeValid": false,
		})
		return
	}

	// 2. Verifikasi Dual-Control Quorum Secret menggunakan Constant-Time Comparison
	kpuSecret := c.GetHeader("x-kpu-secret-key")
	bawasluSecret := c.GetHeader("x-bawaslu-secret-key")

	if kpuSecret == "" || bawasluSecret == "" {
		_ = h.auditRepo.LogAction(&domain.AuditLog{
			ActorID:   actorID,
			ActorRole: actorRole,
			Action:    "TALLY_QUORUM_FAILED",
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Payload:   `{"reason":"missing quorum headers"}`,
			CreatedAt: time.Now(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Diperlukan otorisasi Quorum Ganda (Header 'x-kpu-secret-key' dan 'x-bawaslu-secret-key' wajib disertakan).",
		})
		return
	}

	kpuMatch := subtle.ConstantTimeCompare([]byte(kpuSecret), []byte(h.cfg.KPUMasterSecret)) == 1
	bawasluMatch := subtle.ConstantTimeCompare([]byte(bawasluSecret), []byte(h.cfg.BawasluAuditSecret)) == 1

	if !kpuMatch || !bawasluMatch {
		_ = h.auditRepo.LogAction(&domain.AuditLog{
			ActorID:   actorID,
			ActorRole: actorRole,
			Action:    "TALLY_QUORUM_FAILED",
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
			Payload:   `{"reason":"quorum key mismatch"}`,
			CreatedAt: time.Now(),
		})
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

	// 4. Hitung Percentage per ChairID
	chairTotals := make(map[uint]int64)
	for _, r := range results {
		chairTotals[r.ChairID] += r.TotalVotes
	}

	for i := range results {
		tot := chairTotals[results[i].ChairID]
		if tot > 0 {
			results[i].Percentage = math.Round((float64(results[i].TotalVotes)/float64(tot))*10000) / 100
		} else {
			results[i].Percentage = 0.0
		}
	}

	// 5. Catat Audit Log Pembukaan Sukses
	_ = h.auditRepo.LogAction(&domain.AuditLog{
		ActorID:   actorID,
		ActorRole: actorRole,
		Action:    "TALLY_UNLOCKED",
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Payload:   fmt.Sprintf(`{"chairs_count":%d,"total_results":%d}`, len(chairTotals), len(results)),
		CreatedAt: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"status":     "QUORUM_UNLOCKED",
		"tally":      results,
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

// GetElectionConfig mengembalikan konfigurasi jadwal pemilu untuk panel admin.
func (h *AdminHandler) GetElectionConfig(c *gin.Context) {
	cfg, err := h.auditRepo.GetElectionConfig()
	if err != nil || cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca konfigurasi pemilu"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateElectionConfig memperbarui jadwal pemilu dan emergency pause.
// Hanya role admin yang boleh memanggil endpoint ini (dipasang di router).
// Setelah update, seluruh cache konfigurasi & kandidat diinvalidasi agar
// perubahan langsung terlihat oleh bilik suara.
func (h *AdminHandler) UpdateElectionConfig(c *gin.Context) {
	var req struct {
		Title             *string `json:"title"`
		StartTimeRFC3339  *string `json:"startTime"`
		EndTimeRFC3339    *string `json:"endTime"`
		IsEmergencyPaused *bool   `json:"isEmergencyPaused"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payload tidak valid: " + err.Error()})
		return
	}

	cfg, err := h.auditRepo.GetElectionConfig()
	if err != nil || cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membaca ElectionConfig dari database"})
		return
	}

	changed := make([]string, 0, 4)
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Title tidak boleh kosong"})
			return
		}
		cfg.Title = title
		changed = append(changed, "title")
	}
	if req.StartTimeRFC3339 != nil && strings.TrimSpace(*req.StartTimeRFC3339) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.StartTimeRFC3339))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Format startTime tidak valid (gunakan RFC3339): " + err.Error()})
			return
		}
		cfg.StartTime = t
		changed = append(changed, "startTime")
	}
	if req.EndTimeRFC3339 != nil && strings.TrimSpace(*req.EndTimeRFC3339) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.EndTimeRFC3339))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Format endTime tidak valid (gunakan RFC3339): " + err.Error()})
			return
		}
		cfg.EndTime = t
		changed = append(changed, "endTime")
	}
	if req.IsEmergencyPaused != nil {
		cfg.IsEmergencyPaused = *req.IsEmergencyPaused
		changed = append(changed, "isEmergencyPaused")
	}

	if len(changed) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tidak ada perubahan yang dikirim"})
		return
	}
	if !cfg.StartTime.Before(cfg.EndTime) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "startTime harus lebih awal daripada endTime"})
		return
	}

	cfg.UpdatedAt = time.Now()
	if err := h.auditRepo.UpdateElectionConfig(cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan perubahan ElectionConfig: " + err.Error()})
		return
	}

	service.InvalidateElectionConfigCache(c.Request.Context(), h.redisService)

	actorID, actorRole := actorFromContext(c)
	_ = h.auditRepo.LogAction(&domain.AuditLog{
		ActorID:   actorID,
		ActorRole: actorRole,
		Action:    "CONFIG_CHANGE",
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Payload:   fmt.Sprintf(`{"changed":%q}`, strings.Join(changed, ",")),
		CreatedAt: time.Now(),
	})

	c.JSON(http.StatusOK, cfg)
}

// InvalidateCache menghapus cache konfigurasi/kandidat secara manual setelah
// perubahan data dilakukan di luar API (misalnya seeding terkontrol).
func (h *AdminHandler) InvalidateCache(c *gin.Context) {
	service.InvalidateAllElectionCaches(c.Request.Context(), h.redisService)

	actorID, actorRole := actorFromContext(c)
	_ = h.auditRepo.LogAction(&domain.AuditLog{
		ActorID:   actorID,
		ActorRole: actorRole,
		Action:    "CACHE_INVALIDATE",
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		CreatedAt: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Cache konfigurasi & kandidat berhasil diinvalidasi"})
}

func actorFromContext(c *gin.Context) (string, string) {
	if claims, exists := c.Get("claims"); exists {
		if jwtClaims, ok := claims.(*service.JWTClaims); ok {
			return jwtClaims.VoterID, jwtClaims.Role
		}
	}
	return "unknown", "unknown"
}
