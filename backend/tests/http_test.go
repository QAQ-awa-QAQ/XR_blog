package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/handler"
	"personal_blog/internal/httpx"
	"personal_blog/internal/router"
)

// newRouter 用测试依赖组装出与生产完全相同的路由与中间件链。
func (e *env) newRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	engine, err := router.New(router.Deps{
		Config:       e.Cfg,
		Auth:         e.Auth,
		Sessions:     e.Sessions,
		Guard:        e.Guard,
		AuthHandler:  handler.NewAuthHandler(e.Auth, e.Sessions, e.Cfg),
		AdminHandler: handler.NewAdminHandler(e.Invite, e.Guard, e.Auth),
	})
	if err != nil {
		t.Fatalf("初始化路由失败: %v", err)
	}
	return engine
}

// call 发起一次请求。ip 模拟客户端地址；mutate 用于附加 Cookie、CSRF 头等。
func call(t *testing.T, engine *gin.Engine, method, path, ip string, payload any, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, body)
	req.RemoteAddr = ip + ":54321"
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(req)
	}

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if rec.Body.Len() == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是合法 JSON（%d）: %s", rec.Code, rec.Body.String())
	}
	return out
}

// findCookie 按名字取出响应下发的 Cookie，取不到就直接失败。
func findCookie(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("响应里没有 %q Cookie，实际状态码 %d", name, rec.Code)
	return nil
}

func withCookie(cookies ...*http.Cookie) func(*http.Request) {
	return func(req *http.Request) {
		for _, c := range cookies {
			req.AddCookie(c)
		}
	}
}

func withHeader(key, value string) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set(key, value)
	}
}

// 注册 → 会话可用 → 登出后立即失效。
func TestHTTPSessionFlow(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.10"

	rec := call(t, engine, http.MethodPost, "/api/auth/register", ip, map[string]string{
		"account":    "httpx_user",
		"password":   "password123",
		"nickname":   "接口测试",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("注册应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}

	session := findCookie(t, rec, httpx.SessionCookie)
	if !session.HttpOnly {
		t.Fatal("会话 Cookie 必须是 HttpOnly")
	}

	rec = call(t, engine, http.MethodGet, "/api/auth/session", ip, nil, withCookie(session))
	if rec.Code != http.StatusOK {
		t.Fatalf("会话查询应成功，实际 %d", rec.Code)
	}
	if account := decode(t, rec)["user"].(map[string]any)["account"]; account != "httpx_user" {
		t.Fatalf("会话应返回当前账号，实际 %v", account)
	}

	rec = call(t, engine, http.MethodPost, "/api/auth/logout", ip, nil, withCookie(session))
	if rec.Code != http.StatusOK {
		t.Fatalf("登出应成功，实际 %d", rec.Code)
	}

	rec = call(t, engine, http.MethodGet, "/api/auth/session", ip, nil, withCookie(session))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("登出后会话应立即失效，实际 %d", rec.Code)
	}
}

// 未登录访问受保护接口应返回 401。
func TestHTTPMeRequiresSession(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodGet, "/api/me", "198.51.100.11", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应返回 401，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "unauthenticated" {
		t.Fatalf("错误码应为 unauthenticated，实际 %v", code)
	}
}

// 登录失败的错误码必须稳定，前端据此分支。
func TestHTTPLoginBadCredentials(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.12", map[string]string{
		"account":  "ghost",
		"password": "password123",
	}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("凭据错误应返回 401，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "bad_credentials" {
		t.Fatalf("错误码应为 bad_credentials，实际 %v", code)
	}
}

// 对应 4.1 与 4.2：第 3 次进冷却，第 20 次封禁，且闸门随后同步拦截。
func TestHTTPCooldownThenBan(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "203.0.113.200"

	badLogin := map[string]string{"account": "nobody", "password": "wrong-password"}

	var bannedAt int
	for i := 1; i <= e.Cfg.BanLimit; i++ {
		rec := call(t, engine, http.MethodPost, "/api/auth/login", ip, badLogin, nil)
		switch {
		case i < e.Cfg.CooldownLimit:
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("第 %d 次应为普通失败(401)，实际 %d", i, rec.Code)
			}
		case i == e.Cfg.CooldownLimit:
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("第 %d 次应触发冷却(429)，实际 %d", i, rec.Code)
			}
			if code := decode(t, rec)["code"]; code != "cooldown" {
				t.Fatalf("错误码应为 cooldown，实际 %v", code)
			}
			if rec.Header().Get("Retry-After") == "" {
				t.Fatal("冷却响应必须带 Retry-After")
			}
		case i < e.Cfg.BanLimit:
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("冷却期内第 %d 次应继续 429，实际 %d", i, rec.Code)
			}
		default:
			if rec.Code != http.StatusForbidden {
				t.Fatalf("第 %d 次应触发封禁(403)，实际 %d", i, rec.Code)
			}
			if code := decode(t, rec)["code"]; code != "banned" {
				t.Fatalf("错误码应为 banned，实际 %v", code)
			}
			bannedAt = i
		}
	}
	if bannedAt != e.Cfg.BanLimit {
		t.Fatalf("应在第 %d 次封禁，实际第 %d 次", e.Cfg.BanLimit, bannedAt)
	}

	// Nginx 闸门的调用方式：来源是回环，真实客户端 IP 走 X-Forwarded-For
	rec := call(t, engine, http.MethodGet, "/api/internal/ipcheck", "127.0.0.1", nil, withHeader("X-Forwarded-For", ip))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("闸门应拦截已封禁 IP，实际 %d", rec.Code)
	}
	if block := rec.Header().Get("X-Block"); block != "banned" {
		t.Fatalf("X-Block 应为 banned，实际 %q", block)
	}
}

// 闸门只接受回环调用，且放行干净 IP。
func TestHTTPIPCheckAccess(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodGet, "/api/internal/ipcheck", "198.51.100.60", nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("非回环调用应被拒绝，实际 %d", rec.Code)
	}
	if block := rec.Header().Get("X-Block"); block != "" {
		t.Fatalf("非回环拒绝不应带 X-Block，实际 %q", block)
	}

	rec = call(t, engine, http.MethodGet, "/api/internal/ipcheck", "127.0.0.1", nil, withHeader("X-Forwarded-For", "203.0.113.201"))
	if rec.Code != http.StatusOK {
		t.Fatalf("干净 IP 应放行，实际 %d", rec.Code)
	}
}

// 冷却中的 IP 应被闸门标记为 cooldown，由 Nginx 跳向冷却页而不是封禁页。
func TestHTTPIPCheckReportsCooldown(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "203.0.113.202"

	badLogin := map[string]string{"account": "nobody", "password": "wrong-password"}
	for i := 0; i < e.Cfg.CooldownLimit; i++ {
		call(t, engine, http.MethodPost, "/api/auth/login", ip, badLogin, nil)
	}

	rec := call(t, engine, http.MethodGet, "/api/internal/ipcheck", "127.0.0.1", nil, withHeader("X-Forwarded-For", ip))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("冷却中的 IP 应被闸门拦截，实际 %d", rec.Code)
	}
	if block := rec.Header().Get("X-Block"); block != "cooldown" {
		t.Fatalf("X-Block 应为 cooldown，实际 %q", block)
	}
}

// 后台接口：必须同时满足管理员角色与 CSRF 校验。
func TestHTTPAdminRequiresRoleAndCSRF(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := t.Context()

	// 普通用户
	rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.30", map[string]string{
		"account":    "normal_user",
		"password":   "password123",
		"nickname":   "普通用户",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("注册失败: %d", rec.Code)
	}
	userSession := findCookie(t, rec, httpx.SessionCookie)

	rec = call(t, engine, http.MethodGet, "/api/admin/invites", "198.51.100.30", nil, withCookie(userSession))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户访问后台应 403，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "forbidden" {
		t.Fatalf("错误码应为 forbidden，实际 %v", code)
	}

	// 管理员
	generated, created, err := e.Auth.EnsureAdmin(ctx, "admin", "")
	if err != nil || !created {
		t.Fatalf("初始化管理员失败: created=%v err=%v", created, err)
	}

	rec = call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.31", map[string]string{
		"account":  "admin",
		"password": generated,
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员登录失败: %d %s", rec.Code, rec.Body.String())
	}
	adminSession := findCookie(t, rec, httpx.SessionCookie)
	csrf := findCookie(t, rec, httpx.CSRFCookie)

	// 缺 CSRF 头 → 拒绝
	rec = call(t, engine, http.MethodPost, "/api/admin/invites", "198.51.100.31", nil, withCookie(adminSession))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺少 CSRF 头应 403，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "csrf" {
		t.Fatalf("错误码应为 csrf，实际 %v", code)
	}

	// 带上 CSRF 头 → 成功
	rec = call(t, engine, http.MethodPost, "/api/admin/invites", "198.51.100.31", nil, func(req *http.Request) {
		req.AddCookie(adminSession)
		req.AddCookie(csrf)
		req.Header.Set(httpx.CSRFHeader, csrf.Value)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("带 CSRF 头的请求应成功，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if code, ok := decode(t, rec)["code"].(string); !ok || len(code) != 16 {
		t.Fatalf("应返回 16 位邀请码，实际 %v", decode(t, rec)["code"])
	}

	// 只读接口不需要 CSRF
	rec = call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.31", nil, withCookie(adminSession))
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员读取用户列表应成功，实际 %d", rec.Code)
	}
}

// 邀请码为必填：缺失时注册必须被拒绝。
func TestHTTPRegisterNeedsInvite(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.40", map[string]string{
		"account":  "no_invite_user",
		"password": "password123",
		"nickname": "无邀请码",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺邀请码应 400，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "invalid_request" {
		t.Fatalf("错误码应为 invalid_request，实际 %v", code)
	}
}
