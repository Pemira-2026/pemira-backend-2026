package domain

import (
	"time"
)

// AdminUser merepresentasikan pengguna panel administratif (KPU, Bawaslu, Saksi, Super Admin)
type AdminUser struct {
	ID           string     `gorm:"type:varchar(36);primaryKey" json:"id"`
	Username     string     `gorm:"type:varchar(50);uniqueIndex;not null" json:"username"`
	Email        string     `gorm:"type:varchar(150);uniqueIndex;not null" json:"email"`
	PasswordHash string     `gorm:"type:varchar(255);not null" json:"-"`
	FullName     string     `gorm:"type:varchar(150);not null" json:"fullName"`
	Role         string     `gorm:"type:varchar(20);not null" json:"role"` // 'admin', 'kpu', 'bawaslu', 'saksi'
	IsActive     bool       `gorm:"not null;default:true" json:"isActive"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (AdminUser) TableName() string {
	return "admin_users"
}
