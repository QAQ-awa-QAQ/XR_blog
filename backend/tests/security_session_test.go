package tests

// 安全测试 · 会话与认证攻面。
//
// 覆盖的攻击路径（每条都对应一个真实的突破尝试）：
//   - 会话 ID 可预测 / 可伪造 / 可遍历
//   - 直接向 Redis 注入篡改的会话值（类型混淆、溢出、哨兵 0 提权）
//   - 会话固定（fixation）：登录必须换发新 ID
//   - Cookie 属性缺失（HttpOnly / SameSite / Secure）
//   - 登出无 CSRF（跨站强制登出）与会话 Cookie 未清理
//   - 登录/注册的注入载荷、畸形 JSON、类型混淆
//   - 密码与昵称的边界、响应中的密码泄露

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/config"
	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// loginAs 用完整登录流程换取会话与 CSRF 凭据。
// 注意每个 IP 只登录一次，避免触发 IPGuard 的冷却（调用方负责换 IP）。
func loginAs(t *testing.T, engine *gin.Engine, account, password, ip string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	rec := call(t, engine, http.MethodPost, "/api/auth/login", ip, map[string]string{
		"account":  account,
		"password": password,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("登录 %s 失败: %d %s", account, rec.Code, rec.Body.String())
	}
	return findCookie(t, rec, httpx.SessionCookie), findCookie(t, rec, httpx.CSRFCookie)
}

// guestCreds 走游客入口换取会话与 CSRF 凭据。
func guestCreds(t *testing.T, engine *gin.Engine, ip string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	rec := call(t, engine, http.MethodPost, "/api/auth/guest", ip, nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("游客入口失败: %d %s", rec.Code, rec.Body.String())
	}
	return findCookie(t, rec, httpx.SessionCookie), findCookie(t, rec, httpx.CSRFCookie)
}

// forgeSessionCookie 构造携带任意伪造会话值的请求装饰器。
func forgeSessionCookie(value string) func(*http.Request) {
	return func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: httpx.SessionCookie, Value: value})
	}
}

// Cookie 安全属性：session 必须 HttpOnly + SameSite=Lax；csrf 必须可读（非 HttpOnly）。
// 缺 HttpOnly 会被 XSS 直接偷会话；缺 SameSite 会被跨站请求携带。
func TestSecuritySessionCookieAttributes(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodPost, "/api/auth/guest", "198.51.100.70", nil, nil)
	session := findCookie(t, rec, httpx.SessionCookie)
	csrf := findCookie(t, rec, httpx.CSRFCookie)

	if !session.HttpOnly {
		t.Fatal("session Cookie 必须 HttpOnly（防 XSS 读取会话）")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session Cookie 应为 SameSite=Lax，实际 %v", session.SameSite)
	}
	if session.Path != "/" {
		t.Fatalf("session Cookie Path 应为 /，实际 %q", session.Path)
	}
	if csrf.HttpOnly {
		t.Fatal("csrf Cookie 必须可被前端 JS 读取（HttpOnly 会破坏 double-submit）")
	}
	if csrf.SameSite != http.SameSiteLaxMode {
		t.Fatalf("csrf Cookie 应为 SameSite=Lax，实际 %v", csrf.SameSite)
	}
	if session.Secure || csrf.Secure {
		t.Fatal("COOKIE_SECURE=false 的测试环境不应带 Secure")
	}
}

// COOKIE_SECURE=true（生产默认）时两个 Cookie 都必须带 Secure，防明文信道嗅探。
func TestSecuritySecureFlagHonored(t *testing.T) {
	e := newEnv(t, func(cfg *config.Config) { cfg.CookieSecure = true })
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodPost, "/api/auth/guest", "198.51.100.71", nil, nil)
	if !findCookie(t, rec, httpx.SessionCookie).Secure {
		t.Fatal("COOKIE_SECURE=true 时 session Cookie 必须带 Secure")
	}
	if !findCookie(t, rec, httpx.CSRFCookie).Secure {
		t.Fatal("COOKIE_SECURE=true 时 csrf Cookie 必须带 Secure")
	}
}

// 会话 ID 必须不可预测：256bit 随机、hex 编码、绝不重复。
func TestSecuritySessionIDUnpredictable(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	seen := make(map[string]bool, 16)
	for i := 0; i < 16; i++ {
		sid, err := e.Sessions.Create(ctx, 1)
		if err != nil {
			t.Fatalf("创建会话失败: %v", err)
		}
		if len(sid) != 64 {
			t.Fatalf("会话 ID 应为 64 位 hex（256bit），实际 %d 位：%s", len(sid), sid)
		}
		if _, err := hex.DecodeString(sid); err != nil {
			t.Fatalf("会话 ID 必须是 hex 编码: %v", err)
		}
		if seen[sid] {
			t.Fatal("会话 ID 出现重复——随机源不可用")
		}
		seen[sid] = true
	}
}

// 任意伪造的会话 Cookie 都必须 401：随机值、边界值、注入形态都不例外。
func TestSecurityForgedSessionCookieRejected(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := context.Background()

	good, err := e.Sessions.Create(ctx, 1)
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	// 把真实 ID 改一位：最难以肉眼分辨的伪造
	flip := byte('0')
	if good[0] == '0' {
		flip = '1'
	}

	forge := []string{
		"",                         // 空值
		"deadbeef",                 // 过短
		strings.Repeat("a", 64),    // 长度正确但内容随机
		strings.Repeat("f", 10000), // 超长
		string(flip) + good[1:],    // 真实 ID 改一位
		"../../etc/passwd",         // 路径形态
		"sess:whatever",            // 内部键名形态
		"'; DROP TABLE users;--",   // SQL 注入形态
		"中文会话值",                    // 非 ASCII
	}

	for _, v := range forge {
		rec := call(t, engine, http.MethodGet, "/api/me", "198.51.100.72", nil, forgeSessionCookie(v))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("伪造会话 %q 应 401，实际 %d：%s", v, rec.Code, rec.Body.String())
		}
	}
}

// 即便攻击者能直接向 Redis 写会话值（例如 SSRF 访问 Redis），也不得提权：
// 非数字/负值/溢出一律 401；哨兵 "0" 只能得到游客身份。
func TestSecurityRedisSessionValueTampering(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := context.Background()

	// 非数字或超出 uint64 的值 → 一律无效
	tampered := []string{
		"abc", "1e3", "-1", " 42", "42 ", "0x10", "1.5",
		"",                     // 空串
		"18446744073709551616", // uint64 溢出
		"18446744073709551615", // MaxUint64：解析成功但不存在该用户
	}
	for i, v := range tampered {
		id := fmt.Sprintf("tampered-%d", i)
		if err := e.RDBC.Set(ctx, "sess:"+id, v, time.Hour).Err(); err != nil {
			t.Fatalf("注入会话值失败: %v", err)
		}
		rec := call(t, engine, http.MethodGet, "/api/me", "198.51.100.73", nil, forgeSessionCookie(id))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("会话值 %q 应 401，实际 %d", v, rec.Code)
		}
	}

	// 值 "0" 是游客哨兵（合法存在）：只能得到 guest 身份，绝不能拿到管理员能力
	if err := e.RDBC.Set(ctx, "sess:tampered-zero", "0", time.Hour).Err(); err != nil {
		t.Fatalf("注入哨兵失败: %v", err)
	}
	rec := call(t, engine, http.MethodGet, "/api/me", "198.51.100.73", nil, forgeSessionCookie("tampered-zero"))
	if rec.Code != http.StatusOK {
		t.Fatalf("哨兵会话应放行只读接口，实际 %d", rec.Code)
	}
	user := decode(t, rec)
	if user["role"] != "guest" || int(user["id"].(float64)) != 0 {
		t.Fatalf("哨兵必须解析为游客(id=0)，实际 %v", user)
	}
	rec = call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.73", nil, forgeSessionCookie("tampered-zero"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("哨兵会话访问管理接口必须 403，实际 %d", rec.Code)
	}
}

// 会话固定（fixation）防御：受害者带着攻击者预置的会话 ID 登录后，
// 必须换发新 ID；攻击者手里的旧 ID 依然只是游客。
func TestSecuritySessionFixationRotates(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	// 攻击者先建一个自己已知的游客会话
	attackerSess, _ := guestCreds(t, engine, "198.51.100.74")

	// 受害者"带着这个预置 Cookie"完成登录
	reg := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.75", map[string]string{
		"account":    "fixation_victim",
		"password":   "password123",
		"nickname":   "受害者",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if reg.Code != http.StatusOK {
		t.Fatalf("注册失败: %d", reg.Code)
	}
	login := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.76", map[string]string{
		"account":  "fixation_victim",
		"password": "password123",
	}, forgeSessionCookie(attackerSess.Value))
	if login.Code != http.StatusOK {
		t.Fatalf("登录失败: %d", login.Code)
	}

	newSess := findCookie(t, login, httpx.SessionCookie)
	if newSess.Value == attackerSess.Value {
		t.Fatal("登录必须换发新会话 ID（防会话固定）")
	}

	// 攻击者拿着旧 ID：依然只是游客
	rec := call(t, engine, http.MethodGet, "/api/me", "198.51.100.74", nil, forgeSessionCookie(attackerSess.Value))
	if role := decode(t, rec)["role"]; role != "guest" {
		t.Fatalf("攻击者旧会话不得升级为登录态，实际角色 %v", role)
	}
	// 受害者用新 ID：是本人
	rec = call(t, engine, http.MethodGet, "/api/me", "198.51.100.76", nil, forgeSessionCookie(newSess.Value))
	if account := decode(t, rec)["account"]; account != "fixation_victim" {
		t.Fatalf("新会话应属于受害者，实际 %v", account)
	}
}

// 登出必须校验 CSRF：防跨站表单强制登出（低危但要堵死）；成功后会话立即销毁。
func TestSecurityLogoutRequiresCSRF(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.77", map[string]string{
		"account":    "logout_user",
		"password":   "password123",
		"nickname":   "登出",
		"inviteCode": e.seedInvite(t),
	}, nil)
	session := findCookie(t, rec, httpx.SessionCookie)
	csrf := findCookie(t, rec, httpx.CSRFCookie)

	// 只有会话、没有 CSRF 头 → 拒绝，且会话不能被动掉
	rec = call(t, engine, http.MethodPost, "/api/auth/logout", "198.51.100.77", nil, withCookie(session))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("无 CSRF 的登出应 403，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "csrf" {
		t.Fatalf("错误码应为 csrf，实际 %v", code)
	}
	rec = call(t, engine, http.MethodGet, "/api/auth/session", "198.51.100.77", nil, withCookie(session))
	if rec.Code != http.StatusOK {
		t.Fatalf("被拒的登出不得影响会话，实际 %d", rec.Code)
	}

	// 头与 Cookie 不一致（伪造头）→ 拒绝
	rec = call(t, engine, http.MethodPost, "/api/auth/logout", "198.51.100.77", nil, func(req *http.Request) {
		req.AddCookie(session)
		req.AddCookie(csrf)
		req.Header.Set(httpx.CSRFHeader, "forged-token")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("CSRF 头与 Cookie 不一致应 403，实际 %d", rec.Code)
	}

	// 正确配对 → 登出成功，会话立即失效
	rec = call(t, engine, http.MethodPost, "/api/auth/logout", "198.51.100.77", nil, withCredentials(session, csrf))
	if rec.Code != http.StatusOK {
		t.Fatalf("正常登出应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	rec = call(t, engine, http.MethodGet, "/api/auth/session", "198.51.100.77", nil, withCookie(session))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("登出后会话应立即失效，实际 %d", rec.Code)
	}
}

// 登出响应必须把会话 Cookie 清空（否则浏览器会持续携带死 Cookie）。
func TestSecurityLogoutClearsCookies(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	session, csrf := guestCreds(t, engine, "198.51.100.78")
	rec := call(t, engine, http.MethodPost, "/api/auth/logout", "198.51.100.78", nil, withCredentials(session, csrf))
	if rec.Code != http.StatusOK {
		t.Fatalf("登出失败: %d", rec.Code)
	}
	cleared := findCookie(t, rec, httpx.SessionCookie)
	if cleared.Value != "" {
		t.Fatalf("登出必须清空会话 Cookie，实际值 %q", cleared.Value)
	}
}

// 响应体绝不携带密码或哈希：注册、登录、me 三个出口逐一检查。
func TestSecurityAuthResponsesHidePassword(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	const plain = "super-secret-password-123"
	rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.79", map[string]string{
		"account":    "sec_hash_user",
		"password":   plain,
		"nickname":   "哈希",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", rec.Code, rec.Body.String())
	}
	session := findCookie(t, rec, httpx.SessionCookie)

	for name, body := range map[string]string{
		"注册响应": rec.Body.String(),
	} {
		for _, marker := range []string{plain, "$argon2", "argon2id", "passwordHash", "PasswordHash"} {
			if strings.Contains(body, marker) {
				t.Fatalf("%s 泄露 %q", name, marker)
			}
		}
	}

	rec = call(t, engine, http.MethodGet, "/api/me", "198.51.100.79", nil, withCookie(session))
	meBody := rec.Body.String()
	for _, marker := range []string{plain, "$argon2", "passwordHash", "PasswordHash", "password"} {
		if strings.Contains(meBody, marker) {
			t.Fatalf("/api/me 泄露 %q: %s", marker, meBody)
		}
	}

	// 数据库里必须是 argon2id PHC 哈希，绝无明文
	var u model.User
	if err := e.DB.First(&u, "account = ?", "sec_hash_user").Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}
	if u.PasswordHash == plain {
		t.Fatal("数据库里不能存明文密码")
	}
	if !strings.HasPrefix(u.PasswordHash, "$argon2id$") {
		t.Fatalf("密码必须以 argon2id 哈希存储，实际 %q", u.PasswordHash)
	}
}

// 登录查询必须参数化：SQL 注入载荷只会得到普通的凭据错误，表结构完好。
func TestSecurityLoginInjectionAttempts(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := context.Background()

	payloads := []string{
		"' OR '1'='1' --",
		"admin'--",
		"'; DROP TABLE users; --",
		`" OR ""="`,
		"admin\x00",
		"a' OR 1=1#",
	}
	for i, p := range payloads {
		// 每个载荷换一个来源 IP，避免触发限流遮蔽断言
		rec := call(t, engine, http.MethodPost, "/api/auth/login", fmt.Sprintf("203.0.113.%d", 150+i), map[string]string{
			"account":  p,
			"password": "x",
		}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("注入载荷 %q 应得到 401，实际 %d", p, rec.Code)
		}
		if code := decode(t, rec)["code"]; code != "bad_credentials" {
			t.Fatalf("错误码应为 bad_credentials，实际 %v", code)
		}
	}

	// users 表必须还在（DROP 未执行）
	var count int64
	if err := e.DB.Model(&model.User{}).Count(&count).Error; err != nil {
		t.Fatalf("users 表不可查询，注入攻击可能已生效: %v", err)
	}

	// 注册侧：账号格式正则必须拦住注入形态
	for _, bad := range []string{"admin'--", "a b", "a;b", "дроп", "x%（"} {
		if _, err := e.Auth.Register(ctx, service.RegisterInput{
			Account:    bad,
			Password:   "password123",
			Nickname:   "注入",
			InviteCode: e.seedInvite(t),
			IP:         "198.51.100.80",
		}); err == nil {
			t.Fatalf("非法账号 %q 必须被拒绝", bad)
		}
	}
}

// 畸形请求体：任何解析失败都必须 4xx，绝不允许 5xx 或意外放行。
func TestSecurityAuthPayloadFuzz(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rawCall := func(ip, ctype, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = ip + ":54321"
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec
	}

	deep := strings.Repeat(`{"a":`, 50) + "1" + strings.Repeat("}", 50)
	cases := []struct{ name, ctype, body string }{
		{"纯文本体", "application/json", "not-json-at-all"},
		{"空体", "application/json", ""},
		{"数组体", "application/json", "[1,2,3]"},
		{"字符串体", "application/json", `"just-a-string"`},
		{"null 体", "application/json", "null"},
		{"账号为数字", "application/json", `{"account":123,"password":"x"}`},
		{"账号为对象", "application/json", `{"account":{"$ne":null},"password":"x"}`},
		{"账号为数组", "application/json", `{"account":["a"],"password":"x"}`},
		{"表单编码", "application/x-www-form-urlencoded", "account=a&password=b"},
		{"深层嵌套", "application/json", deep},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := rawCall(fmt.Sprintf("203.0.113.%d", 170+i), tc.ctype, tc.body)
			if rec.Code < 400 || rec.Code >= 500 {
				t.Fatalf("畸形载荷应被 4xx 拒绝，实际 %d：%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 注册字段边界：密码 8-128 位、昵称 1-20 字之外全部拒绝；超大输入必须快速拒绝而非卡死。
func TestSecurityPasswordBoundsHTTP(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	cases := []struct {
		name     string
		account  string
		password string
		nickname string
		want     int
	}{
		{"密码 7 位被拒", "sec_b7", strings.Repeat("a", 7), "昵称", http.StatusBadRequest},
		{"密码 8 位通过", "sec_b8", strings.Repeat("a", 8), "昵称", http.StatusOK},
		{"密码 128 位通过", "sec_b128", strings.Repeat("a", 128), "昵称", http.StatusOK},
		{"密码 129 位被拒", "sec_b129", strings.Repeat("a", 129), "昵称", http.StatusBadRequest},
		{"密码 64KB 被拒", "sec_big", strings.Repeat("a", 64*1024), "昵称", http.StatusBadRequest},
		{"昵称 21 字被拒", "sec_n21", "password123", strings.Repeat("名", 21), http.StatusBadRequest},
		{"账号 21 位被拒", strings.Repeat("a", 21), "password123", "昵称", http.StatusBadRequest},
		{"账号含符号被拒", "sec.bad", "password123", "昵称", http.StatusBadRequest},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := call(t, engine, http.MethodPost, "/api/auth/register", fmt.Sprintf("203.0.113.%d", 190+i), map[string]string{
				"account":    tc.account,
				"password":   tc.password,
				"nickname":   tc.nickname,
				"inviteCode": e.seedInvite(t),
			}, nil)
			if rec.Code != tc.want {
				t.Fatalf("预期 %d，实际 %d：%s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// 昵称里的 HTML/脚本载荷必须原样存储（前端负责转义）且不破坏 API。
func TestSecurityNicknameHTMLVerbatim(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	const nick = `<svg/onload=x>` // 15 字符，位于长度上限内且具备 XSS 形态
	rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.81", map[string]string{
		"account":    "sec_xss_user",
		"password":   "password123",
		"nickname":   nick,
		"inviteCode": e.seedInvite(t),
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", rec.Code, rec.Body.String())
	}
	session := findCookie(t, rec, httpx.SessionCookie)

	rec = call(t, engine, http.MethodGet, "/api/me", "198.51.100.81", nil, withCookie(session))
	if got := decode(t, rec)["nickname"]; got != nick {
		t.Fatalf("昵称应原样返回，实际 %q", got)
	}

	var u model.User
	if err := e.DB.First(&u, "account = ?", "sec_xss_user").Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}
	if u.Nickname != nick {
		t.Fatalf("昵称应原样入库，实际 %q", u.Nickname)
	}
}
