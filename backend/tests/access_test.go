package tests

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"personal_blog/internal/httpx"
	"personal_blog/internal/model"
	"personal_blog/internal/service"
)

// registerInput 构造注册入参（测试用固定密码）。
func registerInput(account, inviteCode string) service.RegisterInput {
	return service.RegisterInput{
		Account:    account,
		Password:   "password123",
		Nickname:   account,
		InviteCode: inviteCode,
		IP:         "198.51.100.50",
	}
}

// withCredentials 把 Cookie 带进请求；csrf cookie 同时写进 X-CSRF-Token 头
// （模拟浏览器的 double-submit 行为）。
func withCredentials(cookies ...*http.Cookie) func(*http.Request) {
	return func(req *http.Request) {
		for _, c := range cookies {
			req.AddCookie(c)
			if c.Name == httpx.CSRFCookie {
				req.Header.Set(httpx.CSRFHeader, c.Value)
			}
		}
	}
}

func TestAccessGroupLifecycle(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	u1, err := e.Auth.Register(ctx, registerInput("ops_user", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}

	// 建组：名称去除首尾空格；初始无授权无成员
	g, err := e.Access.CreateGroup(ctx, " 运维组 ")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	if g.Name != "运维组" {
		t.Fatalf("组名应被 trim，得到 %q", g.Name)
	}
	if _, err := e.Access.CreateGroup(ctx, "运维组"); err == nil {
		t.Fatal("重名建组应被拒绝")
	}

	// 授权两个功能 + 拉一个成员
	feats := []string{"nas", "grafana"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &feats); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Access.SetMembers(ctx, g.ID, []uint{u1.ID}); err != nil {
		t.Fatalf("设置成员失败: %v", err)
	}

	// CanOpen：授权命中放行、未授权拒绝
	if ok, _ := e.Access.CanOpen(ctx, u1.ID, "nas"); !ok {
		t.Fatal("组已授权 nas，应放行")
	}
	if ok, _ := e.Access.CanOpen(ctx, u1.ID, "wrench"); ok {
		t.Fatal("未授权功能不应放行")
	}

	// 未入组用户一律拒绝
	u2, err := e.Auth.Register(ctx, registerInput("lone_user", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if ok, _ := e.Access.CanOpen(ctx, u2.ID, "nas"); ok {
		t.Fatal("未入组用户不应放行")
	}

	// 覆盖式更新：只留 grafana
	only := []string{"grafana"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &only); err != nil {
		t.Fatalf("更新授权失败: %v", err)
	}
	if ok, _ := e.Access.CanOpen(ctx, u1.ID, "nas"); ok {
		t.Fatal("授权已被覆盖，nas 应拒绝")
	}
	if ok, _ := e.Access.CanOpen(ctx, u1.ID, "grafana"); !ok {
		t.Fatal("grafana 应放行")
	}

	// 删组后授权与成员全部清空
	if err := e.Access.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatalf("删组失败: %v", err)
	}
	if ok, _ := e.Access.CanOpen(ctx, u1.ID, "grafana"); ok {
		t.Fatal("删组后不应再放行")
	}
	if err := e.Access.DeleteGroup(ctx, g.ID); err == nil {
		t.Fatal("重复删除应报「不存在」")
	}
}

func TestFeatureValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	// javascript: 伪协议被拒
	if _, err := e.Features.Create(ctx, service.FeatureInput{Title: "终端", Desc: "描述", Tag: "工具", Icon: "terminal", URL: "javascript:alert(1)"}); err == nil {
		t.Fatal("javascript: 伪协议必须被拒绝")
	}
	// 图标白名单
	if _, err := e.Features.Create(ctx, service.FeatureInput{Title: "终端", Desc: "描述", Tag: "工具", Icon: "hacker", URL: ""}); err == nil {
		t.Fatal("非法图标必须被拒绝")
	}
	// 超长标题
	if _, err := e.Features.Create(ctx, service.FeatureInput{Title: "一二三四五六七八九十甲乙丙", Desc: "描述", Tag: "工具", Icon: "book", URL: ""}); err == nil {
		t.Fatal("超长标题必须被拒绝")
	}

	// 合法创建（URL 可空 = 未配置）
	item, err := e.Features.Create(ctx, service.FeatureInput{Title: "终端", Desc: "描述", Tag: "工具", Icon: "terminal", URL: "http://192.168.11.9:5000/"})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if got, err := e.Features.GetURL(ctx, item.Key); err != nil || got != "http://192.168.11.9:5000/" {
		t.Fatalf("地址回读不符: %q, %v", got, err)
	}
	// 清空地址 = 未配置
	if err := e.Features.Update(ctx, item.Key, service.FeatureInput{Title: "终端", Desc: "描述", Tag: "工具", Icon: "terminal", URL: ""}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if _, err := e.Features.GetURL(ctx, item.Key); !errors.Is(err, service.ErrFeatureNotConfigured) {
		t.Fatalf("清空后应报未配置: %v", err)
	}

	// 排序：数组下标即新顺序
	a, err := e.Features.Create(ctx, service.FeatureInput{Title: "甲", Desc: "一", Tag: "x", Icon: "book", URL: ""})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	b, err := e.Features.Create(ctx, service.FeatureInput{Title: "乙", Desc: "一", Tag: "x", Icon: "book", URL: ""})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := e.Features.Reorder(ctx, []string{b.Key, a.Key, item.Key}); err != nil {
		t.Fatalf("排序失败: %v", err)
	}
	list, err := e.Features.List(ctx)
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if len(list) != 3 || list[0].Key != b.Key {
		t.Fatalf("排序后首个应为 %s，得到 %+v", b.Key, list)
	}

	// 删除功能要连带清理组授权
	g, err := e.Access.CreateGroup(ctx, "g1")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	feats := []string{item.Key}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &feats); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Features.Delete(ctx, item.Key); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	var cnt int64
	if err := e.DB.Model(&model.GroupFeature{}).Where("feature_key = ?", item.Key).Count(&cnt).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if cnt != 0 {
		t.Fatal("删除功能后组授权应被清理")
	}
}

// TestFeatureOpenAuthMatrix 验证功能点击鉴权的完整判定矩阵。
func TestFeatureOpenAuthMatrix(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	engine := e.newRouter(t)

	if err := e.DB.Create(&model.Feature{
		Key: "nas", Title: "NAS", Desc: "网络存储设备", Tag: "存储", Icon: "cloud",
		URL: "http://192.168.11.9:5000/",
	}).Error; err != nil {
		t.Fatalf("造功能失败: %v", err)
	}

	// ---- 游客：一律 403 ----
	guestRec := call(t, engine, http.MethodPost, "/api/auth/guest", "198.51.100.40", nil, nil)
	guestSess := findCookie(t, guestRec, httpx.SessionCookie)
	guestCSRF := findCookie(t, guestRec, httpx.CSRFCookie)
	rec := call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.40", nil, withCredentials(guestSess, guestCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("游客应吃 403，得到 %d", rec.Code)
	}

	// ---- 普通用户（未入组）：403 ----
	user, err := e.Auth.Register(ctx, registerInput("plain_user", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	loginRec := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.41",
		map[string]string{"account": "plain_user", "password": "password123"}, nil)
	userSess := findCookie(t, loginRec, httpx.SessionCookie)
	userCSRF := findCookie(t, loginRec, httpx.CSRFCookie)
	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.41", nil, withCredentials(userSess, userCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("未授权用户应吃 403，得到 %d", rec.Code)
	}

	// ---- 入组且授权：200 + 地址下发 ----
	g, err := e.Access.CreateGroup(ctx, "ops")
	if err != nil {
		t.Fatalf("建组失败: %v", err)
	}
	feats := []string{"nas"}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &feats); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	if err := e.Access.SetMembers(ctx, g.ID, []uint{user.ID}); err != nil {
		t.Fatalf("设置成员失败: %v", err)
	}

	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.41", nil, withCredentials(userSess, userCSRF))
	if rec.Code != http.StatusOK {
		t.Fatalf("授权用户应 200，得到 %d（%s）", rec.Code, rec.Body.String())
	}
	if url, _ := decode(t, rec)["url"].(string); url != "http://192.168.11.9:5000/" {
		t.Fatalf("下发地址不符: %q", url)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("响应必须带 no-store，得到 %q", cc)
	}

	// ---- 授权被撤：403 ----
	empty := []string{}
	if err := e.Access.UpdateGroup(ctx, g.ID, nil, &empty); err != nil {
		t.Fatalf("撤权失败: %v", err)
	}
	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.41", nil, withCredentials(userSess, userCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("撤权后应 403，得到 %d", rec.Code)
	}

	// ---- 缺 CSRF 头（只有 cookie）：403 ----
	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.41", nil, func(req *http.Request) {
		req.AddCookie(userSess)
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺 CSRF 头应 403，得到 %d", rec.Code)
	}

	// ---- 未登录：401 ----
	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.42", nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录应 401，得到 %d", rec.Code)
	}

	// ---- 管理员：无视分组直接放行；未配置的功能：404 ----
	if _, _, err := e.Auth.EnsureAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	adminLogin := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.43",
		map[string]string{"account": "admin", "password": "password123"}, nil)
	adminSess := findCookie(t, adminLogin, httpx.SessionCookie)
	adminCSRF := findCookie(t, adminLogin, httpx.CSRFCookie)
	rec = call(t, engine, http.MethodPost, "/api/features/nas/open", "198.51.100.43", nil, withCredentials(adminSess, adminCSRF))
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员应 200，得到 %d", rec.Code)
	}
	rec = call(t, engine, http.MethodPost, "/api/features/wrench/open", "198.51.100.43", nil, withCredentials(adminSess, adminCSRF))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未配置功能应 404，得到 %d", rec.Code)
	}
}

// TestHTTPAdminAccessAPI 走一遍后台：建组 / 授权 / 成员 / 功能地址 / 越权。
func TestHTTPAdminAccessAPI(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	engine := e.newRouter(t)

	if _, _, err := e.Auth.EnsureAdmin(ctx, "admin", "password123"); err != nil {
		t.Fatalf("建管理员失败: %v", err)
	}
	loginRec := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.44",
		map[string]string{"account": "admin", "password": "password123"}, nil)
	adminSess := findCookie(t, loginRec, httpx.SessionCookie)
	adminCSRF := findCookie(t, loginRec, httpx.CSRFCookie)
	creds := withCredentials(adminSess, adminCSRF)

	// 建组
	rec := call(t, engine, http.MethodPost, "/api/admin/groups", "198.51.100.44", map[string]string{"name": "开发组"}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("建组失败: %d %s", rec.Code, rec.Body.String())
	}
	groupID := uint(decode(t, rec)["group"].(map[string]any)["id"].(float64))

	// 改名 + 授权
	rec = call(t, engine, http.MethodPatch, fmt.Sprintf("/api/admin/groups/%d", groupID), "198.51.100.44",
		map[string]any{"name": "研发组", "features": []string{"nas"}}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新组失败: %d %s", rec.Code, rec.Body.String())
	}

	// 成员
	user, err := e.Auth.Register(ctx, registerInput("member1", e.seedInvite(t)))
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	rec = call(t, engine, http.MethodPut, fmt.Sprintf("/api/admin/groups/%d/members", groupID), "198.51.100.44",
		map[string]any{"userIds": []uint{user.ID}}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("设置成员失败: %d %s", rec.Code, rec.Body.String())
	}

	// 功能：新建（含地址）/ 非法地址拒绝 / 排序 / 删除
	rec = call(t, engine, http.MethodPost, "/api/admin/features", "198.51.100.44",
		map[string]any{"title": "NAS", "desc": "网络存储", "tag": "存储", "icon": "cloud", "url": "http://10.0.0.5:9000/"}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("新建功能失败: %d %s", rec.Code, rec.Body.String())
	}
	featKey := decode(t, rec)["feature"].(map[string]any)["key"].(string)
	rec = call(t, engine, http.MethodPatch, "/api/admin/features/"+featKey, "198.51.100.44",
		map[string]any{"title": "NAS", "desc": "网络存储", "tag": "存储", "icon": "cloud", "url": "javascript:alert(1)"}, creds)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法协议应 400，得到 %d", rec.Code)
	}
	rec = call(t, engine, http.MethodPut, "/api/admin/features/order", "198.51.100.44",
		map[string]any{"keys": []string{featKey}}, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("排序失败: %d %s", rec.Code, rec.Body.String())
	}
	rec = call(t, engine, http.MethodDelete, "/api/admin/features/"+featKey, "198.51.100.44", nil, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("删除功能失败: %d", rec.Code)
	}

	// 列表回读
	rec = call(t, engine, http.MethodGet, "/api/admin/groups", "198.51.100.44", nil, creds)
	if rec.Code != http.StatusOK {
		t.Fatalf("读组失败: %d", rec.Code)
	}
	groups := decode(t, rec)["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("应有 1 个组，得到 %d", len(groups))
	}
	if name := groups[0].(map[string]any)["name"]; name != "研发组" {
		t.Fatalf("组名不符: %v", name)
	}

	// 普通用户访问管理接口 → 403
	loginUser := call(t, engine, http.MethodPost, "/api/auth/login", "198.51.100.45",
		map[string]string{"account": "member1", "password": "password123"}, nil)
	userSess := findCookie(t, loginUser, httpx.SessionCookie)
	userCSRF := findCookie(t, loginUser, httpx.CSRFCookie)
	rec = call(t, engine, http.MethodGet, "/api/admin/groups", "198.51.100.45", nil, withCredentials(userSess, userCSRF))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("普通用户读组应 403，得到 %d", rec.Code)
	}
}

// TestPublicFeatureListHidesURL 公开列表：任何访客可读，但绝不能携带内网地址。
func TestPublicFeatureListHidesURL(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	if err := e.DB.Create(&model.Feature{
		Key: "nas", Title: "NAS", Desc: "网络存储设备", Tag: "存储", Icon: "cloud",
		URL: "http://192.168.11.9:5000/",
	}).Error; err != nil {
		t.Fatalf("造功能失败: %v", err)
	}

	rec := call(t, engine, http.MethodGet, "/api/features", "198.51.100.61", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("公开列表应 200，得到 %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "192.168.11.9") {
		t.Fatal("公开列表不得包含内网地址")
	}
	if !strings.Contains(body, "NAS") {
		t.Fatal("公开列表应含展示字段")
	}
}
