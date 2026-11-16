package database

import (
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"testing"
	"time"
)

func TestDatabaseConnectSQLiteAndAutoMigrate(t *testing.T) {
	cfg := &config.Config{
		ServerEnv:         "development",
		DBDriver:          "sqlite",
		DBName:            ":memory:",
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 5 * time.Minute,
		DBAutoMigrate:     true,
		SeedDevData:       true,
	}

	db, err := Connect(cfg)
	if err != nil {
		t.Fatalf("expected successful SQLite connection, got: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("expected sql.DB instance, got: %v", err)
	}
	defer sqlDB.Close()

	// Run migrations
	err = RunMigrations(db, cfg)
	if err != nil {
		t.Fatalf("expected successful RunMigrations, got: %v", err)
	}

	// Verify all tables were created
	expectedTables := []string{
		"voters",
		"chairs",
		"candidate_groups",
		"ballots",
		"receipt_commitments",
		"election_configs",
		"audit_logs",
		"admin_users",
	}

	for _, tbl := range expectedTables {
		if !db.Migrator().HasTable(tbl) {
			t.Errorf("expected table '%s' to exist after AutoMigrate", tbl)
		}
	}
}

func TestDatabaseSeedOnlyInDevelopment(t *testing.T) {
	// 1. Production Mode: Seed must NOT run
	prodCfg := &config.Config{
		ServerEnv:         "production",
		DBDriver:          "sqlite",
		DBName:            "file:prod_test?mode=memory&cache=shared",
		DBMaxOpenConns:    5,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: 1 * time.Minute,
		DBAutoMigrate:     true,
		SeedDevData:       true, // Even if set to true, production must ignore this
	}

	prodDB, err := Connect(prodCfg)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	sqlProd, _ := prodDB.DB()
	defer sqlProd.Close()

	_ = AutoMigrate(prodDB)

	// Call Seed in production
	SeedInitialData(prodDB, prodCfg)

	var chairCount int64
	prodDB.Model(&domain.Chair{}).Count(&chairCount)
	if chairCount != 0 {
		t.Errorf("expected 0 chairs in production mode, got %d", chairCount)
	}

	var candCount int64
	prodDB.Model(&domain.CandidateGroup{}).Count(&candCount)
	if candCount != 0 {
		t.Errorf("expected 0 candidates in production mode, got %d", candCount)
	}

	// 2. Development Mode: Seed MUST run
	devCfg := &config.Config{
		ServerEnv:         "development",
		DBDriver:          "sqlite",
		DBName:            "file:dev_test?mode=memory&cache=shared",
		DBMaxOpenConns:    5,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: 1 * time.Minute,
		DBAutoMigrate:     true,
		SeedDevData:       true,
	}

	devDB, err := Connect(devCfg)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	sqlDev, _ := devDB.DB()
	defer sqlDev.Close()

	_ = AutoMigrate(devDB)
	SeedInitialData(devDB, devCfg)

	devDB.Model(&domain.Chair{}).Count(&chairCount)
	if chairCount == 0 {
		t.Errorf("expected chairs to be seeded in development mode, got 0")
	}

	devDB.Model(&domain.CandidateGroup{}).Count(&candCount)
	if candCount == 0 {
		t.Errorf("expected candidates to be seeded in development mode, got 0")
	}

	var cfgCount int64
	devDB.Model(&domain.ElectionConfig{}).Count(&cfgCount)
	if cfgCount == 0 {
		t.Errorf("expected election config to be seeded in development mode, got 0")
	}
}

func TestDatabaseConnectFailureHandling(t *testing.T) {
	// Invalid Postgres host that will fail to connect / ping
	cfg := &config.Config{
		ServerEnv:         "production",
		DBDriver:          "postgres",
		DBHost:            "127.0.0.1",
		DBPort:            "59998", // Port with no DB listening
		DBUser:            "postgres",
		DBPassword:        "wrong_pass",
		DBName:            "invalid_db",
		DBSSLMode:         "disable",
		DBMaxOpenConns:    5,
		DBMaxIdleConns:    2,
		DBConnMaxLifetime: 1 * time.Minute,
	}

	db, err := Connect(cfg)
	if err == nil {
		t.Fatalf("expected connection failure, got nil")
	}
	if db != nil {
		t.Errorf("expected db to be nil on connection failure, got %v", db)
	}
}
