package tests

// 中转网关测试：Host 认领、鉴权矩阵、反向代理转发、凭据剥离、
// 跳转重写、路径规范化、上游故障与配置校验。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// gatewayUpstream 一个记录请求细节的上游服务。
type gatewayUpstream struct {
	srv *httptest.Server

	mu       sync.Mutex
	lastPath string
	lastRaw  string
	lastHost string
	lastCook string
}

func newGatewayUpstream(t *testing.T, handler http.HandlerFunc) *gatewayUpstream {
	t.Helper()
	up := &gatewayUpstream{}
	up.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up.mu.Lock()
		up.lastPath = r.URL.Path
		up.lastRaw = r.URL.RawQuery
		up.lastHost = r.Host
		up.lastCook = r.Header.Get("Cookie")
		up.mu.Unlock()
		if handler != nil {
			handler(w, r)
			return
		}
		w.Header().Set("Set-Cookie", "app=1; Path=/")
		_, _ = w.Write([]byte("UPSTREAM-OK"))
	}))
	t.Cleanup(up.srv.Close)
	return up
}

func (up *gatewayUpstream) last() (path, raw, host, cookies string) {
	up.mu.Lock()
	defer up.mu.Unlock()
	return up.lastPath, up.lastRaw, up.lastHost, up.lastCook
}

// mkProxyFeature 建一个 proxy 模式的功能并授权给指定用户（nil = 不授权）。
func mkProxyFeature(t *testing.T, e *env, host, target string, userID *uint) *model.Feature {
	t.Helper()
	item, err := e.Features.Create(context.Background(), service.FeatureInput{
		Title: "测试服务", Desc: "网关测试", Tag: "测试", Icon: "server",
		URL: target, Mode: service.ModeProxy, PublicHost: host,
	})
	if err != nil {
		t.Fatalf("创建中转功能失败: %v", err)
	}
	if userID != nil {
		g, err := e.Access.CreateGroup(context.Background(), "g"+item.Key)
		if err != nil {
			t.Fatalf("建组失败: %v", err)
		}
		feats := []string{item.Key}
		if err := e.Access.UpdateGroup(context.Background(), g.ID, nil, &feats); err != nil {
			t.Fatalf("授权失败: %v", err)
		}
		if err := e.Access.SetMembers(context.Background(), g.ID, []uint{*userID}); err != nil {
			t.Fatalf("设置成员失败: %v", err)
		}
	}
	return item
}

// gwRecorder 补齐 http.CloseNotifier：gin 的 CloseNotify 实现会向底层
// ResponseWriter 断言该接口，而 httptest.ResponseRecorder 没有（生产环境
// 底层是 net/http 的 response，无此问题）。ReverseProxy 走到的代码路径依赖它。
type gwRecorder struct{ *httptest.ResponseRecorder }

func (r gwRecorder) CloseNotify() <-chan bool { return make(chan bool, 1) }

// gwRequest 构造带指定 Host 与凭据的网关请求。
func gwRequest(t *testing.T, engine interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}, host, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = "198.51.100.200:12345"
	req.Host = host
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(gwRecorder{rec}, req)
	return rec
}

func sessionCookieFor(t *testing.T, e *env, userID uint) *http.Cookie {
	t.Helper()
	sid, err := e.Sessions.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	return &http.Cookie{Name: httpx.SessionCookie, Value: sid}
}

// Host 认领：未配置的地址、redirect 模式占用的地址都必须 404。
func TestGatewayHostRouting(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	up := newGatewayUpstream(t, nil)

	mkProxyFeature(t, e, "cvat.test.local", up.srv.URL, nil)

	// 未配置任何服务的地址
	rec := gwRequest(t, engine, "nobody.test.local", "/_gw/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未配置的 Host 应 404，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "没有配置任何中转服务") {
		t.Fatalf("应渲染拦截页，实际 %s", rec.Body.String())
	}

	// redirect 模式的功能即使登记了 host，也不参与网关认领
	if _, err := e.Features.Create(context.Background(), service.FeatureInput{
		Title: "直跳服务", Desc: "占位", Tag: "测试", Icon: "cloud",
		URL: "http://192.168.11.9:5000/", Mode: service.ModeRedirect, PublicHost: "jump.test.local",
	}); err != nil {
		t.Fatalf("创建直跳功能失败: %v", err)
	}
	rec = gwRequest(t, engine, "jump.test.local", "/_gw/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("redirect 功能的 host 不应被网关认领，实际 %d", rec.Code)
	}
}

// 鉴权矩阵：未登录 401；游客 403；未授权用户 403；授权用户 / 管理员 200。
func TestGatewayAuthMatrix(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := t.Context()
	up := newGatewayUpstream(t, nil)

	user, err := e.Auth.Register(ctx, registerInput("gw_user", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if _, _, err := e.Auth.EnsureAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatalf("管理员初始化失败: %v", err)
	}
	var admin model.User
	if err := e.DB.First(&admin, "account = ?", "admin").Error; err != nil {
		t.Fatalf("读取管理员失败: %v", err)
	}

	mkProxyFeature(t, e, "svc.test.local", up.srv.URL, &user.ID)
	guestSess, err := e.Sessions.CreateGuest(ctx)
	if err != nil {
		t.Fatalf("创建游客会话失败: %v", err)
	}
	guestCookie := &http.Cookie{Name: httpx.SessionCookie, Value: guestSess}

	const target = "/_gw/some/api"

	// 未登录 → 401 + 登录提示页
	rec := gwRequest(t, engine, "svc.test.local", "/_gw/some/api")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "请先登录") {
		t.Fatal("应渲染登录提示页")
	}

	// 游客 → 403
	rec = gwRequest(t, engine, "svc.test.local", "/_gw/some/api", guestCookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("游客应 403，实际 %d", rec.Code)
	}

	// 未授权用户（另一个没入组的账号）→ 403
	other, err := e.Auth.Register(ctx, registerInput("gw_other", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	rec = gwRequest(t, engine, "svc.test.local", "/_gw/some/api", sessionCookieFor(t, e, other.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("未授权用户应 403，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "没有权限") {
		t.Fatal("应渲染无权限页")
	}

	// 授权用户 → 200
	rec = gwRequest(t, engine, "svc.test.local", target, sessionCookieFor(t, e, user.ID))
	if rec.Code != http.StatusOK || rec.Body.String() != "UPSTREAM-OK" {
		t.Fatalf("授权用户应放行，实际 %d %s", rec.Code, rec.Body.String())
	}

	// 管理员 → 200（无视分组）
	rec = gwRequest(t, engine, "svc.test.local", target, sessionCookieFor(t, e, admin.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员应放行，实际 %d", rec.Code)
	}
}

// 反向代理转发：路径/查询透传、上游 Host 正确、响应与 Set-Cookie 透传，
// 博客自己的 Cookie 必须被剥掉，其余 Cookie 原样带给上游。
func TestGatewayProxyForwarding(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	up := newGatewayUpstream(t, nil)

	user, err := e.Auth.Register(t.Context(), registerInput("gw_fwd", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	mkProxyFeature(t, e, "app.test.local", up.srv.URL, &user.ID)

	rec := gwRequest(t, engine, "app.test.local", "/_gw/some/../deep/path?x=1&y=%E4%B8%AD",
		sessionCookieFor(t, e, user.ID),
		&http.Cookie{Name: httpx.CSRFCookie, Value: "csrf-value"},
		&http.Cookie{Name: "keepme", Value: "yes"},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("应转发成功，实际 %d：%s", rec.Code, rec.Body.String())
	}

	path, raw, host, cookies := up.last()
	if path != "/deep/path" {
		t.Fatalf("路径应被规范化后透传，上游收到 %q", path)
	}
	if !strings.Contains(raw, "x=1") || !strings.Contains(raw, "y=%E4%B8%AD") {
		t.Fatalf("查询串应原样透传，上游收到 %q", raw)
	}
	if host != strings.TrimPrefix(up.srv.URL, "http://") {
		t.Fatalf("上游看到的 Host 应是其自身，实际 %q", host)
	}
	if strings.Contains(cookies, httpx.SessionCookie) {
		t.Fatalf("博客会话 Cookie 必须被剥离，上游收到 %q", cookies)
	}
	if strings.Contains(cookies, httpx.CSRFCookie) {
		t.Fatalf("博客 CSRF Cookie 必须被剥离，上游收到 %q", cookies)
	}
	if !strings.Contains(cookies, "keepme=yes") {
		t.Fatalf("其余 Cookie 应原样透传，上游收到 %q", cookies)
	}
	if sc := rec.Result().Cookies(); len(sc) != 1 || sc[0].Name != "app" {
		t.Fatalf("上游 Set-Cookie 应透传，实际 %v", sc)
	}
}

// 上游返回绝对跳转（指向自己的内网地址）时必须改写为对外域名。
func TestGatewayLocationRewrite(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	up := newGatewayUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://"+r.Host+"/login?next=1")
		w.WriteHeader(http.StatusFound)
	})

	user, err := e.Auth.Register(t.Context(), registerInput("gw_loc", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	mkProxyFeature(t, e, "jump2.test.local", up.srv.URL, &user.ID)

	rec := gwRequest(t, engine, "jump2.test.local", "/_gw/", sessionCookieFor(t, e, user.ID))
	if rec.Code != http.StatusFound {
		t.Fatalf("应透传 302，实际 %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if loc != "http://jump2.test.local/login?next=1" {
		t.Fatalf("绝对跳转应改写为对外域名，实际 %q", loc)
	}
}

// 上游不可达 / 地址非法：给友好页而不是连接错误。
func TestGatewayUpstreamFailures(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	user, err := e.Auth.Register(t.Context(), registerInput("gw_down", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	// 直写库绕过校验：target 指向必然拒绝的端口
	if err := e.DB.Create(&model.Feature{
		Key: "dead", Title: "坏服务", Desc: "上游不可达", Tag: "测试", Icon: "server",
		URL: "http://127.0.0.1:1", Mode: service.ModeProxy, PublicHost: "dead.test.local",
	}).Error; err != nil {
		t.Fatalf("造功能失败: %v", err)
	}
	g, err := e.Access.CreateGroup(t.Context(), "dead-group")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	feats := []string{"dead"}
	if err := e.Access.UpdateGroup(t.Context(), g.ID, nil, &feats); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Access.SetMembers(t.Context(), g.ID, []uint{user.ID}); err != nil {
		t.Fatalf("设置成员失败: %v", err)
	}

	rec := gwRequest(t, engine, "dead.test.local", "/_gw/", sessionCookieFor(t, e, user.ID))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("上游不可达应 502，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "暂不可达") {
		t.Fatal("应渲染友好错误页")
	}
}

// 路径穿越风格请求：转发给上游前必须被规范化，不带 .. 段。
func TestGatewayPathTraversalCleaned(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	up := newGatewayUpstream(t, nil)

	user, err := e.Auth.Register(t.Context(), registerInput("gw_trav", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	mkProxyFeature(t, e, "trav.test.local", up.srv.URL, &user.ID)

	rec := gwRequest(t, engine, "trav.test.local", "/_gw/%2e%2e/%2e%2e/etc/passwd", sessionCookieFor(t, e, user.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("应转发（清理后），实际 %d", rec.Code)
	}
	path, _, _, _ := up.last()
	if strings.Contains(path, "..") {
		t.Fatalf("转发路径必须清理 .. 段，实际 %q", path)
	}
}

// 服务层校验：模式枚举、主机名格式、唯一性、redirect 不携带主机名。
func TestSecurityFeatureModeValidation(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()

	base := func() service.FeatureInput {
		return service.FeatureInput{Title: "校验", Desc: "校验测试", Tag: "测试", Icon: "server", URL: ""}
	}

	// 空 mode 归一为 redirect
	in := base()
	in.URL = "http://192.168.11.9:5000/"
	item, err := e.Features.Create(ctx, in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if item.Mode != service.ModeRedirect {
		t.Fatalf("空 mode 应归一为 redirect，实际 %q", item.Mode)
	}

	// 非法 mode
	bad := base()
	bad.Mode = "tunnel"
	if _, err := e.Features.Create(ctx, bad); err == nil {
		t.Fatal("非法 mode 必须被拒绝")
	}

	// proxy 缺主机名 / 主机名非法
	bad = base()
	bad.Mode = service.ModeProxy
	bad.URL = "http://127.0.0.1:3000/"
	if _, err := e.Features.Create(ctx, bad); err == nil {
		t.Fatal("proxy 缺对外主机名必须被拒绝")
	}
	for _, host := range []string{"http://x.com", "a/b", "a b", "-bad.com", "bad-.com", "a..b"} {
		bad = base()
		bad.Mode = service.ModeProxy
		bad.URL = "http://127.0.0.1:3000/"
		bad.PublicHost = host
		if _, err := e.Features.Create(ctx, bad); err == nil {
			t.Fatalf("非法主机名 %q 必须被拒绝", host)
		}
	}

	// 合法且大小写归一
	ok := base()
	ok.Mode = service.ModeProxy
	ok.URL = "http://127.0.0.1:3000/"
	ok.PublicHost = "CVAT.Test.Local"
	a, err := e.Features.Create(ctx, ok)
	if err != nil {
		t.Fatalf("合法 proxy 创建失败: %v", err)
	}
	if a.PublicHost != "cvat.test.local" {
		t.Fatalf("主机名应归一为小写，实际 %q", a.PublicHost)
	}

	// 唯一性：另一个功能抢同一主机名
	dup := base()
	dup.Mode = service.ModeProxy
	dup.URL = "http://127.0.0.1:3001/"
	dup.PublicHost = "cvat.test.local"
	if _, err := e.Features.Create(ctx, dup); err == nil {
		t.Fatal("重复的对外主机名必须被拒绝")
	}
	// 更新也不能抢占；改自己保留自己可以
	upd := dup
	upd.PublicHost = "other.test.local"
	if err := e.Features.Update(ctx, a.Key, upd); err != nil {
		t.Fatalf("更新主机名失败: %v", err)
	}
	keep := upd
	keep.PublicHost = "other.test.local"
	if err := e.Features.Update(ctx, a.Key, keep); err != nil {
		t.Fatalf("保留自身主机名不应报冲突: %v", err)
	}

	// redirect 更新时清空主机名
	red := base()
	red.URL = "http://192.168.11.9:5000/"
	if err := e.Features.Update(ctx, item.Key, red); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	got, err := e.Features.Get(ctx, item.Key)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.PublicHost != "" || got.Mode != service.ModeRedirect {
		t.Fatalf("redirect 模式不应携带主机名，实际 %+v", got)
	}
}

// /open 响应：proxy 只下发对外主机名（绝不含上游地址），redirect 保持原样。
func TestFeatureOpenModes(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)
	ctx := t.Context()

	user, err := e.Auth.Register(ctx, registerInput("gw_open", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	up := newGatewayUpstream(t, nil)
	mkProxyFeature(t, e, "open.test.local", up.srv.URL, &user.ID)

	red, err := e.Features.Create(ctx, service.FeatureInput{
		Title: "直跳", Desc: "直跳功能", Tag: "测试", Icon: "cloud",
		URL: "http://192.168.11.9:5000/", Mode: service.ModeRedirect,
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 授权直跳功能给同一用户
	g, err := e.Access.CreateGroup(ctx, "open-group")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	feats := []string{red.Key}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &feats); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Access.SetMembers(ctx, g.ID, []uint{user.ID}); err != nil {
		t.Fatalf("设置成员失败: %v", err)
	}

	sess := sessionCookieFor(t, e, user.ID)
	csrf, err := e.Sessions.Create(ctx, user.ID) // csrf cookie 走真登录才有，这里直接造一个配对值
	_ = csrf
	if err != nil {
		t.Fatalf("创建会话失败: %v", err)
	}
	csrfCookie := &http.Cookie{Name: httpx.CSRFCookie, Value: "csrf-x"}

	// proxy：返回 {mode, host}，且不得出现上游 URL
	rec := call(t, engine, http.MethodPost, "/api/features/"+mustFindProxyKey(t, e)+"/open", "198.51.100.201", nil, func(req *http.Request) {
		req.AddCookie(sess)
		req.AddCookie(csrfCookie)
		req.Header.Set(httpx.CSRFHeader, csrfCookie.Value)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("open 应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["mode"] != "proxy" || body["host"] != "open.test.local" {
		t.Fatalf("proxy 响应不符: %v", body)
	}
	if _, hasURL := body["url"]; hasURL {
		t.Fatal("proxy 模式绝不能下发上游地址")
	}
	if strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Fatal("响应泄露了上游地址")
	}

	// redirect：保持 {mode: redirect, url}
	rec = call(t, engine, http.MethodPost, "/api/features/"+red.Key+"/open", "198.51.100.201", nil, func(req *http.Request) {
		req.AddCookie(sess)
		req.AddCookie(csrfCookie)
		req.Header.Set(httpx.CSRFHeader, csrfCookie.Value)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("open 应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	body = decode(t, rec)
	if body["mode"] != "redirect" || body["url"] != "http://192.168.11.9:5000/" {
		t.Fatalf("redirect 响应不符: %v", body)
	}
}

func mustFindProxyKey(t *testing.T, e *env) string {
	t.Helper()
	var item model.Feature
	if err := e.DB.First(&item, "public_host = ?", "open.test.local").Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			t.Fatalf("查询失败: %v", err)
		}
		t.Fatalf("找不到 proxy 功能: %v", err)
	}
	return item.Key
}
