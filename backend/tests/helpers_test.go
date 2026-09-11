package tests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"personal_blog/internal/config"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
	"personal_blog/internal/store"
)

// env 是一套完整的测试依赖：真实 SQLite（临时文件）+ 内存 Redis。
type env struct {
	DB       *gorm.DB
	RDBC     *redis.Client
	Redis    *miniredis.Miniredis
	Cfg      config.Config
	Guard    *service.Guard
	Auth     *service.Auth
	Sessions *service.Sessions
	Invite   *service.Invite
}

// testConfig 与 config.Load() 的默认值保持一致，仅把会话/邀请码时长调短以便断言。
func testConfig() config.Config {
	return config.Config{
		SessionTTL:       time.Hour,
		InviteTTL:        time.Hour,
		CookieSecure:     false, // httptest 走 http，设 true 会拿不到 Cookie
		TrustedProxies:   []string{"127.0.0.1", "::1"},
		CooldownWindow:   10 * time.Second,
		CooldownLimit:    3,
		CooldownDuration: 10 * time.Second,
		BanWindow:        20 * time.Second,
		BanLimit:         20,
		BanDuration:      24 * time.Hour,
	}
}

func newEnv(t *testing.T, mutate ...func(*config.Config)) *env {
	t.Helper()

	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("初始化 SQLite 失败: %v", err)
	}
	// Windows 上文件句柄不释放会导致临时目录清理失败，必须显式关闭。
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	mini := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := testConfig()
	for _, apply := range mutate {
		apply(&cfg)
	}
	sessions := service.NewSessions(rdb, cfg.SessionTTL)

	return &env{
		DB:       db,
		RDBC:     rdb,
		Redis:    mini,
		Cfg:      cfg,
		Guard:    service.NewGuard(rdb, db, cfg),
		Auth:     service.NewAuth(db, sessions),
		Sessions: sessions,
		Invite:   service.NewInvite(db, cfg.InviteTTL),
	}
}

// seedInvite 生成一个可用邀请码，返回其明文。
func (e *env) seedInvite(t *testing.T) string {
	t.Helper()
	invite, err := e.Invite.Create(context.Background(), 1)
	if err != nil {
		t.Fatalf("生成邀请码失败: %v", err)
	}
	return invite.Code
}

// seedExpiredInvite 直接写库造一个已过期邀请码，用于覆盖过期分支。
func (e *env) seedExpiredInvite(t *testing.T) string {
	t.Helper()
	invite := model.InviteCode{
		Code:      "expiredcode1234",
		CreatedBy: 1,
		ExpiresAt: time.Now().Add(-time.Minute),
	}
	if err := e.DB.Create(&invite).Error; err != nil {
		t.Fatalf("写入过期邀请码失败: %v", err)
	}
	return invite.Code
}

// attempt 发起一次登录/注册尝试并返回判定结果，计数失败直接终止测试。
func (e *env) attempt(t *testing.T, ip string) service.Verdict {
	t.Helper()
	verdict, _, _, err := e.Guard.Attempt(context.Background(), ip)
	if err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	return verdict
}
