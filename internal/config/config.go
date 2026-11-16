package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Known insecure default secrets that are strictly forbidden in production
var InsecureDefaultSecrets = []string{
	"super-secret-jwt-signing-key-pemira-2026-default",
	"super-secret-jwt-signing-key-pemira-2026-minimum-32-chars",
	"super-secret-hmac-sha256-receipt-key-pemira-2026",
	"super-secret-hmac-sha256-receipt-key-pemira-2026-change-in-prod",
	"random-salt-salt-salt-2026",
	"kpu-master-secret-key-2026",
	"bawaslu-audit-secret-key-2026",
	"1x0000000000000000000000000000000AA",
}

type Config struct {
	ServerPort string
	ServerEnv  string // "development" | "production" | "test"

	DBDriver          string // "postgres" | "mysql" | "sqlite"
	DatabaseURL       string
	DBHost            string
	DBPort            string
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBAutoMigrate     bool
	SeedDevData       bool

	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int
	RedisRequired bool

	JWTSecret        string
	ReceiptSecretKey string
	ReceiptSalt      string

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	CampusEmailDomain  string

	TurnstileSecretKey    string
	EnableTurnstileVerify bool
	TurnstileVerifyURL    string
	TurnstileTimeout      time.Duration

	KPUMasterSecret    string
	BawasluAuditSecret string

	VoterFrontendURL string
	AdminFrontendURL string

	// Reverse proxy / infrastructure
	TrustedProxies []string

	// Operational hardening
	RateLimitEnabled bool
	MetricsEnabled   bool
	MetricsToken     string

	// MCP operator authorization (STDIO tool server)
	MCPOperatorRole        string // readonly | voter | admin | kpu | bawaslu
	MCPAllowVoteSubmit     bool
	MCPAllowTally          bool
	MCPAllowConfigMutation bool
	MCPAllowAuditLogs      bool
	MCPAllowLookupVoter    bool
	MCPAllowMockLogin      bool
}

// ConfigValidationError represents multiple validation issues in configuration
type ConfigValidationError struct {
	Errors []string
}

func (e *ConfigValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString("configuration validation failed:")
	for _, err := range e.Errors {
		sb.WriteString("\n  - ")
		sb.WriteString(err)
	}
	return sb.String()
}

func LoadConfig() (*Config, error) {
	// Attempt to load .env file if it exists (not required in containerized/production env)
	_ = godotenv.Load()

	serverEnv := strings.ToLower(getEnv("SERVER_ENV", "development"))

	// Sensible defaults based on environment
	isProd := serverEnv == "production"

	maxOpenConns := getEnvInt("DB_MAX_OPEN_CONNS", 100)
	maxIdleConns := getEnvInt("DB_MAX_IDLE_CONNS", 30)
	redisDB := getEnvInt("REDIS_DB", 0)
	turnstileVerify := getEnvBool("ENABLE_TURNSTILE_VERIFY", false)

	// MCP is a local operator tool. Production defaults are deliberately
	// read-only; privileged tools require an explicit opt-in environment flag.
	defaultMCPRole := "readonly"
	if !isProd {
		defaultMCPRole = "admin"
	}
	mcpRole := strings.ToLower(getEnv("MCP_OPERATOR_ROLE", defaultMCPRole))

	// In production, migrations can be opted-in via DB_AUTO_MIGRATE=true, default false.
	// In development, auto-migrate defaults to true.
	dbAutoMigrate := getEnvBool("DB_AUTO_MIGRATE", !isProd)

	// Seeding is ONLY allowed in development, never in production.
	seedDevData := false
	if !isProd {
		seedDevData = getEnvBool("SEED_DEV_DATA", true)
	}

	// Redis is required in production, optional in development unless specified
	redisRequired := getEnvBool("REDIS_REQUIRED", isProd)

	cfg := &Config{
		ServerPort:            getEnv("SERVER_PORT", "8080"),
		ServerEnv:             serverEnv,
		DBDriver:              getEnv("DB_DRIVER", "postgres"),
		DatabaseURL:           getEnv("DATABASE_URL", ""),
		DBHost:                getEnv("DB_HOST", "localhost"),
		DBPort:                getEnv("DB_PORT", "5432"),
		DBUser:                getEnv("DB_USER", "postgres"),
		DBPassword:            getEnv("DB_PASSWORD", ""),
		DBName:                getEnv("DB_NAME", "pemira_db"),
		DBSSLMode:             getEnv("DB_SSLMODE", "disable"),
		DBMaxOpenConns:        maxOpenConns,
		DBMaxIdleConns:        maxIdleConns,
		DBConnMaxLifetime:     getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		DBAutoMigrate:         dbAutoMigrate,
		SeedDevData:           seedDevData,
		RedisHost:             getEnv("REDIS_HOST", "localhost"),
		RedisPort:             getEnv("REDIS_PORT", "6379"),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		RedisDB:               redisDB,
		RedisRequired:         redisRequired,
		JWTSecret:             getEnv("JWT_SECRET", ""),
		ReceiptSecretKey:      getEnv("RECEIPT_SECRET_KEY", ""),
		ReceiptSalt:           getEnv("RECEIPT_SALT", ""),
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:     getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback"),
		CampusEmailDomain:     getEnv("CAMPUS_EMAIL_DOMAIN", "stu.pnj.ac.id"),
		TurnstileSecretKey:    getEnv("TURNSTILE_SECRET_KEY", ""),
		EnableTurnstileVerify: turnstileVerify,
		TurnstileVerifyURL:    getEnv("TURNSTILE_VERIFY_URL", "https://challenges.cloudflare.com/turnstile/v0/siteverify"),
		TurnstileTimeout:      getEnvDuration("TURNSTILE_TIMEOUT", 5*time.Second),
		KPUMasterSecret:       getEnv("KPU_MASTER_SECRET", ""),
		BawasluAuditSecret:    getEnv("BAWASLU_AUDIT_SECRET", ""),
		VoterFrontendURL:      getEnv("VOTER_FRONTEND_URL", "http://localhost:5173"),
		AdminFrontendURL:      getEnv("ADMIN_FRONTEND_URL", "http://localhost:5174"),

		TrustedProxies:   splitCSV(getEnv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		RateLimitEnabled: getEnvBool("RATE_LIMIT_ENABLED", true),
		MetricsEnabled:   getEnvBool("METRICS_ENABLED", true),
		MetricsToken:     getEnv("METRICS_TOKEN", ""),

		MCPOperatorRole:        mcpRole,
		MCPAllowVoteSubmit:     getEnvBool("MCP_ALLOW_VOTE_SUBMIT", false),
		MCPAllowTally:          getEnvBool("MCP_ALLOW_TALLY", false),
		MCPAllowConfigMutation: getEnvBool("MCP_ALLOW_CONFIG_MUTATION", false),
		MCPAllowAuditLogs:      getEnvBool("MCP_ALLOW_AUDIT_LOGS", false),
		MCPAllowLookupVoter:    getEnvBool("MCP_ALLOW_LOOKUP_VOTER", false),
		MCPAllowMockLogin:      getEnvBool("MCP_ALLOW_MOCK_LOGIN", !isProd),
	}

	// Apply dev fallback secrets with visible warning if not supplied
	if serverEnv == "development" || serverEnv == "test" {
		applyDevelopmentDefaults(cfg)
	}

	// Validate the configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	log.Printf("Configuration loaded successfully. Environment: %s (AutoMigrate: %t, SeedDevData: %t, RedisRequired: %t)",
		cfg.ServerEnv, cfg.DBAutoMigrate, cfg.SeedDevData, cfg.RedisRequired)

	return cfg, nil
}

func applyDevelopmentDefaults(cfg *Config) {
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "super-secret-jwt-signing-key-pemira-2026-default"
		log.Println("[SECURITY WARNING] JWT_SECRET not set, using default development secret.")
	}
	if cfg.ReceiptSecretKey == "" {
		cfg.ReceiptSecretKey = "super-secret-hmac-sha256-receipt-key-pemira-2026"
		log.Println("[SECURITY WARNING] RECEIPT_SECRET_KEY not set, using default development secret.")
	}
	if cfg.ReceiptSalt == "" {
		cfg.ReceiptSalt = "random-salt-salt-salt-2026"
		log.Println("[SECURITY WARNING] RECEIPT_SALT not set, using default development salt.")
	}
	if cfg.KPUMasterSecret == "" {
		cfg.KPUMasterSecret = "kpu-master-secret-key-2026"
		log.Println("[SECURITY WARNING] KPU_MASTER_SECRET not set, using default development secret.")
	}
	if cfg.BawasluAuditSecret == "" {
		cfg.BawasluAuditSecret = "bawaslu-audit-secret-key-2026"
		log.Println("[SECURITY WARNING] BAWASLU_AUDIT_SECRET not set, using default development secret.")
	}
}

func (c *Config) Validate() error {
	var errs []string

	// 1. Environment validation
	switch c.ServerEnv {
	case "development", "production", "test":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("invalid SERVER_ENV '%s': must be 'development', 'production', or 'test'", c.ServerEnv))
	}

	// 2. Database validation
	if c.DatabaseURL == "" {
		if c.DBDriver == "" {
			errs = append(errs, "DB_DRIVER must be specified if DATABASE_URL is empty ('postgres', 'mysql', or 'sqlite')")
		}
		if c.DBDriver != "sqlite" {
			if c.DBHost == "" {
				errs = append(errs, "DB_HOST is required when DATABASE_URL is empty")
			}
			if c.DBUser == "" {
				errs = append(errs, "DB_USER is required when DATABASE_URL is empty")
			}
			if c.DBName == "" {
				errs = append(errs, "DB_NAME is required when DATABASE_URL is empty")
			}
		}
	}

	// 3. URLs validation
	if c.VoterFrontendURL == "" {
		errs = append(errs, "VOTER_FRONTEND_URL cannot be empty")
	} else if _, err := url.ParseRequestURI(c.VoterFrontendURL); err != nil {
		errs = append(errs, fmt.Sprintf("invalid VOTER_FRONTEND_URL: %v", err))
	}

	if c.AdminFrontendURL == "" {
		errs = append(errs, "ADMIN_FRONTEND_URL cannot be empty")
	} else if _, err := url.ParseRequestURI(c.AdminFrontendURL); err != nil {
		errs = append(errs, fmt.Sprintf("invalid ADMIN_FRONTEND_URL: %v", err))
	}

	// 3. MCP operator role validation
	switch c.MCPOperatorRole {
	case "readonly", "voter", "admin", "kpu", "bawaslu":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("invalid MCP_OPERATOR_ROLE '%s': must be readonly, voter, admin, kpu, or bawaslu", c.MCPOperatorRole))
	}

	// 4. Production-specific strict security validations
	if c.ServerEnv == "production" {
		// Seeding is strictly forbidden in production
		if c.SeedDevData {
			errs = append(errs, "SEED_DEV_DATA must be false in production")
		}

		// JWT secret validation
		if c.JWTSecret == "" {
			errs = append(errs, "JWT_SECRET is required in production")
		} else {
			if len(c.JWTSecret) < 32 {
				errs = append(errs, "JWT_SECRET must be at least 32 characters in production for cryptographic safety")
			}
			if isInsecureDefault(c.JWTSecret) {
				errs = append(errs, "JWT_SECRET is using an insecure default or placeholder value")
			}
		}

		// Receipt secret key validation
		if c.ReceiptSecretKey == "" {
			errs = append(errs, "RECEIPT_SECRET_KEY is required in production")
		} else {
			if len(c.ReceiptSecretKey) < 32 {
				errs = append(errs, "RECEIPT_SECRET_KEY must be at least 32 characters in production")
			}
			if isInsecureDefault(c.ReceiptSecretKey) {
				errs = append(errs, "RECEIPT_SECRET_KEY is using an insecure default or placeholder value")
			}
		}

		// Receipt salt validation
		if c.ReceiptSalt == "" {
			errs = append(errs, "RECEIPT_SALT is required in production")
		} else {
			if len(c.ReceiptSalt) < 16 {
				errs = append(errs, "RECEIPT_SALT must be at least 16 characters in production")
			}
			if isInsecureDefault(c.ReceiptSalt) {
				errs = append(errs, "RECEIPT_SALT is using an insecure default or placeholder value")
			}
		}

		// Quorum secrets validation (Dual-control quorum)
		if c.KPUMasterSecret == "" {
			errs = append(errs, "KPU_MASTER_SECRET is required in production")
		} else {
			if len(c.KPUMasterSecret) < 16 {
				errs = append(errs, "KPU_MASTER_SECRET must be at least 16 characters in production")
			}
			if isInsecureDefault(c.KPUMasterSecret) {
				errs = append(errs, "KPU_MASTER_SECRET is using an insecure default or placeholder value")
			}
		}

		if c.BawasluAuditSecret == "" {
			errs = append(errs, "BAWASLU_AUDIT_SECRET is required in production")
		} else {
			if len(c.BawasluAuditSecret) < 16 {
				errs = append(errs, "BAWASLU_AUDIT_SECRET must be at least 16 characters in production")
			}
			if isInsecureDefault(c.BawasluAuditSecret) {
				errs = append(errs, "BAWASLU_AUDIT_SECRET is using an insecure default or placeholder value")
			}
		}

		// Dual-control check: secrets must not be identical
		if c.KPUMasterSecret != "" && c.KPUMasterSecret == c.BawasluAuditSecret {
			errs = append(errs, "KPU_MASTER_SECRET and BAWASLU_AUDIT_SECRET must not be identical for quorum security")
		}

		// Turnstile verification validation
		if c.EnableTurnstileVerify {
			if c.TurnstileSecretKey == "" {
				errs = append(errs, "TURNSTILE_SECRET_KEY is required when ENABLE_TURNSTILE_VERIFY is enabled")
			} else if isInsecureDefault(c.TurnstileSecretKey) {
				errs = append(errs, "TURNSTILE_SECRET_KEY is using a dummy test key in production")
			}
		}

		// Redis validation in production
		if c.RedisHost == "" || c.RedisPort == "" {
			errs = append(errs, "REDIS_HOST and REDIS_PORT are required in production")
		}

		// MCP dev-only tooling must never be enabled in production.
		if c.MCPAllowMockLogin {
			errs = append(errs, "MCP_ALLOW_MOCK_LOGIN must be false in production")
		}

		if c.MetricsEnabled && c.MetricsToken == "" {
			log.Println("[SECURITY WARNING] METRICS_TOKEN is empty in production. /metrics should be protected by a private network or bearer token.")
		}

		// Database SSL Mode warning/check in production
		if c.DatabaseURL == "" && c.DBDriver == "postgres" && c.DBSSLMode == "disable" {
			log.Println("[SECURITY WARNING] DB_SSLMODE is set to 'disable' in production! Ensure a secure private network is used.")
		}
	}

	if len(errs) > 0 {
		return &ConfigValidationError{Errors: errs}
	}

	return nil
}

func isInsecureDefault(secret string) bool {
	for _, insecure := range InsecureDefaultSecrets {
		if secret == insecure {
			return true
		}
	}
	return false
}

func getEnv(key, defaultVal string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func getEnvInt(key string, defaultVal int) int {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		return defaultVal
	}
	return d
}

func splitCSV(val string) []string {
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
