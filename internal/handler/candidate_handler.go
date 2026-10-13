package handler

import (
	"net/http"
	"pemira-backend/internal/repository"

	"github.com/gin-gonic/gin"
)

type CandidateHandler struct {
	chairRepo repository.ChairRepository
	voterRepo repository.VoterRepository
}

func NewCandidateHandler(chairRepo repository.ChairRepository, voterRepo repository.VoterRepository) *CandidateHandler {
	return &CandidateHandler{
		chairRepo: chairRepo,
		voterRepo: voterRepo,
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

	chairs, err := h.chairRepo.FindEligibleForMajor(voter.Major)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data kandidat: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, chairs)
}

// GetAllChairs mengembalikan seluruh kursi dan paslon untuk keperluan informasi publik
func (h *CandidateHandler) GetAllChairs(c *gin.Context) {
	chairs, err := h.chairRepo.FindAllActive()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal mengambil data kursi: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, chairs)
}
