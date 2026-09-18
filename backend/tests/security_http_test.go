package tests

// 安全测试 · HTTP 层攻面。
//
// 覆盖：
//   - 所有写端点的 CSRF 强制（逐个枚举，漏一个就是缺口）
//   - 身份矩阵：匿名 401 / 游客 403 / 普通用户 403 / 管理员放行
//   - 方法混淆、尾斜杠与大小写绕过
//   - X-Forwarded-For / X-Real-IP 伪造绕限流与封禁
//   - 游客入口刷会话的资源耗尽
//   - 错误响应不泄露堆栈/内部路径；封禁页转义用户输入
//   - 角色变更即时生效（无缓存）、组员更新的原子性
//   - 路径参数的 fuzz（SQL 注入形态 / 溢出 / 超长 / 编码穿越）

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
)

// adminCreds 确保管理员存在并完成登录，返回会话与 CSRF 凭据。
func adminCreds(t *testing.T, e *env, engine *gin.Engine, ip string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	if _, _, err := e.Auth.EnsureAdmin(t.Context(), "admin", "password123"); err != nil {
		t.Fatalf("初始化管理员失败: %v", err)
	}
	return loginAs(t, engine, "admin", "password123", ip)
}

// adminWriteEndpoints 管理端全部写端点（与 router.go 一一对应，新增端点必须同步补进来）。
func adminWriteEndpoints() []struct {
	method  string
	path    string
	payload any
} {
	return []struct {
		method  string
		path    string
		payload any
	}{
		{http.MethodPost, "/api/admin/invites", nil},
		{http.MethodDelete, "/api/admin/invites/1", nil},
		{http.MethodPost, "/api/admin/bans", map[string]any{"ip": "198.51.100.99"}},
		{http.MethodPost, "/api/admin/bans/198.51.100.99/unban", nil},
		{http.MethodDelete, "/api/admin/bans/198.51.100.99", nil},
		{http.MethodPatch, "/api/admin/users/1/role", map[string]any{"role": "admin"}},
		{http.MethodPost, "/api/admin/groups", map[string]any{"name": "安全测试组"}},
		{http.MethodPatch, "/api/admin/groups/1", map[string]any{"name": "改名组"}},
		{http.MethodDelete, "/api/admin/groups/1", nil},
		{http.MethodPut, "/api/admin/groups/1/members", map[string]any{"userIds": []uint{}}},
		{http.MethodPost, "/api/admin/features", map[string]any{"title": "配置", "desc": "测试功能", "tag": "测试", "icon": "book"}},
		{http.MethodPatch, "/api/admin/features/notexist", map[string]any{"title": "配置", "desc": "测试功能", "tag": "测试", "icon": "book"}},
		{http.MethodDelete, "/api/admin/features/notexist", nil},
		{http.MethodPut, "/api/admin/features/order", map[string]any{"keys": []string{}}},
		{http.MethodPost, "/api/features/terminal/open", nil},
	}
}

// 管理端全部端点（读 + 写 + 功能开放），供身份矩阵遍历。
func adminAllEndpoints() []struct {
	method  string
	path    string
	payload any
} {
	out := []struct {
		method  string
		path    string
		payload any
	}{
		{http.MethodGet, "/api/admin/invites", nil},
		{http.MethodGet, "/api/admin/bans", nil},
		{http.MethodGet, "/api/admin/users", nil},
		{http.MethodGet, "/api/admin/groups", nil},
		{http.MethodGet, "/api/admin/features", nil},
	}
	writes := adminWriteEndpoints()
	// 矩阵里不能对真实用户做 role 变更（会把普通用户提权、破坏后续身份断言），
	// 换成不存在的 id：404 同样证明 RequireAuth + RequireAdmin + CSRF 三重放行。
	for i := range writes {
		if writes[i].path == "/api/admin/users/1/role" {
			writes[i].path = "/api/admin/users/99999/role"
		}
	}
	return append(out, writes...)
}

// 每个写端点都必须强制 CSRF：缺头拒绝、错头拒绝、配对放行。
// 少挂一个 CSRF 中间件就是一条被忽视的伪造面。
func TestSecurityWriteEndpointsRequireCSRSToken(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.90"
	session, csrf := adminCreds(t, e, engine, ip)

	for _, ep := range adminWriteEndpoints() {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			// 1) 只有会话、无 CSRF 头 → 403
			rec := call(t, engine, ep.method, ep.path, ip, ep.payload, withCookie(session))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("缺少 CSRF 头应 403，实际 %d：%s", rec.Code, rec.Body.String())
			}
			if code := decode(t, rec)["code"]; code != "csrf" {
				t.Fatalf("错误码应为 csrf，实际 %v", code)
			}

			// 2) 头与 Cookie 不一致（伪造头）→ 403
			rec = call(t, engine, ep.method, ep.path, ip, ep.payload, func(req *http.Request) {
				req.AddCookie(session)
				req.AddCookie(csrf)
				req.Header.Set(httpx.CSRFHeader, "forged-token")
			})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("CSRF 头不一致应 403，实际 %d", rec.Code)
			}

			// 3) 正确配对 → 必须能通过 CSRF 检查（到达业务层，不再 401/403）
			rec = call(t, engine, ep.method, ep.path, ip, ep.payload, withCredentials(session, csrf))
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Fatalf("CSRF 配对正确却被拦（%d）：%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 身份矩阵：任何管理端点对匿名/游客/普通用户都必须关闭，对管理员开放。
func TestSecurityAdminEndpointsAuthMatrix(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	// 普通用户（注册即登录，IP A）
	reg := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.91", map[string]string{
		"account":    "matrix_user",
		"password":   "password123",
		"nickname":   "矩阵用户",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if reg.Code != http.StatusOK {
		t.Fatalf("注册失败: %d", reg.Code)
	}
	userSess := findCookie(t, reg, httpx.SessionCookie)
	userCSRF := findCookie(t, reg, httpx.CSRFCookie)

	// 游客（IP C）
	guestSess, guestCSRF := guestCreds(t, engine, "198.51.100.93")

	// 管理员（IP B）
	adminSess, adminCSRF := adminCreds(t, e, engine, "198.51.100.92")

	const ip = "198.51.100.94"
	for _, ep := range adminAllEndpoints() {
		name := ep.method + " " + ep.path
		t.Run(name, func(t *testing.T) {
			// 匿名 → 401
			rec := call(t, engine, ep.method, ep.path, ip, ep.payload, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("匿名应 401，实际 %d", rec.Code)
			}
			// 游客 → 403
			rec = call(t, engine, ep.method, ep.path, ip, ep.payload, withCredentials(guestSess, guestCSRF))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("游客应 403，实际 %d：%s", rec.Code, rec.Body.String())
			}
			// 普通用户 → 403
			rec = call(t, engine, ep.method, ep.path, ip, ep.payload, withCredentials(userSess, userCSRF))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("普通用户应 403，实际 %d：%s", rec.Code, rec.Body.String())
			}
			// 管理员 → 放行（非 401/403；业务失败 400/404 允许）
			rec = call(t, engine, ep.method, ep.path, ip, ep.payload, withCredentials(adminSess, adminCSRF))
			if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
				t.Fatalf("管理员不应被拦，实际 %d：%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 错误方法不得命中同路径的其它逻辑；未知方法一律 404/405，不能 200 也不能 5xx。
func TestSecurityWrongMethodRejected(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	probes := []struct{ method, path string }{
		{http.MethodGet, "/api/auth/login"},
		{http.MethodPut, "/api/auth/login"},
		{http.MethodDelete, "/api/auth/login"},
		{http.MethodGet, "/api/auth/register"},
		{http.MethodGet, "/api/auth/logout"},
		{http.MethodPost, "/api/features"},
		{http.MethodPut, "/api/features"},
		{http.MethodDelete, "/api/features"},
		{http.MethodPost, "/api/me"},
		{http.MethodDelete, "/api/me"},
		{http.MethodPost, "/api/admin/features/order"},
	}
	for _, p := range probes {
		rec := call(t, engine, p.method, p.path, "198.51.100.95", nil, nil)
		if rec.Code == http.StatusOK || rec.Code >= 500 {
			t.Fatalf("%s %s 应 404/405，实际 %d", p.method, p.path, rec.Code)
		}
	}
}

// 尾斜杠与大小写不得绕过路由（重定向或 404 都可以，但绝不能直接执行业务逻辑）。
func TestSecurityTrailingSlashAndCaseSensitive(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	session, csrf := adminCreds(t, e, engine, "198.51.100.96")

	trailing := []struct {
		method, path string
		creds        func(*http.Request)
	}{
		{http.MethodPost, "/api/auth/login/", nil},
		{http.MethodGet, "/api/features/", nil},
		{http.MethodPost, "/api/admin/invites/", withCredentials(session, csrf)},
		{http.MethodGet, "/api/admin/users/", withCredentials(session, csrf)},
	}
	for _, p := range trailing {
		rec := call(t, engine, p.method, p.path, "198.51.100.96", nil, p.creds)
		switch rec.Code {
		case http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusNotFound:
			// 重定向（GET 301 / POST 307）或未匹配，均未执行业务逻辑
		default:
			t.Fatalf("%s %s 应为重定向(301/307/308)或 404，实际 %d", p.method, p.path, rec.Code)
		}
	}

	// 路由大小写敏感：全大写路径必须 404
	for _, p := range []string{"/api/Auth/login", "/api/FEATURES", "/API/features", "/api/ADMIN/users"} {
		rec := call(t, engine, http.MethodGet, p, "198.51.100.96", nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s 应 404（大小写敏感），实际 %d", p, rec.Code)
		}
	}
}

// 伪造 X-Forwarded-For / X-Real-IP 不能逃脱限流与封禁：
// 计数必须落在真实连接 IP 上，换多少个伪造 IP 都一样。
func TestSecurityXFFCannotEvadeBan(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "203.0.113.240"

	bad := map[string]string{"account": "nobody", "password": "wrong-password"}
	banned := false
	for i := 0; i < e.Cfg.BanLimit; i++ {
		rec := call(t, engine, http.MethodPost, "/api/auth/login", ip, bad, func(req *http.Request) {
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.%d.%d", i/250, i%250))
			req.Header.Set("X-Real-IP", fmt.Sprintf("10.1.%d.%d", i/250, i%250))
		})
		if rec.Code == http.StatusOK {
			t.Fatalf("第 %d 次失败登录不应成功", i+1)
		}
		if rec.Code == http.StatusForbidden {
			if code := decode(t, rec)["code"]; code == "banned" {
				banned = true
			}
		}
	}
	if !banned {
		t.Fatalf("换了 %d 个伪造 IP 仍应在真实 IP 上触发封禁", e.Cfg.BanLimit)
	}

	// 封禁后继续更换伪造头 → 依旧被拦
	rec := call(t, engine, http.MethodPost, "/api/auth/login", ip, bad, func(req *http.Request) {
		req.Header.Set("X-Forwarded-For", "10.9.9.9")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("封禁后伪造头不应放行，实际 %d", rec.Code)
	}
	if code := decode(t, rec)["code"]; code != "banned" {
		t.Fatalf("错误码应为 banned，实际 %v", code)
	}

	// 闸门视角：该 IP 同样被标记为 banned
	rec = call(t, engine, http.MethodGet, "/api/internal/ipcheck", "127.0.0.1", nil,
		withHeader("X-Forwarded-For", ip))
	if rec.Code != http.StatusForbidden || rec.Header().Get("X-Block") != "banned" {
		t.Fatalf("闸门应标记该 IP 为 banned，实际 %d / %q", rec.Code, rec.Header().Get("X-Block"))
	}
}

// 游客入口会创建服务端会话，必须同样吃限流与封禁（防脚本无限刷会话）。
func TestSecurityGuestFloodRequiresRateLimit(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "203.0.113.241"

	codes := map[int]int{}
	for i := 0; i < e.Cfg.BanLimit; i++ {
		rec := call(t, engine, http.MethodPost, "/api/auth/guest", ip, nil, nil)
		codes[rec.Code]++
	}
	if codes[http.StatusOK] != 2 {
		t.Fatalf("游客入口前两次应放行（实际 200 次数 %d）", codes[http.StatusOK])
	}
	if codes[http.StatusTooManyRequests] == 0 {
		t.Fatal("游客入口必须同样触发冷却（429）")
	}
	if codes[http.StatusForbidden] == 0 {
		t.Fatal("刷满窗口后游客入口必须封禁（403）")
	}
}

// 错误响应体内不得出现堆栈、内部包名、SQL 或文件路径；健康检查只回 "ok"。
func TestSecurityErrorBodiesClean(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	var bodies []string
	collect := func(rec *httptest.ResponseRecorder) { bodies = append(bodies, rec.Body.String()) }

	collect(call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.97",
		map[string]string{"account": "", "password": ""}, nil))
	collect(call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.97",
		map[string]string{"account": "ghost", "password": "wrong-password-1"}, nil))
	collect(call(t, engine, http.MethodGet, "/api/me", "198.51.100.97", nil, nil))
	guestSess, _ := guestCreds(t, engine, "198.51.100.98")
	collect(call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.98", nil, withCookie(guestSess)))
	collect(call(t, engine, http.MethodGet, "/api/no-such-route", "198.51.100.97", nil, nil))

	markers := []string{
		"goroutine", "runtime.", "panic", ".go:",
		"C:\\", "/app/", "personal_blog",
		"SELECT ", "INSERT ", "UPDATE ", "sqlite", "gorm",
	}
	for _, body := range bodies {
		for _, m := range markers {
			if strings.Contains(body, m) {
				t.Fatalf("错误响应泄露内部信息 %q：%.200s", m, body)
			}
		}
	}

	rec := call(t, engine, http.MethodGet, "/healthz", "198.51.100.97", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz 应只回 ok，实际 %d %q", rec.Code, rec.Body.String())
	}
}

// 封禁页会把管理员填写的原因渲染进 HTML：必须严格转义，防存储型 XSS。
func TestSecurityBannedPageEscapesReason(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "203.0.113.242"

	payload := `<script>alert(1)</script>`
	if _, err := e.Guard.MarkBanned(t.Context(), ip, payload, true); err != nil {
		t.Fatalf("封禁失败: %v", err)
	}

	rec := call(t, engine, http.MethodGet, "/banned", ip, nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("封禁页应 403，实际 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, payload) {
		t.Fatal("封禁页把原因原样插入 HTML（存储型 XSS）")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("原因应以转义形式展示")
	}
}

// 角色变更必须即时生效：不缓存角色，提权/降权在同一会话的下一个请求就生效。
func TestSecurityRoleChangeTakesEffectImmediately(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	reg := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.100", map[string]string{
		"account":    "role_target",
		"password":   "password123",
		"nickname":   "目标用户",
		"inviteCode": e.seedInvite(t),
	}, nil)
	if reg.Code != http.StatusOK {
		t.Fatalf("注册失败: %d", reg.Code)
	}
	targetSess := findCookie(t, reg, httpx.SessionCookie)
	targetCSRF := findCookie(t, reg, httpx.CSRFCookie)

	var target model.User
	if err := e.DB.First(&target, "account = ?", "role_target").Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}

	adminSess, adminCSRF := adminCreds(t, e, engine, "198.51.100.101")

	// 提权前：403
	rec := call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.100", nil, withCredentials(targetSess, targetCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户应 403，实际 %d", rec.Code)
	}

	// 提升为管理员
	rec = call(t, engine, http.MethodPatch, fmt.Sprintf("/api/admin/users/%d/role", target.ID), "198.51.100.101",
		map[string]any{"role": "admin"}, withCredentials(adminSess, adminCSRF))
	if rec.Code != http.StatusOK {
		t.Fatalf("提升角色失败: %d %s", rec.Code, rec.Body.String())
	}
	// 同一会话立即获得管理能力
	rec = call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.100", nil, withCredentials(targetSess, targetCSRF))
	if rec.Code != http.StatusOK {
		t.Fatalf("提权应立即生效，实际 %d", rec.Code)
	}

	// 降级
	rec = call(t, engine, http.MethodPatch, fmt.Sprintf("/api/admin/users/%d/role", target.ID), "198.51.100.101",
		map[string]any{"role": "user"}, withCredentials(adminSess, adminCSRF))
	if rec.Code != http.StatusOK {
		t.Fatalf("降级失败: %d", rec.Code)
	}
	rec = call(t, engine, http.MethodGet, "/api/admin/users", "198.51.100.100", nil, withCredentials(targetSess, targetCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("降权应立即生效，实际 %d", rec.Code)
	}
}

// 角色只能取 admin / user；未知值、空值、畸形 id 一律 4xx。
func TestSecuritySetRoleRejectsBadValues(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	if rec := call(t, engine, http.MethodPost, "/api/auth/register", "198.51.100.102", map[string]string{
		"account":    "role_probe",
		"password":   "password123",
		"nickname":   "探针",
		"inviteCode": e.seedInvite(t),
	}, nil); rec.Code != http.StatusOK {
		t.Fatalf("注册失败: %d %s", rec.Code, rec.Body.String())
	}
	var target model.User
	if err := e.DB.First(&target, "account = ?", "role_probe").Error; err != nil {
		t.Fatalf("读取用户失败: %v", err)
	}

	adminSess, adminCSRF := adminCreds(t, e, engine, "198.51.100.103")
	creds := withCredentials(adminSess, adminCSRF)

	for _, role := range []string{"root", "superuser", "GUEST", "Admin ", ""} {
		rec := call(t, engine, http.MethodPatch, fmt.Sprintf("/api/admin/users/%d/role", target.ID), "198.51.100.103",
			map[string]string{"role": role}, creds)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("角色 %q 应 400，实际 %d", role, rec.Code)
		}
	}

	for _, id := range []string{"abc", "0", "1.5", "99999999999999999999"} {
		rec := call(t, engine, http.MethodPatch, "/api/admin/users/"+id+"/role", "198.51.100.103",
			map[string]string{"role": "user"}, creds)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("id %q 应 400，实际 %d", id, rec.Code)
		}
	}

	rec := call(t, engine, http.MethodPatch, "/api/admin/users/99999/role", "198.51.100.103",
		map[string]string{"role": "user"}, creds)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("不存在的用户应 404，实际 %d", rec.Code)
	}
}

// 组员更新是覆盖式事务：非法输入必须整体回滚，绝不能留下"清空了一半"的状态。
func TestSecuritySetMembersAtomicity(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := t.Context()

	adminSess, adminCSRF := adminCreds(t, e, engine, "198.51.100.104")
	creds := withCredentials(adminSess, adminCSRF)

	rec := call(t, engine, http.MethodPost, "/api/admin/groups", "198.51.100.104",
		map[string]any{"name": "原子性组"}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("建组失败: %d", rec.Code)
	}
	gid := uint(decode(t, rec)["group"].(map[string]any)["id"].(float64))

	u1, err := e.Auth.Register(ctx, registerInput("member_a", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	u2, err := e.Auth.Register(ctx, registerInput("member_b", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	members := func() []uint {
		rec := call(t, engine, http.MethodGet, "/api/admin/groups", "198.51.100.104", nil, creds)
		for _, g := range decode(t, rec)["groups"].([]any) {
			gm := g.(map[string]any)
			if uint(gm["id"].(float64)) != gid {
				continue
			}
			var out []uint
			for _, m := range gm["members"].([]any) {
				out = append(out, uint(m.(float64)))
			}
			return out
		}
		t.Fatalf("组 %d 不存在", gid)
		return nil
	}

	// 初始成员 [u1]
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", gid), "198.51.100.104",
		map[string]any{"userIds": []uint{u1.ID}}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("设置成员失败: %d %s", rec.Code, rec.Body.String())
	}

	// 重复 uid → 400，且老成员保留
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", gid), "198.51.100.104",
		map[string]any{"userIds": []uint{u1.ID, u1.ID}}, creds)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("重复 uid 应 400，实际 %d", rec.Code)
	}
	if got := members(); len(got) != 1 || got[0] != u1.ID {
		t.Fatalf("非法输入不得破坏已有成员，实际 %v", got)
	}

	// 含不存在的用户 → 400，且老成员保留（事务回滚）
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", gid), "198.51.100.104",
		map[string]any{"userIds": []uint{u1.ID, 99999}}, creds)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("不存在用户应 400，实际 %d", rec.Code)
	}
	if got := members(); len(got) != 1 || got[0] != u1.ID {
		t.Fatalf("非法输入不得破坏已有成员，实际 %v", got)
	}

	// 类型错 → 400
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", gid), "198.51.100.104",
		map[string]any{"userIds": "oops"}, creds)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("类型错误应 400，实际 %d", rec.Code)
	}

	// 正常覆盖为 [u2]
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", gid), "198.51.100.104",
		map[string]any{"userIds": []uint{u2.ID}}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("覆盖成员失败: %d", rec.Code)
	}
	if got := members(); len(got) != 1 || got[0] != u2.ID {
		t.Fatalf("成员应为 [%d]，实际 %v", u2.ID, got)
	}
}

// 邀请码 / 封禁的路径参数 fuzz：注入形态、溢出、超长都只能得到 4xx。
func TestSecurityInviteAndBanParamFuzz(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.105"
	creds := withCredentials(adminCreds(t, e, engine, ip))

	for _, f := range []string{"abc", "-1", "0", "1.5", "99999999999999999999", "1%20"} {
		rec := call(t, engine, http.MethodDelete, "/api/admin/invites/"+f, ip, nil, creds)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("邀请码 id %q 应 400/404，实际 %d", f, rec.Code)
		}
	}

	for _, f := range []string{"%27--", "%3Cscript%3E", strings.Repeat("9", 300), "%20", "..%2F.."} {
		rec := call(t, engine, http.MethodPost, "/api/admin/bans/"+f+"/unban", ip, nil, creds)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Fatalf("解封 IP %q 应 404/400，实际 %d", f, rec.Code)
		}
		rec = call(t, engine, http.MethodDelete, "/api/admin/bans/"+f, ip, nil, creds)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Fatalf("清除封禁 %q 应 404/400，实际 %d", f, rec.Code)
		}
	}

	// 封禁表必须完好
	var n int64
	if err := e.DB.Model(&model.BanRecord{}).Count(&n).Error; err != nil {
		t.Fatalf("ban_records 表不可查询: %v", err)
	}
}

// 功能 key 的路径参数 fuzz：路径穿越、SQL 注入、超长、中文都只能得到 4xx，
// 且 features 表数据完好（PATCH/DELETE 不能误伤或删表）。
func TestSecurityFeatureKeyFuzz(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	const ip = "198.51.100.106"
	creds := withCredentials(adminCreds(t, e, engine, ip))

	// 先建一个真实功能，fuzz 结束后验证它完好
	rec := call(t, engine, http.MethodPost, "/api/admin/features", ip,
		map[string]any{"title": "哨兵", "desc": "fuzz 用功能", "tag": "测试", "icon": "book"}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("建功能失败: %d", rec.Code)
	}
	sentinel := decode(t, rec)["feature"].(map[string]any)["key"].(string)

	payload := map[string]any{"title": "改", "desc": "被篡改", "tag": "改", "icon": "book"}
	fuzzPaths := []string{
		"/api/admin/features/../secret",
		"/api/admin/features/..%2F..%2Fsecret",
		"/api/admin/features/TERMINAL",
		"/api/admin/features/x%27--",
		"/api/admin/features/x%20y",
		"/api/admin/features/-lead",
		"/api/admin/features/" + strings.Repeat("a", 65),
		"/api/admin/features/x--",
		"/api/admin/features/%E4%B8%AD%E6%96%87",
	}
	for _, p := range fuzzPaths {
		rec := call(t, engine, http.MethodPatch, p, ip, payload, creds)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("PATCH %s 应 400/404，实际 %d", p, rec.Code)
		}
		rec = call(t, engine, http.MethodDelete, p, ip, nil, creds)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Fatalf("DELETE %s 应 400/404，实际 %d", p, rec.Code)
		}
	}

	// open 路由同样收口
	for _, p := range []string{"/api/features/x--/open", "/api/features/TERMINAL/open", "/api/features/%E4%B8%AD%E6%96%87/open"} {
		rec := call(t, engine, http.MethodPost, p, ip, nil, creds)
		if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
			t.Fatalf("open %s 应 404/400，实际 %d", p, rec.Code)
		}
	}

	// 哨兵功能必须原样存在
	var feat model.Feature
	if err := e.DB.First(&feat, "key = ?", sentinel).Error; err != nil {
		t.Fatalf("哨兵功能被误伤: %v", err)
	}
	if feat.Title != "哨兵" || feat.Desc != "fuzz 用功能" {
		t.Fatalf("哨兵功能被篡改: %+v", feat)
	}
}
