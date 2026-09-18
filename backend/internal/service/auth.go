package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"

	"personal_blog/internal/model"
	"personal_blog/internal/security"
)

var (
	ErrAccountTaken   = errors.New("账号已被占用")
	ErrBadCredentials = errors.New("账号或密码错误")
	ErrInviteInvalid  = errors.New("邀请码无效或已失效")
	ErrBadAccount     = errors.New("账号需为 3-20 位字母、数字或下划线")
	ErrBadPassword    = errors.New("密码长度至少 8 位")
	ErrBadNickname    = errors.New("昵称需为 1-20 个字符")
)

var accountPattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,20}$`)

const maxPasswordLen = 128

type Auth struct {
	db       *gorm.DB
	sessions *Sessions
}

func NewAuth(db *gorm.DB, sessions *Sessions) *Auth {
	return &Auth{db: db, sessions: sessions}
}

type RegisterInput struct {
	Account    string
	Password   string
	Nickname   string
	InviteCode string
	IP         string
}

// Register 在单个事务里消耗邀请码并创建用户：邀请码必须存在、未使用、未过期。
func (a *Auth) Register(ctx context.Context, in RegisterInput) (*model.User, error) {
	in.Account = strings.TrimSpace(in.Account)
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.InviteCode = strings.TrimSpace(in.InviteCode)

	if !accountPattern.MatchString(in.Account) {
		return nil, ErrBadAccount
	}
	if len([]rune(in.Password)) < 8 || len(in.Password) > maxPasswordLen {
		return nil, ErrBadPassword
	}
	if n := len([]rune(in.Nickname)); n < 1 || n > 20 {
		return nil, ErrBadNickname
	}

	var user model.User
	err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// ① 先做原子占用：条件 UPDATE 是事务里的第一条语句，立刻拿到写锁。
		// 若按「先读后写」的顺序，SQLite WAL 下并发事务会因读快照过期直接报
		// BUSY（database is locked）——两个用户同时用同一码注册时一方会吃 500，
		// 攻击者可故意并发触发。这里从根上规避：写在前，读在后。
		res := tx.Model(&model.InviteCode{}).
			Where("code = ? AND used_count < max_uses AND expires_at > ?", in.InviteCode, now).
			Updates(map[string]any{
				"used_count": gorm.Expr("used_count + 1"),
				"used_at":    now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// 码不存在 / 已用尽 / 已过期；并发下也只会有一方占到
			return ErrInviteInvalid
		}

		var invite model.InviteCode
		if err := tx.First(&invite, "code = ?", in.InviteCode).Error; err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&model.User{}).Where("account = ?", in.Account).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrAccountTaken
		}

		hash, err := security.HashPassword(in.Password)
		if err != nil {
			return err
		}

		user = model.User{
			Account:      in.Account,
			PasswordHash: hash,
			Nickname:     in.Nickname,
			Role:         model.RoleUser,
			LastIP:       in.IP,
		}
		if err := tx.Create(&user).Error; err != nil {
			// 并发注册同一账号时唯一索引会拒绝，统一转成可读错误。
			if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
				return ErrAccountTaken
			}
			return err
		}

		return tx.Model(&model.InviteCode{}).Where("id = ?", invite.ID).
			Update("used_by", user.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Login 校验凭据，成功则刷新 last_ip 并由调用方下发会话。
func (a *Auth) Login(ctx context.Context, account, password, ip string) (*model.User, error) {
	account = strings.TrimSpace(account)

	var user model.User
	err := a.db.WithContext(ctx).First(&user, "account = ?", account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 仍然做一次哈希运算，避免通过响应时间区分"账号不存在"与"密码错误"。
		_ = security.VerifyPassword(password, dummyHash)
		return nil, ErrBadCredentials
	}
	if err != nil {
		return nil, err
	}
	if !security.VerifyPassword(password, user.PasswordHash) {
		return nil, ErrBadCredentials
	}

	if user.LastIP != ip {
		if err := a.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", user.ID).Update("last_ip", ip).Error; err != nil {
			return nil, err
		}
		user.LastIP = ip
	}
	return &user, nil
}

func (a *Auth) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	if err := a.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// ListUsers 供后台展示，按创建时间倒序。
func (a *Auth) ListUsers(ctx context.Context) ([]model.User, error) {
	var out []model.User
	err := a.db.WithContext(ctx).Order("id ASC").Find(&out).Error
	return out, err
}

// SetRole 仅允许 admin / user 两个值。
func (a *Auth) SetRole(ctx context.Context, id uint, role string) error {
	if role != model.RoleAdmin && role != model.RoleUser {
		return errors.New("角色只能是 admin 或 user")
	}
	res := a.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// EnsureAdmin 冷启动：账号不存在则用给定密码（为空则随机生成）创建管理员。
// 返回生成的明文密码（仅在本次创建时非空），由调用方打印一次到日志。
func (a *Auth) EnsureAdmin(ctx context.Context, account, password string) (generated string, created bool, err error) {
	var count int64
	if err = a.db.WithContext(ctx).Model(&model.User{}).Where("account = ?", account).Count(&count).Error; err != nil {
		return "", false, err
	}
	if count > 0 {
		return "", false, nil
	}

	if password == "" {
		if password, err = randomPassword(20); err != nil {
			return "", false, err
		}
		generated = password
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return "", false, err
	}
	user := model.User{
		Account:      account,
		PasswordHash: hash,
		Nickname:     account,
		Role:         model.RoleAdmin,
	}
	if err = a.db.WithContext(ctx).Create(&user).Error; err != nil {
		return "", false, err
	}
	return generated, true, nil
}

func randomPassword(n int) (string, error) {
	const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	out := make([]byte, n)
	for i := range out {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		out[i] = alphabet[idx.Int64()]
	}
	return string(out), nil
}

// dummyHash 是一个合法的 argon2id 哈希（明文为固定的随机串），
// 用于账号不存在时消耗等量 CPU 时间。
var dummyHash = func() string {
	h, err := security.HashPassword("dummy-password-for-timing")
	if err != nil {
		panic(fmt.Sprintf("初始化 dummyHash 失败: %v", err))
	}
	return h
}()
