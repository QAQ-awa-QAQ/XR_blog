package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// 会话只存 user_id；账号/昵称/IP 一律回查数据库，密码永不下发客户端。
// 游客会话存哨兵值 0：user_id 自增从 1 起，库里不存在 0，
// 因此解析出 0 即表示“没有用户行的游客”（见 CreateGuest）。
type Sessions struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessions(rdb *redis.Client, ttl time.Duration) *Sessions {
	return &Sessions{rdb: rdb, ttl: ttl}
}

func (s *Sessions) key(id string) string { return "sess:" + id }

// newID 生成随机会话 ID。
func (s *Sessions) newID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Create 生成随机会话 ID 并写入 Redis。
func (s *Sessions) Create(ctx context.Context, userID uint) (string, error) {
	id, err := s.newID()
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, s.key(id), strconv.FormatUint(uint64(userID), 10), s.ttl).Err(); err != nil {
		return "", err
	}
	return id, nil
}

// CreateGuest 创建游客会话：没有对应用户行，只代表“通过闸门的匿名访客”，
// 权限仅限登录态只读接口；管理接口由 RequireAdmin 的角色判定兜底拒绝。
func (s *Sessions) CreateGuest(ctx context.Context) (string, error) {
	id, err := s.newID()
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, s.key(id), "0", s.ttl).Err(); err != nil {
		return "", err
	}
	return id, nil
}

// UserID 读取会话对应的用户 ID。游客会话按哨兵 0 返回 (0, true)，
// 与“无效会话 (0, false)”区分开。
func (s *Sessions) UserID(ctx context.Context, id string) (uint, bool) {
	if id == "" {
		return 0, false
	}
	v, err := s.rdb.Get(ctx, s.key(id)).Result()
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(n), true
}

// Touch 滑动续期：有操作就延长有效期。
func (s *Sessions) Touch(ctx context.Context, id string) {
	if id != "" {
		s.rdb.Expire(ctx, s.key(id), s.ttl)
	}
}

func (s *Sessions) Destroy(ctx context.Context, id string) {
	if id != "" {
		s.rdb.Del(ctx, s.key(id))
	}
}
