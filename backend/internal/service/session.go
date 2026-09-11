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
type Sessions struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewSessions(rdb *redis.Client, ttl time.Duration) *Sessions {
	return &Sessions{rdb: rdb, ttl: ttl}
}

func (s *Sessions) key(id string) string { return "sess:" + id }

// Create 生成随机会话 ID 并写入 Redis。
func (s *Sessions) Create(ctx context.Context, userID uint) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	if err := s.rdb.Set(ctx, s.key(id), strconv.FormatUint(uint64(userID), 10), s.ttl).Err(); err != nil {
		return "", err
	}
	return id, nil
}

// UserID 读取会话对应的用户 ID。
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
