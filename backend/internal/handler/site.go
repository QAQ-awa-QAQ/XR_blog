package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"personal_blog/internal/httpx"
	"personal_blog/internal/service"
)

// SiteHandler 站点内容与布局配置：公开读取 + 管理员整体更新。
// 配置同时可从服务器上的 data/site.json 直接手改 —— 见 service.SiteConfigStore。
type SiteHandler struct {
	store *service.SiteConfigStore
}

func NewSiteHandler(store *service.SiteConfigStore) *SiteHandler {
	return &SiteHandler{store: store}
}

// Public 主页正文配置（欢迎页文案、板块编排与各板块文案）。
// 内容本就是公开的；no-cache 让管理员改完配置后刷新即生效。
func (h *SiteHandler) Public(c *gin.Context) {
	c.Header("Cache-Control", "no-cache")
	httpx.OK(c, h.store.Get())
}

// Update（admin 组：RequireAuth + RequireAdmin + CSRF）整体替换配置。
// 返回规范化后的最新配置，后台可直接用它刷新表单。
func (h *SiteHandler) Update(c *gin.Context) {
	var cfg service.SiteConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", "请求体格式不正确")
		return
	}
	if err := h.store.Update(&cfg); err != nil {
		httpx.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	httpx.OK(c, gin.H{"ok": true, "config": h.store.Get()})
}
