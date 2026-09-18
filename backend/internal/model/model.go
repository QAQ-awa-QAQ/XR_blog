package model

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
	// RoleGuest 游客：只读浏览，没有数据库行（身份由 GuestUser 合成）。
	RoleGuest = "guest"
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

// GuestUser 构造游客的合成身份：没有数据库行，ID 保持 0 这个哨兵值
// （与会话层的约定一致）。昵称固定为「访客」，避免与真实用户混淆。
func GuestUser(ip string) *User {
	return &User{Account: "guest", Nickname: "访客", Role: RoleGuest, LastIP: ip}
}

type InviteCode struct {
	ID        uint   `gorm:"primaryKey"`
	Code      string `gorm:"size:32;uniqueIndex;not null"`
	CreatedBy uint   `gorm:"not null"`
	UsedBy    *uint
	// 可用次数：一码可多用；UsedCount >= MaxUses 即失效
	MaxUses   int       `gorm:"not null;default:1"`
	UsedCount int       `gorm:"not null;default:0"`
	ExpiresAt time.Time `gorm:"not null"`
	UsedAt    *time.Time
	CreatedAt time.Time
}

// Used 判断邀请码是否已耗尽可用次数（MaxUses 非法时按 1 处理）。
func (i InviteCode) Used() bool {
	max := i.MaxUses
	if max <= 0 {
		max = 1
	}
	return i.UsedCount >= max
}

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

// ---------- 访问控制：用户组 / 功能授权 / 功能地址 ----------

// Group 用户组。管理员在后台建组、勾选授权功能、挑选成员；
// 用户可点的功能 = 所隶属组授权集合的并集。
type Group struct {
	ID        uint   `gorm:"primaryKey"`
	Name      string `gorm:"size:32;uniqueIndex;not null"`
	CreatedAt time.Time
}

// GroupFeature 组 × 功能授权。FeatureKey 对应前端配置文件里功能的稳定 key。
type GroupFeature struct {
	GroupID    uint   `gorm:"primaryKey"`
	FeatureKey string `gorm:"primaryKey;size:64"`
}

// UserGroup 用户与组的多对多关系。
type UserGroup struct {
	UserID  uint `gorm:"primaryKey"`
	GroupID uint `gorm:"primaryKey"`
}

// Feature 功能入口。标题/描述/标签/图标/内网地址统一由管理后台维护；
// 公开接口只下发展示字段，URL 永不进入前端产物。
type Feature struct {
	Key       string `gorm:"primaryKey;size:64"`
	Title     string `gorm:"size:32;not null"`
	Desc      string `gorm:"size:128;not null"`
	Tag       string `gorm:"size:16;not null"`
	Icon      string `gorm:"size:32;not null"`
	URL       string `gorm:"size:512"`
	Sort      int    `gorm:"not null;default:0;index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}
