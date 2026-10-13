package database

import (
	"fmt"
	"log"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if cfg.DBDriver == "mysql" {
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
		)
		dialector = mysql.Open(dsn)
	} else {
		// Default: PostgreSQL
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=Asia/Jakarta",
			cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode,
		)
		dialector = postgres.Open(dsn)
	}

	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	db, err := gorm.Open(dialector, gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB instance: %w", err)
	}

	// Performance & Concurrency Connection Pooling Tuning
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)

	log.Printf("Connected to %s database with pool (MaxOpen: %d, MaxIdle: %d)",
		cfg.DBDriver, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns,
	)

	return db, nil
}

// AutoMigrate membuat tabel relasional secara aman
func AutoMigrate(db *gorm.DB) error {
	log.Println("Running AutoMigration for Zero-Linkability E-Voting Schema...")
	return db.AutoMigrate(
		&domain.Voter{},
		&domain.Chair{},
		&domain.CandidateGroup{},
		&domain.Ballot{},
		&domain.ElectionConfig{},
		&domain.AuditLog{},
	)
}

// SeedInitialData menginisialisasi kursi pemilihan & konfigurasi awal pemilu jika belum ada
func SeedInitialData(db *gorm.DB) {
	// 1. Seed Chairs
	var count int64
	db.Model(&domain.Chair{}).Count(&count)
	if count == 0 {
		log.Println("Seeding default election chairs...")
		chairs := []domain.Chair{
			{Name: "Presiden & Wakil Presiden Mahasiswa (BEM)", Scope: "ALL", MaxSelection: 1, IsActive: true},
			{Name: "Dewan Perwakilan Mahasiswa (MPM)", Scope: "ALL", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan TIK", Scope: "Teknik Informatika dan Komputer", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Elektro", Scope: "Teknik Elektro", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Mesin", Scope: "Teknik Mesin", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Sipil", Scope: "Teknik Sipil", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Akuntansi", Scope: "Akuntansi", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Niaga", Scope: "Administrasi Niaga", MaxSelection: 1, IsActive: true},
			{Name: "Ketua Himpunan Mahasiswa Jurusan Grafika", Scope: "Teknik Grafika dan Penerbitan", MaxSelection: 1, IsActive: true},
		}
		db.Create(&chairs)
	}

	// 2. Seed Election Config (Window Pemilihan)
	var cfgCount int64
	db.Model(&domain.ElectionConfig{}).Count(&cfgCount)
	if cfgCount == 0 {
		log.Println("Seeding default election schedule window...")
		now := time.Now()
		config := domain.ElectionConfig{
			Title:             "Pemilihan Umum Raya Mahasiswa 2026",
			StartTime:         now.Add(-2 * time.Hour), // Aktif
			EndTime:           now.Add(24 * time.Hour),
			IsEmergencyPaused: false,
		}
		db.Create(&config)
	}
}
