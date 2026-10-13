package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"pemira-backend/internal/repository"
	"pemira-backend/pkg/redis"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTClaims struct {
	VoterID string `json:"voterId"`
	NIM     string `json:"nim"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Major   string `json:"major"`
	jwt.RegisteredClaims
}

type AuthService interface {
	GenerateOAuthState(ctx context.Context) (string, error)
	ValidateOAuthState(ctx context.Context, state string) (bool, error)
	GenerateJWT(voter *domain.Voter) (string, error)
	ValidateJWT(tokenStr string) (*JWTClaims, error)
	GetVoterByEmail(email string) (*domain.Voter, error)
	CreateMockVoterIfNotExist(email, nim, name, major string) (*domain.Voter, error)
}

type authService struct {
	voterRepo    repository.VoterRepository
	redisService *redis.RedisService
	cfg          *config.Config
}

func NewAuthService(
	voterRepo repository.VoterRepository,
	redisService *redis.RedisService,
	cfg *config.Config,
) AuthService {
	return &authService{
		voterRepo:    voterRepo,
		redisService: redisService,
		cfg:          cfg,
	}
}

// GenerateOAuthState membuat token state acak berumur 5 menit di Redis untuk mencegah OAuth Login CSRF
func (s *authService) GenerateOAuthState(ctx context.Context) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := hex.EncodeToString(b)

	err := s.redisService.SetWithTTL(ctx, "oauth:state:"+state, "valid", 5*time.Minute)
	if err != nil {
		return state, nil // Graceful jika Redis dev belum jalan
	}

	return state, nil
}

// ValidateOAuthState memvalidasi dan langsung menghapus state agar tidak bisa di-replay
func (s *authService) ValidateOAuthState(ctx context.Context, state string) (bool, error) {
	val, err := s.redisService.Get(ctx, "oauth:state:"+state)
	if err != nil || val == "" {
		return false, errors.New("invalid or expired oauth state")
	}
	_ = s.redisService.ReleaseLock(ctx, "oauth:state:"+state)
	return true, nil
}

func (s *authService) GenerateJWT(voter *domain.Voter) (string, error) {
	claims := JWTClaims{
		VoterID: voter.ID,
		NIM:     voter.NIM,
		Email:   voter.Email,
		Role:    voter.Role,
		Major:   voter.Major,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(12 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "pemira-2026-auth",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecret))
}

func (s *authService) ValidateJWT(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(s.cfg.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid jwt token")
	}

	return claims, nil
}

func (s *authService) GetVoterByEmail(email string) (*domain.Voter, error) {
	return s.voterRepo.FindByEmail(email)
}

func (s *authService) CreateMockVoterIfNotExist(email, nim, name, major string) (*domain.Voter, error) {
	voter, err := s.voterRepo.FindByEmail(email)
	if err == nil && voter != nil {
		return voter, nil
	}

	newVoter := domain.Voter{
		ID:         "voter-" + hex.EncodeToString([]byte(nim)),
		NIM:        nim,
		FullName:   name,
		Email:      email,
		Faculty:    "Teknik",
		Major:      major,
		Generation: 2023,
		Role:       "voter",
		HasVoted:   false,
	}

	if err := s.voterRepo.Create(&newVoter); err != nil {
		return nil, err
	}
	return &newVoter, nil
}
