# ⚡ PEMIRA 2026 - CORE BACKEND ENGINE
**High-Concurrency & Zero-Linkability Digital Ballot API**  
*Mendukung 8.000+ Pemilih dengan Lonjakan 3.000–5.000 Request Serentak*

---

## 🌟 Prinsip Desain & Fitur Keamanan

1. **Zero-Linkability Ballot Box (Luber Jurdil)**:
   - Pemisahan total antara identitas pemilih (`voters`) dan surat suara (`ballots`).
   - SAMA SEKALI TIDAK ADA *Foreign Key* atau kolom identitas pemilih pada tabel `ballots`.
   - Dilengkapi *micro-batch timing jitter* untuk menggagalkan *timing correlation attacks*.
2. **Pencegahan Suara Ganda (Two-Layer Concurrency Guard)**:
   - **Layer 1:** Redis Distributed Lock (`SETNX lock:vote:<voter_id>`).
   - **Layer 2:** Atomic Conditional SQL Update (`WHERE id = ? AND has_voted = FALSE`).
3. **Cryptographic Voting Receipt**:
   - Pemilih memperoleh bukti tanda terima digital (`HMAC-SHA256 Token`) yang membuktikan keikutsertaan tanpa mengungkap paslon yang dipilih.
4. **Quarantined Tally Protection (Dual-Control Quorum)**:
   - Hasil rekapitulasi suara dikarantina otomatis hingga jam pemilu berakhir (`NOW() > end_time`) dan hanya dapat dibuka dengan kunci gabungan KPU & Bawaslu.

---

## 📂 Struktur Folder (Clean Architecture)

```text
pemira-backend/
├── cmd/
│   └── api/
│       └── main.go                  # Bootstrap aplikasi & dependency injection
├── internal/
│   ├── config/                      # Environment variables loader
│   ├── database/                    # Koneksi GORM, connection pool, & seeder
│   │   └── migrations/              # DDL SQL PostgreSQL 16
│   ├── domain/                      # Struct Entity & DTO (Voter, Chair, Ballot, dll.)
│   ├── repository/                  # Lapisan database query (AtomicMarkVoted, Ballot, dll.)
│   ├── service/                     # Logika bisnis (Vote Engine, Auth, Caching)
│   ├── handler/                     # HTTP Handler (Gin Controllers)
│   └── middleware/                  # JWT Auth RBAC & CORS
├── pkg/
│   ├── crypto/                      # HMAC-SHA256 Receipt generator
│   └── redis/                       # Redis client & SETNX distributed locker
├── docker/
│   ├── Dockerfile
│   └── docker-compose.yml           # PostgreSQL 16 + Redis 7
├── .env.example
├── go.mod
└── README.md
```

---

## 🚀 Panduan Memulai Cepat (Quick Start)

### 1. Menjalankan Database PostgreSQL & Redis
Gunakan Docker Compose yang telah disediakan:
```bash
cd docker
docker-compose up -d
```
Layanan yang aktif:
- **PostgreSQL 16:** `localhost:5432` (User: `postgres`, Pass: `postgres`, DB: `pemira_db`)
- **Redis 7:** `localhost:6379`

### 2. Konfigurasi Environment
Salin file `.env.example` ke `.env`:
```bash
cp .env.example .env
```

### 3. Menjalankan Aplikasi
```bash
go run cmd/api/main.go
```
Server akan berjalan di `http://localhost:8080`.

---

## 📡 Daftar Endpoint API Kunci

### 1. Autentikasi Pemilih
* `GET  /api/v1/auth/google/login` : Inisialisasi OAuth 2.0 dengan state anti-CSRF.
* `GET  /api/v1/auth/google/callback`: Pertukaran token Google OAuth.
* `POST /api/v1/auth/mock-login` : Login simulasi untuk pengujian lokal:
  ```json
  {
    "email": "13523001@stu.pnj.ac.id",
    "nim": "13523001",
    "name": "Andi Pratama",
    "major": "Teknik Informatika dan Komputer"
  }
  ```
* `GET  /api/v1/auth/me` : Profil pemilih login & daftar kursi yang berhak dicoblos.

### 2. Bilik Suara Digital
* `GET  /api/v1/booth/candidates` : Mengambil daftar paslon sesuai jurusan pemilih (Protected).
* `POST /api/v1/vote/submit` : Mengirim surat suara (Atomic & Zero-Linkability):
  ```json
  {
    "turnstileToken": "dummy-token-dev",
    "votes": [
      { "chairId": 1, "candidateGroupId": "uuid-paslon-bem" },
      { "chairId": 3, "candidateGroupId": "uuid-paslon-hmj-tik" }
    ]
  }
  ```
* `GET  /api/v1/vote/receipt/:code` : Verifikasi keabsahan bukti tanda terima suara publik.

### 3. Panel Admin & Saksi
* `GET  /api/v1/admin/turnout` : Monitoring persentase kehadiran pemilih secara real-time.
* `POST /api/v1/admin/tally` : Pembongkaran kotak suara (Wajib header `x-kpu-secret-key` & `x-bawaslu-secret-key` setelah pemilu usai).
