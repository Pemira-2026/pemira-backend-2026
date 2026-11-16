package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var (
	ErrRedisUnavailable = errors.New("redis service is unavailable")
)

type RedisService struct {
	Client *goredis.Client
}

func NewRedisService(host, port, password string, db int, required bool) (*RedisService, error) {
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
		if required {
			return nil, fmt.Errorf("failed to connect to required Redis server at %s: %w", addr, err)
		}
		// In optional/development mode, return non-nil struct with nil Client and return warning error
		return &RedisService{Client: nil}, fmt.Errorf("redis connection failed (running in fallback mode): %w", err)
	}

	return &RedisService{Client: client}, nil
}

// IsAvailable returns true if Redis client is connected and active
func (r *RedisService) IsAvailable() bool {
	return r != nil && r.Client != nil
}

// Ping checks if Redis is responsive
func (r *RedisService) Ping(ctx context.Context) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}
	return r.Client.Ping(ctx).Err()
}

// AcquireLock menggunakan SETNX untuk mencegah race condition double-voting
func (r *RedisService) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if r == nil || r.Client == nil {
		return false, ErrRedisUnavailable
	}
	return r.Client.SetNX(ctx, key, "locked", ttl).Result()
}

// AcquireLockWithToken mengunci dengan token kepemilikan unik
func (r *RedisService) AcquireLockWithToken(ctx context.Context, key string, token string, ttl time.Duration) (bool, error) {
	if r == nil || r.Client == nil {
		return false, ErrRedisUnavailable
	}
	return r.Client.SetNX(ctx, key, token, ttl).Result()
}

// ReleaseLock menghapus kunci setelah transaksi selesai
func (r *RedisService) ReleaseLock(ctx context.Context, key string) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}
	return r.Client.Del(ctx, key).Err()
}

// ReleaseLockWithToken melepaskan kunci secara atomik hanya jika token kepemilikan cocok (menggunakan Lua script)
func (r *RedisService) ReleaseLockWithToken(ctx context.Context, key string, token string) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}
	luaRelease := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	return r.Client.Eval(ctx, luaRelease, []string{key}, token).Err()
}

// SetWithTTL menyimpan nilai sementara (misal: OAuth State acak)
func (r *RedisService) SetWithTTL(ctx context.Context, key string, value string, ttl time.Duration) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}
	return r.Client.Set(ctx, key, value, ttl).Err()
}

// Get mengambil nilai dari Redis
func (r *RedisService) Get(ctx context.Context, key string) (string, error) {
	if r == nil || r.Client == nil {
		return "", ErrRedisUnavailable
	}
	return r.Client.Get(ctx, key).Result()
}

// Delete menghapus satu cache key.
func (r *RedisService) Delete(ctx context.Context, key string) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}
	return r.Client.Del(ctx, key).Err()
}

// DeleteByPattern menghapus seluruh key yang cocok dengan pola menggunakan SCAN
// (bukan KEYS) agar tidak memblokir Redis saat jumlah key besar.
func (r *RedisService) DeleteByPattern(ctx context.Context, pattern string) error {
	if r == nil || r.Client == nil {
		return ErrRedisUnavailable
	}

	var cursor uint64
	for {
		keys, next, err := r.Client.Scan(ctx, cursor, pattern, 200).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.Client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

// IncrWithExpire menaikkan counter dan memasang TTL secara atomik.
// Dipakai oleh rate limiter terdistribusi.
func (r *RedisService) IncrWithExpire(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	if r == nil || r.Client == nil {
		return 0, ErrRedisUnavailable
	}
	ms := ttl.Milliseconds()
	if ms <= 0 {
		ms = 1000
	}
	luaIncr := `
		local current = redis.call("INCR", KEYS[1])
		if current == 1 then
			redis.call("PEXPIRE", KEYS[1], ARGV[1])
		end
		return current
	`
	return r.Client.Eval(ctx, luaIncr, []string{key}, ms).Int64()
}

// Close menutup koneksi Redis (untuk graceful shutdown / test cleanup).
func (r *RedisService) Close() error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Close()
}
