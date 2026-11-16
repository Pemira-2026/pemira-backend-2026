package handler

import (
	"errors"
	"net/http"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/service"

	"github.com/gin-gonic/gin"
)

type VoteHandler struct {
	voteService service.VoteService
}

func NewVoteHandler(voteService service.VoteService) *VoteHandler {
	return &VoteHandler{voteService: voteService}
}

// SubmitVote menangani transaksi pengiriman surat suara (Zero-Linkability)
func (h *VoteHandler) SubmitVote(c *gin.Context) {
	voterID, exists := c.Get("voterId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized voter session"})
		return
	}

	var req domain.SubmitVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid vote payload: " + err.Error()})
		return
	}

	res, err := h.voteService.SubmitVote(c.Request.Context(), voterID.(string), req)
	if err != nil {
		if errors.Is(err, service.ErrTurnstileInvalid) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrAlreadyVoted) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrElectionClosed) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrConcurrentConflict) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrInvalidCandidate) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memproses suara: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, res)
}

// VerifyReceipt memverifikasi token tanda terima suara pemilih secara publik
func (h *VoteHandler) VerifyReceipt(c *gin.Context) {
	receiptCode := c.Param("code")
	if receiptCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Receipt code is required"})
		return
	}

	valid, err := h.voteService.GetReceiptVerification(receiptCode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !valid {
		c.JSON(http.StatusNotFound, gin.H{
			"valid":   false,
			"receipt": receiptCode,
			"error":   "Tanda terima tidak valid atau tidak terdaftar",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"receipt": receiptCode,
		"status":  "Suara telah terverifikasi tersimpan di dalam kotak suara digital.",
	})
}
