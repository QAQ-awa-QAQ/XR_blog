package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"personal_blog/internal/service"
)

// 文件不存在时：Get 应落盘内置默认配置（4 个板块）并返回。
func TestSiteConfigDefaultAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.json")
	store := service.NewSiteConfigStore(path)

	cfg := store.Get()
	if cfg.Brand == "" || len(cfg.Sections) != 4 {
		t.Fatalf("默认配置不完整: %+v", cfg)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("默认配置应已落盘: %v", err)
	}
	if !strings.Contains(string(raw), `"sections"`) {
		t.Fatal("落盘内容不像配置文件")
	}
}

// 手改文件（mtime 变化）后 Get 应自动重读 —— 不需要重启进程。
func TestSiteConfigHotReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.json")
	store := service.NewSiteConfigStore(path)
	_ = store.Get() // 先生成默认文件

	cfg := service.DefaultSiteConfig()
	cfg.Brand = "热加载测试"
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("手改文件失败: %v", err)
	}
	// 文件系统 mtime 精度可能让"同一秒内的两次写入"不可区分，显式推后
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("调整 mtime 失败: %v", err)
	}

	if got := store.Get(); got.Brand != "热加载测试" {
		t.Fatalf("应热加载到手改值，实际 %q", got.Brand)
	}
}

// Update：校验 + 落盘 + 缓存刷新；非法配置不落盘。
func TestSiteConfigUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.json")
	store := service.NewSiteConfigStore(path)

	cfg := store.Get()
	cfg.Welcome.Title = "新的欢迎标题"
	if err := store.Update(cfg); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件失败: %v", err)
	}
	if !strings.Contains(string(raw), "新的欢迎标题") {
		t.Fatal("更新应已写入文件")
	}
	if store.Get().Welcome.Title != "新的欢迎标题" {
		t.Fatal("缓存应已刷新")
	}

	// 非法配置：应报错且不动文件
	before, _ := os.ReadFile(path)
	bad := store.Get()
	bad.Sections = nil
	if err := store.Update(bad); err == nil {
		t.Fatal("空板块应校验失败")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("校验失败不应改写文件")
	}
}

// 校验矩阵：常见编辑错误都要被拦住，且放行合法配置。
func TestSiteConfigValidate(t *testing.T) {
	find := func(c *service.SiteConfig, typ service.SectionType) *service.SectionConfig {
		for i := range c.Sections {
			if c.Sections[i].Type == typ {
				return &c.Sections[i]
			}
		}
		return nil
	}

	if err := service.DefaultSiteConfig().Validate(); err != nil {
		t.Fatalf("默认配置必须合法: %v", err)
	}

	cases := []struct {
		name string
		edit func(*service.SiteConfig)
	}{
		{"空板块", func(c *service.SiteConfig) { c.Sections = nil }},
		{"重复类型", func(c *service.SiteConfig) { c.Sections = append(c.Sections, c.Sections[0]) }},
		{"版本错误", func(c *service.SiteConfig) { c.Version = 2 }},
		{"欢迎标题过短", func(c *service.SiteConfig) { c.Welcome.Title = "嗨" }},
		{"描述超长", func(c *service.SiteConfig) {
			for i := range c.Sections {
				c.Sections[i].Desc = strings.Repeat("长", 200)
			}
		}},
		{"导航名过短", func(c *service.SiteConfig) { find(c, service.SectionIntro).Label = "" }},
		{"intro 空数据卡", func(c *service.SiteConfig) { find(c, service.SectionIntro).Stats = nil }},
		{"contact 空条目", func(c *service.SiteConfig) { find(c, service.SectionContact).Items = nil }},
		{"javascript 链接", func(c *service.SiteConfig) {
			find(c, service.SectionContact).Items[0].Href = "javascript:alert(1)"
		}},
		{"mailto 无邮箱", func(c *service.SiteConfig) {
			find(c, service.SectionContact).Items[0].Href = "mailto:hello"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := service.DefaultSiteConfig()
			tc.edit(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("%s 应校验失败", tc.name)
			}
		})
	}

	// 「更多」页缺少设置文案时会被自动补齐默认值（不算错误）
	cfg := service.DefaultSiteConfig()
	find(cfg, service.SectionMore).Settings = nil
	if err := cfg.Validate(); err != nil {
		t.Fatalf("应自动补默认设置文案: %v", err)
	}
	if find(cfg, service.SectionMore).Settings == nil {
		t.Fatal("校验后设置文案不应为空")
	}

	// 编辑时留下的全空表格行会被自动剔除
	cfg = service.DefaultSiteConfig()
	intro := find(cfg, service.SectionIntro)
	intro.Stats = append(intro.Stats, service.StatItem{})
	if err := cfg.Validate(); err != nil {
		t.Fatalf("全空行应被剔除而不是报错: %v", err)
	}
	if len(intro.Stats) != 4 {
		t.Fatalf("空行应被剔除，剩 %d 行", len(intro.Stats))
	}
}

// HTTP：公开读取 + 管理端写入（鉴权 / CSRF / 校验错误原样返回）。
func TestSiteHTTP(t *testing.T) {
	e := newEnv(t)
	engine := e.newRouter(t)

	// 1) 公开读取：匿名可读、no-cache、结构完整
	rec := call(t, engine, http.MethodGet, "/api/site", "198.51.100.61", nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("公开读取应 200，实际 %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("应为 no-cache，实际 %q", got)
	}
	var cfg service.SiteConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("响应不是合法配置: %v", err)
	}
	if len(cfg.Sections) != 4 {
		t.Fatalf("默认应有 4 个板块，实际 %d", len(cfg.Sections))
	}

	// 2) 匿名写入 → 401
	payload := service.DefaultSiteConfig()
	payload.Brand = "新站点名"
	rec = call(t, engine, http.MethodPut, "/api/admin/site", "198.51.100.62", payload, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("匿名写入应 401，实际 %d", rec.Code)
	}

	// 3) 管理员缺 CSRF → 403；配对 → 200 且文件更新
	session, csrf := adminCreds(t, e, engine, "198.51.100.63")
	rec = call(t, engine, http.MethodPut, "/api/admin/site", "198.51.100.63", payload, withCookie(session))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("缺 CSRF 应 403，实际 %d", rec.Code)
	}
	rec = call(t, engine, http.MethodPut, "/api/admin/site", "198.51.100.63", payload, withCredentials(session, csrf))
	if rec.Code != http.StatusOK {
		t.Fatalf("管理员更新应 200，实际 %d：%s", rec.Code, rec.Body.String())
	}
	if e.Site.Get().Brand != "新站点名" {
		t.Fatal("更新后应读出新值")
	}
	raw, err := os.ReadFile(e.SitePath)
	if err != nil || !strings.Contains(string(raw), "新站点名") {
		t.Fatalf("更新应写入 %s（%v）", e.SitePath, err)
	}

	// 4) 非法配置 → 400 且错误信息可读
	bad := service.DefaultSiteConfig()
	bad.Welcome.Title = "嗨"
	rec = call(t, engine, http.MethodPut, "/api/admin/site", "198.51.100.63", bad, withCredentials(session, csrf))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法配置应 400，实际 %d", rec.Code)
	}
	if msg, _ := decode(t, rec)["message"].(string); msg == "" {
		t.Fatal("400 应带可读的错误信息")
	}
}
