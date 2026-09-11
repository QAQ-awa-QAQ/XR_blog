package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"personal_blog/internal/config"
	"personal_blog/internal/model"
)

// Verdict 是一次登录/注册尝试的判定结果。
type Verdict int

const (
	VerdictOK Verdict = iota
	VerdictCooldown
	VerdictBanned
)

// Guard 实现 design.md 第 4 章的限流与封禁规则。
// 计数使用 Redis ZSET 滑动窗口（毫秒精度），封禁名单落 SQLite 以便重启不丢。
type Guard struct {
	rdb *redis.Client
	db  *gorm.DB
	cfg config.Config
}

func NewGuard(rdb *redis.Client, db *gorm.DB, cfg config.Config) *Guard {
	return &Guard{rdb: rdb, db: db, cfg: cfg}
}

func cooldownKey(ip string) string { return "cd:" + ip }
func attemptKey(ip string) string  { return "auth:" + ip }

// ActiveBan 返回该 IP 当前生效的封禁记录；未封禁（或已到期）返回 nil。
func (g *Guard) ActiveBan(ctx context.Context, ip string) (*model.BanRecord, error) {
	var rec model.BanRecord
	err := g.db.WithContext(ctx).First(&rec, "ip = ?", ip).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !rec.Active(time.Now()) {
		return nil, nil
	}
	return &rec, nil
}

// Cooldown 返回剩余冷却时长，0 表示不在冷却中。
func (g *Guard) Cooldown(ctx context.Context, ip string) time.Duration {
	ttl, err := g.rdb.TTL(ctx, cooldownKey(ip)).Result()
	if err != nil || ttl <= 0 {
		return 0
	}
	return ttl
}

// Attempt 记录一次尝试并判定。
// 注意：即使正处于冷却期也必须照常计数——否则 20 次阈值永远无法达到，
// design.md 4.2 就成了死条文。
func (g *Guard) Attempt(ctx context.Context, ip string) (Verdict, *model.BanRecord, time.Duration, error) {
	now := time.Now()
	nowMs := now.UnixMilli()
	key := attemptKey(ip)

	member := strconv.FormatInt(nowMs, 10) + "-" + strconv.FormatUint(randUint64(), 10)
	if err := g.rdb.ZAdd(ctx, key, redis.Z{Score: float64(nowMs), Member: member}).Err(); err != nil {
		return VerdictOK, nil, 0, err
	}

	// 窗口之外的记录直接裁掉，避免集合无限增长。
	banWindowStart := strconv.FormatInt(now.Add(-g.cfg.BanWindow).UnixMilli(), 10)
	g.rdb.ZRemRangeByScore(ctx, key, "-inf", "("+banWindowStart)
	g.rdb.Expire(ctx, key, g.cfg.BanWindow+time.Minute)

	banCount, err := g.rdb.ZCount(ctx, key, banWindowStart, "+inf").Result()
	if err != nil {
		return VerdictOK, nil, 0, err
	}

	// 先判封禁，再判冷却：达到 20 次直接升级，封禁优先于冷却。
	if banCount >= int64(g.cfg.BanLimit) {
		rec, err := g.escalate(ctx, ip, now)
		if err != nil {
			return VerdictOK, nil, 0, err
		}
		g.rdb.Del(ctx, key)
		return VerdictBanned, rec, 0, nil
	}

	coolWindowStart := strconv.FormatInt(now.Add(-g.cfg.CooldownWindow).UnixMilli(), 10)
	coolCount, err := g.rdb.ZCount(ctx, key, coolWindowStart, "+inf").Result()
	if err != nil {
		return VerdictOK, nil, 0, err
	}
	if coolCount >= int64(g.cfg.CooldownLimit) {
		ttl := g.Cooldown(ctx, ip)
		if ttl <= 0 {
			g.rdb.Set(ctx, cooldownKey(ip), "1", g.cfg.CooldownDuration)
			ttl = g.cfg.CooldownDuration
		}
		return VerdictCooldown, nil, ttl, nil
	}

	return VerdictOK, nil, 0, nil
}

// MarkBanned 由管理员手动封禁时写入，保持与自动封禁同一套记录。
func (g *Guard) MarkBanned(ctx context.Context, ip, reason string, permanent bool) (*model.BanRecord, error) {
	return g.upsertBan(ctx, ip, reason, permanent, time.Now())
}

// escalate 触犯次数 +1：首次 24h 限时，第二次起永久。
func (g *Guard) escalate(ctx context.Context, ip string, now time.Time) (*model.BanRecord, error) {
	return g.upsertBan(ctx, ip, "自动封禁：20 秒内登录/注册尝试超过阈值", false, now)
}

func (g *Guard) upsertBan(ctx context.Context, ip, reason string, permanent bool, now time.Time) (*model.BanRecord, error) {
	var rec model.BanRecord
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.First(&rec, "ip = ?", ip).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			rec = model.BanRecord{IP: ip}
		case err != nil:
			return err
		}

		rec.OffenseCount++
		if permanent || rec.OffenseCount >= 2 {
			rec.Level = model.BanLevelPermanent
			rec.ExpiresAt = nil
		} else {
			rec.Level = model.BanLevelTimed
			expires := now.Add(g.cfg.BanDuration)
			rec.ExpiresAt = &expires
		}
		rec.Reason = reason
		return tx.Save(&rec).Error
	})
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// Unban 解除当前封禁但保留触犯次数（再犯仍升级为永久）。
func (g *Guard) Unban(ctx context.Context, ip string) error {
	now := time.Now()
	res := g.db.WithContext(ctx).Model(&model.BanRecord{}).
		Where("ip = ?", ip).
		Updates(map[string]any{"level": model.BanLevelTimed, "expires_at": now, "reason": "管理员手动解封"})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	g.rdb.Del(ctx, cooldownKey(ip))
	return nil
}

// Reset 彻底删除该 IP 的记录，触犯次数归零。
func (g *Guard) Reset(ctx context.Context, ip string) error {
	res := g.db.WithContext(ctx).Where("ip = ?", ip).Delete(&model.BanRecord{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	g.rdb.Del(ctx, cooldownKey(ip), attemptKey(ip))
	return nil
}

// ListBans 按最近更新时间倒序列出封禁记录。
func (g *Guard) ListBans(ctx context.Context) ([]model.BanRecord, error) {
	var out []model.BanRecord
	err := g.db.WithContext(ctx).Order("updated_at DESC").Find(&out).Error
	return out, err
}

// BanSummary 供封禁页展示，避免 handler 直接拼装模型。
type BanSummary struct {
	IP        string
	Permanent bool
	Remaining time.Duration
	Reason    string
}

func (g *Guard) Summary(rec *model.BanRecord) BanSummary {
	s := BanSummary{IP: rec.IP, Reason: rec.Reason, Permanent: rec.Level == model.BanLevelPermanent}
	if !s.Permanent && rec.ExpiresAt != nil {
		if d := time.Until(*rec.ExpiresAt); d > 0 {
			s.Remaining = d
		}
	}
	return s
}

func randUint64() uint64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint64(time.Now().UnixNano())
	}
	return binary.BigEndian.Uint64(b[:])
}
