package model

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// 封禁档位：第一次 24h 限时，第二次起永久。
const (
	BanLevelTimed     = 1
	BanLevelPermanent = 2
)

type User struct {
	ID           uint   `gorm:"primaryKey"`
	Account      string `gorm:"size:32;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	Nickname     string `gorm:"size:32;not null"`
	Role         string `gorm:"size:16;not null;default:user"`
	LastIP       string `gorm:"size:64"`
	CreatedAt    time.Time
}

type InviteCode struct {
	ID        uint   `gorm:"primaryKey"`
	Code      string `gorm:"size:32;uniqueIndex;not null"`
	CreatedBy uint   `gorm:"not null"`
	UsedBy    *uint
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

// Used 判断邀请码是否已使用。
func (i InviteCode) Used() bool { return i.UsedAt != nil }

// Expired 判断邀请码是否过期。
func (i InviteCode) Expired(now time.Time) bool { return now.After(i.ExpiresAt) }

// BanRecord 每个 IP 一条，offense_count 永不清除——否则重启后无法区分首次与再犯。
type BanRecord struct {
	IP           string `gorm:"primaryKey;size:64"`
	OffenseCount int    `gorm:"not null"`
	Level        int    `gorm:"not null"`
	ExpiresAt    *time.Time
	Reason       string `gorm:"size:128"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Active 判断该记录当前是否处于封禁生效期。永久封禁的 ExpiresAt 为 nil。
func (b BanRecord) Active(now time.Time) bool {
	if b.Level == BanLevelPermanent || b.ExpiresAt == nil {
		return true
	}
	return now.Before(*b.ExpiresAt)
}
