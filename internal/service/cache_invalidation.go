package service

import (
	"context"
	"log"

	"pemira-backend/pkg/redis"
)

// Cache key patterns used across handlers/services. Keep these in one place so
// every mutation path invalidates the same set of keys.
const (
	ElectionConfigCacheKey  = "cache:election_config"
	EligibleChairsCacheGlob = "cache:eligible_chairs:*"
	BoothChairsCacheGlob    = "cache:booth:chairs:*"
	AllChairsCacheKey       = "cache:all_chairs"
)

// InvalidateElectionConfigCache menghapus cache konfigurasi pemilu dan seluruh
// cache kandidat/kursi yang bergantung pada konfigurasi tersebut.
func InvalidateElectionConfigCache(ctx context.Context, redisService *redis.RedisService) {
	if redisService == nil || !redisService.IsAvailable() {
		return
	}

	if err := redisService.Delete(ctx, ElectionConfigCacheKey); err != nil {
		log.Printf("cache invalidation: failed to delete %s: %v", ElectionConfigCacheKey, err)
	}
	InvalidateCandidateCaches(ctx, redisService)
}

// InvalidateCandidateCaches menghapus cache kandidat untuk semua jurusan.
// Dipakai setelah admin menambah/mengubah kandidat atau kursi.
func InvalidateCandidateCaches(ctx context.Context, redisService *redis.RedisService) {
	if redisService == nil || !redisService.IsAvailable() {
		return
	}

	for _, pattern := range []string{EligibleChairsCacheGlob, BoothChairsCacheGlob} {
		if err := redisService.DeleteByPattern(ctx, pattern); err != nil {
			log.Printf("cache invalidation: failed to delete pattern %s: %v", pattern, err)
		}
	}
	if err := redisService.Delete(ctx, AllChairsCacheKey); err != nil {
		log.Printf("cache invalidation: failed to delete %s: %v", AllChairsCacheKey, err)
	}
}

// InvalidateAllElectionCaches menghapus seluruh cache baca-yang-berat.
func InvalidateAllElectionCaches(ctx context.Context, redisService *redis.RedisService) {
	InvalidateElectionConfigCache(ctx, redisService)
}
