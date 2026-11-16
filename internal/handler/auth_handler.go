package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type AuthHandler struct {
	authService service.AuthService
	voterRepo   repository.VoterRepository
	chairRepo   repository.ChairRepository
	oauthConfig *oauth2.Config
	cfg         *config.Config
}

func NewAuthHandler(
	authService service.AuthService,
	voterRepo repository.VoterRepository,
	chairRepo repository.ChairRepository,
	cfg *config.Config,
) *AuthHandler {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	return &AuthHandler{
		authService: authService,
		voterRepo:   voterRepo,
		chairRepo:   chairRepo,
		oauthConfig: oauthConfig,
		cfg:         cfg,
	}
}

// GoogleLogin menginisialisasi alur OAuth 2.0 dengan token State dinamis anti-CSRF
func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	state, err := h.authService.GenerateOAuthState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate security state"})
		return
	}

	url := h.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// GoogleCallback menangani pertukaran authorization code Google OAuth
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	state := c.Query("state")
	valid, err := h.authService.ValidateOAuthState(c.Request.Context(), state)
	if !valid || err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired OAuth CSRF state"})
		return
	}

	code := c.Query("code")
	token, err := h.oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to exchange auth token: " + err.Error()})
		return
	}

	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user profile from Google"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read Google profile response"})
		return
	}

	var googleUser struct {
		Email         string `json:"email"`
		Name          string `json:"name"`
		VerifiedEmail bool   `json:"verified_email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(body, &googleUser); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Google profile payload"})
		return
	}

	if !googleUser.VerifiedEmail && !googleUser.EmailVerified {
		c.JSON(http.StatusForbidden, gin.H{"error": "Akses ditolak: Email Google belum diverifikasi"})
		return
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(googleUser.Email))

	// Validasi Domain Kampus Resmi
	if h.cfg.CampusEmailDomain != "" {
		allowedDomain := strings.ToLower(strings.TrimSpace(h.cfg.CampusEmailDomain))
		if !strings.HasSuffix(normalizedEmail, "@"+allowedDomain) {
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("Akses ditolak: Gunakan email resmi kampus @%s", h.cfg.CampusEmailDomain)})
			return
		}
	}

	// Cek apakah mahasiswa terdaftar di DPT
	voter, err := h.voterRepo.FindByEmail(normalizedEmail)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/not-registered", h.cfg.VoterFrontendURL))
		return
	}

	jwtToken, err := h.authService.GenerateJWT(voter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to issue JWT token"})
		return
	}

	// Redirect kembali ke Voter Frontend dengan JWT
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/auth/callback?token=%s", h.cfg.VoterFrontendURL, jwtToken))
}

// DevMockLogin endpoint praktis untuk pengujian lokal & simulasi akun tanpa Google OAuth
func (h *AuthHandler) DevMockLogin(c *gin.Context) {
	if h.cfg.ServerEnv == "production" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Dev mock login is disabled in production"})
		return
	}

	var req struct {
		Email string `json:"email" binding:"required"`
		NIM   string `json:"nim"`
		Name  string `json:"name"`
		Major string `json:"major"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.NIM == "" {
		req.NIM = "13523001"
	}
	if req.Name == "" {
		req.Name = "Mahasiswa Uji Coba"
	}
	if req.Major == "" {
		req.Major = "Teknik Informatika dan Komputer"
	}

	voter, err := h.authService.CreateMockVoterIfNotExist(req.Email, req.NIM, req.Name, req.Major)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	jwtToken, err := h.authService.GenerateJWT(voter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": jwtToken,
		"voter": voter,
	})
}

// GetMe mengembalikan profil lengkap pemilih beserta daftar kursi yang berhak dipilih
func (h *AuthHandler) GetMe(c *gin.Context) {
	voterID, exists := c.Get("voterId")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	voter, err := h.voterRepo.FindByID(voterID.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Voter not found in DPT"})
		return
	}

	eligibleChairs, _ := h.chairRepo.FindEligibleForMajor(voter.Major)
	var chairSummaries []domain.ChairSummary
	for _, c := range eligibleChairs {
		chairSummaries = append(chairSummaries, domain.ChairSummary{
			ID:   c.ID,
			Name: c.Name,
		})
	}

	resp := domain.VoterProfileResponse{
		ID:             voter.ID,
		NIM:            voter.NIM,
		FullName:       voter.FullName,
		Email:          voter.Email,
		Faculty:        voter.Faculty,
		Major:          voter.Major,
		Generation:     voter.Generation,
		Role:           voter.Role,
		HasVoted:       voter.HasVoted,
		EligibleChairs: chairSummaries,
	}

	c.JSON(http.StatusOK, resp)
}
