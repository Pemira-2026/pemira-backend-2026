-- =============================================================================
-- PEMIRA 2026 - ADMIN USERS TABLE MIGRATION
-- Engine: PostgreSQL 16
-- =============================================================================

CREATE TABLE IF NOT EXISTS admin_users (
    id VARCHAR(36) PRIMARY KEY,                         -- UUID v4
    username VARCHAR(50) NOT NULL UNIQUE,               -- Username unik panitia
    email VARCHAR(150) NOT NULL UNIQUE,                 -- Email resmi panitia
    password_hash VARCHAR(255) NOT NULL,                -- Argon2id / Bcrypt hashed password
    full_name VARCHAR(150) NOT NULL,
    role VARCHAR(20) NOT NULL,                          -- 'admin', 'kpu', 'bawaslu', 'saksi'
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMP WITH TIME ZONE NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_admin_users_username ON admin_users(username);
CREATE INDEX IF NOT EXISTS idx_admin_users_email ON admin_users(email);
CREATE INDEX IF NOT EXISTS idx_admin_users_role ON admin_users(role);

