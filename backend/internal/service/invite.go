package service

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	"gorm.io/gorm"

	"personal_blog/internal/model"
)

// Invite 管理邀请码。注册必须携带有效邀请码（用户已确认）。
type Invite struct {
	db  *gorm.DB
	ttl time.Duration
}

func NewInvite(db *gorm.DB, ttl time.Duration) *Invite {
	return &Invite{db: db, ttl: ttl}
}

// 去掉易混淆字符（0/O/1/I/l），便于口头或手抄传递。
const inviteAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func (s *Invite) Create(ctx context.Context, createdBy uint) (*model.InviteCode, error) {
	for attempt := 0; attempt < 5; attempt++ {
		code, err := randomCode(16)
		if err != nil {
			return nil, err
		}
		invite := model.InviteCode{
			Code:      code,
			CreatedBy: createdBy,
			ExpiresAt: time.Now().Add(s.ttl),
		}
		err = s.db.WithContext(ctx).Create(&invite).Error
		if err == nil {
			return &invite, nil
		}
		if !isUniqueViolation(err) {
			return nil, err
		}
		// 撞码就重试。
	}
	return nil, errors.New("生成邀请码失败，请重试")
}

func (s *Invite) List(ctx context.Context) ([]model.InviteCode, error) {
	var out []model.InviteCode
	err := s.db.WithContext(ctx).Order("id DESC").Find(&out).Error
	return out, err
}

// Revoke 只能撤销尚未使用的邀请码。
func (s *Invite) Revoke(ctx context.Context, id uint) error {
	res := s.db.WithContext(ctx).
		Where("id = ? AND used_at IS NULL", id).
		Delete(&model.InviteCode{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func randomCode(n int) (string, error) {
	out := make([]byte, n)
	for i := range out {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteAlphabet))))
		if err != nil {
			return "", err
		}
		out[i] = inviteAlphabet[idx.Int64()]
	}
	return string(out), nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for i := 0; i+6 <= len(msg); i++ {
		if msg[i:i+6] == "UNIQUE" {
			return true
		}
	}
	return false
}
