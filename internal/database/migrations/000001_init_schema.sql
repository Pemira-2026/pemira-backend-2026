-- =============================================================================
-- PEMIRA 2026 - ZERO-LINKABILITY RELATIONAL DATABASE MIGRATION
-- Engine: PostgreSQL 16
-- =============================================================================

-- 1. EXTENSIONS (UUID)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 2. TABEL DAFTAR PEMILIH TETAP (DPT) & STATUS KEIKUTSERTAAN
CREATE TABLE IF NOT EXISTS voters (
    id VARCHAR(36) PRIMARY KEY,                         -- UUID v4
    nim VARCHAR(25) NOT NULL UNIQUE,                    -- Nomor Induk Mahasiswa
    full_name VARCHAR(150) NOT NULL,
    email VARCHAR(150) NOT NULL UNIQUE,                 -- Email resmi kampus
    faculty VARCHAR(100) NOT NULL,                      -- Fakultas
    major VARCHAR(100) NOT NULL,                        -- Program Studi / Jurusan
    generation INT NOT NULL,                            -- Tahun Angkatan (2022, 2023, dst.)
    role VARCHAR(20) NOT NULL DEFAULT 'voter',          -- 'voter', 'admin', 'kpu', 'bawaslu'
    has_voted BOOLEAN NOT NULL DEFAULT FALSE,           -- Flag hak suara
    voted_at TIMESTAMP WITH TIME ZONE NULL,             -- Waktu pencoblosan
    receipt_hash VARCHAR(64) NULL,                      -- HMAC-SHA256 Token Hash
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_voters_email ON voters(email);
CREATE INDEX IF NOT EXISTS idx_voters_nim ON voters(nim);
CREATE INDEX IF NOT EXISTS idx_voters_major ON voters(major);
CREATE INDEX IF NOT EXISTS idx_voters_has_voted ON voters(has_voted);

-- 3. TABEL KURSI JABATAN / KATEGORI PEMILIHAN (CHAIRS)
CREATE TABLE IF NOT EXISTS chairs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,                         -- Contoh: "BEM", "MPM", "HMJ TIK"
    scope VARCHAR(100) NOT NULL DEFAULT 'ALL',          -- 'ALL' (Kampus) atau Nama Jurusan spesifik
    max_selection INT NOT NULL DEFAULT 1,               -- Jumlah paslon yang boleh dipilih
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 4. TABEL PASANGAN CALON (CANDIDATE GROUPS)
CREATE TABLE IF NOT EXISTS candidate_groups (
    id VARCHAR(36) PRIMARY KEY,                         -- UUID v4
    chair_id INT NOT NULL REFERENCES chairs(id) ON DELETE CASCADE,
    ballot_number INT NOT NULL,                         -- Nomor urut paslon (1, 2, 3...)
    leader_name VARCHAR(150) NOT NULL,
    vice_name VARCHAR(150) NULL,
    vision TEXT,
    mission TEXT,
    work_programs TEXT,
    image_url VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_chair_ballot_number UNIQUE (chair_id, ballot_number)
);

CREATE INDEX IF NOT EXISTS idx_candidates_chair ON candidate_groups(chair_id);

-- 5. KOTAK SUARA DIGITAL (BALLOTS) - ZERO LINKABILITY (LUBER JURDIL)
-- CATATAN KEAMANAN MUTLAK:
-- SAMA SEKALI TIDAK ADA KOLOM voter_id, nim, ATAU FOREIGN KEY KE IDENTITAS PEMILIH!
CREATE TABLE IF NOT EXISTS ballots (
    id VARCHAR(36) PRIMARY KEY,                         -- UUID v4 Acak
    chair_id INT NOT NULL REFERENCES chairs(id),
    candidate_group_id VARCHAR(36) NOT NULL REFERENCES candidate_groups(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ballots_chair ON ballots(chair_id);
CREATE INDEX IF NOT EXISTS idx_ballots_candidate ON ballots(candidate_group_id);

-- 5B. TABEL KOMITMEN TANDA TERIMA SUARA (RECEIPT COMMITMENTS) - ZERO-LINKABILITY
-- SAMA SEKALI TIDAK ADA voter_id, chair_id, ATAU candidate_group_id
CREATE TABLE IF NOT EXISTS receipt_commitments (
    id VARCHAR(36) PRIMARY KEY,                         -- UUID v4
    token_hash VARCHAR(64) NOT NULL UNIQUE,             -- SHA-256 hash dari receipt token publik
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_receipt_commitments_hash ON receipt_commitments(token_hash);

-- 6. PENGATURAN JADWAL PEMILU (ELECTION CONFIGS)
CREATE TABLE IF NOT EXISTS election_configs (
    id SERIAL PRIMARY KEY,
    title VARCHAR(150) NOT NULL,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    is_emergency_paused BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- 7. AUDIT TRAIL LOGS (LOG KEAMANAN AKSES PANITIA)
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id VARCHAR(36) NOT NULL,
    actor_role VARCHAR(50) NOT NULL,
    action VARCHAR(100) NOT NULL,                       -- 'LOGIN', 'DPT_IMPORT', 'CONFIG_CHANGE', dll.
    ip_address VARCHAR(45) NOT NULL,
    user_agent TEXT,
    payload JSONB NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_id);
