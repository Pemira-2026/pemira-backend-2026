package database

import (
	"fmt"
	"log"
	"pemira-backend/internal/config"
	"pemira-backend/internal/domain"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if cfg.DatabaseURL != "" {
		dialector = postgres.Open(cfg.DatabaseURL)
	} else if cfg.DBDriver == "mysql" {
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

	// 3. Seed Candidates jika belum ada
	var candCount int64
	db.Model(&domain.CandidateGroup{}).Count(&candCount)
	if candCount == 0 {
		log.Println("Seeding default election candidates...")
		var allChairs []domain.Chair
		db.Find(&allChairs)
		chairMap := make(map[string]uint)
		for _, c := range allChairs {
			chairMap[c.Name] = c.ID
		}

		candidates := []domain.CandidateGroup{
			// BEM Paslon 1
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Presiden & Wakil Presiden Mahasiswa (BEM)"],
				BallotNumber: 1,
				LeaderName:   "Arya Wijaya",
				ViceName:     "Nadia Rahmawati",
				Vision:       "Mewujudkan KM yang berdaya, kritis, dan berorientasi pada kebermanfaatan nyata bagi seluruh mahasiswa.",
				Mission:      "1. Transformasi digital advokasi kampus.\n2. Wadah kolaborasi riset & karya lintas jurusan.\n3. Pendampingan karir dan beasiswa terpadu.",
				WorkPrograms: "PEMIRA Hub, Portal Beasiswa Kampus, Pekan Karya Inovasi Mahasiswa",
				ImageURL:     "https://images.unsplash.com/photo-1534528741775-53994a69daeb?w=500&auto=format&fit=crop&q=80",
			},
			// BEM Paslon 2
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Presiden & Wakil Presiden Mahasiswa (BEM)"],
				BallotNumber: 2,
				LeaderName:   "Fikri Maulana",
				ViceName:     "Dinda Permata",
				Vision:       "Sinergi KM Berintegritas, Inklusif, dan Berprestasi di Kancah Nasional & Internasional.",
				Mission:      "1. Penguatan fungsi pengawasan dan tata kelola organisasi.\n2. Optimalisasi fasilitas dan ruang kreatif mahasiswa.\n3. Kampus hijau dan berkelanjutan.",
				WorkPrograms: "Kampus Hijau 2026, Crisis Center Mahasiswa, Sinergi Ormawa Fest",
				ImageURL:     "https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?w=500&auto=format&fit=crop&q=80",
			},
			// MPM Calon 1
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Dewan Perwakilan Mahasiswa (MPM)"],
				BallotNumber: 1,
				LeaderName:   "Rizky Ramadhan",
				Vision:       "Legislatif mahasiswa yang transparan, aspiratif, dan berkeadilan.",
				Mission:      "1. Keterbukaan risalah sidang dan regulasi kampus.\n2. Kanal aspirasi mahasiswa 24/7.",
				WorkPrograms: "Ruang Dengar Terbuka, Audit Terbuka UKM, LegislaZone",
				ImageURL:     "https://images.unsplash.com/photo-1500648767791-00dcc994a43e?w=500&auto=format&fit=crop&q=80",
			},
			// MPM Calon 2
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Dewan Perwakilan Mahasiswa (MPM)"],
				BallotNumber: 2,
				LeaderName:   "Siti Rahma",
				Vision:       "Optimalisasi fungsi legislasi dan anggaran yang akuntabel untuk kesejahteraan mahasiswa.",
				Mission:      "1. Uji publik rancangan undang-undang mahasiswa.\n2. Pengawasan keuangan organisasi berbasis data.",
				WorkPrograms: "Parlemen Mengabdi, Open Financial Audit, Public Hearing Rutin",
				ImageURL:     "https://images.unsplash.com/photo-1494790108377-be9c29b29330?w=500&auto=format&fit=crop&q=80",
			},
			// HMJ Elektro Calon 1
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Ketua Himpunan Mahasiswa Jurusan Elektro"],
				BallotNumber: 1,
				LeaderName:   "Bagas Pratama",
				Vision:       "Himpunan Mahasiswa Elektro yang adaptif, inovatif, dan berdaya saing di era industri 4.0.",
				Mission:      "1. Peningkatan kompetensi IoT dan energi terbarukan.\n2. Kolaborasi riset bersama industri.",
				WorkPrograms: "Electro Hackathon 2026, IoT Boot Camp, HME Care",
				ImageURL:     "https://images.unsplash.com/photo-1519085360753-af0119f7cbe7?w=500&auto=format&fit=crop&q=80",
			},
			// HMJ Elektro Calon 2
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Ketua Himpunan Mahasiswa Jurusan Elektro"],
				BallotNumber: 2,
				LeaderName:   "Anisa Putri",
				Vision:       "Mewujudkan keluarga besar Teknik Elektro yang solid, berprestasi, dan humanis.",
				Mission:      "1. Penguatan kebersamaan antar angkatan.\n2. Pelatihan sertifikasi kelistrikan dan instrumentasi.",
				WorkPrograms: "Electro Mentorship, Sertifikasi K3 Listrik, Expo Inovasi Elektro",
				ImageURL:     "https://images.unsplash.com/photo-1517841905240-472988babdf9?w=500&auto=format&fit=crop&q=80",
			},
			// HMJ TIK Calon 1
			{
				ID:           uuid.New().String(),
				ChairID:      chairMap["Ketua Himpunan Mahasiswa Jurusan TIK"],
				BallotNumber: 1,
				LeaderName:   "Alifian Firdaus",
				Vision:       "Himpunan TIK sebagai episentrum teknologi, riset open-source, dan inovasi kecerdasan buatan.",
				Mission:      "1. Inkubasi startup mahasiswa.\n2. Tech competition mentorship.",
				WorkPrograms: "CodeFest 2026, AI Study Club, Open Source Weekend",
				ImageURL:     "https://images.unsplash.com/photo-1539571696357-5a69c17a67c6?w=500&auto=format&fit=crop&q=80",
			},
		}

		for _, cand := range candidates {
			if cand.ChairID != 0 {
				db.Create(&cand)
			}
		}
	}
}
