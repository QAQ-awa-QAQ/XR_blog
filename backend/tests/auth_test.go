package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"personal_blog/internal/security"
	"personal_blog/internal/service"
)

// argon2id 哈希必须能验证通过，并在任何畸形输入下安全返回 false。
func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := security.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if hash == "correct-horse-battery" {
		t.Fatal("绝不能明文存储密码")
	}
	if !security.VerifyPassword("correct-horse-battery", hash) {
		t.Fatal("正确密码应校验通过")
	}
	if security.VerifyPassword("wrong-password", hash) {
		t.Fatal("错误密码不应通过")
	}

	// 同一密码每次哈希应不同（随机盐）
	another, err := security.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("生成哈希失败: %v", err)
	}
	if hash == another {
		t.Fatal("相同密码的哈希应因随机盐而不同")
	}

	for _, malformed := range []string{"", "not-a-hash", "$argon2id$v=19$broken", "$argon2i$v=19$m=65536,t=1,p=4$c2FsdA$aGFzaA"} {
		if security.VerifyPassword("correct-horse-battery", malformed) {
			t.Fatalf("非法哈希 %q 不应通过校验", malformed)
		}
	}
}

// 注册必须携带有效邀请码（用户确认的选择）。
func TestRegisterRequiresValidInvite(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		code  string
		setup func(t *testing.T) string
	}{
		{name: "邀请码为空", code: "", setup: func(t *testing.T) string { return "" }},
		{name: "邀请码不存在", code: "nosuchcode123456", setup: func(t *testing.T) string { return "nosuchcode123456" }},
		{name: "邀请码已过期", code: "", setup: func(t *testing.T) string { return e.seedExpiredInvite(t) }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := tc.setup(t)
			_, err := e.Auth.Register(ctx, service.RegisterInput{
				Account:    "newuser01",
				Password:   "password123",
				Nickname:   "新人",
				InviteCode: code,
				IP:         "198.51.100.1",
			})
			if !errors.Is(err, service.ErrInviteInvalid) {
				t.Fatalf("预期 ErrInviteInvalid，实际 %v", err)
			}
		})
	}
}

// 邀请码一次性：用过之后不能再注册第二个账号。
func TestInviteIsSingleUse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	code := e.seedInvite(t)

	if _, err := e.Auth.Register(ctx, service.RegisterInput{
		Account: "firstuser", Password: "password123", Nickname: "一号", InviteCode: code, IP: "198.51.100.2",
	}); err != nil {
		t.Fatalf("持有效邀请码应注册成功: %v", err)
	}

	_, err := e.Auth.Register(ctx, service.RegisterInput{
		Account: "seconduser", Password: "password123", Nickname: "二号", InviteCode: code, IP: "198.51.100.3",
	})
	if !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("同一邀请码不应复用，实际 %v", err)
	}
}

// 注册成功后可以登录；密码错误与账号不存在都应返回同一个错误，避免枚举账号。
func TestLoginFlow(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	code := e.seedInvite(t)

	user, err := e.Auth.Register(ctx, service.RegisterInput{
		Account: "xr_user", Password: "password123", Nickname: "XR", InviteCode: code, IP: "198.51.100.4",
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if user.Role != "user" {
		t.Fatalf("注册用户角色应为 user，实际 %s", user.Role)
	}

	logged, err := e.Auth.Login(ctx, "xr_user", "password123", "198.51.100.9")
	if err != nil {
		t.Fatalf("正确凭据应登录成功: %v", err)
	}
	if logged.LastIP != "198.51.100.9" {
		t.Fatalf("登录应刷新最近 IP，实际 %s", logged.LastIP)
	}

	if _, err := e.Auth.Login(ctx, "xr_user", "bad-password", "198.51.100.9"); !errors.Is(err, service.ErrBadCredentials) {
		t.Fatalf("错误密码应返回 ErrBadCredentials，实际 %v", err)
	}
	if _, err := e.Auth.Login(ctx, "ghost_user", "password123", "198.51.100.9"); !errors.Is(err, service.ErrBadCredentials) {
		t.Fatalf("账号不存在应返回同一个错误，实际 %v", err)
	}
}

// 账号格式与密码强度校验。
func TestRegisterValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	cases := []struct {
		name  string
		input service.RegisterInput
		want  error
	}{
		{
			name:  "账号过短",
			input: service.RegisterInput{Account: "ab", Password: "password123", Nickname: "昵称"},
			want:  service.ErrBadAccount,
		},
		{
			name:  "账号含非法字符",
			input: service.RegisterInput{Account: "bad name!", Password: "password123", Nickname: "昵称"},
			want:  service.ErrBadAccount,
		},
		{
			name:  "密码过短",
			input: service.RegisterInput{Account: "gooduser", Password: "short", Nickname: "昵称"},
			want:  service.ErrBadPassword,
		},
		{
			name:  "昵称为空",
			input: service.RegisterInput{Account: "gooduser", Password: "password123", Nickname: "  "},
			want:  service.ErrBadNickname,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code := e.seedInvite(t)
			input := tc.input
			input.InviteCode = code
			input.IP = "198.51.100.5"

			if _, err := e.Auth.Register(ctx, input); !errors.Is(err, tc.want) {
				t.Fatalf("预期 %v，实际 %v", tc.want, err)
			}
		})
	}
}

// 同一账号只能注册一次。
func TestDuplicateAccountRejected(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	if _, err := e.Auth.Register(ctx, service.RegisterInput{
		Account: "dup_user", Password: "password123", Nickname: "一号", InviteCode: e.seedInvite(t), IP: "198.51.100.6",
	}); err != nil {
		t.Fatalf("首次注册应成功: %v", err)
	}

	_, err := e.Auth.Register(ctx, service.RegisterInput{
		Account: "dup_user", Password: "password123", Nickname: "二号", InviteCode: e.seedInvite(t), IP: "198.51.100.7",
	})
	if !errors.Is(err, service.ErrAccountTaken) {
		t.Fatalf("预期 ErrAccountTaken，实际 %v", err)
	}
}

// 会话：创建后可解析用户，销毁后立即失效，并支持滑动续期。
func TestSessionLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	sid, err := e.Sessions.Create(ctx, 42)
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}

	userID, ok := e.Sessions.UserID(ctx, sid)
	if !ok || userID != 42 {
		t.Fatalf("会话应解析出 user_id=42，实际 %d (ok=%v)", userID, ok)
	}

	e.Sessions.Touch(ctx, sid)
	if ttl := e.RDBC.TTL(ctx, "sess:"+sid).Val(); ttl <= 0 {
		t.Fatalf("续期后 TTL 应大于 0，实际 %v", ttl)
	}

	e.Sessions.Destroy(ctx, sid)
	if _, ok := e.Sessions.UserID(ctx, sid); ok {
		t.Fatal("销毁后会话不应再有效")
	}
}

// 游客会话的哨兵：CreateGuest 的会话解析为 (0, true)，
// 与“无效会话”的 (0, false) 区分开；同样支持续期与销毁。
func TestGuestSessionSentinel(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	sid, err := e.Sessions.CreateGuest(ctx)
	if err != nil {
		t.Fatalf("创建游客会话失败: %v", err)
	}

	userID, ok := e.Sessions.UserID(ctx, sid)
	if !ok || userID != 0 {
		t.Fatalf("游客会话应解析为 (0,true)，实际 (%d,%v)", userID, ok)
	}

	e.Sessions.Touch(ctx, sid)
	if ttl := e.RDBC.TTL(ctx, "sess:"+sid).Val(); ttl <= 0 {
		t.Fatalf("续期后 TTL 应大于 0，实际 %v", ttl)
	}

	e.Sessions.Destroy(ctx, sid)
	if _, ok := e.Sessions.UserID(ctx, sid); ok {
		t.Fatal("销毁后游客会话不应再有效")
	}
}

// 冷启动：管理员只创建一次，且不会覆盖已存在的账号。
func TestEnsureAdminCreatesOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	generated, created, err := e.Auth.EnsureAdmin(ctx, "admin", "")
	if err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	if !created || len(generated) < 16 {
		t.Fatalf("首次应创建并返回随机密码，实际 created=%v 长度=%d", created, len(generated))
	}

	admin, err := e.Auth.Login(ctx, "admin", generated, "198.51.100.8")
	if err != nil {
		t.Fatalf("生成的密码应可登录: %v", err)
	}
	if admin.Role != "admin" {
		t.Fatalf("冷启动账号应为管理员，实际 %s", admin.Role)
	}

	_, createdAgain, err := e.Auth.EnsureAdmin(ctx, "admin", "another-password")
	if err != nil {
		t.Fatalf("二次初始化不应报错: %v", err)
	}
	if createdAgain {
		t.Fatal("管理员已存在时不应重复创建")
	}
	if _, err := e.Auth.Login(ctx, "admin", "another-password", "198.51.100.8"); !errors.Is(err, service.ErrBadCredentials) {
		t.Fatal("已存在的管理员密码不应被覆盖")
	}
}

// 邀请码一码多用：用满 MaxUses 次之后才失效。
func TestInviteMultiUse(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	invite, err := e.Invite.Create(ctx, 1, 2, time.Hour)
	if err != nil {
		t.Fatalf("生成邀请码失败: %v", err)
	}
	for i, name := range []string{"multi_a", "multi_b"} {
		if _, err := e.Auth.Register(ctx, registerInput(name, invite.Code)); err != nil {
			t.Fatalf("第 %d 次注册应成功: %v", i+1, err)
		}
	}
	// 用尽后第三次必须失败
	if _, err := e.Auth.Register(ctx, registerInput("multi_c", invite.Code)); !errors.Is(err, service.ErrInviteInvalid) {
		t.Fatalf("用尽后应报邀请码失效，实际 %v", err)
	}
}

// 邀请码默认 7 天有效（config.InviteTTL 的默认值）；ttl 传 0 表示用默认。
func TestInviteDefaultTTL(t *testing.T) {
	e := newEnv(t)

	invite, err := e.Invite.Create(context.Background(), 1, 3, 0)
	if err != nil {
		t.Fatalf("生成邀请码失败: %v", err)
	}
	if remaining := time.Until(invite.ExpiresAt); remaining < e.Cfg.InviteTTL-time.Minute {
		t.Fatalf("有效期应接近 %v，实际剩余 %v", e.Cfg.InviteTTL, remaining)
	}
	if len(invite.Code) != 16 {
		t.Fatalf("邀请码应为 16 位，实际 %d", len(invite.Code))
	}
	if invite.MaxUses != 3 || invite.UsedCount != 0 {
		t.Fatalf("次数应记录为 3 次上限，实际 %d/%d", invite.UsedCount, invite.MaxUses)
	}
}
