package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigDevelopmentValid(t *testing.T) {
	// Set minimal dev env
	os.Clearenv()
	os.Setenv("SERVER_ENV", "development")
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_NAME", ":memory:")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected valid dev config, got error: %v", err)
	}

	if cfg.ServerEnv != "development" {
		t.Errorf("expected ServerEnv to be development, got %s", cfg.ServerEnv)
	}
	if !cfg.DBAutoMigrate {
		t.Errorf("expected DBAutoMigrate to be true by default in development")
	}
	if !cfg.SeedDevData {
		t.Errorf("expected SeedDevData to be true by default in development")
	}
	if cfg.RedisRequired {
		t.Errorf("expected RedisRequired to be false by default in development")
	}
	// Verify dev defaults applied
	if cfg.JWTSecret == "" || cfg.ReceiptSecretKey == "" {
		t.Errorf("expected dev default secrets to be populated")
	}
}

func TestConfigInvalidServerEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("SERVER_ENV", "invalid_env")
	os.Setenv("DB_DRIVER", "sqlite")
	os.Setenv("DB_NAME", ":memory:")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected error for invalid SERVER_ENV, got nil")
	}
	if !strings.Contains(err.Error(), "invalid SERVER_ENV") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestConfigProductionInsecureDefaultsRejected(t *testing.T) {
	os.Clearenv()
	os.Setenv("SERVER_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	os.Setenv("VOTER_FRONTEND_URL", "https://pemira.univ.ac.id")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin-pemira.univ.ac.id")
	os.Setenv("REDIS_HOST", "redis.prod.internal")
	os.Setenv("REDIS_PORT", "6379")

	// Set default insecure secrets
	os.Setenv("JWT_SECRET", "super-secret-jwt-signing-key-pemira-2026-default")
	os.Setenv("RECEIPT_SECRET_KEY", "super-secret-hmac-sha256-receipt-key-pemira-2026")
	os.Setenv("RECEIPT_SALT", "random-salt-salt-salt-2026")
	os.Setenv("KPU_MASTER_SECRET", "kpu-master-secret-key-2026")
	os.Setenv("BAWASLU_AUDIT_SECRET", "bawaslu-audit-secret-key-2026")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected production config with default secrets to be rejected, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "JWT_SECRET is using an insecure default") {
		t.Errorf("expected JWT_SECRET rejection in: %s", errStr)
	}
	if !strings.Contains(errStr, "RECEIPT_SECRET_KEY is using an insecure default") {
		t.Errorf("expected RECEIPT_SECRET_KEY rejection in: %s", errStr)
	}
	if !strings.Contains(errStr, "RECEIPT_SALT is using an insecure default") {
		t.Errorf("expected RECEIPT_SALT rejection in: %s", errStr)
	}
	if !strings.Contains(errStr, "KPU_MASTER_SECRET is using an insecure default") {
		t.Errorf("expected KPU_MASTER_SECRET rejection in: %s", errStr)
	}
	if !strings.Contains(errStr, "BAWASLU_AUDIT_SECRET is using an insecure default") {
		t.Errorf("expected BAWASLU_AUDIT_SECRET rejection in: %s", errStr)
	}
}

func TestConfigProductionShortSecretsRejected(t *testing.T) {
	os.Clearenv()
	os.Setenv("SERVER_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	os.Setenv("VOTER_FRONTEND_URL", "https://pemira.univ.ac.id")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin-pemira.univ.ac.id")
	os.Setenv("REDIS_HOST", "redis.prod.internal")
	os.Setenv("REDIS_PORT", "6379")

	// Short secrets
	os.Setenv("JWT_SECRET", "too-short-jwt")
	os.Setenv("RECEIPT_SECRET_KEY", "too-short-receipt")
	os.Setenv("RECEIPT_SALT", "short-salt")
	os.Setenv("KPU_MASTER_SECRET", "short-kpu")
	os.Setenv("BAWASLU_AUDIT_SECRET", "short-bawaslu")

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected production config with short secrets to be rejected, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "JWT_SECRET must be at least 32 characters") {
		t.Errorf("expected JWT_SECRET length error in: %s", errStr)
	}
	if !strings.Contains(errStr, "RECEIPT_SECRET_KEY must be at least 32 characters") {
		t.Errorf("expected RECEIPT_SECRET_KEY length error in: %s", errStr)
	}
}

func TestConfigProductionIdenticalQuorumSecretsRejected(t *testing.T) {
	os.Clearenv()
	os.Setenv("SERVER_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	os.Setenv("VOTER_FRONTEND_URL", "https://pemira.univ.ac.id")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin-pemira.univ.ac.id")
	os.Setenv("REDIS_HOST", "redis.prod.internal")
	os.Setenv("REDIS_PORT", "6379")

	secureKey := "a-very-strong-and-unique-random-secret-key-32chars!"
	os.Setenv("JWT_SECRET", "jwt-prod-custom-secure-signing-key-32-chars-long!")
	os.Setenv("RECEIPT_SECRET_KEY", "rcpt-prod-custom-secure-signing-key-32-chars-long!")
	os.Setenv("RECEIPT_SALT", "custom-salt-16-chars-long!")

	// Same secret for both KPU and Bawaslu
	os.Setenv("KPU_MASTER_SECRET", secureKey)
	os.Setenv("BAWASLU_AUDIT_SECRET", secureKey)

	_, err := LoadConfig()
	if err == nil {
		t.Fatalf("expected identical quorum secrets to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "KPU_MASTER_SECRET and BAWASLU_AUDIT_SECRET must not be identical") {
		t.Errorf("expected identical quorum error in: %v", err)
	}
}

func TestConfigProductionValidConfigPasses(t *testing.T) {
	os.Clearenv()
	os.Setenv("SERVER_ENV", "production")
	os.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	os.Setenv("VOTER_FRONTEND_URL", "https://pemira.univ.ac.id")
	os.Setenv("ADMIN_FRONTEND_URL", "https://admin-pemira.univ.ac.id")
	os.Setenv("REDIS_HOST", "redis.prod.internal")
	os.Setenv("REDIS_PORT", "6379")

	os.Setenv("JWT_SECRET", "jwt-prod-custom-secure-signing-key-32-chars-long!")
	os.Setenv("RECEIPT_SECRET_KEY", "rcpt-prod-custom-secure-signing-key-32-chars-long!")
	os.Setenv("RECEIPT_SALT", "custom-salt-16-chars-long!")
	os.Setenv("KPU_MASTER_SECRET", "kpu-distinct-quorum-secret-16chars!")
	os.Setenv("BAWASLU_AUDIT_SECRET", "bawaslu-distinct-quorum-secret-16chars!")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("expected valid production config to pass, got error: %v", err)
	}

	if cfg.ServerEnv != "production" {
		t.Errorf("expected ServerEnv to be production, got %s", cfg.ServerEnv)
	}
	if cfg.SeedDevData {
		t.Errorf("expected SeedDevData to be false in production")
	}
	if !cfg.RedisRequired {
		t.Errorf("expected RedisRequired to be true in production")
	}
}
