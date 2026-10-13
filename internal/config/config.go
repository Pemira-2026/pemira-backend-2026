package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	ServerPort string
	ServerEnv  string

	DBDriver          string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	JWTSecret        string
	ReceiptSecretKey string
	ReceiptSalt      string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	CampusEmailDomain  string

	TurnstileSecretKey    string
	EnableTurnstileVerify bool

	KPUMasterSecret    string
	BawasluAuditSecret string

	VoterFrontendURL string
	AdminFrontendURL string
}

func LoadConfig() *Config {
	// Load .env if present
	_ = godotenv.Load()

	maxOpenConns, _ := strconv.Atoi(getEnv("DB_MAX_OPEN_CONNS", "100"))
	maxIdleConns, _ := strconv.Atoi(getEnv("DB_MAX_IDLE_CONNS", "30"))
	redisDB, _ := strconv.Atoi(getEnv("REDIS_DB", "0"))
	turnstileVerify, _ := strconv.ParseBool(getEnv("ENABLE_TURNSTILE_VERIFY", "false"))

	cfg := &Config{
		ServerPort:            getEnv("SERVER_PORT", "8080"),
		ServerEnv:             getEnv("SERVER_ENV", "development"),
		DBDriver:              getEnv("DB_DRIVER", "postgres"),
		DBHost:                getEnv("DB_HOST", "localhost"),
		DBPort:                getEnv("DB_PORT", "5432"),
		DBUser:                getEnv("DB_USER", "postgres"),
		DBPassword:            getEnv("DB_PASSWORD", "postgres"),
		DBName:                getEnv("DB_NAME", "pemira_db"),
		DBSSLMode:             getEnv("DB_SSLMODE", "disable"),
		DBMaxOpenConns:        maxOpenConns,
		DBMaxIdleConns:        maxIdleConns,
		DBConnMaxLifetime:     5 * time.Minute,
		RedisHost:             getEnv("REDIS_HOST", "localhost"),
		RedisPort:             getEnv("REDIS_PORT", "6379"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		RedisDB:               redisDB,
		JWTSecret:             getEnv("JWT_SECRET", "super-secret-jwt-signing-key-pemira-2026-default"),
		ReceiptSecretKey:      getEnv("RECEIPT_SECRET_KEY", "super-secret-hmac-sha256-receipt-key-pemira-2026"),
		ReceiptSalt:           getEnv("RECEIPT_SALT", "random-salt-salt-salt-2026"),
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:     getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		CampusEmailDomain:     getEnv("CAMPUS_EMAIL_DOMAIN", "stu.pnj.ac.id"),
		TurnstileSecretKey:    getEnv("TURNSTILE_SECRET_KEY", ""),
		EnableTurnstileVerify: turnstileVerify,
		KPUMasterSecret:       getEnv("KPU_MASTER_SECRET", "kpu-master-secret-key-2026"),
		BawasluAuditSecret:    getEnv("BAWASLU_AUDIT_SECRET", "bawaslu-audit-secret-key-2026"),
		VoterFrontendURL:      getEnv("VOTER_FRONTEND_URL", "http://localhost:5173"),
		AdminFrontendURL:      getEnv("ADMIN_FRONTEND_URL", "http://localhost:5174"),
	}

	log.Println("Configuration loaded successfully.")
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
