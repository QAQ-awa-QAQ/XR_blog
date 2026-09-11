package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"personal_blog/internal/config"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// 对应 design.md 4.1：10 秒内 3 次以上触发冷却。
func TestCooldownTriggersOnThirdAttempt(t *testing.T) {
	e := newEnv(t)
	const ip = "203.0.113.10"

	for i := 1; i <= 2; i++ {
		if v := e.attempt(t, ip); v != service.VerdictOK {
			t.Fatalf("第 %d 次尝试不应触发限制，实际得到 %v", i, v)
		}
	}

	verdict, _, cooldown, err := e.Guard.Attempt(context.Background(), ip)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if verdict != service.VerdictCooldown {
		t.Fatalf("第 3 次尝试应触发冷却，实际得到 %v", verdict)
	}
	if cooldown <= 0 || cooldown > e.Cfg.CooldownDuration {
		t.Fatalf("冷却时长应在 (0, %v] 区间，实际 %v", e.Cfg.CooldownDuration, cooldown)
	}
}

// 冷却期内必须继续计数——否则 4.2 的 20 次阈值永远无法达到。
func TestCooldownKeepsCountingUntilBan(t *testing.T) {
	e := newEnv(t)
	const ip = "203.0.113.20"

	for i := 1; i < e.Cfg.BanLimit; i++ {
		verdict := e.attempt(t, ip)
		if verdict == service.VerdictBanned {
			t.Fatalf("第 %d 次尝试不应封禁（阈值 %d）", i, e.Cfg.BanLimit)
		}
	}

	verdict, record, _, err := e.Guard.Attempt(context.Background(), ip)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if verdict != service.VerdictBanned {
		t.Fatalf("第 %d 次尝试应触发封禁，实际得到 %v", e.Cfg.BanLimit, verdict)
	}
	if record == nil {
		t.Fatal("封禁时应返回记录")
	}
	if record.OffenseCount != 1 {
		t.Fatalf("首次封禁的触犯次数应为 1，实际 %d", record.OffenseCount)
	}
	if record.Level != model.BanLevelTimed {
		t.Fatalf("首次封禁应为限时档，实际 level=%d", record.Level)
	}
	if record.ExpiresAt == nil {
		t.Fatal("限时封禁必须有到期时间")
	}
	if remaining := time.Until(*record.ExpiresAt); remaining < e.Cfg.BanDuration-time.Minute {
		t.Fatalf("封禁时长应接近 %v，实际剩余 %v", e.Cfg.BanDuration, remaining)
	}
}

// 再犯永久：解封后再次触发阈值，应升级为永久封禁。
func TestSecondOffenseBecomesPermanent(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const ip = "203.0.113.30"

	for i := 0; i < e.Cfg.BanLimit; i++ {
		e.attempt(t, ip)
	}

	first, err := e.Guard.ActiveBan(ctx, ip)
	if err != nil {
		t.Fatalf("查询封禁失败: %v", err)
	}
	if first == nil || first.Level != model.BanLevelTimed {
		t.Fatalf("预期首次为限时封禁，实际 %+v", first)
	}

	// 管理员解封：保留触犯次数
	if err := e.Guard.Unban(ctx, ip); err != nil {
		t.Fatalf("解封失败: %v", err)
	}
	if rec, err := e.Guard.ActiveBan(ctx, ip); err != nil || rec != nil {
		t.Fatalf("解封后不应再处于封禁状态，实际 %+v (err=%v)", rec, err)
	}

	for i := 0; i < e.Cfg.BanLimit; i++ {
		e.attempt(t, ip)
	}

	second, err := e.Guard.ActiveBan(ctx, ip)
	if err != nil {
		t.Fatalf("查询封禁失败: %v", err)
	}
	if second == nil {
		t.Fatal("再犯应再次被封禁")
	}
	if second.Level != model.BanLevelPermanent {
		t.Fatalf("再犯应永久封禁，实际 level=%d", second.Level)
	}
	if second.ExpiresAt != nil {
		t.Fatalf("永久封禁不应有到期时间，实际 %v", *second.ExpiresAt)
	}
	if second.OffenseCount != 2 {
		t.Fatalf("触犯次数应为 2，实际 %d", second.OffenseCount)
	}
}

// 限时封禁到期后自动失效，但触犯次数必须保留。
func TestTimedBanExpiresButKeepsOffenseCount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const ip = "203.0.113.40"

	if _, err := e.Guard.MarkBanned(ctx, ip, "测试", false); err != nil {
		t.Fatalf("手动封禁失败: %v", err)
	}

	// 把到期时间改到过去，模拟 24 小时之后
	past := time.Now().Add(-time.Second)
	if err := e.DB.Model(&model.BanRecord{}).Where("ip = ?", ip).Update("expires_at", past).Error; err != nil {
		t.Fatalf("调整到期时间失败: %v", err)
	}

	rec, err := e.Guard.ActiveBan(ctx, ip)
	if err != nil {
		t.Fatalf("查询封禁失败: %v", err)
	}
	if rec != nil {
		t.Fatalf("已过期的封禁不应生效，实际 %+v", rec)
	}

	var stored model.BanRecord
	if err := e.DB.First(&stored, "ip = ?", ip).Error; err != nil {
		t.Fatalf("记录不应被删除: %v", err)
	}
	if stored.OffenseCount != 1 {
		t.Fatalf("触犯次数应保留为 1，实际 %d", stored.OffenseCount)
	}
}

// 清除记录会把触犯次数归零，该 IP 再犯按首次处理。
func TestResetClearsHistory(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	const ip = "203.0.113.50"

	if _, err := e.Guard.MarkBanned(ctx, ip, "测试", true); err != nil {
		t.Fatalf("手动封禁失败: %v", err)
	}
	if err := e.Guard.Reset(ctx, ip); err != nil {
		t.Fatalf("清除记录失败: %v", err)
	}

	var count int64
	if err := e.DB.Model(&model.BanRecord{}).Where("ip = ?", ip).Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 0 {
		t.Fatalf("清除后不应有记录，实际 %d 条", count)
	}

	// 直接封禁一次，应为限时档（次数从 0 重新开始）
	rec, err := e.Guard.MarkBanned(ctx, ip, "再测", false)
	if err != nil {
		t.Fatalf("封禁失败: %v", err)
	}
	if rec.OffenseCount != 1 || rec.Level != model.BanLevelTimed {
		t.Fatalf("清除记录后应按首次处理，实际 %+v", rec)
	}
}

// 解封不存在的 IP 应返回 gorm.ErrRecordNotFound，便于 handler 转成 404。
func TestUnbanUnknownIP(t *testing.T) {
	e := newEnv(t)

	err := e.Guard.Unban(context.Background(), "203.0.113.99")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("预期 ErrRecordNotFound，实际 %v", err)
	}
}

// 不同 IP 的计数互不干扰。
func TestCountersArePerIP(t *testing.T) {
	e := newEnv(t)

	for i := 0; i < e.Cfg.CooldownLimit; i++ {
		e.attempt(t, "203.0.113.60")
	}

	if v := e.attempt(t, "203.0.113.61"); v != service.VerdictOK {
		t.Fatalf("另一个 IP 不应受影响，实际 %v", v)
	}
}

// 冷却是临时状态：冷却时长与统计窗口都过去之后应恢复放行。
//
// 说明：统计窗口用的是 Go 的墙钟时间，内存 Redis 的 FastForward 只推进键的 TTL，
// 因此这里把窗口与冷却都调短，用真实等待穿过窗口，再用 FastForward 过期冷却键。
func TestCooldownExpiresWithWindow(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) {
		cfg.CooldownWindow = 40 * time.Millisecond
		cfg.CooldownDuration = time.Second
	})
	ctx := context.Background()
	const ip = "203.0.113.70"

	for i := 0; i < e.Cfg.CooldownLimit; i++ {
		e.attempt(t, ip)
	}
	if ttl := e.Guard.Cooldown(ctx, ip); ttl <= 0 {
		t.Fatalf("应处于冷却中，实际 TTL=%v", ttl)
	}

	time.Sleep(e.Cfg.CooldownWindow + 80*time.Millisecond)
	e.Redis.FastForward(e.Cfg.CooldownDuration + time.Second)

	if ttl := e.Guard.Cooldown(ctx, ip); ttl != 0 {
		t.Fatalf("冷却应已结束，实际 TTL=%v", ttl)
	}
	if v := e.attempt(t, ip); v != service.VerdictOK {
		t.Fatalf("窗口过期后应恢复放行，实际 %v", v)
	}
}
