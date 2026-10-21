package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type RedisService struct {
	Client *goredis.Client
}

func NewRedisService(host, port, password string, db int) (*RedisService, error) {
	addr := fmt.Sprintf("%s:%s", host, port)
	client := goredis.NewClient(&goredis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		PoolSize: 100, // connection pool untuk concurrency tinggi
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		// Log warning jika redis tidak aktif saat testing lokal, set client = nil agar fallback otomatis jalan
		return &RedisService{Client: nil}, err
	}

	return &RedisService{Client: client}, nil
}

// AcquireLock menggunakan SETNX untuk mencegah race condition double-voting
func (r *RedisService) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if r == nil || r.Client == nil {
		return true, nil // Fallback jika redis belum di-setup / offline
	}
	success, err := r.Client.SetNX(ctx, key, "locked", ttl).Result()
	if err != nil {
		// Jika redis error koneksi di tengah jalan, fallback ke DB-level lock
		return true, nil
	}
	return success, nil
}

// ReleaseLock menghapus kunci setelah transaksi selesai
func (r *RedisService) ReleaseLock(ctx context.Context, key string) error {
	if r.Client == nil {
		return nil
	}
	return r.Client.Del(ctx, key).Err()
}

// SetWithTTL menyimpan nilai sementara (misal: OAuth State acak)
func (r *RedisService) SetWithTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	if r.Client == nil {
		return nil
	}
	return r.Client.Set(ctx, key, value, ttl).Err()
}

// Get mengambil nilai dari Redis
func (r *RedisService) Get(ctx context.Context, key string) (string, error) {
	if r.Client == nil {
		return "", nil
	}
	return r.Client.Get(ctx, key).Result()
}
