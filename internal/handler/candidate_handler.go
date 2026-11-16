package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/pkg/redis"
	"time"

	"github.com/gin-gonic/gin"
)

type CandidateHandler struct {
	chairRepo    repository.ChairRepository
	voterRepo    repository.VoterRepository
	redisService *redis.RedisService
}

func NewCandidateHandler(chairRepo repository.ChairRepository, voterRepo repository.VoterRepository, redisService *redis.RedisService) *CandidateHandler {
	return &CandidateHandler{
		chairRepo:    chairRepo,
		voterRepo:    voterRepo,
		redisService: redisService,
	}
}

// GetBoothCandidates mengembalikan daftar kursi dan paslon yang berhak dipilih mahasiswa sesuai jurusannya
func (h *CandidateHandler) GetBoothCandidates(c *gin.Context) {
	voterID, exists := c.Get("voterId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	voter, err := h.voterRepo.FindByID(voterID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pemilih tidak ditemukan di DPT"})
		return
	}

	cacheKey := fmt.Sprintf("cache:booth:chairs:%s", voter.Major)
	if h.redisService != nil && h.redisService.IsAvailable() {
		val, err := h.redisService.Get(c.Request.Context(), cacheKey)
		if err == nil && val != "" {
			var cachedChairs []domain.Chair
			if err := json.Unmarshal([]byte(val), &cachedChairs); err == nil {
				c.JSON(http.StatusOK, cachedChairs)
				return
			}
		}
	}

	chairs, err := h.chairRepo.FindEligibleForMajor(voter.Major)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data kandidat: " + err.Error()})
		return
	}

	if h.redisService != nil && h.redisService.IsAvailable() {
		if bytes, err := json.Marshal(chairs); err == nil {
			_ = h.redisService.SetWithTTL(c.Request.Context(), cacheKey, string(bytes), 30*time.Second)
		}
	}

	c.JSON(http.StatusOK, chairs)
}

// GetAllChairs mengembalikan seluruh kursi dan paslon untuk keperluan informasi publik
func (h *CandidateHandler) GetAllChairs(c *gin.Context) {
	cacheKey := "cache:all_chairs"
	if h.redisService != nil && h.redisService.IsAvailable() {
		val, err := h.redisService.Get(c.Request.Context(), cacheKey)
		if err == nil && val != "" {
			var cachedChairs []domain.Chair
			if err := json.Unmarshal([]byte(val), &cachedChairs); err == nil {
				c.JSON(http.StatusOK, cachedChairs)
				return
			}
		}
	}

	chairs, err := h.chairRepo.FindAllActive()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data kursi: " + err.Error()})
		return
	}

	if h.redisService != nil && h.redisService.IsAvailable() {
		if bytes, err := json.Marshal(chairs); err == nil {
			_ = h.redisService.SetWithTTL(c.Request.Context(), cacheKey, string(bytes), 30*time.Second)
		}
	}

	c.JSON(http.StatusOK, chairs)
}
