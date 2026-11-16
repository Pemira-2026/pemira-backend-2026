package redis

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRedisServiceNilClientHandling(t *testing.T) {
	service := &RedisService{Client: nil}

	if service.IsAvailable() {
		t.Errorf("expected IsAvailable to be false for nil client")
	}

	ctx := context.Background()

	// Ping
	if err := service.Ping(ctx); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected ErrRedisUnavailable from Ping, got %v", err)
	}

	// AcquireLock
	acquired, err := service.AcquireLock(ctx, "test-key", 5*time.Second)
	if acquired || !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected (false, ErrRedisUnavailable) from AcquireLock, got (%v, %v)", acquired, err)
	}

	// ReleaseLock
	if err := service.ReleaseLock(ctx, "test-key"); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected ErrRedisUnavailable from ReleaseLock, got %v", err)
	}

	// SetWithTTL
	if err := service.SetWithTTL(ctx, "test-key", "value", 5*time.Second); !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected ErrRedisUnavailable from SetWithTTL, got %v", err)
	}

	// Get
	val, err := service.Get(ctx, "test-key")
	if val != "" || !errors.Is(err, ErrRedisUnavailable) {
		t.Errorf("expected ('', ErrRedisUnavailable) from Get, got ('%s', %v)", val, err)
	}
}

func TestNewRedisServiceRequiredModeFailsFast(t *testing.T) {
	// Trying to connect to a non-existent port in required mode
	svc, err := NewRedisService("127.0.0.1", "59999", "", 0, true)
	if err == nil {
		t.Fatalf("expected error connecting to non-existent redis in required mode, got nil")
	}
	if svc != nil {
		t.Errorf("expected nil service when required connection fails, got %v", svc)
	}
}

func TestNewRedisServiceOptionalModeFallback(t *testing.T) {
	// Trying to connect to a non-existent port in optional mode
	svc, err := NewRedisService("127.0.0.1", "59999", "", 0, false)
	if err == nil {
		t.Fatalf("expected warning error connecting to non-existent redis in optional mode, got nil")
	}
	if svc == nil {
		t.Fatalf("expected non-nil service struct in optional fallback mode")
	}
	if svc.IsAvailable() {
		t.Errorf("expected IsAvailable to be false for fallback service")
	}
}
