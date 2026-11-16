package service

import (
	"context"
	"testing"

	"pemira-backend/internal/config"
	"pemira-backend/pkg/redis"
)

func TestStatelessOAuthStateReplayRejected(t *testing.T) {
	cfg := &config.Config{
		JWTSecret: "test-jwt-secret-for-stateless-oauth-state-32chars!",
	}
	svc := NewAuthService(nil, &redis.RedisService{Client: nil}, cfg)
	ctx := context.Background()

	state, err := svc.GenerateOAuthState(ctx)
	if err != nil {
		t.Fatalf("failed to generate state: %v", err)
	}

	valid, err := svc.ValidateOAuthState(ctx, state)
	if err != nil || !valid {
		t.Fatalf("first validation should succeed, got (%v, %v)", valid, err)
	}

	valid, err = svc.ValidateOAuthState(ctx, state)
	if valid || err == nil {
		t.Fatalf("replayed state must be rejected, got (%v, %v)", valid, err)
	}
}
